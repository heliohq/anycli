# Tool design: SignNow (`signnow`)

Catalog row 220 — anycli id `signnow`, provider catalog key `signnow`, auth lane
`oauth_light`, wave 3, category Scheduling & eSign. Scratch design file on
branch `tool/signnow`; the batch lead strips it at batch-end.

## 1. Verified auth model (official docs vs catalog)

Verified against the official docs (`docs.signnow.com` — JS-rendered, so
corroborated with SignNow's official Postman collection
`github.com/signnow/postman-collection` and the official Node SDK reference,
both maintained by SignNow):

- **OAuth 2.0 authorization-code flow, confirmed.**
  - Authorize: `GET https://app.signnow.com/authorize?client_id={id}&redirect_uri={uri}&response_type=code`.
    No wire-level `scope` parameter on the authorize URL — app access is
    all-or-nothing per account (token scope comes back as `*`).
  - Token: `POST https://api.signnow.com/oauth2/token`,
    `Content-Type: application/x-www-form-urlencoded`, client authenticated via
    `Authorization: Basic base64(client_id:client_secret)` (the "basic
    authorization token" in SignNow's docs; Basic auth is valid **only** at the
    token endpoint). Body: `grant_type=authorization_code&code=...`.
  - Response: `access_token`, `refresh_token`, `expires_in` (default 30 days),
    `token_type`, `scope`.
  - Refresh: same endpoint, `grant_type=refresh_token`. **Refresh tokens are
    single-use (rotating)** — a concurrent double-refresh breaks the chain and
    forces re-auth. This drives `refresh_lease: credential` below.
  - Revoke: `DELETE /oauth2/token` — a non-standard HTTP method the declarative
    revoker cannot express, so disconnect is `local_only` (bitly/notion
    precedent).
- **Registration model, confirmed `oauth_light`.** App registration is
  self-serve in the API dashboard (sandbox account at signnow.com/developers);
  switching an app from Development to Live is self-serve **but requires a paid
  API plan** (from ~$84/mo). No app review or partner gate. The audit verdict
  (`oauth_light`, medium confidence) is **confirmed**; no divergence.
- **Two isolated environments.** Sandbox: `https://api-eval.signnow.com` /
  `https://app-eval.signnow.com` — free, unlimited, fully functional, but
  **shares no data or keys with production**. Production:
  `https://api.signnow.com` / `https://app.signnow.com`. The provider bundle
  bakes production URLs; consequence for testing is in §5.

## 2. What the tool wraps (AI-teammate driven surface)

What an AI teammate actually does with SignNow: send a document out for
signature (upload, or instantiate a template), track who has signed, nudge or
cancel pending signers, pull down the executed PDF, and mint a signing link
when there is no known signer email. That maps to this v1 endpoint set (all
verified in the official Postman collection):

| Verb | Endpoint | Why |
|---|---|---|
| `GET /user` | whoami / identity | account sanity check |
| `GET /user/documentsv2` | list **modified / in-flight** documents (fields, texts, signatures added) | one half of `document list` |
| `GET /user/documents` | list **freshly-uploaded, untouched** documents | the other half — a just-uploaded doc lives here until its first edit |
| `POST /document` | upload PDF/DOCX | start a signature flow |
| `POST /document/fieldextract` | upload with text-tag field extraction | tagged docs skip manual field placement |
| `GET /document/{id}` | document detail: roles, fields, `field_invites` + statuses, signatures | the status-tracking primitive |
| `PUT /document/{id}` | add fillable fields (signature/text/date, x/y/role) | fields required before a role-based invite |
| `DELETE /document/{id}` | delete | cleanup |
| `GET /document/{id}/download?type=collapsed` | download executed PDF (`with_history=1` optional) | deliver the signed artifact |
| `POST /document/{id}/invite` | field invite (role-based `to[]`) or free-form invite (no fields) | the core "send for signature" action |
| `PUT /fieldinvite/{id}/resend` | resend/remind | nudge a pending signer |
| `PUT /document/{id}/fieldinvitecancel` | cancel field invite | recall a sent doc |
| `POST /template` | document → reusable template | recurring agreements |
| `POST /template/{id}/copy` | template → fresh document | recurring agreements |
| `POST /link` | signing link for a document | no-email signing |

