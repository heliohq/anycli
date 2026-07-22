# Razorpay — per-tool design (`heliox tool razorpay`)

Scratch design doc on branch `tool/razorpay`. Batch lead strips it at batch-end.

- **anycli id (axis ②):** `razorpay`
- **provider catalog key (axis ③):** `razorpay`
- **CLI command word (axis ①):** `razorpay` (flat, ungrouped)
- **Auth lane:** `oauth_review` · **Wave:** 3 · **Category:** Payments & Commerce
- **Catalog row:** 171 of `008-300-integrations-rollout-plan.md` §4 · OAuth-audit row 173
- **Tool form:** `service` type (no agent-suitable official CLI; HTTP API only — §3)

Razorpay (razorpay.com) is India's largest payment-gateway + business-banking
platform: accept payments (orders → payments), issue refunds, manage customers,
create payment links, track settlements and subscriptions, plus RazorpayX
banking (payouts/contacts/fund-accounts). This tool wraps the **Razorpay REST
API** so a Helio teammate can act as a finance / revenue-ops / support colleague
on a connected merchant's account.

Everything below was verified against Razorpay's official docs (July 2026) and
the actual repo code, not inherited from the catalog. The auth-lane question —
the one the review flagged — is resolved from first-party docs in §1, with the
catalog-vs-primary-auth tension called out explicitly.

---

## 1. Auth-lane verification — independent, against the official rubric

**The catalog lists Razorpay as `oauth_review`. The review finding is correct
that Razorpay's _primary, self-serve_ API auth is HTTP Basic with
`key_id:key_secret` — but that does NOT contradict `oauth_review`, and no
catalog amendment is warranted.** Razorpay is the exact dual-auth shape as
Stripe (catalog row 35, also `oauth_review`): a Basic-auth secret-key path for
single-account/self-serve use, _and_ a real multi-tenant authorization-code
OAuth path for platforms acting on behalf of many merchants — the latter gated
behind a partner-review program. Helio is the multi-tenant platform case, so the
OAuth path is the correct integration, and its review gate is what makes the lane
`oauth_review`, not `api_key` or `oauth_light`.

Both facts are first-party-confirmed:

- **Basic-auth secret keys — self-serve (the api_key fallback).** "All Razorpay
  APIs are authenticated using `Basic Auth`" with
  `Authorization: Basic base64(key_id:key_secret)` against
  `https://api.razorpay.com/v1` (verified: `razorpay.com/docs/api/authentication/`).
  Any merchant self-generates a key pair from their own dashboard — no review.
  This is exactly the audit's stated fallback: "merchant-provided
  key_id/key_secret API keys."
