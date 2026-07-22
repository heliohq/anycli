# Tool design: Sage Accounting (`sage`)

Scratch design for the `tool/sage` branch. The batch lead strips this at
batch end. Everything below was verified against Sage's official developer
documentation (developer.sage.com — the portal blocks unauthenticated
fetching, so the endpoint/flow facts were cross-checked across multiple
independent write-ups of the official docs) and the actual repo precedents in
this worktree base, not inherited from the catalog.

## 0. Catalog row and independent verification

Master-plan §4 row 180: **Sage** — anycli id `sage`, provider key `sage`,
auth lane `oauth_light`, Wave 3, Finance.

- **Lane check (oauth_light) — CONFIRMED, independently.** Sage is *not* in
  the 2026-07-21 OAuth audit table: that audit only re-laned tools sitting in
  the pre-audit `api_key` lane, and Sage was already `oauth_light` at catalog
  creation, so there is no audit verdict to inherit or contradict. Verified
  against the official docs directly: the Sage Accounting API uses OAuth 2.0
  authorization-code grant, and app registration is fully **self-serve** at
  developer.sage.com (free developer account → Create App → client_id +
  client_secret + redirect URLs issued immediately). No partner program, no
  marketplace review, and no publish gate stands between a registered app and
  an arbitrary Sage Accounting user authorizing it. That is exactly the
  `oauth_light` rubric (self-serve registration, no review gate). Lane holds.
- **Which Sage product.** "Sage" is a family (Intacct, 50, 200, People,
  Active, Accounting). This tool wraps **Sage Accounting** (formerly Sage
  Business Cloud Accounting) — the SMB cloud-accounting product — consistent
  with the catalog's Finance categorization and the `sage` bare key. The
  separate **Sage Active** product also has a public API but uses a *different*
  OAuth flow (OAuth 2.0 **with PKCE**, for public clients); that is out of
  scope and its PKCE requirement does **not** apply here (see §3).
- **Naming axes — no divergence.** ① CLI word `sage`, ② anycli id `sage`,
  ③ provider key `sage` are identical. **No `toolToProvider` entry**, no
  `tool.group`, no resolver change. Go package `sage` (no dash/leading-digit
  normalization needed).

## 1. API surface this tool wraps, and why

Driven by what an AI finance/ops teammate actually does with a company's
books: look up customers and suppliers, read and raise sales invoices, read
supplier bills, record payments, and read the chart of accounts / bank
balances for financial context. That maps to a focused slice of the
**Sage Accounting API v3.1**.

- **Base URL:** `https://api.accounting.sage.com/v3.1`
- **Auth on every call:** `Authorization: Bearer <access_token>`
- **Business scoping:** `X-Business: <business_id>` header selects which
  business a request targets. Omitting it falls back to the user's **lead
  business** (fine for the common single-business user). `GET /businesses`
  and `GET /user` are the two endpoints that do **not** require/return the
  header — which is why identity resolution (§4) can hit `/user` before any
  business is chosen.
- **List response shape:** paginated envelope with `$items`, `$total`,
  `$page`, `$next`, `$back`, `$itemsPerPage`; query params `page` and
  `items_per_page`. POSTs accept an idempotency key for safe retries.
- **Rate limits:** ~100 req/min and ~2,500 req/day **per business** — low;
  the service surfaces `429` as a retryable runtime error (exit 1) rather
  than silently looping.

Endpoints wrapped (high-value core, read-first with targeted writes):

| Resource | Method + path | Why an AI teammate needs it |
|---|---|---|
| Businesses | `GET /businesses`, `GET /businesses/lead` | Discover `X-Business` ids to scope every other call |
| User (identity) | `GET /user` | Connection identity anchor (§4) |
| Contacts | `GET /contacts`, `GET /contacts/{key}`, `POST /contacts` | Customers & suppliers lookup/create |
| Sales invoices | `GET /sales_invoices`, `GET /sales_invoices/{key}`, `POST /sales_invoices` | Read/raise customer invoices |
| Purchase invoices | `GET /purchase_invoices`, `GET /purchase_invoices/{key}` | Read supplier bills |
| Purchase payments | `POST /purchase_invoices/{key}/payments` | Record a bill payment |
| Ledger accounts | `GET /ledger_accounts` | Chart of accounts for coding lines |
| Bank accounts | `GET /bank_accounts`, `GET /bank_accounts/{key}` | Balances / cash context |
| Products & services | `GET /products`, `GET /services` | Line-item catalog |
| Tax rates | `GET /tax_rates` | Correct tax on new invoices |
| Escape hatch | generic `fetch --method --path [--body]` | Reach the ~40 other v3.1 resources without modeling each |

Writes are deliberately narrow (contacts, sales invoices, purchase payments)
— an accounting book is high-stakes, so the default surface is read-heavy and
every write is an explicit verb, never implicit.

## 2. anycli definition

- **Type: `service`** (per stage-1 rubric). No official non-interactive
  `--json` Sage CLI exists to wrap as `cli` type; the integration is the
  REST API. Implementation in `internal/tools/sage/`, registered as
  `RegisterService("sage", &sage.Service{})` in `internal/tools/register.go`.