Deliberately out of v1 (add on demand): document groups + group invites, bulk
invite, embedded invites, webhooks (`/api/v2/events` — Helio has no callback
receiver in this path), folders, smart fields, merge/history.

## 3. anycli definition & service

**Stage-1 rubric: `service` type.** SignNow ships SDKs, not an official CLI —
no binary to wrap, so the default holds (21/23 precedent).

- `definitions/tools/signnow.json`:

  ```json
  {
    "name": "signnow",
    "type": "service",
    "description": "SignNow e-signature: send documents for signing, track invites, download signed PDFs",
    "auth": {
      "credentials": [
        {"source": {"field": "access_token"},
         "inject": {"type": "env", "env_var": "SIGNNOW_ACCESS_TOKEN"}},
        {"source": {"field": "api_base_url"},
         "inject": {"type": "env", "env_var": "SIGNNOW_API_BASE_URL"}}
      ]
    }
  }
  ```

  `api_base_url` is an **optional** binding: `ApplyBindings` skips empty
  values (verified in `internal/credential/inject.go` +
  `TestApplyBindings_SkipsEmptyValues`), and a resolver that lacks the field
  yields empty — so Helio's token gateway never sets it and the service falls
  back to production, while the L2 harness sets
  `ANYCLI_CRED_API_BASE_URL=https://api-eval.signnow.com` to target the free
  sandbox. No definition-schema change needed.

- `internal/tools/signnow/` (package `signnow`), registered as
  `RegisterService("signnow", &signnow.Service{})` in
  `internal/tools/register.go` (registry entry rides the batch-end merge; the
  package + definition merge freely). Copy the notion/bitly shape: `Service`
  struct with `BaseURL`/`HC`/`Out`/`Err`, `DefaultBaseURL =
  "https://api.signnow.com"` (env-map `SIGNNOW_API_BASE_URL` overrides, then
  struct field for httptest), exit-code contract 0/1/2, typed `apiError`
  surfacing SignNow's JSON error bodies (`{"errors":[{"code","message"}]}` and
  legacy `{"error": "..."}` — both dialects exist), `--json` structured error
  envelope per the built-in service conventions (anycli design 003 §3).

**Subcommand tree** (resource-grouped, notion precedent):

```
signnow whoami
signnow document list [--limit N]            # fans out to /user/documentsv2 + /user/documents, merged & deduped by id
signnow document get <document-id>            # incl. roles, invite statuses
signnow document upload --file <path> [--extract-fields] [--name <s>]
signnow document add-fields <document-id> --fields <json>   # PUT /document/{id}
signnow document download <document-id> --out <path> [--with-history]
signnow document delete <document-id>
signnow invite send <document-id> --to <json>|--email <addr> [--subject <s>] [--message <s>] [--no-email]
signnow invite resend <field-invite-id>
signnow invite cancel <document-id>
signnow template create <document-id> --name <s>
signnow template copy <template-id> --name <s>
signnow link create <document-id>
```

`document list` covers the account fully, not just in-flight docs. SignNow
splits listing across two endpoints (verified in the official Postman
collection): `GET /user/documentsv2` returns documents that "were modified
(added fields, texts, signatures, etc.)", while `GET /user/documents` returns
documents "that were not modified yet". A doc an agent (or a human) just
uploaded and has not yet fielded/invited lives **only** in the latter — so a
single-endpoint `documentsv2` list would silently hide exactly the docs the
"find the doc to act on" flow most needs. `document list` therefore fetches
both (concurrently), merges, and dedupes by `id` (a doc mid-transition can
appear in both). The L2 loop asserts this coverage empirically (§5). No
`--modified-only`/`--unmodified` split filter in v1 — add on demand; the merged
default is the correct agent primitive.

