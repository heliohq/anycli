# Tool design — PayPal (`heliox tool paypal`)

Scratch design for the `paypal` external tool provider. Batch-lead strips this
file at batch end. Branch: `tool/paypal` (both repos). Catalog row **39**,
category **Payments & Commerce**, Wave **1**, auth lane **`api_key`**.

## 0. Naming axes (master plan §3) and lane

| Axis | Value |
|---|---|
| ① CLI command word (`tool.command`) | `paypal` (flat; no family group) |
| ② anycli tool id (`definitions/tools/<id>.json`) | `paypal` |
| ③ provider catalog key (bundle dir / `key:`) | `paypal` |

②≡③ (identity, no dash/underscore divergence) → **no `toolToProvider` entry**
in `helio-cli/internal/toolcred/resolver.go`. Go package `internal/tools/paypal/`
(no leading digit, no dash to normalize).

## 1. Auth verdict — `api_key` re-confirmed, with the real mechanism

The OAuth audit (row 39) put PayPal in `api_key` as "no viable multi-tenant
path". I verified against PayPal's official developer docs and **re-confirm
`api_key`**, but the one-line reason in the audit is imprecise — recording the
divergence per the prompt:

- **Divergence 1 — the mechanism is OAuth2 `client_credentials`, not "no OAuth".**
  Verified at `developer.paypal.com/api/rest/authentication/`. Every PayPal REST
  call is authorized by a **bearer access token minted from an app-owned
  client-credentials grant**: the user creates a REST app in the PayPal Developer
  Dashboard, receives a **client_id + client_secret** pair, and exchanges it via
  `POST {host}/v1/oauth2/token` — HTTP **Basic** auth (base64 `client_id:secret`),
  form body `grant_type=client_credentials` → `{"access_token","token_type":
  "Bearer","expires_in":≈32400,"scope","app_id","nonce"}` (token lives ≈9h).
  There is **no authorization-code / consent screen** a single Helio app could
  register for arbitrary PayPal accounts. (PayPal *does* run a partner "Connected
  Path" onboarding OAuth for platforms provisioning sub-merchants, but that is a
  partner-program/marketplace gate, not a self-serve consent flow — so it does
  not lift PayPal out of `api_key` under the audit rubric.) The credential is a
  **user-supplied secret pair** → exactly the `api_key` lane's shape. **Verdict
  re-confirmed.**

- **Divergence 2 — Sandbox/Live host split (most api_key tools don't have one).**
  Credentials are **environment-bound**: a Sandbox app's pair only works against
  `https://api-m.sandbox.paypal.com`, a Live app's only against
  `https://api-m.paypal.com`. The environment is therefore part of the
  connection's identity, not a per-call whim — it drives the 3-field credential
  shape and the verifier host selection in §4 (Option A). This is the single
  biggest departure from the plain single-token api_key precedent.

- **Divergence 3 — feature-enablement 403s.** The scopes a token carries depend on
  which **features are enabled on the user's REST app + account** (Invoicing,
  Transaction Search). Transaction Search in particular must be toggled on in the
  app's feature list *and* on the PayPal account, or a fully-authenticated call
  returns **403** (`https://uri.paypal.com/services/reporting/search/read` not
  granted). A correct token can still 403 → the service maps this to a distinct
  "feature not enabled on your PayPal app" message and the AI-facing doc calls it
  out. This is not an auth bug to retry.

## 2. What an AI teammate does with PayPal → which API surface we wrap

Driven by the teammate use case (a **finance/ops colleague** reconciling money in
and chasing receivables), scoped **read-first + safe-create only**. Endpoints
verified against the official reference pages cited inline.

| Verb (subcommand) | Method + path | Purpose |
|---|---|---|
| `invoice list` | `GET /v2/invoicing/invoices?page=&page_size=&total_required=true` | Enumerate invoices (page 1-1000, page_size 1-100). Primary receivables view. |
| `invoice get` | `GET /v2/invoicing/invoices/{invoice_id}` | One invoice's full detail + status. |
| `invoice search` | `POST /v2/invoicing/search-invoices` | Filter by recipient/status/date/amount (JSON body). **Note:** unlike list/get/create/send which all sit under `/v2/invoicing/invoices...`, `search-invoices` is a top-level `/v2/invoicing/search-invoices` sibling — do **not** derive it by appending to the invoices collection path (verified against paypal/paypal-rest-api-specifications `openapi/invoicing_v2.json`, whose `paths` list `/v2/invoicing/invoices` and `/v2/invoicing/search-invoices` as separate keys; the rendered developer.paypal.com page can mislead here). |
| `invoice create-draft` | `POST /v2/invoicing/invoices` | Create a **draft** invoice (safe: not sent until `send`). |
| `invoice send` | `POST /v2/invoicing/invoices/{invoice_id}/send` | Send/email a drafted invoice to the recipient. |
| `transaction list` | `GET /v1/reporting/transactions?start_date=&end_date=&page=&page_size=` | Transaction history, **≤31-day** window, RFC3339 dates, covers last 3y. Primary reconciliation read. |
| `balance list` | `GET /v1/reporting/balances?as_of_time=` | Account balances snapshot. |
| `subscription get` | `GET /v1/billing/subscriptions/{subscription_id}` | Look up a customer's subscription status. Read-only. |
| `order get` | `GET /v2/checkout/orders/{order_id}` | Inspect a specific payment/order. Read-only. |

