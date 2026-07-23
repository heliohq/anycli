# Tool design: PandaDoc

Per-tool design for the `helio-tool-provider` pipeline (300-integrations rollout,
Wave 1, catalog row 44). Scratch file on branch `tool/pandadoc`; the batch lead
strips it at batch end.

- **anycli tool id (axis ②):** `pandadoc`
- **Provider catalog key (axis ③):** `pandadoc`
- **CLI command word (axis ①):** `pandadoc` (flat command, no group)
- **Auth lane:** `oauth_light` — re-verified against official docs (§3), stands.
- **Go package:** `internal/tools/pandadoc/`

Because axis ② == axis ③, there is **no** `toolToProvider` entry in
`helio-cli/internal/toolcred/resolver.go`, and no `toolGroups` membership.

## 1. What an AI teammate does with PandaDoc, and the API surface that serves it

PandaDoc is a document workflow / eSignature product. The jobs an AI teammate
actually performs:

1. **Send a document for signature from a template** — "send the NDA template to
   alice@acme.com" is the core loop: pick a template, fill tokens/fields, add
   recipients with roles, create the document, send it.
2. **Track signing status** — "has the contract been signed?" — poll status,
   read details (recipients, dates, fields).
3. **Retrieve the signed artifact** — download the completed (or protected/
   signed) PDF to the workspace so it can be attached, filed, or summarized.
4. **Find things** — list/search documents by status/name, list templates and
   inspect a template's roles/tokens/fields before creating from it.
5. **Light recipient/contact hygiene** — look up or create contacts so repeated
   sends reuse correct names/companies.

All of this rides the PandaDoc **Public API v1**, base URL
`https://api.pandadoc.com/public/v1/` (OAuth endpoints sit outside that prefix,
see §3). Endpoints wrapped:

| Endpoint | Why |
|---|---|
| `GET /documents` | list/filter (q, status, template_id, folder_uuid, count/page, ordering) |
| `GET /documents/{id}` | lightweight status poll (`document.uploaded/draft/sent/viewed/completed/…`) |
| `GET /documents/{id}/details` | full details: recipients, fields, tokens, dates |
| `POST /documents` | create from template (`template_uuid` + recipients + tokens/fields/metadata) |
| `POST /documents/{id}/send` | send for signature (subject/message, `silent` option) |
| `POST /documents/{id}/session` | embedded/shareable signing session link for a recipient |
| `GET /documents/{id}/download` | download PDF (binary → file on disk) |
| `GET /documents/{id}/download-protected` | download the certified/signed PDF |
| `DELETE /documents/{id}` | delete a draft/mistake |
| `GET /templates` | list templates (q, count/page) |
| `GET /templates/{id}/details` | template roles/tokens/fields — needed before create |
| `GET /contacts`, `POST /contacts` | contact lookup/create for recipient reuse |
| `GET /members/current` | identity/whoami (also the Helio identity endpoint, §4) |

Deliberately **out of scope** for v1 of this tool: content library, forms,
quotes/catalog, notary, webhooks (webhooks are a server concern, not a CLI
verb), bulk/AI-beta endpoints, folders CRUD, document editing sessions. An
`api` escape hatch (notion precedent) covers the long tail without growing the
verb surface.

**Async-create semantics (real-world gotcha):** `POST /documents` returns with
`status: document.uploaded`; the document is only sendable once background
processing flips it to `document.draft`. `document create` therefore polls
`GET /documents/{id}` until `document.draft` (bounded, ~60 s, 2 s interval)
before returning, with `--no-wait` to skip. `document send` on a still-uploading
id surfaces the provider's error as-is (no hidden retry).

## 2. anycli definition and service implementation

**Stage-1 form decision: `service` type.** PandaDoc ships no official CLI (only
HTTP SDKs: python/node/java/php/go clients), so the `cli`-type rubric fails at
the first gate. Implement `service` type against the HTTP API — matching 21 of
23 existing definitions.

