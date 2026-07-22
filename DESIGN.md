# Ramp — per-tool design (`heliox tool ramp`)

Scratch design for the `tool/ramp` batch branch. Batch lead strips this at
batch-end. Catalog row 144: Product **Ramp**, anycli id `ramp`, provider key
`ramp`, auth lane **oauth_review**, wave 2, category Finance.

Every claim below was verified against Ramp's official developer docs
(`docs.ramp.com`, machine-readable `/llms-*.txt` + OpenAPI, July 2026) and the
actual repo code — not inherited from the catalog or the audit. The
load-bearing decisions — **(a) is `oauth_review` the right lane, (b) does the
token endpoint's HTTP-Basic client auth need capability growth, and (c) do we
ride Ramp's Developer-API partner OAuth app or its separate MCP OAuth client** —
are confronted in §0/§3, and every divergence from the catalog/audit is recorded
in §7.

---

## 0. Lane verdict (the question the review demanded)

**Verdict: BUILD on the `oauth_review` lane, `service` type, `standard_oauth`
runtime strategy. The audit's `oauth_review` classification is CORRECT and stays
`high` confidence after verifying the official docs. The self-serve
single-account API-token path exists but is single-account-only and is rejected
as the primary lane for a multi-tenant teammate (§3.3).**

The audit's own rubric (`oauth-audit.md`): *"a human review, partner-program,
verification, or publish gate before external accounts can authorize →
`oauth_review`."* Ramp fits this exactly:

- **Multi-tenant authorization-code OAuth genuinely exists.** Verified against
  Ramp's Authorization guide (`docs.ramp.com/developer-api/v1/authorization`):
  the **Authorization Code** grant is documented for *"Third-party apps, public
  integrations"* — one registered partner app that arbitrary Ramp customer
  businesses authorize. The user is redirected to Ramp's authorization URL with
  `response_type=code`, `scope`, `client_id`, `redirect_uri`, `state`,
  authenticates, and approves; the code is exchanged at `/developer/v1/token`.
  This is the multi-tenant authorization-code flow the rubric requires — **not**
  a per-instance or developer-only token.
- **The gate is a production-access review, not a self-serve toggle.** Verified
  against Ramp's "Build with Ramp" partner guide
  (`docs.ramp.com/developer-api/v1/build-with-ramp`): the four-phase flow is
  *"Phase 1: Build → Phase 2: Apply for Production Access → Phase 3: Beta Testing
  → Phase 4: Apply for a Public Listing"*, and *"Approval is required before
  production access"* — with security disclosures (SOC 2 / ISO 27001 / pen test)
  named in the audit's evidence. Only *Admin / Business Owner* users may
  authorize a third-party app. This human production-access review is precisely
  why the tool is `oauth_review` and not `oauth_light`.

**Why this does not break Helio's model.** The production-access review maps
cleanly onto the plan's hidden-first, three-lane execution model (master plan §2):

- Lane 1 (OAuth app registration/review queue) already owns *"creating provider
  apps, redirect URIs, scope requests, and… publisher review"* — Ramp's "build a
  dev app, then apply for production access" is exactly this lane's job.
- Ramp lets you **build and test against a Developer-API app before production
  approval** (Phase 1 Build precedes Phase 2 Apply). So **dev-app creation gates
  L4/L5** while the **production-access approval gates only the visible flip** —
  the standard hidden-first decoupling. Code lands complete-but-hidden in Wave 2
  regardless of the production-approval clock; if approval never arrives the tool
  simply waits in code-complete/hidden state (zero code waste, the plan's stall
  mitigation).

**No catalog change**: row 144 stays `ramp | ramp | ramp | oauth_review | 2 |
Finance`.

---

## 1. What an AI teammate does with Ramp — and the API surface that serves it

