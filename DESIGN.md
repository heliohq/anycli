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

**Token response (verified):** `{ token_type, expires_in, access_token,
public_token, refresh_token, razorpay_account_id }`. Semantics:
- **Access token TTL: 90 days** (`expires_in` in seconds). **Refresh token TTL:
  180 days.**
- **Refresh token ROTATES** — calling `/token` with `grant_type=refresh_token`
  returns a new `access_token` **and** a new `refresh_token`; "the old refresh
  token will be expired automatically from this point." → bundle
  **`refresh_lease: credential`** (the rotating value the Xero / Sage /
  FreshBooks / Square bundles already exercise). **No new integration-service
  capability expected** — the `standard_oauth` `refresh_lease` allowed-set
  already carries `credential`. Confirm the set membership on the branch base
  before assuming reuse.
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
/razorpay_account_id` (present in the token response — no extra userinfo GET
needed). Label: the account id is always available; a human-readable business
name would need a stage-1-confirmed merchant/account GET, so `label_candidates`
falls back to `/razorpay_account_id` and adds a business-name pointer only if
stage-1 confirms a cheap account-fetch endpoint. `account get` (whoami) for L5
resolves against whatever account endpoint the granted scope exposes — confirm
at stage 1.

**Revoke:** `https://auth.razorpay.com/revoke` exists (verified) →
`disconnect_mode: declarative` with a declarative revoker (the golden-path
`standard_oauth` revoker), not `local_only`.

**`‹stage-1›` open items** (resolve from the partner dev account before the dev
branch finalizes request shapes): `token_exchange_style` — Razorpay posts
`client_id`/`client_secret` in the request **body** (not a Basic header), so
`form_secret` vs `json_secret` per the exact `Content-Type` the `/token`
endpoint accepts (confirm on the dev app; default `form_secret`); the `mode`
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
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://auth.razorpay.com/authorize
    token_url: https://auth.razorpay.com/token
    token_exchange_style: form_secret            # ‹stage-1› form_secret|json_secret (secret in body, not Basic)
    pkce: none                                   # ‹stage-1› (undocumented for this confidential client)
    display_scopes: [read_write]                 # or read_only for a read-first first pass; rx_* deferred
    single_active_token: false                   # per-merchant multi-tenant tokens
    refresh_lease: credential                    # refresh token rotates (old expires on refresh) — verified

identity:
  source: token_response
  stable_key: /razorpay_account_id               # present in token response — verified
  label_candidates: [/razorpay_account_id]       # + business-name pointer if stage-1 confirms an account GET

connection:
  mode: isolated
  disconnect_mode: declarative                   # https://auth.razorpay.com/revoke — verified
  runtime_strategy: standard_oauth               # golden path — zero provider Go expected

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
- **runtime_strategy `standard_oauth`.** Razorpay is a standard
  authorization-code + rotating-refresh Bearer provider with a declarative
  revoke endpoint and token-response identity — the golden path composes the
  exchanger + declarative identity resolver + declarative revoker with **zero
  provider-specific Go**. Do **not** pre-build a `service/adapter_razorpay.go`;
  reach for one only if stage-1 uncovers a non-standard token-exchange dialect
  (it should not — the flow is textbook OAuth2).
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

**`‹stage-1›` open items (resolve from the partner dev account before the dev
branch writes final request shapes):** `token_exchange_style` (`form_secret` vs
`json_secret` — secret is in the body, not a Basic header); the authorize `mode`
(`test|live`) param wiring (static extra-param vs config field); PKCE support
(default `none`); the exact account/whoami endpoint + a business-name label
pointer; per-resource `/v1` vs `/v2` paths; whether Technology-Partner enrollment
exposes a self-serve dev/test tier to Helio (the only branch that could flip the
lane to `api_key`, §1 — not expected).

**Auth-lane decision (resolved, not deferred):** `oauth_review` — confirmed
against the official docs and the audit rubric (§1). The catalog stands; no §6
amendment. The single fallback branch (`api_key` Basic-secret) triggers only if
Technology-Partner enrollment is wholly unavailable to Helio at stage 1, which
the public, documented partner onboarding does not indicate.
