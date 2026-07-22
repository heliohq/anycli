# Tool design: Xero

Scratch design for the `xero` tool provider (catalog row 38, Wave 1, Payments &
Commerce). Committed on branch `tool/xero`; the batch lead strips it at
batch-end. Everything below was verified against Xero's official developer
documentation and the actual repo code on the worktree bases, not inherited
from the prompt or catalog.

## 0. Naming axes (master plan §3)

| Axis | Value | Notes |
|---|---|---|
| ① CLI command word | `xero` | flat command, no `tool.group` — Xero is a standalone brand, not a family |
| ② anycli tool id | `xero` | `definitions/tools/xero.json`; Go pkg `internal/tools/xero/` |
| ③ provider catalog key | `xero` | bundle dir `integrations/providers/xero/` |

All three are identical. Xero is **not** a dashed id, so it needs **no**
`toolToProvider` divergence entry in `helio-cli/internal/toolcred/resolver.go`
(the plan's 24-entry divergence budget does not include it).

## 1. Auth lane verification (catalog says `oauth_review`)

**Verdict: confirmed `oauth_review`.** Xero exposes a genuine multi-tenant
authorization-code OAuth2 flow (one registered app that arbitrary Xero
customers authorize), which per the audit rubric would normally be an OAuth
lane. It is `oauth_review` rather than `oauth_light` because of a **hard
platform gate, not a slow review**: an **uncertified** Xero app can connect to
a maximum of **25 tenants (organisations)**, and each Xero organisation may
connect at most **2 uncertified apps**. Removing the 25-cap requires becoming a
**Xero App Partner** (certification: onboard ≥3 active customer connections in a
30-day review window, plus a commercial agreement with a 15% App Store revenue
share). This matches the master plan's stated reason ("uncertified apps cap at
25 connections").

Rollout consequence, exactly per the plan's `oauth_review` semantics: a
**dev-mode app** can be created immediately and gates only L4 (a real token must
reach the live API) — it does **not** gate dev, L4, or the batch-end merge. Only
the **visible flip** waits on certification. Code lands **complete but hidden**
in Wave 1; the flip is decoupled from the certification clock. Nothing about the
Xero flow is unusual for `oauth_review`; the lane assignment is correct as
written and needs no amendment-log entry.

Sources checked: Xero "The standard authorization code flow", "OAuth 2.0 API
limits", "Token types", "Xero Tenants", "Managing Connections", and the
uncertified-app-limit community guidance.

## 2. Which official API surface this tool wraps, and why

An AI teammate working "in Xero" is doing **small-business accounting ops**:
read and raise sales invoices, enter bills, record payments, look up and create
contacts, inspect the chart of accounts, pull financial reports (P&L, balance
sheet, aged receivables/payables), and read bank transactions. That maps
cleanly onto Xero's **Accounting API** (`https://api.xero.com/api.xro/2.0/`),
plus two Identity-plane calls the flow structurally requires.

Three official surfaces are in play — this is the load-bearing Xero-specific
fact:

1. **Identity / OpenID Connect** — `GET https://identity.xero.com/connect/userinfo`
   returns the authorizing user's claims (`sub`, `xero_userid`, `email`,
   `given_name`, `family_name`). Used **once, by Helio's connect flow**, for
   connection identity (see §4). The anycli tool never calls it.
2. **Connections (tenant discovery)** — `GET https://api.xero.com/connections`
   returns the array of Xero organisations (tenants) the current token can act
   on: `[{id, tenantId, tenantName, tenantType, createdDateUtc, updatedDateUtc}]`.
   No tenant header required. **The anycli tool calls this at runtime** to
   resolve which organisation a command targets (see §3).
3. **Accounting API** — `https://api.xero.com/api.xro/2.0/<Resource>`. Every
   call carries `Authorization: Bearer <token>`, `Accept: application/json`, and
   the **`Xero-Tenant-Id: <tenantId>`** header selecting the organisation. This
   is the tool's actual working surface.