### Definition — `definitions/tools/pandadoc.json`

```json
{
  "name": "pandadoc",
  "type": "service",
  "description": "PandaDoc as a tool (documents, templates, eSignature)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "PANDADOC_ACCESS_TOKEN"}
      }
    ]
  }
}
```

PandaDoc has two native auth header schemes: `Authorization: Bearer {token}`
(OAuth) and `Authorization: API-Key {key}`. Helio ships the **oauth_light** lane,
so the only credential the Helio provider projects — and therefore the only one
this definition declares — is `access_token` (Bearer). We deliberately do **not**
declare a second `api_key` binding: `anycli.ListTools()` surfaces every declared
credential field, and Helio's `TestGeneratedToolProvidersMatchPinnedAnyCLI`
requires each to be projected by the provider bundle, so an `api_key` field the
OAuth bundle can never supply would fail that parity gate (there is no valid
credential source in an OAuth connection that yields an API key). Keeping the
definition Bearer-only matches what production actually supplies and removes a
dead code path. Service auth: `PANDADOC_ACCESS_TOKEN` → `Bearer`; unset → fail
fast with the notion-style usage error ("PANDADOC_ACCESS_TOKEN is not set"). 401
responses are classified via `execution.RejectCredential` (notion
`auth_error.go` precedent) so the host can invalidate cached credentials.

### Command tree (cobra, notion shape: `BaseURL`/`HC`/`Out`/`Err` struct)

```
pandadoc document list      [--q --status --template --folder --count --page --order]
pandadoc document create    --template <uuid> --name <name>
                            --recipient email[:role[:first[:last]]] (repeatable)
                            --token name=value (repeatable)
                            [--field name=value] [--metadata k=v] [--no-wait]
                            [--body / --body-file]   # full JSON payload escape hatch (mutually exclusive with the flags)
pandadoc document status    <id>
pandadoc document details   <id>
pandadoc document send      <id> [--subject --message --silent]
pandadoc document link      <id> --recipient <email> [--lifetime <seconds>]
pandadoc document download  <id> --out <path> [--protected]
pandadoc document delete    <id>
pandadoc template list      [--q --count --page]
pandadoc template details   <id>
pandadoc contact list       [--email]
pandadoc contact create     --email <email> [--first --last --company --phone]
pandadoc whoami
pandadoc api                <METHOD> <path> [--body|--body-file] [--query k=v]   # raw passthrough, notion api precedent
```

### Output and exit-code contract (notion precedent, verbatim)

- Exit 0 success; exit 1 runtime/API failure (typed `apiError` carrying HTTP
  status); exit 2 usage/parse errors (typed `usageError` + cobra parse errors).
- Every command supports `--json`. Default output is concise human-readable
  text (id, name, status one-liners for lists); `--json` prints the provider's
  JSON response body (passthrough, not re-modeled) so agents get the full
  fidelity of the API. `document download` prints `{"path": "...", "bytes": N}`
  under `--json`.
- Errors under `--json` use the structured error envelope on stderr (notion
  `renderError` shape); plain mode prints the message to stderr.

## 3. Auth flow — oauth_light verification against official docs

Audit verdict was `oauth_light`, confidence `medium`, which mandates this
stage-1 re-check. **Verified against developers.pandadoc.com on 2026-07-21 —
the lane stands.** Findings, from the official pages
(`reference/authentication-process`, `reference/authorize-a-user`,
`reference/state-parameter`, `reference/access-token`,
`reference/api-key-authentication-process`):

