# QuickBooks Online — per-tool design (`quickbooks`)

Scratch design for the `heliox tool quickbooks` provider. Batch-lead strips this
file at batch end. Branch: `tool/quickbooks` (both repos). Catalog row 37 —
Payments & Commerce, Wave 1, lane `oauth_review`. Naming axes are all identical:
① CLI word `quickbooks` · ② anycli id `quickbooks` · ③ provider key `quickbooks`
→ **no `toolToProvider` divergence entry** (identity mapping).

All facts below were verified against Intuit's official developer docs
(`developer.intuit.com` / `help.developer.intuit.com`), not inherited from the
catalog. Divergences from the catalog are called out in §7.

---

## 1. What an AI teammate does with QuickBooks Online, and the API surface it needs

QuickBooks Online (QBO) is a small/mid-business accounting system. An AI teammate
sitting in a finance/ops channel is asked things like: "what's our AR aging",
"list unpaid invoices over 30 days", "pull this month's P&L", "create an invoice
for Acme for $4,000", "who are our top vendors by spend", "record a bill from
Stripe", "what's the balance on account 4000". These map cleanly onto Intuit's
**Accounting API v3** plus the **Reports API** — the tool wraps those and nothing
else. QBO Payments (`com.intuit.quickbooks.payment`) and Payroll are deliberately
out of scope: different scope, different review track, and not what a teammate is
asked to do in a channel.

Every Accounting/Reports call is **company-scoped** and hangs off one base:

```
https://quickbooks.api.intuit.com/v3/company/{realmId}/{resource}?minorversion=75
```

`realmId` (a.k.a. company id) is the tenant key — it is returned on the OAuth
redirect (see §3) and required in every URL. `minorversion` pins the response
schema; we send a fixed, current value (75 at time of writing) so output is
stable across Intuit's rolling schema bumps.

The endpoints the tool exposes, chosen by teammate task frequency:

| Task | Endpoint(s) |
|---|---|
| Company identity / health check | `GET /companyinfo/{realmId}` |
| Ad-hoc read (the workhorse) | `GET /query?query=<QBO SQL>` (e.g. `select * from Invoice where Balance > '0'`) |
| Customers | `GET /customer/{id}`, `POST /customer` (create/update via sparse), query |
| Invoices | `GET /invoice/{id}`, `POST /invoice`, `POST /invoice/{id}/send` (email) |
| Bills (AP) | `GET /bill/{id}`, `POST /bill`, query |
| Vendors | `GET /vendor/{id}`, `POST /vendor`, query |
| Payments (received) | `GET /payment/{id}`, `POST /payment`, query |
| Accounts (chart of accounts) | `GET /account/{id}`, query |
| Items (products/services) | `GET /item/{id}`, query |
| Reports | `GET /reports/{name}` — `ProfitAndLoss`, `BalanceSheet`, `AgedReceivables`, `AgedPayables`, `CashFlow`, `GeneralLedger`, … with `start_date`/`end_date`/`date_macro` params |

`query` covers most read intents with one verb, so the resource `list` surfaces
stay thin; the named-resource verbs exist for get-by-id and the create/mutate
paths (which QBO models as full or **sparse** upserts on `POST /{resource}`).

---

## 2. anycli definition — form, verbs, output

**Tool form: `service` type** (per SKILL.md stage-1 rubric). No official QBO CLI
exists; the surface is a plain REST+JSON API with Bearer auth. This is the
default and correct choice (matches 21/23 existing definitions). Go package
`internal/tools/quickbooks/` (id has no dashes, no leading digit → package name
== id), registered `RegisterService("quickbooks", &quickbooks.Service{})`.

**Definition** `definitions/tools/quickbooks.json`: `name: quickbooks`, `type:
service`, one-line description, and an `auth` block with three credential
bindings, all `type: env` (§3):

- `access_token` → `QUICKBOOKS_ACCESS_TOKEN` (Bearer)
- `account_key` → `QUICKBOOKS_REALM_ID` (the realmId; `credential.fields`
  already exposes `account_key`, §4)
- `environment` → `QUICKBOOKS_ENVIRONMENT` (optional; `production` default,
  `sandbox` only for the L2 harness — selects the `sandbox-quickbooks.api.intuit.com`
  base). Sourced from a bundle static/config field, never a user secret.