- **`definitions/tools/sage.json`:**
  ```json
  {
    "name": "sage",
    "type": "service",
    "description": "Sage Accounting (Business Cloud) as a tool (OAuth token)",
    "auth": {
      "credentials": [
        {
          "source": {"field": "access_token"},
          "inject": {"type": "env", "env_var": "SAGE_ACCESS_TOKEN"}
        }
      ]
    }
  }
  ```
  Single credential field `access_token` (the resolver-supplied field name),
  injected as `SAGE_ACCESS_TOKEN`. The service reads it and sends the Bearer
  header. This mirrors `notion.json` exactly — OAuth complexity lives entirely
  on the Helio side; anycli only receives a ready access token.
- **Cobra tree** (copy the `internal/tools/notion/` shape: resource-grouped
  subcommands, `BaseURL`/`HC`/`Out`/`Err` struct so tests point at
  `httptest`, exit codes 0 success / 1 runtime+API failure via typed
  `apiError` / 2 usage-parse, `--json` structured error envelope):
  - `business list` / `business lead`
  - `contact list|get|create`
  - `sales-invoice list|get|create`
  - `purchase-invoice list|get`
  - `purchase-invoice pay` (POST payment)
  - `ledger-account list`
  - `bank-account list|get`
  - `product list` / `service list`
  - `tax-rate list`
  - top-level `fetch` (generic passthrough)
- **Global flag `--business <id>`** on every subcommand → sets the
  `X-Business` header when present; omitted → Sage lead-business default.
  `business list` exists precisely so the AI can discover the id to pass.
- **Pagination flags** `--page` / `--items-per-page` → `page` /
  `items_per_page` query params; list output surfaces `$items` (and `$next`
  for continuation) in the `--json` envelope.
- **JSON output:** provider-neutral. `--json` emits the parsed resource JSON
  (or `{items, total, page, next}` for lists) on success and the typed error
  envelope on failure; plain-text default renders a compact human summary.

## 3. Credentials and the exact OAuth flow

Standard OAuth 2.0 **authorization-code** grant, confidential client. There is
**no** client-credentials / S2S flow — every connection requires an initial
interactive user authorization (this is why L5 is human-in-the-loop, §6).

- **Authorize URL:** `https://www.sageone.com/oauth2/auth/central`
  params `response_type=code`, `client_id`, `redirect_uri`, `scope=full_access`,
  `state`.
- **Token URL:** `https://oauth.accounting.sage.com/token`
  form-encoded `grant_type=authorization_code`, `client_id`, `client_secret`,
  `code`, `redirect_uri` (server-to-server; secret never in the browser).
- **Scope:** single `full_access` scope (Sage does not offer granular scopes
  on this API; `full_access` is the documented value).
