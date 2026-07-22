# Pennylane — per-tool design (batch scratch)

Catalog row 182 · anycli id `pennylane` · provider key `pennylane` · auth
`oauth_review` · Wave 3 · Finance. Audit row 184 verdict `oauth_review` (high
confidence, evidence https://pennylane.readme.io/docs/oauth-20-walkthrough).
This scratch file is committed on branch `tool/pennylane`; the batch lead
strips it at batch-end.

All facts below were re-verified against Pennylane's official developer docs
(pennylane.readme.io, v2.0) on 2026-07-22 — see §7 for the divergence log
against the prompt/catalog. No divergence from the audit lane: `oauth_review`
holds.

---

## 1. What an AI teammate does with Pennylane (drives the surface)

Pennylane is a French accounting + finance-management platform (SMB
bookkeeping, invoicing, supplier bills, banking reconciliation) used by
companies and by accounting firms managing many client companies. An AI
teammate's realistic jobs:

- "What supplier invoices are unpaid / awaiting validation?" → read supplier
  invoices, statuses, matched transactions.
- "Draft/issue a customer invoice for €X to customer Y" → create customer
  invoices; look up customers and products.
- "List this month's uncategorized bank transactions" and "categorize this
  transaction to ledger account 6xx" → read transactions, categorize.
- "Who are our top customers/suppliers?" → list customers, suppliers.
- "Pull the trial balance / ledger entries for the quarter" → read accounting
  data (journals, ledger entries, trial balance).

These map onto the **v2 external REST API** (`/api/external/v2/…`), the
provider's own current integration surface. We are NOT wrapping v1 (deprecated
in favor of v2) and NOT touching the firm-level bulk endpoints beyond what a
single-company token grants — scoping to a Company Token keeps the identity
and credential model simple (§4).

### API surface wrapped (and why these)

Base: `https://app.pennylane.com/api/external/v2`. Bearer token in
`Authorization: Bearer <token>`. Provider-neutral JSON pass-through.

| Resource (verb group) | Endpoints (v2) | Why (teammate job) |
|---|---|---|
| `customer` | `GET /customers` (list, both types), `GET /customers/{id}` (retrieve, both types), `POST /company_customers` (create) | invoice recipients; CRM-lite lookups |
| `supplier` | `GET /suppliers`, `GET /suppliers/{id}` | AP counterpart lookups |
| `customer-invoice` | `GET /customer_invoices`, `GET /customer_invoices/{id}`, `POST /customer_invoices` | issue/track AR — the highest-value write |
| `supplier-invoice` | `GET /supplier_invoices`, `GET /supplier_invoices/{id}` | AP triage ("what's unpaid/unvalidated") |
| `product` | `GET /products`, `GET /products/{id}` | invoice line items |
| `transaction` | `GET /transactions`, `GET /transactions/{id}`, `POST /transactions/{id}/…` categorize | bank reconciliation / categorization |
| `ledger` (read) | `GET /trial_balance`, `GET /ledger_entries`, `GET /journals`, `GET /ledger_accounts` | accounting reporting |

**Customer endpoint asymmetry (verified, load-bearing):** Pennylane v2 splits
customer *reads* from customer *creates*. List (`GET /customers`) and retrieve
(`GET /customers/{id}`) both return **company and individual** customers and are
the right general-purpose read paths. Create, however, has **no** `POST
/customers` — creation is split by type into `POST /company_customers` and `POST
/individual_customers` (refs `postcompanycustomer` / `postindividualcustomer`;
the create-customer-invoice use-case doc creates via `POST /company_customers`
and keeps the returned `customer_id`). We wrap the company-customer create (the
B2B-invoicing default). This asymmetry — read via `/customers`, create via
`/company_customers` — is reflected in the surface table and the cobra tree
below so the anycli service wires the right paths. (An earlier draft mapped
create to `POST /customers`, which 404s.)

Deliberately **out of scope for v1 of the tool**: SEPA mandates,
subscriptions, FEC/AGL fiscal exports, file attachments upload — niche or
compliance-heavy, add later if demand appears. Keep the first cut to the
read-heavy + invoice-write core (Code Health: subtract before adding).

---

## 2. anycli definition

- **Type: `service`** (stage-1 rubric). No official Pennylane CLI binary
  exists; the integration is a REST API → `service` type against the v2 HTTP
  API, following the `notion`/`stripe` service precedent. (`cli` type is
  ruled out: no `--json` official binary to provision.)