Base URLs: Invoicing `{host}/v2/invoicing`, Reporting `{host}/v1/reporting`,
Billing `{host}/v1/billing`, Checkout `{host}/v2/checkout`, where `{host}` is the
env-selected `api-m[.sandbox].paypal.com`.

**Deliberately EXCLUDED from v1 (Divergence 4 — read-first + no money movement):**
order **capture** (`POST /v2/checkout/orders/{id}/capture`), **refunds**
(`POST /v2/payments/captures/{id}/refund`), **payouts**
(`POST /v1/payments/payouts` — send money to arbitrary recipients), and invoice
**cancel/delete**. These move or reverse real money and are one malformed
argument away from an irreversible transfer. Only invoicing's `create-draft` +
`send` are write verbs, and a draft is inert until explicitly sent. If a
money-movement verb is ever wanted it is a separate, human-gated follow-up —
noted here so the decision is explicit, not accidental. (Same posture as the
`hotjar` tool's exclusion of its destructive `delete_all_hits` mode.)

## 3. anycli definition & implementation

- **Type: `service`** (stage-1 rubric). No official non-interactive `--json`
  PayPal CLI exists → HTTP service in `internal/tools/paypal/` against the REST
  API. Mirror `internal/tools/notion/` shape: cobra tree grouped by resource
  (`invoice`, `transaction`, `balance`, `subscription`, `order`),
  `BaseURL`/`HC`/`Out`/`Err` struct for httptest fakes, exit-code contract
  (0 ok / 1 API failure via typed `apiError` / 2 usage) and a `--json`
  structured output + error envelope.

- **Credential shape — three fields, token exchange done in-service.** The
  service receives `client_id` + `api_secret` + `environment` (see §4 for the
  Helio projection) and performs the **client-credentials exchange itself**
  (`POST {host}/v1/oauth2/token`, Basic auth, `grant_type=client_credentials`),
  caches the bearer for the process lifetime, and calls the data endpoints with
  `Authorization: Bearer <token>`. `environment` (`live`|`sandbox`, default
  `live`) selects `{host}`. Keeping the whole OAuth-ish dance inside anycli means
  Helio stores a static secret pair + an env label and needs **zero
  token-gateway/OAuth machinery** — the same in-service client-credentials
  pattern already shipped by the `hotjar` and `snov` services (the precedent to
  copy). anycli processes are short-lived, so the 9h token TTL is irrelevant — it
  exchanges fresh each run.

- **Definition `auth.credentials`** — three `field`→`env` bindings. Secret field
  is named **`api_secret`** (NOT `client_secret`) to keep the integration-service
  credential-source denylist intact (Divergence 5, §4) — its inject env var is
  still the semantically correct `PAYPAL_CLIENT_SECRET`:

```json
{
  "name": "paypal",
  "type": "service",
  "description": "PayPal as a tool — invoicing, transaction reporting, subscriptions (REST client-credentials)",
  "auth": {
    "credentials": [
      { "source": {"field": "client_id"},   "inject": {"type": "env", "env_var": "PAYPAL_CLIENT_ID"} },
      { "source": {"field": "api_secret"},   "inject": {"type": "env", "env_var": "PAYPAL_CLIENT_SECRET"} },
      { "source": {"field": "environment"},  "inject": {"type": "env", "env_var": "PAYPAL_ENV"} }
    ]
  }
}
```

- **JSON output shape**: list verbs emit `{ "results": [ … ], "page": N,
  "total_pages": N, "total_items": N }` — PayPal returns HATEOAS
  `{items|transaction_details, total_pages, total_items, links[]}`; we normalize
  the collection key to `results`, surface page numbers as first-class
  `--page`/`--page-size` flags (mirroring PayPal's own pagination), and hide the
  raw `links` array. Single-object verbs emit the object directly. Errors →
  `apiError` envelope; map upstream **401** (bad/expired credentials — surfaces as
  AnyCLI `CredentialRejected`), **403** (scope/feature not enabled — distinct
  human message per Divergence 3), **422** (`UNPROCESSABLE_ENTITY` validation),
  and **429** (rate limit) to distinct messages.

- **TDD (anycli AGENTS.md)**: `httptest.Server` fakes assert (a) the
  client-credentials POST carries `grant_type=client_credentials` + the correct
  Basic header, and the returned bearer is injected on the subsequent data call;
  (b) **host selection** — `PAYPAL_ENV=sandbox` routes to the sandbox base, absent/
  `live` routes to the live base; (c) request shape + query params for
  `invoice list` (page/page_size/total_required) and `transaction list`
  (start_date/end_date, ≤31-day window guard); (d) plain + `--json` rendering of
  401/403/422/429. Never hit the real API from unit tests. Register in
  `internal/tools/register.go` `init()` (`RegisterService("paypal", &paypal.Service{})`)
  — batch-end shared-surface merge.

## 4. Helio provider bundle plan (hidden-first)

`integrations/providers/paypal/provider.yaml`, `presentation.visible: false`.
Model on the **mongodb** manual-credential bundle extended to multi-field (the
`hotjar`/`mixpanel` two-secret precedent), plus one **non-secret** field
(`environment`). **No OAuth block, no service adapter, and — because the
client_id/secret are the *user's*, not Helio's — zero integration-service
`config/` + `deploy/` appends** (lane-1 config landing is N/A for PayPal).

- `auth.type: credentials`, `owner: individual`.
- `auth.credential_input.fields`:
  - `environment` — `secret: false`, `required: true`, select `live`|`sandbox`
    (default `live`).
  - `client_id` — `secret: true`, `required: true`.
  - `api_secret` — `secret: true`, `required: true` (label "Client secret").
  - `setup_url` → PayPal Developer Dashboard app credentials page.
- `connection.runtime_strategy: manual_credentials`, `mode: isolated`,
  `disconnect_mode: local_only`.
- `credential.fields` (projected to AnyCLI as the three named fields + account
  key): `client_id: token.client_id`, `api_secret: token.api_secret`,
  `environment: token.environment`, `account_key: connection.account_key`.
- `tool.name: paypal`, `tool.kind: api-key` (wire-compat value; drawer routes by
  auth_type). No `tool.group`, no `experiment` gate (GA once flipped).

### Identity — two options; **Option A recommended**

- **Option A (recommended) — client-credentials verifier + `app_id` identity.**
  Add a compiled `paypalClientCredentialsVerifier` (the shape of the existing
  `sendgridScopesVerifier` / `postmarkServerVerifier` / `mastodonAccountVerifier`
  sibling verifiers): parse the JSON-composite secret `{client_id, api_secret,
  environment}`, pick the host from `environment`, do the
  `POST /v1/oauth2/token` client-credentials exchange, and on 200 derive the
  stable `account_key` + label from the returned **`app_id`** (e.g.
  `APP-80W284485P519543T`, suffixed with the environment). On 401 reject the pair
  as invalid at connect time. Benefits: catches typo/wrong-env credentials
  *before* storage, and yields a readable, environment-scoped account identity —
  worth the small capability growth given the sandbox/live split already forces
  host-selection logic server-side.
- **Option B (fallback, zero-verify) — `hotjar` shape.** `identity.source:
  strategy` with the generic `multiFieldClientIdentityDeriver` deriving
  `account_key` from the first field (`client_id`), no network call at connect
  (a bad pair surfaces at first use via `CredentialRejected`). Lower capability
  cost; use if the batch prefers to avoid a new verifier. Loses early validation
  and readable identity (client_id is an opaque 80-char string).

### Integration-service capability growth (design 317 D8 multi-field)

The worktree base still enforces the design 317 D5 **single-secret** rule
(`model.validateCredentialInputSchema` requires exactly one required field) and
does not carry the multi-field vault face. PayPal is a **Wave-1** tool, so it
**co-introduces** the D8 multi-field growth in its Wave-1 batch — the earlier-cited
`hotjar`/`snov`/`mixpanel`/`zoominfo` are all later waves (snov/mixpanel = Wave 2;
hotjar/zoominfo = 3-hold, Wave 3's final batch) and have **not** merged by PayPal's
batch-end, so PayPal cannot ride a no-op behind them. Its actual Wave-1 multi-field
peers are the two-field api_key tools in the same phase — **Twilio** (Account SID +
Auth Token, row 5) and **AWS** (Access Key ID + Secret Access Key, row 30). The
growth below is **idempotent**, so whichever Wave-1 multi-field tool lands it first,
the rest merge it as a no-op; **the batch lead must ensure it actually lands in the
Wave-1 batch** rather than assuming a sibling already carries it:

1. Relax `validateCredentialInputSchema` to allow **N≥1 required fields** for
   `AuthCredentials` (JSON-composite storage), permitting one **non-secret**
   required field (`environment`) alongside the two secrets.
2. Add credential sources `token.client_id`, `token.api_secret`,
   `token.environment` to the safe credential-source vocabulary in
   `model/catalog.go`.
   - **Divergence 5 — use `token.api_secret`, never `token.client_secret`.** The
     credential-source denylist reserves `client_secret` (a generator-reserved
     config name); storing the secret under `api_secret` keeps the denylist
     total, exactly as `hotjar` does.
3. Identity per the chosen option above (A: `paypalClientCredentialsVerifier`
   registered in `service/provider_registry.go`; B: `multiFieldClientIdentityDeriver`).

PayPal ships **zero provider-specific runtime/exchange Go in integration-service**
beyond the (optional, Option A) verifier — the token exchange lives entirely in
anycli.

### Non-generated frontend + docs (batch-end shared surfaces)

- UI icon `ui/helio-app/src/integrations/icons/paypal.svg` +
  `providerIcons.ts` append (manual, never generated).
- i18n: `tools.desc.paypal` + connect-form field labels
  (`environment`/`client_id`/`api_secret`) in all locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/`: document the
  read-first surface, the **draft→send** invoice safety, the **excluded**
  money-movement verbs, the **Sandbox vs Live** environment field, and the
  **feature-enablement 403** gotcha (enable Invoicing + Transaction Search on the
  REST app).

## 5. Testing plan — five layers (SKILL.md `references/integration-testing.md`)

| Layer | What it proves for PayPal | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes: client-credentials POST body + Basic header + bearer injected on data call; host selection by `PAYPAL_ENV`; `invoice list`/`get`/`search`, `transaction list` (start/end + 31-day guard), `subscription get`, `order get` request shapes; plain + `--json` 401/403/422/429 rendering. No real API. | No |
| **L2** `anycli paypal -- …` harness | `ANYCLI_CRED_CLIENT_ID` / `ANYCLI_CRED_API_SECRET` / `ANYCLI_CRED_ENVIRONMENT=sandbox` against a real PayPal **Sandbox** REST app → `invoice list`, `transaction list --start-date … --end-date …` return real data. Proves field names + injection + request shape match the live API. | **Yes** — PayPal Sandbox business app (free, self-serve at developer.paypal.com) |
| **L3** `provider-gen --check` + both suites | Five projections regenerate clean; integration-service tests for the relaxed multi-field schema + (Option A) `paypalClientCredentialsVerifier`; helio-cli `cmd/heliox/cmds/tool` build with local `replace` on the anycli branch. | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with the JSON-composite token `{client_id, api_secret, environment}` (user-token provider → seedable; no `refresh_token`/`expires_at` — anycli mints its own short-lived bearer) → `heliox tool paypal -- invoice list` reaches the live Sandbox API through the token gateway. | **Yes** — same Sandbox app |
| **L5** key-entry connect flow | api_key L5 path (master plan §2): open connect link → enter environment + client_id + secret → (Option A) verifier's client-credentials exchange confirms `connected`/`configured` in `GET /connections` → one **unseeded** `heliox tool paypal -- invoice list` succeeds. Agent-drivable via agent-browser (human lane-3 fallback on UI breakage). | **Yes** — same Sandbox app |

**Externally supplied credentials** are needed at **L2, L4, L5** and are a single
artifact: one **PayPal Sandbox business app** (client_id + client_secret) with
Invoicing + Transaction Search features enabled. Sandbox apps are free and
self-serve, so this is agent-procurable (no partner review, no paid tier) — a
notable contrast with the review-lane tools in this wave.

## 6. Rollout

Ship hidden (`visible: false`); land anycli tool + pin bump; regenerate; run
L1–L4 on-branch (L4 via seed + local `replace`); then the per-batch L5 sweep;
finally flip `presentation.visible: true` + `provider-gen` as the single go-live
change (SKILL.md stage 10). No review clearance gate (api_key lane) — L5 is the
only visible-flip gate.
