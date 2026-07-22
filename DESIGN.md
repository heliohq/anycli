# Bill.com (`bill-com`) — per-tool design

Catalog row 149 · anycli id `bill-com` · provider key `bill_com` · lane
`oauth_review` · wave **3-hold** · category Finance
(master plan §4). Flagged in master plan §6 as a **non-standard auth shape**
and an **adapter candidate**; re-laned to the 3-hold holdback batch on
2026-07-21 pending a pre-verify auth-shape decision (§5). This document is that
decision, grounded in the official BILL developer docs and the actual repo code.

Scope note: this is a scratch design file on the `tool/bill-com` branch. The
batch lead strips it at batch-end.

---

## 0. Executive summary + the one divergence that matters

**The catalog lane `oauth_review` does not describe an OAuth authorize flow —
BILL has none.** BILL's v3 developer API authenticates with a **developer key
(`devKey`) + a per-org username / password / organizationId login** that mints
a short-lived **`sessionId`**; both `devKey` and `sessionId` then ride as HTTP
**headers** on every call. There is no `/authorize` redirect, no `code`
exchange, no `access_token`/`refresh_token`, no consent screen. (Sources below.)

So the `oauth_review` label is **not** describing an OAuth app-review gate, and
the earlier "restricted to select partners" reading of it was **too strong**.
Verified against BILL's own `get-started-in-production` page (§3): a **per-org
production devKey is self-service** — sign up at `bill.com/signup`, then
Settings → Sync & Integrations → Manage Developer Keys → Generate developer key.
The genuine human/partner clock applies to the narrower **app-partner /
multi-tenant path** — the "As a BILL app partner…" onboarding model and its **AP
& AR sync token** (§3.1) — not to obtaining a devKey per se. So `oauth_review`'s
real content here is: *(a)* it is not OAuth (no authorize/consent/refresh), and
*(b)* if we adopt the app-partner model (the recommended multi-tenant shape),
that model's onboarding + sync-token issuance carries a partner review clock.
Recording the divergence per the independent-judgment mandate:

| Axis | Catalog says | Official docs say | This design |
|---|---|---|---|
| Auth protocol | `oauth_review` (implies OAuth2 authorize) | devKey + credential login → `sessionId` header | **manual multi-field credentials** (api_key-class), **no OAuth** |
| Helio runtime strategy | (implied `standard_oauth`) | n/a | `manual_credentials` (multi-field) |
| Where the "adapter" lives | "one of the §5 narrow **integration-service** service adapters" (§6) | n/a | **anycli service** owns the login→call session dance; integration-service stays on the existing manual path + one narrow **verifier** — **no** `service/adapter_billcom.go` |
| Production API access | (implied hard partner gate) | **self-serve per-org devKey**; partner gate is on the app-partner/sync-token path only | 3-hold is a *choice* tied to the recommended app-partner model, **not** a hard "no devKey without a partner" constraint (§3, §6) |
| Per-user credential | (n/a) | raw username/password (v3 login) **or** AP & AR sync token (partner, v2 login, no-payments) | **prefer the sync token** in the app-partner model (§3.1/§4.3); raw password only in the single-org fallback |
| L4 seedability | (oauth_review normally seedable as user-token) | credentials are static, api_key-class | **seedable** (multi-field user-token) |

The master plan anticipated a compiled integration-service adapter. Verified
against the code, the cleaner orthogonal split puts the multi-step session
logic **inside the anycli service** (exactly what a `service`-type tool is for —
NetSuite signs OAuth1.0a inside anycli; bill-com logs in inside anycli), and
keeps the Helio side on the **existing multi-field `manual_credentials`** path
plus **one narrow login verifier**. This is subtract-before-add: no new
integration-service lifecycle strategy, no token-exchange/refresh machinery for
a provider that has neither.

---

## 1. Official API surface — what it is, and why these endpoints

**Sources (verified 2026-07-23):**
- BILL v3 auth / keys & tokens: https://developer.bill.com/docs/bill-keys-tokens ,
  https://developer.bill.com/docs/get-started-in-production ,
  https://developer.bill.com/reference/login , getsessioninfo
  (`GET /connect/v3/login/session`).
- Environments: https://developer.bill.com/docs/environmental-differences —
  sandbox `https://gateway.stage.bill.com/connect/v3`, production
  `https://gateway.prod.bill.com/connect/v3` (swap `stage`↔`prod`; sandbox keys
  never work in prod).
