# Gumroad — per-tool design (`tool/gumroad`)

Scratch design for the Gumroad provider, produced per the `helio-tool-provider`
pipeline and the 298-integrations master rollout plan (row 174). Committed on
branch `tool/gumroad`; the batch lead strips it at batch end.

- **Catalog row:** 174 — Gumroad, Payments & Commerce, **Wave 3**.
- **Naming (three axes):** ① CLI command word `gumroad` · ② anycli id `gumroad`
  · ③ provider key `gumroad`. All three identical → **no `toolToProvider`
  divergence entry**, no `tool.group`. `gumroad` is a standalone brand
  (gmail/youtube precedent: key == id, no family prefix).
- **Auth lane:** `oauth_light` (audit row 176, confidence **high**) — **confirmed
  against official docs**. Multi-tenant authorization-code OAuth 2.0, self-serve
  app registration in Gumroad Settings → Advanced, no review/partner gate.

## 1. Provider verification against official docs

Independent re-verification of the audit verdict against Gumroad's own
documentation (`gumroad.com/api`, the antiwork/Gumroad docs mirror, and the
Doorkeeper-backed OAuth provider config). Findings that **diverge** from the
audit/catalog are called out; none change the `oauth_light` lane.

| Fact | Verified value | Source | Divergence from audit? |
|---|---|---|---|
| OAuth model | OAuth 2.0 authorization-code (Rails Doorkeeper provider), self-serve app creation, no review | gumroad.com/api; audit row 176 | No — confirms `oauth_light` |
| Authorize URL | `https://gumroad.com/oauth/authorize` | official | — |
| Token URL | `https://api.gumroad.com/oauth/token` (host **differs** from authorize) | official | New detail |
| Token exchange | `POST` form-urlencoded: `client_id`, `client_secret`, `code`, `grant_type=authorization_code`, `redirect_uri` → `form_secret` | official; Whyounes ref client | — |
| Access-token expiry | **Non-expiring** (Doorkeeper `access_token_expires_in nil`) | Doorkeeper config | Refines audit |
| Refresh token | **Issued** (`use_refresh_token` enabled), but unused because access token never expires | Doorkeeper config | Audit implied none; net effect = `refresh_lease: none` |
| Auth-code TTL | 10 minutes | Doorkeeper config | — |
| PKCE | Supported, not required (confidential client) → `pkce: none` | Doorkeeper | — |
| Scopes | Exactly **six** third-party scopes: `view_profile`, `edit_products`, `view_sales`, `view_payouts`, `mark_sales_as_shipped`, `edit_sales`. **No `refund_sales`** — refunding and receipt-resend are granted by `edit_sales`. **No `view_public`** in the current documented set (it and internal-only scopes such as `revenue_share`/`ifttt`/`mobile_api` appear only in the auto-generated antiwork mintlify wiki, not gumroad.com/api). `view_profile` is the minimal read scope. | gumroad.com/api + Help Center art. 280 (authoritative) | Corrects audit + earlier draft: dropped non-existent `refund_sales`, dropped stale `view_public` default, added `edit_sales` |
| API base | `https://api.gumroad.com/v2/`, `Authorization: Bearer <token>` | official | — |
| Identity endpoint | `GET /v2/user` → `{"success":true,"user":{"user_id","name","email","url",…}}` | official | — |

