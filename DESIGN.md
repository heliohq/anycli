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
    # NO `scopes:` key — deliberately, matching the notion precedent. FreshBooks
    # takes no `scope` authorize-URL param (§5, official docs); scopes are fixed
    # in the developer-portal app config. In provider-gen the bundle `scopes:`
    # key is NOT inert: manifest.oauthManifest.Scopes → render_go.go DefaultScopes
    # → oauth_start.go `q.Set("scope", strings.Join(DefaultScopes," "))`, so
    # declaring `scopes:` WOULD inject a `scope=...` param the endpoint does not
    # expect. `display_scopes` maps to DisplayScopes/ConsentScopes (consent copy)
    # and NEVER enters the authorize URL — so it is the only scope list we set.
    # These are illustrative consent slugs (real vocabulary is portal-defined;
    # confirm the exact identifiers at L2 — UI copy only, not a connect gate).
    display_scopes: [profile.read, invoices.rw, clients.rw, expenses.rw, estimates.rw, payments.rw]
    single_active_token: false
    refresh_lease: credential  # single-use rotating refresh → serialize per-credential; REQUIRES the standard_oauth refresh-lease allowed-set growth (§5 #2)

identity:
  source: userinfo
  url: https://api.freshbooks.com/auth/api/v1/users/me
  headers:
    Api-Version: alpha        # official /users/me example sends this header — pre-planned identity.headers capability (§5 #3), confirm mandatory at L2
  stable_key: /response/id    # JSON NUMBER — requires numeric-stable-key coercion (§4a, verify at L3)
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

### 4a. Numeric stable-key capability (REQUIRED capability check, verify at L3)

`/users/me` returns `response.id` as a **JSON number**, but the current
`declarativeIdentityResolver` extracts the stable key via `jsonPointerString`
(`declarative_identity.go`), which type-asserts the RFC 6901 leaf to `string`
and rejects anything else (`text, ok := value.(string)`) — a numeric leaf yields
`ok == false` → `"identity has no string value at stable key"` → empty
`AccountKey`. This is **not** a "nice to have": without numeric coercion the
connection cannot be keyed or looked up and is silently unusable despite
appearing connected (finding-3 failure).

The hubspot / typefully / kit numeric-id providers in this program each called
out the same requirement. This design does **not** assume the coercion is
already merged on the worktree base — on the current checkout the resolver is
still string-only. **L3 must prove** that numeric-stable-key coercion (string
fast-path, else `float64`/`json.Number` → canonical integer string, no `1.0`/
scientific-notation drift) exists in the declarative resolver and covers
`/response/id`. If absent, grow it here (parallel to the hubspot branch; the
batch lead reconciles a single coercion addition — do not commit projections).
It is a hard gate: identity extraction must be known-good before implementation,
not discovered at L2/L5.

## 5. Integration-service capability growth (the real work)

Verified against official FreshBooks docs
(https://www.freshbooks.com/api/authentication). Growths #1a, #1b and #2 are
**all REQUIRED** — FreshBooks' JSON token endpoint serves *both* the initial
code exchange and the refresh, refresh is load-bearing (bearer tokens are
short-lived and refresh tokens rotate single-use), and the single-use rotation
also forces a per-credential refresh lease — which itself requires a
runtime-contract allowed-set growth (the `standard_oauth` gate pins
`refresh_lease: none`), not just a bundle field. #3 is verify-at-L2-then-grow.

**#1a — `json_secret` token-exchange style, INITIAL code exchange (REQUIRED,
new enum).** FreshBooks' token endpoint (`POST /auth/oauth/token`) takes a
**JSON body carrying `client_id` AND `client_secret` in the body** (not
form-encoded, not Basic header):

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
`buildTokenRequest` body/auth selection in `oauth_exchange.go`
(`UsesHTTPBasicClientAuth()` must return **false** for `json_secret` so creds
go in the body, not a Basic header), with a synthetic-provider unit test.
**Batch note:** the parallel Square branch adds the same enum value — the batch
lead reconciles a single `json_secret` addition; do not commit projections.