- **Multi-tenant authorization-code OAuth — partner-reviewed (the chosen path,
  → `oauth_review`).** Razorpay ships a production OAuth 2.0 flow at
  `https://auth.razorpay.com/{authorize,token,revoke}` that lets a platform
  "obtain an access token with the consent of the user, without them having to
  compromise their API key secret" and call APIs on the sub-merchant's behalf
  via `Authorization: Bearer <access_token>` (verified:
  `razorpay.com/docs/partners/.../integrate-oauth/` and `.../integration-steps/`).
  Registering the OAuth app requires **enrolling as a Razorpay Technology
  Partner** — a sign-up/onboarding gate ("Sign up with Razorpay as a Technology
  Partner … to register your application on the Dashboard"), not a self-serve app
  create. That partner-onboarding review is precisely the audit rubric's
  `oauth_review` trigger ("human review / partner-program / verification gate →
  `oauth_review`").

**Verdict: `oauth_review` is confirmed and correct.** It matches the catalog, the
OAuth-audit row 173 verdict, and the Stripe precedent. The review gate blocks
**only the visible flip** (master plan §2) — never dev, L4, or the batch-end
merge; Razorpay ships **code-complete but hidden** in its Wave-3 batch and the
Technology-Partner review clearance is what ungates the flip. Dev/test-mode
partner apps are creatable pre-clearance (standard for `oauth_review`, and what
makes L4 runnable), which the master plan's lane-1 dev-mode app creation
provisions. **No divergence to record; no `§6` catalog amendment.** The one honest
caveat, logged here for the batch lead: if Technology-Partner enrollment proves
_wholly_ unavailable to Helio at stage 1 (pure invite-only, no dev/test tier),
Razorpay falls back to the `api_key` Basic-secret path — a genuine catalog
amendment (`oauth_review → api_key`) that collapses §5's bundle to a
credential-entry connect flow. That is the only branch under which the lane
changes, and it is not the expected outcome (the partner program is a public,
documented, self-sign-up onboarding, not invite-only).

---

## 2. Which official API surface this tool wraps, and why

Driven by what an **AI teammate** actually does on a connected Razorpay account —
"what did we collect this week", "refund this payment", "look up this customer",
"send a payment link", "when's the next settlement" — not by mirroring the full
gateway API. Razorpay's domain model (confirmed from `razorpay.com/docs/api/`) is
**orders → payments → refunds**, plus **customers**, **payment links**,
**settlements**, and **subscriptions**; RazorpayX adds **payouts/contacts/fund
accounts**. The AI-relevant, read-mostly-plus-scoped-write surface:

| Resource | AI teammate use | Verbs (planned) |
|---|---|---|
| **Payments** | "list recent payments", "show payment X", "what failed today" | `payment list`, `payment get`, (`payment capture` — write, gated) |
| **Orders** | "list orders", "show order X", reconcile an order to its payments | `order list`, `order get`, (`order create` — write, gated) |
| **Refunds** | "refund payment X", "list refunds", "status of refund Y" | `refund list`, `refund get`, (`refund create` — money movement, §3 safety) |
| **Customers** | "look up customer X", "list customers", create a customer before invoicing | `customer list`, `customer get`, (`customer create`) |
| **Payment Links** | "send a payment link for ₹X", "list open links", "status of link Y" | `payment-link list`, `payment-link get`, (`payment-link create` — write) |
| **Settlements** | "when's the next settlement", "list settlements", settlement reporting | `settlement list`, `settlement get` |
| **Subscriptions** | "list active subscriptions", "show subscription X" | `subscription list`, `subscription get` |
| **Account / identity** | connection labeling; L5 whoami | `account get` (whoami — see §4 identity) |

**Why these and not more:** they are the nouns a payments/finance colleague
reasons over; they map 1:1 onto provider-neutral JSON an agent can consume; and
read verbs are safe to ship first. **Money-moving writes** (`refund create`,
`payment capture`, `payment-link create`, `order create`) are **deferred to a
second pass** — Razorpay moves real money; ship read-first, and gate any
create/capture/refund verb behind explicit confirmation semantics plus a stage-1
review of Razorpay's idempotency-key requirement before enabling. **Webhooks**
(the platform advertises them) are **out of scope** for a `heliox tool`
request/response passthrough — event ingest belongs to a Helio service, not this
tool. **RazorpayX banking** (payouts) is out of this tool's first scope: it is a
separate scope family (`rx_*`, §4) and a higher-risk money-out surface; add it
only after the core gateway verbs ship and a dedicated review.

Cross-cutting request conventions (verified against `razorpay.com/docs/api/`):
- **Base URL** `https://api.razorpay.com/v1` (a few resources are on `/v2`; the
  service pins per-resource paths, not a single global version).
- **Pagination**: `count` (max 100) + `skip` offset, plus optional `from`/`to`
  Unix-timestamp filters on list endpoints. Surfaced as `--count` / `--skip`
  (+ `--from`/`--to`) on every list verb; `--json` returns the
  `{ "entity": "collection", "count": N, "items": [...] }` envelope verbatim.
- **Amounts** are in the smallest currency unit (paise for INR) — passed through
  verbatim; the tool does not silently rescale.
- **Errors**: Razorpay returns `{ "error": { "code", "description", "source",
  "step", "reason", "metadata" } }` with a 4xx. The service maps this to the
  typed `apiError` exit-code contract in §3.

---

## 3. anycli definition (data plane)

**Type: `service`** — no agent-suitable official Razorpay binary exists (rules
out `cli` type per SKILL.md stage-1 rubric; there are language SDKs, not a
non-interactive `--json` CLI to provision into the image), so implement in
`internal/tools/razorpay/` against the HTTP API. Go package name `razorpay` (id
has no dashes/leading digit → no normalization), registered
`RegisterService("razorpay", &razorpay.Service{})` in `internal/tools/register.go`,
definition file `definitions/tools/razorpay.json`.

- **Shape**: copy the `internal/tools/notion/` reference impl (per
  `references/anycli-development.md`) — a cobra tree grouped by resource
  (`payment`, `order`, `refund`, `customer`, `payment-link`, `settlement`,
  `subscription`, `account`), a `BaseURL`/`HC`/`Out`/`Err` struct so tests point
  at an `httptest` server and capture output, and the documented exit-code
  contract: **0** success, **1** runtime/API failure (typed `apiError` from
  Razorpay's error envelope), **2** usage/parse error. `--json` on every
  subcommand emits a structured envelope (and a structured `--json` error
  envelope). Non-interactive, `--json`-first, agent-consumable. No raw Razorpay
  passthrough beyond the `--json` envelope.

- **Auth injection** — the token gateway projects the connection's OAuth bearer
  token into `token.access_token`; the bundle `credential.fields.access_token`
  maps it, and the anycli definition injects it as an env var the service reads
  and sends as `Authorization: Bearer <token>`:

  ```json
  {
    "name": "razorpay",
    "type": "service",
    "description": "Razorpay payments, refunds, customers, and payment links (OAuth)",
    "auth": {
      "credentials": [
        { "source": {"field": "access_token"},
          "inject": {"type": "env", "env_var": "RAZORPAY_ACCESS_TOKEN"} }
      ]
    }
  }
  ```

  Razorpay accepts the OAuth access token as `Authorization: Bearer <token>`
  against `api.razorpay.com/v1` — confirmed by the OAuth integration-steps doc
  ("Provide the access token in the `Bearer` authorisation header"). This is the
  same single-credential Bearer shape as the Stripe bundle; the Basic-auth
  secret-key path (§1) is **not** used, so the definition binds only
  `access_token`, not a key/secret pair.

- **L1 TDD**: `httptest.Server` fakes for each verb (payment/order/refund/
  customer/payment-link/settlement/subscription/account) assert request path,
  method, the injected `Authorization: Bearer` header, the `count`/`skip`
  pagination params, and both plain-text and `--json` error rendering. No live
  API in unit tests (that is L2).

---

## 4. Credential fields & the exact auth flow (oauth_review)

**Registration model:** one Razorpay **Technology Partner OAuth app**
(Helio-owned), registered on the Partner Dashboard after Technology-Partner
enrollment — **reviewed** (partner onboarding), not a self-serve app create;
yields `client_id` + `client_secret`. Dev/test-mode app creation precedes
review clearance (standard `oauth_review`), which is what makes L4 runnable
before the visible flip.

**Flow (authorization code) — all endpoints first-party-verified:**
1. `heliox tool razorpay auth` mints a connect intent; user is redirected to
   `https://auth.razorpay.com/authorize` with `client_id`, `redirect_uri`,
   `scope`, `state` (+ a `mode` param `test|live` — see stage-1 note below).
2. Merchant consents on Razorpay's authorize screen (per-account); Razorpay
   calls back with `code`.
3. integration-service exchanges `code` at `https://auth.razorpay.com/token`
   (`grant_type=authorization_code`, `client_id`, `client_secret`, `redirect_uri`,
   `code`) for the token response.
4. Token gateway serves `access_token` to the resolver; anycli injects it as
   `RAZORPAY_ACCESS_TOKEN`; the service sends it as `Authorization: Bearer`.

**Token response (verified):** the **initial `authorization_code`** exchange
returns `{ token_type, expires_in, access_token, public_token, refresh_token,
razorpay_account_id }`; the **`refresh_token`** response returns only
`{ token_type, expires_in, access_token, public_token, refresh_token }` — **no
`razorpay_account_id`** (see Identity below). Semantics:
- **Access token TTL: 90 days** (`expires_in` in seconds). **Refresh token TTL:
  180 days.**
- **Refresh token ROTATES** — calling `/token` with `grant_type=refresh_token`
  returns a new `access_token` **and** a new `refresh_token`; "the old refresh
  token will be expired automatically from this point." → bundle
  **`refresh_lease: credential`** (the rotating value the Xero / Sage /
  FreshBooks / Square bundles already exercise). **No `refresh_lease` capability
  growth needed** — the `standard_oauth` `refresh_lease` allowed-set already
  carries `credential` (confirm the set membership on the branch base before
  assuming reuse). This is separate from — and does **not** cancel — the
  `token_exchange_style: json_secret` growth this tool **does** need (§4 exchange
  style, §5 runtime_strategy).
- `public_token` is a client-side publishable token — **not** stored/used by this
  tool (server-side Bearer only). `razorpay_account_id` drives identity (below).

**Credential fields the bundle declares** (never real values — those go in
integration-service config): `required_config_fields: [oauth.client_id,
oauth.client_secret]`.

**Scopes (verified):** `read_only`, `read_write` (gateway), and the RazorpayX
family `rx_read_only`, `rx_read_write`, `rx_partner_read_write`. First scope
request: **`read_write`** (gateway payments/refunds/customers/links — the §2
surface), or `read_only` if the first pass ships read-verbs only. The `rx_*`
banking scopes are **not** requested in the first bundle (payouts out of scope,
§2). `display_scopes` lists exactly what is granted.

**Identity:** `identity.source: token_response`, `stable_key:
/razorpay_account_id`. **Captured at connect-time from the initial
`authorization_code` exchange** — verified against the official integration-steps
doc: `razorpay_account_id` ("Identifies the sub-merchant ID who granted the
authorisation") is returned **only** on the initial code exchange; the
`grant_type=refresh_token` response carries just `token_type` / `expires_in` /
`access_token` / `public_token` / `refresh_token` and **omits** the account id.
This is exactly Stripe's shape (its bundle documents the acct id as a connect-time
capture): `token_response` identity capture happens once at connect, not on every
refresh, so the resolver must persist the id at initial connect and never expect
it back on a refresh cycle. No extra userinfo GET is needed. Label: the account
id is always available; a human-readable business
name would need a stage-1-confirmed merchant/account GET, so `label_candidates`
falls back to `/razorpay_account_id` and adds a business-name pointer only if
stage-1 confirms a cheap account-fetch endpoint. `account get` (whoami) for L5
resolves against whatever account endpoint the granted scope exposes — confirm
at stage 1.

**Revoke:** Razorpay ships a real revoke endpoint — `POST
https://auth.razorpay.com/revoke` with mandatory `client_id`, `client_secret`,
`token_type_hint` (`access_token|refresh_token`), and `token` (verified against
the official integration-steps doc). So the disconnect is a **provider revoke**,
not a client-side no-op:
- `connection.disconnect_mode: provider_revoke` (**not** `declarative` — that is
  not a valid enum value; `validate.go` accepts only `provider_revoke` /
  `local_only` / `strategy`. "Declarative revoker" is the *implementation facet*
  the `standard_oauth` runtime strategy composes, **not** the `disconnect_mode`
  value).
- For `standard_oauth`, `provider_revoke` **requires** an `auth.oauth.revoke`
  block (`validate.go:503-505`), and `local_only` **forbids** one. We provide the
  block, modeled on the Xero bundle:
  - `url: https://auth.razorpay.com/revoke`
  - `client_auth: form` — `client_id`/`client_secret` go in the request body, not
    a Basic header (`validate.go:480` allows `none|basic|form`; Razorpay uses
    neither Basic nor a bare call, so `form`). `‹stage-1›` the endpoint samples a
    JSON body; the closed revoke capability has no `json` client-auth variant, so
    `form` is the chosen approximation — confirm on the dev app that the revoke
    exchanger's form-encoded client creds are accepted, and if the endpoint is
    strictly JSON-only, scope a `json`-client-auth revoke capability growth.
  - `token: refresh_token`, `token_type_hint: refresh_token` — revoke the
    rotating refresh token to tear down the whole chain (Razorpay requires
    `token_type_hint`; no `fallback_token`, so `token_type_hint` may be non-`none`
    per `validate.go:495`).

**Token exchange style — `json_secret`, and it needs a capability growth (docs-resolved, not stage-1).**
The official `/token` sample posts `Content-Type: application/json` with
`client_id` and `client_secret` in the **JSON body** (verified — the
integration-steps curl sends `-H "Content-type: application/json" -d '{ "client_id":
…, "client_secret": …, "grant_type": "authorization_code", … }'`). The docs-correct
style is therefore **JSON-body-with-secret (`json_secret`)** — which is **not** on
the branch base: `validate.go:243` allows only `form_secret` / `form_basic` /
`json_basic` (`json_basic` puts client auth in a Basic header, which Razorpay does
**not** use; `json_secret` is absent). So the correct exchange requires an
**integration-service capability growth** as part of this tool's work: add
`json_secret` as a reviewed `token_exchange_style` enum value **plus** the matching
exchanger branch in `service/oauth_exchange.go` (JSON body carrying `client_id` +
`client_secret`). Confirm the enum's absence on the branch base first (per
`validate.go` it is absent); if a later base rev already adds it, reuse it. **This
contradicts any "pure golden path, zero provider-specific Go / no new capability"
framing** — the exchange-style enum growth is real, reviewed integration-service
work (a small, contained enum + exchanger branch, not a per-provider adapter).

**`‹stage-1›` open items** (resolve from the partner dev account before the dev
branch finalizes request shapes): the `mode`
(`test|live`) authorize param — whether it rides as a static authorize extra-param
or a config field (default `live` for production, `test` for the dev app);
PKCE — undocumented for this confidential-client flow, so `pkce: none` pending
confirmation; the exact account/whoami endpoint for `account get` + label; and
per-resource `/v1` vs `/v2` paths.

---

## 5. Helio provider bundle plan (hidden-first)

`integrations/providers/razorpay/provider.yaml`, modeled on the `stripe` /
`notion` `standard_oauth` bundles (directory name = key = `razorpay`; the
generator enforces directory-key equality). Skeleton (values marked `‹stage-1›`
are confirmed once the partner dev app is in hand):

```yaml
schema: helio.provider/v1
key: razorpay
go_name: Razorpay

presentation:
  name: Razorpay
  description_key: razorpay
  consent_domain: razorpay.com
  visible: false                               # hidden-first (SKILL.md stage 4/10)
  order: <next>

auth:
  type: oauth
  owner: individual                              # per-merchant consent — matches Stripe/Xero (NOT assistant; that triggers the app-bot org-admin gate)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://auth.razorpay.com/authorize
    token_url: https://auth.razorpay.com/token
    token_exchange_style: json_secret            # /token is Content-Type: application/json with client_id/secret in the JSON body — needs json_secret enum + exchanger branch (capability growth, §4)
    pkce: none                                   # ‹stage-1› (undocumented for this confidential client)
    display_scopes: [read_write]                 # or read_only for a read-first first pass; rx_* deferred
    single_active_token: false                   # per-merchant multi-tenant tokens
    refresh_lease: credential                    # refresh token rotates (old expires on refresh) — verified
    revoke:
      url: https://auth.razorpay.com/revoke      # verified — real revoke endpoint
      client_auth: form                          # client_id/client_secret in body (validate.go allows none|basic|form); ‹stage-1› endpoint samples JSON, confirm form-encoded creds accepted
      token: refresh_token                       # revoke the rotating refresh token to tear down the chain
      token_type_hint: refresh_token             # Razorpay requires token_type_hint

identity:
  source: token_response
  stable_key: /razorpay_account_id               # captured at INITIAL connect (authorization_code response); NOT present on refresh — verified
  label_candidates: [/razorpay_account_id]       # + business-name pointer if stage-1 confirms an account GET

connection:
  mode: isolated
  disconnect_mode: provider_revoke               # real revoke endpoint → provider_revoke (NOT the invalid `declarative`); standard_oauth+provider_revoke requires the auth.oauth.revoke block above
  runtime_strategy: standard_oauth               # authorization-code + rotating-refresh, declarative identity/revoke — but see §4: the json_secret exchange style needs an enum + exchanger branch growth

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: razorpay
  kind: oauth
```

- **Three axes:** ① `razorpay` ② `razorpay` ③ `razorpay` — all identical, so
  **no `toolToProvider` resolver entry** (identity holds) and no grouped
  `tool.group`.
- **runtime_strategy `standard_oauth`, but with one reviewed capability growth.**
  Razorpay is a standard authorization-code + rotating-refresh Bearer provider
  with a provider revoke endpoint and token-response identity, so the strategy
  composes the generic exchanger + declarative identity resolver + declarative
  revoker. **It is not a pure zero-Go golden path**, though: its `/token` endpoint
  is JSON-body-with-secret (`json_secret`), which is **not** on the branch base
  (`validate.go:243` = `form_secret|form_basic|json_basic`). This tool must add
  `json_secret` as a reviewed `token_exchange_style` enum value **plus** the
  matching exchanger branch in `service/oauth_exchange.go` (§4) — a small,
  contained capability growth, **not** a per-provider `service/adapter_razorpay.go`.
  Do **not** pre-build an adapter; the exchange-style growth is the entire
  provider-specific surface, and everything else (identity, revoke, refresh) is
  declarative.
- **Config Sync:** `oauth.client_id`/`oauth.client_secret` land in **both**
  `config/` and the `deploy/` Helm Secret together (partial config fails
  integration-service startup; fully-absent renders `configured: false` and is
  safe hidden) — lane-1's landing, before Razorpay's L5. Never commit a real
  secret or an empty placeholder.
- **UI icon:** `ui/helio-app/src/integrations/icons/razorpay.svg` + register in
  `providerIcons.ts` (manual, never generated) + i18n label for
  `description_key: razorpay`.
- **AI-facing doc:** provider sub-doc under `agents/plugins/heliox/skills/tool/`,
  published on the batch-end plugin version bump.

---

## 6. Test plan — five layers (honest gating)

| Layer | Razorpay-specific plan | External creds needed? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: `httptest` fake for each verb (payment/order/refund/customer/payment-link/settlement/subscription/account); assert request path + method, injected `Authorization: Bearer`, `count`/`skip` pagination params, plain + `--json` error envelopes, exit codes. No live API. **Runnable now** — request shapes are public-doc-verified (unlike a partner-gated provider), so fakes need no stage-1 unblock. | No creds |
| **L2** harness real-API | `ANYCLI_CRED_ACCESS_TOKEN=<merchant OAuth access token> anycli razorpay -- payment list` (etc.) against Razorpay's **real API** — the mandatory gate before pinning. A Razorpay **test-mode** account + a dev-app-minted access token suffices (test-mode keys/tokens are self-serve in the dashboard). | **Yes** — a test-mode Razorpay account + a dev-app OAuth token (lane 1 dev app + lane 2 account) |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` against the `razorpay` bundle; `helio-cli` + integration-service unit suites. Point `helio-cli/go.mod` at the anycli branch via a local **uncommitted** `replace`. Confirm `refresh_lease: credential` is in the `standard_oauth` allowed-set on the branch base (expected present). | No |
| **L4** singleton + seeded token | Start singleton (`env: dev`); `POST /internal/test-only/connections/seed` a **real** `access_token` (+ `refresh_token`, deliberately short `expires_at` to force the token gateway's refresh-and-write-back path — Razorpay's real 90-day TTL won't self-exercise refresh) for provider `razorpay` against a **real seeded assistant/org**; run `heliox tool razorpay -- payment list` through the real token gateway. Success = live Razorpay data returned. | **Yes** — a real access token from the registered dev app (lane 1) |
| **L5** full connect flow | `heliox tool razorpay auth` → Razorpay authorize consent on the dev/test-mode app → `oauth_connected` system event on the auth-session channel → unseeded live `heliox tool razorpay -- account get`. Human-in-the-loop (`oauth_review` → master-plan lane 3). Runs once, **still hidden**, before the visible flip; the visible flip is **additionally** gated on Technology-Partner review clearance. | **Yes** — human consent on a real Razorpay account + partner review clearance for the flip |

**Credential-gated layers: L2, L4, L5** — all depend on lane-1 partner dev app
creation and lane-2 test-mode account. **L1 and L3 are agent-runnable now**
(Razorpay's request shapes and OAuth endpoints are fully public-doc-verified, so
there is no docs-gate on writing the unit fakes — unlike a WAF/partner-gated
provider). Per master-plan §2, the tool branch is **expected** to fail
`provider-gen --check` in CI until the batch lead's single batch-end regen; do
not commit local regens to green the branch.

---

## 7. Sources & open items

**Confirmed (first-party / official — `razorpay.com/docs`):**
- Basic-auth self-serve secret keys + base URL `https://api.razorpay.com/v1`:
  `razorpay.com/docs/api/authentication/`, `razorpay.com/docs/api/`.
- Multi-tenant OAuth 2.0 partner flow, Technology-Partner enrollment gate,
  endpoints `https://auth.razorpay.com/{authorize,token,revoke}`, Bearer usage:
  `razorpay.com/docs/partners/platform/onboard-businesses/integrate-oauth/` and
  `.../technology-partners/onboard-businesses/integrate-oauth/integration-steps/`.
- Scopes (`read_only`, `read_write`, `rx_read_only`, `rx_read_write`,
  `rx_partner_read_write`), token response fields (`token_type`, `expires_in`,
  `access_token`, `public_token`, `refresh_token`, `razorpay_account_id`), access
  90-day / refresh 180-day TTLs, and **rotating refresh token**: the
  integration-steps doc above.
- Resource model (orders, payments, refunds, payment links, settlements,
  subscriptions; RazorpayX payouts/contacts/fund-accounts): `razorpay.com/docs/api/`.

**Resolved from official docs (no longer stage-1):**
- **Token exchange style = `json_secret`** — the `/token` endpoint is
  `Content-Type: application/json` with `client_id`/`client_secret` in the JSON
  body. Requires an integration-service capability growth (new `json_secret`
  enum + exchanger branch); see §4/§5.
- **Revoke = `provider_revoke`** — `POST https://auth.razorpay.com/revoke` exists
  with `client_id`/`client_secret`/`token_type_hint`/`token`; the bundle carries
  the `auth.oauth.revoke` block (`client_auth: form`, `token: refresh_token`).
  `disconnect_mode: declarative` was an **invalid enum value** and is dropped.
- **Identity capture = connect-time only** — `razorpay_account_id` is returned on
  the initial `authorization_code` exchange, **not** on `refresh_token` responses.
- **Owner = `individual`** — per-merchant consent, matching Stripe/Xero.

**`‹stage-1›` open items (resolve from the partner dev account before the dev
branch writes final request shapes):** the authorize `mode` (`test|live`) param
wiring (static extra-param vs config field); PKCE support (default `none`); the
exact account/whoami endpoint + a business-name label pointer; per-resource `/v1`
vs `/v2` paths; whether the `form`-encoded revoke client creds are accepted (the
endpoint samples JSON — else a `json`-client-auth revoke growth); whether
Technology-Partner enrollment exposes a self-serve dev/test tier to Helio (the
only branch that could flip the lane to `api_key`, §1 — not expected).

**Auth-lane decision (resolved, not deferred):** `oauth_review` — confirmed
against the official docs and the audit rubric (§1). The catalog stands; no §6
amendment. The single fallback branch (`api_key` Basic-secret) triggers only if
Technology-Partner enrollment is wholly unavailable to Helio at stage 1, which
the public, documented partner onboarding does not indicate.

---

## 8. As-built notes & divergences from this design

Recorded for the batch lead. All verified against the official Razorpay docs
(July 2026) and the actual branch-base code.

1. **`json_secret` needs a JSON *refresh* branch too, not only the exchanger
   (§4 refinement).** §4 called for the `json_secret` exchange-style enum + the
   `service/oauth_exchange.go` exchanger branch. That is necessary but not
   sufficient: Razorpay's `/token` is `Content-Type: application/json` with the
   client creds in the body for **both** grants, and its access token expires in
   90 days with a **rotating** refresh token — so refresh *is* exercised (L4
   forces it with a short `expires_at`). The stock refresh path
   (`token_refresh.go`) uses `golang.org/x/oauth2`, which posts
   form-encoded and would be rejected. Implemented `refreshTokenJSONSecret` +
   `refreshOAuthTokenJSONSecret`: a JSON `refresh_token` grant, with non-2xx
   wrapped as `*oauth2.RetrieveError` so the existing `isPermanentRefreshError`
   classifier treats 4xx as reconnect-required and 5xx/transport as transient —
   identical semantics to the oauth2-library path, A3 strict write-back intact.
2. **`standard_oauth` refresh_lease allowed-set had to grow (§4 assumption
   corrected).** §4 said "confirm the `standard_oauth` `refresh_lease`
   allowed-set already carries `credential`." On the branch base it does **not**
   — the runtime contract pinned a single `refreshLeaseScope: OAuthLeaseNone`,
   and the generator rejected `credential` (`requires auth.oauth.refresh_lease
   "none"`). Grew `oauthRuntimeContract.refreshLeaseScope` (single) to
   `refreshLeaseScopes []OAuthLeaseScope` and set standard_oauth to
   `[none, credential]` (reviewed capability growth in `model/runtime_contract.go`
   + tests). This is the same growth the Xero/Sage/SignNow rotating-refresh
   bundles need; whoever lands first, the second's identical change conflicts at
   the batch-end merge — batch lead resolves.
3. **`account get` (whoami) deferred — no gateway-scope self-account endpoint.**
   §2/§4 listed an `account get` verb for connection labeling + L5 whoami.
   Against the official docs there is **no** self-account read usable with the
   granted gateway OAuth token: the `/v2/accounts/:id` Partner endpoint requires
   *partner* auth (the platform's own key/scope), not the sub-merchant's gateway
   Bearer token, so it would 403. The `account` resource is therefore **not**
   shipped; the anycli tool exposes the seven fully-doc-verified data resources.
   **L5 whoami uses `razorpay payment list --count 1`** (a guaranteed read under
   `read_only`/`read_write`). Identity/label still resolve from the connect-time
   `razorpay_account_id` (`identity.source: token_response`), which needs no
   account GET. If stage-1 surfaces a cheap merchant-facing account endpoint,
   add `account get` + a business-name label pointer then.
4. **Confirmed exactly as designed:** `json_secret` exchange style; provider
   `disconnect_mode: provider_revoke` with the `auth.oauth.revoke` block
   (`client_auth: form`, `token: refresh_token`, `token_type_hint:
   refresh_token`); `identity.source: token_response` on `/razorpay_account_id`;
   `owner: individual`; `refresh_lease: credential`; `pkce: none`. The bundle
   validates and `provider-gen --check` fails only on the expected batch-end
   regen drift (§6 / master-plan §2).