- v3 reference index (bills / vendors / payments / invoices / customers /
  organizations): https://developer.bill.com/reference/api-reference-overview .

**Base URL:** `https://gateway.{stage|prod}.bill.com/connect/v3`.

**What an AI teammate actually does with BILL** drives the surface. BILL is an
AP (accounts payable — money owed to vendors) + AR (accounts receivable — money
owed by customers) automation platform. A teammate's real jobs: "what bills are
due this week", "pull the outstanding invoices for customer X", "draft a bill
for this vendor from the attached PDF", "who are our vendors", "is our BILL
session healthy". That maps to **read + draft-create** over five resources plus
session introspection:

| Resource | Endpoints wrapped | Why (teammate job) |
|---|---|---|
| **Bills** (AP) | `GET /bills`, `GET /bills/{id}`, `POST /bills` | list/inspect payables; draft a bill |
| **Vendors** | `GET /vendors`, `GET /vendors/{id}`, `POST /vendors` | vendor lookup; onboard a vendor |
| **Invoices** (AR) | `GET /invoices`, `GET /invoices/{id}`, `POST /invoices` | outstanding receivables; draft an invoice |
| **Customers** | `GET /customers`, `GET /customers/{id}`, `POST /customers` | customer lookup; add a customer |
| **Payments** (AP, money-out) | `GET /payments`, `GET /payments/{id}` | **read only** (see money-movement carve-out) |
| **Session/org** | `GET /login/session`, `GET /organizations` | whoami / health / list login orgs |

**Money-movement carve-out (safety boundary; sourcing hedged).** `POST /login`
mints a **limited-access** session. BILL documents that creating payments /
adding bank accounts needs an elevated ("MFA-trusted") session rather than a
plain login session — but note the **exact** mechanism (whether the gate is
`rememberMeId`+`device`, an MFA step-up, or another trusted-session flow, and
whether it applies to `POST /payments` specifically) **could not be pinned to a
single official page during this pass** (the MFA-trusted doc URL 404s). What
**is** corroborated by the keys-&-tokens doc: a partner **AP & AR sync token**
session has "BILL payment capabilities are not available" (limited access, §3.1).
Either way the design outcome is the same and conservative: **`POST /payments`,
`POST /payments/bulk`, bank-account setup, and `/bulk` money endpoints are OUT OF
SCOPE for v1**; payments are exposed **read-only**. The valuable, low-risk
surface (payables/receivables visibility + draft creation) ships; irreversible
money movement does not. If we adopt the **sync-token** per-user credential
(§3.1/§4.3, the recommended app-partner shape), this carve-out becomes
**automatic** — payments are simply unavailable on that session, so the boundary
no longer rests on the unverified MFA-step-up argument. Revisit payment writes
only if a service-account / MFA-trusted-session story is confirmed at the
pre-verify gate against an official page that actually states it.

---

## 2. anycli definition (stage-1 rubric → `service` type)

**Type decision: `service`.** The `cli`-type rubric (SKILL.md stage 1) requires
an official, non-interactive, `--json`-capable binary provisionable into the
runtime image. BILL ships no such CLI. So this is a `service`-type tool against
the v3 HTTP API — matching 21/23 existing definitions.

- **Definition:** `definitions/tools/bill-com.json`, `name: "bill-com"`,
  `type: "service"`, one-line description.
- **Go package:** `internal/tools/billcom/` (id with dashes dropped — the
  master-plan §3 stage-2 rule, mirroring `microsoft-calendar` →
  `microsoftcalendar/`). `RegisterService("bill-com", &billcom.Service{})` in
  `internal/tools/register.go`.
- **Reference shape to copy:** `internal/tools/notion/` — resource-grouped cobra
  tree, a `BaseURL`/`HC`/`Out`/`Err` struct so httptest fakes can point at a
  fake server and capture output, exit-code contract (0 success, 1
  runtime/API failure via typed `apiError`, 2 usage/parse), `--json` structured
  error envelope.

**Command tree (verbs):**

```
bill-com bill    list|get|create
bill-com vendor  list|get|create
bill-com invoice list|get|create
bill-com customer list|get|create
bill-com payment list|get              # read-only (money-movement carve-out)
bill-com org     list                  # list login organizations
bill-com whoami                        # getsessioninfo: org id, user id, MFA status
```