**#1b — `json_secret` refresh path in `service/token_refresh.go` (REQUIRED,
NOT a fold-in of #1a).** This is the load-bearing correction. FreshBooks bearer
tokens are short-lived (docs: "not long lived", check the expiry) and refresh
tokens are **one-time-use and rotate** — each refresh returns a fresh
refresh_token that immediately invalidates the prior one, and only one is alive
per user per app. Refresh therefore fires within hours of every connect and is
the steady-state path, not an edge case.

The refresh call hits the **same JSON endpoint**, and the official docs show the
refresh body as JSON carrying `grant_type=refresh_token`, `refresh_token`,
`client_id`, `client_secret`, **and `redirect_uri`**:

```json
{"grant_type":"refresh_token","client_id":"…","client_secret":"…",
 "refresh_token":"…","redirect_uri":"https://…"}
```

But `requestOAuthRefresh` (`token_refresh.go:148-169`) is built on
`golang.org/x/oauth2` (`oauth2.Config.TokenSource`), which can only emit
`application/x-www-form-urlencoded` bodies (`AuthStyleInHeader` /
`AuthStyleInParams`) and **cannot** send a JSON body or add a custom
`redirect_uri` param. #1a's `buildTokenRequest` case is on the
authorization_code exchanger only and does nothing for refresh. A FreshBooks
refresh routed through the existing x/oauth2 path would be form-encoded, and by
the JSON-only premise of #1a the token endpoint rejects it → permanent refresh
error → `401 reconnect required` within ~hours of connect (finding-1 failure).
A3 strict write-back only *persists a result*; it cannot fix a request that
never succeeds.

Required change (mirrors the Square branch's explicit "Growth B — json_secret
refresh, larger, not a fold-in"): for the `json_secret` style, **abandon
`oauth2.TokenSource` and hand-roll a JSON POST** in `token_refresh.go` — body
`{grant_type: refresh_token, refresh_token, client_id, client_secret,
redirect_uri}`, `Content-Type: application/json`, creds in the body (never the
Basic header). The `redirect_uri` is reconstructed from the same configured
callback source used to build the authorize URL and the code-exchange
`redirect_uri` (it is fixed per registered app, not per connection, so nothing
new is stored). Decode the JSON token response, preserve the existing
refresh-token carry-forward (`firstNonEmpty(newTok.RefreshToken,
td.RefreshToken)`) and A3 strict write-back of the **rotated** refresh_token, so
the gateway persists the new refresh token before returning. Unit test: a
`json_secret` refresh request carries a JSON body with `grant_type=refresh_token`
+ both creds + `redirect_uri`, and the rotated refresh_token is written back.

**Batch reconciliation — `redirect_uri` divergence from Square (flag to the batch
lead).** The parallel Square branch also lands a `json_secret` JSON-body refresh,
so the batch lead reconciles a *single* shared `json_secret` refresh
implementation — but the two refresh bodies are **NOT identical**. FreshBooks'
official docs REQUIRE `redirect_uri` in the refresh body (verified verbatim:
`{"grant_type":"refresh_token","client_id":"…","refresh_token":"…",
"client_secret":"…","redirect_uri":"YOUR_APP_HTTPS_REDIRECT_URI"}` posted to
`/auth/oauth/token`), whereas Square's `json_secret` refresh body carries no
`redirect_uri`. A literally-single shared implementation must therefore either
(a) always include `redirect_uri` and rely on Square tolerating an extra body
field it ignores, or (b) make `redirect_uri` **conditional per provider** (emit
it only when the provider bundle supplies a configured callback). Do **not** ship
a shared refresh that silently drops FreshBooks' `redirect_uri` (→ refresh
rejected → 401 reconnect within hours) — this is the exact failure #1b guards
against. The reconciliation note must call this out explicitly: confirm Square
tolerates the extra field at reconcile time, else take the conditional-per-
provider path. Flag to the batch lead rather than assuming the branches' refresh
bodies match.

L4 assertion (see §6): seed a connection with a real refresh_token and a
short/expired `expires_at`, run `heliox tool freshbooks -- invoice list`, and
assert the refresh actually rotates and the connection keeps working — proving
#1b end to end, not just that connect succeeded.