**Command tree** (cobra, grouped by resource; copy `internal/tools/notion/`'s
shape — `BaseURL`/`HC`/`Out`/`Err` struct so tests point at `httptest`):

```
heliox tool quickbooks -- company get
heliox tool quickbooks -- query --sql "select * from Invoice where Balance > '0'"
heliox tool quickbooks -- customer  list|get|create   [--id ..] [--json-body ..]
heliox tool quickbooks -- invoice   list|get|create|send [--id ..] [--to ..]
heliox tool quickbooks -- bill      list|get|create
heliox tool quickbooks -- vendor    list|get|create
heliox tool quickbooks -- payment   list|get|create
heliox tool quickbooks -- account   list|get
heliox tool quickbooks -- item      list|get
heliox tool quickbooks -- report    get --name ProfitAndLoss [--start-date ..] [--end-date ..] [--date-macro "This Fiscal Year"]
```

`list` verbs are thin wrappers that build a `SELECT * FROM <Entity>` query with
optional `--where`/`--max`/`--start-position` (QBO paginates via
`STARTPOSITION`/`MAXRESULTS` inside the query language, not header links). Create
verbs take a `--json-body` (the QBO entity JSON) to stay non-interactive and
agent-friendly; there is no bespoke field modelling in v1.

**Output & exit codes** (notion contract): success prints the provider JSON
(unwrapped entity or report) on stdout, exit 0; API/runtime failure → exit 1 with
a typed `apiError` and, under `--json`, a structured `{"error":{...}}` envelope
that surfaces QBO's `Fault`/`Error` array (QBO returns rich `{"Fault":{"Error":
[{"Message","Detail","code"}]}}` bodies — we pass code/detail through so the
teammate sees *why*, e.g. `ValidationFault` on a bad `Line`); usage/parse errors
→ exit 2. All non-interactive: input from flags/env only (AGENTS.md rule).

L1 unit tests: `httptest.Server` fake asserts the URL carries
`/v3/company/<realm>/`, the `minorversion` query param, the `Authorization:
Bearer` header, `Accept: application/json`, and both plaintext and `--json`
rendering of a QBO `Fault` body. Never hits the real API.

---

## 3. Credential fields & the exact OAuth flow (`oauth_review` verified)

QBO is **OAuth 2.0 authorization-code only** — there are no API keys or basic
auth for the data API (confirmed: "The QuickBooks Online API uses OAuth 2.0
Authorization Code flow exclusively"). Verified endpoints:

| Purpose | URL |
|---|---|
| Authorize | `https://appcenter.intuit.com/connect/oauth2` |
| Token / refresh | `https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer` |
| Revoke | `https://developer.api.intuit.com/v2/oauth2/tokens/revoke` |
| OIDC userinfo (unused, see below) | `https://accounts.platform.intuit.com/v1/openid_connect/userinfo` |

- **Scope:** `com.intuit.quickbooks.accounting` (single scope covers all
  Accounting + Reports endpoints). We do **not** request `openid/profile/email`
  — see identity below; skipping them keeps the review surface minimal.
- **Client auth at the token endpoint:** HTTP **Basic** (`client_id:client_secret`)
  → bundle `token_exchange_style: form_basic`.
- **PKCE:** not required for a confidential server-side client → `pkce: none`.
- **Refresh token is always returned** for the accounting scope; no
  `access_type=offline`/`offline_access` param is needed (unlike Google/MS).
- **Token lifetimes (verified):** access token **3600 s (1 h)**; refresh token
  **100 days** but it **rotates on every refresh** — a new refresh token is
  issued roughly every 24 h and the previous value is invalidated. Reusing an
  already-rotated refresh token throws `invalid_grant` and **revokes the whole
  authorization chain**, forcing re-consent. Consequence for Helio: the token
  gateway's refresh-and-write-back must persist the newest refresh token every
  time (standard_oauth already does), and concurrent refreshes on one connection
  must be serialized (§4, refresh lease).
- **realmId delivery — the one real quirk:** on a successful authorize, Intuit
  redirects to `redirect_uri?code=…&state=…&realmId=…`. `realmId` is **not** in
  the token response body and **not** in userinfo — it exists only as a redirect
  query param. It must be captured at connect time and persisted as the tenant
  key (§4).

**Identity / AccountKey = realmId.** A QBO connection is a *company*, not a
person; the same human can connect several companies, and every API URL needs the
realmId. So the connection's `AccountKey` is the **realmId** (not an OIDC `sub`).
Human-readable label: fetch `CompanyInfo.CompanyName` once post-connect for the
label, falling back to the realmId string. This is why OIDC scopes are omitted —
`sub` would be the wrong grain.

