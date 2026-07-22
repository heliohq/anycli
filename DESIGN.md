# Tool design: Shopify

Batch scratch doc for the `tool/shopify` branch (both repos). Stripped by the
batch lead at batch-end. Everything below was verified against Shopify's
official developer docs and the actual repo code on 2026-07-23, not inherited
from the catalog or audit.

- **Catalog row 36** — Product: Shopify · anycli id: `shopify` · provider key:
  `shopify` · auth lane: `oauth_review` · Wave 1 · Payments & Commerce.
- **Naming axes** — ① CLI word `shopify` · ② anycli id `shopify` · ③ provider
  key `shopify`. All three identical → **no `toolToProvider` entry** (identity
  holds; do not add a resolver map row).
- **Go package** — `internal/tools/shopify/` (id has no dash/leading digit).

---

## 1. Divergences from the catalog / audit (record-and-follow-official-docs)

Per the master plan's "independent judgment" rule, two facts materially change
the build from what a naive "Shopify = REST + non-expiring token" design would
assume. Both are driven by Shopify changelog deadlines that are **already past**
as of the 2026-07-23 build date:

1. **GraphQL is mandatory; REST is off the table for a new app.** The REST
   Admin API became *legacy* on 2024-10-01, and since **2025-04-01 all new
   public apps must be built exclusively on the GraphQL Admin API** — a new REST
   app cannot be listed or newly integrated
   (https://shopify.dev/changelog/starting-april-2025-new-public-apps-submitted-to-shopify-app-store-must-use-graphql).
   Helio's Shopify app is a *new* public app, so the anycli service wraps the
   **GraphQL Admin API** (`POST /admin/api/<version>/graphql.json`), not REST
   resource routes. This is the single biggest shape decision and it is not
   optional.

2. **Expiring offline tokens are mandatory → Shopify is a refresh-cycle
   provider, not a static-token one.** Non-expiring offline tokens were the old
   default, but **as of 2026-04-01 new public apps must use expiring offline
   access tokens** (https://shopify.dev/changelog/offline-access-tokens-now-support-expiry-and-refresh
   + Power Commerce changelog mirror). Expiring offline access tokens live
   **1 hour (3600s)**; they come with a **refresh token** that **rotates on every
   refresh** and itself expires after **90 days** of non-use. So Shopify sits on
   the standard-OAuth **refresh + strict write-back (A3)** path — the same class
   as Square/Adobe Sign, *not* the non-expiring-bot-token class (Slack/Notion).

The catalog's `oauth_review` lane is **confirmed correct** (see §4): dev/test
against development stores needs no review, but distributing to arbitrary
production merchants and reading order/customer PII both gate on Shopify review
— exactly the hidden-first / review-gates-only-the-visible-flip model.

A third structural fact — **per-shop instance-scoped OAuth** (the authorize and
token URLs both embed `{shop}.myshopify.com`) — is not a divergence from the
catalog but is the main integration-service capability question (§4, §5).

---

## 2. Official API surface this tool wraps, and why

**What an AI teammate actually does with Shopify** (drives endpoint choice): a
merchant-facing teammate answers "how many orders today / what's low on stock /
who are my recent customers / bump this product's price / draft an order",
i.e. **read-mostly store operations plus targeted writes** over the four core
commerce objects. That maps to a thin, verb-first wrapper over the **GraphQL
Admin API**, which is the *only* API a new app may use.

- **Single endpoint**: `POST https://{shop}.myshopify.com/admin/api/<version>/graphql.json`
  with the GraphQL query/mutation in the JSON body. Current stable version
  **`2026-07`** (quarterly, date-based, 12-month support; pin it, don't use
  `latest`) — https://shopify.dev/docs/api/admin-graphql/latest and
  https://shopify.dev/docs/api/usage/versioning.
- **Auth header** (verified): `X-Shopify-Access-Token: <access_token>` — a
  Shopify-specific header, **not** `Authorization: Bearer`. Content-Type
  `application/json`.

**Endpoint/verb coverage** (all one GraphQL POST under the hood; the service
exposes them as ergonomic subcommands so the AI never hand-writes GraphQL):

| Resource | Reads | Writes |
|---|---|---|
| `product` | `list` (query `products`), `get` | `create`, `update` (e.g. price/status via `productUpdate` / `productVariantsBulkUpdate`) |
| `order` | `list` (query `orders`), `get` | `create` (`draftOrderCreate` → complete), `update` tags/note |
| `customer` | `list`, `get` | `create`, `update` |
| `inventory` | `levels` (inventory levels by location) | `adjust` (`inventoryAdjustQuantities`) |
| `shop` | `info` (`shop { name currencyCode ... }`) | — (identity/health check) |
| `graphql` | raw passthrough: `-- graphql --query '<gql>' [--variables '<json>']` | escape hatch for anything not modeled |

**Why these, not more**: products/orders/customers/inventory + shop-info are the
store objects a teammate reasons about daily; the raw `graphql` passthrough
keeps the surface small while leaving the full Admin schema reachable. Payments
capture, fulfillment orchestration, and app-billing are deliberately **out of
scope for v1** (heavier consent, narrower teammate value) and reachable via the
passthrough if needed.

**Non-goals**: Storefront API (unauthenticated buyer surface), Partner API, and
webhooks/inbound (that is gateway/design-228 territory, not `heliox tool`).

---

## 3. The anycli definition

**Stage-1 tool-form rubric → `service` type.** No official Shopify CLI fits the
`cli` bar: the Shopify CLI (`@shopify/cli`) is an app-*scaffolding* / dev tool,
not a non-interactive, `--json`, credential-via-env data-plane binary against a
merchant store. So implement a **`service`-type** tool in
`internal/tools/shopify/` against the GraphQL Admin API — matching 21/23 shipped
definitions.

**Definition JSON** (`definitions/tools/shopify.json`) — note the **two**
credential bindings, because the service needs both the token and the shop host:

```json
{
  "name": "shopify",
  "type": "service",
  "description": "Shopify store admin (GraphQL Admin API) as a tool",
  "auth": {
    "credentials": [
      { "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SHOPIFY_ACCESS_TOKEN"} },
      { "source": {"field": "store"},
        "inject": {"type": "env", "env_var": "SHOPIFY_STORE"} }
    ]
  }
}
```

- `access_token` → `X-Shopify-Access-Token` header (set in service code).
- `store` → the `{shop}.myshopify.com` host used to build the base URL. This is
  sourced from the connection's `account_key` on the Helio side (§5 credential
  projection), so anycli stays credential-shape-agnostic — it just receives two
  string fields from the resolver map.

