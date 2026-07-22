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

Base URLs: Invoicing `{host}/v2/invoicing`, Reporting `{host}/v1/reporting`,
Billing `{host}/v1/billing`, where `{host}` is the env-selected
`api-m[.sandbox].paypal.com`.

**Dropped from v1 — `order get` (`GET /v2/checkout/orders/{order_id}`).** Verified
against `developer.paypal.com/docs/api/orders/v2/`: v2 Checkout orders are created
by, and scoped to, the REST app that created them, and are short-lived checkout
sessions retrievable only around the approve/capture window. A finance/ops
teammate reconciling money-in works from Transaction Search and invoices, not
arbitrary checkout order IDs it never created — so this verb would mostly return
**404** on IDs the assistant did not mint, which the AI would misread as a bug. A
money-in lookup routes through `transaction list` / `invoice *` instead. (If a
later batch adds Helio-originated checkout, `order get` returns as a scoped,
same-app follow-up.)

**Deliberately EXCLUDED from v1 (Divergence 4 — read-first + no money movement):**
order **capture** (`POST /v2/checkout/orders/{id}/capture`), **refunds**
(`POST /v2/payments/captures/{id}/refund`), **payouts**
(`POST /v1/payments/payouts` — send money to arbitrary recipients), and invoice
**cancel/delete**. These move or reverse real money and are one malformed
argument away from an irreversible transfer. Only invoicing's `create-draft` +
`send` are write verbs, and a draft is inert until explicitly sent. If a
money-movement verb is ever wanted it is a separate, human-gated follow-up —
noted here so the decision is explicit, not accidental. (This read-first + no
money-movement posture is a PayPal-tool decision, not inherited from a sibling
tool in the base.)

## 3. anycli definition & implementation

- **Type: `service`** (stage-1 rubric). No official non-interactive `--json`
  PayPal CLI exists → HTTP service in `internal/tools/paypal/` against the REST
  API. Mirror `internal/tools/notion/` shape: cobra tree grouped by resource
  (`invoice`, `transaction`, `balance`, `subscription`),
  `BaseURL`/`HC`/`Out`/`Err` struct for httptest fakes, exit-code contract
  (0 ok / 1 API failure via typed `apiError` / 2 usage) and a `--json`
  structured output + error envelope.

- **Credential shape — three fields, token exchange done in-service.** The
  service receives `client_id` + `api_secret` + `environment` (see §4 for the
  Helio projection) and performs the **client-credentials exchange itself**
  (`POST {host}/v1/oauth2/token`, Basic auth, `grant_type=client_credentials`),
  caches the bearer for the process lifetime, and calls the data endpoints with
  `Authorization: Bearer <token>`. `environment` (`live`|`sandbox`, default
  `live`) selects `{host}`. Keeping the OAuth token exchange inside anycli means
  Helio never runs the client-credentials grant or any refresh loop — it stores a
  static secret pair + an env label and projects them, so integration-service
  grows only static multi-field credential storage/projection, **not** OAuth /
  refresh machinery (the residual integration-service work is enumerated in §4.3;
  it is not "zero"). **PayPal introduces the in-service client-credentials
  exchange pattern for anycli — there is no shipped anycli precedent for it.** A
  grep across `internal/tools/` finds no existing `grant_type=client_credentials`
  token-exchange service, and integration-service itself only exchanges
  `authorization_code` / `refresh_token`, never `client_credentials`. anycli
  processes are short-lived, so the 9h token TTL is irrelevant — it exchanges
  fresh each run.

