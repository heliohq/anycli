# Lemon Squeezy — `heliox tool` provider design

**Catalog row:** #173 · anycli id `lemon-squeezy` · provider key `lemon_squeezy` ·
auth lane `api_key` · Wave 3 · category Payments & Commerce.

**Branch:** `tool/lemon-squeezy` (both repos). **Type:** `service` (no official CLI).
**Rollout:** hidden-first (`presentation.visible: false`).

Scratch design file for the batch; the batch lead strips it at batch-end.

---

## 1. Auth-lane verification (official docs vs catalog/audit)

Catalog says `api_key`; the 2026-07-21 OAuth audit (row 175) verdict is **"no
viable multi-tenant path — stays api_key."** Confirmed against the official docs.

- Lemon Squeezy has **no third-party OAuth2 authorization-code flow at all**.
  The only credential is a personal **API key** minted by the account owner in
  the dashboard (**Settings → API → create API key**). There is no app-registration
  model, no `authorize`/`token` endpoints, no client id/secret, and no consent
  screen for external accounts. A shared Helio OAuth client is therefore
  impossible.
- Auth is **Bearer** on every request: `Authorization: Bearer <api_key>`.
- **Token semantics:** a generated key is valid for **~1 year** and is revocable
  from the dashboard; there is no refresh cycle (non-expiring within its window,
  from the token-gateway's point of view). Test-mode keys exist for the sandbox
  store but ride the same header and base URL.
- **No scopes.** A key grants full account access; there is no per-key scoping.

**Verdict: `api_key` is correct.** No divergence from the audit. This is a
manual-token (user pastes a key) provider, like the other Payments/Commerce
api_key tools in the wave (Braintree, Adyen, Paddle, Recurly, Zuora).

---

## 2. Official API surface this tool wraps

Base URL **`https://api.lemonsqueezy.com/v1/`** (major version in the path). The
API is **JSON:API** (`https://jsonapi.org`) and **requires** these headers on
every request:

```
Accept:        application/vnd.api+json
Content-Type:  application/vnd.api+json
Authorization: Bearer <api_key>
```

Rate limit: **300 requests/min** on the main API; a `429 Too Many Requests` on
exceed, with rate-limit headers on success. (The separate **License API** —
`/v1/licenses/*`, rate-limited to 60/min, keyed by license key not the API key —
is **out of scope**: it is a distinct customer-facing surface, not account
management.)

### Why these endpoints (driven by what an AI teammate does with Lemon Squeezy)

An AI teammate operating a founder/indie SaaS account on Lemon Squeezy needs to
**read the money picture and take routine merchant actions**: check sales and
orders, look up a customer and their subscriptions, issue a refund, spin up a
checkout link for a deal, mint/adjust a discount, inspect license keys, and read
subscription/usage state for billing questions. That maps onto the JSON:API
resource set:

| Resource group | Endpoints (verbs) | AI use |
|---|---|---|
| **user** | `GET /users/me` | identity (also the connect verifier — §4) |
| **stores** | `GET /stores`, `GET /stores/{id}` | pick/inspect the store to scope other calls |
| **products / variants / prices** | `GET /products[/{id}]`, `GET /variants[/{id}]`, `GET /prices[/{id}]` | catalog lookup, pricing questions |
| **files** | `GET /files[/{id}]` | digital-download asset listing |
| **orders / order-items** | `GET /orders[/{id}]`, `GET /order-items[/{id}]`, `POST /orders/{id}/refund` (issue refund), `GET /orders/{id}/generate-invoice` | revenue reporting, refunds, invoices |
| **customers** | `GET /customers[/{id}]`, `POST /customers`, `PATCH /customers/{id}` (create/update/archive) | CRM-style customer ops |
| **subscriptions** | `GET /subscriptions[/{id}]`, `PATCH /subscriptions/{id}` (update/pause/change plan), `DELETE /subscriptions/{id}` (cancel) | churn/dunning ops, plan changes |
| **subscription-invoices** | `GET /subscription-invoices[/{id}]`, `POST .../refund`, `GET .../generate-invoice` | billing history, refunds |
| **subscription-items / usage-records** | `GET /subscription-items[/{id}]`, `PATCH /subscription-items/{id}`, `GET .../current-usage`, `GET /usage-records[/{id}]`, `POST /usage-records` | usage-based billing |
| **discounts** | `GET /discounts[/{id}]`, `POST /discounts`, `DELETE /discounts/{id}` | promo/coupon management |
| **license-keys / instances** | `GET /license-keys[/{id}]`, `PATCH /license-keys/{id}`, `GET /license-key-instances[/{id}]` | license lookup/enable-disable |
| **checkouts** | `GET /checkouts[/{id}]`, `POST /checkouts` | generate a custom checkout link for a lead/deal |
| **webhooks** | `GET /webhooks[/{id}]`, `POST/PATCH/DELETE /webhooks` | integration plumbing |

JSON:API paging is `?page[number]=&page[size]=`; filtering is
`?filter[<field>]=`; relationship expansion is `?include=`. The service exposes
these as flat flags (`--page`, `--page-size`, `--filter key=value`, `--include`)
so the AI never hand-builds bracketed query strings.

---

## 3. anycli definition (stage-1 rubric → `service` type)

**Tool form: `service`.** The `cli` type is only for wrapping an official,
non-interactive, `--json`-capable, image-provisionable binary. Lemon Squeezy
ships no official CLI (the `@lemonsqueezy/lemonsqueezy.js` SDK is a browser-unsafe
JS library, not a binary). So implement `service` type against the HTTP API,
matching the 21-of-23 precedent and every api_key tool in this program.

### 3.1 Definition JSON — `definitions/tools/lemon-squeezy.json`

```json
{
  "name": "lemon-squeezy",
  "type": "service",
  "description": "Lemon Squeezy as a tool (API key) — stores, products, orders, subscriptions, customers, discounts, license keys, checkouts",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "LEMONSQUEEZY_API_KEY"}
      }
    ]
  }
}
```

- `source.field = access_token` — the resolver-supplied field name. It is
  `access_token` (not `api_key`) because the Helio token gateway projects the
  stored manual token through `token.access_token` (§4); anycli only cares that
  the field name matches what the host injects. (mongodb sets this precedent:
  the pasted secret lands in `access_token`.)
- Injected as **env** `LEMONSQUEEZY_API_KEY`, never an arg (keeps the secret out
  of the process argv/`ps`).

### 3.2 Service implementation — `internal/tools/lemonsqueezy/`

Go package name **`lemonsqueezy`** (dashes dropped per naming rule; leading
non-digit so no normalization). Registered in `internal/tools/register.go`:
`RegisterService("lemon-squeezy", &lemonsqueezy.Service{})` (the registration
string carries the exact dashed id).

Copy the **notion** service shape (the reference impl):

- `Service{ BaseURL, HC *http.Client, Out, Err io.Writer }` so unit tests point
  `BaseURL` at an `httptest.Server` and capture stdout/stderr.
- `client.go`: builds requests against `DefaultBaseURL =
  "https://api.lemonsqueezy.com/v1"`, and on **every** request sets the three
  required headers: `Authorization: Bearer <LEMONSQUEEZY_API_KEY>`,
  `Accept: application/vnd.api+json`, `Content-Type: application/vnd.api+json`.
  **Sending `application/vnd.api+json` (not plain `application/json`) is
  load-bearing** — a JSON:API server may answer `application/json` requests with
  `406 Not Acceptable`.
- cobra tree grouped by resource, mirroring §2:
  `store list|get`, `product list|get`, `variant list|get`, `price list|get`,
  `file list|get`, `order list|get|refund|invoice`,
  `customer list|get|create|update|archive`,
  `subscription list|get|update|cancel`,
  `subscription-invoice list|get|refund|invoice`,
  `subscription-item list|get|update|current-usage`,
  `usage-record list|get|create`,
  `discount list|get|create|delete`,
  `license-key list|get|update`, `license-key-instance list|get`,
  `checkout list|get|create`, `webhook list|get|create|update|delete`,
  plus top-level `whoami` (`GET /users/me`).
- Shared list flags: `--page`, `--page-size`, `--filter k=v` (repeatable →
  `filter[k]=v`), `--include a,b`.
- **JSON output shape:** pass the provider's JSON:API document through as `--json`
  (default on, agent-friendly), i.e. the `{data, meta, links, included}` envelope
  verbatim — do not re-shape it. On error, the notion `--json` error envelope:
  `{"error": {"message", "status", "code"}}`.
- **Exit codes** (notion contract): `0` success; `1` runtime/API failure (a typed
  `apiError` carrying the provider's JSON:API `errors[]` — Lemon Squeezy returns
  `{"errors":[{"status","title","detail"}]}`); `2` usage/parse error.

### 3.3 TDD (L1)

Per anycli AGENTS.md (tests first): `httptest.Server` fakes for each resource
group asserting **request shape** (method, path, `?page[...]`/`?filter[...]`
query, JSON:API request body for create/update), **injected headers** (Bearer +
both `vnd.api+json` headers), and **both** plain-text and `--json` rendering of a
`{"errors":[...]}` response. Never hit the real API from a unit test.

---

## 4. Credential fields & exact auth flow (Helio side)

**No OAuth.** The user pastes their API key through the connect drawer; it is
stored in Vault via the write-only `POST /connections/credentials` path and
served by the token gateway. This is the **manual-token, verify-first** shape
(`AuthAPIKey` / `RuntimeStrategyManualAPIToken`) — unlike mongodb (`credentials`
/ `manual_credentials`, no-verify), Lemon Squeezy **has** an HTTPS identity
endpoint, so we verify the key before it reaches Vault and derive a human
account identity.

- **Verifier:** `GET https://api.lemonsqueezy.com/v1/users/me` with
  `Authorization: Bearer <token>`. `200` → connected; `401` → rejected before any
  Vault write.
- **Identity mapping** (JSON:API response
  `{"data":{"type":"users","id":"1","attributes":{"name":"…","email":"…"}}}`):
  - `identity.stable_key = /data/id` — JSON:API ids are always **strings**, so no
    numeric-coercion capability is needed (the hubspot/kit numeric-key growth
    does not apply here).
  - `identity.label_candidates = [/data/attributes/name, /data/attributes/email]`
    — human-readable account label, never a hash.

### 4.1 Capability check — the two verifier divergences (flag at stage 1)

The base `declarativeManualTokenVerifier` (`service/manual_token_verifier.go`)
does two things that **do not fit Lemon Squeezy out of the box**:

1. It sets the identity header value to the **raw token**
   (`req.Header.Set(APIKey.Header, token)`). Lemon Squeezy needs the **`Bearer `
   scheme prefix** on `Authorization`.
2. It hard-codes `Accept: application/json`. Lemon Squeezy's JSON:API endpoint
   needs **`Accept: application/vnd.api+json`** (risk of `406`).

Both are already-solved shapes in this program: the **Bearer-scheme manual-token
verifier** capability was added by the Tally/Loops/SendGrid tools (a reviewed
verifier that prepends `Bearer ` and can set the provider's required `Accept`).
Plan of record:

- **Reuse** the existing Bearer-scheme manual-token verifier if the branch base
  already carries it (Wave 1/2 landed it). Confirm at stage 1 whether that
  verifier also lets the bundle pin the `Accept` media type; if it hard-codes
  `application/json`, grow it (Option A, narrow) to send the bundle-declared
  `Accept` — Lemon Squeezy is the first `vnd.api+json` manual-token verifier, so
  this may be a one-enum capability addition, registered by provider key in
  `service/provider_registry.go`. **Never** an unbounded YAML expression.
- The `APIKeyPolicy` gains only reviewed, closed fields (a `scheme`/`accept`
  selector already present or a single new enum value) — no per-provider Go
  branch beyond registry wiring + a unit test.

If stage-1 verification of the live `/users/me` shows it tolerates
`Accept: application/json` and a raw `Authorization` value, the plain
`declarativeManualTokenVerifier` suffices and no capability growth is needed —
**verify against the real endpoint before deciding** (L2).

### 4.2 Token-gateway projection

Non-expiring within its window → seed/serve `access_token` only, no
`refresh_token`/`expires_at` (the Slack-bot-token class in
`references/integration-testing.md`). Projection is the standard manual-token
map — **zero token-gateway code**:

```
credential:
  fields:
    access_token: token.access_token
    account_key:  connection.account_key
```

---

## 5. Helio provider bundle plan (`integrations/providers/lemon_squeezy/provider.yaml`)

Hidden-first. Three naming axes:

- **① CLI command word:** `lemon-squeezy` — flat (no `tool.group`; not a family).
  Rendered as `heliox tool lemon-squeezy -- …`.
- **② anycli tool id:** `lemon-squeezy` (= `tool.name`).
- **③ provider catalog key / directory:** `lemon_squeezy`.

②↔③ diverge only by the mechanical dash↔underscore. Per master-plan §3 this is
one of the 23 mechanical pairs. Add `toolToProvider["lemon-squeezy"] =
"lemon_squeezy"` in `helio-cli/internal/toolcred/resolver.go` **unless** open
question 1 (mechanical `ProviderFor` normalization) has landed before this
branch — if it has, add **no** entry (the auto dash→underscore covers it) and
note that in the PR. Decide off the resolver contract at branch start; do not
guess.

Bundle sketch (manual-token / api_key shape, modeled on the verify-first
precedents, contrasted with mongodb's no-verify):

```yaml
schema: helio.provider/v1
key: lemon_squeezy
go_name: LemonSqueezy

presentation:
  name: Lemon Squeezy
  description_key: lemon_squeezy
  consent_domain: lemonsqueezy.com
  visible: false            # hidden-first; flip is the single go-live change

auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization          # + Bearer scheme via the reviewed verifier (§4.1)
    setup_url: https://app.lemonsqueezy.com/settings/api

identity:
  source: userinfo
  url: https://api.lemonsqueezy.com/v1/users/me
  stable_key: /data/id
  label_candidates: [/data/attributes/name, /data/attributes/email]

connection:
  mode: isolated
  disconnect_mode: local_only      # no provider-side token-revoke endpoint
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key:  connection.account_key

tool:
  name: lemon-squeezy
  kind: api-key
```

- **No `required_config_fields`, no `config/` + `deploy/` appends.** Manual-token
  providers carry no Helio-side client id/secret — configuration is the compiled
  verifier contract. This tool is **not** in the master-plan §2 "seventh shared
  surface" (oauth config) set; it skips human lane 1 entirely.
- **Config Sync hard rule is N/A** here (nothing to sync), which is the point of
  the api_key lane running first/cheapest.
- **Generation:** from `go-services/integration-service`, `go run
  ./cmd/provider-gen` then `--check`; the five projections
  (`provider_catalog.gen.go`, `providerCatalog.gen.ts`,
  `connectionProviders.gen.ts`, `toolCatalogDefaults.gen.ts`,
  `providers_gen.go`) commit together **at batch-end only** — run locally for L3
  validation, never commit the regen on this branch (master-plan §2).
- **Icon (manual, never generated):** `ui/helio-app/src/integrations/icons/lemon_squeezy.svg`
  + register in `providerIcons.ts`. i18n: `tools.desc.lemon_squeezy` (+
  `label`/`setup` keys) across locales.
- **AI-facing docs:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/`; plugin version bump + marketplace publish
  ride the batch-end merge.

---

## 6. Test plan — the five layers

| Layer | What runs | External creds? |
|---|---|---|
| **L1** | anycli `go test ./...` — definition load + `lemonsqueezy` service unit tests against `httptest` fakes (request shape, three injected headers, JSON:API `{errors:[]}` rendering both plain + `--json`) | No |
| **L2** | `LEMONSQUEEZY_API_KEY=<real key> anycli lemon-squeezy -- whoami` and one read per major group (`store list`, `order list`, `subscription list`) against the **real** api.lemonsqueezy.com; a mutating happy-path (`checkout create` or `discount create`+`delete`) in a **test-mode** store | **Yes** — a real Lemon Squeezy account API key (test-mode store) from the account pool |
| **L3** | `provider-gen --check` (bundle strict-decode + closed-contract + directory-key equality) + `helio-cli`/`integration-service` unit suites, incl. the `toolToProvider` (or normalization) test and the Bearer/`vnd.api+json` verifier unit test | No |
| **L4** | singleton + `POST /internal/test-only/connections/seed` with `access_token` = real test key, then `heliox tool lemon-squeezy -- whoami` through the real token gateway (helio-cli built with an uncommitted `go.mod replace` at the anycli branch) | **Yes** — same real key (seeded); real Helio identities |
| **L5** | one full connect: open the connect link → paste the API key in the real drawer (stored via `POST /connections/credentials`, verified against `/users/me`) → connection shows connected in `GET /connections` → one **unseeded** live `whoami`/`store list` through the token gateway. api_key L5 is **agent-drivable** (agent-browser through the real connect UI; human lane 3 fallback on UI breakage) | **Yes** — real key + the account-pool account |

**Layers needing externally supplied credentials: L2, L4, L5** (all need one real
Lemon Squeezy account API key from the test-account pool; a test-mode store key
is sufficient and preferred for the mutating checks). L1 and L3 are hermetic.

**Done** = all five green, docs published, icon registered, then the single
go-live change: `presentation.visible: true` + regenerate. api_key L5 is a
per-batch agent-driven sweep after the batch-end merge; there is no review clock
(no OAuth app), so nothing gates the flip except L5 itself.

---

## 7. Risks / notes

- **JSON:API Accept header** is the one real integration gotcha — validate at L2
  whether `/users/me` and the data endpoints tolerate the verifier's/`anycli`'s
  media type, and grow the verifier's `Accept` only if the live endpoint demands
  `vnd.api+json` (it almost certainly does). Fail fast; no silent `application/json`
  fallback.
- **License API is out of scope** — it is key-per-license, not account-key auth;
  do not wire `/v1/licenses/*` into this tool.
- **No provider-side revoke** — `disconnect_mode: local_only`; Helio just deletes
  the Vault credential. Key revocation stays a dashboard action.
- **Rate limit 300/min** — the service should surface `429` as a typed exit-1
  error with the provider's retry hint, not silently retry-loop.
