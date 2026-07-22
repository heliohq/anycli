# Stripe — per-tool design (`heliox tool stripe`)

Scratch design for the `tool/stripe` batch branch. Batch lead strips this at
batch-end. Catalog row 35: Product **Stripe**, anycli id `stripe`, provider key
`stripe`, auth lane **oauth_review**, wave 1, category Payments & Commerce.

Everything below was verified against Stripe's official docs (July 2026) and the
actual repo code, not inherited from the catalog. Divergences from the audit are
called out explicitly in §3.

---

## 1. What an AI teammate does with Stripe — and the API surface that serves it

A Helio teammate connected to a customer's Stripe account is a **finance /
revenue-ops / support colleague**, not a checkout integration. The high-value,
low-risk actions are read-mostly reporting plus a few well-scoped support
mutations (issue a refund, draft/send an invoice, cancel a subscription). It
does **not** need PaymentIntent confirmation, card tokenization, or the webhook
event bus — those are edge/customer-facing concerns outside an assistant's job.

That intent maps onto the **Stripe REST API** (`https://api.stripe.com/v1/…`,
`application/x-www-form-urlencoded` request bodies, JSON responses). The tool
wraps exactly the resources the colleague reaches for:

| Resource group | Endpoints wrapped | Why |
|---|---|---|
| `balance` | `GET /v1/balance`, `GET /v1/balance_transactions` | "How much is in the account / recent settlement activity." |
| `charge` | `GET /v1/charges`, `GET /v1/charges/:id` | Inspect payments. |
| `payment-intent` | `GET /v1/payment_intents`, `GET /v1/payment_intents/:id` | Modern payment records. Read-only (no confirm/capture — that's customer-side). |
| `customer` | `GET/POST /v1/customers`, `GET/POST /v1/customers/:id`, `GET /v1/customers/search` | Look up + maintain customer records. |
| `invoice` | `GET /v1/invoices`, `GET /v1/invoices/:id`, `POST /v1/invoices`, `POST /v1/invoices/:id/finalize`, `POST /v1/invoices/:id/send` | Draft/send invoices — a real assistant task. |
| `subscription` | `GET /v1/subscriptions`, `GET /v1/subscriptions/:id`, `DELETE /v1/subscriptions/:id` | Report + cancel. |
| `refund` | `GET /v1/refunds`, `GET /v1/refunds/:id`, `POST /v1/refunds` | Issue refunds (top support action). |
| `payout` | `GET /v1/payouts`, `GET /v1/payouts/:id` | Settlement reporting. |
| `product` / `price` | `GET /v1/products`, `GET /v1/prices` (+ `/:id`) | Catalog lookups for invoicing. |
| `dispute` | `GET /v1/disputes`, `GET /v1/disputes/:id` | Chargeback triage. |
| `event` | `GET /v1/events`, `GET /v1/events/:id` | Audit trail / "what changed". |
| top-level `search` | `GET /v1/{customers,charges,invoices,subscriptions,prices}/search?query=` | Stripe Search Query Language passthrough. |
| top-level `get` | arbitrary `GET /v1/<path>` passthrough | Long-tail reads without a per-resource verb. |

Cross-cutting request conventions (verified against Stripe docs):
- **Pagination**: cursor-based — `limit` (1–100), `starting_after`/`ending_before`
  (object ids). List responses carry `{ "object": "list", "data": [...],
  "has_more": bool, "url": ... }`. Surfaced as `--limit` / `--starting-after` /
  `--ending-before` on every list verb; `--json` returns the envelope verbatim.
- **API version pin**: the tool sends a fixed `Stripe-Version:` header (a pinned
  dated version, e.g. `2025-xx-xx`) so response shapes don't drift under us. This
  is a constant in the service, not a credential.
- **Idempotency**: create/refund verbs accept `--idempotency-key`, forwarded as
  the `Idempotency-Key` header (Stripe's documented safe-retry mechanism).
- **Errors**: Stripe returns `{ "error": { "type", "code", "message",
  "param" } }` with a 4xx/402. The service maps this to the typed `apiError`
  exit-code contract below.

### Why NOT the `stripe` CLI (cli-type rejected per stage-1 rubric)

An official `stripe` binary exists and can run `stripe get /v1/charges --api-key
sk_…` non-interactively. It is still rejected: (a) it must be provisioned into
the runtime image (extra image weight for a thin REST wrapper); (b) its primary
mode is interactive `stripe login` + `stripe listen` websocket tailing, not
agent JSON I/O; (c) a `service`-type gives us provider-neutral JSON shaping,
pagination flags, and the typed error envelope with zero binary dependency. So
Stripe is a **service-type** tool (matching 21/23 shipped definitions).

---

## 2. anycli definition (data plane)

- **Type**: `service`. Package `internal/tools/stripe/` (id has no dashes → Go
  package `stripe`), registered `RegisterService("stripe", &stripe.Service{})`
  in `internal/tools/register.go`. Definition file `definitions/tools/stripe.json`.
- **Shape**: copy the `internal/tools/notion/` reference — a cobra tree grouped
  by resource (`balance`, `charge`, `payment-intent`, `customer`, `invoice`,
  `subscription`, `refund`, `payout`, `product`, `price`, `dispute`, `event`)
  plus top-level `search`/`get`, a `BaseURL`/`HC`/`Out`/`Err` struct so tests can
  point at an `httptest` server, and the documented exit-code contract: **0**
  success, **1** runtime/API failure (typed `apiError` from Stripe's error
  envelope), **2** usage/parse error. `--json` emits a structured error envelope.
- **Auth injection** — the token gateway projects the connection's bearer token
  into `token.access_token`; the bundle `credential.fields.access_token` maps it,
  and the anycli definition injects it as an env var the service reads and sends
  as `Authorization: Bearer <token>`:

  ```json
  {
    "name": "stripe",
    "type": "service",
    "description": "Stripe payments, invoicing, and customer data (OAuth)",
    "auth": {
      "credentials": [
        { "source": {"field": "access_token"},
          "inject": {"type": "env", "env_var": "STRIPE_ACCESS_TOKEN"} }
      ]
    }
  }
  ```

  Stripe accepts the OAuth access token via `Authorization: Bearer <token>`
  (equivalent to `-u <token>:` Basic) against `api.stripe.com/v1` — confirmed by
  the token-verification example in the Stripe Apps OAuth docs
  (`curl https://api.stripe.com/v1/customers -u "<ACCESS_TOKEN>:"`).

- **L1 TDD**: `httptest.Server` fakes for each resource assert request path,
  method, the injected `Authorization: Bearer` header, the `Stripe-Version`
  pin, pagination/idempotency params, and both text and `--json` error
  rendering. No live API in unit tests.

---

## 3. Credentials & the exact auth flow (the load-bearing decision)

**Two different Stripe OAuth flows exist. We use Stripe Apps OAuth. This is the
audit's cited evidence and the only one that yields a first-class, refreshable,
per-account bearer token that fits Helio's token gateway.**

### 3.1 Chosen flow — Stripe Apps OAuth

Verified at `https://docs.stripe.com/stripe-apps/api-authentication/oauth`:

| Piece | Value |
|---|---|
| Authorize URL | `https://marketplace.stripe.com/oauth/v2/authorize` (params: `client_id`, `redirect_uri`, `state`) |
| Token URL (exchange **and** refresh) | `https://api.stripe.com/v1/oauth/token` |
| Client auth at token endpoint | HTTP Basic, **username = app developer secret key** (`sk_live_***` / `sk_test_***`), **empty password**. `client_id` is NOT sent at the token endpoint — it appears only in the authorize URL. |
| Exchange body | exactly `grant_type=authorization_code` + `code=ac_***` (one-time, 5-min TTL). **No `redirect_uri`** — the documented `curl` sends only these two params (see §7). |
| Refresh body | `grant_type=refresh_token`, `refresh_token=…` |
| access_token | bearer, **1-hour expiry** |
| refresh_token | **1-year expiry, rotated on every exchange** (each exchange issues a fresh 1-year token and expires the prior one) |
| Token-response expiry field | **NONE.** The exchange and refresh responses carry only `access_token`, `refresh_token`, `livemode`, `scope`, `stripe_publishable_key`, `stripe_user_id`/`account_id`, `token_type` — there is **no `expires_in`**. The 1-hour lifetime is documented in prose only, never returned in the payload. This is the load-bearing fact behind the §4 assumed-TTL growth. |
| Scope | none in the authorize request — permissions are declared in the app manifest; the token response returns fixed `"scope": "stripe_apps"` |
| Identity | token response carries `stripe_user_id` = `acct_***` (the connected account) + `stripe_publishable_key`, `livemode`. (The **refresh** response names the same `acct_***` value `account_id` instead of `stripe_user_id` — identity is captured at connect from `stripe_user_id`, so this rename does not affect us.) |

**Consequence (drives §4).** Because the token response has no `expires_in`, the generic exchanger's `tokenResponse.expiry()` returns `nil` (it derives expiry solely from `ExpiresIn`), the persisted `tokenData.Expiry` is `nil`, and `needsRefresh()` returns `false` for a nil expiry (`token_gateway.go`: "non-expiring (e.g. bot token)"). The refresh path would then **never fire**, and every connection would 401 ~1 hour after connect with no recovery but a full reconnect. Stripe access tokens are not bot tokens — they expire — so the tool must **synthesize** a 1-hour expiry at exchange time. That is a net-new capability (§4b); no assumed-TTL/synthetic-expiry capability exists on this worktree base.

### 3.2 Why not Stripe Connect OAuth

`https://docs.stripe.com/connect/oauth-reference` (authorize
`connect.stripe.com/oauth/authorize`, token `connect.stripe.com/oauth/token`) is
the classic platform flow. Its `access_token`, `refresh_token`, and
`stripe_publishable_key` fields are **officially deprecated** — Stripe now steers
platforms to the `Stripe-Account` header + the platform's own secret key. That
model is a different credential shape (platform secret + a stored
`stripe_user_id`, not a per-connection bearer token) and does not fit anycli's
`Authorization: Bearer <token>` injection or the token gateway's
refresh-and-write-back. Stripe Apps OAuth is the forward-supported per-account
bearer path, so we use it.

### 3.3 Why not the restricted-API-key fallback

The audit note mentions "customers pasting a restricted API key" as the
review-free fallback. That is the **api_key** lane (a separate `manual_api_token`
bundle). We deliberately follow the plan's **oauth_review** lane: a proper OAuth
connection with per-account tokens, refresh, and disconnect-revoke — not a pasted
long-lived secret. Recorded here as considered-and-rejected; no catalog change.

### 3.4 Lane confirmation — oauth_review is correct

Official docs: *"The public OAuth install links don't work until the app is
published"*, and publishing requires Stripe App Marketplace **review**
(`distribution_type: public`, manifest `stripe_api_access_type: oauth`). This is
the review gate → **oauth_review**. Crucially, dev/test-mode apps work
pre-review (the review team installs via the link too), so **dev-mode app
creation gates L4/L5 but review clearance gates only the visible flip** —
exactly the plan's hidden-first decoupling. Lane classification: **unchanged
from the audit** (oauth_review, high confidence). No DESIGN divergence to the
catalog lane; the refinements are the token-endpoint auth mechanics (§4a) and
the synthesized access-token expiry (§4b), neither of which the audit
enumerated.

### 3.5 Config fields (integration-service, per environment)

`auth.required_config_fields: [oauth.client_id, oauth.client_secret]`, supplied
via integration-service config (`config/` + the `deploy/` Helm Secret together —
Config Sync). `client_id` = the OAuth install-link client id; `client_secret` =
the app developer **secret key** (`sk_live_***`). Never in the bundle. A fully
unset provider renders `configured:false` (Connect disabled, safe hidden); a
**partial** config fails startup — so id + secret land in the same change, before
this provider's L5.

---

## 4. Helio integration-service capability growth (two reviewed capabilities)

`standard_oauth` almost covers Stripe — declarative identity, refresh
write-back, declarative/no-op revoke all fit. **Two orthogonal axes diverge**,
and neither is expressible on this worktree base. Both are closed, reviewed
capabilities — **not** an unbounded adapter (the skill's guidance: "first check
whether the generic capability set should grow one more reviewed enum value
instead"). A narrow `adapter_stripe.go` + dedicated `runtime_strategy` was
considered and rejected: nothing about Stripe's response shape, identity, or
lifecycle is provider-specific — only (a) the token-endpoint client-auth
encoding and (b) the absence of a returned expiry are.

### 4a. Token-endpoint client auth — `token_exchange_style: form_secret_basic`

The token endpoint authenticates the client with the **secret key as the HTTP
Basic username and an empty password, with `client_id` omitted** from both the
Basic header and the body. The three existing styles don't express this:

- `form_secret` → sends `client_id` + `client_secret` in the body.
- `form_basic` / `json_basic` → `SetBasicAuth(client_id, client_secret)` — i.e.
  `Basic base64(client_id:client_secret)`. Stripe needs `Basic base64(sk_key:)`.

**Proposed value: `token_exchange_style: form_secret_basic`** — a form-encoded
body of **exactly `grant_type=authorization_code` + `code`** (no `redirect_uri`,
no `client_id`, no `client_secret` in the body — see §7) + HTTP Basic where
username = the configured `client_secret` (the `sk` key), password empty.
Threaded through exactly these five points:

1. `model/catalog.go`: add `TokenExchangeFormSecretBasic TokenExchangeStyle =
   "form_secret_basic"`; make `UsesHTTPBasicClientAuth()` return true for it.
2. `service/oauth_exchange.go` `buildTokenRequest`: new **dedicated** case (not a
   fall-through into the shared form path — that path unconditionally appends
   `redirect_uri` and, for non-`form_basic` styles, `client_id`+`client_secret`
   in the body, none of which Stripe's documented request carries). The case
   builds a minimal `url.Values{grant_type, code}` body and calls
   `req.SetBasicAuth(exchange.ClientSecret, "")` (secret as username, empty
   password); add it to `validateTokenExchangeStyle`.
3. `service/token_refresh.go` `requestOAuthRefresh`: when the style is
   `form_secret_basic`, build `oauth2.Config{ClientID: creds.ClientSecret,
   ClientSecret: ""}` with `AuthStyleInHeader`. `golang.org/x/oauth2` then emits
   `Authorization: Basic base64(url.QueryEscape(sk):)` — byte-for-byte Stripe's
   `-u sk_live_***:`. (This is why a plain config-level swap won't do: the
   authorize URL still needs the real `client_id`, so the swap must live in the
   exchange/refresh mapping, not in config.)
4. `cmd/provider-gen/validate.go`: add `form_secret_basic` to the
   `oneOf(o.TokenExchangeStyle, …)` allow-list.
5. `cmd/provider-gen/render_symbols.go`: map `"form_secret_basic" ->
   "TokenExchangeFormSecretBasic"`.

### 4b. Assumed access-token TTL — `oauth.access_token_ttl_seconds`

Stripe's token response carries **no `expires_in`** (§3.1), so the generic
exchanger persists `tokenData.Expiry = nil`, `needsRefresh()` reads it as
non-expiring, and the entire §4a/refresh-lease/A3 machinery becomes **dead
code** — the connection silently 401s ~1 h after connect. The fix is to
**synthesize** the documented 1-hour expiry at exchange time: an *assumed TTL*,
following the salesforce `assumed_ttl` precedent (salesforce likewise returns no
expiry) and the square `expires_at`-capture precedent (square returns one, so it
captures rather than assumes). Neither precedent's bundle is present on this
worktree base, so this is genuinely net-new here.

**Proposed field: `oauth.access_token_ttl_seconds: 3600`** on the bundle,
projected to `OAuthEndpoints.AssumedAccessTokenTTL time.Duration`. When set and
the provider returns no `expires_in`, the exchange-side expiry computation
synthesizes `now + AssumedAccessTokenTTL`; when the provider *does* return
`expires_in`, the returned value always wins (so the field is a fallback, never
an override). Threaded through exactly these four points:

1. `model/catalog.go`: add `AssumedAccessTokenTTL time.Duration` to
   `OAuthEndpoints` (zero = today's behavior, expiry solely from `expires_in`).
2. `service/oauth_exchange.go`: give `tokenResponse.expiry()` an assumed-TTL
   fallback — e.g. `expiryWithFallback(assumed time.Duration)` returning
   `now+assumed` when `ExpiresIn <= 0 && assumed > 0`, else the existing
   `ExpiresIn`-derived value (and still `nil` when both are absent). The two
   call sites in `service/oauth_credentials.go`
   (`writeIndividualCredential`, `writeAssistantCredential`) pass
   `def.OAuth.AssumedAccessTokenTTL`.
3. `cmd/provider-gen/validate.go`: validate `oauth.access_token_ttl_seconds` as
   an optional non-negative integer.
4. `cmd/provider-gen` catalog renderer: project it into the generated
   `OAuthEndpoints` literal as `AssumedAccessTokenTTL: <n> * time.Second`.

Unit tests (§6): a Stripe-shaped token response (**no `expires_in`**) through
the real exchange persists a **non-nil** `Expiry` ≈ `now + 1 h`; `form_secret_basic`
exchange builds `Basic(sk:)` with no `client_id`/`redirect_uri` anywhere;
refresh maps to the x/oauth2 header form; `provider-gen --check` accepts the
bundle; generator golden tests for the new symbol and the rendered TTL.

> Batch note: concurrent branches are adding adjacent capabilities on the same
> base — square (row 40) adds a *different* exchange style (`json_secret`) plus
> `expires_at`-capture, and salesforce adds the `assumed_ttl` field this section
> mirrors. The axes are distinct and all net-new here; the batch lead reconciles
> enum/field ordering **and the exact assumed-TTL field name** (`assumed_ttl`
> vs `access_token_ttl_seconds`) against the salesforce landing at the batch-end
> regen so the generated allow-lists and catalog literal don't churn. This is a
> batch-lead concern, not a per-tool blocker.

Everything else is stock `standard_oauth`:
- `refresh_lease: credential` — Stripe rotates the refresh token every exchange,
  so two concurrent refreshes would burn each other's rolled token. A
  per-credential lease serializes them; the existing A3 strict write-back then
  persists the rotated refresh token before returning (never returns an
  unpersisted token). `OAuthLeaseCredential` already exists — no new lease code.
- `disconnect_mode: local_only` — Stripe Apps OAuth exposes no standard
  per-token revoke endpoint for the app→account grant (deauthorize is a Connect
  concept keyed on `client_id`+`stripe_user_id`); local delete + vault credential
  delete is correct and the declarative no-op revoker covers it. (Revisit at L2
  if a first-party Apps deauthorize is confirmed.)

---

## 5. Helio provider bundle plan (`integrations/providers/stripe/provider.yaml`)

Naming axes — **no ②↔③ divergence**, so **no `toolToProvider` entry** and no
grouped-command word:

| Axis | Value |
|---|---|
| ① CLI command word | `stripe` (flat) → `heliox tool stripe` |
| ② anycli tool id | `stripe` |
| ③ provider catalog key / bundle dir | `stripe` |

`ProviderFor("stripe")` falls through to identity (`stripe`), and `ToolFor`
likewise — nothing to register in `resolver.go`.

Bundle (hidden-first — `visible: false`):

```yaml
schema: helio.provider/v1
key: stripe
go_name: Stripe

presentation:
  name: Stripe
  description_key: stripe
  consent_domain: stripe.com
  visible: false          # hidden-first; flip true only after L5 + review clearance
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual        # per-account user-authorized token; connection belongs to the assistant.
                           # NOT `assistant` — that triggers the app-bot org-admin gate in oauth_start.go.
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://marketplace.stripe.com/oauth/v2/authorize
    token_url: https://api.stripe.com/v1/oauth/token
    token_exchange_style: form_secret_basic   # NEW (see §4a)
    access_token_ttl_seconds: 3600            # NEW (see §4b): Stripe returns no
                                              # expires_in; synthesize the
                                              # documented 1-hour expiry so
                                              # needsRefresh() fires.
    pkce: none
    # no scopes: Stripe Apps permissions are declared in the app manifest, not
    # as authorize-request scope params. Leaving display_scopes empty keeps the
    # authorize URL builder from appending a `scope` param.
    single_active_token: false
    refresh_lease: credential

identity:
  source: token_response
  stable_key: /stripe_user_id            # acct_*** — stable per connected account
  label_candidates: [/stripe_user_id]

connection:
  mode: isolated
  disconnect_mode: local_only
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
  name: stripe
  kind: oauth
```

Identity note: token-response `/stripe_user_id` is a zero-cost, robust stable
key (no dependency on an account-endpoint shape). A nicer human label
(`GET /v1/account` → `settings.dashboard.display_name` / `business_profile.name`
/ `email`) is a possible L2 enhancement via `identity.source: userinfo`; not
required for hidden-first and deliberately deferred (subtract before adding).

Non-generated companions landing on the batch-end merge:
- UI icon `ui/helio-app/src/integrations/icons/stripe.svg` + a `providerIcons.ts`
  registration (manual, never generated).
- i18n `description_key: stripe` label string.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` describing the
  verbs, `--json`/pagination conventions, and the read-mostly posture.
- The five `provider-gen` projections regenerate together (never committed on
  this tool branch — batch lead owns the one canonical regen).

---

## 6. Test plan — five layers

| Layer | Stripe-specific content | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fakes per resource: assert path/method, `Authorization: Bearer` injection, `Stripe-Version` pin, `--limit`/`--starting-after` params, `Idempotency-Key` on create/refund, and typed `apiError` from Stripe's `{error:{type,code,message}}` in text + `--json`. (The assumed-TTL and exchange-style assertions are integration-service-side, in L3.) | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=sk_test_… anycli stripe -- charge list --limit 3` (a test-mode secret key doubles as a valid bearer for `api.stripe.com` in the harness) against a **real Stripe test-mode account**. Proves field names, Bearer injection, pagination envelope, and error shape against the live API before the pin bump. | **Yes** — a Stripe test-mode account/key (account pool). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` accept the bundle incl. the new `form_secret_basic` value **and `access_token_ttl_seconds`**; integration-service unit tests for the new exchange/refresh style **and the assumed-TTL exchange path** (§4b: a no-`expires_in` Stripe response persists a non-nil `Expiry` ≈ `now+1h`); `helio-cli` builds against the anycli branch via local `replace`; both repos' unit suites green. | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` a `stripe` connection with `access_token` + `refresh_token` and an **assumed-TTL-derived `expiry` that is the same kind the real exchange persists (§4b), backdated past the refresh skew** — i.e. representing the guaranteed real state ~1 h after any connect. This is *not* a hand-fabricated `expires_at` shape the exchange never produces (the earlier draft's mistake): the load-bearing "the real exchange makes a non-nil expiry from a no-`expires_in` response" proof lives in the L1/L3 unit above; L4 only ages that same expiry so the next `heliox tool stripe -- balance get` forces the token gateway's refresh-and-write-back (A3) through `api.stripe.com/v1/oauth/token` with `form_secret_basic` Basic auth. Success = live Stripe data returned, not a replayed seed. | **Yes** — a real OAuth access+refresh token pair from the dev-mode app (dev-mode app creation gates this; lane 1 distributes dev `client_id`/`client_secret` as uncommitted local `config/cloud.yaml` entries). |
| **L5** full connect | Once, hidden, pre-flip: `heliox tool stripe auth` → consent on the Stripe dev/test-mode app at `marketplace.stripe.com/oauth/v2/authorize` → `oauth_connected` system event fires on the origin channel → one **unseeded** live `heliox tool stripe` run through the created connection. Human-in-the-loop (oauth L5, plan lane 3). | **Yes** — live Stripe consent on a real test account; human consent session. |

Rollout: land hidden + generated + L1–L4 green; run L5 while hidden; then flip
`visible: true` + regenerate as the single go-live change — and, because Stripe
is oauth_review, only after **Marketplace review clearance** for the public
install link (review gates the flip, never dev/L4/merge).

---

## 7. Divergences recorded (independent-judgment check)

- **Auth flow selection is a real decision, not a given.** The catalog says
  "oauth_review"; the audit cited Stripe Apps OAuth. Both are confirmed correct,
  but a naive reading could have picked the more famous **Connect OAuth** — which
  has deprecated tokens and does not fit our per-connection bearer model. Chose
  Stripe Apps OAuth deliberately (§3.1–3.2).
- **Token-endpoint client auth ≠ standard `client_id:client_secret` Basic.**
  Official docs authenticate with the **secret key alone** as the Basic
  username. Neither the catalog nor the audit enumerated this; it drives the
  §4a capability growth. Verified against the official curl (`-u sk_live_***:`).
- **The token response carries no `expires_in`, but the access token expires in
  1 hour.** Verified against the official code-exchange and refresh responses
  (7 fields each: `access_token`, `refresh_token`, `livemode`, `scope`,
  `stripe_publishable_key`, `stripe_user_id`/`account_id`, `token_type` — no
  expiry field). Without a synthesized expiry, `tokenResponse.expiry()` returns
  `nil`, `needsRefresh()` never fires, and the connection 401s ~1 h after
  connect with no recovery. This drives the **§4b assumed-TTL** growth
  (`oauth.access_token_ttl_seconds: 3600`), which the salesforce `assumed_ttl`
  precedent mirrors and which does not exist on this worktree base. **An earlier
  draft of this DESIGN missed this**: it declared the refresh machinery but never
  made the expiry non-nil, and its L4 forced refresh with a hand-fabricated
  `expires_at` the real exchange never produces — corrected in §3.1, §4b, §6.
- **Exchange body is `grant_type` + `code` only — no `redirect_uri`.** The
  documented `curl` sends exactly those two params (plus the `-u` secret-key
  Basic auth); `redirect_uri` appears only in the authorize URL. An earlier
  draft included `redirect_uri` in the exchange body; the §4a `form_secret_basic`
  case therefore builds its own minimal body rather than falling through to the
  shared form path (which appends `redirect_uri` + body client creds).
- **Lane unchanged.** oauth_review stands; the refinement is mechanics + the
  hidden-first note that dev-mode apps unblock L4/L5 pre-review.
- **No `toolToProvider` entry** (id == key == `stripe`), and **no grouped
  command** (Stripe is not a corporate family in this catalog).
