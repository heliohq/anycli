# Mercury — per-tool design (`heliox tool mercury`)

Scratch planning doc on branch `tool/mercury`. Batch lead strips it at batch-end.

- **anycli id (axis ②):** `mercury`
- **provider catalog key (axis ③):** `mercury`
- **CLI command word (axis ①):** `mercury` (flat, ungrouped)
- **Auth lane:** `oauth_review` · **Wave:** 2 · **Category:** Finance
- **Catalog row:** 145 of `008-300-integrations-rollout-plan.md`; audit row 147 of `oauth-audit.md`
- **Tool form:** `service` type (HTTP API against `api.mercury.com`; see §3)

Mercury (mercury.com) is a US banking platform for startups and businesses: checking/savings
accounts, ACH/wire/check payments, recipients (payees), treasury (money-market/T-bill yield),
and corporate cards. It exposes a first-party REST **Banking API** (`api.mercury.com/api/v1`)
plus a reviewed **OAuth2** authorization path for third-party integrations that act on behalf of
Mercury customers.

Unlike Melio (the sibling Finance tool), **Mercury's API docs are fully public** — every fact
below is grounded in `docs.mercury.com` and its OpenAPI (`docs.mercury.com/llms.txt`), verified
in this environment. There are only a handful of genuine `‹stage-1›` unknowns (marked inline),
none of which block planning; they resolve once the reviewed OAuth app is registered.

---

## 1. Auth-lane verification — independent, against the official rubric