`list` verbs expose BILL's cursor pagination (`--max`, `--page`/`nextPage`
token) and pass through documented filters; anycli normalizes BILL's response
into a provider-neutral `{"items":[...],"next_page":"..."}` JSON envelope.
`--json` is the default agent output; a compact human table is the fallback.

### 2.1 The session dance — the "adapter" that lives here, not in Helio

anycli is the credential-safe execution engine: it "never imports Helio code
and knows nothing about OAuth, Vault, or connections — it only executes one
embedded definition against a resolver-supplied credential map"
(`references/anycli-development.md`). The bill-com service receives a **static
credential map** and performs the login→call dance itself, per invocation:

1. Read injected env: `BILLCOM_DEV_KEY`, `BILLCOM_USERNAME`,
   `BILLCOM_PASSWORD`, `BILLCOM_ORG_ID`, optional `BILLCOM_ENV`
   (`prod` default | `stage`) selecting the base URL, and `BILLCOM_AUTH_MODE`
   (`v3` default | `sync_token`) selecting the login endpoint (§3.1).
2. Log in → `sessionId`. Endpoint depends on the credential type (§3.1): the
   raw/console credential uses `POST {base}/v3/login` with JSON body
   `{devKey, username, password, organizationId}`; the AP & AR sync-token
   credential uses the v2 login (`POST /v2/Login.json`) per the keys-&-tokens
   callout (pre-verify: confirm against sandbox). A single `BILLCOM_AUTH_MODE`
   env flag (`v3` default | `sync_token`) selects the path; the returned
   `sessionId` rides as a header on the v3 resource calls in both cases.
3. Set headers `devKey: <devKey>`, `sessionId: <sessionId>`,
   `content-type: application/json` on the requested operation and execute it.
4. On `sessionId`-expired errors (35-min inactivity), re-login once and retry —
   idempotent for GETs; POSTs are attempted once (no blind retry of a create).

Each `heliox tool bill-com …` invocation is a fresh process, so it logs in once
per invocation (login + operation = 2 round-trips). This is directly analogous
to NetSuite's per-request OAuth1.0a signing living inside its anycli service —
multi-step auth logic is precisely what a `service`-type tool encapsulates. No
Helio code, no token gateway change for the session: the gateway serves four
static fields; anycli mints the session.

### 2.2 Credential injection (definition `auth`)

Four `CredentialBinding` entries, each a `field` source → `env` inject:

| resolver field | env var | injected as |
|---|---|---|
| `dev_key` | `BILLCOM_DEV_KEY` | login body `devKey` + `devKey` header |
| `username` | `BILLCOM_USERNAME` | login body `username` |
| `password` | `BILLCOM_PASSWORD` | login body `password` |
| `organization_id` | `BILLCOM_ORG_ID` | login body `organizationId` |

(`BILLCOM_ENV` (base URL) and `BILLCOM_AUTH_MODE` (`v3` | `sync_token`, selecting
the login endpoint per §2.1/§3.1) are non-credential env flags projected from the
definition/bundle, not secret resolver fields.) The map keys stay `username` /
`password` regardless of model — Model A projects `token.sync_token_name` →
`username` and `token.sync_token_value` → `password` (§4.3), so anycli is
agnostic to whether the pair is a raw login or a sync token, and to whether each
field originated from Helio app-config or per-user Vault — it only sees the map
(see §4 for the split).

**L1 (unit tests):** httptest fake serving `/login` (asserts JSON body carries
all four fields) + each resource endpoint (asserts `devKey`/`sessionId` headers,
request shape, pagination). Cover: successful login→list, `sessionId`-expiry
re-login-and-retry, `4xx` typed `apiError` rendering in plain + `--json`, and
usage/parse exit-2. Never hit the real API from a unit test.

---

## 3. Credential fields & the exact auth flow (verified)

**Login (POST `/connect/v3/login`), JSON body:**
`devKey` (dev account key, **required**), `username` (**required**),
`password` (**required**), `organizationId` (**required**). The `username` /
`password` pair carries one of three documented credential shapes (see §3.1):
(a) the customer's raw BILL sign-in email + password; (b) an Accountant Console
`consoleApiToken`; or (c) an **AP & AR sync token** name/value pair (partner
onboarding; note the v2-login wrinkle in §3.1). Optional `rememberMeId`+`device`
for an elevated session (money movement only — out of scope).

