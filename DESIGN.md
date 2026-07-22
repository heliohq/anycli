# FreshBooks — `heliox tool freshbooks` design

Scratch design doc for the `tool/freshbooks` batch branch (stripped by the batch
lead at batch-end). Drives the anycli definition + service, the Helio provider
bundle, and the integration-service capability growth this tool needs.

- **Catalog row:** #148 — Product FreshBooks · anycli id `freshbooks` · provider
  key `freshbooks` · **auth lane `oauth_light`** · Wave 2 · Finance.
- **Three axes (no divergence):** ① CLI word `freshbooks` (flat, no group) → `heliox tool freshbooks` · ② anycli id `freshbooks` · ③ provider key `freshbooks`. ② == ③, so **no `toolToProvider` entry** is added (`ProviderFor` returns the tool id unchanged).
- **Go package:** `internal/tools/freshbooks/` (id has no dashes/leading digit).

## 1. Auth-lane verification against official docs

The master plan lists FreshBooks as `oauth_light` (it was never in the api_key
audit set — it started oauth_light). Verified against the official FreshBooks
API docs, the lane holds:

- FreshBooks exposes a standard multi-tenant **OAuth 2.0 authorization-code**
  flow. Any FreshBooks user can authorize one registered app.
- **App registration is self-serve** in the FreshBooks Developer portal
  (Settings → Developer): you create an app with a name + HTTPS redirect URI and
  receive `client_id`/`client_secret` immediately.
- Docs describe **no review/approval gate before external users can authorize**
  a registered app. There is an *optional* "FreshBooks App Store" listing with
  its own public-app requirements, but that is a marketplace-visibility track,
  not on the connect path — the same optional-listing shape as Airtable / Square
  / Vercel in the audit, all `oauth_light`.

**Verdict: `oauth_light` confirmed.** No divergence from the catalog. (The
App Store listing review, if ever pursued, gates only catalog marketing, never
Connect — it is not the `oauth_review` "review clearance gates the visible flip"
gate.)

Official endpoints (verified):
- Authorize: `https://auth.freshbooks.com/oauth/authorize`
- Token: `https://api.freshbooks.com/auth/oauth/token`
- Revoke: `https://api.freshbooks.com/auth/oauth/revoke`
- Identity (me): `https://api.freshbooks.com/auth/api/v1/users/me`
- Accounting base: `https://api.freshbooks.com/accounting/account/<accountId>/...`

## 2. What an AI teammate does with FreshBooks (drives the API surface)

FreshBooks is small-business cloud accounting. An AI teammate acting as a
bookkeeper/ops assistant does: look up a client; draft, create, and **send** an
invoice; check outstanding/overdue invoices; log an expense; issue an estimate;
record a payment; and summarize receivables. That intent selects the
**Accounting API** resource families plus the identity endpoint, and nothing
else (no reports firehose, no admin/team management for MVP).

### API surface wrapped (and why)

| Surface | Endpoint pattern | Why |
|---|---|---|
| Identity / account discovery | `GET /auth/api/v1/users/me` | Required first call — yields the `account_id` every accounting URL needs (see §3) and the connection identity. |
| Clients | `GET/POST/PUT .../users/clients` under `/accounting/account/<a>/` | Teammate resolves/creates the bill-to party before invoicing. |
| Invoices | `.../invoices/invoices[/<id>]` (+ send action) | Core AR object: list/overdue triage, create, update, send. |
| Expenses | `.../expenses/expenses[/<id>]` | Log costs, categorize spend. |
| Estimates | `.../estimates/estimates[/<id>]` | Quote before invoicing. |
| Payments | `.../payments/payments[/<id>]` | Record money received against invoices. |
| Items (billable) | `.../items/items` | Line-item catalog referenced when building invoices. |

Time-tracking/projects (`/timetracking/business/<businessId>/time_entries`,
keyed by `business_id` not `account_id`) is **out of MVP scope** — deferred; the
accounting families above cover the bookkeeper intent and keep the tool to one
ID axis.

## 3. anycli definition + service

**Type: `service`** (stage-1 rubric). No official FreshBooks CLI exists; the
integration is the Accounting HTTP API. Follows the `internal/tools/notion/`
shape: resource-grouped cobra tree, `BaseURL`/`HC`/`Out`/`Err` struct for
httptest injection, typed `apiError`, exit codes **0** success / **1**
runtime/API failure / **2** usage/parse, and a `--json` structured envelope.

`definitions/tools/freshbooks.json`:

```json
{
  "name": "freshbooks",
  "type": "service",
  "description": "FreshBooks cloud accounting (invoices, clients, expenses, estimates, payments) via OAuth token",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "FRESHBOOKS_TOKEN"}
      }
    ]
  }
}
```