- **Go package**: `internal/tools/pennylane/` (id has no dashes/leading
  digit, so package name == id == `pennylane`). Registered as
  `RegisterService("pennylane", &pennylane.Service{})` in
  `internal/tools/register.go`.
- **Definition file**: `definitions/tools/pennylane.json`.
  - `name`: `pennylane`
  - `type`: `service`
  - `description`: one line — "Pennylane accounting: customers, suppliers,
    invoices, transactions, and ledger."
  - `auth`: one `CredentialBinding` — `source.field: access_token` →
    `inject: { type: env, env_var: PENNYLANE_ACCESS_TOKEN }`. The service
    reads that env var and sets `Authorization: Bearer <token>`. (Minimal
    slack-shaped single-credential binding; the gateway owns refresh, anycli
    never sees the refresh token.)

### Subcommand tree / verbs

Cobra tree grouped by resource, copying the `notion` service shape
(`BaseURL`/`HC`/`Out`/`Err` struct so httptest can drive it, exit-code
contract 0 success / 1 API-or-runtime / 2 usage-parse, `--json` structured
error envelope):

```
pennylane customer         list [--cursor] [--filter] | get <id> | create --json <body>
#   list   → GET  /customers            (company + individual)
#   get    → GET  /customers/{id}       (company + individual)
#   create → POST /company_customers    (asymmetry: no POST /customers exists)
pennylane supplier         list | get <id>
pennylane customer-invoice list [--status] [--cursor] | get <id> | create --json <body>
pennylane supplier-invoice list [--status] [--cursor] | get <id>
pennylane product          list | get <id>
pennylane transaction      list [--cursor] | get <id> | categorize <id> --json <body>
pennylane ledger           trial-balance | entries [--cursor] | journals | accounts
```

- **Pagination**: v2 uses cursor-based pagination (`has_more` + a cursor
  token in the list envelope). `list` exposes `--cursor` and passes it
  through; the tool does NOT auto-loop all pages (agent decides, bounded
  fan-out per Code Health).
- **Output shape**: pass the provider's JSON response body straight through
  on success (single object for `get`, `{ items: [...], has_more, next_cursor }`
  shape for `list` — mirror provider field names, no re-mapping). Errors
  render as anycli's typed `apiError` → `--json` envelope `{ "error": {
  "code", "message", "status" } }`.

---

## 3. Credential fields & auth flow (oauth_review lane — verified)

### Registration model (why oauth_review)

Pennylane OAuth client credentials are **not self-serve**: client_id /
client_secret are issued only after contacting Pennylane's Partnerships team
and passing app validation (docs: "fill the form… validated by their
partnerships team… assign you a specific Client Id and Client Secret").
Credentials cannot be retrieved after creation. This is a partner-review gate
before external companies can authorize → **oauth_review** (matches audit row
184). Dev/test apps are available pre-review, so **dev-mode app creation gates
L4** and **review clearance gates only the visible flip** (master plan §2
lane 1). No divergence from the catalog.

### OAuth endpoints (official, verified)

| Field | Value |
|---|---|
| authorize_url | `https://app.pennylane.com/oauth/authorize` |
| token_url | `https://app.pennylane.com/oauth/token` |
| revoke_url | `https://app.pennylane.com/oauth/revoke` |
| grant | authorization_code (`response_type=code`) |
| PKCE | not documented → `pkce: none` |
| scope delimiter | space-separated |
| access token TTL | **24h** (`expires_in: 86400`, `token_type: Bearer`) |
| refresh token TTL | **90 days**, **Refresh Token Rotation (RTR)** |

**RTR is the load-bearing auth fact**: every refresh immediately invalidates
the old refresh token and returns a brand-new one; the old access token also
stops working. The token gateway MUST persist the rotated refresh token on
each refresh, and MUST NOT fire concurrent refreshes with the same token.
This is exactly the **Xero / QuickBooks / FreshBooks / Sage** shape already
shipped in this program — `standard_oauth` with `refresh_lease: credential`.
**No capability growth expected**; the golden `standard_oauth` path
(`standardOAuthExchanger` + declarative refresh + credential-lease writeback)
already covers it. If L1–L3 prove otherwise, that is a finding to raise, not a
silent adapter.

### Token exchange style