A Helio teammate connected to a customer's Ramp account is a **finance /
spend-ops colleague**: it answers "what did we spend, on which cards, by whom,
in which department/location", reconciles card transactions and reimbursements,
and reports on users and org structure. It is **read-only** for hidden-first. It
does **not** issue cards, move money, approve reimbursements, or run onboarding —
high-blast-radius or platform concerns outside an assistant's job and outside
hidden-first scope.

That intent maps onto the **Ramp Developer API** (`https://api.ramp.com`, JSON,
`/developer/v1/` prefix, `Authorization: Bearer <token>`). The tool wraps exactly
the resources the colleague reaches for, grouped by Ramp's API products, each
gated by a `resource:read` scope:

| Group | Endpoints wrapped (all `GET`) | Scope | Why |
|---|---|---|---|
| `transaction` | `/developer/v1/transactions`, `/developer/v1/transactions/:id` | `transactions:read` | The core "what did we spend" card ledger. |
| `reimbursement` | `/developer/v1/reimbursements`, `/developer/v1/reimbursements/:id` | `reimbursements:read` | Out-of-pocket expense reconciliation. |
| `card` | `/developer/v1/cards`, `/developer/v1/cards/:id` | `cards:read` | Which cards exist, limits, status. |
| `user` | `/developer/v1/users`, `/developer/v1/users/:id` | `users:read` | Who spent / cardholder lookup. |
| `department` | `/developer/v1/departments`, `/developer/v1/departments/:id` | `departments:read` | Dimension lookup for grouping spend. |
| `location` | `/developer/v1/locations`, `/developer/v1/locations/:id` | `locations:read` | Dimension lookup for grouping spend. |
| `business` | `/developer/v1/business` | `business:read` | Connected-business info; also serves identity (§3.4). |
| top-level `get` | arbitrary `GET /developer/v1/<path>` passthrough | (as scoped) | Long-tail reads without a per-resource verb. |

Write verbs and Ramp's **agentic** surfaces (`cards:read_agentic`,
`spend_limits:write`, agent-card issuance) are **deliberately deferred** past
hidden-first — subtract before adding; they are a post-L2 enhancement and several
need elevated scopes and a different auth posture (§7, the MCP-client note).

Cross-cutting conventions (verified against Ramp docs):
- **Pagination**: Ramp list endpoints are cursor/page-based — responses carry a
  `data` array plus a `page.next` cursor URL. Surfaced as `--cursor` / `--limit`
  (`page_size`); `--json` returns the envelope verbatim so the caller can follow
  `page.next`.
- **Auth**: every call sends `Authorization: Bearer <access_token>` against
  `api.ramp.com` (production; the OAuth token scopes the request to the connected
  Ramp business server-side — no per-account data host).
- **Errors**: Ramp returns a JSON error body with a 4xx/5xx; the service maps it
  to the typed `apiError` exit-code contract below.

### Why NOT a cli-type tool

Ramp ships no official first-party general CLI (its "Ramp MCP" server is an MCP
endpoint, not a provisionable binary, and is the wrong auth posture — §7).
`service` type against the REST API is the only option and matches 21/23 shipped
definitions. Stage-1 rubric: `service`.

---

## 2. anycli definition (data plane)

- **Type**: `service`. Package `internal/tools/ramp/` (id has no dashes → Go
  package `ramp`), registered `RegisterService("ramp", &ramp.Service{})` in
  `internal/tools/register.go`. Definition file `definitions/tools/ramp.json`.
