# Keap — per-tool design (scratch, batch lead strips at batch end)

**Tool:** Keap (formerly Infusionsoft) — small-business CRM + marketing automation.
**Catalog row:** #64 · anycli id `keap` (axis ②) · provider key `keap` (axis ③) · lane `oauth_light` · wave 2 · category CRM.
**Branches:** anycli `tool/keap` (this worktree) · Helio `tool/keap`.

## 0. Lane verification & audit divergence

Keap has **no row in `oauth-audit.md`** — that audit's scope was the 250 tools sitting in the
`api_key` lane pre-audit, and Keap was already `oauth_light` in the base catalog. Verified the
lane independently against official docs; **`oauth_light` is confirmed**, no divergence:

- Multi-tenant authorization-code OAuth2 exists and is the documented third-party path
  (developer.infusionsoft.com/authentication/ — same content mirrors on developer.keap.com).
  One registered app; any Keap account's user can authorize it.
- Registration is self-serve at the developer portal (`keys.developer.keap.com` → My Apps →
  "+ NEW APP" → enable the Keap API product → key/secret issued immediately). No review gate
  before external accounts can authorize.
- Caveat for lane 1 (not a lane change): Keap auto-approves a developer account's **first**
  app key; a second (e.g. prod vs test split) needs manual approval, and >2 needs a support
  ticket. Plan: one dev-mode app for L4/L5 now; the prod app registration is a one-time lane-1
  follow-up before go-live if we want separate dev/prod clients.
- Simpler credentials exist (Personal Access Tokens / Service Account Keys via
  `X-Keap-API-Key`) but are explicitly positioned for first-party/self integrations; official
  guidance for third-party integrations is the OAuth2 access-code flow. They also carry
  lower/tenant-admin-shaped semantics. OAuth stays the right lane; PATs are still handy as an
  L2 shortcut — but note L2 must exercise the **same header the definition injects**, so the
  harness runs on a real OAuth access token, not a PAT (see §5).

## 1. Official API surface wrapped, and why

**Target: REST v2** — `https://api.infusionsoft.com/crm/rest` base, `/v2/...` paths
(developer.keap.com/docs/restv2/, OpenAPI at `/docs/restv2/2025-11-05-v2.json`). Rationale:
v2 is the portal's "Default"; v1 is frozen ("no other improvements"); XML-RPC sunsets
2026-12-31 with brownouts through 2026. Building anything on v1 now would buy a migration.
Auth on every call: `Authorization: Bearer <access_token>`.

Scope of the tool = what an AI teammate actually does with a CRM: look up and maintain
contacts/companies, segment with tags, move deals, track follow-up tasks and notes, drop
contacts into automations, and send the odd one-off email. E-commerce (orders, subscriptions,
products, affiliates), reporting, files, and settings endpoints are **out of v1 scope** —
they're a storefront/back-office surface, not teammate work; extend later if demand shows.

Wrapped endpoints (all REST v2):

| Area | Endpoints |
|---|---|
| Contacts | `GET/POST /v2/contacts`, `GET/PATCH/DELETE /v2/contacts/{id}` (list supports `filter` incl. `email`/`given_name`/`family_name`, `order_by`, `fields`, `page_size`, `page_token`) |
| Companies | `GET/POST /v2/companies`, `GET/PATCH /v2/companies/{id}` |
| Tags | `GET/POST /v2/tags`, `GET /v2/tags/{id}`, `GET /v2/tags/{id}/contacts`, `POST /v2/tags/{id}/contacts:applyTags`, `POST /v2/tags/{id}/contacts:removeTags` |
| Opportunities | `GET/POST /v2/opportunities`, `GET/PATCH /v2/opportunities/{id}`, `GET /v2/opportunities/stages` |
| Tasks | `GET/POST /v2/tasks`, `GET/PATCH/DELETE /v2/tasks/{id}` (create requires `assigned_to_user_id`) |
| Notes | `GET/POST /v2/contacts/{id}/notes`, `GET/PATCH /v2/contacts/{id}/notes/{note_id}` |
| Email | `POST /v2/emails:send` (requires `contacts`, `subject`, `user_id`), `GET /v2/emails` |
| Automations | `GET /v2/automations`, `GET /v2/automations/{id}`, `POST /v2/automations/{id}/sequences/{seq}:addContacts`; legacy campaigns read via `GET /v2/campaigns`, `GET /v2/campaigns/{id}` |
| Users | `GET /v2/users`, `GET /v2/oauth/connect/userinfo` (as `keap user me`) |

Rate limits (documented): ~10,000 req/min and 250,000 req/day per application instance,
25 req/s spike policy; throttle headers `x-keap-tenant-throttle-available` /
`x-keap-product-quota-available`; 429 + `retry-after` on breach. The tool does not retry
internally (fail fast, surface the provider error).