**Response:** `200` with a BILL-generated **`sessionId`** (scoped to the
`devKey` used).

**Authenticated calls:** headers `content-type: application/json`,
`devKey: <devKey>`, `sessionId: <sessionId>`.

**Session lifetime (verified).** The two credential types have **different**
sessions, and the original draft conflated them:
- **Developer-key login session:** expires after **35 minutes inactivity**
  (keys-&-tokens). No documented 48-hour figure for this session type.
- **AP & AR sync-token session:** "The generated `sessionId` expires when it is
  inactive for **48 hours**, and provides limited access to the BILL API"
  (keys-&-tokens). The 48-hour window belongs to the sync-token session, not the
  plain dev-key session.

The §2.1-step-4 re-login-and-retry design is unaffected either way (both are
inactivity-expiry windows; anycli re-logs-in on an expired `sessionId`).

**Access gate (verified — scoped precisely).** Two distinct things must not be
conflated:
- **(a) Per-org production devKey — self-service.** BILL's own
  `get-started-in-production` page documents: sign up at `bill.com/signup`
  (select Accounts Payable & Receivable), then Settings → Sync & Integrations →
  Manage Developer Keys → Generate developer key → accept terms. **No account
  manager, no approval, no marketplace step** for a single-org devKey.
- **(b) App-partner / multi-tenant path — partner-gated.** The "As a BILL app
  partner, you can onboard your customers…" model and its **AP & AR sync token**
  issuance are the genuine partner clock. This is the true, *narrow* content of
  the `oauth_review` lane for bill-com.

Sandbox (`gateway.stage.bill.com`) is available for dev/L1–L4. Whether L2/L5 are
gated depends on the §4.3 model choice: the single-org self-serve model can
obtain a devKey without partner approval; the recommended app-partner model
takes on the partner clock for prod sync-token issuance (§6). The earlier blanket
"production access is restricted to select partners" label is dropped — it
overstated a gate its own cited source contradicts.

**No OAuth2.** Verified across login reference, keys-&-tokens, and
get-started-in-production: no authorize endpoint, no code exchange, no
refresh token. → `manual_credentials`, not `standard_oauth`.

### 3.1 Credential-type analysis — sync token vs raw password (decision)

BILL's login body accepts more than one `username`/`password` shape, and the
choice is a **material security decision** the original draft skipped. The three
per-user credential types, per keys-&-tokens + get-started-in-production:

| Per-user credential | `username` / `password` fields | Login op | Session access | Who it's for |
|---|---|---|---|---|
| **Raw sign-in** | customer's BILL email + **account password** | `POST /v3/login` | full (payments possible with an elevated session) | any org |
| **Accountant Console** | email + `consoleApiToken` | `POST /v3/login` | full AP/AR in client orgs | Accountant Console users |
| **AP & AR sync token** | `{sync_token_name}` + `{sync_token_value}` | see wrinkle ↓ | **limited — "BILL payment capabilities are not available"**; 48h inactivity | **"BILL app partners… to onboard customers, who can then pull or push key financial data"** |

**The AP & AR sync token is a near-exact match for this design's scope and
model,** and is materially safer than storing the raw password:

1. **Scope fit.** Its stated purpose — partners onboarding customers who then
   "pull or push key financial data for reporting or ERP syncing" — is precisely
   this design's read + non-payment draft-create surface (§1).
2. **Security posture.** The raw password is the customer's **master
   credential**: it can move money, change bank accounts, and do anything in the
   web app. Persisting it in Vault is a strictly worse blast radius than a
   **revocable, named, limited-access** token minted for integration use. If the
   token leaks or is misused, the customer revokes that one token; the master
   password is untouched.
3. **Automatic money-movement carve-out.** Because payment capabilities are
   "not available" on a sync-token session, the §1 carve-out becomes a property
   of the credential, not an argument resting on the (unverified, §1) MFA
   step-up mechanism. Subtract-before-add: the safer credential *removes* a risk
   we were otherwise reasoning our way around.

