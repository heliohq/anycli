# Tool design: Square (`heliox tool square`)

Scratch design doc for the `tool/square` branch pair (anycli + Helio). Batch lead
strips this at batch-end. English-only, per anycli AGENTS.md.

- **anycli id (axis ②):** `square`
- **provider catalog key (axis ③):** `square`
- **CLI command word (axis ①):** `square`
- All three axes are identical → **no `toolToProvider` resolver entry** (the map
  is only for ②↔③ divergences). No grouped family.
- **Auth lane:** `oauth_light` (master plan row 40; OAuth audit row 40, high
  confidence). **Confirmed against official docs** — see §3.
- **Wave/batch:** Wave 1, Payments & Commerce.
- **Tool form:** `service` type (§2).

Everything below was verified against Square's official developer docs, not
inherited from the catalog:
- OAuth API overview: https://developer.squareup.com/docs/oauth-api/overview
- ObtainToken reference: https://developer.squareup.com/reference/square/oauth-api/obtain-token
- API index: https://developer.squareup.com/reference/square

---

## 1. What an AI teammate does with Square, and the API surface it wraps

Square is a payments + commerce platform for sellers (in-person POS, online
store, invoicing, subscriptions). An AI teammate connected to a seller's Square
account is a back-office operator: it answers "how are sales doing", pulls order
and payment history, looks up and updates customer profiles, reads the product
catalog and inventory, and drafts/sends invoices. All of that is the **v2 REST
API** under a single host with a single bearer token — no SDK, no binary.

- **Base host:** `https://connect.squareup.com` (production);
  `https://connect.squareupsandbox.com` (sandbox). All resource paths are
  `/v2/<resource>`.
- **Every request:** `Authorization: Bearer <access_token>` +
  `Square-Version: <YYYY-MM-DD>` (pin one date, e.g. `2026-07-15`) + JSON
  bodies. Confirmed on the ObtainToken curl example and the API index's
  "API version 2026-07-15" label.

Resource groups the tool wraps (driven by the teammate jobs above), each a v2
REST group with list/search/retrieve/create semantics:

| Resource | Path | Why (teammate job) |
|---|---|---|
| Payments | `/v2/payments` | list/retrieve payments — "how much did we take" |
| Orders | `/v2/orders/search`, `/v2/orders/{id}` | itemized sales data (search is POST) |
| Customers | `/v2/customers` | list/search/retrieve/create/update CRM profiles |
| Catalog | `/v2/catalog/list`, `/v2/catalog/search`, `/v2/catalog/object/{id}` | product/price lookup |
| Invoices | `/v2/invoices`, `/v2/invoices/search` | draft / list / retrieve invoices |
| Inventory | `/v2/inventory/counts/batch-retrieve` | stock levels for a variation |
| Locations | `/v2/locations` | resolve `location_id` (needed by orders/catalog) |

`Locations` is load-bearing: most write/list calls are location-scoped, so
`location list` is the discovery primitive an agent calls first. Payments,
Orders, and Invoices are the highest-value reads. Writes (create customer,
create/publish invoice) are in scope but flagged as side-effecting (§2).

**Not wrapped:** taking live card payments (`CreatePayment` with a card nonce)
is a POS/checkout flow, not an agent job — omit from the initial verb set;
revisit only if a real teammate scenario needs it. Loyalty/Subscriptions/Team
are lower demand and can be added incrementally without changing the auth or
bundle shape.

## 2. anycli definition

**Form decision — `service` type.** Stage-1 rubric: a `cli` type is justified
only when an official, non-interactive, `--json`-capable binary can be
provisioned into the runtime image. Square ships client **SDKs** (language
libraries) but no official agent-friendly CLI binary, so none of the `cli`
conditions hold. Implement `service` type against the v2 REST API — matching 21
of 23 existing definitions and the `notion`/`bitly` precedents.