Catalog + audit both say `oauth_review` (audit row 147: "OAuth supported yes · oauth_review ·
confidence high", evidence `docs.mercury.com/docs/integrations-with-oauth2`). I re-verified
against the official docs and the audit rubric ("multi-tenant authorization-code OAuth → not
`api_key`; self-serve registration → `oauth_light`; human/partner review gate before external
accounts can authorize → `oauth_review`"):

- **Multi-tenant authorization-code OAuth2 exists.** Confirmed against
  `docs.mercury.com/reference/startoauth2flow` + `.../obtainaccesstoken`: one registered client
  redirects arbitrary Mercury customers to `https://oauth2.mercury.com/oauth2/auth`
  (`response_type=code`, PKCE `S256`), then exchanges the code at
  `https://oauth2.mercury.com/oauth2/token` for a scoped, refreshable token. That is exactly the
  "one app, many customer accounts authorize it" shape → **not** `api_key`.
- **A human review gate precedes client issuance.** `docs.mercury.com/docs/integrations-with-oauth2`
  states OAuth access "requires prior approval": submit a form (company details, integration
  use-case, API usage plans) + a **GPG public key** (so Mercury can encrypt the delivered
  credentials) + production/dev redirect URIs + ToS/privacy/logo links; Mercury then reviews on
  "security, use-case fit, and regulatory considerations" and manually issues `client_id` /
  `client_secret` + sandbox creds. "Approval timelines vary." This is the textbook `oauth_review`
  gate — peer to Stripe / Brex / Melio in the catalog.

**Note (not a divergence):** Mercury *also* offers self-serve, single-account API tokens
(`Authorization: Bearer secret-token:<token>`, per `docs.mercury.com/reference/getting-started-with-your-api`).
Those are **per-account only** — the docs explicitly direct account-owners to that path and reserve
OAuth2 "for companies building integrations for Mercury customers." A single-account static token
would technically fit the `api_key` lane, but it cannot serve Helio's multi-tenant model (one Helio
client, many customers' accounts), so the OAuth path is the correct choice and the `oauth_review`
lane stands.

**Verdict: `oauth_review` is correct. No divergence from catalog or audit.** The review gate blocks
**only the visible flip** (master plan §2), never dev / L4 / batch-end merge — Mercury ships
**code-complete but hidden** in its Wave-2 batch, and dev/test-mode app creation (which gates L4)
front-runs review per lane 1.

---

## 2. Which official API surface this tool wraps, and why

Driven by what an **AI teammate** actually does with a company's Mercury account — "what's our
balance", "show last week's transactions", "who did we pay", "categorize/annotate this charge",
and (gated) "pay this vendor" — not by mirroring the whole Banking API. Mercury's object model
(confirmed from the OpenAPI index at `docs.mercury.com/llms.txt`) is
**accounts → transactions → recipients → (treasury, cards, statements)**. The AI-relevant,
read-first surface:

| Resource | AI teammate use | Verbs (planned) | Endpoint (verified from OpenAPI index) |
|---|---|---|---|
| **Accounts** | "what accounts / balances do we have", pick an account to inspect | `account list`, `account get <id>` | `GET /api/v1/accounts`, `GET /api/v1/account/{id}` |
| **Transactions** | "show recent transactions", "did payment X clear", reconcile | `transaction list --account <id>`, `transaction get --account <id> <txId>` | `GET /api/v1/account/{accountId}/transactions` (verified), `GET /api/v1/account/{id}/transaction/{txId}` |
| **Recipients** | "who is recipient X", list payees before paying | `recipient list`, `recipient get <id>` | `GET /api/v1/recipients`, `GET /api/v1/recipient/{id}` |
| **Treasury** | "how much is in treasury / what's the yield" | `treasury get` | `GET /api/v1/treasury` |
| **Cards** | "list the cards on this account" | `card list --account <id>` | account-scoped card list (`getAccountCards`) |
| **Send money** (write, gated) | "pay vendor X $Y from account Z" | `transaction send` / `request-send-money` (deferred, §3) | `POST /api/v1/account/{id}/transactions` / `requestSendMoney` |

**Why these and not more:** they are the nouns a finance teammate reasons over, each maps 1:1 onto
provider-neutral JSON an agent can consume, and read verbs are safe to ship first. **Statement
PDFs** (`getStatementPdf`), **attachment uploads**, and **card lifecycle mutations**
(freeze/cancel/create) are out of the first pass — low agent value or high-blast-radius writes.
**Webhooks** (the platform has them) are **out of scope** for a `heliox tool` passthrough — heliox
is request/response, not a subscriber; event ingest belongs to a Helio service, not this tool.

`‹stage-1›` on exact paths: `GET /api/v1/accounts` and `GET /api/v1/account/{id}/transactions`
are verified verbatim from the docs; the singular-`account`/`recipient`-detail and single-transaction
paths follow Mercury's documented pattern but should be re-confirmed against the live OpenAPI at
implementation time (the anycli L2 harness run confirms them for real).

---

## 3. anycli definition

**Type: `service`.** Mercury does advertise a "terminal-native CLI" on `mercury.com/api`, but it
does not clear the SKILL.md stage-1 `cli`-type bar: that bar requires a binary that takes
credentials by **env/flag injection** and is non-interactive — Mercury's CLI is built around its
own interactive login/session, not a clean "inject one OAuth Bearer and run" model, and provisioning
a third-party fintech binary into the runtime image for one provider is not justified. So implement
`service` type in `internal/tools/mercury/` against the documented HTTP API — matching all 5 shipped
Finance/payments precedents (stripe, xero, sage, freshbooks, quickbooks are all `service` type).
Package name `mercury` (no dashes/leading digit → no normalization).

- **`definitions/tools/mercury.json`:** `name: "mercury"`, `type: "service"`, one-line description,
  and a single `auth` credential binding — `access_token` (field) injected as env var
  `MERCURY_ACCESS_TOKEN` (`type: env`). The service reads it and sends
  `Authorization: Bearer <access_token>`. **Verified:** OAuth access tokens use the plain Bearer
  scheme (`Authorization: Bearer <access_token>`, per `.../reference/using-the-access-token`) — the
  `secret-token:` prefix in the getting-started page applies **only** to the self-serve static API
  tokens, NOT to OAuth tokens. Do not prepend `secret-token:` to the OAuth token.
- **`internal/tools/mercury/`:** cobra tree grouped by resource, copying the **notion service**
  shape (the reference impl per `references/anycli-development.md`): a `BaseURL`/`HC`/`Out`/`Err`
  struct so tests point at an `httptest` server and capture output; documented exit-code contract
  (0 success / 1 API-or-runtime failure via typed `apiError` / 2 usage-parse); `--json` on every
  subcommand emitting a structured envelope, plus a structured `--json` error envelope. BaseURL
  default `https://api.mercury.com/api/v1`.

**Subcommands (verbs), read-first:** `account list`, `account get <id>`,
`transaction list --account <id> [--limit --offset --start --end]`,
`transaction get --account <id> <txId>`, `recipient list`, `recipient get <id>`, `treasury get`,
`card list --account <id>`. **Money-movement writes** (`transaction send`, `request-send-money`,
recipient create/update, internal transfer) are **deferred to a second pass** — Mercury moves real
money; ship read-first and gate any send verb behind explicit confirmation semantics + a stage-1
review of Mercury's idempotency-key requirement and the approval-request flow
(`send-money-approval-requests`) before enabling.

**JSON output shape:** provider-neutral, matching the sibling tools — `{ "data": [...] }` for list
verbs, `{ "data": {...} }` for get verbs; Mercury pagination surfaced as flags (`--limit`,
`--offset` — Mercury's `listAccountTransactions` documents limit/offset-style paging, `‹stage-1›`
confirm exact param names/date filters at L2); `--json` error envelope on failure. No raw Mercury
response passthrough — normalize amounts/status into flat fields an agent can filter on.

---

## 4. Credential fields & the exact auth flow (oauth_review)

**Registration model:** one Helio-owned Mercury OAuth **client**, registered via Mercury's reviewed
application (`docs.mercury.com/docs/integrations-with-oauth2`) — yields `client_id` + `client_secret`
(delivered GPG-encrypted). Mercury issues sandbox test credentials at the same time, so a
**dev/sandbox app exists pre-review** — that is what makes L4 runnable before the visible flip
(review clearance gates only the flip, per §1 + master plan §2).

**Flow (authorization code + PKCE `S256`), all endpoints verified:**
1. `heliox tool mercury auth` mints a connect intent; integration-service redirects the user to
   **`https://oauth2.mercury.com/oauth2/auth`** with `client_id`, `redirect_uri` (Mercury's docs
   spell it `redirect_url` in the authorize step — the value must exactly match a registered URI),
   `response_type=code`, `scope` (space-separated), `state` (≥8 chars, CSRF), and PKCE
   `code_challenge` + `code_challenge_method=S256`.
2. User consents on Mercury's consent screen (per Mercury customer account); Mercury calls back
   with `code`.
3. integration-service POSTs to **`https://oauth2.mercury.com/oauth2/token`**,
   `Content-Type: application/x-www-form-urlencoded`, body
   `grant_type=authorization_code&code=…&redirect_uri=…&code_verifier=…`, authenticating the client
   with **HTTP Basic** (`Authorization: Basic base64(client_id:client_secret)` — the docs' `-u
   "client_id:client_secret"`). Response JSON: `access_token`, `token_type` ("Bearer"),
   `expires_in` (3600 = **1 hour**), `refresh_token`, `scope`.
4. Token gateway serves `access_token` to the resolver; anycli injects `MERCURY_ACCESS_TOKEN`.

**Token semantics (verified):** access token lives **1 hour**; refresh token lives **720 hours and
is single-use** — refreshing (same `/oauth2/token` endpoint, `grant_type=refresh_token` +
`refresh_token` + `scope`, still HTTP-Basic client auth) returns **both a new access token and a new
refresh token**. Rotating single-use refresh ⇒ bundle **`refresh_lease: credential`** (same axis
Xero/Sage/FreshBooks/QuickBooks already use). The `offline_access` scope MUST be requested at
authorize time to receive a refresh token at all.

**Scopes (verified):** `read` (accounts + transactions) and `offline_access` (refresh tokens),
space-separated. A distinct write scope governs money movement (the docs reference a
send-money capability, `‹stage-1›` exact scope string — likely `RequestSendMoney` / a `write`-class
scope); it is **not requested** in the read-first bundle and only added when the gated send verbs
ship. `display_scopes: [read, offline_access]` for v1.

**Credential fields the bundle declares** (never real values — those go in integration-service
config): `required_config_fields: [oauth.client_id, oauth.client_secret]`. No partner-level static
header is required (Bearer token is the whole auth story).

---

## 5. Helio provider bundle plan (hidden-first)

`integrations/providers/mercury/provider.yaml`, modeled on the `notion` standard_oauth bundle
(directory name = key = `mercury`; generator enforces equality). **All enum values below are
confirmed present in the generator's closed sets on this branch's base** (see §5a) — so this is a
**pure golden-path `standard_oauth` bundle with ZERO integration-service Go changes**, peer to
Stripe/Xero/Sage. Concrete skeleton:

```yaml
schema: helio.provider/v1
key: mercury
go_name: Mercury

presentation:
  name: Mercury
  description_key: mercury
  consent_domain: mercury.com
  visible: false                               # hidden-first (SKILL.md stage 4/10)
  order: <next-free>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://oauth2.mercury.com/oauth2/auth      # verified
    token_url: https://oauth2.mercury.com/oauth2/token          # verified
    token_exchange_style: form_basic            # verified: HTTP Basic client auth, form body
    pkce: s256                                  # verified: code_challenge_method=S256
    display_scopes: [read, offline_access]      # verified
    single_active_token: false
    refresh_lease: credential                   # verified: rotating single-use refresh token

identity:
  source: userinfo                              # no token_response identity fields; see §5b
  url: https://api.mercury.com/api/v1/accounts  # ‹stage-1› pointer stability, see §5b
  stable_key: /accounts/0/id                    # ‹stage-1› — or fingerprint deriver fallback
  label_candidates: [/accounts/0/name, /accounts/0/nickname]

connection:
  mode: isolated
  disconnect_mode: local_only                   # no documented OAuth revoke endpoint (§5c)
  runtime_strategy: standard_oauth              # golden path — zero provider Go

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: mercury
  kind: oauth
```

- **Three axes:** ① `mercury` ② `mercury` ③ `mercury` — all identical ⇒ **no `toolToProvider`
  resolver entry** (identity holds; confirmed against `resolver.go` — the map only holds the
  microsoft-*/google-* divergences) and no grouped `tool.group`.

### 5a. Capability check — no integration-service growth needed

Every enum Mercury needs is already in the closed sets on this branch's base (verified by grep):
`token_exchange_style: form_basic` (`validate.go` allows `form_secret|form_basic|json_basic`),
`pkce: s256` (`validate.go` allows `none|s256`; `oauth_start.go` emits `code_challenge_method=S256`),
`refresh_lease: credential` (`validate.go` allows `none|credential|provider`),
`identity.source: userinfo` with `identity.url` (`validate.go` allows `userinfo|token_response|strategy`
and permits `url` only for `userinfo`), `disconnect_mode: local_only` (allowed; and correctly
**forbids** an `auth.oauth.revoke`, matching Mercury having no revoke endpoint). `expires_in` is
returned in the token response, so **no assumed-TTL growth** (unlike Stripe/Salesforce). **Net: zero
new capability, zero adapter.** If stage-1 uncovers a non-standard response dialect, reconsider — but
nothing in the verified docs suggests one.

### 5b. Identity — the one real design decision

Mercury has **no `userinfo`/`/me` endpoint** and the token response carries no identity fields, so
the connection's stable key/label must come from a lightweight authenticated GET. Plan: point
`identity.url` at `GET /api/v1/accounts` and extract `/accounts/0/id` as `stable_key` with
`/accounts/0/name` (+ `/nickname`) as label candidates. **Risk:** an org can hold multiple Mercury
accounts and list ordering is not contractually stable, so `accounts[0]` could drift. This is
acceptable for hidden-first, but flag two fallbacks at stage 1, **both already-existing capabilities
(no growth):** (a) the **fingerprint identity deriver** (used by knock/paperform/paddle) hashing the
accounts response into a stable per-connection key; or (b) if Mercury's accounts payload exposes an
org/business id field, pointer to that instead. Decide at L2 once the real `/accounts` shape is in
hand.

### 5c. Disconnect / revoke

No OAuth token-revoke endpoint is documented in Mercury's OAuth reference, so
`disconnect_mode: local_only` (Helio drops the stored credential; Mercury access lapses when the
refresh token expires or the user revokes in Mercury's dashboard). Confirm at stage 1 whether a
revoke endpoint exists; if one appears, switch to `provider_revoke` + `auth.oauth.revoke` (a
declarative revoker — still no Go).

- **Config Sync:** `oauth.client_id` / `oauth.client_secret` land in **both** `config/` and the
  `deploy/` Helm Secret together (a *partially* configured provider fails integration-service
  startup; a fully-absent one renders `configured: false` and is safe hidden) — lane-1's landing,
  before Mercury's L5.
- **UI icon:** `ui/helio-app/src/integrations/icons/mercury.svg` + register in `providerIcons.ts`
  (manual, never generated) + i18n label for `description_key: mercury`.
- **AI-facing doc:** provider sub-doc under `agents/plugins/heliox/skills/tool/`, published on the
  batch-end plugin version bump.

---

## 6. Test plan — five layers

| Layer | Mercury-specific plan | External creds needed? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: `httptest` fake per verb (account/transaction/recipient/treasury/card); assert request path (`/api/v1/accounts`, `/api/v1/account/{id}/transactions`, …), injected `Authorization: Bearer <token>` (plain, **no** `secret-token:` prefix), `--json` + plain error envelopes, exit codes 0/1/2. No live API. | No |
| **L2** harness real-API | `ANYCLI_CRED_ACCESS_TOKEN=<sandbox/dev token> anycli mercury -- account list` (and each verb) against Mercury's **real sandbox** (`docs.mercury.com/reference/using-mercury-sandbox`) — the mandatory gate before pinning; also confirms the `‹stage-1›` detail paths, pagination params, and `/accounts` identity shape for real. | **Yes** — Mercury sandbox token from the registered dev app (lane 1/2) |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` against the `mercury` bundle; `helio-cli` + integration-service unit suites. Point `helio-cli/go.mod` at the anycli branch via local `replace` (uncommitted). No projections committed on the tool branch (batch lead owns the canonical regen). | No |
| **L4** singleton + seeded token | Start singleton (`env: dev`); `POST /internal/test-only/connections/seed` a **real** Mercury OAuth `access_token` + `refresh_token` with a deliberately short `expires_at` (forces the refresh-and-write-back path, since Mercury access tokens are 1h) for provider `mercury` against a **real seeded assistant/org**; run `heliox tool mercury -- account list` through the real token gateway. Success = live Mercury data returned. | **Yes** — real OAuth token from the dev app (lane 1) |
| **L5** full connect flow | `heliox tool mercury auth` → Mercury `oauth2.mercury.com` consent on the dev/sandbox app → `oauth_connected` event on the originating channel → unseeded live `heliox tool mercury -- account list`. Human-in-the-loop (`oauth_review` → lane 3). Runs once, still hidden, before the visible flip; **visible flip additionally gated on Mercury review clearance**. | **Yes** — human consent on a real Mercury account + review clearance for the flip |

**Credential-gated layers: L2, L4, L5.** L1/L3 are agent-runnable now. L2/L4/L5 all depend on lane-1
Mercury OAuth app registration (dev/sandbox app suffices for L2/L4; review clearance is needed only
to *flip visible*, not to run L5).

---

## 7. Sources & open items

**Confirmed (official — `docs.mercury.com`, verified this session):**
- OAuth authorize `https://oauth2.mercury.com/oauth2/auth`, token `https://oauth2.mercury.com/oauth2/token`;
  `response_type=code`, PKCE `S256`, HTTP-Basic client auth, `application/x-www-form-urlencoded`
  body; response `access_token`/`token_type`/`expires_in`/`refresh_token`/`scope`
  (`/reference/startoauth2flow`, `/reference/obtainaccesstoken`).
- Access token 1h; refresh token 720h, single-use, rotates on refresh; `offline_access` required
  for refresh (`/reference/obtain-the-tokens`).
- Scopes `read`, `offline_access` (`/reference/start-oauth2-flow`).
- Reviewed client issuance ("requires prior approval", GPG key, redirect URIs, sandbox creds)
  (`/docs/integrations-with-oauth2`).
- API base `https://api.mercury.com/api/v1`; OAuth token used as plain `Authorization: Bearer`
  (`/reference/getting-started-with-your-api`, `/reference/using-the-access-token`).
- Resource inventory (accounts, `getAccount`, `getAccountCards`, statements, transactions
  `GET /account/{accountId}/transactions`, `getTransaction`, `createTransaction`/`requestSendMoney`,
  recipients CRUD, treasury, cards) — OpenAPI index `docs.mercury.com/llms.txt`.

**`‹stage-1›` open items (resolve at L2 / before writing request shapes):** exact
detail-resource paths (`/account/{id}`, `/recipient/{id}`, single-transaction) and transaction
pagination/date-filter param names; the `/accounts` payload's identity fields → final `stable_key`
choice (pointer vs fingerprint deriver vs org-id, §5b); the write/send-money scope string; whether a
token-revoke endpoint exists (→ `disconnect_mode`). None block L1/L3 or planning.

**Risk flag for stage 1 / batch lead:** none material. Docs are public, sandbox exists, and the auth
shape is the plain golden `standard_oauth` path — the only gating dependency is the reviewed OAuth
app (lane 1), which per the master plan front-runs the batch; review clearance gates only the
visible flip, not dev/L4/merge. Mercury is a clean Wave-2 `oauth_review` tool.