**Two real wrinkles (why it isn't a free swap):**

- **v2-login wrinkle.** keys-&-tokens carries a callout that AP & AR sync-token
  sign-in must use the **BILL v2 login operation `POST /v2/Login.json`**, while
  the same page's sync-token example shows `POST /v3/login` — the official page
  is **internally inconsistent**. If v2 login is required, the anycli service's
  session dance (§2.1) must POST `/v2/Login.json` for the sync-token credential
  and `/v3/login` for the raw/console credential (the returned `sessionId` still
  rides as a header on the v3 resource calls). **Pre-verify item:** confirm the
  exact login endpoint for the sync token against the sandbox before coding.
- **Partner-scoping wrinkle.** The sync token only exists in the app-partner
  onboarding model (Settings → Sync & Integrations → **Tokens**), so choosing it
  couples us to the app-partner path — the same path that carries the partner
  clock (§3 access gate (b), §6). In the single-org self-serve model there is no
  sync token; the per-user credential is necessarily the raw password.

**Decision.** In the **recommended app-partner model** (§4.3), adopt the **AP &
AR sync token** as the per-user credential (`token.username` =
`sync_token_name`, `token.password` = `sync_token_value` (secret)), and teach
the anycli service the v2-login path for it. Use the **raw password only** as
the explicit fallback for the single-org self-serve model, where no sync token
is available and payments are held read-only by the §1 carve-out (accepting the
larger blast radius as the documented cost of the self-serve shape). Either way
the Helio field set stays a multi-field `manual_credentials` store; only the
verifier's login endpoint and the credential's semantics differ.

---

## 4. Helio provider bundle plan (hidden-first)

### 4.1 Naming (master plan §3)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `bill-com` (flat; no family group) | bundle `tool.command` (defaults to `tool.name`) |
| ② anycli tool id | `bill-com` (dash-case) | `definitions/tools/bill-com.json` |
| ③ provider catalog key | `bill_com` (underscore-case) | bundle dir `integrations/providers/bill_com/` |

②↔③ is a **mechanical dash↔underscore divergence** → **one** `toolToProvider`
entry `"bill-com": "bill_com"` in `helio-cli/internal/toolcred/resolver.go`
(verified: the current map has no mechanical normalization — OQ1 has **not**
landed — so the explicit entry is required; add a `resolve_test.go` case). This
is one of the 24 entries master plan §3 budgets.

### 4.2 `provider.yaml` (shape, hidden-first)

```yaml
schema: helio.provider/v1
key: bill_com
go_name: BillCom
presentation:
  name: Bill.com
  consent_domain: bill.com
  visible: false            # hidden-first; flip is the single go-live change
auth:
  type: credentials
  owner: individual
  # RECOMMENDED app-partner model (§3.1 / §4.3): per-user credential is the
  # AP & AR sync token (revocable, limited-access, no-payments). The single-org
  # self-serve fallback swaps sync_token_name/value → username/password (raw).
  credential_input:         # PER-USER fields → Vault (see 4.3)
    fields:
      - { name: sync_token_name,  label_key: billcom_sync_token_name,  secret: false, required: true }
      - { name: sync_token_value, label_key: billcom_sync_token_value, secret: true,  required: true }
      - { name: organization_id,  label_key: billcom_organization_id,  secret: false, required: true }
    setup_url: https://developer.bill.com/docs/bill-keys-tokens
  required_config_fields: [billcom.dev_key]   # APP/PARTNER-level → integration-service config
identity:
  source: strategy          # derived by the login verifier, not a userinfo GET
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials
credential:                 # outbound projection into the anycli credential map
  fields:
    dev_key:         config.billcom.dev_key      # app/partner-level
    username:        token.sync_token_name        # per-user (Vault) → login username
    password:        token.sync_token_value       # per-user (Vault) → login password (secret)
    organization_id: token.organization_id        # per-user (Vault)
    auth_mode:       "sync_token"                  # selects anycli v2-login path (§2.1)
    account_key:     connection.account_key
tool:
  name: bill-com
  kind: api-key             # clients route the connect drawer by auth_type
```

### 4.3 The credential model — two coherent shapes, one recommended

There are **two axes** here — (i) where the `devKey` lives, and (ii) what the
per-user credential is — and they are **coupled**, because the AP & AR sync token
only exists inside the app-partner path. So the decision collapses to a choice
between two coherent models, not four independent knobs:

**Model A — app-partner + sync token (RECOMMENDED).**
- **devKey → app/partner config (lane 1):** one partner `billcom.dev_key` landed
  in `config/` **and** the `deploy/` Helm Secret together (Config Sync hard
  rule), projected via a `config.billcom.dev_key` source. This is the **exact
  shape of Google Ads' `config.developer_token`** growth and Twitch's
  `config.oauth.client_id` source — a reviewed, existing capability class.
- **Per-user → sync token (connect form → Vault):** `sync_token_name`,
  `sync_token_value` (secret), `organization_id`. Login via the v2 path (§2.1),
  `auth_mode: sync_token`.
- **Why recommended:** materially smaller blast radius than a stored master
  password, automatic money-movement carve-out (§1/§3.1), and it matches the
  onboard-customers use case BILL built the token for.
- **Cost:** takes on the partner clock for prod sync-token issuance (§3 access
  gate (b), §6), and the v2-login wrinkle (§3.1) must be confirmed at pre-verify.

**Model B — single-org self-serve + raw password (fallback).**
- **devKey → per-customer, self-serve.** Because per-org devKeys are self-serve
  (§3), `dev_key` can move into the **Vault field set** (four-field manual; drop
  `required_config_fields`/`config.billcom.dev_key`), or stay in config if the
  operator provisions one devKey. No partner gate to obtain it.
- **Per-user → raw:** `username` + `password` (secret) + `organization_id`, v3
  login (`auth_mode: v3`).
- **Cost:** stores the customer's master credential in Vault (larger blast
  radius); payments held read-only by the §1 carve-out, which then rests on the
  *unverified* MFA-step-up argument rather than an intrinsic session limit.

