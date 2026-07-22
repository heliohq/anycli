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

So the `oauth_review` label captures the *right operational gate for the wrong
reason*: BILL API access is restricted to **select partners** ("reach out to
your account manager"), which is a real human review/partner clock — but it is
an **API-access partner gate**, not an OAuth app-review gate. Recording the
divergence per the independent-judgment mandate:

| Axis | Catalog says | Official docs say | This design |
|---|---|---|---|
| Auth protocol | `oauth_review` (implies OAuth2 authorize) | devKey + credential login → `sessionId` header | **manual multi-field credentials** (api_key-class), **no OAuth** |
| Helio runtime strategy | (implied `standard_oauth`) | n/a | `manual_credentials` (multi-field) |
| Where the "adapter" lives | "one of the §5 narrow **integration-service** service adapters" (§6) | n/a | **anycli service** owns the login→call session dance; integration-service stays on the existing manual path + one narrow **verifier** — **no** `service/adapter_billcom.go` |
| Wave / scheduling | 3-hold, partner-gated | partner-only API access confirmed | **keep** 3-hold + partner gate |
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

**Money-movement carve-out (auth-model-driven, not arbitrary).** `POST /login`
mints a **limited-access** session. BILL requires an **MFA-trusted API session**
(minted via `rememberMeId`+`device`, or MFA step-up) to **create payments** or
**add bank accounts**. A headless AI teammate cannot complete an interactive MFA
step-up, so **`POST /payments`, `POST /payments/bulk`, bank-account setup, and
`/bulk` money endpoints are OUT OF SCOPE for v1**. Payments are exposed
**read-only**. This is a deliberate safety + feasibility boundary: the valuable,
low-risk surface (payables/receivables visibility + draft creation) ships;
irreversible money movement that also can't authenticate headless does not.
Revisit only if a service-account/MFA-trusted-session story lands at the
pre-verify gate.

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
four-field credential map** and performs the login→call dance itself, per
invocation:

1. Read injected env: `BILLCOM_DEV_KEY`, `BILLCOM_USERNAME`,
   `BILLCOM_PASSWORD`, `BILLCOM_ORG_ID`, optional `BILLCOM_ENV`
   (`prod` default | `stage`) selecting the base URL.
2. `POST {base}/login` with JSON body
   `{devKey, username, password, organizationId}` → `sessionId`.
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

(`BILLCOM_ENV` is a non-credential env flag set by the definition/runtime, not a
resolver field.) anycli is agnostic to whether each field originated from Helio
app-config or per-user Vault — it only sees the map (see §4 for the split).

**L1 (unit tests):** httptest fake serving `/login` (asserts JSON body carries
all four fields) + each resource endpoint (asserts `devKey`/`sessionId` headers,
request shape, pagination). Cover: successful login→list, `sessionId`-expiry
re-login-and-retry, `4xx` typed `apiError` rendering in plain + `--json`, and
usage/parse exit-2. Never hit the real API from a unit test.

---

## 3. Credential fields & the exact auth flow (verified)

**Login (POST `/connect/v3/login`), JSON body:**
`devKey` (dev account key, **required**), `username` (BILL sign-in email,
**required**), `password` (**required**; or `consoleApiToken` for Accountant
Console users), `organizationId` (**required**). Optional `rememberMeId`+`device`
for an MFA-trusted session (out of scope — money movement only).

**Response:** `200` with a BILL-generated **`sessionId`** (scoped to the
`devKey` used).

**Authenticated calls:** headers `content-type: application/json`,
`devKey: <devKey>`, `sessionId: <sessionId>`.

**Session lifetime (verified):** expires after **35 minutes inactivity**;
`sessionId` invalid after **48 hours** inactive; explicit `POST /logout`.

**Access gate (verified):** production BILL API access is granted to **select
partners only** — a real partner/account-manager review clock. This is the true
content of the `oauth_review` lane for bill-com. Sandbox
(`gateway.stage.bill.com`) is available for dev/L1–L4; production keys are
partner-gated and are the L2/L5 blocker (§6).

**No OAuth2.** Verified across login reference, keys-&-tokens, and
get-started-in-production: no authorize endpoint, no code exchange, no
refresh token. → `manual_credentials`, not `standard_oauth`.

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
  credential_input:         # PER-USER fields → Vault (see 4.3)
    fields:
      - { name: username,        label_key: billcom_username,        secret: false, required: true }
      - { name: password,        label_key: billcom_password,        secret: true,  required: true }
      - { name: organization_id, label_key: billcom_organization_id, secret: false, required: true }
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
    dev_key:         config.billcom.dev_key      # app-level
    username:        token.username              # per-user (Vault)
    password:        token.password              # per-user (Vault)
    organization_id: token.organization_id       # per-user (Vault)
    account_key:     connection.account_key
tool:
  name: bill-com
  kind: api-key             # clients route the connect drawer by auth_type
```

### 4.3 The credential split — devKey is app-level (recommended), with a gate

BILL's `devKey` is issued to the **developer/partner** ("sent to you by BILL
when you create a developer account"), while username/password/organizationId
identify the **customer's** BILL org. The natural, precedent-backed split:

- **App/partner-level (integration-service config, lane 1):** `billcom.dev_key`
  — landed in `config/` **and** the `deploy/` Helm Secret together (Config Sync
  hard rule), projected via a `config.billcom.dev_key` credential source. This
  is the **exact shape of Google Ads' `config.developer_token`** growth (an
  app-level developer token injected alongside per-user auth) and Twitch's
  `config.oauth.client_id` source — a reviewed, existing capability class.
- **Per-user (connect form → Vault):** `username`, `password`,
  `organization_id`.

**Pre-verify decision gate (§5 / stage 1):** confirm whether BILL issues **one
partner devKey** (→ config, as above — recommended) or **per-customer devKeys**
(→ move `dev_key` into the Vault field set: four-field manual, drop
`required_config_fields`/`config.billcom.dev_key`). The bundle switches cleanly
between the two; the anycli side is unchanged (it always receives four map
fields). Default to the partner/config model unless the pre-verify proves
per-customer keys.

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
   `zuoraTokenVerifier`, `postmarkServerVerifier`) that at connect time:
   `POST {base}/login` with `{config devKey, user username/password/org}` →
   `200`+`sessionId` proves the whole credential set before it reaches Vault;
   then `GET /login/session` (getsessioninfo) or `GET /organizations` for a
   human-readable org name. Returns **account_key = `organizationId`** (stable,
   non-secret) and **label = org name** (fallback: `organizationId`). The
   password never enters the identity map or Connection metadata.

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
- **i18n:** `billcom_username` / `billcom_password` / `billcom_organization_id`
  label keys across all locales; `tools.desc.bill_com`.
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
| **L2** | `BILLCOM_DEV_KEY=… BILLCOM_USERNAME=… BILLCOM_PASSWORD=… BILLCOM_ORG_ID=… BILLCOM_ENV=stage anycli bill-com -- bill list --max 5` against the **real BILL sandbox** (`gateway.stage.bill.com`). Proves the live login→session→call chain and field/header names. | **YES** — sandbox devKey + a sandbox org login (partner-gated; the 3-hold account-pool blocker) |
| **L3** | `provider-gen --check` (bundle validates: dir==key, HTTPS setup_url, `manual_credentials`, closed field/enum contract, unique names) + `go test` in both repos (helio-cli incl. new `toolToProvider`/`resolve_test.go` entry; integration-service incl. multi-field policy + `billcomLoginVerifier` tests). helio-cli built against this anycli branch via **local uncommitted `go.mod` replace**. | **No** |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` seeding `username`/`password`/`organization_id` (multi-field user-token; **seedable** — api_key-class, not a minted provider) with `billcom.dev_key` in local `config/cloud.yaml`, then `heliox tool bill-com -- whoami` reaches the live sandbox through the token gateway → anycli login → getsessioninfo. Bypasses the connect UI. | **YES** — same sandbox creds as L2 |
| **L5** | Hidden-still: mint connect intent (`heliox tool bill-com auth`), enter username/password/org in the **real connect UI** → `billcomLoginVerifier` runs → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool bill-com -- vendor list` through the real token gateway. This is the **api_key key-entry L5 path** (master plan §2), agent-drivable with human fallback — **not** the OAuth-consent path (there is no consent screen). Runs once before the visible flip. | **YES** — sandbox creds + `billcom.dev_key` landed in config |

**Layers needing externally-supplied credentials: L2, L4, L5** — all blocked on
a **partner-gated BILL sandbox devKey + a sandbox org login** (account-pool lane
2 / 3-hold pre-verify). L1 and L3 are fully agent-runnable now.

**Pre-verify gate exit criteria (before the 3-hold batch starts bill-com):**
(a) confirm the auth shape above against the sandbox (login→sessionId→headers);
(b) resolve the devKey placement gate (§4.3: partner-key-in-config vs
per-customer-in-Vault); (c) secure a sandbox devKey + test org login. If (c)
cannot be procured, bill-com is swapped out via the catalog-amendment mechanism
(master plan §6 risk #2), holding the 298 total. Because access is
production-partner-gated, the **visible flip** additionally waits on production
partner approval (the genuine `oauth_review`-class clock) — dev, L1–L4, and the
batch-end merge are not gated by it (hidden-first).

---

## 6. Risks / open items

- **Partner-gated API access** (verified) is the dominant risk — it blocks L2/L4/
  L5 (sandbox devKey) and the visible flip (prod partner approval). This is why
  bill-com sits in 3-hold. Mitigation: hidden-first (zero code waste); swap via
  §6 amendment if the sandbox key can't be procured.
- **devKey placement** (§4.3) — one real decision, gated at pre-verify; the
  bundle supports both outcomes with no anycli change.
- **Money movement out of scope** (§1) — payment/bank writes need an MFA-trusted
  session a headless agent can't mint; v1 is read + non-money draft-create. Must
  be stated in the AI-facing doc.
- **Session churn** — one login per invocation (35-min/48-h session, fresh
  process each call). Acceptable; the token gateway caches only the four static
  fields, and anycli caches the resolved credential per its own `CacheUntil`.