- **PKCE: none.** Confidential server-side client with a client_secret; the
  Accounting API does not require PKCE. (PKCE belongs to the separate Sage
  Active Public API V2 for public clients — recorded here so a future author
  does not copy Sage Active's PKCE into this bundle.)
- **Token semantics — the operationally load-bearing part:**
  - Access token TTL = **300s (5 minutes)** (`expires_in: 300`).
  - Refresh token **rotates on every use**: each refresh returns a *new*
    refresh token and invalidates the old one. Refresh token expires after
    **31 days of inactivity** (`refresh_token_expires_in: 2678400`).
  - Consequence: the token gateway MUST (a) capture `expires_at` from
    `expires_in` so it refreshes proactively before the 5-minute window, and
    (b) **persist the rotated refresh token on every refresh** (write-back).
    Dropping a rotated refresh token strands the connection with no recovery
    but full re-consent.

### Helio provider bundle plan (`integrations/providers/sage/provider.yaml`)

Hidden-first (`presentation.visible: false`). `standard_oauth` runtime
strategy — the flow is fully within the standard capability set (authorize +
`form_secret` token exchange + userinfo identity + refresh), so **no compiled
`service/adapter_*.go`**. Shape (modeled on the shipped `gmail` bundle, which
is the closest precedent: `form_secret`, refresh, `userinfo` identity,
`owner: individual`):

```yaml
schema: helio.provider/v1
key: sage
go_name: Sage
presentation:
  name: Sage Accounting
  description_key: sage
  consent_domain: sageone.com
  visible: false
  order: <batch-assigned>
auth:
  type: oauth
  owner: individual                 # Sage authorizes a Sage ID (a person)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.sageone.com/oauth2/auth/central
    token_url: https://oauth.accounting.sage.com/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [full_access]
    display_scopes: [full_access]
    single_active_token: false
    refresh_lease: <rotating-refresh value>   # see capability note below
identity:
  source: userinfo
  url: https://api.accounting.sage.com/v3.1/user
  stable_key: /id
  label_candidates: [/email, /display_name, /id]
connection:
  mode: isolated
  disconnect_mode: local_only       # no documented OAuth revoke endpoint (verify L2)
  runtime_strategy: standard_oauth
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: sage
  kind: oauth
```

- **Axes ①②③** all resolve to `sage` (bundle dir `sage/`, `key: sage`,
  `tool.name: sage`) — no `tool.group`, no `toolToProvider` divergence entry.
- **Config landing (lane 1):** `oauth.client_id` / `oauth.client_secret` for
  the registered Sage dev app land in integration-service config — `config/`
  **and** the Helm Secret under `deploy/` together (Config Sync hard rule),
  both fields in the same change (a partially configured provider fails
  service startup). Not committed in this branch; lane 1 distributes the dev
  app's id/secret as uncommitted local `config/cloud.yaml` entries for the
  on-branch L4 run.
- **UI icon:** `ui/helio-app/src/integrations/icons/sage.svg` + manual
  `providerIcons.ts` registration (never generated). **AI-facing doc:** a
  `sage` sub-doc under `agents/plugins/heliox/skills/tool/`, riding the
  batch-end plugin version bump.

### Integration-service capability note (verify on worktree base)

The one non-boilerplate risk is the **rotating refresh token + 5-minute TTL**.
The shipped `gmail` bundle uses `refresh_lease: none` because Google's refresh
token does **not** rotate; Sage's **does**. Recent rotating-refresh /
short-TTL providers (keap, signnow, hootsuite, square) each required a small
`standard_oauth` capability growth — adding the provider to the
`refresh_lease` allowed-set and/or ensuring `expires_at` capture from
`expires_in`. So at implementation time, on the worktree base:
1. Confirm the exact `refresh_lease` enum value that enables **write-back of
   the rotated refresh token**, and add `sage` to that allowed-set if it is
   gated (mirroring the keap/signnow/square precedents).
2. Confirm the token exchange captures `expires_at` from `expires_in: 300`.
Grow the capability with a reviewed enum value + test only if the existing set
does not already cover it — do not fork a Sage-specific adapter for it.

## 4. Identity

`identity.source: userinfo` against `GET /user` (which needs no `X-Business`
header, so it resolves cleanly at connect time before any business is
selected). `stable_key: /id`; `label_candidates: [/email, /display_name, /id]`.
Exact JSON pointers confirmed at L2 against a real `/user` response. The
connection is per-authorizing-user (`owner: individual`); multi-business is a
per-invocation `--business`/`X-Business` concern, **not** a per-connection
identity axis — one Sage connection can address all businesses the user can
access.

## 5. Test plan — five layers

| Layer | What it proves for Sage | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes for `/businesses`, `/user`, `/contacts`, `/sales_invoices`, error envelope; assert Bearer header, `X-Business` injection when `--business` set + omission when not, pagination params, `--json` vs plain-text, exit codes 0/1/2 | **No** |
| **L2** `anycli sage -- …` harness vs REAL api.accounting.sage.com | `ANYCLI_CRED_ACCESS_TOKEN=<fresh token> anycli sage -- business list` then `contact list`; confirms field name, Bearer injection, and real v3.1 request/response shapes (incl. `/user` pointers, `$items` envelope). Access tokens live 5 min → mint fresh immediately before running | **Yes** — real Sage Accounting access token from a test business (account pool + dev app) |
| **L3** `provider-gen --check` + both repos' unit suites | bundle strict-decode, closed field/enum contract, directory-key equality, HTTPS auth/setup URLs, helio-cli generated CLI test | **No** |
| **L4** singleton + seed endpoint + `heliox tool sage -- …` | `POST /internal/test-only/connections/seed` with `access_token` + `refresh_token` + short `expires_at` from a dedicated Sage test account; run `heliox tool sage -- business list`. The short TTL **forces** the token-gateway refresh-and-write-back path (A3) — the point that most needs proving given rotation | **Yes** — real seedable Sage token + dev app client id/secret in local uncommitted `config/cloud.yaml` for the refresh path (lane 1 output) |
| **L5** full `tool auth` → connect → consent → run (once, pre-visible-flip) | `heliox tool sage auth` → real Sage ID login + `full_access` consent on the dev app → `oauth_connected` system event → unseeded `heliox tool sage -- business list` through the freshly created connection | **Yes** — real Sage account + registered dev app; **human-in-the-loop** (oauth lane, human lane 3) |

External-credential layers: **L2, L4, L5**. Credential-free: **L1, L3**.

## 6. Rollout

Deploy hidden (`visible: false`), land the anycli pin, run L1–L4 while hidden,
run the one human L5 consent, then flip `presentation.visible: true` +
regenerate as the single go-live change. Nothing here needs a compiled
adapter; the only integration-service touch is the possible `refresh_lease`
allowed-set / `expires_at` capability check in §3, verified and grown (if
needed) at implementation time. Divergences recorded above vs. inherited
assumptions: (a) no audit row for Sage — lane verified independently;
(b) PKCE explicitly `none` (Sage Active's PKCE does not apply);
(c) rotating refresh token is the design-critical behavior and the only likely
capability-growth point.
