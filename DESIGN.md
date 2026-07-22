# Zuora — per-tool design (`heliox tool zuora`)

Scratch design doc for the `tool/zuora` branch (both repos). The batch lead
strips this at batch end. Catalog row 176 (Payments & Commerce, Wave 3):
`zuora` / `zuora` / `zuora`, lane `api_key`. OAuth audit row 178: *"no viable
multi-tenant path → stays api_key."* Both are **confirmed against Zuora's
official docs** below; the one thing worth recording is a divergence of
*mechanism*, not lane: Zuora's "api_key" lane is realized on the wire as a
per-tenant **OAuth 2.0 client-credentials token exchange**, and on the Helio
side as a **multi-field `manual_credentials`** connection (three inputs), not a
single opaque key.

Everything below was verified against `developer.zuora.com` and the actual repo
code on this worktree base, not inherited from the catalog.

---

## 1. Auth flow — verified, and why it stays `api_key`

**Registration model (per-tenant, not multi-tenant).** A Zuora customer creates
an **OAuth client** inside their own Zuora tenant UI (Administration Settings →
user → *OAuth Clients* → *Create*), which mints a **Client ID** + **Client
Secret** bound to that user's roles/entities. There is **no** single registered
"Helio" application that an arbitrary customer's Zuora tenant can authorize —
each customer stands up their own client and hands Helio the id/secret pair.
That is exactly the audit rubric's "OAuth is per-instance / no viable
multi-tenant authorization-code path" ⇒ **`api_key` lane holds.** No
`oauth_light`/`oauth_review` app registration, and therefore **no human lane 1
dependency** (nothing lands in `config/` or `deploy/`).

**Token exchange (official).** `POST {base_url}/oauth/token`, body
`application/x-www-form-urlencoded`:

```
grant_type=client_credentials
client_id=<id>
client_secret=<secret>
```