`invite send` inspects intent, not state: `--to <json>` sends a role-based
field invite (roles array with `email`, `role`/`role_id`, `order`); bare
`--email` sends a free-form invite (documents without fields only — SignNow
rejects free-form on fielded docs, surfaced as the API's own error, no client
guessing). `--no-email` appends `?email=disable` (embedded-style suppress).

**JSON output shape:** `--json` on every verb. Lists render
`{"documents":[{"id","document_name","created","updated","field_invites":[{"id","email","status"}]}]}`
(trimmed projection of the raw payload — SignNow's raw document object is
hundreds of lines; project the fields an agent acts on: ids, names,
role/invite/status, timestamps). Mutations echo `{"id": ...}` plus the
provider's status field. `download` writes the file and prints
`{"saved_to": "...", "bytes": N}`.

## 4. Helio provider bundle (`integrations/providers/signnow/provider.yaml`)

Naming axes: ① CLI command `signnow` (flat, no group) = ② anycli id `signnow`
= ③ provider key `signnow` — **no `toolToProvider` entry** (identity holds).

```yaml
schema: helio.provider/v1
key: signnow
go_name: SignNow

presentation:
  name: SignNow
  description_key: signnow
  consent_domain: signnow.com
  visible: false          # hidden-first; flip is the go-live change
  order: <next free>

auth:
  type: oauth
  owner: individual       # provider sees a person's SignNow account
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.signnow.com/authorize
    token_url: https://api.signnow.com/oauth2/token
    token_exchange_style: form_basic   # form body + Basic client auth (official docs)
    pkce: none
    single_active_token: false
    refresh_lease: credential          # single-use rotating refresh tokens — requires the §4a
                                        # standard_oauth capability growth to land first
    # No wire-level scope param (all-or-nothing account access, token scope "*").
    # Display-only capability slugs, bitly/notion pattern:
    display_scopes: [send_documents, track_invites, download_signed, manage_templates]

identity:
  source: userinfo
  url: https://api.signnow.com/user
  stable_key: /id
  label_candidates: [/primary_email, /id]

connection:
  mode: isolated
  disconnect_mode: local_only   # SignNow revoke is DELETE /oauth2/token — not expressible declaratively
  runtime_strategy: standard_oauth   # generic path (no per-provider adapter), but its compiled
                                      # capability set must first admit refresh_lease: credential (§4a)

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: signnow
  kind: oauth
```

Notes:
- `refresh_lease: credential` (not `provider`): rotation invalidates only the
  same credential's chain, not other accounts under the app — per-credential
  serialization is exactly `OAuthLeaseCredential`'s contract
  (`model/catalog.go`). The runtime for this is already built —
  `service/token_refresh.go` acquires a Mongo lease keyed
  `refresh:<provider>:<credential_id>` when the scope is `OAuthLeaseCredential`
  (verified: `token_refresh.go` lines 80–113). What is **missing** is the
  compiled contract's permission to pair this scope with `standard_oauth`; that
  gap is a shared integration-service change scoped in §4a. x's `provider`
  scope is the single-active-token case, which SignNow is not.
- `standard_oauth` (generic path, no per-provider adapter): Basic-auth form
  exchange is `form_basic`, identity is a plain `GET /user` userinfo probe — no
  adapter justification. But this is **not** "zero service code": the bundle
  fails the design's own L3 gate (`provider-gen --check`) and integration-service
  startup until the §4a capability growth lands, because the compiled contract
  currently pins `standard_oauth` to `refresh_lease: none`. Verify
  `/primary_email` on the live L5 response; fall back to `/id` label if absent.
- Config: `oauth.client_id`/`oauth.client_secret` under the `signnow` provider
  key in `config/` + the `deploy/` Helm Secret together (lane 1 lands them;
  partial config fails startup).
