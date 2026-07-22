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

Square's JSON-body-with-secret contract forces **three distinct capability
growths**, not one shared enum case. The initial exchange, the refresh path, and
expiry capture are separate code paths in integration-service; a single
`buildTokenRequest` case only covers the first. All three land with the bundle.

**Growth A — `json_secret` token-exchange style (initial code exchange).** Add
`TokenExchangeJSONSecret` to the closed enum: JSON body carrying `client_id` /
`client_secret` / `code` / `grant_type` / `redirect_uri` as body fields, no
`Authorization` header. This is the minimal orthogonal enum addition the
integration-service AGENTS.md sanctions ("a response shape … outside that closed
capability set needs a compiled generic capability … never an unbounded YAML
expression"), mirroring how `json_basic` was added for Notion. Touch points:
`model/catalog.go` (const + `UsesHTTPBasicClientAuth()` **stays false** for it —
creds go in the body, never Basic), `service/oauth_exchange.go`
(`buildTokenRequest` new case: JSON body including both creds;
`validateTokenExchangeStyle`), `cmd/provider-gen/{validate.go,render_symbols.go}`
(allow the value + Go symbol). Unit test: exchange builds a JSON body with both
creds and no Basic header.

**Growth B — `json_secret` refresh in `service/token_refresh.go` (larger, not a
`buildTokenRequest` case).** This is the correction to the original design's
under-scoping. `buildTokenRequest` is called **only** by `exchangeAuthCode` (the
initial code exchange); it is never on the refresh path. Refresh runs through
`requestOAuthRefresh` (`token_refresh.go:148-169`), which builds its request with
`golang.org/x/oauth2` (`cfg.TokenSource(ctx, …).Token()`). `x/oauth2` **always**
emits `application/x-www-form-urlencoded` and can never emit a JSON body — so a
`json_secret` case in `buildTokenRequest` does nothing for refresh, and a Square
refresh routed through `x/oauth2` would be form-encoded and non-conforming to
Square's documented JSON-only `/oauth2/token` contract (the same endpoint serves
`grant_type=refresh_token`, verified in the official ObtainToken reference).
Required change: for the `json_secret` style, **abandon `oauth2.TokenSource` and
hand-roll a JSON POST** in `token_refresh.go` — body
`{grant_type: refresh_token, refresh_token, client_id, client_secret}`,
`Content-Type: application/json`, no Basic header — reusing the existing
`tokenResponse` decode. It must preserve its own A3 strict write-back,
refresh-token carry-forward (`firstNonEmpty(newTok.RefreshToken, td.RefreshToken)`),
and permanent-vs-transient error classification, exactly as the `x/oauth2` branch
does today. `UsesHTTPBasicClientAuth()` must stay false for `json_secret`, so the
existing confidential-client creds guard still applies without forcing Basic.
Unit test: a `json_secret` refresh request carries a JSON body with **both** creds
and **no** `Authorization: Basic` header.

**Growth C — capture Square's absolute `expires_at` on BOTH exchange and refresh
write-back.** Correcting a false claim in the original design: the generic
`tokenResponse` (`oauth_exchange.go:21-37`) reads **only** `expires_in` (int
seconds), and `tok.expiry()` returns `nil` when `ExpiresIn <= 0`. Square's token
response carries **no** `expires_in` — it returns `expires_at` (absolute ISO 8601;
verified in the official ObtainToken reference). There is currently **zero**
`expires_at` parsing anywhere on the exchange/refresh path. Consequence if left
unfixed: after a real exchange, `oauth_credentials.go` stores `Expiry=nil`;
`token_gateway.go:needsRefresh` (`429-434`) returns `false` whenever
`Expiry==nil` ("non-expiring"), so the token is **never refreshed** — the Square
access token silently dies at 30 days and the connection bricks until a manual
reconnect, precisely the failure the refresh machinery exists to prevent (and a
poor experience for an AI teammate). The refresh side compounds it: even if
refresh fired, `x/oauth2` parses `expires_in` too, so `newTok.Expiry` would be
zero and `refreshed.Expiry` (`token_refresh.go:53-55`) would stay `nil`. Required
change: an `expires_at`-aware decode (analogous to the `assumed_ttl` /
metadata-deriver precedents on other providers) that parses Square's absolute
`expires_at` into the credential `Expiry` on **both** the initial exchange
write-back (`oauth_credentials.go`) and the `json_secret` refresh write-back
(`token_refresh.go`). Unit test: after decoding a Square token response, the
persisted `Expiry` equals the parsed `expires_at` (non-nil, ~30 days out), so
`needsRefresh` fires as the token nears expiry.

