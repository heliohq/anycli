# Zoho Books — per-tool design (batch scratch file)

**Tool:** Zoho Books · anycli id `zoho-books` · provider key `zoho_books` · catalog row 181 (Wave 3, Finance) · auth lane `oauth_light`.
**Branches:** anycli `tool/zoho-books` (this worktree) + Helio `tool/zoho-books`.
**Status:** design. This file is stripped by the batch lead at batch end; durable facts land in the provider bundle, the anycli definition/tests, and the heliox plugin sub-doc.

All provider facts below were verified against Zoho's official docs on 2026-07-23:

- Books API introduction / base URL / organization_id / multi-DC domains / auth header:
  `zoho.com/books/api/v3/introduction/`
- Books OAuth (scopes, grant/token, offline+consent for refresh):
  `zoho.com/books/api/v3/oauth/`
- Books response envelope (`code`/`message`/resource node, ISO 8601 dates):
  `zoho.com/books/api/v3/response/`
- Resource endpoints: `zoho.com/books/api/v3/{invoices,contacts,items,estimates,bills}/`
- Accounts-server userinfo (Bearer-accepting identity probe), OAuth protocol,
  multi-DC token host: shared with the Zoho CRM design, re-verified —
  `zoho.com/accounts/protocol/oauth/{use-access-token,multi-dc}.html`

Zoho Books is one app in the **Zoho Finance** OAuth family. Per the master plan §3 seed corrections, **Zoho Invoice was intentionally dropped** as "the same Zoho Finance OAuth app/API family as Zoho Books" — Books' `invoices`/`estimates`/`customerpayments` surface subsumes it, so this one tool covers both product intents.

## 0. Audit-verdict note (divergence log)

The 2026-07-21 OAuth audit (`docs/design/008-300-integrations-rollout-plan/oauth-audit.md`) has **no row for Zoho Books** — its scope was only the 250 tools that sat in the `api_key` lane pre-audit, and Zoho Books was `oauth_light` from the seed catalog. The lane was therefore re-verified from official docs for this design:

- **`oauth_light` is confirmed.** Client registration at `api-console.zoho.com` is fully self-serve (server-based client type; credentials issued immediately; no partner/publisher review gate — only input validation). One registered client is authorized by arbitrary Zoho accounts via a standard OAuth2 authorization-code flow. Same registration model already proven for the shipped-hidden `zoho_crm`.
- **Two real divergences, neither lane-affecting:** (a) **`organization_id` is mandatory on every data-plane call** (§1, §2.3) — a structural difference from CRM that shapes the CLI, not the auth lane; (b) **multi-DC** (§3.4) — identical to CRM, V1 is US-DC-pinned.