- UI icon `ui/helio-app/src/integrations/icons/signnow.svg` + manual
  `providerIcons.ts` registration; provider sub-doc under
  `agents/plugins/heliox/skills/tool/` — both ride the batch-end merge.
- Do **not** commit provider-gen projections from this branch; run
  `go run ./cmd/provider-gen && go run ./cmd/provider-gen --check` locally for
  validation only (master plan §2). `--check` is expected red in CI until the
  batch lead's canonical regen.

### 4a. Prerequisite Helio-side capability change (blocks L3)

**This is a shared integration-service change for the batch lead, not a
self-contained bundle.** The `refresh_lease: credential` choice above is correct
for SignNow's single-use rotating refresh tokens (a concurrent double-refresh
breaks the chain — verified against SignNow's official OAuth docs), but the
compiled runtime contract does not currently admit that pairing under
`standard_oauth`:

- `go-services/integration-service/model/runtime_contract.go` (line 42):
  `RuntimeStrategyStandardOAuth` declares
  `oauth: &oauthRuntimeContract{singleActiveToken: false, refreshLeaseScope: OAuthLeaseNone}`
  — a **single** allowed lease scope.
- `ValidateRuntimeContract` (line 224) rejects any bundle whose
  `auth.oauth.refresh_lease` differs from that one value with a scalar `!=`
  check, and `provider-gen` runs the same validation via
  `runtimeContractDefinition` → `ValidateRuntimeContract`
  (`cmd/provider-gen/validate.go`).
- Two tests currently **assert** the rejection of exactly this bundle's pairing:
  `service/provider_registry_test.go` line 358 and
  `cmd/provider-gen/generator_test.go` line 357 both mutate a standard_oauth
  definition to `OAuthLeaseCredential` and expect the error `refresh_lease "none"`.

So this bundle (correctly `refresh_lease: credential`) **fails
`provider-gen --check` and integration-service startup** with no code change,
and there is no adapter escape hatch. The runtime work is already done
(`token_refresh.go`, above); only the contract gate needs to grow. The fix is
a small, orthogonal compiled-capability growth (per `provider-yaml.md`: grow the
generic `standard_oauth` capability set rather than fork an adapter):

1. Turn the contract's single `refreshLeaseScope` field into an **allowed-set**
   of lease scopes (mirroring `disconnectModes []DisconnectMode`, already a list
   validated with `slices.Contains`). Set `standard_oauth`'s allowed set to
   `{OAuthLeaseNone, OAuthLeaseCredential}` — `none` stays the default for
   existing standard providers; `credential` becomes admissible for
   rotating-refresh-token providers. `single_active_token` stays `false` (the
   lease is orthogonal to single-active activation), and X's narrow
   `x_exclusive_grant` strategy keeps its own exact `{OAuthLeaseProvider}` set —
   this change does not widen X.
2. Update `ValidateRuntimeContract`'s lease check (scalar `!=` → set membership)
   and the `manifestOAuthLease` error rendering to list the allowed set.
3. Flip the two rejection tests (`provider_registry_test.go` line 358,
   `generator_test.go` line 357) from *expecting rejection* to *accepting*
   `credential` under standard_oauth (X's "X refresh lease" case is unchanged),
   and add a positive contract-level test for the newly-admitted pairing.

**Merge ordering.** This is an integration-service source change, **not** one of
the batch-end shared surfaces (master plan §2) — it does not touch
`register.go`, the anycli pin, the five `provider-gen` projections,
`toolToProvider`, `providerIcons.ts`, the plugin publish, or the OAuth config
appends. It is a per-tool compiled-capability prerequisite, in the same class as
this batch's other integration-service capability items (dataforseo's
query-param api_key, hubspot's numeric stable-key, salesforce's instance_url
capture). **It is also shared with keap** (whose bundle takes the identical
`standard_oauth` + `refresh_lease: credential` pairing and scopes the same change
in its own §3a) — the two must land as **one** reviewed integration-service
change, not two conflicting edits to `runtime_contract.go`. It should be
reviewed and merged on its own, ahead of the batch-end bundle merge, so the
batch lead's single canonical `provider-gen` validates. Until it lands, this
branch's local L3 (`provider-gen --check` against the branch bundle) is expected
to fail specifically on the lease pairing — a signal distinct from the ordinary
batch-end regen red noted in §4.