Standard `application/x-www-form-urlencoded` token POST carrying `grant_type`,
`code`, `redirect_uri`, `client_id`, `client_secret` in the body →
`token_exchange_style: form_secret` (LinkedIn precedent). **Confirmed from the
official OAuth 2.0 walkthrough**, whose token-exchange curl sends the credentials
as POST body form fields (`-d "client_id=…" -d "client_secret=…"`), not an HTTP
Basic `Authorization` header — so `form_secret` is settled now, not an L2 open
item. (Defensive aside only: `form_basic` is the same exchanger's fallback enum
value if a future Pennylane change moved to Basic client auth — zero service
code either way.)

### Scopes (verified, v2)

Granular `resource:readonly` / `resource:all` scopes (v2-scopes reference,
current as of Oct 2025). The bundle carries these as the **wire** `scopes:` (what
the authorize URL requests — load-bearing; a missing required scope returns 403
per the docs). They double as the consent copy, so we mirror them into
`display_scopes:` too (both are real bundle-schema fields — see §4). Cover the
wrapped surface, minimum-necessary and read-biased:

```
customers:all           # customer read + create (POST /company_customers)
products:readonly
customer_invoices:all   # AR read + create
suppliers:readonly
supplier_invoices:readonly
transactions:all        # bank read + categorize
journals:readonly       # GET /journals
ledger_entries:readonly # GET /ledger_entries
ledger_accounts:readonly# GET /ledger_accounts
trial_balance:readonly  # GET /trial_balance
```

**Ledger scoping (finding, verified):** the `ledger` subcommand wraps four
distinct endpoints, and Pennylane exposes each behind its own scope —
`journals:readonly`, `ledger_entries:readonly`, `ledger_accounts:readonly`,
`trial_balance:readonly` all exist as separate scopes. An earlier draft listed a
single suffix-less `ledger` scope; per the reference that scope is **read/write**
over "journals, ledger entries, and their attachments" and does **not** cover
`ledger_accounts` or `trial_balance`. Requesting the broad write scope would be
both under-covering (misses accounts + trial balance → 403 at L2/L5) and
over-privileged (write on a read-only surface). We therefore drop `ledger` and
request the four granular **readonly** scopes — this exactly matches the wrapped
read-only ledger surface and stays minimum-necessary.

No `offline_access` scope exists — refresh tokens are issued unconditionally
with the grant, so no offline scope needs requesting (unlike Google).

### Identity (verified — `GET /api/external/v2/me`)

An access token is bound to **a specific user AND a specific company**. The
stable connection key must be the **company id** (so re-auth of the same
company upserts, and one assistant can hold distinct connections per company).

Pennylane publicly documents the user-profile endpoint that returns exactly
this (api-overview + changelog "v2 new user profile endpoint", reference
`getme`): `GET https://app.pennylane.com/api/external/v2/me`. It **requires no
scope** — "works with any valid access token, regardless of its scopes" — so the
identity probe never 403s. Response shape:

```json
{
  "user":    { "id": 12345, "first_name": "…", "last_name": "…",
               "email": "…", "locale": "fr" },
  "company": { "id": 123456, "name": "Pennylane", "reg_no": 123456789 }
}
```

There is **no** top-level `/id` — the company id is nested at `/company/id`.
Plan: `identity.source: userinfo`, `url:
https://app.pennylane.com/api/external/v2/me`, `stable_key: /company/id`,
`label_candidates: [/company/name, /company/id]`. This is the upsert-by-company
key the whole connection model rests on; it is confirmable from public docs and
is **not** a stage-1 open item. (An earlier draft used `stable_key: /id`, which
resolves to nothing against this body and would fail identity extraction.)

### Disconnect

Pennylane exposes `POST https://app.pennylane.com/oauth/revoke` (accepts
access or refresh token, returns 200 empty body) → `disconnect_mode:
provider_revoke` with the standard_oauth declarative revoker (token param in
form body). If the revoke request shape needs a client-auth header the
declarative revoker can't express, fall back to `local_only` — but the
endpoint is a vanilla RFC 7009 revoke, so `provider_revoke` should hold.

### Credential fields (bundle → anycli)

```
credential.fields:
  access_token: token.access_token   # gateway-refreshed, injected as PENNYLANE_ACCESS_TOKEN
  account_key:  connection.account_key
```

---

## 4. Helio provider bundle plan (`integrations/providers/pennylane/provider.yaml`)

Hidden-first. Three-axis naming: ① CLI word `pennylane`, ② anycli id
`pennylane`, ③ provider key `pennylane` — **all identical**, so **no
`toolToProvider` entry**, no grouping, no resolver change. (Confirmed against
`helio-cli/internal/toolcred/resolver.go`: identity mapping holds; the map is
only for divergences.)