- **Shape**: copy the `internal/tools/notion/` reference — a cobra tree grouped
  by resource (`transaction`, `reimbursement`, `card`, `user`, `department`,
  `location`, `business`) plus a top-level `get`, a `BaseURL`/`HC`/`Out`/`Err`
  struct so tests point at an `httptest` server, and the documented exit-code
  contract: **0** success, **1** runtime/API failure (typed `apiError` from
  Ramp's error body), **2** usage/parse error. `--json` emits a structured error
  envelope.
- **Auth injection** — the token gateway projects the connection's bearer token
  into `token.access_token`; the bundle `credential.fields.access_token` maps it,
  and the anycli definition injects it as an env var the service reads and sends
  as `Authorization: Bearer <token>`:

  ```json
  {
    "name": "ramp",
    "type": "service",
    "description": "Ramp transactions, reimbursements, cards, users, departments, and locations (OAuth)",
    "auth": {
      "credentials": [
        { "source": {"field": "access_token"},
          "inject": {"type": "env", "env_var": "RAMP_ACCESS_TOKEN"} }
      ]
    }
  }
  ```

  Ramp accepts the OAuth access token via `Authorization: Bearer <token>` against
  `api.ramp.com/developer/v1` — the same header for both authorization-code and
  client-credentials access tokens.

- **L1 TDD**: `httptest.Server` fakes per resource assert request path, method,
  the injected `Authorization: Bearer` header, `--cursor`/`--limit` params, the
  `data` + `page.next` pagination envelope, and both text and `--json` error
  rendering. No live API in unit tests.

---

## 3. Credentials & the exact auth flow (the load-bearing decision)

### 3.1 Chosen flow — Ramp Developer-API partner OAuth 2.0 (authorization code)

Verified against `docs.ramp.com/developer-api/v1/authorization` +
`/llms-full.txt`:

| Piece | Value |
|---|---|
| Grant | **Authorization Code** (docs: *"Third-party apps, public integrations"*). Client Credentials is a *different* grant, for internal server-to-server only — not used here. |
| Authorize URL | Ramp's OAuth authorization endpoint — **host not printed verbatim in the machine-readable docs**; working value `https://app.ramp.com/v1/authorize`, **confirmed at stage 1** against Ramp's app registration screen (§7 open item). Params (verified): `response_type=code`, `scope`, `client_id`, `redirect_uri`, `state`. |
| Token URL (exchange **and** refresh) | `https://api.ramp.com/developer/v1/token` (docs: *"requests to /developer/v1/token"*; host `api.ramp.com`). |
| Client auth at token endpoint | **HTTP Basic** — *"send client_id:client_secret, base64-encoded, in the `Authorization: Basic …` header"* (some clients send them in the form body). → maps to the existing **`token_exchange_style: form_basic`** stock enum value (form-encoded body + Basic client auth), verified present on main (`model/catalog.go:15` `TokenExchangeFormBasic`, `validate.go:243`). **This is the one axis that differs from Brex** (which used `form_secret`); no growth — `form_basic` is already stock. |
| Scopes | space-delimited `resource:read` scopes for §1: `transactions:read cards:read users:read reimbursements:read departments:read locations:read business:read`. |
| PKCE | S256 *"supported"* (changelog notes an optional `code_challenge` for the application OAuth handoff), **not required**. → `pkce: none` for the confidential server-side client (the partner secret lives in integration-service); S256 is available if a later posture wants it. |
| access_token | **opaque** (not JWT), **1-hour** lifetime (docs: *"1 hour (3,600 seconds) for Authorization Code and Refresh Token access tokens"*). |
| refresh_token | issued for the auth-code flow (*"Use the refresh token to obtain a new access token"*); lifetime/rotation **not documented** → default refresh write-back, rotation confirmed at L2 (§4). Authorization codes last 10 minutes. |
| API data host | `https://api.ramp.com` (production; `/developer/v1/` prefix). |
| Production gate | *"Approval is required before production access"* (four-phase partner flow, §0). Gates the visible flip, not dev/L4. |

### 3.2 Lane confirmation is in §0; this section is the mechanics

The lane decision (viable, `oauth_review`, unchanged) is fully argued in §0. This
section fixes the two **technical** items that decide capability growth: client
auth (§3.1 — `form_basic`, stock, no growth) and the token-response `expires_in`
question (§3.5).

### 3.3 Why not the self-serve single-account API token

Ramp also exposes **self-serve Developer-API credentials inside one business's
own dashboard** (a client_id/secret an org admin creates to build against *their
own* Ramp account, typically via Client Credentials). It is tempting because it
skips the production-access review. It is **rejected as the primary lane** and no
catalog change is proposed, because:

- It authorizes only **that one** business's data. A multi-tenant AI teammate
  connecting to arbitrary customer businesses cannot use one org's dashboard
  credential for another org — that is the single-account `api_key`/internal
  model, a different lane.
- The multi-tenant story Ramp itself documents for *"third-party apps"* is the
  **Authorization Code** partner flow (§3.1). Choosing the single-account token
  would strand every customer except the one that minted it.

Recorded as considered-and-rejected. (If a future single-account-only Ramp use
case appears, it would be a *separate* `manual_api_token` bundle, not this one.)

### 3.4 Identity

The connected business's identity comes from the **`business:read`**-scoped
`GET /developer/v1/business` endpoint, which returns the authenticated Ramp
business's `id` and name/entity fields.

- **Chosen: `identity.source: api`** against `/developer/v1/business`,
  `stable_key: /id` (the business id — the natural per-connection account key),
  `label_candidates: [/business_name_legal, /business_name_on_card, /id]`.
- Ramp's opaque access tokens are **not** JWTs, so there is no `sub` to read
  from the token, and Ramp is not documented as an OIDC provider with a
  `userinfo` endpoint (contrast Brex, which used OIDC `userinfo`/`/sub`).
  `GET /developer/v1/business` is the reviewed HTTPS identity endpoint. The exact
  identity JSON field names are confirmed at stage 1 against the OpenAPI schema
  (§7 open item); `business:read` is already in the scope set so no extra scope
  is needed.

### 3.5 The one load-bearing technical fact — `expires_in` is documented PRESENT

**This decides whether Ramp needs the assumed-TTL capability.** Unlike Brex
(whose auth-code token example *omits* `expires_in`), Ramp's docs **state the
access-token lifetime numerically** — *"1 hour (3,600 seconds) for Authorization
Code and Refresh Token access tokens"* — and Ramp follows a standard OAuth2 token
response, so `expires_in: 3600` is **expected to be present** in the token
response. Therefore:

- **Expected case — `expires_in` present.** Ramp is **pure stock
  `standard_oauth`**: the generic `standardOAuthExchanger` reads `expires_in`,
  persists a real `Expiry`, and the token gateway's refresh-and-write-back fires
  at ~1 h. **No capability growth on this axis.**
- **Unlikely case — `expires_in` absent** (the live token endpoint drops it
  despite the documented TTL). Then Ramp hits the **Stripe/Salesforce failure
  mode**: a nil persisted `Expiry` reads as non-expiring, refresh never fires,
  every connection 401s ~1 h after connect. The fix would be the already-designed
  **assumed-TTL** field (`oauth.access_token_ttl_seconds: 3600` →
  `OAuthEndpoints.AssumedAccessTokenTTL`), which is **not yet on this worktree's
  `oauthManifest`** (verified — `provider-gen` rejects it today; it lands on the
  salesforce/stripe/brex sibling branches). Ramp would then **consume** that field
  (one bundle line), not author it.

So the plan is the **opposite ordering posture to Brex**: assume **no growth**
(case: `expires_in` present) and keep the **L2 confirmation against the real
`/developer/v1/token`** as the gate that flips to the assumed-TTL fallback *only
if* `expires_in` unexpectedly turns out absent. No bespoke adapter either way —
this is the skill's "grow one reviewed field, not an adapter" guidance, and here
even that field is a fallback, not the baseline.

### 3.6 Disconnect — `local_only` (no documented revoke endpoint)

Ramp's machine-readable docs (authorization guide + build-with-ramp) contain
**no documented token-revocation endpoint**. Per the validator's closed
`disconnect_mode` enum (`provider_revoke | local_only | strategy`,
`validate.go`), and because `provider_revoke` on `standard_oauth` **requires** a
declarative `auth.oauth.revoke` block that we have no verified URL for, the
bundle ships **`connection.disconnect_mode: local_only`** — the vault credential
delete + status flip, the mandatory part of every disconnect. This is the
validator-safe, honest choice: do **not** invent a revoke URL.

**Divergence recorded (see §7).** If stage 1 / L2 surfaces an official Ramp token
revocation endpoint (e.g. an RFC 7009 `/developer/v1/token/revoke`), the upgrade
is a declarative `provider_revoke` + `revoke:` block with `client_auth: basic`
(matching the `form_basic` exchange) and `token: refresh_token` — **no code**,
one bundle edit. Absent that verified endpoint, `local_only` stands; never a
bespoke revoke adapter for hidden-first.

### 3.7 Config fields (integration-service, per environment)

`auth.required_config_fields: [oauth.client_id, oauth.client_secret]`, supplied
via integration-service config (`config/` + the `deploy/` Helm Secret together —
Config Sync hard rule). `client_id`/`client_secret` = the Ramp-issued **partner**
credentials (from lane 1's dev app; production credentials after the
production-access review). Never in the bundle. A fully unset provider renders
`configured:false` (Connect disabled, safe hidden); a **partial** config fails
integration-service startup — so id + secret land in the same change, before this
provider's L5.

---

## 4. Helio integration-service capability growth

**Expected: ZERO capability growth — Ramp is pure stock `standard_oauth`.** Every
axis is already stock:

- **Client auth = `form_basic`** (HTTP Basic at the token endpoint, §3.1) —
  verified present on main (`TokenExchangeFormBasic`, `validate.go:243`). This is
  the one axis that differs from Brex; it needs **no growth** because the enum
  value already exists.
- **`expires_in` documented present** (§3.5) → the generic exchanger persists a
  real `Expiry`; **no assumed-TTL field needed** in the expected case (contrast
  Brex, which carries a probable assumed-TTL ordering dependency).
- **Identity** via declarative `identity.source: api` against
  `/developer/v1/business`, `stable_key: /id` — stock `declarativeIdentityResolver`.
- **Refresh-token write-back** — stock; refresh tokens are **not** documented as
  rotating, so the default write-back suffices (`refresh_lease: none`). Confirm at
  L2 — if rotation is observed, set `refresh_lease: credential` (the existing
  `OAuthLeaseCredential`, verified in the stock enum `none|credential|provider`,
  `validate.go:249`), no new code.
- **Disconnect** `local_only` (§3.6) — the default declarative no-op revoker;
  stock.

So the only real risk is the §3.5 `expires_in` L2 confirmation. If it turns out
absent, Ramp acquires a **conditional** ordering dependency on the assumed-TTL
field landing on its Wave-2 base (the field it would then consume, not author).
No Ramp-specific adapter in any case.

---

## 5. Helio provider bundle plan (`integrations/providers/ramp/provider.yaml`)

Naming axes — **no ②↔③ divergence**, so **no `toolToProvider` entry** and no
grouped-command word:

| Axis | Value |
|---|---|
| ① CLI command word | `ramp` (flat) → `heliox tool ramp` |
| ② anycli tool id | `ramp` |
| ③ provider catalog key / bundle dir | `ramp` |

`ProviderFor("ramp")` falls through to identity (`ramp`), and `ToolFor`
likewise — nothing to register in `resolver.go`.

Bundle (hidden-first — `visible: false`):

```yaml
schema: helio.provider/v1
key: ramp
go_name: Ramp

presentation:
  name: Ramp
  description_key: ramp
  consent_domain: ramp.com
  visible: false          # hidden-first; flip true only after L5 + Ramp production-access approval
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual        # per-account user-authorized token; connection belongs to the assistant.
                           # NOT `assistant` — that triggers the app-bot org-admin gate in oauth_start.go.
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.ramp.com/v1/authorize   # host CONFIRM at stage 1 (§3.1/§7); params verified
    token_url: https://api.ramp.com/developer/v1/token
    token_exchange_style: form_basic     # HTTP Basic client auth at the token endpoint (§3.1) — stock enum, no growth
    pkce: none                           # S256 supported but not required; confidential server-side client
    display_scopes: [transactions:read, cards:read, users:read, reimbursements:read, departments:read, locations:read, business:read]
    single_active_token: false
    refresh_lease: none                  # refresh tokens not documented as rotating; ADD credential only if L2 proves rotation
    # access_token_ttl_seconds: 3600     # NOT needed in the expected case — Ramp documents expires_in=3600 (§3.5).
    #                                    # Uncomment ONLY if L2 proves the live token response omits expires_in, AND
    #                                    # only after the assumed-TTL field lands on this base (not yet on oauthManifest).

identity:
  source: api                            # GET /developer/v1/business (business:read); opaque non-JWT token, no OIDC userinfo (§3.4)
  endpoint: https://api.ramp.com/developer/v1/business
  stable_key: /id
  label_candidates: [/business_name_legal, /business_name_on_card, /id]  # exact field names CONFIRM at stage 1 (§7)

connection:
  mode: isolated
  disconnect_mode: local_only            # no documented Ramp revoke endpoint (§3.6). Upgrade to provider_revoke + revoke: block
                                         # ONLY if a real revocation endpoint is verified at stage 1/L2.
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
  name: ramp
  kind: oauth
```

Non-generated companions landing on the batch-end merge:
- UI icon `ui/helio-app/src/integrations/icons/ramp.svg` + a `providerIcons.ts`
  registration (manual, never generated).
- i18n `description_key: ramp` label string.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` describing the
  verbs, `--cursor`/`--limit`/`--json` conventions, and the read-only posture.
- The five `provider-gen` projections regenerate together (**never committed on
  this tool branch** — batch lead owns the one canonical regen; the branch is
  expected to fail `provider-gen --check` in CI until batch end, per master plan
  §2).

---

## 6. Test plan — five layers

| Layer | Ramp-specific content | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fakes per resource: assert path/method, `Authorization: Bearer` injection, `--cursor`/`--limit`, the `data` + `page.next` envelope, and typed `apiError` in text + `--json`. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli ramp -- transaction list --limit 3` against a **real Ramp account** (a dashboard-minted token doubles as a valid `api.ramp.com` bearer for the *data-plane* harness). Proves field names, Bearer injection, the `data`/`page.next` pagination, error shape. **This layer also resolves the §3.5 `expires_in` question** by exercising the real `/developer/v1/token` exchange with the dev partner app and inspecting the token response (present → no growth; absent → assumed-TTL fallback). | **Yes** — a Ramp account/token (account pool) + a dev partner app (lane 1). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` accept the bundle (stock `form_basic` + `api` identity; no growth in the expected case); `helio-cli` builds against the anycli branch via local `replace`; both repos' unit suites green. | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` a `ramp` connection with `access_token` + `refresh_token` (+ an aged expiry so the next `heliox tool ramp -- card list` forces the token gateway's refresh-and-write-back through `/developer/v1/token`). Success = live Ramp data, not a replayed seed. | **Yes** — a real OAuth access+refresh pair from the dev partner app (dev-cred issuance gates this; lane 1 distributes `client_id`/`client_secret` as uncommitted local `config/cloud.yaml` entries). |
| **L5** full connect | Once, hidden, pre-flip: `heliox tool ramp auth` → consent on the Ramp authorize screen (Admin/Business-Owner login on a real account) → `oauth_connected` system event → one **unseeded** live `heliox tool ramp` run. Human-in-the-loop (oauth L5, plan lane 3). | **Yes** — live Ramp consent on a real account; human consent session. |

Rollout: land hidden + generated + L1–L4 green; run L5 while hidden; then flip
`visible: true` + regenerate as the single go-live change — and, because Ramp is
`oauth_review`, only after **production-access approval** has cleared (approval
gates the flip, never dev/L4/merge).

---

## 7. Divergences & open items recorded (independent-judgment check)

- **Lane viability confirmed YES; audit `high` confidence stands.** The audit
  correctly flagged that Ramp's third-party OAuth requires *"apply for production
  access"* with security disclosures before other businesses can be served.
  Verified against the official four-phase partner flow (§0). The multi-tenant
  authorization-code flow is real and first-class; the production-access review is
  a **publish/verification gate of the same class as Stripe's App Marketplace
  review** — handled by lane 1 and decoupled from dev by hidden-first + a dev app.
  `oauth_review` is **viable and correct**; the self-serve single-account token is
  single-account-only and rejected as the primary lane (§3.3). **No §6 catalog
  amendment.**
- **Client auth is HTTP Basic → `form_basic`, the one axis that differs from
  Brex — and it needs NO growth (§3.1/§4).** Ramp authenticates the token request
  with base64 `client_id:client_secret` in the `Authorization: Basic` header,
  which maps onto the stock `token_exchange_style: form_basic` enum value
  (verified present on main). Brex used `form_secret`; Ramp uses `form_basic`.
  Both are stock; the difference is a one-line bundle value, not a capability.
- **`expires_in` is documented PRESENT — the assumed-TTL path is the UNLIKELY
  fallback, not the baseline (§3.5/§4).** This is the **inverse of Brex**: Brex's
  auth-code token example omits `expires_in` (assumed-TTL expected), while Ramp
  states the 1-hour/3600-second TTL numerically and follows a standard token
  response (`expires_in` expected present, no growth). The **L2 exchange against
  the real `/developer/v1/token`** is the gate that would flip Ramp to the
  assumed-TTL fallback *only if* `expires_in` is unexpectedly absent — and that
  field is not yet on this base's `oauthManifest` (would be a conditional ordering
  dependency, consumed not authored).
- **Helio rides Ramp's Developer-API partner OAuth app, NOT the Ramp MCP OAuth
  client (§1/§3.1).** Ramp documents a separate "build for AI agents" path where
  *"you must authorize through the shared Ramp MCP OAuth client, not through your
  own Developer API OAuth application"*, plus agentic scopes
  (`cards:read_agentic`, `spend_limits:write`) for **issuing agent cards**. That
  path is **out of scope**: heliox is a passthrough REST tool (not an MCP client),
  and this tool's posture is **read-only spend reporting**, not agent-card
  issuance or spend control. Recorded so the batch lead does not mistake the MCP
  path for the integration path — the correct lane is the standard partner
  authorization-code Developer-API app.
- **Authorize-URL host and identity field names are stage-1 confirms (§3.1/§3.4).**
  The machine-readable docs verify the authorize **params** (`response_type=code`,
  `scope`, `client_id`, `redirect_uri`, `state`) and the token host
  (`api.ramp.com/developer/v1/token`) but do **not** print the authorize URL host
  verbatim; working value `https://app.ramp.com/v1/authorize`, confirmed at the
  app-registration screen. The exact `GET /developer/v1/business` identity JSON
  field names (`stable_key`/`label_candidates`) are confirmed against the OpenAPI
  schema at stage 1. Neither blocks the design; both are read off the real app/API
  before L1.
- **Disconnect is `local_only` — no invented revoke URL (§3.6).** No official Ramp
  token-revocation endpoint is documented, so the bundle does not ship a
  `provider_revoke` block (which the validator would require a URL for). Upgrade to
  `provider_revoke` (declarative, `client_auth: basic`, `token: refresh_token`)
  only if a real revocation endpoint is verified at stage 1/L2 — never a bespoke
  revoke adapter.
- **No `toolToProvider` entry** (id == key == `ramp`), and **no grouped command**
  (Ramp is not a corporate family in this catalog).