**`oauth_review` lane confirmed.** Intuit issues development-mode keys
immediately (dev/sandbox app, gates L4/L5 on a dev app — no wait). **Production**
keys require passing Intuit's app assessment / security questionnaire before
external QBO companies can connect at scale. This gates only the visible flip,
not dev/L4/merge — exactly the hidden-first decoupling the plan relies on. Lane
matches the catalog; no divergence.

**Config fields (Helio side, never in the bundle):** `oauth.client_id`,
`oauth.client_secret` → `auth.required_config_fields: [oauth.client_id,
oauth.client_secret]`; landed by lane 1 into `config/` + the `deploy/` Helm
Secret together (Config Sync rule).

---

## 4. Helio provider bundle plan (`integrations/providers/quickbooks/provider.yaml`, hidden-first)

Model on the `standard_oauth` bundles (gmail/microsoft_outlook shape), with the
realmId capture being the single non-standard need. Sketch:

```yaml
schema: helio.provider/v1
key: quickbooks
go_name: QuickBooks
presentation:
  name: QuickBooks Online
  description_key: quickbooks
  consent_domain: appcenter.intuit.com
  visible: false            # hidden-first; flip is the go-live change
  order: <assigned at batch end>
auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://appcenter.intuit.com/connect/oauth2
    token_url: https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer
    token_exchange_style: form_basic
    pkce: none
    scopes: [com.intuit.quickbooks.accounting]
    display_scopes: [quickbooks.accounting]
    single_active_token: false
    refresh_lease: credential          # serialize refresh per connection (rotation safety)
    revoke:
      url: https://developer.api.intuit.com/v2/oauth2/tokens/revoke
      client_auth: basic
      token: refresh_token             # revoking refresh kills the chain
      token_type_hint: none
identity:
  source: callback_param               # NEW: realmId from the redirect query (see growth)
  stable_key: realmId
  label_candidates: [company_name, realmId]
connection:
  mode: isolated
  disconnect_mode: provider_revoke
  runtime_strategy: standard_oauth
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key   # == realmId → injected as QUICKBOOKS_REALM_ID
tool:
  name: quickbooks
  kind: oauth
```

### Integration-service capability growths (two; verify against the batch base first)