**Net:** golden-path multi-tenant OAuth. `oauth_light` stands. The only
corrections to record are cosmetic (asymmetric authorize/token hosts; refresh
tokens exist but are inert because tokens don't expire) — logged here per the
"record the divergence" instruction, no lane change.

## 2. API surface wrapped, and why

An AI teammate on a Gumroad creator account does **store operations**: check
what's selling, inspect/adjust products, look at buyers/subscribers, run
discount codes, verify software licenses. The wrapped surface is the Gumroad
**API v2** REST resources that map to those jobs — all Bearer-auth against
`https://api.gumroad.com/v2/`:

| Resource | Endpoints (method path) | Why (teammate job) |
|---|---|---|
| User | `GET /user` | Whoami / identity; sanity-check the connection |
| Products | `GET /products`, `GET /products/:id`, `PUT /products/:id/enable`, `PUT /products/:id/disable`, `DELETE /products/:id` | List catalog, inspect a product, toggle availability, retire |
| Sales | `GET /sales` (filters: `after`, `before`, `email`, `product_id`, `page_key`), `GET /sales/:id`, `PUT /sales/:id/mark_as_shipped`, `PUT /sales/:id/refund` | Revenue reporting, fulfil physical orders, issue refunds |
| Subscribers | `GET /products/:product_id/subscribers`, `GET /subscribers/:id` | Membership/subscription roster and status |
| Offer codes | `GET /products/:product_id/offer_codes`, `GET /products/:product_id/offer_codes/:id`, `POST/PUT/DELETE …/offer_codes[/:id]` | Create/adjust discount campaigns |
| Variant categories | `GET/POST/PUT/DELETE /products/:product_id/variant_categories[/:id]` (+ nested `/variants`) | Product tiers/options (secondary) |
| Custom fields | `GET/POST/PUT/DELETE /products/:product_id/custom_fields[/:name]` | Checkout form fields (secondary) |
| Licenses | `POST /licenses/verify` (no auth), `PUT /licenses/enable`, `PUT /licenses/disable`, `PUT /licenses/decrement_uses_count` | Software-license validation/seat management |
| Resource subscriptions | `GET /resource_subscriptions?resource_name=…`, `PUT /resource_subscriptions`, `DELETE /resource_subscriptions/:id` | Webhook wiring (advanced) |

**In scope (v1 of the tool):** user, products, sales, subscribers, offer codes,
licenses. **Deferred / thin coverage:** variant categories, custom fields,
resource subscriptions — lower-frequency for an assistant and safe to add later
without a schema change. **Excluded:** none needed; there is no separate
analytics API (the `view_sales` sales list covers revenue reporting).

**Verb → requested scope (least-privilege check — every shipped verb maps to a
requested scope, and every requested scope is exercised by ≥1 verb):**

| Verb(s) | Scope |
|---|---|
| `user get` | `view_profile` |
| `product get`/`list` (read) | `view_profile` |
| `product enable`/`disable`/`delete`, `offer-code create`/`update`/`delete`, `license enable`/`disable` | `edit_products` |
| `sale list`/`get`, `subscriber list`/`get`, `offer-code list`/`get` (read) | `view_sales` |
| `sale mark-shipped` | `mark_sales_as_shipped` |
| `sale refund` (and receipt-resend, if added) | `edit_sales` |

`view_payouts` is **not** requested: no payouts verb is wrapped in v1 (§3), so
requesting it would show payout-read on the consent screen for access the
assistant never exercises. If a payouts verb (`GET /v2/... `) is added later,
add `view_payouts` back in the same change.

## 3. anycli definition

### Tool form — `service` (stage-1 rubric)

`service` type. No official Gumroad CLI binary exists to wrap, so the `cli`
branch (github→gh precedent) does not apply. Implemented in
`internal/tools/gumroad/` against the v2 HTTP API, following the `notion`
reference shape (cobra tree grouped by resource; `BaseURL`/`HC`/`Out`/`Err`
struct for httptest; documented exit-code contract 0/1/2; `--json` structured
error envelope). Go package name `gumroad` (no dash/digit normalization needed).

### `definitions/tools/gumroad.json`

```json
{
  "name": "gumroad",
  "type": "service",
  "description": "Gumroad creator commerce (products, sales, subscribers, offer codes, licenses) via API v2 (OAuth token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "GUMROAD_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Registered as `RegisterService("gumroad", &gumroad.Service{})` in
`internal/tools/register.go` (batch-serialized shared file).

### Subcommand tree (verbs)

```
gumroad user get
gumroad product list [--page N]
gumroad product get --id <id>
gumroad product enable  --id <id>
gumroad product disable --id <id>
gumroad product delete  --id <id>
gumroad sale list [--after YYYY-MM-DD] [--before YYYY-MM-DD] [--email <e>] [--product-id <id>] [--page-key <k>]
gumroad sale get   --id <id>
gumroad sale mark-shipped --id <id> [--tracking-url <url>]
gumroad sale refund --id <id> [--amount-cents <n>]
gumroad subscriber list --product-id <id>
gumroad subscriber get  --id <id>
gumroad offer-code list   --product-id <id>
gumroad offer-code get    --product-id <id> --id <id>
gumroad offer-code create --product-id <id> --name <code> --amount-off <n> [--percent] [--max-purchase-count <n>]
gumroad offer-code update --product-id <id> --id <id> [--max-purchase-count <n>]
gumroad offer-code delete --product-id <id> --id <id>
gumroad license verify  --product-id <id> --license-key <k> [--increment-uses-count]
gumroad license enable  --product-id <id> --license-key <k>
gumroad license disable --product-id <id> --license-key <k>
```

Auth is injected once via `GUMROAD_ACCESS_TOKEN`; `license verify` also works
unauthenticated per Gumroad, but the tool always sends the Bearer token it has.

### JSON output shape

Gumroad wraps every response as `{"success":true,"<resource>":[...]}` or
`{"success":true,"<resource>":{…}}`. The service **unwraps** to the
provider-neutral payload agents consume:

- List verbs → a top-level JSON array of the resource objects (pass-through of
  Gumroad's per-object fields; no re-mapping).
- Single-object verbs → the resource object.
- Mutations (enable/disable/refund/mark-shipped/offer-code CRUD) → the returned
  object, or `{"success":true,"id":"…"}` when Gumroad returns only a flag.
- Errors: Gumroad returns `{"success":false,"message":"…"}` (often HTTP 200 or
  401/402/404). The service maps non-success to exit 1 and, under `--json`,
  emits the standard anycli error envelope `{"error":{"code","message"}}`
  carrying Gumroad's `message`. Usage/parse errors → exit 2. (Note the
  Slack-style "200 with success:false" dialect: status code alone is not the
  success signal — the service must read the `success` field, handled inside the
  service impl, **not** an integration-service adapter.)

## 4. Credentials & auth flow

**Credential kind:** single OAuth bearer `access_token` (non-expiring). Vault
stores it via the standard user-token path; anycli receives only
`{access_token}` and injects `GUMROAD_ACCESS_TOKEN`.

**Registration model (lane 1):** the Gumroad dev app is created self-serve in
**Settings → Advanced → Applications**: set the Helio redirect URI, receive
**Application ID** (`client_id`) + **Application Secret** (`client_secret`). No
review, no partner program (audit high-confidence, re-confirmed). These land in
integration-service config (`config/` + `deploy/` Helm Secret together, per
Config Sync) as `oauth.client_id` / `oauth.client_secret`; distributed to
batch agents as **uncommitted local `config/cloud.yaml`** entries for L4/L5.

**Authorize-code flow (standard_oauth golden path, zero adapter):**

1. `heliox tool gumroad auth` → integration-service builds
   `https://gumroad.com/oauth/authorize?client_id=…&redirect_uri=…&scope=<space-joined>&response_type=code`,
   where `<space-joined>` is the five-scope set `view_profile view_sales
   edit_products mark_sales_as_shipped edit_sales` (§5). No `refund_sales`
   (non-existent — `edit_sales` grants refunds) and no `view_payouts` (no
   payouts verb).
2. User consents on Gumroad → callback with `code` (valid 10 min).
3. Token exchange: `POST https://api.gumroad.com/oauth/token` form-urlencoded
   (`client_id`, `client_secret`, `code`, `grant_type=authorization_code`,
   `redirect_uri`) → `{access_token, refresh_token, scope, token_type}`.
   → `token_exchange_style: form_secret`, `pkce: none`.
4. Identity: `GET https://api.gumroad.com/v2/user` with the new bearer →
   `stable_key: /user/user_id`, labels from `/user/name`,`/user/email`.
5. Access token is non-expiring → `refresh_lease: none`, `single_active_token:
   false`. The issued `refresh_token` is stored but never exercised (no expiry
   to refresh against); the resolver caches under `defaultTokenTTL`.

**Pitfall recorded:** authorize host (`gumroad.com`) ≠ token host
(`api.gumroad.com`) — do not collapse them.

## 5. Helio provider bundle plan

`integrations/providers/gumroad/provider.yaml`, **hidden-first**
(`presentation.visible: false`). This is a **pure `standard_oauth` golden-path
bundle — no integration-service capability growth required**: `form_secret`
exchange + `userinfo` identity + `refresh_lease: none` are all already exercised
by the shipped `google_*` bundles, so zero provider-specific Go.

```yaml
schema: helio.provider/v1
key: gumroad
go_name: Gumroad

presentation:
  name: Gumroad
  description_key: gumroad
  consent_domain: gumroad.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://gumroad.com/oauth/authorize
    token_url: https://api.gumroad.com/oauth/token
    token_exchange_style: form_secret
    pkce: none
    # Exactly the verbs' union of real Gumroad scopes (§2 map). refund is
    # granted by edit_sales (no refund_sales scope exists); view_payouts is
    # omitted because no payouts verb is wrapped.
    scopes: [view_profile, view_sales, edit_products,
             mark_sales_as_shipped, edit_sales]
    display_scopes: [view_profile, view_sales, edit_products,
                     mark_sales_as_shipped, edit_sales]
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://api.gumroad.com/v2/user
  stable_key: /user/user_id
  label_candidates: [/user/name, /user/email, /user/user_id]

connection:
  mode: isolated
  disconnect_mode: local_only     # Doorkeeper /oauth/revoke exists; local_only is the safe default, revoke is a later enhancement
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
  name: gumroad
  kind: oauth
```

**Naming axes ①/②/③:** command `gumroad`, anycli id `gumroad`, key `gumroad` —
identical, so **no** `toolToProvider` entry and **no** `tool.group`. Confirmed
against `helio-cli/internal/toolcred/resolver.go` (`ProviderFor("gumroad")`
returns `gumroad` by identity).

**Other batch-end shared-surface touches:** UI icon
`ui/helio-app/src/integrations/icons/gumroad.svg` + `providerIcons.ts` append;
AI-facing sub-doc under `agents/plugins/heliox/skills/tool/`; the five
provider-gen projections regenerated by the batch lead.

## 6. Test plan — five layers

| Layer | Gumroad-specific plan | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: httptest fake for `api.gumroad.com/v2/*`. Assert Bearer header injection, request shape per verb, `success:true` unwrap → array/object, and the `success:false` / non-2xx → exit-1 + `--json` error envelope path (the 200-with-`success:false` dialect). Table-drive products/sales/subscribers/offer-codes/licenses. | No (fakes) |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<personal token from Settings→Advanced> anycli gumroad -- user get` then `product list`, `sale list`, `license verify`. **Mandatory before pin bump** — proves field names/injection/request shape against the live API. | **Yes** — a real Gumroad account access token (personal token suffices; no app needed for L2) |
| **L3** gen + suites | `provider-gen` + `provider-gen --check` locally on-branch; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with a local `go.mod replace` → anycli branch. Branch is **expected** to fail `--check` in CI until the batch-end regen. | No |
| **L4** singleton + seed | `make run-singleton` (dev), `POST /internal/test-only/connections/seed` provider `gumroad` with a **real** access token (seed `access_token` only — non-expiring, omit `refresh_token`/`expires_at`, Slack-token class), then `heliox tool gumroad -- user get` / `product list` reaches the live API through the token gateway. Requires lane-1 dev app **only** transitively (the seeded token can be a personal token); real identities per the seed doc. | **Yes** — real access token; real seeded org/assistant identities |
| **L5** full connect | Hidden tool. `heliox tool gumroad auth` → real Gumroad consent on the dev app → `oauth_connected` event → unseeded `heliox tool gumroad -- user get`. Human-in-the-loop (oauth L5). Gates the visible flip. | **Yes** — lane-1 dev app (client id/secret in config) **and** a real Gumroad account to consent |

**Credential-gated layers:** L2, L4, L5 (test-account pool + lane-1 dev app).
L1/L3 are hermetic. Per the rollout plan, `oauth_light` L5 is human-in-the-loop.

**Rollout:** land hidden; bump/validate anycli pin (L1–L3); run L4/L5 while
hidden; then flip `presentation.visible: true` + regenerate as the single
go-live change (SKILL stage 10).