Single Bearer credential injected as env `FRESHBOOKS_TOKEN`; the service sends
`Authorization: Bearer $FRESHBOOKS_TOKEN` on every call.

### Command tree (verbs)

```
freshbooks me                                 # identity + businesses/accounts (raw account discovery)
freshbooks client   list|get|create|update
freshbooks invoice  list|get|create|update|delete|send
freshbooks expense  list|get|create|update
freshbooks estimate list|get|create
freshbooks payment  list|get|create
freshbooks item     list
```

### The `account_id` resolution problem (service-layer, not Helio)

FreshBooks accounting URLs need `account_id` =
`business_memberships[].business.account_id` from `/users/me` — this is **not**
the identity stable key and is **not** in the token response. The service
resolves it at runtime like Salesforce/Zoho instance context, but entirely
inside anycli (no Helio connection metadata):

1. On any accounting command, if `--account <accountId>` is not supplied, the
   service calls `GET /auth/api/v1/users/me`, reads `business_memberships`.
2. Exactly one business with an `account_id` → use it silently.
3. Multiple → **fail fast** (exit 2) with a clear message listing the available
   `account_id`s and instructing `--account`. Never guess.
4. Zero accounting accounts (pure-client identity) → exit 1 with an explicit
   "this FreshBooks identity has no accounting account" error.

`--account` short-circuits the me call. `freshbooks me` exposes the mapping so a
teammate can discover the value.

### JSON output shape

FreshBooks wraps list results as
`{"response":{"result":{"invoices":[...],"page":1,"pages":3,"per_page":15,"total":42}}}`.
The service **unwraps** to a provider-neutral envelope so agents don't parse
FreshBooks' nesting:

```json
{"items": [ ... ], "page": 1, "pages": 3, "per_page": 15, "total": 42}
```

Single-object gets emit the unwrapped resource object. Errors under `--json`
render `{"error": {"message": "...", "status": <http>, "code": "<freshbooks code>"}}`
per the notion `apiError` pattern.

## 4. Helio provider bundle plan (hidden-first)

`integrations/providers/freshbooks/provider.yaml` (axis ③ dir == `key`):

```yaml
schema: helio.provider/v1
key: freshbooks
go_name: Freshbooks

presentation:
  name: FreshBooks
  description_key: freshbooks
  consent_domain: freshbooks.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual         # provider sees a person (email/identity)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://auth.freshbooks.com/oauth/authorize
    token_url: https://api.freshbooks.com/auth/oauth/token
    token_exchange_style: json_secret    # NEW capability — see §5
    pkce: none
    display_scopes: [profile.read, invoices.rw, clients.rw, expenses.rw, estimates.rw, payments.rw]
    scopes:
      - user:profile:read
      - user:invoices:read
      - user:invoices:write
      - user:clients:read
      - user:clients:write
      - user:expenses:read
      - user:expenses:write
      - user:estimates:read
      - user:estimates:write
      - user:payments:read
      - user:payments:write
    single_active_token: false
    refresh_lease: none      # rotation write-back is inherent to the gateway (§5)

identity:
  source: userinfo
  url: https://api.freshbooks.com/auth/api/v1/users/me
  stable_key: /response/id
  label_candidates: [/response/email, /response/first_name]

connection:
  mode: isolated
  disconnect_mode: local_only    # see §5 revoke note
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
  name: freshbooks
  kind: oauth
```

Config (lane 1, per-provider append landed together in `config/` **and** the
`deploy/` Helm Secret): `oauth.client_id` + `oauth.client_secret` for the
registered FreshBooks dev app. No secret ever in the bundle.

UI: `ui/helio-app/src/integrations/icons/freshbooks.svg` + `providerIcons.ts`
append; i18n `description_key: freshbooks`. AI-facing sub-doc under
`agents/plugins/heliox/skills/tool/`.

## 5. Integration-service capability growth (the real work)

Three divergences from the `standard_oauth` golden path, verified against
official docs. Growth #1 is required; #2 and #3 are verify-at-L2-then-grow.

**#1 — `json_secret` token-exchange style (REQUIRED, new enum).**
FreshBooks' token endpoint takes a **JSON body carrying `client_id` AND
`client_secret` in the body** (not form-encoded, not Basic header):

```json
{"grant_type":"authorization_code","client_id":"…","client_secret":"…",
 "code":"…","redirect_uri":"https://…"}
```

The current allowed set (`model/catalog.go`, `provider-gen/validate.go`) is
`form_secret | form_basic | json_basic` — there is **no `json_secret`**
(`json_basic` puts client creds in the *Basic header*, not the body). So this
tool adds one reviewed enum value: `json_secret` = JSON body + client creds in
the body, i.e. the JSON analog of `form_secret`. Touches `TokenExchangeStyle`
const + `render_symbols.go` map + `validate.go` `oneOf(...)` + the
`standardOAuthExchanger` body/auth selection, with a synthetic-provider unit
test. **Batch note:** a parallel branch (Square) may add the same value — the
batch lead reconciles a single `json_secret` addition; do not commit projections.