```yaml
schema: helio.provider/v1
key: pennylane
go_name: Pennylane

presentation:
  name: Pennylane
  description_key: pennylane
  consent_domain: pennylane.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.pennylane.com/oauth/authorize
    token_url: https://app.pennylane.com/oauth/token
    token_exchange_style: form_secret     # confirmed from OAuth walkthrough (-d body creds)
    pkce: none
    # `scopes:` = the wire scopes requested at authorize (manifest.go:87,
    # load-bearing); `display_scopes:` = the consent-copy override
    # (manifest.go:88). Same list here — Pennylane's scope strings are already
    # readable. Precedent: sage/quickbooks bundles carry both keys.
    scopes: [customers:all, products:readonly, customer_invoices:all,
             suppliers:readonly, supplier_invoices:readonly,
             transactions:all, journals:readonly, ledger_entries:readonly,
             ledger_accounts:readonly, trial_balance:readonly]
    display_scopes: [customers:all, products:readonly, customer_invoices:all,
                     suppliers:readonly, supplier_invoices:readonly,
                     transactions:all, journals:readonly, ledger_entries:readonly,
                     ledger_accounts:readonly, trial_balance:readonly]
    single_active_token: false
    refresh_lease: credential             # RTR — rotated refresh persisted

identity:
  source: userinfo
  url: https://app.pennylane.com/api/external/v2/me   # verified: no scope required
  stable_key: /company/id                             # company id (nested, not top-level /id)
  label_candidates: [/company/name, /company/id]

connection:
  mode: isolated
  disconnect_mode: provider_revoke
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
  name: pennylane
  kind: oauth
```

**Config (lane 1, both sides in one change per Config Sync):** append
`oauth.client_id` / `oauth.client_secret` for `pennylane` to integration-service
config in `config/` locally **and** the Helm Secret under `deploy/`. A provider
with *all* config fields absent renders `configured: false` (safe hidden); a
*partial* config fails startup — so id+secret land together, before Pennylane's
L5. No `experiment:` gate (GA lane, leave empty).

**UI icon**: `ui/helio-app/src/integrations/icons/pennylane.svg` + register in
`providerIcons.ts` (manual, never generated). **AI-facing docs**: provider
sub-doc under `agents/plugins/heliox/skills/tool/`, one plugin bump per batch.

**No service-side Go, no capability growth** anticipated: RTR-credential-lease
`standard_oauth` is already shipped (xero/quickbooks/freshbooks/sage). If
stage-1 finds the token exchange or identity needs a shape outside the closed
`standard_oauth` enum set, that is a reviewed capability-enum addition, not a
bespoke `adapter_*.go` — flag it before writing code.

---

## 5. Generation & shared surfaces (batch-end)

Runs at batch-end with the batch lead, not mid-branch:
- `provider-gen` + `provider-gen --check` from `go-services/integration-service`
  → the five projections (`provider_catalog.gen.go`, `providerCatalog.gen.ts`,
  `connectionProviders.gen.ts`, `toolCatalogDefaults.gen.ts`,
  `providers_gen.go`). **Not committed on this branch** — validated locally
  only (master plan §2). Branch is *expected* to fail `provider-gen --check` in
  CI until batch-end.
- anycli tag → `helio-cli/go.mod` pin bump (locally an uncommitted `replace` to
  this anycli worktree for the L4 build).

---

## 6. Test plan → five layers

| Layer | Pennylane specifics | Needs external creds? |
|---|---|---|
| **L1** anycli unit | httptest fake for `/api/external/v2/*`: assert Bearer header injection, request shape per verb, cursor pass-through, and both plain + `--json` error rendering. No real API. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<tok> anycli pennylane -- customer list`. Proves field names, injection, and real v2 request/response shape (incl. the customer read-vs-create path asymmetry and `GET /me` → `/company/id`). **Only remaining stage-1 gate: the `/oauth/revoke` request shape for `provider_revoke`** (token_exchange_style, identity endpoint, and scopes are all doc-confirmed in §3). | **Yes** — a real Pennylane access token from the test-account pool (lane 2). |
| **L3** generate + suites | `provider-gen --check` green locally; anycli `go test ./...`; `helio-cli` build with `replace` + `go test ./cmd/heliox/cmds/tool/`; integration-service unit suite (no new capability expected → no new tests beyond generation). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` for provider `pennylane` with a **real** 24h access token **and** refresh token, short `expires_at`, so the next `heliox tool pennylane -- customer list` is forced through the gateway refresh-and-writeback (exercises the RTR credential-lease path, not just token replay). Seed uses a real existing org/assistant/owner identity. | **Yes** — real access+refresh token (needs the dev-mode app from lane 1 to mint). |
| **L5** full connect | `heliox tool pennylane auth` → consent on Pennylane's **dev/test app** on a real company account → confirm `oauth_connected` event → one unseeded live `customer list`. Human-in-the-loop (oauth L5, lane 3) — French-account 2FA defeats automation. Run once, hidden, before the visible flip. | **Yes** — dev app (client_id/secret) + a real Pennylane company account with consent. |