**Why the tenant header dominates the design.** Unlike a single-host API, a
Xero access token is not scoped to one account — one authorization can act on
**N organisations**, and *which* org a call hits is chosen per-request via the
`Xero-Tenant-Id` header. That header value comes only from the connections
endpoint. So the tool must resolve a tenant before any accounting call. This is
distinct from Salesforce's `instance_url` (which changes the API *host* and is
fixed per connection): Xero's host is constant, the tenant is a per-call header,
and one connection can legitimately span many tenants.

Endpoints wrapped (Accounting API v2.0, chosen by the AI-teammate task model):

| Verb group | Endpoint(s) | Scope needed |
|---|---|---|
| `connections` (org list) | `GET /connections` (Identity host) | any valid token |
| `organisation` | `GET /Organisation` | `accounting.settings[.read]` |
| `contact` | `GET/POST/PUT /Contacts`, `GET /Contacts/{id}` | `accounting.contacts` |
| `invoice` | `GET/POST/PUT /Invoices`, `GET /Invoices/{id}`, `POST /Invoices/{id}/Email` | `accounting.transactions` |
| `payment` | `GET/PUT /Payments`, `GET /Payments/{id}` | `accounting.transactions` |
| `bank-transaction` | `GET/POST/PUT /BankTransactions` | `accounting.transactions` |
| `account` | `GET /Accounts` (chart of accounts) | `accounting.settings[.read]` |
| `item` | `GET/POST/PUT /Items` (products & services) | `accounting.settings` |
| `tax-rate` | `GET /TaxRates` | `accounting.settings[.read]` |
| `report` | `GET /Reports/{ProfitAndLoss,BalanceSheet,TrialBalance,AgedReceivablesByContact,AgedPayablesByContact}` | `accounting.reports.read` |
| `fetch` (escape hatch) | `GET /<any api.xro/2.0 path>` | scope of the path |

`fetch` is the notion-style raw passthrough so the AI can reach quotes, credit
notes, journals, tracking categories, etc. without the tool enumerating every
Xero resource. Write coverage is deliberately scoped to the high-frequency
invoice/bill/contact/payment path; long-tail writes are out of the initial
surface (a later growth, not a v1 gap).

## 3. anycli definition

**Type decision (skill stage-1 rubric): `service`.** The `cli` type is only for
wrapping an official, non-interactive, `--json`-capable binary provisionable
into the runtime image (the `github`→`gh`, `lark`→`lark-cli` exceptions). Xero
ships **no** official CLI. So this is a `service`-type definition backed by an
HTTP implementation in `internal/tools/xero/`, following the `notion`
reference shape (resource-grouped cobra tree; `BaseURL`/`HC`/`Out`/`Err`
struct so tests point at an `httptest` server; exit codes 0 success / 1
runtime+API failure via typed `apiError` / 2 usage/parse; `--json` structured
error envelope).

`definitions/tools/xero.json` (service type, single credential binding):

```jsonc
{
  "name": "xero",
  "type": "service",
  "description": "Xero accounting: invoices, bills, contacts, payments, accounts, and reports",
  "auth": {
    "credentials": [
      { "source": { "field": "access_token" },
        "inject": { "type": "env", "env_var": "XERO_ACCESS_TOKEN" } }
    ]
  }
}
```

Only `access_token` is injected. The tool resolves the tenant itself; no tenant
credential field exists (see the resolution algorithm below).

### Subcommand tree

```
xero connections                          # GET /connections — list orgs (id, name, type)
xero organisation get
xero contact  list|get|create|update
xero invoice  list|get|create|update|email
xero payment  list|get|create
xero bank-transaction list|get|create
xero account  list
xero item     list|get|create|update
xero tax-rate list
xero report   pnl|balance-sheet|trial-balance|aged-receivables|aged-payables
xero fetch    <path> [--query k=v ...]     # raw GET under api.xro/2.0
```