**Package name (stage-2 rule):** anycli id `square` has no dashes and no leading
digit, so the Go package is `internal/tools/square/` and the registration string
is `RegisterService("square", &square.Service{})` in `internal/tools/register.go`.

**Definition JSON** (`definitions/tools/square.json`) — the credential contract
only; identical shape to `bitly.json`:

```json
{
  "name": "square",
  "type": "service",
  "description": "Square as a tool (OAuth 2.0 access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SQUARE_ACCESS_TOKEN"}
      }
    ]
  }
}
```

**Command tree** (cobra, grouped-by-resource, mirroring `notion.go`'s shape —
`BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest` server;
exit-code contract 0 success / 1 API-or-transport failure / 2 usage/parse):

```
square payment  list | get
square order    search | get
square customer  list | search | get | create | update
square catalog   list | search | get
square invoice   list | search | get | create | publish
square inventory get
square location  list | get
square api  <METHOD> <path> [--body JSON]      # raw v2 escape hatch
```

- Read verbs (`list`/`get`/`search`) → `anycli.side_effect: false`. Note Square
  **search endpoints are POST** (`/v2/orders/search`, `/v2/customers/search`,
  `/v2/catalog/search`, `/v2/invoices/search`) but are documented read-only, so
  they are classified `false` — the same POST-is-a-lookup case the notion
  `search` / `data-source query` annotation table already handles.
- Write verbs (`create`/`update`/`publish`) → `side_effect: true`.
- `api` raw escape hatch: method is runtime input → `side_effect: true`.