- **Model:** RFC 6749 authorization-code flow. App creation ("Create an
  Application") is fully self-serve in the Developer Dashboard
  (`app.pandadoc.com/a/#/settings/api-dashboard/configuration`); no review or
  publish gate is documented for external users to authorize. The authorize
  step is explicitly a browser flow associating **any PandaDoc user** with the
  app. ⇒ multi-tenant, self-serve, no review ⇒ `oauth_light` confirmed.
- **Authorize URL:** `https://app.pandadoc.com/oauth2/authorize` with
  `client_id`, `redirect_uri`, `scope=read+write`, `response_type=code`, and a
  recommended `state` parameter (integration-service supplies state itself).
  `read+write` is `+`-encoded form data, i.e. the two scopes `read` and
  `write`.
- **Token URL:** `POST https://api.pandadoc.com/oauth2/access_token`,
  `Content-Type: application/x-www-form-urlencoded`, params `grant_type=
  authorization_code`, `client_id`, `client_secret`, `code`, optional `scope`.
  Client credentials go in the **form body** ⇒ `token_exchange_style:
  form_secret`. No PKCE is documented ⇒ `pkce: none`.
- **Token semantics:** response carries `access_token`, `refresh_token`,
  `expires_in`. Docs state `expires_in` is currently **31535999 s (1 year)**.
  Refresh: on 401, `grant_type=refresh_token` with the stored refresh token.
  Refresh-token lifetime is not documented; docs do not describe single-use/
  rotating refresh tokens ⇒ `refresh_lease: none` (google precedent), not
  `provider` (the X rotating-lease case). If the L4 refresh exercise observes
  rotation, revisit before the visible flip and note it here.
- **No OAuth revoke endpoint** is documented ⇒ `disconnect_mode: local_only`
  (bitly/notion precedent), no `revoke:` block.
- **API request auth:** `Authorization: Bearer {access_token}`.

**Divergences from catalog/audit: none.** The audit's open question
(cross-account distribution not spelled out) resolves in favor of oauth_light:
the authorize flow is documented as linking any PandaDoc user to the app's
`client_id`. Caveats worth recording (they gate lanes 1/2, not the lane
choice):

- Developer Dashboard / API access requires a PandaDoc plan with API access —
  the lane-2 test workspace must be provisioned with it (Enterprise trial or
  API add-on).
- **Production API keys** require Sales approval — irrelevant here: we ship the
  OAuth lane only, and the definition is Bearer-only, so no PandaDoc API key is
  ever involved (L2 uses an OAuth access token from the lane-1 dev app).
- Sandbox documents carry a watermark; fine for L2/L4.

## 4. Helio provider bundle plan

`integrations/providers/pandadoc/provider.yaml` — **hidden-first**, standard
golden path, zero Helio service code (`standard_oauth`; nothing in §3 falls
outside the declarative capability set — no adapter):

```yaml
schema: helio.provider/v1
key: pandadoc
go_name: PandaDoc

presentation:
  name: PandaDoc
  description_key: pandadoc
  consent_domain: pandadoc.com
  visible: false        # hidden-first; flip + regen is the single go-live change
  order: 200            # batch lead may renumber at batch-end merge

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.pandadoc.com/oauth2/authorize
    token_url: https://api.pandadoc.com/oauth2/access_token
    token_exchange_style: form_secret
    pkce: none
    scopes: [read, write]        # joined on the wire as scope=read+write
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://api.pandadoc.com/public/v1/members/current
  stable_key: /user_id
  label_candidates: [/email, /user_id]

connection:
  mode: isolated
  disconnect_mode: local_only     # no documented OAuth revoke endpoint
  runtime_strategy: standard_oauth

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: pandadoc
  kind: oauth
```

Identity notes: `GET /public/v1/members/current` returns the authenticated
member (`user_id`, `membership_id`, `email`, `first_name`, `last_name`,
`workspace_id`, `role`, …) and accepts both auth schemes. `user_id` is the
stable per-user key; `membership_id`/`workspace_id` are workspace-scoped and
would fragment one user across workspaces, so they are not the stable key.
Exact field paths are re-verified against a live response during L4 before the
bundle is finalized (docs page for this endpoint was rate-limited during
research; fields corroborated via the official python SDK MembersApi).

The bundle's `credential.fields` maps `access_token` (+ `account_key`), which is
exactly the definition's single declared credential field (§2) — the two stay in
parity so `TestGeneratedToolProvidersMatchPinnedAnyCLI` passes once the pin ships
the tool.

Other Helio-side artifacts (batch-end surfaces, per master plan §2):

- **Generation:** run `provider-gen` + `--check` locally for on-branch L3/L4
  validation only; projections are NOT committed from this branch — the batch
  lead produces the canonical regen.
- **OAuth config:** lane 1 registers the PandaDoc dev app (self-serve, redirect
  URI = integration-service callback) and distributes client id/secret as
  uncommitted local `config/cloud.yaml` entries for L4; the committed
  `config/` + `deploy/` Helm Secret appends land together (Config Sync rule)
  before L5.
- **Icon:** `ui/helio-app/src/integrations/icons/pandadoc.svg` + manual
  registration in `providerIcons.ts` (batch-end append).
- **AI-facing docs:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` (notion sub-doc precedent) covering the
  send-from-template loop, the uploaded→draft async gotcha, and download-to-
  workspace; plugin version bump + marketplace publish ride the batch-end
  merge.
- **helio-cli:** no code — flat generated command from the bundle; pin bump is
  the batch lead's. On-branch builds use a local, uncommitted `go.mod`
  `replace github.com/heliohq/anycli => ../../../anycli/.claude/worktrees/tool-pandadoc`.

## 5. Test plan (five layers)

| Layer | Plan | External credentials needed |
|---|---|---|
| **L1** | anycli unit tests, TDD-first, httptest fake of `api.pandadoc.com`: assert request paths/query/body shape for every verb; Bearer header injection; 401 → `RejectCredential` classification; uploaded→draft polling in `document create` (fake flips status on Nth poll) and `--no-wait`; download writes bytes to `--out`; `--json` passthrough and error envelope; exit codes 0/1/2. `go test ./...` green. | None |
| **L2** | Dev harness against the REAL API with an OAuth bearer token: `ANYCLI_CRED_ACCESS_TOKEN=<access token> anycli pandadoc -- template list`, then the full loop — `document create` from a real sandbox template, `status`, `send` (silent), `link`, `download`, `whoami`. Success = real data back from `api.pandadoc.com`. (The definition is Bearer-only — §2 — so a PandaDoc `API-Key` key is not a harness credential; mint the token from the dev app registered in lane 1.) | **Yes — lanes 1+2**: registered PandaDoc dev app + a minted OAuth access token; PandaDoc test workspace with API access and one template with at least one signer role |
| **L3** | Local `provider-gen` + `provider-gen --check` against the branch bundle (not committed); helio-cli build + `go test ./cmd/heliox/cmds/tool/` with the local `replace`; anycli + integration-service unit suites. Branch CI `--check` red is expected until batch-end regen. | None |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with a REAL OAuth `access_token` **and** `refresh_token` from the dev app, seeded with a deliberately short `expires_at` so the first `heliox tool pandadoc -- template list` exercises the token-gateway refresh-and-write-back path (PandaDoc's 1-year `expires_in` would otherwise never exercise it). Use real seeded org/assistant ObjectIDs. Then run the document loop end-to-end as the assistant. Verify whether the refresh response rotates the refresh token (feeds the `refresh_lease` decision, §3). | **Yes — lane 1**: registered PandaDoc dev app (client id/secret in local uncommitted `config/cloud.yaml`) + one manually minted token pair from the test account |
| **L5** | Human-in-the-loop (lane 3, per-batch sweep, OAuth path): `heliox tool pandadoc auth` → connect link → real PandaDoc consent (`app.pandadoc.com/oauth2/authorize`, scope read+write) → `oauth_connected` event on the channel → one unseeded live command (`document list`). Gates the visible flip. | **Yes — lanes 1+3**: configured (committed) client id/secret + a human on the pool test account |

Definition of done follows master plan §2: all five layers green, docs
published, icon registered, then `visible: true` + regenerate as the single
go-live change.