**Pre-verify decision gate (§5 / stage 1):** pick A vs B, and for the sync-token
path confirm the v2-login endpoint against the sandbox. **Default to Model A**
unless the partner clock is unacceptable for the wave, in which case Model B
ships hidden without a partner gate (at the documented security cost). Either
way the anycli side is unchanged in shape — it always receives
`{dev_key, username, password, organization_id, auth_mode}` and branches the
login endpoint on `auth_mode` (§2.1). The provider bundle switches between the
two by swapping the Vault field set and the `credential.auth_mode` literal.

### 4.4 Integration-service capabilities (reuse-or-grow, no compiled adapter)

Verified against this worktree's base (integration-service `model/catalog.go`:
`CredentialInputPolicy` P3 doc still says "exactly one required field";
`RuntimeStrategyManualCredentials` exists; verifiers registered via the
`manualTokenVerifier` interface, e.g. `declarativeManualTokenVerifier`,
`dsnHostIdentityDeriver`). Two capability needs — **reuse if a sibling
finance/3-hold branch (NetSuite / Plaid / ZoomInfo / PayPal) already merged it,
else grow**, mirroring their pattern:

1. **Multi-field `manual_credentials` (Growth #1, likely shared).** Relax the
   single-required-field constraint to N reviewed fields, and mix a
   `config.*` credential source with `token.*` sources in one projection.
   NetSuite (4-field TBA), Plaid, ZoomInfo, PayPal all need the same; this is
   not bill-com-specific.
2. **`billcomLoginVerifier` (Growth #2, narrow, bill-com-specific).** A compiled
   verifier (precedents: `sproutClientVerifier`, `mastodonAccountVerifier`,
   `zuoraTokenVerifier`, `postmarkServerVerifier`) that at connect time logs in
   (v2 `POST /v2/Login.json` for the sync-token model, v3 `POST {base}/login` for
   the raw model — branch on the same `auth_mode` §2.1 uses) with
   `{config devKey, user credential/org}` → `200`+`sessionId` proves the whole
   credential set before it reaches Vault; then `GET /login/session`
   (getsessioninfo) or `GET /organizations` for a human-readable org name.
   Returns **account_key = `organizationId`** (stable, non-secret) and
   **label = org name** (fallback: `organizationId`). The secret
   (sync-token-value or password) never enters the identity map or Connection
   metadata.

   A verifier is chosen over mongodb-style **no-verify** because a login POST
   cleanly validates all four credentials at once (a wrong password is otherwise
   only discovered at first tool use), and BILL gives us a first-class identity
   (org) to key/label the connection on. It reuses the multi-field verifier
   variant Growth #1 introduces.

**No `service/adapter_bill_com.go`.** The provider needs no bespoke
integration-service lifecycle strategy — the session is minted in anycli, and
the Helio side is the generic manual-credentials store + a narrow verifier.
This is the design's core subtraction versus the master-plan expectation.

### 4.5 Other artifacts

- **Config (lane 1):** append `billcom.dev_key` to `config/` **and** `deploy/`
  Helm Secret together (a *partially* configured provider fails
  integration-service startup; `dev_key` all-absent renders `configured:false`,
  safe to ship hidden). Must land before bill-com's L5.
- **Icon:** `ui/helio-app/src/integrations/icons/bill_com.svg` + hand-register
  in `providerIcons.ts` (never generated).
- **i18n:** `billcom_sync_token_name` / `billcom_sync_token_value` /
  `billcom_organization_id` label keys across all locales (Model A;
  `billcom_username` / `billcom_password` if Model B is chosen at pre-verify);
  `tools.desc.bill_com`.
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/`, plugin version bump — must call out the
  money-movement carve-out (read-only payments; no headless payment creation)
  so the assistant never attempts an MFA-gated write.
- **Generation:** `provider-gen` + `provider-gen --check` from
  `go-services/integration-service`; five projections committed together at
  batch-end (not on this branch — validate locally only).

---

## 5. Test plan — five layers (SKILL.md + `references/integration-testing.md`)

| Layer | What it proves for bill-com | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: login body carries 4 fields; `devKey`+`sessionId` headers on ops; pagination envelope; `sessionId`-expiry re-login-retry; typed `apiError` plain + `--json`; exit 0/1/2. httptest fakes only. | **No** |
| **L2** | `BILLCOM_DEV_KEY=… BILLCOM_USERNAME=… BILLCOM_PASSWORD=… BILLCOM_ORG_ID=… BILLCOM_ENV=stage anycli bill-com -- bill list --max 5` against the **real BILL sandbox** (`gateway.stage.bill.com`). Proves the live login→session→call chain and field/header names. | **YES** — sandbox devKey (self-serve) + a sandbox org login; a sandbox sync token too if testing Model A |
| **L3** | `provider-gen --check` (bundle validates: dir==key, HTTPS setup_url, `manual_credentials`, closed field/enum contract, unique names) + `go test` in both repos (helio-cli incl. new `toolToProvider`/`resolve_test.go` entry; integration-service incl. multi-field policy + `billcomLoginVerifier` tests). helio-cli built against this anycli branch via **local uncommitted `go.mod` replace**. | **No** |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` seeding the per-user fields (Model A: `sync_token_name`/`sync_token_value`/`organization_id`; Model B: `username`/`password`/`organization_id`) — multi-field user-token, **seedable** (api_key-class, not a minted provider) — with `billcom.dev_key` in local `config/cloud.yaml`, then `heliox tool bill-com -- whoami` reaches the live sandbox through the token gateway → anycli login → getsessioninfo. Bypasses the connect UI. | **YES** — same sandbox creds as L2 |
| **L5** | Hidden-still: mint connect intent (`heliox tool bill-com auth`), enter the per-user fields (§4.2) in the **real connect UI** → `billcomLoginVerifier` runs → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool bill-com -- vendor list` through the real token gateway. This is the **api_key key-entry L5 path** (master plan §2), agent-drivable with human fallback — **not** the OAuth-consent path (there is no consent screen). Runs once before the visible flip. | **YES** — sandbox creds + `billcom.dev_key` landed in config |

**Layers needing externally-supplied credentials: L2, L4, L5** — all need a
**BILL sandbox devKey + a sandbox org login** (self-serve to obtain per §3;
Model A additionally needs a sandbox sync token). Procuring the sandbox account
is the account-pool lane-2 item; it is not itself partner-gated. L1 and L3 are
fully agent-runnable now.

**Pre-verify gate exit criteria (before the 3-hold batch starts bill-com):**
(a) confirm the auth shape above against the sandbox (login→sessionId→headers),
**including the sync-token v2-login endpoint** (§3.1) if Model A is chosen;
(b) pick the credential model (§4.3 Model A app-partner+sync-token vs Model B
single-org+raw), which also fixes devKey placement; (c) secure a sandbox devKey
+ test org login (and, for Model A, a sandbox sync token). If (c) cannot be
procured, bill-com is swapped out via the catalog-amendment mechanism (master
plan §6 risk #2), holding the 298 total. In **Model A only**, the **visible
flip** additionally waits on production partner approval for sync-token issuance
(the genuine `oauth_review`-class clock); in Model B the prod devKey is
self-serve so the flip is gated only by hidden-first L5, not a partner clock.
Dev, L1–L4, and the batch-end merge are not gated by partner approval in either
model (hidden-first).

---

## 6. Risks / open items

- **Access gate — scoped, not blanket** (§3, corrected). Obtaining a **per-org
  devKey is self-service** (get-started-in-production), so it does **not** block
  dev/L1–L4 by itself. The genuine partner clock applies to the **app-partner /
  sync-token path (Model A)** — its prod sync-token issuance is what gates the
  **visible flip**. The 3-hold placement is therefore a *choice* coupled to
  recommending Model A (safer credential), **not** a hard "no API without a
  partner" constraint; Model B could ship on self-serve devKeys with no partner
  clock at a documented security cost. Mitigation either way: hidden-first (zero
  code waste); swap via master-plan §6 amendment if sandbox creds can't be
  procured. The earlier "restricted to select partners (verified)" framing
  overstated its own cited source and is retracted.
- **Credential model + devKey placement** (§3.1/§4.3) — the one real design
  decision, gated at pre-verify: Model A (app-partner + AP & AR **sync token**,
  recommended for its smaller blast radius and automatic money-movement
  carve-out) vs Model B (single-org self-serve + raw password). Also confirm the
  sync-token **v2-login endpoint** (`POST /v2/Login.json`) against the sandbox —
  the official page is internally inconsistent (v2 callout vs v3 example). The
  bundle supports both outcomes by swapping the Vault field set + `auth_mode`;
  anycli branches the login endpoint on `auth_mode`.
- **Money movement out of scope** (§1) — payment/bank writes are held read-only.
  Under Model A this is intrinsic (sync-token sessions have "BILL payment
  capabilities are not available"); under Model B it is a deliberate
  conservative boundary (the specific MFA-step-up-for-payments mechanism is
  **not** confirmed from an official page, §1/§4). Must be stated in the
  AI-facing doc so the assistant never attempts an MFA/partner-gated write.
- **Session churn** — one login per invocation (35-min/48-h session, fresh
  process each call). Acceptable; the token gateway caches only the four static
  fields, and anycli caches the resolved credential per its own `CacheUntil`.

---

## 7. Shipped-v1 implementation note (divergence from §4.2–§4.4)

Verified against the **actual worktree base** (branched from `origin/main`),
not the richly-grown base §4 assumed. On this base:

- `manual_credentials` is a **single-secret, no-verify, DSN-class** contract
  (design 317 D5/D8, `runtime_contract.go`): it projects only the existing
  `token.access_token` / `connection.account_key` sources and forbids new
  `CredentialSource` values.
- The `CredentialSource` allowlist has **no** `config.*` or multi-field
  `token.*` sources, `CredentialInputPolicy` P3 still forces **exactly one**
  required field, and there is **no** custom-verifier registry (only
  `declarativeManualTokenVerifier` + `dsnHostIdentityDeriver`). The multi-field
  manual capability + config sources + the precedent verifiers §4.4 named
  (sprout/zuora/postmark/mastodon) live only on **unmerged sibling branches**.

Growing that whole subsystem for one hidden tool would duplicate work multiple
sibling branches independently own and would contradict the documented
single-secret contract. **Subtract before add:** BILL's structured credential
ships as **one JSON secret** through the existing single-secret path — all the
multi-field structure and the entire login→call session dance already live in
anycli (which owns them), so Helio stores one opaque secret and projects it via
`token.access_token`. The narrow growth is a single local, no-network
`billcomCredentialIdentityDeriver` (parses `organization_id` from the JSON for
the account key/label; no secret enters the identity map), mirroring
`dsnHostIdentityDeriver` and selected per-provider in the `manual_credentials`
registration — **no** wire/Vault/CredentialSource/generator/P3 changes, **no**
`config.billcom.dev_key` append (the devKey rides in the per-user JSON secret,
since no config source exists), and **no** connect-time login verifier (the
no-verify contract stands; a bad credential surfaces at first use via AnyCLI's
`CredentialRejected`, exactly like mongodb).

**Deferred to the batch's shared multi-field `manual_credentials` capability**
(before the visible flip): the §4.2 multi-field connect form
(`sync_token_name` / `sync_token_value` / `organization_id`) and the §4.4
Growth #2 connect-time login verifier. The anycli side already accepts either
shape — individual `BILLCOM_*` env vars OR the `BILLCOM_CREDENTIALS` JSON blob —
so adopting the multi-field capability later needs only a bundle swap, no anycli
change. The money-movement carve-out (§1) and the auth-shape divergence (§0)
are unchanged.
