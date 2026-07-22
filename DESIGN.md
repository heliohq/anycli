# Tool design: Paddle (`paddle`)

Scratch per-tool design for the `helio-tool-provider` pipeline. Batch lead strips
this at batch-end. Catalog row 172 (master plan §4): Product **Paddle**, anycli id
`paddle`, provider key `paddle`, auth lane **api_key**, wave **3**, category
Payments & Commerce.

## 0. Verdict check against official docs (independent judgment)

The catalog assigns `api_key`; the 2026-07-21 OAuth audit (row 174) keeps Paddle in
`api_key` — "no viable multi-tenant path". Confirmed against Paddle's official docs:

- **Two incompatible product generations.** *Paddle Classic* (vendor API:
  `vendor_id` + `vendor_auth_code`, form-encoded POSTs) is legacy and separate.
  *Paddle Billing* (current) authenticates with a **Bearer API key** and is the
  target here. We wrap **Paddle Billing only** — Classic is out of scope. (Recorded
  divergence-relevant fact; does not contradict the audit.)
- **No multi-tenant OAuth.** Paddle Billing has no authorization-code OAuth for a
  third party to act on arbitrary sellers' accounts. Its only OAuth-shaped surface
  is client-side tokens for checkout (`ctm_...`), which are publishable, checkout-scoped,
  and cannot read/write the management API. So a shared Helio client cannot mint
  per-seller tokens → `api_key` is correct. **No divergence to record in DESIGN.**

Sources: `developer.paddle.com/api-reference/about/authentication`,
`.../api-reference/overview`, and the Billing vs Classic distinction on
`developer.paddle.com`.

## 1. What an AI teammate does with Paddle → which API surface we wrap

Paddle is a merchant-of-record subscription-billing platform. The teammate's real
jobs are billing/support and revenue ops on the seller's own account:

- "What plan is this customer on, and did their last payment go through?" →
  read **customers**, their **subscriptions**, and **transactions**.
- "Pause / cancel / resume this subscription", "apply a one-time charge", "preview a
  plan change before I commit it" → **subscription lifecycle actions**.
- "Send this customer their invoice" → **transaction** get + **invoice PDF** URL.
- "Create a discount code for this deal", "what's our current catalog?" →
  **discounts**, **products**, **prices**.
- "Refund / credit this charge" → **adjustments**.
- "What's MRR / recovered revenue this month?" → **reports** + **metrics**.

That maps to the Paddle Billing management API. Base surface (Billing resources):
Catalog (products, prices, discounts), Customers (customers, addresses, businesses),
Billing (transactions, subscriptions, adjustments, pricing-preview), Reporting
(reports, metrics), Events (events, event-types, notifications, notification-settings).

**Endpoints wrapped (v1 cut — read-first, targeted writes):**

| Resource | Method + path | Why |
|---|---|---|
| customers | `GET /customers`, `GET /customers/{id}`, `POST /customers`, `PATCH /customers/{id}` | look up / create / update the billed party |
| customers | `GET /customers/{id}/credit-balances` | outstanding credit for support answers |
| addresses/businesses | `GET /customers/{id}/addresses`, `.../businesses` | invoicing/tax context |
| subscriptions | `GET /subscriptions`, `GET /subscriptions/{id}` | plan/status lookup |
| subscriptions | `PATCH /subscriptions/{id}` | change items/quantity/proration |
| subscriptions | `POST /subscriptions/{id}/cancel`, `.../pause`, `.../resume`, `.../activate` | lifecycle actions |
| subscriptions | `POST /subscriptions/{id}/charge`, `.../charge/preview`, `.../update/preview` | one-time charge + dry-run previews |
| transactions | `GET /transactions`, `GET /transactions/{id}` | payment history |
| transactions | `GET /transactions/{id}/invoice` | invoice PDF URL |
| transactions | `POST /transactions`, `POST /transactions/preview` | create / dry-run a charge |
| products | `GET /products`, `GET /products/{id}`, `POST /products`, `PATCH /products/{id}` | catalog |
| prices | `GET /prices`, `GET /prices/{id}`, `POST /prices`, `PATCH /prices/{id}` | catalog |
| discounts | `GET /discounts`, `GET /discounts/{id}`, `POST /discounts`, `PATCH /discounts/{id}` | promo codes |
| adjustments | `GET /adjustments`, `POST /adjustments` | refunds / credits |
| reports | `POST /reports`, `GET /reports`, `GET /reports/{id}`, `GET /reports/{id}/download-url` | revenue exports |
| events | `GET /event-types`, `GET /events`, `GET /notification-settings` | audit + the verify endpoint (below) |