## 2. anycli definition

**Stage-1 rubric: `service` type.** No official Keap CLI exists (official SDKs are
codegen libraries only), so the `cli` conditions fail at the first gate. HTTP service
implementation it is (matching 21/23 precedents).

`definitions/tools/keap.json`:

```json
{
  "name": "keap",
  "type": "service",
  "description": "Keap CRM as a tool (OAuth access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "KEAP_ACCESS_TOKEN"}
      }
    ]
  }
}
```

**Package:** `internal/tools/keap/` (id has no dashes; package name == id), registered as
`RegisterService("keap", &keap.Service{})` in `internal/tools/register.go` — registration
rides the **batch-end merge**; the definition JSON and package merge freely mid-batch.

**Service shape** (notion/bitly precedent): `Service{BaseURL, HC, Out, Err}` with zero values
defaulting to `https://api.infusionsoft.com/crm/rest` and `http.DefaultClient`; cobra tree
built per `Execute` call; missing `KEAP_ACCESS_TOKEN` → exit 1 with explicit message.

**Cobra tree** (resource → verb):

```
keap contact   list|get|create|update|delete
keap company   list|get|create|update
keap tag       list|get|create|contacts|apply|remove
keap opportunity list|get|create|update|stages
keap task      list|get|create|update|delete
keap note      list|get|create|update            (contact-scoped: --contact-id)
keap email     send|list
keap automation list|get|add-contacts            (+ campaign list/get under `keap campaign`)
keap user      list|me
```

List verbs expose `--page-size`, `--page-token`, `--filter`, `--order-by`, `--fields`
mapping 1:1 to the v2 query params; the provider's `next_page_token` rides the passthrough
JSON. Write verbs take explicit flags for common fields (e.g. `contact create --email
--given-name --family-name ...`) plus a `--json-body` escape hatch for full-shape payloads
(custom fields), following the conventions already in the tree.