1. **Callback-query-param identity source (the real work).** Today the generic
   OAuth callback (`OAuthService.prepareOAuthCallback`, DTO `OAuthCallbackRequest`
   = `{state, code, error}`) discards any other redirect query param, and the
   declarative identity resolver extracts `stable_key` only from the
   token-response map or a userinfo GET (`declarative_identity.go`). realmId lives
   in neither. Growth: thread a whitelisted, provider-declared callback param
   (`identity.source: callback_param`, `stable_key: realmId`) from the redirect
   through the frontend → `OAuthCallbackRequest` → callback plan, and use it as
   the `AccountKey` (and stamp `metadata.realm_id`). This is the QBO analogue of
   the salesforce `instance_url` capture already done in this program (task #168)
   — same idea (a tenant scoper captured at connect), different source (callback
   query vs token response). Keep it declarative and closed: a bundle names one
   allowed callback param; nothing else is read from the redirect.
2. **`refresh_lease: credential` under standard_oauth.** On this worktree base,
   `runtime_contract.go` pins standard_oauth's `refreshLeaseScope` to `none`
   (single-value equality check). QBO's rotate-and-invalidate refresh token needs
   per-connection single-flight refresh (`OAuthLeaseCredential`) to avoid two
   concurrent runtime calls racing and tripping `invalid_grant` → chain
   revocation. The keap/signnow/hootsuite work (#152/#189/#420) already grew this
   allowed-set to a `slices.Contains` set on other branches — **first confirm
   whether that landed on the batch base**; if yes, no-op, just set `credential`;
   if not, grow the standard_oauth contract to accept `{none, credential}` with a
   test. Do **not** use `provider` scope (that's X's org-wide single-active-token
   semantics; QBO is per-company).

No compiled `service/adapter_*.go` is needed — the realmId capture is a
declarative-source growth, not a provider-shaped response dialect. If the
callback-param growth is judged too broad for one tool, the fallback is a narrow
`adapter_quickbooks.go` that reads the one param, but the declarative route is
preferred (it also serves any future callback-param providers, e.g. some Xero-
style flows).

Runtime injection needs no growth: `credential.fields.account_key` already flows
the realmId into anycli's credential map; the definition binds it to
`QUICKBOOKS_REALM_ID` and the service builds `/v3/company/<realmId>/`.

### Non-service artifacts (batch-end merge)

- Resolver: **no** `toolToProvider` entry (id == key).
- UI icon: `ui/helio-app/src/integrations/icons/quickbooks.svg` + register in
  `providerIcons.ts` (hand, never generated); i18n label + `description_key`
  string `quickbooks`.
- AI-facing doc: provider sub-doc under `agents/plugins/heliox/skills/tool/`
  documenting the `query` workhorse, the report names, and that create verbs take
  raw QBO entity JSON via `--json-body`; one plugin version bump per batch.
- `provider-gen` + `--check` produce the five projections — committed by the
  batch lead only; run locally for validation, never committed on this branch.

---

## 5. Test plan — five layers

| Layer | What runs | External credentials? |
|---|---|---|
| **L1** | anycli `go test ./...`: definition + `internal/tools/quickbooks/` unit tests against an `httptest` QBO fake — asserts `/v3/company/<realm>/` path, `minorversion` param, Bearer header, and `Fault`-body error rendering (plain + `--json`). | No |
| **L2** | Dev harness against the **real sandbox** QBO API: `QUICKBOOKS_ENVIRONMENT=sandbox ANYCLI_CRED_ACCESS_TOKEN=… ANYCLI_CRED_ACCOUNT_KEY=<realmId> anycli quickbooks -- company get` and a `query`. Mandatory before the pin bump — proves field names, injection, and request shape match live Intuit. | **Yes** — a QBO sandbox company + a live access token + its realmId (from the OAuth Playground or a dev-app authorize). |
| **L3** | `provider-gen --check` (on-branch, local) + both repos' unit suites (`helio-cli` build with a local `go.mod replace` → anycli branch; integration-service tests incl. the two capability growths). | No |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with `provider: quickbooks`, `account_key: <realmId>`, seeded `access_token` **and** `refresh_token` with a deliberately short `expires_at` (forces the token-gateway refresh+write-back path — the critical rotation exercise), then `heliox tool quickbooks -- company get`. QBO is a user-token OAuth provider → seedable (not minted). | **Yes** — a real access+refresh token pair + realmId from a dev/sandbox app (lane 1 dev-mode app gates L4). |
| **L5** | Once, hidden, pre-flip: `heliox tool quickbooks auth` → Intuit consent on the dev/sandbox app → confirm `oauth_connected` fires and realmId landed as the AccountKey → unseeded live `query`. Validates the callback-param capture end-to-end (the growth's real proof). Human-in-the-loop (Intuit login/2FA). | **Yes** — a real Intuit account able to authorize the dev app against a QBO company. |

Layers needing externally supplied credentials: **L2, L4, L5** (sandbox company +
tokens for L2/L4; a live Intuit authorizing account for L5). L1/L3 are
self-contained. The visible flip additionally waits on **production-app review
clearance** (§3) — gated separately from L5, per the hidden-first model.

---

## 6. Sequencing / done

Hidden-first: land anycli (def + service + L1) and the bundle (`visible: false`)
+ the two capability growths; pin-bump at batch end; run L2→L5 while hidden;
flip `visible: true` + regenerate as the single go-live change only after L5 and
production review both clear. Until then: code-complete (hidden), not done.

## 7. Divergences from catalog / audit (recorded per instructions)

- **None material.** Catalog row 37: id `quickbooks`, key `quickbooks`, lane
  `oauth_review`, Wave 1 — all confirmed against official docs. Lane is correct:
  dev keys are self-serve (dev/L4/L5 unblocked) while production keys require
  Intuit's app assessment (visible-flip gate). No resolver divergence (id == key).
- **Note for the batch lead:** QBO needs a **callback-query-param identity
  source** growth (realmId is only on the redirect, absent from token response
  and userinfo). This is a genuine, reusable integration-service capability, not
  a per-tool hack — flagged here at stage 1 as the audit rubric asks. It parallels
  the shipped salesforce `instance_url` tenant-capture growth. Also confirm the
  standard_oauth `refresh_lease` allowed-set includes `credential` on the batch
  base before relying on it.