**JSON output shape.** Square already returns JSON envelopes
(`{"payments":[...],"cursor":"..."}`, `{"errors":[...]}`). The service emits the
provider body **verbatim** to stdout (`emitJSON`, the notion precedent) — no
re-wrapping, so cursors and Square's structured `errors[]` reach the agent
intact. Pagination `--cursor` flag registers locally on `list`/`search` verbs
only (notion's `registerPaginationFlags` pattern). Errors: non-2xx → `apiError`
(exit 1) rendering Square's `errors[].detail`; under `--json` the structured
`{"error":{"kind":"api","status":<code>,...}}` envelope.

**Client auth injection.** anycli injects `SQUARE_ACCESS_TOKEN`; the service
sets `Authorization: Bearer <token>` and the fixed `Square-Version` header on
every request. anycli never sees OAuth/refresh — the host resolves a live access
token through the token gateway (design 227).

## 3. Credentials and the exact auth flow (oauth_light, verified)

**Registration model — self-serve, no review (oauth_light confirmed).** Square's
OAuth API overview documents a standard OAuth 2.0 authorization-code flow where
**any** Square seller authorizes your **one** registered app; app creation and
production credentials are self-serve in the Developer Console. App Marketplace
listing and the partner program are **optional**, not gates. This matches the
audit's row-40 `oauth_light` verdict — no divergence to record.

**Flow choice — confidential code flow, not PKCE.** Square supports both. Helio
is a confidential server-side client (integration-service holds the secret), and
the **code flow yields a multi-use, non-expiring refresh token**, whereas the
PKCE flow's refresh tokens are **single-use and expire after 90 days**. The code
flow is strictly better for a long-lived server integration → `pkce: none`,
client authenticated with `client_secret`.

**Endpoints (official):**
- Authorize: `https://connect.squareup.com/oauth2/authorize`
  (`?client_id=&scope=&state=&session=false`), scopes space-delimited.
- Token (ObtainToken): `POST https://connect.squareup.com/oauth2/token`.

**Token exchange transport — the one capability gap.** ObtainToken requires a
**JSON body** (`Content-Type: application/json`) carrying `client_id`,
`client_secret`, `code`, `grant_type`, `redirect_uri` **as JSON body fields**
(the official curl example). The refresh call is the same endpoint with
`grant_type=refresh_token` + `refresh_token`.

The integration-service `standard_oauth` exchanger's closed enum today is
exactly `form_secret | form_basic | json_basic`
(`model/catalog.go`, `service/oauth_exchange.go:buildTokenRequest`,
`cmd/provider-gen/validate.go:255`). None fits Square:
- `form_secret` / `form_basic` → form-encoded body (Square wants JSON).
- `json_basic` → JSON body but client creds in an `Authorization: Basic` header
  (Square wants them **in the JSON body**; it does not document Basic).

→ **Capability growth required: add a `json_secret` token-exchange style** — JSON
body with `client_id`/`client_secret` as body fields. This is the minimal
orthogonal enum addition the integration-service AGENTS.md explicitly sanctions
("a response shape … outside that closed capability set needs a compiled generic
capability … never an unbounded YAML expression"), mirroring exactly how
`json_basic` was added for Notion. Touch points:
`model/catalog.go` (const + `UsesHTTPBasicClientAuth` stays false for it),
`service/oauth_exchange.go` (`buildTokenRequest` new case: JSON body including
`client_id`/`client_secret`; `validateTokenExchangeStyle`),
`cmd/provider-gen/{validate.go,render_symbols.go}` (allow the value + Go symbol),
`service/token_refresh.go` (refresh reuses the same JSON-body-with-secret shape).
Unit tests: exchange builds a JSON body with both creds and no Basic header.

**Token semantics / refresh:**
- Access token expires in **30 days**; response carries `expires_at` (ISO 8601)
  and `merchant_id`. The `tokenResponse` struct already captures
  `access_token`/`refresh_token`; `expires_at` drives the gateway's lazy
  refresh + A3 strict write-back.
- Code-flow refresh tokens are multi-use, non-expiring, and refreshing does
  **not** invalidate previously issued access tokens (multiple active tokens
  allowed) → **no cross-token invalidation** → `refresh_lease: none`. Verified
  that `acquireRefreshLease` returns `(nil,nil)` for `OAuthLeaseNone` while
  `requestOAuthRefresh` still runs, so refresh works without a lease.

**Identity:** `merchant_id` is returned **inline in the token response**
(ObtainToken), so no separate userinfo GET is needed:
```yaml
identity:
  source: token_response
  stable_key: /merchant_id
  label_candidates: [/merchant_id]
```
(`merchant_id` is a stable string; no numeric-coercion concern.)

**Disconnect / revoke — `local_only`.** Square's revoke endpoint
(`POST /oauth2/revoke`) uses a **non-standard** client-auth scheme
(`Authorization: Client <application_secret>`), which the declarative revoker's
reviewed set (`none | basic | form`) cannot express. Rather than a bespoke
adapter for best-effort cleanup, use `disconnect_mode: local_only` — the same
choice `notion` and `bitly` make. (A `square`-specific declarative-revoke enum
value is a possible later addition; not needed to ship.)

**Config fields:** `auth.required_config_fields: [oauth.client_id,
oauth.client_secret]`. Lane 1 lands the real Square app id/secret in
integration-service config — `config/` locally **and** the `deploy/` Helm Secret
together (Config Sync rule), id + secret in the same change (a partially
configured provider fails startup). Never in the bundle.

**Scopes (display + wire).** Space-delimited on the authorize URL. Initial set
covers the read-first surface plus customer/invoice writes:
`MERCHANT_PROFILE_READ PAYMENTS_READ ORDERS_READ CUSTOMERS_READ CUSTOMERS_WRITE
ITEMS_READ INVENTORY_READ INVOICES_READ INVOICES_WRITE`. `display_scopes` uses
the same slugs for the consent card (i18n `tools.scopes.<slug>`).

## 4. Helio provider bundle plan

`integrations/providers/square/provider.yaml`, **hidden-first**
(`presentation.visible: false`) — decouples rollout from anycli pin timing;
registers as a cobra-hidden but runnable command so L4/L5 run against it as-is.
Modeled on the `notion` bundle (token-response identity, `standard_oauth`), with
the Square-specific auth block:

```yaml
schema: helio.provider/v1
key: square
go_name: Square

presentation:
  name: Square
  description_key: square
  consent_domain: squareup.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <next in Payments & Commerce>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://connect.squareup.com/oauth2/authorize
    token_url: https://connect.squareup.com/oauth2/token
    token_exchange_style: json_secret     # NEW enum value (§3)
    pkce: none
    authorize_params: {}
    display_scopes: [MERCHANT_PROFILE_READ, PAYMENTS_READ, ORDERS_READ,
      CUSTOMERS_READ, CUSTOMERS_WRITE, ITEMS_READ, INVENTORY_READ,
      INVOICES_READ, INVOICES_WRITE]
    single_active_token: false
    refresh_lease: none

identity:
  source: token_response
  stable_key: /merchant_id
  label_candidates: [/merchant_id]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: standard_oauth

resources: {selection: none, discovery: none, enforcement: none}

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: square
  kind: oauth
```

Axis naming per master-plan §3: command word / anycli id / provider key all
`square` (hidden-first; no group). Generation (`provider-gen` + `--check`) is run
locally for validation only and **not committed** — the batch lead produces the
one canonical regen (five projections) and pin bump at batch end.

**Also required, riding the batch-end merge:**
- UI icon `ui/helio-app/src/integrations/icons/square.svg` + `providerIcons.ts`
  registration (manual; never generated).
- i18n `tools.scopes.*` strings for the display scopes + `description_key`.
- AI-facing provider sub-doc under `agents/plugins/heliox/skills/tool/`.
- integration-service `json_secret` capability growth (§3) — lands with the
  bundle so `provider-gen --check` and the OAuth callback both accept the style.

## 5. Test plan — five layers

| Layer | What it proves for Square | External creds needed? |
|---|---|---|
| **L1** anycli `go test ./...` | `square` service unit tests against an `httptest` fake: Bearer + `Square-Version` header injected; POST-search request shapes; verbatim JSON stdout; `errors[]` → exit 1; `--json` error envelope; `side_effect` annotation table. | No (fakes only). |
| **L2** dev harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<sandbox token> anycli square -- location list` against `connect.squareupsandbox.com` returns real data. Proves field name / injection / request shape match the live API — **mandatory before pin bump**. | **Yes** — a Square **sandbox access token** from a test app (self-serve). |
| **L3** `provider-gen --check` + both repos' suites | Bundle strict-decodes; the new `json_secret` enum passes validate + the integration-service exchanger/refresh unit tests; `helio-cli` builds with a local `go.mod replace` at the anycli branch. | No. |
| **L4** singleton + seeded creds | `POST /internal/test-only/connections/seed` a `square` connection (user-token provider, seedable) with a real sandbox `access_token` + `refresh_token` + short `expires_at`, then `heliox tool square -- payment list` reaches the live API through the token gateway; short expiry forces the refresh + A3 write-back path (exercising `json_secret` refresh). | **Yes** — sandbox `access_token`/`refresh_token` from a registered app. |
| **L5** full connect flow (once, pre-flip) | `heliox tool square auth` → consent on the Square sandbox app → `oauth_connected` event on the auth channel → unseeded live `square` run. Validates the real authorize URL, `json_secret` code exchange, `merchant_id` identity extraction, callback. Human-in-the-loop (oauth L5, master-plan lane 3). | **Yes** — a real Square **dev/sandbox app** (lane 1) + a **sandbox seller account** to consent (lane 2). |

L1/L3 are self-contained. L2/L4/L5 need externally supplied credentials: a Square
dev application (client id/secret, lane 1) and a sandbox seller account / token
(lane 2). Square's sandbox is self-serve, so procurement is light — no review
clock (that's exactly why row 40 is `oauth_light`).

**Definition of done:** all five layers green, docs published, icon registered,
then `visible: true` + regenerate as the single go-live change. Until the flip:
code-complete (hidden).