**Output contract** (003 §3): success = provider JSON passthrough to stdout + newline;
failure = one-line error with the provider's code/message on stderr, exit 1; usage errors
exit 2; `--json` renders the structured error envelope (notion's `apiError` pattern).
401 from Keap rejects the credential (engine stale-marking).

## 3. Helio provider bundle plan

Naming axes: ① CLI command `keap` (flat, no group — independent brand, no family), ② anycli
id `keap`, ③ provider key `keap`. **No ②↔③ divergence → no `toolToProvider` entry.**

`integrations/providers/keap/provider.yaml` (hidden-first):

```yaml
schema: helio.provider/v1
key: keap
go_name: Keap

presentation:
  name: Keap
  description_key: keap
  consent_domain: accounts.infusionsoft.com
  visible: false            # flip + regenerate is the single go-live change
  order: <assigned at batch end>

auth:
  type: oauth
  owner: assistant          # tenant-wide CRM grant (scope=full), notion-style, not a personal identity
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://accounts.infusionsoft.com/app/oauth/authorize
    token_url: https://api.infusionsoft.com/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [full]          # the only valid Keap scope
    single_active_token: false
    refresh_lease: credential   # rotating single-use refresh token — serialize per credential
    # no revoke block: Keap documents no RFC-7009 revocation endpoint

identity:
  source: token_response
  stable_key: /scope        # "full|xy123.infusionsoft.com" — per-tenant stable
  label_candidates: [/scope]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: standard_oauth   # generic path, no per-provider adapter — but needs ONE
                                     # reviewed capability-set growth first: standard_oauth must
                                     # admit refresh_lease: credential (see §3a). Not a golden
                                     # zero-code drop-in until that lands.

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: keap
  kind: oauth
```

Decisions worth recording:

- **Identity from `token_response /scope`, not userinfo `/sub`.** Keap's token response is
  `{access_token, refresh_token, token_type, expires_in, scope}` with
  `scope: "full|<tenant>.infusionsoft.com"` — the only tenant identifier in the flow. The
  userinfo `sub` identifies the authorizing *user*, which conflates two different Keap
  tenants authorized by the same user (second connect would upsert over the first). The
  tenant is the account. Cost: the connection label renders as the raw
  `full|xy123.infusionsoft.com` string — functional but ugly; if we later want a pretty
  label, that's a narrow-adapter or a generic "label transform" enum discussion, not a
  blocker (per provider-yaml.md, prefer growing the generic capability set).
- **`refresh_lease: credential`.** Keap refresh tokens are single-use and rotate on every
  refresh ("you must use the newly provided refresh_token... store it every time");
  concurrent refresh across replicas would orphan the credential. `credential` scope (not
  X's global `provider` scope — Keap has no cross-connection token cap). The lease *runtime*
  already implements this scope — `service/token_refresh.go` `acquireRefreshLease` keys the
  Mongo lease on `refresh:<provider>:<credentialID>` when scope is `OAuthLeaseCredential`, so
  concurrent refreshes of the same credential serialize and A3 strict write-back persists the
  rotated token before it is served. What is **missing** is the compiled contract admitting
  this pairing under `standard_oauth`; see §3a — this bundle does **not** pass `provider-gen`
  until that lands, and must never be downgraded to `refresh_lease: none` (that is precisely the
  orphaned-credential risk this decision avoids).
- **`form_secret` with one documented asymmetry to verify at L4/L5.** Official docs show the
  code exchange with `client_id`/`client_secret` in the form body, but the refresh example
  with HTTP Basic client auth. Helio applies one style to both legs; with `form_secret` the
  refresh path goes through `x/oauth2` `AuthStyleAutoDetect`, which probes header/body and
  caches what works — and community evidence says Keap's token endpoint accepts both styles
  on both grants. **Checkpoint:** L4's forced-refresh run must confirm refresh succeeds; if
  Keap rejects body creds on refresh, flip the bundle to `form_basic` and re-verify the code
  exchange leg at L5.
- **`disconnect_mode: local_only`** — no revocation endpoint documented; users deauthorize
  from their Keap account settings.
- **Access-token lifetime:** docs define it only via `expires_in` (example shows 3600;
  community reports 24 h in practice). The token gateway trusts `expires_in`; nothing to
  configure.
- **Config:** lane 1 lands `keap` client id/secret in integration-service config (`config/`
  + `deploy/` Helm Secret together); dev-mode values arrive as uncommitted local
  `config/cloud.yaml` entries for L4. Redirect URI must be HTTPS (Keap rejects non-HTTPS).

### 3a. Prerequisite Helio-side capability change (blocks L3)

The compiled runtime contract currently pins `standard_oauth` to a **single** allowed refresh
lease scope, `none`:

- `go-services/integration-service/model/runtime_contract.go` — `RuntimeStrategyStandardOAuth`
  declares `oauth: &oauthRuntimeContract{singleActiveToken: false, refreshLeaseScope: OAuthLeaseNone}`,
  and `ValidateRuntimeContract` rejects any bundle whose `auth.oauth.refresh_lease` differs from
  that one value.
- `provider-gen` runs the same validation: `cmd/provider-gen/generator_test.go`'s
  "standard OAuth refresh lease" case asserts that a standard_oauth bundle with
  `refresh_lease: "credential"` is rejected with `refresh_lease "none"`.

So Keap's bundle (correctly `refresh_lease: credential`) **fails `provider-gen` / `provider-gen --check`**
— the design's own L3 gate — with no code change. There is no adapter escape hatch and no way to
express the correct decision under the current contract. The fix is a small, orthogonal
integration-service **compiled-capability growth**, per `provider-yaml.md`'s guidance to "grow the
generic `standard_oauth` capability set" with one more reviewed enum value rather than fork an
adapter:

1. Turn the contract's single `refreshLeaseScope` into an **allowed-set** of lease scopes
   (mirroring how `disconnectModes []DisconnectMode` is already a list validated with
   `slices.Contains`). Set `standard_oauth`'s allowed set to `{OAuthLeaseNone, OAuthLeaseCredential}`
   — `none` stays the default/only requirement for the existing standard providers; `credential`
   becomes admissible for rotating-refresh-token providers like Keap. X's narrow strategy keeps its
   own exact `{OAuthLeaseProvider}` set — this change does not widen X, and `single_active_token`
   stays `false` (the lease is orthogonal to single-active activation).
2. Update `ValidateRuntimeContract`'s lease check (scalar `!=` → set membership) and the
   `manifestOAuthLease` error rendering to list the allowed set.
3. Update the provider-gen validation tests: the `generator_test.go` "standard OAuth refresh lease"
   case must flip from *expecting rejection* to *accepting* `credential` under standard_oauth (X's
   "X refresh lease" case is unchanged), plus a positive contract-level test in
   `model` for the newly-admitted pairing.

**Merge ordering.** This is an integration-service source change, **not** one of the seven
batch-end shared surfaces (§2 of the master plan) — it does not touch `register.go`, the anycli
pin, the five `provider-gen` projections, `toolToProvider`, `providerIcons.ts`, the plugin publish,
or the OAuth config appends. It is a per-tool compiled-capability prerequisite, in the same class as
this batch's other integration-service capability items (e.g. dataforseo's query-param api_key,
hubspot's numeric stable-key). It must be **reviewed and merged on its own** (it can land mid-batch,
ahead of the batch-end bundle merge) so that when the batch lead runs the single canonical
`provider-gen`, Keap's bundle validates. Until it lands, this branch's local L3 (`provider-gen --check`
against the branch bundle) is expected to fail on the lease pairing — that failure is the signal the
prerequisite is still outstanding, distinct from the ordinary batch-end regen red noted in §4.

Also riding the batch-end merge: UI icon `ui/helio-app/src/integrations/icons/keap.svg` +
`providerIcons.ts` registration; provider sub-doc under `agents/plugins/heliox/skills/tool/`
+ plugin version bump/publish; the single canonical `provider-gen` run (five projections).
Per master plan §2 this branch does **not** commit regenerated projections — `provider-gen`
runs locally for validation only, and CI on this branch is expected red on
`provider-gen --check` until batch end.

## 4. What merges when

- Mid-batch (this branch, conflict-free): `internal/tools/keap/` + tests,
  `definitions/tools/keap.json`, icon SVG (unregistered), provider sub-doc (unpublished).
- Mid-batch, separately reviewed (Helio side, prerequisite for L3): the §3a
  `standard_oauth` lease-scope-set growth in `integration-service/model/runtime_contract.go`
  + provider-gen validation tests. Not a batch-end shared surface; must merge before the
  batch-end bundle regen so Keap's bundle validates.
- Batch end (batch lead): `register.go` entry, anycli tag + `helio-cli/go.mod` pin bump,
  `integrations/providers/keap/provider.yaml`, one `provider-gen` run (five projections),
  `providerIcons.ts` append, plugin publish. Local-only during dev: `helio-cli/go.mod`
  `replace github.com/heliohq/anycli => <this worktree>` (uncommitted, sanctioned), local
  regens, dev client id/secret in `config/cloud.yaml`.

## 5. Test plan (five layers)

| Layer | What runs here | External creds needed? |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes for every subcommand asserting method/path/query/body shape, `Authorization: Bearer` injection from `KEAP_ACCESS_TOKEN`, pagination param passthrough, plain + `--json` error rendering, 401 → exit 1, missing-credential → exit 1. TDD: tests first per AGENTS.md. | No |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real OAuth token> anycli keap -- contact list ...` against the live API (contact CRUD roundtrip, tag apply/remove, task create, userinfo). Token minted through the dev app's OAuth flow (one manual consent on the sandbox tenant; any local HTTPS redirect catcher works). Not a PAT — must exercise the exact injected header semantics. Mandatory before the pin bump. | **Yes** — lane-1 dev app (client id/secret) + lane-2 Keap sandbox tenant login |
| L3 | Local `go run ./cmd/provider-gen` + `--check` against the branch bundle (validation only, not committed); helio-cli build/tests with the local `replace`; both repos' unit suites. **Gated on §3a**: `provider-gen` rejects `refresh_lease: credential` under `standard_oauth` until the compiled-contract growth lands, so L3 passes only once the §3a change is merged (or applied locally to prove L3 on-branch). | No |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with the real `access_token` **and** `refresh_token` and a deliberately short `expires_at`, forcing the very next `heliox tool keap -- contact list` through the refresh-and-write-back path (A3) — this is also the `form_secret`-on-refresh checkpoint and the rotating-refresh-token persistence check (run twice: second run must refresh again off the *rotated* token). Dev client id/secret in uncommitted local `config/cloud.yaml`. | **Yes** — same dev app + a real token pair from the sandbox tenant |
| L5 | Human-in-the-loop (lane 3, post-batch-merge, pre-flip): `heliox tool keap auth` → connect link → Keap consent on the sandbox tenant → `oauth_connected` event on the channel → one unseeded live command. Verifies authorize URL params (`scope=full`, HTTPS redirect), identity extraction (`/scope` → account key), and the exchange leg under the final style. | **Yes** — sandbox tenant login; human consent (oauth lane) |

Definition of done per master plan §2: L1–L5 green, docs published, icon registered, then
`visible: true` + regenerate as the single go-live change.

## 6. Open items / risks

1. **`standard_oauth` must admit `refresh_lease: credential` (blocks L3)** — the compiled
   contract in `integration-service/model/runtime_contract.go` currently allows only
   `refresh_lease: none` for `standard_oauth`, so Keap's (correct) `credential` bundle fails
   `provider-gen`. Resolution is the reviewed compiled-capability growth in §3a, merged on its own
   ahead of the batch-end bundle merge. Owner: whoever lands this tool's Helio-side prerequisite;
   status: pending, not optional.
2. **Connection label cosmetics** — `/scope` label shows `full|<tenant>.infusionsoft.com`;
   acceptable for launch, candidate for a generic label-transform capability later.
3. **Refresh client-auth style** — resolved empirically at L4 (see §3 checkpoint);
   fallback is `form_basic`.
4. **Dev-app key count** — Keap generally grants 2 keys/developer (extra via ticket); lane 1
   should decide dev/prod split at registration time.
5. **v1 sunset watch** — nothing here uses v1, but the `/v2/oauth/connect/userinfo` and
   token endpoints are shared infrastructure and unaffected by the XML-RPC 2026 sunset.