**#2 — `refresh_lease: credential` + a REQUIRED runtime-contract allowed-set
growth (not just a bundle field).**
FreshBooks refresh tokens are **single-use rotating** (§1b). With
`refresh_lease: none`, two tool calls for the same connection arriving near-
simultaneously after expiry each call `GET /connections/token`, each read the
same stored refresh_token, and each POST it to FreshBooks; the first rotation
invalidates it, so the second gets a permanent refresh error → spurious
`401 reconnect required`. This race is **Helio-inducible**, not "inherent to
FreshBooks": `refresh_lease: credential` (`OAuthLeaseCredential` in
`model/catalog.go`, honored by `acquireRefreshLease` +
`reloadRefreshSnapshot` in `token_refresh.go`) is the *designed* mechanism that
serializes refreshes per `(provider, credential_id)`. Under it, the loser
blocks on the lease, then re-reads the freshly-written token in
`reloadRefreshSnapshot` and reuses it instead of re-refreshing — the second call
succeeds. `none` is therefore wrong here; a single-use rotating provider is
exactly the `credential`-lease case (precedent: keap, signnow, hootsuite all set
a refresh lease for rotating tokens).

**But the lease enum + machinery existing is necessary, NOT sufficient — this
needs a reviewed integration-service capability growth, exactly like #1a.**
On the worktree base, `model/runtime_contract.go` validates the refresh lease by
**exact equality**: the `RuntimeStrategyStandardOAuth` contract pins
`oauth: {…, refreshLeaseScope: OAuthLeaseNone}` (runtime_contract.go:42), and
`ValidateRuntimeContract` rejects any mismatch —
`definition.OAuth.RefreshLeaseScope != contract.oauth.refreshLeaseScope`
(runtime_contract.go:224–232). A `standard_oauth` bundle declaring
`refresh_lease: credential` therefore **fails provider-gen** with
`requires auth.oauth.refresh_lease "none", got "credential"`. So `credential` is
not currently in the `standard_oauth` allowed set; putting it in the bundle
without growing the gate breaks the build (contradicting L3 "provider-gen regen
clean"). Required change: convert the `standard_oauth` refresh-lease check from
exact-equality to an **allowed-set** that includes `OAuthLeaseNone` **and**
`OAuthLeaseCredential` (model the contract's `refreshLeaseScope` as a permitted-
scope set, or add `OAuthLeaseCredential` to the strategy's allowed scopes), with
a unit test asserting a synthetic `standard_oauth` bundle with
`refresh_lease: credential` now validates and one with an out-of-set scope still
fails. This is the same "standard_oauth refresh_lease allowed-set" growth the
parallel keap / signnow / hootsuite branches perform; the batch lead reconciles a
single allowed-set addition. Do not commit projections.

**#3 — identity userinfo static header `Api-Version: alpha` (REQUIRED — pre-plan,
confirm at L2).** FreshBooks' official Identity Model doc shows the `/users/me`
example request with `Api-Version: alpha` in the headers *explicitly* (verbatim:
`curl -X GET -H 'Authorization: Bearer <…>' -H 'Api-Version: alpha' -H
'Content-Type: application/json' "https://api.freshbooks.com/auth/api/v1/users/me"`).
So this is **very likely mandatory, not optional** — treat the identity
static-header growth as **expected**, not conditional. The
`declarativeIdentityResolver` issues a plain Bearer GET with **no** custom
header, so if the header is required, L5 identity extraction fails silently with
an empty `AccountKey`. Pre-plan a small reviewed `identity.headers`
(static-header) capability on the identity GET now — precedent: adobe-sign
`base_uri` source, docusign userinfo metadata deriver — so L5 is not the first
place the missing header surfaces. Bundle shape (to be projected onto the
identity resolver):

```yaml
identity:
  source: userinfo
  url: https://api.freshbooks.com/auth/api/v1/users/me
  headers:
    Api-Version: alpha
```

**L2 keeps the confirmation** (send `/users/me` with `Authorization` only and see
whether it 4xx's or drops identity fields), but the design assumes the header
**will** be needed and plans the capability accordingly. The **accounting**
endpoints (verified) do **not** require the header, so this is scoped to identity
resolution only (the anycli service sends it on `me`/discovery internally, but
the Helio-side identity resolver needs the capability to send it too).

**Revoke.** FreshBooks revoke (`/auth/oauth/revoke`) also takes a JSON body with
client creds — it does not fit the declarative revoker's form/Basic shapes
cleanly. MVP ships `disconnect_mode: local_only` (notion precedent); a
`provider_revoke` follow-up would need a JSON-body revoker variant and is not
worth blocking the hidden ship. Flag, don't build.

**Authorize scope param — resolved by official docs: scope is NOT an
authorize-URL parameter, so the bundle must NOT declare `scopes:`.** The official
FreshBooks authentication docs show the authorize URL as
`https://auth.freshbooks.com/oauth/authorize/?response_type=code&redirect_uri=<…>&client_id=<…>`
— only `response_type`, `redirect_uri`, `client_id` (plus an optional `state`
that is echoed into the redirect). There is **no `scope` parameter**; the app's
scopes are fixed in the FreshBooks Developer portal app config, which is the
authoritative source of truth.

**Critical: the bundle `scopes:` key is NOT inert.** In provider-gen it maps
`oauthManifest.Scopes` (`cmd/provider-gen/manifest.go:87`) →
`OAuth.DefaultScopes` (`render_go.go:178-180`), and `buildOAuthAuthorizeURL`
appends `q.Set("scope", strings.Join(def.OAuth.DefaultScopes, " "))`
(`service/oauth_start.go:226-227`) whenever `DefaultScopes` is non-empty. So
declaring `scopes:` in the bundle **would inject a `scope=…` param into the
FreshBooks authorize URL** — sending scope strings to an endpoint that per the
official docs expects only `response_type`/`redirect_uri`/`client_id`. This tool
therefore **omits `scopes:` entirely** and declares **only `display_scopes`**,
which maps to `DisplayScopes`/`ConsentScopes` (consent copy) and **never** enters
the authorize URL — matching the notion precedent (notion's bundle has
`display_scopes` and no `scopes`). With `scopes:` omitted, `DefaultScopes` is
empty and no `scope` param is emitted, which is exactly what the design wants.
Only `display_scopes` is "documentation/UI copy"; a populated `scopes:` field is
the opposite of inert, which is precisely why it is left out.

**Caveat:** the concrete `display_scopes` slugs listed in the §4 YAML
(`invoices.rw`, etc.) are **illustrative** consent copy — FreshBooks' exact scope
vocabulary is defined in the portal, not the public auth doc, so treat those
strings as placeholders pending L2 confirmation of the exact identifiers against
a real registered app (they are UI copy only and do not gate the connect path
either way, since they never enter the authorize URL).

## 6. Five-layer test plan

| Layer | What it proves | Needs external creds |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes for `/users/me`, invoice list/create, client list, expense create; asserts Bearer injection, `account_id` resolution (single→silent, multi→exit 2 with `--account` guidance, zero→exit 1), `--json` envelope unwrap, plain+`--json` error rendering, exit codes 0/1/2. | No |
| **L2** `ANYCLI_CRED_ACCESS_TOKEN=… anycli freshbooks -- me` / `invoice list` against the **real** FreshBooks API | Field names, Bearer injection, real request shape; **confirms** the `Api-Version: alpha` header on `/users/me` is mandatory (#3 — the identity.headers capability is pre-planned regardless). Also opportunistically confirms the exact `display_scopes` slugs (the §4 slugs are illustrative UI copy; scope is not an authorize param and `scopes:` is omitted, so this never gates the connect path). (The authorize URL taking no `scope` param, and refresh `redirect_uri` being required, are already confirmed by official docs — not L2 unknowns.) | **Yes** — a real FreshBooks account + a token minted from a dev app (account pool + lane-1 dev app). |
| **L3** `provider-gen --check` + both repos' unit suites (incl. the new `json_secret` synthetic-provider test **and** the `standard_oauth` refresh-lease allowed-set test) | Bundle validity, five projections regen clean, `json_secret` enum wired end to end **for both exchange (#1a) and refresh (#1b)**, the `standard_oauth` runtime contract **accepting `refresh_lease: credential`** (§5 #2 allowed-set growth — without it provider-gen fails), and **numeric-stable-key coercion covers `/response/id`** (§4a) — all proven before implementation, not discovered later. | No |
| **L4** singleton + `POST /internal/test-only/connections/seed` → `heliox tool freshbooks -- invoice list` | Token-gateway → anycli path with a seeded connection; seed `access_token` + `refresh_token` + short/expired `expires_at` to force the **json_secret refresh-and-write-back** path (#1b). **Asserts the refresh actually rotates and the second call keeps working** — the load-bearing proof that FreshBooks connections survive past the first token expiry. | **Yes** — real access+refresh token from the dev app / test account. |
| **L5** `heliox tool freshbooks auth` → FreshBooks consent → `oauth_connected` event → unseeded live run | The actual connect UX: authorize URL, `json_secret` code exchange, `/users/me` identity extraction, notification. oauth L5 = human-in-the-loop. | **Yes** — registered dev app (lane 1) + real FreshBooks account consent. |

Externally-supplied credentials gate **L2, L4, L5**; **L1 and L3 need none**.
Definition of done stays hidden until L1–L4 pass and L5 runs once; the
`visible: true` flip + regenerate is the single go-live change.

## 7. Open items for stage-1/L2 (must close before merge)

1. `json_secret` **initial code exchange** landed + tested (§5 #1a) — required.
2. `json_secret` **JSON-body refresh** in `token_refresh.go` (JSON body with
   `grant_type=refresh_token` + both creds + `redirect_uri`, hand-rolled off
   `x/oauth2`, A3 write-back of the rotated refresh_token) landed + tested
   (§5 #1b) — **required**; this is what keeps connections alive past ~first
   token expiry. L4 asserts a real seeded refresh rotates. **Batch flag:**
   FreshBooks' refresh body REQUIRES `redirect_uri` (verified in official docs)
   whereas the parallel Square `json_secret` refresh does not — the reconciled
   single shared refresh must carry `redirect_uri` (Square tolerating the extra
   field) or make it conditional per-provider; do not let reconciliation drop it
   (§5 #1b batch-reconciliation note).
3. `refresh_lease: credential` set in the bundle **and** the `standard_oauth`
   runtime-contract refresh-lease check grown from exact-equality to an
   allowed-set that admits `OAuthLeaseCredential` (§5 #2) — **required**; the
   base contract pins `refresh_lease: none`, so without the allowed-set growth
   the bundle fails `ValidateRuntimeContract` / provider-gen. Batch-reconciled
   with keap / signnow / hootsuite; ships with a unit test. Serializes single-use
   rotating refreshes, preventing the Helio-inducible double-refresh race.
4. Numeric-stable-key coercion confirmed on the resolver base and covering
   `/response/id`; grow it (batch-reconciled with hubspot) if absent (§4a) —
   verify at L3, before implementation.
5. Identity static-header capability (`identity.headers: {Api-Version: alpha}`)
   pre-planned and landed (§5 #3) — the official `/users/me` example sends the
   header explicitly, so treat it as **required**, not conditional; L2 confirms
   mandatory but the capability is planned up front so L5 identity extraction is
   not the first place a missing header surfaces.
6. ~~Authorize `scope` param~~ — **resolved by official docs**: scope is not an
   authorize-URL parameter; scopes are portal-configured. The bundle therefore
   **omits `scopes:` entirely** (it is NOT inert — it projects to
   `OAuth.DefaultScopes` → a `scope=…` authorize-URL param) and declares only
   `display_scopes` (consent copy, never the authorize URL), matching the notion
   precedent (§5). Residual (non-blocking): L2 confirms the exact
   `display_scopes` slugs, since the §4 slugs are illustrative UI-copy
   placeholders.
7. Revoke stays `local_only` for MVP; `provider_revoke` deferred (§5).