**Service shape** (copy `internal/tools/notion/` structure): a cobra tree
grouped by resource (`product`, `order`, `customer`, `inventory`, `shop`) plus
the top-level `graphql` passthrough; a `BaseURL`/`APIVersion`/`HC`/`Out`/`Err`
struct so tests point at an `httptest` server; the documented exit-code contract
(0 ok, 1 runtime/API failure via typed `apiError`, 2 usage/parse) with a
`--json` structured error envelope. Base URL built as
`https://<SHOPIFY_STORE>/admin/api/<APIVersion>/graphql.json`; `APIVersion`
defaults to `2026-07` (a constant, overridable by flag for forward-compat).

**JSON output shape** (provider-neutral, agent-tuned — the built-in-service
conventions of design 003): every subcommand supports `--json` and prints a
flat, unwrapped envelope, unwrapping Shopify's GraphQL `{ "data": {...} }` and
surfacing `userErrors` as a non-zero exit with a structured error, not a silent
success. Example `shopify product list --json`:

```json
{"products":[{"id":"gid://shopify/Product/123","title":"Tee","status":"ACTIVE","totalInventory":42}],"page_info":{"has_next_page":true,"end_cursor":"ey..."}}
```

Cursor pagination is exposed via `--limit` / `--after <cursor>` flags mapping to
GraphQL `first`/`after`; the service never auto-follows unbounded pages.

**GraphQL `userErrors` are failures.** Shopify mutations return HTTP 200 with a
`userErrors: [...]` array on validation failure (Slack-`ok:false`-style dialect,
but inside `data`). The service MUST treat a non-empty `userErrors` as exit 1
with the messages in the error envelope — never report a no-op mutation as
success.

**L1 unit tests** (TDD, httptest fakes, never the live API): assert the request
POSTs to `/admin/api/2026-07/graphql.json`, carries `X-Shopify-Access-Token`,
sends the expected query/variables per subcommand, and renders both plain and
`--json` output including the `userErrors`→exit-1 and HTTP-401→exit-1 paths.

---

## 4. Credential fields & the exact auth flow (oauth_review, verified)

**Registration model.** A **public app** is created in the **Shopify Partner
Dashboard**, yielding a **Client ID + Client secret** (API key / secret). These
are the Helio-environment OAuth client credentials — server config, never in the
bundle. Dev/test installs on **development stores** work immediately with no
review; installing on arbitrary **production** stores and reading protected data
is what gates on review.