**#2 — `redirect_uri` on the refresh call (verify at L2).**
FreshBooks docs show `redirect_uri` in the **refresh_token** body too, which is
unusual (standard exchangers omit it on refresh). Verify at L2 whether FreshBooks
actually enforces it; if so, the `json_secret` exchanger must echo the stored
`redirect_uri` on refresh (small addition folded into #1). Refresh tokens are
**one-time-use and rotate** — only one alive per user per app; each refresh
returns a fresh refresh_token invalidating the prior. The gateway's
refresh-and-write-back (A3) already persists the rotated refresh_token, so
`refresh_lease: none` is correct; no lease capability needed. (Note the
concurrency hazard: two racing refreshes will lose one token — inherent to
FreshBooks, not a Helio bug.)

**#3 — identity userinfo static header `Api-Version: alpha` (verify at L2).**
FreshBooks' identity model doc states `GET /auth/api/v1/users/me` wants an
`Api-Version: alpha` header. The `declarativeIdentityResolver` issues a plain
Bearer GET. **L2 must confirm** whether `/users/me` returns identity with only
`Authorization`; if the header is mandatory, add a small reviewed
`identity.headers` (static-header) capability on the identity GET — precedent:
adobe-sign `base_uri` source, docusign userinfo metadata deriver. The
**accounting** endpoints (verified) do **not** require the header, so this is
scoped to identity resolution only.

**Revoke.** FreshBooks revoke (`/auth/oauth/revoke`) also takes a JSON body with
client creds — it does not fit the declarative revoker's form/Basic shapes
cleanly. MVP ships `disconnect_mode: local_only` (notion precedent); a
`provider_revoke` follow-up would need a JSON-body revoker variant and is not
worth blocking the hidden ship. Flag, don't build.

**Authorize scope param (verify at L2).** Confirm whether the authorize URL
accepts a `scope` param or whether scopes are fixed per-app in the developer
portal. If app-configured only, `scopes` stays for documentation/UI and the
app's portal config is the source of truth; `display_scopes` drives the UI
either way.

## 6. Five-layer test plan

| Layer | What it proves | Needs external creds |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes for `/users/me`, invoice list/create, client list, expense create; asserts Bearer injection, `account_id` resolution (single→silent, multi→exit 2 with `--account` guidance, zero→exit 1), `--json` envelope unwrap, plain+`--json` error rendering, exit codes 0/1/2. | No |
| **L2** `ANYCLI_CRED_ACCESS_TOKEN=… anycli freshbooks -- me` / `invoice list` against the **real** FreshBooks API | Field names, Bearer injection, real request shape; **resolves the three open questions**: does `/users/me` need `Api-Version: alpha` (#3), does refresh need `redirect_uri` (#2), does authorize accept `scope`. | **Yes** — a real FreshBooks account + a token minted from a dev app (account pool + lane-1 dev app). |
| **L3** `provider-gen --check` + both repos' unit suites (incl. the new `json_secret` synthetic-provider test) | Bundle validity, five projections regen clean, `json_secret` enum wired end to end. | No |
| **L4** singleton + `POST /internal/test-only/connections/seed` → `heliox tool freshbooks -- invoice list` | Token-gateway → anycli path with a seeded connection; seed `access_token` + `refresh_token` + short `expires_at` to force the **refresh-and-write-back** path (exercises rotation, #2). | **Yes** — real access+refresh token from the dev app / test account. |
| **L5** `heliox tool freshbooks auth` → FreshBooks consent → `oauth_connected` event → unseeded live run | The actual connect UX: authorize URL, `json_secret` code exchange, `/users/me` identity extraction, notification. oauth L5 = human-in-the-loop. | **Yes** — registered dev app (lane 1) + real FreshBooks account consent. |

Externally-supplied credentials gate **L2, L4, L5**; **L1 and L3 need none**.
Definition of done stays hidden until L1–L4 pass and L5 runs once; the
`visible: true` flip + regenerate is the single go-live change.

## 7. Open items for stage-1/L2 (must close before merge)

1. `json_secret` exchange style landed + tested (§5 #1) — required.
2. L2: `/users/me` `Api-Version: alpha` requirement → identity static-header
   capability iff mandatory (§5 #3).
3. L2: `redirect_uri` on refresh enforced? → fold into `json_secret` exchanger
   (§5 #2).
4. L2: authorize `scope` param accepted vs app-configured scopes (§5).
5. Revoke stays `local_only` for MVP; `provider_revoke` deferred (§5).