Every subcommand except `connections` accepts `--tenant <tenantId|tenantName>`.

### Tenant resolution algorithm (the core of the service)

For any accounting call, resolve the target tenant, then send
`Xero-Tenant-Id`:

1. If `--tenant` is set (or `XERO_TENANT_ID` env is present): accept a tenant
   GUID directly, or match case-insensitively against `tenantName` from
   `GET /connections`. Ambiguous name match → exit 2 with the candidate list.
2. Else `GET /connections`:
   - exactly **1** tenant → use it (the common single-org case; zero friction);
   - **>1** → exit 2, error `multiple Xero organisations connected; pass
     --tenant <id|name>`, listing `tenantName`/`tenantId` pairs so the AI can
     retry deterministically;
   - **0** → exit 1, error `no Xero organisation connected to this login`.

This keeps tenant selection **inside** anycli (credential-safe, no Helio-side
metadata plumbing), makes the single-org path invisible, and gives the AI a
self-describing recovery path in the multi-org case. `connections` is also a
first-class subcommand so the AI can enumerate orgs before acting.

### JSON output shape

Xero returns JSON when `Accept: application/json` is sent. Xero wraps
collections in a PascalCase envelope (`{"Invoices": [ ... ]}`,
`{"Contacts": [ ... ]}`, `{"Reports": [ ... ]}`). The service emits the Xero
JSON body **verbatim** on stdout (agent-neutral, no lossy re-shaping), exit 0.
Errors render as the typed envelope on stderr:
`{"error":{"tool":"xero","code":"api_error","status":<http>,"message":"...","details":<xero body>}}`
with `--json`; a one-line human message without it. Xero's own error bodies
(`{"Type":"ValidationException","Elements":[...]}` on 400,
`{"Detail":"..."}` on 401/403) are surfaced under `details` rather than
swallowed.

## 4. Credential fields & the exact OAuth flow

**Registration model.** One Helio-owned confidential web application registered
in the Xero developer portal (My Apps), with Helio's integration-service OAuth
callback as the redirect URI. Client id + client secret are issued at
registration; a dev-mode app is usable immediately (subject to the 25-tenant
cap) and gates only L4. Certification (App Partner) gates only the visible flip.

**Endpoints & token semantics (verified):**

- Authorize: `https://login.xero.com/identity/connect/authorize`
- Token (code exchange **and** refresh): `https://identity.xero.com/connect/token`
- Client auth: HTTP **Basic** — `Authorization: Basic base64(client_id:client_secret)` — with an
  `application/x-www-form-urlencoded` body ⇒ `token_exchange_style: form_basic`.
- PKCE: **not used**. PKCE is Xero's path for public/mobile apps *without* a
  secret; Helio is a confidential web app with a client secret ⇒ `pkce: none`
  (matches the Google/LinkedIn confidential-client precedents).
- Access token TTL: **30 minutes**.
- Refresh token: **rotates on every refresh** (each refresh returns a new
  `refresh_token`; the old one is invalidated after a 30-minute grace window)
  and **expires after 60 days** if unused.
- `offline_access` scope is **required** to receive a refresh token at all.
- Authorization code is single-use, 5-minute TTL.

**Scopes requested:**

```
offline_access openid profile email
accounting.transactions accounting.contacts
accounting.settings accounting.reports.read
```

`offline_access` (refresh), `openid profile email` (identity via userinfo), and
the four `accounting.*` scopes cover the §2 surface. `accounting.settings`
(not `.read`) is requested because `item` create/update writes settings-plane
objects; if the eventual surface stays read-only there, it can narrow to
`accounting.settings.read`.