Deletes are intentionally absent — Paddle transactions/subscriptions are financial
records that "can't be deleted"; the lifecycle verbs (cancel/pause/archive-via-status)
are the mutation surface. Reporting `metrics` and webhook-simulator endpoints are
deferred (low teammate value in v1).

## 2. AnyCLI definition (stage-1 rubric → `service` type)

`cli` type is rejected by the rubric: no official Paddle binary to provision. So
**`service` type**, HTTP against the Billing API, matching the `notion` reference
shape (`internal/tools/paddle/`, cobra tree grouped by resource, injectable
`BaseURL`/`HC`/`Out`/`Err` for httptest).

- **anycli id (axis ②):** `paddle` → `definitions/tools/paddle.json`, `type:"service"`.
- **Go package (naming §):** `internal/tools/paddle/` (no digit/dash issue),
  `RegisterService("paddle", &paddle.Service{})` in `internal/tools/register.go`.
- **Auth binding:** one credential `field: access_token` injected as env
  `PADDLE_API_KEY` (env inject; the service reads it and sets
  `Authorization: Bearer <key>`). anycli never sees Helio/OAuth — it only receives
  the resolver-supplied `access_token`.

### 2a. Command tree (verbs)

```
paddle customer   list|get|create|update|credit-balances|addresses|businesses
paddle subscription list|get|update|cancel|pause|resume|activate|charge|preview-update|preview-charge
paddle transaction list|get|create|invoice|preview
paddle product    list|get|create|update
paddle price      list|get|create|update
paddle discount   list|get|create|update
paddle adjustment list|create
paddle report     create|list|get|download-url
paddle event      list|types|notification-settings
```