**Build-time divergence (recorded 2026-07-23, implementation).** The design (§2.3) assumed Books exposes a distinct invalid-token *body code* the classifier could key on (mirroring CRM's string enum `INVALID_TOKEN` vs `OAUTH_SCOPE_MISMATCH`). Verified against the official Books error table (`zoho.com/books/api/v3/errors/`), Books does **not**: **HTTP 401 = "Unauthorized (Invalid AuthToken)"** is the authoritative signal, and both an invalid token and a missing-scope/permission error surface as HTTP 401 with the generic body `code 57` ("You are not authorized to perform this operation"). So `classifyCredentialError` keys the `execution.RejectCredential` on the **HTTP 401 status alone**, not a body code. Consequence: unlike CRM, a scope-miss that reaches this path also rejects the credential (forces reconnect) rather than surviving — which is the correct remedy anyway, since a missing scope can only be granted by re-consenting, and the token gateway already refreshes proactively before expiry so a 401 reaching the CLI means re-auth is genuinely needed. The §2.4 test fixtures use the 401 shape for the rejection case and a 400/`code 57` shape for the non-rejecting permission case.

## 1. What an AI teammate does with Zoho Books → API surface

The tool wraps the **Zoho Books REST API v3** at `https://www.zohoapis.com/books/v3`. Books is accounting/receivables software; the driving teammate use cases, in priority order:

1. **Answer "did customer X pay?" / "what's outstanding?"** — List/Get Invoices (`GET /invoices`, `GET /invoices/{invoice_id}`), filtered by customer and status (e.g. `filter_by=Status.Overdue`), and List Customer Payments (`GET /customerpayments`).
2. **Look up a customer or vendor before/after a conversation** — List/Get Contacts (`GET /contacts`, `GET /contacts/{contact_id}`); customers and vendors are both contacts distinguished by `contact_type` ∈ {`customer`,`vendor`} (verified on the Contacts page).
3. **Create receivables from a conversation** — Create Invoice (`POST /invoices`) and Create Estimate (`POST /estimates`).
4. **Capture a new customer/vendor** — Create Contact (`POST /contacts`).
5. **Price/line-item lookup for building documents** — List/Get Items (`GET /items`, `GET /items/{item_id}`); item rates/descriptions are what invoice/estimate line items reference.
6. **Payables context** — List/Get Bills (`GET /bills`) and record/list Expenses (`GET/POST /expenses`).
7. **Discover the organization to operate in** — List Organizations (`GET /organizations`), the one endpoint that takes **no** `organization_id` and yields the ids every other call requires.

Explicitly **out of scope for V1** (thin value per line of code, or a separate review/side-effect surface): PDF/attachment download and document email-send (`POST /invoices/{id}/email` has a real side effect — deferred, notes below), banking/reconciliation, projects/timesheets, recurring-invoice administration, taxes/chart-of-accounts settings administration, credit/debit notes, sales/purchase orders, and webhooks (a callback surface anycli does not have). These are natural V2 subcommands under the same tool.

Books v3 contract details that shape the CLI (verified):

- **`organization_id` is required on every call except `GET /organizations`.** The introduction states it "should be sent in with every API request to identify the organization." It is a **query parameter** (`?organization_id=…`), not a header. A Books login can own multiple organizations, so the id is a per-call selector the agent must supply — it is neither in the OAuth token nor returned by the token exchange.
- **Response envelope is uniform:** `{ "code": 0, "message": "success", "<resource>": … }` where `<resource>` is the endpoint's key (`invoices` array on a list, `invoice` object on a get) plus a `page_context` object on list endpoints. **`code` is an integer** — `0` on success, non-zero on error (verified on the Response page). This differs from CRM, whose error `code` is a string enum. Dates are ISO 8601 (`2016-06-11T17:38:06-0700`).
- **Pagination/filtering** on list endpoints: `page`, `per_page` (default 200), `filter_by` (status views, e.g. `Status.Overdue`), `search_text`, `sort_column`; `page_context.has_more_page` drives the agent's paging loop. Some string fields support `_startswith`/`_contains` variants (e.g. `invoice_number_contains`).
- **Auth header:** every `zohoapis.com/books/v3` call uses `Authorization: Zoho-oauthtoken <access_token>` — the Zoho-custom scheme, **not** `Bearer` (verified on the introduction page). The Helio-side identity probe does **not** hit the Books host; it hits the accounts-server userinfo endpoint, which does accept `Bearer` (§3.3). Do not conflate the two hosts.

## 2. anycli definition & service

### 2.1 Stage-1 form decision: `service`

`cli` type fails the rubric: Zoho ships no official, non-interactive, `--json`-capable Books binary that takes an injected token (the Zoho CLI targets Catalyst/serverless dev, not Books data ops). → `service` type in `internal/tools/zohobooks/` (dashes dropped per the §3 naming rule; matches the `microsoftcalendar`/`zohocrm` precedent). 21 of 23 shipped definitions are `service`.

### 2.2 Definition `definitions/tools/zoho-books.json`

```json
{
  "name": "zoho-books",
  "type": "service",
  "description": "Zoho Books as a tool (OAuth user access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "ZOHO_BOOKS_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single binding, `<TOOL>_ACCESS_TOKEN` naming per the `zoho-crm`/`microsoft-outlook`/`bitly` precedents. **`organization_id` is deliberately NOT a credential binding** — it is a runtime `--organization-id` flag the agent supplies per call (§2.3), discovered via `org list`, not a secret from Vault. No `api_domain` binding in V1 (US-DC scoping, §3.4); the service's `BaseURL` zero-value defaults to `https://www.zohoapis.com` and stays injectable for tests and the future multi-DC field (missing credential fields resolve to empty and are skipped at injection — `internal/credential/resolve.go` + `ApplyBindings` — so adding an optional `api_domain` binding later is non-breaking).

Registration: `RegisterService("zoho-books", &zohobooks.Service{})` in `internal/tools/register.go` — **batch-end merge only** (shared surface); the definition JSON and `internal/tools/zohobooks/` merge freely mid-batch.

### 2.3 Cobra tree (resource-grouped, zohocrm-style)

```
zoho-books org list                                             # GET /organizations — NO --organization-id; discovers the ids
zoho-books contact  list   --organization-id O [--contact-type customer|vendor] [--filter-by V] [--search-text S] [--page N] [--per-page N]
zoho-books contact  get    --organization-id O --id ID
zoho-books contact  create --organization-id O --data '<json>'
zoho-books invoice  list   --organization-id O [--customer-id C] [--status S] [--filter-by V] [--search-text S] [--page N] [--per-page N]
zoho-books invoice  get    --organization-id O --id ID
zoho-books invoice  create --organization-id O --data '<json>'
zoho-books estimate list   --organization-id O [--customer-id C] [--filter-by V] [--page N] [--per-page N]
zoho-books estimate get    --organization-id O --id ID
zoho-books estimate create --organization-id O --data '<json>'
zoho-books item     list   --organization-id O [--search-text S] [--page N] [--per-page N]
zoho-books item     get    --organization-id O --id ID
zoho-books bill     list   --organization-id O [--vendor-id V] [--filter-by V] [--page N] [--per-page N]
zoho-books bill     get    --organization-id O --id ID
zoho-books payment  list   --organization-id O [--customer-id C] [--page N] [--per-page N]   # GET /customerpayments
zoho-books payment  get    --organization-id O --id ID
zoho-books expense  list   --organization-id O [--filter-by V] [--page N] [--per-page N]
zoho-books expense  get    --organization-id O --id ID
zoho-books expense  create --organization-id O --data '<json>'
```

Design points:

- **`--organization-id` is a persistent (root) flag, required on every subcommand except `org list`.** A missing/empty value on an org-scoped command is a **usage error (exit 2)** whose message tells the agent to run `zoho-books org list` first — mirroring the way CRM makes the API-mandated `--fields` a hard CLI error rather than a silent default. No "pick the default org" fallback (Hard Rules: no silent fallback). The service appends `organization_id=O` to the query string of every org-scoped call.
- **`--data`** accepts a JSON object; the service sends it as the raw request body (Books create endpoints take a flat JSON body, not a `{"data":[…]}` wrapper — this is a Books/CRM divergence). Line-item-bearing creates (invoice/estimate) put `line_items` inside `--data`; the agent composes it after an `item list`/`contact list` lookup.
- **List filters** (`--filter-by`, `--status`, `--customer-id`, `--vendor-id`, `--search-text`, `--page`, `--per-page`) map straight to Books query params; unknown filter strings are passed through verbatim so status views (`Status.Overdue`, `Status.Sent`, …) work without a client-side allowlist.
- **Output/exit contract** per design 003 §3 and the zohocrm precedent: success prints the provider's JSON response verbatim to stdout (envelope included, so the agent sees `page_context`); failure is exit 1 with a one-line stderr error carrying Zoho's `code`/`message` (typed `apiError`); usage errors exit 2; `--json` gives the structured error envelope.
- **Error surfacing — the Books nuance:** Books returns proper HTTP status codes (400/401/…) *and* a body `{code:<int>, message}`. The service classifies on HTTP status (non-2xx → `apiError`, surfacing body `code`+`message`), and **defensively also treats a 2xx carrying a non-zero body `code` as an error** — Books' `code` is the authoritative success signal. A 401 with an invalid-token body `code` is marked `execution.RejectCredential` so the engine invalidates the stored token; a permission/scope error is **not** a rejection (the token is valid, it just lacks a scope) so the credential survives and the agent gets an actionable message. This mirrors zohocrm's `classifyCredentialError`, adapted to Books' integer `code`.
- **Struct:** `Service{ BaseURL string; HC *http.Client; Out, Err io.Writer }`, zero values → production endpoint, per the built-in service conventions. Reuse the zohocrm `client.go` call/emit/error shape near-verbatim (apiPrefix `/books/v3`, env `ZOHO_BOOKS_ACCESS_TOKEN`, integer-`code` `apiMessage`).

### 2.4 anycli tests (TDD, L1)

Write tests first per anycli AGENTS.md. `httptest` fakes assert, per subcommand: URL path/method, **`organization_id` query propagation on every org-scoped call and its absence on `org list`**, the `--organization-id`-missing usage error (exit 2) on an org-scoped command, `Authorization: Zoho-oauthtoken` header injection from `ZOHO_BOOKS_ACCESS_TOKEN`, list filter/pagination param propagation (`filter_by`, `page`, `per_page`, `contact_type`, `customer_id`), `--data` raw-body passthrough on create, and both plain + `--json` error rendering of a Books error body (`{"code":57,"message":"You are not authorized to perform this operation"}`-shape and a 401 invalid-token shape). Plus: missing credential → exit 1 explicit message; the defensive 2xx-with-nonzero-`code` → exit 1 path. Never hit the real API from a unit test.

## 3. Credentials & OAuth (verified against official docs)

Zoho Books uses the **same Zoho Accounts OAuth2** as Zoho CRM — the flow below is re-verified against the Books OAuth page, and is deliberately kept congruent with the shipped `zoho_crm` bundle so the `standard_oauth` runtime strategy is reused with zero new service code.

### 3.1 Flow

Standard OAuth2 authorization-code, server-based client, self-serve at `api-console.zoho.com`:

- Authorize: `https://accounts.zoho.com/oauth/v2/auth` with `response_type=code`, `access_type=offline` (**required** to receive a refresh token — Books OAuth page: "include access_type=offline and prompt=consent"), `prompt=consent` (forces fresh refresh token on reconnect), scopes comma-separated.
- Grant code: single-use, ~2 min validity; callback carries `code`, `location`, `accounts-server`.
- Token: `POST {accounts_URL}/oauth/v2/token`, **form body** with `grant_type=authorization_code`, `client_id`, `client_secret`, `redirect_uri`, `code` → matches `token_exchange_style: form_secret` exactly.
- Response: `{access_token, refresh_token, api_domain, token_type:"Bearer", expires_in:3600}`.
- Refresh: `POST {accounts_URL}/oauth/v2/token` with `grant_type=refresh_token` — **no rotation**: a new access token only; the refresh token is permanent until revoked (max 20 refresh tokens/user). The `standard_oauth` refresh path already keeps the old refresh token.
- Revoke: `POST {accounts_URL}/oauth/v2/token/revoke?token={refresh_token}`.
- Token semantics: access token 1 h; tokens are organization- and environment-specific. A Books login with multiple organizations still gets one token; which *organization* a call targets is chosen per-call via `organization_id` (§2.3), not at consent — so one connection can address every org the user can see. Fine under `connection.mode: isolated`.

### 3.2 Scopes (least-privilege, granular — not `fullaccess.all`)

`ZohoBooks.fullaccess.all` exists but is a documented security-review red flag; V1 requests the minimal granular set matching the §1 surface (operation types are `CREATE/READ/UPDATE/DELETE/ALL`):

```
ZohoBooks.contacts.READ,  ZohoBooks.contacts.CREATE
ZohoBooks.invoices.READ,  ZohoBooks.invoices.CREATE
ZohoBooks.estimates.READ, ZohoBooks.estimates.CREATE
ZohoBooks.settings.READ                      # items live under the settings/catalog scope area
ZohoBooks.bills.READ
ZohoBooks.customerpayments.READ              # payment list/get (customerpayments.* has no split READ in older docs → fall back to .ALL if READ is rejected at L2)
ZohoBooks.expenses.READ,  ZohoBooks.expenses.CREATE
AaaServer.profile.READ                       # identity probe: accounts.zoho.com/oauth/user/info
```

Stage-1/L2 re-check: some Books scope areas historically only publish `.ALL` (no `.READ`/`.CREATE` split) — `customerpayments`, `bills`, `items`. If L2 rejects a granular scope with `OAUTH_SCOPE_MISMATCH`, widen just that area to `.ALL` and record it in the bundle's `scopes` — do **not** jump to `fullaccess.all`. The `org list` call needs no extra scope (organization listing is available to any authorized Books token).

### 3.3 Identity

Identical to the shipped `zoho_crm` design — `identity.source: userinfo` against `https://accounts.zoho.com/oauth/user/info` (accounts server, query-free):

- Why the accounts endpoint, not a Books one: the declarative resolver fetches userinfo with `Authorization: Bearer <token>` (`oauth_exchange.go` `fetchUserInfo`). Books requires the `Zoho-oauthtoken` prefix on `zohoapis.com/books/v3` calls, so no Books endpoint is Bearer-callable by the resolver; and every Books data endpoint needs `organization_id`. The accounts-server `/oauth/user/info` endpoint accepts `Bearer`, takes no query, and needs only `AaaServer.profile.READ`.
- Response: `{"First_Name":…,"Last_Name":…,"Email":"<string>","Display_Name":"<string>","ZUID":<number>}` → `stable_key: /Email`, `label_candidates: [/Display_Name, /Email]`. `ZUID` is the natural person id but a JSON **number**, and the resolver's `jsonPointerString` requires a **string**; `Email` is the only string identity field. (Generic numeric-stable-key support is being added on another row — Kit/HubSpot; not needed here, and `Email` is the more human-legible account key anyway.)
- **No adapter, no capability growth:** identity stays fully declarative — the shipped `zoho_crm` bundle already exercises this exact userinfo path, so Books needs nothing new integration-service-side. Account key is account-level (one Zoho user), not org-scoped, so the same human connecting once addresses all their Books orgs via `--organization-id`.

### 3.4 Multi-DC — the one structural OAuth divergence (V1 scoping decision)

Identical to the Zoho CRM finding (that design flagged this as "shared with Zoho Books"). Zoho runs isolated DCs (`.com`, `.eu`, `.in`, `.com.au`, `.jp`, `.ca`, `.com.cn`, `.sa`) with **DC-specific `zohoapis.<dc>/books/` hosts** and **DC-specific accounts/token hosts**; exchanging an EU code at `accounts.zoho.com` fails `invalid_client`. With the console's Multi-DC option, each DC gets its own client secret.

`standard_oauth` posts to one fixed `token_url`, serves one `oauth.client_secret`, and the service's `BaseURL` is one fixed host — full multi-DC is outside its closed capability set. Decision:

- **V1 ships US-DC-pinned**: `authorize_url`/`token_url`/identity URL/service `BaseURL` all on `.com` hosts. US-DC accounts (the test-account pool + the primary beta cohort) work end-to-end; a non-US Zoho account fails at token exchange with an explicit provider error, never a silent fallback (Hard Rules).
- **Follow-up (flag to batch lead — the SAME follow-up the zoho_crm design raised; do them together):** proper multi-DC needs (a) callback-`accounts-server`-directed token/refresh/revoke, (b) per-DC client secrets in config, and (c) delivering the token-response `api_domain` (and, for Books, letting the service target that host) as a credential field — `connection.metadata.*` credential sources are a closed reviewed enum (`provider-gen/validate.go: knownCredentialSources`). Per the provider-yaml guidance this should grow as a **generic reviewed capability shared across the Zoho family**, not a per-provider adapter. Until then, US-only is the honest scope and must be stated in the AI-facing sub-doc.

### 3.5 Config

`auth.required_config_fields: [oauth.client_id, oauth.client_secret]` — supplied by lane 1 in `config/` + the `deploy/` Helm Secret together (Config Sync rule); dev client id/secret arrive as uncommitted local `config/cloud.yaml` entries for the on-branch L4 run. Zoho app registration inputs for lane 1: server-based client; redirect URI = the standard integration-service callback; homepage = helio.im. **A single Zoho console client can carry both CRM and Books scopes**, but Books ships its **own** `zoho_books` bundle + its own config keys (`zoho_books.oauth.*`) so the two providers stay independently configurable and independently revocable — do not share `zoho_crm`'s config keys. No review gate; registration is minutes, not weeks.

## 4. Helio provider bundle plan

`integrations/providers/zoho_books/provider.yaml` (held to batch-end merge; sketch mirrors the shipped `zoho_crm` bundle):

```yaml
schema: helio.provider/v1
key: zoho_books
go_name: ZohoBooks

presentation:
  name: Zoho Books
  description_key: zoho_books
  consent_domain: accounts.zoho.com
  visible: false            # hidden-first; flip is the separate go-live change
  order: <batch-lead assigns at batch end>

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://accounts.zoho.com/oauth/v2/auth
    token_url: https://accounts.zoho.com/oauth/v2/token
    token_exchange_style: form_secret
    pkce: none
    authorize_params:
      access_type: offline   # required: yields the refresh token
      prompt: consent        # forces fresh refresh token on reconnect
    scopes: [ZohoBooks.contacts.READ, ZohoBooks.contacts.CREATE,
             ZohoBooks.invoices.READ, ZohoBooks.invoices.CREATE,
             ZohoBooks.estimates.READ, ZohoBooks.estimates.CREATE,
             ZohoBooks.settings.READ, ZohoBooks.bills.READ,
             ZohoBooks.customerpayments.READ,
             ZohoBooks.expenses.READ, ZohoBooks.expenses.CREATE,
             AaaServer.profile.READ]
    single_active_token: false
    refresh_lease: none
    revoke:
      url: https://accounts.zoho.com/oauth/v2/token/revoke
      client_auth: none
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://accounts.zoho.com/oauth/user/info   # accounts server; accepts Bearer, query-free
  stable_key: /Email                                # ZUID is a JSON number; resolver is string-only
  label_candidates: [/Display_Name, /Email]

connection:
  mode: isolated
  disconnect_mode: provider_revoke
  runtime_strategy: standard_oauth   # zero service-side Go; no adapter — reuses zoho_crm's proven path

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: zoho-books
  kind: oauth
```

Confirm at implementation time against generator validation (not docs): the `revoke` token placement matches Zoho's `?token=` expectation (the shipped `zoho_crm` bundle is the template — copy its verified block), and the exact scope enum accepted by `provider-gen/validate.go` for scope strings.

**Naming axes:** ① CLI word = `zoho-books`; ② anycli id = `zoho-books`; ③ provider key = `zoho_books`. **Grouping decision (resolving master-plan open question 2, which the zoho_crm design explicitly punted "to the zoho-books batch"):** ship **flat commands** (`heliox tool zoho-books`, `heliox tool zoho-crm`), **no `tool.group: zoho`**. Rationale: (1) `zoho_crm` already landed hidden as a flat command — introducing a `zoho` group now is a breaking command rename of an already-merged tool; (2) the design-303 group pattern (google/microsoft) groups apps that share **one OAuth app + one consent surface**; Zoho CRM and Zoho Books are **different scope families with separate client registrations and separate bundles**, so they don't share the consent that makes grouping coherent; (3) only two Zoho tools exist in the whole 298-catalog, below the bar where a group earns its keep. Record this as the final answer to open question 2 for the Zoho family so no later batch re-litigates it.

The ②↔③ pair is mechanical dash↔underscore: **one `toolToProvider` entry** (`"zoho-books": "zoho_books"`) in `helio-cli/internal/toolcred/resolver.go` at batch end — unless master-plan open question 1 (mechanical normalization in `ProviderFor`/`ToolFor`) has landed by then, in which case no entry.

**Hidden-first:** bundle lands `visible: false`; the visible flip + regen is the single go-live change after L5.

**Other batch-end riders:** icon `ui/helio-app/src/integrations/icons/zoho_books.svg` + `providerIcons.ts` append; i18n `description_key` label `zoho_books`; provider sub-doc under `agents/plugins/heliox/skills/tool/` (must state the US-DC limitation, the `org list`-first workflow, and the `--organization-id`-required rule); plugin version bump + marketplace publish; anycli pin bump. **Not committed from this branch:** provider-gen regenerated projections (run locally for validation only), `helio-cli/go.mod` `replace` pointing at this anycli worktree, and dev client id/secret in local `config/cloud.yaml` — all per master plan §2.

## 5. Test plan (five layers)

| Layer | What runs for zoho-books | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...` — `internal/tools/zohobooks/` httptest suites of §2.4 (org_id propagation, missing-org usage error, filter/pagination params, `--data` passthrough, integer-`code` error rendering plain + `--json`) | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real US-DC token> anycli zoho-books -- org list`, then `contact list`/`invoice list`/`item list --organization-id <id>`, a `contact create` + `invoice create` round-trip on a scratch org, `payment list`, `expense list` — against live `www.zohoapis.com/books/v3`. **This is where the granular-scope-vs-`.ALL` fallback (§3.2) is confirmed** | **yes** — a real US-DC Zoho Books test org + access token. Before lane 1's app exists, a Self Client grant token from `api-console.zoho.com` (self-serve, same org; grant codes live ≤ a few min — mint and use immediately) mints an equivalent token with the exact scope set |
| L3 | local `provider-gen` + `provider-gen --check` against the branch bundle (not committed); helio-cli build/tests with local uncommitted `go.mod` `replace` → this worktree; integration-service unit suite (no growth expected — `standard_oauth` + declarative userinfo already shipped for `zoho_crm`) | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` with real `access_token` **and** `refresh_token`, deliberately short `expires_at` so the first `heliox tool zoho-books -- org list` is forced through the token-gateway refresh path (Zoho refresh returns no new refresh token — verify write-back keeps the old one); then live-API success | **yes** — real token pair from lane 1's registered dev app (client id/secret in local uncommitted `config/cloud.yaml`); real seeded org/user/assistant identities |
| L5 | full `heliox tool zoho-books auth` → connect link → Zoho consent (US-DC test account) → `oauth_connected` event → one unseeded live run (`org list` then an org-scoped `invoice list`). Human-in-the-loop (lane 3), gates the visible flip. Also verify the declarative revoke against Zoho's `/token/revoke` on disconnect | **yes** — lane 1 app config landed in `config/` + `deploy/`; pool US-DC account with a human driving consent |

L1 and L3 are agent-only; **L2/L4/L5 require externally supplied credentials** (test-account pool + lane 1's dev app) as marked. Because Books reuses the shipped `zoho_crm` OAuth/identity path, no integration-service capability growth is anticipated — if L3 surprises with a validator gap, it would be shared Zoho-family work, flagged to the batch lead (§3.4).

Definition of done tracks the master plan: L1–L5 green, docs published, icon registered, then the visible flip as its own change.