**Connection identity (bundle `identity:`).** Source **userinfo**, URL
`https://identity.xero.com/connect/userinfo`, `stable_key: /sub` (== the stable
`xero_userid` GUID), `label_candidates: [/email, /name, /sub]`. Identity is the
authorizing **Xero user**, not a tenant — deliberately. A single user's
authorization can grant N organisations, and the newest token accumulates all
of that user's tenant grants, so a user-scoped `account_key` means a reconnect
**widens** tenant access via idempotent upsert-replace rather than fragmenting
into per-tenant connections. Runtime tenant selection (§3) then picks the org
per command. (Rejected alternative: tenant-scoped `account_key` — it forces a
choice of "which tenant is the key" when one auth returns many, and would need
Salesforce-style connect-time metadata capture; strictly worse and non-orthogonal.)

**Disconnect.** `disconnect_mode: provider_revoke`. Xero implements RFC 7009
token revocation at `https://identity.xero.com/connect/revocation` (POST
`token=<refresh_token>`, Basic client auth), which revokes the whole token set.
Bundle `auth.oauth.revoke`: `url` = that endpoint, `client_auth: basic`,
`token: refresh_token`, `token_type_hint: none`.

**`required_config_fields`:** `[oauth.client_id, oauth.client_secret]`. Both land
together in integration-service config — `config/` locally **and** the Helm
Secret under `deploy/` — per the Config Sync hard rule (lane 1 owns the landing;
must precede Xero's L5). No secret is ever committed to the bundle.

## 5. Helio provider bundle plan (hidden-first)

`integrations/providers/xero/provider.yaml`, `presentation.visible: false`:

```yaml
schema: helio.provider/v1
key: xero
go_name: Xero

presentation:
  name: Xero
  description_key: xero
  consent_domain: xero.com
  visible: false            # hidden-first; flip is the single go-live change post-certification + L5
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual         # provider sees a person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://login.xero.com/identity/connect/authorize
    token_url: https://identity.xero.com/connect/token
    token_exchange_style: form_basic
    pkce: none
    authorize_params:
      prompt: consent        # force the org-consent screen on reconnect
    scopes:
      - offline_access
      - openid
      - profile
      - email
      - accounting.transactions
      - accounting.contacts
      - accounting.settings
      - accounting.reports.read
    display_scopes: [offline_access, openid, email, accounting.transactions,
                     accounting.contacts, accounting.settings, accounting.reports.read]
    single_active_token: false
    refresh_lease: credential            # rotating refresh token — serialize per credential
    revoke:
      url: https://identity.xero.com/connect/revocation
      client_auth: basic
      token: refresh_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://identity.xero.com/connect/userinfo
  stable_key: /sub
  label_candidates: [/email, /name, /sub]

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
  name: xero
  kind: oauth
```

### The one integration-service capability item: `refresh_lease: credential`

This is the **only** Helio-side capability question, and it is a
**reuse-or-grow**, not new machinery. `refresh_lease: credential`
(`OAuthLeaseCredential`) exists precisely to serialize refreshes per credential
across replicas — the exact race Xero's rotating refresh token creates (two
workers rotating the same token → one gets `invalid_grant`; Xero's own guidance
is to treat "refresh token per connection" as a shared resource).