- **Definition `auth.credentials`** — three `field`→`env` bindings. Secret field
  is named **`api_secret`** (NOT `client_secret`) to keep the integration-service
  secret-field-name denylist intact (Divergence 5, §4.3) — its inject env var is
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
Two real in-base precedents combine here (no fictional siblings): the
**mongodb** bundle is the `runtime_strategy: manual_credentials` precedent —
store a user-supplied static secret, derive a readable account key, no
provider-side verify — but it is **single-field** (`connection_string` →
`token.access_token`). The **multi-field decoded-credential projection** has one
genuine in-base precedent: the **Lark** self-built-app path
(`runtime_strategy: lark_self_built_app`, design 255), whose token gateway
decodes a stored multi-field app credential into named `credential.*` sub-fields
(`credential.app_id`, `credential.brand`). PayPal is structurally the same
"self-built app credential, decoded into named fields" shape, so it extends the
`credential.*` family rather than inventing a parallel one (§4.3 step 2). It adds
one **non-secret** field (`environment`) alongside the two secrets. **No OAuth
block, no service adapter, and — because the client_id/secret are the *user's*,
not Helio's — zero integration-service `config/` + `deploy/` appends** (lane-1
config landing is N/A for PayPal).

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
  key): `client_id: credential.client_id`, `api_secret: credential.api_secret`,
  `environment: credential.environment`, `account_key: connection.account_key`.
  These extend the existing `credential.*` decoded-multi-field family (Lark
  precedent), **not** a new `token.*` family — see §4.3 step 2 for why the model
  invariant forces this and what token-gateway code must be added to populate
  them.
- `tool.name: paypal`, `tool.kind: api-key` (wire-compat value; drawer routes by
  auth_type). No `tool.group`, no `experiment` gate (GA once flipped).

### Identity — two options; **Option A recommended**

- **Option A (recommended) — client-credentials verifier + `app_id` identity.**
  Add a compiled `paypalClientCredentialsVerifier`. **No sibling verifier exists
  in the worktree base** — the only manual facet present is
  `declarativeManualTokenVerifier` (header + identity-endpoint) and the
  no-verify `dsnHostIdentityDeriver`; `sendgridScopesVerifier` /
  `postmarkServerVerifier` / `mastodonAccountVerifier` are unmerged sibling
  branches, not precedent to copy. The new verifier implements the same manual
  facet interface (`Verify(ctx, hc, def, secret) (identity, accountKey, label,
  err)`): parse the JSON-composite secret `{client_id, api_secret,
  environment}`, pick the host from `environment`, do the
  `POST /v1/oauth2/token` client-credentials exchange (the first such exchange in
  integration-service — it too has no in-base precedent), and on 200 derive the
  stable `account_key` + label from the returned **`app_id`** (e.g.
  `APP-80W284485P519543T`, suffixed with the environment). On 401 reject the pair
  as invalid at connect time. Benefits: catches typo/wrong-env credentials
  *before* storage, and yields a readable, environment-scoped account identity —
  worth the small capability growth given the sandbox/live split already forces
  host-selection logic server-side.
- **Option B (fallback, zero-verify) — `manual_credentials` no-verify shape.**
  `identity.source: strategy` reusing the `manual_credentials` runtime strategy,
  but the base's only deriver is `dsnHostIdentityDeriver` (DSN-host specific), so
  Option B still adds a new generic first-secret-field deriver (call it
  `multiFieldClientIdentityDeriver`) that decodes the JSON-composite secret and
  derives `account_key` from `client_id`, no network call at connect (a bad pair
  surfaces at first use via `CredentialRejected`). Lower capability cost — no
  client-credentials call — but not literally zero new code. Loses early
  validation and readable identity (client_id is an opaque 80-char string).

### Integration-service capability growth (design 317 D8 multi-field)

The worktree base still enforces the design 317 D5 **single-secret** rule — the
`cmd/provider-gen` validator rejects any declared `credential_input` that is not
**exactly one required field** (generator_test.go asserts the `"exactly one
required field"` error), and `mongodb`, the only shipped `manual_credentials`
tool, stores that single secret straight through `token.access_token` with
**zero new CredentialSource and zero token-gateway changes**. PayPal breaks that
"free ride": it needs three named sub-fields, so the whole multi-field
storage → decode → projection path is new work, not a no-op behind a sibling.

PayPal is a **Wave-1** tool, so it **co-introduces** the multi-field growth in
its Wave-1 batch. The earlier-cited `hotjar`/`snov`/`mixpanel`/`zoominfo` are all
later waves (snov/mixpanel = Wave 2; hotjar/zoominfo = 3-hold, Wave 3's final
batch) and have **not** merged by PayPal's batch-end, so PayPal cannot ride a
no-op behind them — and none of them exist in the worktree base to copy. Its
actual in-base structural precedents are `mongodb` (the `manual_credentials`
strategy) and `lark` (the multi-field `credential.*` decode-and-project path).
The growth below is **idempotent** across the Wave-1 multi-field peers (**Twilio**
Account SID + Auth Token row 5, **AWS** Access Key ID + Secret row 30), so
whichever lands it first, the rest merge it as a no-op; **the batch lead must
ensure it actually lands in the Wave-1 batch** rather than assuming a sibling
already carries it:

1. **Relax the connect-input schema to N≥1 required fields.** Loosen the
   `cmd/provider-gen` `credential_input` validator (and the mirror check in
   `service/provider_registry_validation.go`) from "exactly one required field"
   to allow multiple, permitting the two secrets plus one **non-secret** required
   field (`environment`). The `manual_credentials` connect path
   (`service/manual_credential.go`) must then **serialize the N submitted fields
   into the single stored vault secret** as a JSON composite `{client_id,
   api_secret, environment}` (today it stores one raw string).
2. **Add a token-gateway resolve arm that decodes the composite into named
   `credential.*` sub-fields** — this is the load-bearing new code, not just a
   vocabulary entry. Adding source constants alone does nothing: `projectCredential`
   (token_gateway.go) is a closed `switch` over five fixed sources reading
   pre-populated `TokenResult` fields, with **no path that reads vault credential
   JSON into named values**. So:
   - Extend the `credential.*` **decoded-multi-field** family (not a parallel
     `token.*` family) with `credential.client_id`, `credential.api_secret`,
     `credential.environment` in `model/catalog.go`. This is forced by the model
     invariant: the `CredentialSource` doc comment states client credentials
     "have no representable value" under `token.*`, and reserves `credential.*`
     for "resolve arms that already decode a multi-field self-built app
     credential (design 255 Lark)." PayPal's stored pair is exactly that shape,
     so it belongs in `credential.*`; inventing `token.*client_id` would
     contradict the invariant the comment sets.
   - Add the matching `TokenResult` fields and a `manual_credentials` resolve arm
     that JSON-decodes the stored secret and populates them, plus the
     `projectCredential` `case`s that map `credential.client_id →
     result.ClientID`, etc. (mirroring Lark's `result.AppID`/`result.Brand`
     population, but sourced from the stored composite rather than a minted
     token). Update the `CredentialSource` doc comment too: it currently names
     Lark as the *only* arm populating `credential.*`; PayPal's
     `manual_credentials` decode is a second such arm, so the invariant text must
     acknowledge it rather than be silently contradicted.
   - **Divergence 5 — the connect-input secret field is named `api_secret`,
     never `client_secret`.** The `cmd/provider-gen` secret-field-name denylist
     (`isForbiddenSecretFieldName`, validate.go) rejects any secret field whose
     name contains `client_secret` (a generator-reserved config token); naming it
     `api_secret` keeps the denylist total. This is orthogonal to the projection
     source name, which is `credential.api_secret` per the bullet above.
3. **Identity / verifier selection.** The `manual_credentials` arm in
   `service/provider_registry.go` currently **hardcodes** `dsnHostIdentityDeriver`
   with no branching, so either option requires branching that arm by provider /
   strategy: Option A selects `paypalClientCredentialsVerifier`; Option B selects
   the new generic first-secret-field deriver. It is not a pure "register a new
   facet" change.

**Scope honesty (correcting an earlier draft).** PayPal is **not** "zero
provider-specific runtime/exchange Go in integration-service beyond the
verifier." The OAuth **token exchange** (client-credentials → bearer) does live
entirely in anycli — integration-service never mints a PayPal bearer for data
calls. But integration-service still grows real multi-field plumbing: the
connect-path serialization (step 1), the composite-decode resolve arm +
`credential.*` projection cases (step 2), and the verifier-selection branch
(step 3). Only the *data-plane* token exchange is anycli-owned; the
storage/projection is shared-surface Go that this batch must land.

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
| **L1** anycli `go test ./...` | httptest fakes: client-credentials POST body + Basic header + bearer injected on data call; host selection by `PAYPAL_ENV`; `invoice list`/`get`/`search`, `transaction list` (start/end + 31-day guard), `subscription get` request shapes; plain + `--json` 401/403/422/429 rendering. No real API. | No |
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