- No auth headers on this call (docs: "do not set Authorization,
  apiAccessKeyId, or apiSecretAccessKey").
- 200 response JSON: `access_token`, `token_type` (`bearer`), `expires_in`
  (seconds; ~3600), `jti` (per-token uuid), `scope` (space-delimited).
- Rate-limited by IP; docs: *"each token should be used until it expires."*
  ⇒ the anycli service mints **once per process invocation** and reuses the
  bearer for every REST call in that command; never one token per HTTP call.
- Subsequent REST calls: `Authorization: Bearer <access_token>`.

**Base URL is a required credential input.** Each Zuora environment/data center
has a **distinct** host and cannot be inferred from the id/secret:

| Environment | Base URL |
|---|---|
| US Production (Cloud 1 / Cloud 2) | `https://rest.na.zuora.com` / `https://rest.zuora.com` |
| US API Sandbox (Cloud 1 / Cloud 2) | `https://rest.sandbox.na.zuora.com` / `https://rest.apisandbox.zuora.com` |
| EU Production | `https://rest.eu.zuora.com` |
| EU Sandbox | `https://rest.sandbox.eu.zuora.com` |

The user reads their REST base URL from the Zuora UI and supplies it. Optional
`Zuora-Org-Ids` / `Zuora-Entity-Ids` headers (multi-org / multi-entity tenants)
are exposed as optional command flags, **not** credential fields.

**Credential fields (3):** `base_url`, `client_id`, `client_secret`. All three
are the durable secret Helio stores in Vault; the short-lived bearer is minted
at call time inside anycli and **never touches Helio's token gateway** — the
correct orthogonal split (Helio persists the durable secret; anycli mints the
ephemeral bearer).

---

## 2. API surface — driven by what an AI teammate does with Zuora

Zuora is a subscription-billing / monetization platform. An AI teammate's real
jobs: *"what's this customer's balance and open invoices,"* *"show me this
account's active subscriptions,"* *"list recent payments,"* *"what rate plans
are in the catalog,"* and arbitrary ad-hoc billing lookups. That is **read-first
plus one generic query escape hatch**; billing **writes** (create subscription,
generate invoice) are high-risk and are deliberately **out of v1** (documented
extension, gated behind explicit flags later). The wrapped surface (Zuora
Billing REST v1):

| Verb | Endpoint | Why |
|---|---|---|
| `account get --key K` | `GET /v1/accounts/{K}` | Account record (billing/sold-to, balance, currency). |
| `account summary --key K` | `GET /v1/accounts/{K}/summary` | Rolled-up subscriptions + recent invoices/payments/usage — the "one look at a customer" call. |
| `subscription list --account K` | `GET /v1/subscriptions/accounts/{K}` | All subscriptions for an account. |
| `subscription get --key S` | `GET /v1/subscriptions/{S}` | One subscription (rate plans, charges, term). |
| `invoice get --key I` | `GET /v1/invoices/{I}` | One invoice (amount, balance, status, due date). |
| `invoice list --account K` | `POST /v1/action/query` (ZOQL over `Invoice`) | Account's invoices (no first-class list-by-account GET; ZOQL is the supported filter). |
| `payment get --key P` | `GET /v1/payments/{P}` | One payment. |
| `payment list --account K` | `POST /v1/action/query` (ZOQL over `Payment`) | Account's payments. |
| `catalog products [--page N]` | `GET /v1/catalog/products` | Product catalog + rate plans; also the cheapest authenticated read for smoke checks. |
| `query --zoql "select …"` | `POST /v1/action/query` | Read-only ZOQL escape hatch over any queryable object (`Account`, `Subscription`, `Invoice`, `Payment`, `RatePlan`, …) — covers everything not first-classed above. |

Chosen because they map 1:1 to the teammate jobs above with the smallest safe
surface: three resource reads (account / subscription / invoice / payment),
the catalog, and ZOQL as the power tool. Larger async exports (Data Query
`POST /query/jobs`) are intentionally excluded from v1 — agent commands are
synchronous and short.

---

## 3. anycli definition & service

**Type: `service`** (stage-1 rubric). No official Zuora CLI exists; the surface
is HTTP + a client-credentials token exchange. All four `cli`-type conditions
fail ⇒ `service` type against the REST API (matching 21/23 shipped defs). Go
package `internal/tools/zuora/`; `RegisterService("zuora", &zuora.Service{})` in
`internal/tools/register.go` (conflict-free per-tool file merges freely; only
the `register.go` line rides the batch-end merge).

**`definitions/tools/zuora.json`** — axis ② id `zuora`, three credential
bindings injected as env vars:

```json
{
  "name": "zuora",
  "type": "service",
  "description": "Zuora Billing as a tool (OAuth 2.0 client-credentials; reads accounts, subscriptions, invoices, payments, catalog, and ZOQL queries)",
  "auth": {
    "credentials": [
      { "source": {"field": "base_url"},      "inject": {"type": "env", "env_var": "ZUORA_BASE_URL"} },
      { "source": {"field": "client_id"},     "inject": {"type": "env", "env_var": "ZUORA_CLIENT_ID"} },
      { "source": {"field": "client_secret"}, "inject": {"type": "env", "env_var": "ZUORA_CLIENT_SECRET"} }
    ]
  }
}
```

**Service behavior.** cobra tree grouped by resource (`account`, `subscription`,
`invoice`, `payment`, `catalog`) plus top-level `query`. `Service` struct
carries `BaseURL`/`HC`/`Out`/`Err` so unit tests point at an `httptest` server
(the notion/bitly shape). On `Execute`: read the three env vars → **mint the
bearer once** (`POST {base_url}/oauth/token`, form body, no auth headers) →
cache in the process → run the subcommand with `Authorization: Bearer …`.

**JSON output shape.** Every command emits the Zuora JSON body as passthrough on
stdout (+ trailing newline); no re-wrapping on success. `--json` is accepted for
uniformity (always structured).

**Error dialect — Slack-like `success:false` on 200.** Zuora's `/v1/action/query`
(and Object APIs) return **HTTP 200 with `{"success": false, "reasons":[{code,
message}]}`** on logical failure, while plain REST reads return normal non-2xx.
The service treats **both** as failures: a typed `apiError` carrying the Zuora
`reasons[]` / HTTP status, rendered as a `--json` error envelope. A 401 on the
token exchange (bad id/secret) or on a REST call is a hard credential rejection.
**Exit codes:** `0` success, `1` runtime/API failure (non-2xx or `success:false`),
`2` usage/parse. Unit tests assert form-encoded client-credentials body, Bearer
injection, `success:false` handling, and both plain + `--json` error rendering.

---

## 4. Helio provider bundle plan

**Naming (§3 — no divergence).**

| Axis | Value |
|---|---|
| ① CLI command word | `zuora` — flat, **no** `tool.group` (Payments & Commerce, no family). Renders as `heliox tool zuora`. |
| ② anycli tool id | `zuora` |
| ③ provider catalog key / dir | `zuora` |

②≡③ ⇒ **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`,
and no `toolGroups` change. Go package `internal/tools/zuora`.

**Bundle** `integrations/providers/zuora/provider.yaml`, `presentation.visible:
false` (hidden-first). Because Zuora is a manual-credential provider, the bundle
declares **no** `auth.required_config_fields` (there is no Helio-owned client
id/secret) — it declares the three-field `credential_input` and reviewed
verification metadata instead. `experiment:` empty (GA lane, not preview).

**Runtime strategy & the capability it needs.** Zuora is `AuthType: credentials`
/ `runtime_strategy: manual_credentials` with a **three-field**
`credential_input.fields` (`base_url`, `client_id`, `client_secret`). Verified
on this worktree base:

- `resolveManualSecret` (`service/manual_credential.go`) currently rejects
  `len(CredentialInput.Fields) != 1`; the near-main checkout does too. So
  **multi-field `manual_credentials` is genuine capability growth**, not present
  on the base. It is the **same growth** paypal (#564), zoominfo (#365), and
  braintree (#554) are landing in-flight — Zuora must **reuse** it, not fork a
  parallel packer. Verify it on the batch-merge base at stage 2; if it has not
  landed, coordinate with those siblings rather than duplicating.
- Verification: the merged `manual_credentials` path wires
  `dsnHostIdentityDeriver` and does **no** provider-side verification (design
  317 D5: a connection-string secret has no HTTPS identity endpoint). Zuora is
  different and **better** — it *can* be verified cheaply with one token call.
  Plan: register a narrow compiled **`zuoraTokenVerifier`** for the `zuora`
  provider (the servicenow endpoint+secret / later client-credentials /
  sprout `sproutClientVerifier` precedent) that does the client-credentials
  `POST {base_url}/oauth/token`; **200 ⇒ valid**, 401 ⇒
  `invalid_provider_credential`. Identity: `account_key = client_id` (stable per
  tenant/client), `label = host(base_url)`. This depends on the multi-field
  growth also widening the verifier interface to receive the field **map**
  (today it is `Verify(…, token string)`); that widening is part of the same
  paypal/zoominfo growth — reuse it. At stage 2, first check whether an existing
  base_url-templated client-credentials verifier can be reused before adding
  `zuoraTokenVerifier`.
- **Fallback (Option B)** if multi-field growth is *not* in this batch: pack the
  three values into a single `manual_credentials` field and accept the braze-style
  no-verify path (bad secret surfaces at first `heliox tool zuora` call). Rejected
  as the default because Zuora is genuinely verifiable and base_url routing is
  first-class, but it is the zero-growth escape hatch.

**Icon + docs (batch-end surfaces).** `ui/helio-app/src/integrations/icons/zuora.svg`
+ manual `providerIcons.ts` registration; i18n label "Zuora"; AI-facing sub-doc
under `agents/plugins/heliox/skills/tool/` (verbs above, ZOQL examples, the
three credential fields, where to create the OAuth client in the Zuora UI, and
the sandbox base URLs). Five provider-gen projections regenerate together at
batch end — **not committed on this branch** (validated locally via
`provider-gen --check` + a `helio-cli/go.mod` local `replace` at the anycli
branch; tool branches are expected to fail `provider-gen --check` in CI until
the batch lead's canonical regen).

---

## 5. Test plan → five layers

| Layer | What it proves for Zuora | Ext. creds? |
|---|---|---|
| **L1** | anycli unit tests: httptest fake serving `/oauth/token` + resource/query endpoints. Assert form-encoded `grant_type=client_credentials` body with **no** auth headers, single token mint per invocation, `Authorization: Bearer` on REST, `success:false`-on-200 → exit 1, plain + `--json` error envelopes, exit codes 0/1/2. | **No** |
| **L2** | Dev harness against a **real Zuora API Sandbox** tenant: `ANYCLI_CRED_BASE_URL=https://rest.apisandbox.zuora.com ANYCLI_CRED_CLIENT_ID=… ANYCLI_CRED_CLIENT_SECRET=… anycli zuora -- catalog products` (and an `account summary` / `query`). Mandatory before the pin bump — proves field names, form body, and REST shapes match live. | **Yes** (Zuora sandbox tenant + OAuth client; test-account pool, human lane 2) |
| **L3** | `provider-gen --check` + both repos' unit suites, **plus** the integration-service tests for the multi-field `manual_credentials` growth and `zuoraTokenVerifier` (200-valid / 401-reject / identity = client_id + host). | **No** |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` (manual/api-key providers are seedable): seed `base_url`+`client_id`+`client_secret` for a real seeded assistant/org, then `heliox tool zuora -- catalog products` reaches live Zuora through the token gateway → anycli. Seeded row is indistinguishable from a real connect. | **Yes** (same sandbox OAuth client — the seeded creds must reach live Zuora) |
| **L5** | api_key key-entry path (agent-drivable, §2 master plan): open the connect link → paste base_url/client_id/client_secret in the real connect UI → `zuoraTokenVerifier` accepts → connection shows connected/configured in `GET /connections` → **one unseeded** `heliox tool zuora` live command succeeds. Run once, still hidden, before the visible flip. | **Yes** (Zuora sandbox creds) |

**Externally supplied credentials required for L2, L4, L5** (a Zuora API Sandbox
tenant with a self-created OAuth client from the test-account pool). **L1 and L3
need none.** No OAuth app-registration lane (human lane 1) applies — Zuora is a
manual-credential provider.

## 6. Divergence log (vs catalog / audit)

- **Lane:** none — `api_key` **confirmed** against official docs (per-tenant
  client-credentials, no multi-tenant authorization-code path).
- **Mechanism nuance (recorded, not a lane change):** the `api_key` lane is
  realized as (a) an OAuth 2.0 client-credentials **token exchange performed
  inside the anycli service**, and (b) a **three-field `manual_credentials`**
  Helio connection (`base_url`, `client_id`, `client_secret`) with a compiled
  `zuoraTokenVerifier` — not a single-header static key. This is the
  paypal/zoominfo (multi-field) + servicenow/later (client-credentials verifier)
  capability lineage, reused rather than forked.