**Credential-supplied layers: L2, L4, L5.** L1/L3 are fully agent-runnable
offline. L4/L5 both block on lane 1's dev-mode Pennylane app (partner-review
gate does NOT block them — dev apps predate review). L5 additionally blocks on
review clearance only for the *visible flip*, not for the run itself.

---

## 7. Divergence log (independent verification vs. prompt/catalog)

- **Auth lane**: catalog + audit say `oauth_review`; official docs confirm
  partner-team registration + app validation gate → **no divergence**, keep
  `oauth_review`.
- **Auth shape**: standard authorization-code with RTR (24h access / 90d
  rotating refresh). Fits shipped `standard_oauth` + `refresh_lease:
  credential` (Xero-class) → **no adapter, no capability growth** expected.
  This is a positive finding: Pennylane is NOT one of the master-plan's
  flagged non-standard-auth providers (Bill.com / NetSuite / Mastodon).
- **API version**: wrap **v2** (`/api/external/v2`), the current surface; v1 is
  deprecated. Divergence from any older reference that names v1.
- **Identity** (`GET /api/external/v2/me`): publicly documented, scope-free,
  returns `{user, company}`; `stable_key: /company/id`,
  `label_candidates: [/company/name, /company/id]`. Corrects an earlier draft
  that called the endpoint "unverifiable / behind partner login" and used
  `stable_key: /id` (which resolves to nothing). **Now verified, not an open
  item.**
- **Customer endpoint asymmetry**: list/get use `GET /customers[/id]` (company +
  individual); create uses `POST /company_customers` (there is **no** `POST
  /customers`). Divergence from a review suggestion to also route *get* to
  `/company_customers/{id}`: the general `GET /customers/{id}` **does** exist
  (ref `getcustomer`, returns both types) and is the correct read path, so we
  keep it — only *create* moves to `/company_customers`.
- **Ledger scopes**: request the four granular readonly scopes
  (`journals:readonly`, `ledger_entries:readonly`, `ledger_accounts:readonly`,
  `trial_balance:readonly`) instead of the broad read/write `ledger` scope.
  Corrects an earlier draft that under-covered (missing accounts/trial-balance →
  403) and over-privileged (write on read-only surface).
- **token_exchange_style**: `form_secret` **confirmed** from the OAuth
  walkthrough curl (credentials in `-d` body). No longer an open item;
  `form_basic` retained only as a defensive fallback note.
- **Bundle scope keys**: the §4 YAML carries both `scopes:` (wire request,
  load-bearing) and `display_scopes:` (consent copy) — both are real
  bundle-schema fields (`manifest.go:87-88`), matching the sage/quickbooks
  precedent. Not a rename of one to the other.
- **Remaining open item (L2, does not change the lane)**: revoke request shape
  for `provider_revoke` (RFC 7009 `POST /oauth/revoke`); confirm the exact
  client-auth/param form with a real token. Not a blocker to
  code-complete-hidden.

Sources: [OAuth 2.0 walkthrough](https://pennylane.readme.io/docs/oauth-20-walkthrough),
[v2 scopes](https://pennylane.readme.io/docs/v2-scopes),
[user profile `/me`](https://pennylane.readme.io/reference/getme) + [changelog](https://pennylane.readme.io/changelog/v2-new-user-profile-endpoint),
[retrieve a customer](https://pennylane.readme.io/reference/getcustomer),
[create a company customer](https://pennylane.readme.io/reference/postcompanycustomer),
[API v2 vs v1](https://pennylane.readme.io/docs/api-v2-vs-v1),
[audit row 184](../../docs/design/008-300-integrations-rollout-plan/oauth-audit.md).