On the current worktree base, `runtimeStrategyContracts[RuntimeStrategyStandardOAuth]`
still pins `refreshLeaseScope: OAuthLeaseNone` as a single required value, so a
bundle declaring `refresh_lease: credential` would fail
`runtime_contract.go`'s equality check. **However**, keap (#152), signnow
(#189), and hootsuite (#420) already performed the "standard_oauth refresh_lease
allowed-set" growth on their branches — converting that single scope into an
admitted **set** that includes `OAuthLeaseCredential`. By the time Xero's batch
merges, that growth is expected to be present on the batch base.

Therefore:

- **If the allowed-set growth is already on Xero's base:** Xero adds **zero**
  integration-service code — it just selects `refresh_lease: credential`, and
  the whole provider is pure standard_oauth golden path (no adapter, no metadata
  capture, no deriver).
- **If it is not yet present:** Xero's branch performs the identical minimal
  growth (make `standard_oauth`'s admitted `refreshLeaseScope` a set containing
  `none` + `credential`, add the table-test case), which the batch lead
  de-dupes at merge. Either way it is a shared-surface edit the batch lead owns,
  not a Xero-specific mechanism.

No `service/adapter_xero.go` is warranted: Xero's exchange (`form_basic`),
identity (userinfo JSON-pointer), and revoke (RFC 7009) all fall inside the
declarative `standard_oauth` capability set. The tenant complexity lives
entirely in the anycli tool, below the token gateway.

### Other Helio-side artifacts

- **Resolver:** none — id == key == `xero`, no divergence entry.
- **UI icon:** `ui/helio-app/src/integrations/icons/xero.svg` + hand-registered
  in `ui/helio-app/src/integrations/providerIcons.ts` (manual, never generated).
- **i18n:** `xero` presentation/description strings in the locale catalog.
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` documenting the subcommands, the
  `--tenant` selection rule, and the multi-org behaviour; plugin version bump +
  marketplace publish ride the batch-end merge.
- **Five generated projections** regenerate together at batch-end
  (`provider-gen`); never committed on the tool branch.

## 6. Test plan mapped to the five layers

| Layer | Xero-specific content | Needs supplied creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fakes for `/connections`, `/api.xro/2.0/*`; assert `Authorization: Bearer` + `Xero-Tenant-Id` + `Accept: application/json` headers; assert tenant-resolution branches (single → auto; multi → exit 2 + candidate list; zero → exit 1; `--tenant` by GUID and by name); assert `--json` error envelope maps Xero 400 `ValidationException` / 401 bodies | No — fakes only |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<real> anycli xero -- connections` then `-- organisation get` / `-- invoice list --tenant <name>` against the live Xero API; confirms real field names, header names, and tenant resolution against a real multi-tenant token | **Yes** — a real Xero access token from a demo-company sandbox org (from the dev app) |
| **L3** generation + suites | `provider-gen --check` green with the bundle; integration-service suite green incl. the `refresh_lease: credential` contract test; helio-cli build via local `replace` + `go test ./cmd/heliox/cmds/tool/` (hidden provider registers as cobra-hidden) | No |
| **L4** singleton + seed | Start singleton (`env: dev`); `POST /internal/test-only/connections/seed` with a **real** Xero `access_token` + `refresh_token` and a deliberately short `expires_at` (exercise the token-gateway refresh-and-write-back **and** per-credential lease under rotation), against a **real** seeded org/assistant/owner identity; then `heliox tool xero -- connections` and `-- invoice list` return real data | **Yes** — real Xero access+refresh tokens (dev-mode app); real seeded Helio identities |
| **L5** full connect | With Xero still hidden: `heliox tool xero auth` → complete Xero consent + org selection on the dev app (real account) → confirm `oauth_connected` event on the originating channel → run `heliox tool xero -- organisation get` unseeded through the new connection. Human-in-the-loop (oauth L5, plan lane 3) | **Yes** — a real Xero login + org, human-driven consent |

L4 additionally validates the rotating-token path specifically: the short
`expires_at` forces the very next call through refresh, proving both the
write-back of the rotated `refresh_token` and that the `credential`-scoped lease
serializes it. Certification (App Partner) is **not** a test-layer input — it
gates only the post-L5 visible flip, per §1.

## 7. Open decisions recorded

- **Scope breadth vs. certification friction.** More `accounting.*` scopes ease
  the API surface but can lengthen certification review. v1 stays at the seven
  scopes in §5; broaden only with a driving need.
- **Write surface.** v1 writes cover invoice/bill/contact/payment/item; long-tail
  writes (quotes, credit notes, manual journals) are reachable read-only via
  `fetch` and are a later additive growth, not a v1 hole.
- **`accounting.settings` vs `.read`.** Requested read-write to support
  `item` create/update; narrow to `.read` if the shipped surface ends up
  settings-read-only.