Common flags: `--id`, `--status`, `--customer-id`, `--after <cursor>`,
`--per-page <n>`, `--json`; write verbs take resource-specific flags plus
`--data <json>` passthrough for the full Paddle request body (agent-friendly for
fields we don't surface as flags).

### 2b. Environment routing (the one Paddle-specific wrinkle)

The **key prefix encodes the environment** (`^pdl_(live|sdbx)_apikey_...`), and
sandbox vs live are **separate base URLs with non-interchangeable keys**. The service
selects the base URL from the injected key, so the user never supplies a base URL:

- `pdl_live_apikey_…` → `https://api.paddle.com`
- `pdl_sdbx_apikey_…` → `https://sandbox-api.paddle.com`
- Legacy unstructured 50-char keys (pre-2025-05-06, environment not encoded) →
  default `https://api.paddle.com`, overridable by injected `PADDLE_ENV=sandbox`.
  Recommend new-format keys in docs.

Confirmed against official docs: live/sandbox are **separate environments with
completely separate credentials and datasets** (`api.paddle.com` vs
`sandbox-api.paddle.com`; keys, products, customers, notifications are not shared),
and the **API version is a header, not a path segment**. The service sends
`Paddle-Version: 1` on every request so the response envelope shape the L1 fakes and
the `{data,meta}` parser assume stays pinned across Paddle's dated releases (omitting
it makes the account's default version — potentially newer — the implicit contract).

### 2c. JSON output shape

Pass Paddle's envelope through, don't re-wrap:

- Success: print the response `data` (object or array). With `--json`, emit
  `{"data": …, "meta": {"request_id": …, "pagination": {"per_page","next","has_more","estimated_total"}}}`
  so agents can page with `--after` from `meta.pagination.next`. Human mode prints a
  compact per-resource summary.
- Error: Paddle returns `{"error": {"type","code","detail","documentation_url"}, "meta":{"request_id"}}`.
  Map to the notion-style typed `apiError`: exit **1**, `--json` error envelope
  carrying `code`/`detail`/`documentation_url`/`request_id`. Validation (400) errors
  additionally carry an `error.errors[]` array of per-field failures — surface it in
  the `--json` envelope. Exit **2** for usage/parse errors; exit **0** success. 429
  (rate limit) surfaces as a retryable typed error (no built-in retry in v1); it
  carries a `Retry-After` header the envelope should echo. **Verified limits:** the
  cap is **per IP address**, not per key — **240 req/min** general, **1000 req/min**
  for pricing-preview endpoints, plus a tight subscription immediate-charge cap
  (~20/hr). (Earlier drafts said "100 req/s per key"; that is wrong on both axes.)

## 3. Credentials & auth flow (api_key lane, verified)

- **Credential:** a single Paddle Billing API key. Created in the Paddle dashboard
  (Developer Tools → Authentication), **server-side only**, **scoped** by granular
  permissions, default **90-day expiry** (max 1 year), optionally rotatable.
- **Wire:** `Authorization: Bearer pdl_<env>_apikey_…`. No client secret, no
  redirect, no refresh cycle — a raw bearer PAT. Expiry is real but long; on expiry
  the user pastes a new key (there is no programmatic refresh a shared client could
  run — consistent with the `api_key` lane).
- **Nuance for scoped keys:** a restricted key may lack permission for some resources;
  the verify endpoint (below) is chosen to succeed for **any** valid key regardless of
  resource scopes.

## 4. Helio provider bundle plan (`integrations/providers/paddle/provider.yaml`)

Three axes (all aligned → **no `toolToProvider` divergence entry, no resolver change**):
① CLI command word `paddle` · ② anycli id `paddle` · ③ provider key `paddle`.

Hidden-first: `presentation.visible: false` until the anycli pin ships `paddle` and
L1–L5 pass. `tool.kind: api-key`. No `experiment` gate (GA lane).

### 4a. Strategy: `manual_api_token` with liveness-verify + credential-fingerprint identity

The connect surface is a single secret field (`access_token`), stored via the
write-only `POST /connections/credentials` path into Vault; the key never touches the
bundle. Two facets, both **already precedented in this program** (reuse if the batch
base carries them; otherwise a minimal orthogonal add — see §4b):

1. **Verify (liveness, not identity):** `GET https://api.paddle.com/event-types`
   with `Authorization: Bearer <key>`. `event-types` is a platform catalog readable
   by any valid key irrespective of resource scopes — ideal liveness probe. 200 =
   valid; 401/403 = reject before any Vault write; wrong-environment keys (sandbox key
   against the live URL) fail here, which is the desired connect-time feedback.
2. **Identity (account_key + label):** Paddle exposes **no account/seller identifier**
   in any simple response (`event-types` is account-agnostic; `meta.request_id` is
   per-request). So account_key is a **SHA-256 fingerprint of the credential** (the
   knock/paperform "fingerprint identity deriver" precedent), with a static label
   (`Paddle account`). This is stable per key and leaks no secret bytes.

Bundle sketch:

```yaml
schema: helio.provider/v1
key: paddle
go_name: Paddle
presentation:
  name: Paddle
  description_key: paddle
  consent_domain: paddle.com
  visible: false
auth:
  type: api_key
  owner: individual
  required_config_fields: []          # no client secret → configured:true always
  api_key:
    header: Authorization
    scheme: bearer                     # verify sends "Authorization: Bearer <key>"
    setup_url: https://developer.paddle.com/api-reference/about/authentication
identity:
  source: strategy                     # fingerprint deriver, not a response pointer
  url: https://api.paddle.com/event-types   # liveness verify target (live)
connection:
  mode: isolated
  disconnect_mode: local_only          # no provider-side key-revocation API
  runtime_strategy: manual_api_token
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: paddle
  kind: api-key
```

(Exact field names — `scheme`, the fingerprint-deriver selector — follow whatever the
batch base already names them; the shape above is the intent, not a novel schema.)

### 4b. integration-service capability check (reuse-first)

Measured against **this worktree's base**, the declarative manual-token verifier
(`declarativeManualTokenVerifier`) sets the header to the **raw** token and **requires
a JSON-pointer stable key from the response** — neither fits Paddle. Two orthogonal,
already-precedented capabilities are needed; both should be **reused** if the batch
base has merged them, else added minimally:

- **Bearer scheme on the declarative verifier** — send `Authorization: Bearer <token>`
  (precedent: tally, loops "Bearer-scheme verifier").
- **Credential-fingerprint identity deriver** — derive account_key from the credential
  when the provider returns no account id (precedent: knock, paperform).

Both are general options, not Paddle-specific adapters — no new `service/adapter_*.go`.
If Paddle's batch predates both, this bundle contributes the same minimal enum/option
growth those tools did, with tests. **No new named RuntimeStrategy.**

Fallback if fingerprint-deriver reuse is unavailable and verify is undesired:
`manual_credentials` (design 317 D5, no verify, bad key surfaces at first use via
AnyCLI `CredentialRejected`) — weaker connect-time feedback and still needs a non-DSN
identity deriver, so it is **not** preferred.

### 4c. Config, icon, docs

- **Config:** none. `api_key` needs no Helio client id/secret, so nothing lands in
  `config/` or `deploy/` (the Config Sync hard rule is a no-op here) and the provider
  is `configured:true` immediately.
- **Icon:** `ui/helio-app/src/integrations/icons/paddle.svg` + register in
  `providerIcons.ts` (manual, never generated).
- **AI-facing docs:** provider sub-doc under `agents/plugins/heliox/skills/tool/`
  (env routing, invoice-PDF flow, previews-before-writes, cursor pagination), plugin
  version bump + marketplace publish riding the batch-end merge.
- **Generation:** `provider-gen` + `provider-gen --check` from
  `go-services/integration-service` — five projections committed together at batch end
  (not on this tool branch; CI red on this branch until then is expected per §2 of the
  master plan).

## 5. Test plan → five layers

| Layer | Paddle specifics | External creds? |
|---|---|---|
| **L1** anycli unit | httptest fake: assert `Authorization: Bearer` header, prefix→base-URL routing (live/sandbox/legacy), `{data,meta}` decode, cursor paging via `meta.pagination.next`, Paddle `error` object → typed `apiError` (exit 1) + `--json` error envelope, usage errors exit 2 | no |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=pdl_sdbx_apikey_… anycli paddle -- customer list --per-page 2`; a sandbox key exercises the sandbox base-URL route end-to-end. Verify a lifecycle read (`subscription get`) + `transaction invoice`. | **yes** — a Paddle **sandbox** account API key (free, self-serve) |
| **L3** provider-gen + suites | `provider-gen --check` green with the bundle; anycli `go test ./...` + integration-service tests (incl. the Bearer-scheme/fingerprint capability tests if added) | no |
| **L4** singleton + seed | api_key provider is seedable: `POST /internal/test-only/connections/seed` with `access_token` (no refresh cycle), then `heliox tool paddle -- product list` reaches the live/sandbox API through the token gateway | **yes** — same key as L2 (seed a **live** key if L5 uses live; see §5 note) |
| **L5** full connect flow | api_key L5 path (master plan §2): open connect link → paste key in the real UI → `POST /connections/credentials` verifies against `event-types` → connection shows connected in `GET /connections` → one **unseeded** `heliox tool paddle` live run. Agent-drivable (agent-browser), human fallback. | **yes** — one key from the account pool |

**L5/verify environment note:** the connect-time verify endpoint is fixed to the
**live** base (`https://api.paddle.com/event-types`), so **L5 must use a live key**
(a live account with a restricted, read-only-scoped key is sufficient and safe). L2/L4
can use a sandbox key because the anycli service routes the base URL by prefix and L4's
seed bypasses connect-time verify. If the account pool can only supply sandbox keys for
L5, the alternative is a prefix-routed verify endpoint (small capability add) — flag at
stage 1, don't discover mid-wave.

## 6. Open questions / flags for the batch lead

1. **Verify environment routing.** Fixed live verify URL vs prefix-routed verify (§5
   note). Recommend fixed-live for v1 (teammates manage live accounts; sandbox is a
   dev-only L2 concern) and record the account-pool key environment before L5.
2. **Capability reuse vs add.** Confirm whether the batch base already carries the
   Bearer-scheme verifier and fingerprint identity deriver (tally/loops, knock/paperform).
   If yes → zero integration-service growth; if no → this tool adds the same minimal
   options with tests.
3. **Command-surface breadth.** v1 wraps the resources in §1; `metrics` and the webhook
   simulator are deferred. Confirm no higher-priority resource for the teammate.

## 7. Implementation divergences from this DESIGN (recorded during build)

Verified against the official Paddle API reference while implementing; the code
follows the official paths, not the §1 draft:

- **Subscription one-time charge is `POST /subscriptions/{id}/charge` (singular)**
  and its dry run is `POST /subscriptions/{id}/charge/preview` — matching the §1
  table. (An earlier build wrongly pluralized these to `/charges` /
  `/charges/preview`; that was a bug — the official reference
  `developer.paddle.com/api-reference/subscriptions/create-one-time-charge` and
  `.../preview-subscription-charge` are singular. Now corrected in code and the
  L1 path test.) CLI verbs unchanged (`subscription charge` / `preview-charge`).
- **Subscription update preview is `POST /subscriptions/{id}/preview`** — not
  `/update/preview`. CLI verb `subscription preview-update`.
- **Paddle Billing paths carry no `/v1` segment** — the version is the
  `Paddle-Version` header (pinned to `1`), so endpoints are bare
  (`https://api.paddle.com/transactions`, not `/api/v1/transactions`). This
  confirms §2b.

**Capability-growth naming, for the batch lead (OQ2).** The worktree base
carried neither the Bearer-scheme verifier nor the fingerprint identity deriver,
so this tool added them (both with tests): `APIKeyPolicy.Scheme` (`""`/`bearer`)
threaded through generator + verifier, and a new `identity.source: fingerprint`
for `manual_api_token` (liveness GET + SHA-256 credential fingerprint, label =
provider display name). If the tally/loops (Bearer) or knock/paperform
(fingerprint) branches shipped equivalents under different names, reconcile to
one at the batch-end merge — the paddle bundle only needs `scheme: bearer` +
`identity.source: fingerprint` to exist, however they are spelled.