**Token semantics / refresh:**
- Access token expires in **30 days**; response carries `expires_at` (ISO 8601)
  and `merchant_id`, and **no** `expires_in`. Growth C parses `expires_at` into
  the credential `Expiry` on both exchange and refresh write-back so the gateway's
  lazy refresh + A3 strict write-back actually fire (without it, `Expiry=nil` ⇒
  `needsRefresh` never triggers ⇒ the token is never refreshed).
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
- integration-service `json_secret` capability growth (§3, Growths A/B/C) —
  the exchange style, the hand-rolled JSON refresh in `token_refresh.go`, and the
  `expires_at`-aware `Expiry` capture all land with the bundle so
  `provider-gen --check`, the OAuth callback, and the token gateway's refresh
  cadence are all correct.

## 5. Test plan — five layers

| Layer | What it proves for Square | External creds needed? |
|---|---|---|
| **L1** anycli `go test ./...` | `square` service unit tests against an `httptest` fake: Bearer + `Square-Version` header injected; POST-search request shapes; verbatim JSON stdout; `errors[]` → exit 1; `--json` error envelope; `side_effect` annotation table. | No (fakes only). |
| **L2** dev harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<sandbox token> anycli square -- location list` against `connect.squareupsandbox.com` returns real data. Proves field name / injection / request shape match the live API — **mandatory before pin bump**. | **Yes** — a Square **sandbox access token** from a test app (self-serve). |
| **L3** `provider-gen --check` + both repos' suites | Bundle strict-decodes; the new `json_secret` enum passes validate + the integration-service exchanger/refresh unit tests; `helio-cli` builds with a local `go.mod replace` at the anycli branch. | No. |
| **L4** singleton + seeded creds | `POST /internal/test-only/connections/seed` a `square` connection (user-token provider, seedable) with a real sandbox `access_token` + `refresh_token` + a near-past/short `expires_at`, then `heliox tool square -- payment list` reaches the live API through the token gateway. The seeded short expiry makes `needsRefresh` fire, driving the `json_secret` refresh (Growth B) + A3 write-back; assert the persisted credential afterward carries a **non-nil `Expiry` ~30 days out** parsed from the refresh response's `expires_at` (Growth C) — proving the refresh both fired and re-armed the next refresh, not just fired once. Without Growth C the write-back would persist `Expiry=nil` and the cadence would silently stop, so this assertion is the honest proof, not "a repeating cadence" by itself. | **Yes** — sandbox `access_token`/`refresh_token` from a registered app. |
| **L5** full connect flow (once, pre-flip) | `heliox tool square auth` → consent on the Square sandbox app → `oauth_connected` event on the auth channel → unseeded live `square` run. Validates the real authorize URL, `json_secret` code exchange, `merchant_id` identity extraction, callback. Human-in-the-loop (oauth L5, master-plan lane 3). | **Yes** — a real Square **dev/sandbox app** (lane 1) + a **sandbox seller account** to consent (lane 2). |

L1/L3 are self-contained. L2/L4/L5 need externally supplied credentials: a Square
dev application (client id/secret, lane 1) and a sandbox seller account / token
(lane 2). Square's sandbox is self-serve, so procurement is light — no review
clock (that's exactly why row 40 is `oauth_light`).

**Definition of done:** all five layers green, docs published, icon registered,
then `visible: true` + regenerate as the single go-live change. Until the flip:
code-complete (hidden).