**Why `oauth_review` (confirmed against official docs, two independent gates):**
1. **App Store listing / public distribution review** to reach arbitrary
   merchants beyond dev stores (https://shopify.dev/docs/apps/launch/app-store-review/submit-app-for-review).
2. **Protected Customer Data approval.** Orders/customers carry customer PII;
   since API version 2022-10 the Admin API **redacts protected customer data by
   default**, and access to it (and to protected *fields* like name/email/
   address) requires a Partner-Dashboard request **reviewed by Shopify**.
   Critically: *"You don't need to submit a request for review for apps that are
   installed only on development stores… you will need approval to access
   protected customer data on any store that isn't a development store"*
   (https://shopify.dev/docs/apps/launch/protected-customer-data). This is the
   textbook hidden-first shape: **dev/L4 unaffected, review clearance gates only
   the visible flip.**

**OAuth authorization-code flow (offline, expiring — the mandated path):**

1. **Instance capture.** The user supplies their shop (`myshop.myshopify.com` or
   just `myshop`). This is required *before* the authorize URL can be built
   because the host is per-shop.
2. **Authorize** (redirect, per-shop host):
   `https://{shop}.myshopify.com/admin/oauth/authorize?client_id=<id>&scope=<csv>&redirect_uri=<cb>&state=<state>`
   — Shopify scopes are **comma-separated** (e.g. `read_products,write_products,read_orders,read_customers`),
   which differs from the space-joined default in `buildOAuthAuthorizeURL` (§5).
   Offline is the default grant (no `grant_options[]=per-user`).
3. **Merchant consent** on the shop's admin.
4. **Callback** → `code`. **Exchange** (POST, per-shop host):
   `POST https://{shop}.myshopify.com/admin/oauth/access_token`
   form/JSON body `{client_id, client_secret, code}` **plus `expiring=1`** to get
   the mandated expiring offline token. Response:
   `{access_token, refresh_token, expires_in≈3600, refresh_token_expires_in, scope}`.
5. **Refresh** (server-side, near expiry): same endpoint,
   `grant_type=refresh_token` → **new access token AND new refresh token**
   (rotation). Losing the rotated refresh token bricks the connection → this is
   exactly the A3 strict-write-back invariant.

**Credential fields**
- Helio env config (per environment, `config/` + `deploy/` Secret in sync):
  `oauth.client_id`, `oauth.client_secret`. Bundle
  `auth.required_config_fields: [oauth.client_id, oauth.client_secret]`.
- Vault-stored per connection: `access_token` (+ `refresh_token`, `expires_at`).
- **`account_key` = the `{shop}.myshopify.com` domain** — stable per store,
  captured at connect start. The offline token exchange response contains **no**
  shop identifier, so identity must come from the captured instance, not the
  token response (contrast Salesforce, whose `instance_url` is in the token
  response).

---

## 5. Helio provider bundle plan (`integrations/providers/shopify/provider.yaml`, hidden-first)

Axis ③ key `shopify`; `visible: false` initially. Sketch (fields firm up
against the batch-base capability set):

```yaml
schema: helio.provider/v1
key: shopify
go_name: Shopify
presentation:
  name: Shopify
  description_key: shopify
  consent_domain: myshopify.com
  visible: false
  order: <next>
auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    # instance-scoped: {shop} is templated from the captured instance (see below)
    authorize_url: https://{shop}.myshopify.com/admin/oauth/authorize
    token_url: https://{shop}.myshopify.com/admin/oauth/access_token
    token_exchange_style: form_secret       # client_id+secret in the form body
    scope_separator: ","                     # Shopify scopes are comma-separated
    pkce: none
    authorize_params: { }                    # offline default; add expiring=1 at exchange
    display_scopes: [read_products, write_products, read_orders, read_customers, read_inventory]
    single_active_token: false
    refresh_lease: none                       # per-connection independent refresh
identity:
  source: strategy                            # account_key = captured shop instance
  stable_key: <shop-instance>                 # not from token response
connection:
  mode: isolated
  disconnect_mode: local_only                 # Shopify offline tokens revoke on app uninstall; no RFC-7009 endpoint
  runtime_strategy: standard_oauth
credential:
  fields:
    access_token: token.access_token
    store: connection.account_key             # → SHOPIFY_STORE env in anycli
tool:
  name: shopify
  kind: oauth
```

**Axis ①/②/③**: all `shopify`; no `tool.group`, no `tool.command`, no
`toolToProvider` row.

**Capability growth (the real work — `standard_oauth`, not an adapter).** Shopify
needs three generic capabilities on the standard-OAuth path; none is
Shopify-specific scripting, and each is a reviewed enum/field, per
integration-service/CLAUDE.md ("a new standard provider should not need an
adapter"):

1. **Instance-scoped authorize/token URL templating (`{shop}`).** The user
   supplies the shop at connect start; the service templates it into the
   authorize host and stores it so the token exchange + refresh reuse the same
   host, and sets `account_key` from it. **This is the same capability the
   Zendesk `{subdomain}.zendesk.com` instance-scoped OAuth (batch task "Option
   A") and Salesforce instance work introduced.** *Reuse it if it has landed on
   the batch base by dev time* — grep `oauth_start.go` / `connect_intent.go` /
   `catalog.go` for an instance/subdomain capture field before writing new code;
   only if absent do we add it, mirroring that design. Do **not** fork a
   parallel Shopify-only path.
2. **Comma scope separator.** `buildOAuthAuthorizeURL` currently space-joins
   `DefaultScopes`; Shopify needs `,`. Add a reviewed `scope_separator` enum
   (`" "` default | `","`), reused by any comma-scope provider.
3. **Expiring-offline exchange param `expiring=1` + rotating-refresh write-back.**
   The exchange must add `expiring=1`; refresh rotates BOTH tokens → strict A3
   write-back (already the standard path for rotation providers). If a
   bundle-level "extra exchange param" field exists (Instagram/TikTok-class
   growths), reuse it; otherwise a small `exchange_params` map on the endpoints.

Client id/secret land in integration-service config (`config/` + `deploy/`
Secret together) as lane-1 per-provider appends; absent → `configured:false`
(safe hidden); partial → startup failure (id+secret land atomically).

**UI icon**: `ui/helio-app/src/integrations/icons/shopify.svg` +
`providerIcons.ts` (manual, never generated). **AI-facing doc**: a `shopify`
sub-doc under `agents/plugins/heliox/skills/tool/` (verbs, GraphQL passthrough,
`userErrors`, shop-domain connect note), riding the batch plugin bump.

---

## 6. Test plan → the five layers

| Layer | Shopify specifics | Needs external creds? |
|---|---|---|
| **L1** anycli unit | httptest fake of `graphql.json`: assert endpoint, `X-Shopify-Access-Token`, per-verb query/variables, `userErrors`→exit-1, 401→exit-1, `--json` envelope, cursor flags. | No |
| **L2** harness real-API | `ANYCLI_CRED_ACCESS_TOKEN=shpat_… ANYCLI_CRED_STORE=<shop>.myshopify.com anycli shopify -- shop info` against a **development store**'s custom-app admin token; then `product list`, a `product update` on a throwaway product. Proves the GraphQL shape + header are right. | **Yes** — a dev store + Admin API token (custom app on a dev store yields `shpat_…` directly; no OAuth needed for L2). |
| **L3** provider-gen + suites | `provider-gen --check` (five projections committed together at batch end); both repos' unit suites; the new capability's tests (instance templating, comma scopes, `expiring=1`). | No |
| **L4** singleton + seed | Seed `access_token`(+`refresh_token`, short `expires_at`) and `account_key=<shop>.myshopify.com` via `POST /internal/test-only/connections/seed` (Shopify is a user-token OAuth provider → seedable). Short expiry forces the token gateway's rotate-and-write-back path (A3) on the next `heliox tool shopify -- shop info`. | **Yes** — a real dev-store token for the seed to hit the live API. |
| **L5** full connect | Once, hidden, before the visible flip: `heliox tool shopify auth` → enter shop domain → real merchant consent on a **dev store** → `oauth_connected` event → unseeded `heliox tool shopify -- shop info`. Human-in-the-loop (oauth lane). | **Yes** — a registered Partner dev-mode app (client id/secret in local `config/cloud.yaml`) + a dev store to consent on. |

**Rollout**: land hidden; L1–L4 green; L5 once on a dev store. The **visible
flip additionally waits on `oauth_review` clearance** — App Store submission and
Protected Customer Data approval for production-store access — decoupled from
dev per hidden-first. Flip `visible: true` + regenerate as the single go-live
change.

**Lane-1 note for the batch lead**: Shopify dev-mode app creation (Partner
Dashboard, free) gates L4/L5 but not the merge; Protected-Customer-Data +
App-Store review gate only the visible flip. Flag at stage 1: the instance-
scoped-OAuth capability dependency (§5.1) — confirm whether the Zendesk/
Salesforce instance capability is on the batch base before dev starts.