## 5. Test plan (five layers)

| Layer | Plan | External credentials needed |
|---|---|---|
| L1 | anycli unit tests, httptest fakes (notion `newMux` pattern): request shape per §2 endpoint, `Authorization: Bearer` injection, multipart upload body, field-invite vs free-form payload split, `--json` success + both error dialects, exit codes 0/1/2. TDD first. | none |
| L2 | `make build-harness`; `ANYCLI_CRED_API_BASE_URL=https://api-eval.signnow.com ANYCLI_CRED_ACCESS_TOKEN=<eval token> anycli signnow -- document upload/get/invite send/download ...` against the **free eval sandbox**. Eval token minted once via `POST /oauth2/token` with the sandbox app's Basic token + `grant_type=password` (official sandbox bootstrap path). Full loop: upload → add-fields → invite → resend → cancel → template → link → download → delete. **List-coverage assertion:** upload a fresh doc and, *before* touching it, run `document list` and assert the new id appears (proves the `/user/documents` unmodified leg); then add-fields/invite and assert it still appears (the `/user/documentsv2` modified leg) — confirming the merged+deduped list covers both states, per §2. | **yes** — free SignNow sandbox account (account pool; self-serve at signnow.com/developers) |
| L3 | local `provider-gen` + `--check` against this bundle; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with an **uncommitted** `go.mod` `replace github.com/heliohq/anycli => /Users/wenfeng/workspace/helio/anycli/.claude/worktrees/tool-signnow`; integration-service unit suite. Note: `provider-gen --check` is **expected red on the lease pairing** until the §4a capability growth lands — validate green against a local integration-service tree that already carries the §4a change. | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `signnow`, real ObjectID identities) seeding `access_token` + `refresh_token` with short `expires_at` to force the gateway refresh path (form_basic + rotation write-back, `refresh_token_rotated` log); then `heliox tool signnow -- document list` must return live data. **Environment caveat:** the bundle/service bake production URLs, so the seeded token must be a **production** token — requires lane 1's registered app in **Live mode, i.e. a paid SignNow API plan**, not just the free sandbox. | **yes** — production (Live) app client id/secret as uncommitted local `config/cloud.yaml` entries + a production-account token pair (lane 1 + account pool) |
| L5 | hidden pre-flip run: `heliox tool signnow auth` → connect link → SignNow consent on the Live app → `oauth_connected` event on the channel → one unseeded `document upload` + `invite send` round trip. Confirms identity extraction (`/id`, `/primary_email` label). Human-in-the-loop (oauth lane 3), after lane 1's config append lands. | **yes** — same Live app + a pool SignNow account |

**Flag for lane 1 / batch lead (recorded divergence-adjacent finding):** unlike
most oauth_light providers, SignNow's dev-mode (sandbox) app cannot exercise
L4/L5 because sandbox and production are fully isolated stacks and the bundle
bakes production URLs — the "dev app" that gates L4 here is a **Live-mode app
on a paid API plan** (~$84/mo). Budget it in the account pool for wave 3's
eSign batch (shared across signnow L4+L5; the free sandbox still covers all of
L2).

## 6. Rollout

Hidden-first: bundle lands `visible: false` in the batch-end merge; visible
flip (+ the one canonical regen) only after L5, as the single go-live change.
No experiment flag (GA path). No resolver entry, no group. Docs sub-doc +
plugin version bump ride the batch publish.
