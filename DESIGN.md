# Snov.io — per-tool integration design

Scratch design for the `snov` tool (batch lead strips this at batch-end).

- **anycli id (axis ②):** `snov`
- **provider key (axis ③):** `snov`
- **CLI command word (axis ①):** `snov`
- **auth lane:** `api_key` (catalog row 73, Wave 2, Sales Engagement)
- **anycli branch:** `tool/snov` · **Helio branch:** `tool/snov`
- **tool form:** `service` type (stage-1 rubric — see §2)

All three axes are the identical token `snov` → **no `toolToProvider`
divergence entry, no grouped family**. `ProviderFor("snov")`/`ToolFor("snov")`
return identity.

---

## 1. Audit verdict vs official docs (independent verification)

**Catalog lane:** `api_key`. **Audit verdict (row 75):** "no viable
multi-tenant path → stays api_key per rubric."

**Verified against official docs** (`https://snov.io/api`,
`https://snov.io/api-pricing`): **CONFIRMED, no divergence.** Snov.io's only
authentication is a **two-legged OAuth2 `client_credentials` grant** — the
account owner's own `client_id` + `client_secret` (from Snov account settings)
are exchanged for a short-lived bearer token. There is **no authorization-code
/ user-consent multi-tenant authorize flow** — no single registered app that
arbitrary Snov accounts could authorize. Under the audit rubric ("a tool moves
to an OAuth lane only when the provider offers a multi-tenant
authorization-code OAuth flow"), `client_credentials` with per-account
customer-supplied app secrets is exactly the api_key shape. The lane is correct
as written; nothing to record in the amendment log.

**Consequence for Helio wiring:** this is NOT the `standard_oauth` runtime
strategy (no Helio-registered client, no authorize URL, no Helio-side refresh).
The user pastes their own two secrets; the token exchange happens **inside
anycli** at invocation time. Helio only stores + injects the two secrets
(`manual_credentials`), exactly as it stores a Mongo DSN today.

**Account-pool note (feeds human lane 2/§5 risk):** API access + the
`client_id`/`client_secret` pair require a **paid Snov plan** (Starter $39/mo
and up); the free Trial excludes "API & webhooks access." L2/L4/L5 therefore
need a paid test account, not a free signup. Flag for the account-pool budget.

---

## 2. Tool form decision — `service` type

`cli` type is rejected (no official Snov binary exists). `service` type per the
stage-1 rubric, and it is additionally forced by two Snov-specific shapes that
require code, not a passthrough binary:

1. **Runtime token exchange.** Every call must first
   `POST /v1/oauth/access_token` (form body `grant_type=client_credentials`,
   `client_id`, `client_secret`) → `{access_token, token_type: Bearer,
   expires_in: 3600}`, then send `Authorization: Bearer <token>`. The service
   performs this exchange once per process invocation and caches the token
   in-memory for the invocation (well within the 3600s life). This is provider
   business logic anycli must own; anycli stays credential-safe (it receives
   only the two user secrets, knows nothing of Helio).
2. **Async start/result tasks.** The v2 workhorse endpoints are asynchronous:
   `POST /v2/<resource>/start` returns a `task_hash` + `links.result`, and the
   caller polls `GET /v2/<resource>/result/{task_hash}` until
   `status: completed`. An agent-facing tool must **hide this**: the service
   wraps start → bounded poll → final result into ONE synchronous blocking
   command with a timeout, so the AI issues one command and gets the finished
   payload (never a raw `task_hash` it has to re-poll). A `--async` escape
   hatch that returns the `task_hash` immediately is optional/out of scope for
   v1.

Reference implementation to copy the shape of: `internal/tools/notion/`
(cobra tree grouped by resource; `BaseURL`/`HC`/`Out`/`Err` struct for
httptest injection; exit codes 0 success / 1 runtime-API / 2 usage; `--json`
structured error envelope). The token-exchange + poll helpers live in a
`client.go` (cf. `internal/tools/bitly/client.go`).

---

## 3. API surface wrapped — driven by what an AI teammate does

Base URL `https://api.snov.io`. Snov is a **sales-intelligence / cold-outreach**
platform; an AI teammate's real jobs are: *find* a prospect's business email,
*verify* it's deliverable, *enrich* a person/company, *organize* prospects into
lists, and *inspect/manage* outreach campaigns. Rate limit is 60 req/min — the
poll loop must respect it (bounded interval + attempt cap).

Proposed cobra tree (subcommands = verbs grouped by resource):

| Group / verb | Endpoint(s) | Why an AI needs it |
|---|---|---|
| `email find domain` | `POST /v2/domain-search/start` → `GET /v2/domain-search/result/{hash}` | All emails for a company domain |
| `email find by-name` | `POST /v2/emails-by-domain-by-name/start` → result | Find a specific person's email from name + domain |
| `email count` | `POST /v1/get-domain-emails-count` | Free pre-check of how many emails a domain has |
| `email verify` | `POST /v2/email-verification/start` → result | Confirm an address is deliverable before sending |
| `enrich by-email` | `POST /v1/get-profile-by-email` | Enrich a person from a known email |
| `enrich company` | `POST /v2/company-domain-by-name/start` → result | Resolve a company name to its domain |
| `enrich linkedin` | `POST /v2/li-profiles-by-urls/start` → result | Profile/contact data from LinkedIn URLs |
| `prospect add` / `prospect get` / `prospect list` | prospect-management endpoints (`/v1/add-prospect-to-list`, find-by-id/email, `/v1/lists`) | Save + retrieve prospects and user lists |
| `campaign list` / `campaign recipients` / `campaign stats` | multi-channel campaign read endpoints | Inspect outreach status/analytics |
| `account balance` | `GET` user balance/credits (v1 `get-balance` family — exact path confirmed at stage-2 build against live docs) | Report remaining credits; doubles as the connectivity/identity check |

**Scope for v1 of the tool:** ship `email` (find + verify), `enrich`, and
`account balance` first (the highest-value, read-mostly, agent-clear surface).
`prospect` and `campaign` write verbs are a natural follow-up but not required
for the definition-of-done; the design keeps their group names reserved so the
tree is stable. Write/consumption endpoints (`email find`, `verify`, `enrich`)
consume Snov credits — surface that in `--json` output and the AI-facing doc so
the assistant doesn't burn credits blindly.

---

## 4. anycli definition

`definitions/tools/snov.json`:

```json
{
  "name": "snov",
  "type": "service",
  "description": "Snov.io sales intelligence — email finder, verifier, enrichment, prospects (OAuth client_credentials)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "client_id"},
        "inject": {"type": "env", "env_var": "SNOV_CLIENT_ID"}
      },
      {
        "source": {"field": "client_secret"},
        "inject": {"type": "env", "env_var": "SNOV_CLIENT_SECRET"}
      }
    ]
  }
}
```

- **Go package:** `internal/tools/snov/` (id has no dashes/leading digit →
  package name == id). Registered `RegisterService("snov", &snov.Service{})`
  in `internal/tools/register.go`.
- **Two credential fields**, both `secret`: `client_id` and `client_secret`.
  Injected as env vars; the service reads them, does the client_credentials
  exchange, and never sees a Helio concept.
- **JSON output:** every command supports `--json` (default machine-readable
  envelope for agents); async commands emit the *resolved* result, not the
  intermediate task. Exit-code contract per notion (0/1/2), typed `apiError`
  for Snov error bodies, and a distinct exit/message for `client_credentials`
  rejection (bad/expired secrets) so the token gateway feedback loop is clean.

---

## 5. Credential fields & exact auth flow

**What the user provides:** two secrets from Snov *Settings → API* —
`client_id` (called "API user ID") and `client_secret` ("API secret key").
Both are app-level account credentials, not user tokens; neither is a Helio
config value (Helio registers no Snov app).

**Flow, end to end:**

1. User pastes `client_id` + `client_secret` into the Helio connect drawer
   (api_key key-entry path). Stored write-only via
   `POST /connections/credentials` into Vault; nothing lands in the bundle or
   repo.
2. At `heliox tool snov -- …`, the token gateway serves the provider-neutral
   credential map `{client_id, client_secret}` to anycli's resolver.
3. anycli injects them as `SNOV_CLIENT_ID` / `SNOV_CLIENT_SECRET`. The snov
   service `POST /v1/oauth/access_token` (form:
   `grant_type=client_credentials`, `client_id`, `client_secret`) → bearer,
   cached for the invocation, then calls the real endpoint(s) with
   `Authorization: Bearer`.

**Helio does not manage the bearer token** — there is no Helio-side refresh
lease, no `expires_at` on the stored credential; the two secrets are long-lived
and the 3600s bearer is anycli's ephemeral concern. This is the `mongodb`
manual-secret model extended to two fields, which already exists on main
(multi-field `manual_credentials`, confirmed via the mixpanel/amplitude work).

---

## 6. Helio provider bundle plan (`integrations/providers/snov/provider.yaml`)

Hidden-first (`presentation.visible: false`). Axis ③ dir/key = `snov`.

```yaml
schema: helio.provider/v1
key: snov
go_name: Snov

presentation:
  name: Snov.io
  description_key: snov
  consent_domain: snov.io
  visible: false            # hidden-first; flip is the single go-live change

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: client_id
        label_key: snov_client_id
        secret: true
        required: true
      - name: client_secret
        label_key: snov_client_secret
        secret: true
        required: true
    setup_url: https://app.snov.io/account#api   # Settings → API (secrets page)

identity:
  source: strategy          # no HTTPS endpoint that verifies the raw secrets
                            # (verification needs the client_credentials
                            # exchange first) — derive account_key = client_id

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    client_id: token.client_id
    client_secret: token.client_secret
    account_key: connection.account_key

tool:
  name: snov
  kind: api-key             # wire-compat value; client routes drawer by auth_type
```

**Axis naming (master plan §3):** ① `snov`, ② `snov`, ③ `snov` — identical, so
no `toolToProvider` entry and no `toolGroups` membership. This is a flat,
ungrouped provider.

**Identity / account_key = `client_id`.** The `client_id` is a stable,
human-recognizable, **non-secret** account identifier (unlike the connection
string, it is not sensitive on its own) — the right stable key + label, derived
by a `strategy` deriver (precedent: mongodb host deriver, crisp keypair
deriver, amplitude first-colon-split deriver). It avoids storing a hash and
avoids a wasted verify round-trip.

**Verification — Option A (recommended, hidden-first): no-verify**, exactly
like mongodb. Bad secrets are stored and surface at first use via anycli's
`client_credentials` rejection → the token-gateway feedback path. Zero new
integration-service code.

**Option B (follow-up, not blocking): a `client_credentials` verifier
capability** — integration-service does the token exchange (+ a cheap
`account balance` GET) at connect time to validate before storing, mirroring
the semrush/moz/fullstory verifier-capability precedents. Deferred: it adds a
provider-specific exchange path in integration-service for marginal connect-time
UX; the no-verify path is sufficient to reach `visible`. **Decision: ship
Option A; open Option B only if connect-time validation is required for the
flip.**

**Capability dependency (must exist on main before the bundle merges):**
multi-field `manual_credentials` with two `secret` fields projecting to
`token.<field>` — already landed (task set around mixpanel #233). No new
capability growth is required for Option A. If it were somehow absent, this
tool would block on it exactly as mixpanel did; flagged here per stage-1.

**Also required (not generated):**
- UI icon `ui/helio-app/src/integrations/icons/snov.svg` + register in
  `providerIcons.ts` (manual).
- i18n: `tools.desc.snov` and the two `credential_input` `label_key`s
  (`snov_client_id`, `snov_client_secret`) across all locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` documenting the
  two-secret setup, the credit-consuming verbs, and the async-is-hidden
  behavior; plugin version bump + marketplace publish at batch end.
- Config Sync: **none** — `manual_credentials` needs no
  `required_config_fields`, so there is no integration-service `config/` +
  `deploy/` Secret append for this provider (contrast the oauth lane). This
  provider renders `configured: true` with no env, since the user supplies all
  secrets.

---

## 7. Test plan — five layers (per `references/integration-testing.md`)

| Layer | What it proves for `snov` | Needs external creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes for: the `/v1/oauth/access_token` exchange (asserts form body + Bearer injection on downstream calls), each wrapped endpoint's request shape, the **async start→poll→result loop** (fake returns `in progress` then `completed`), and both plain + `--json` error rendering incl. `client_credentials` rejection | No |
| **L2** dev harness vs REAL API | `ANYCLI_CRED_CLIENT_ID=… ANYCLI_CRED_CLIENT_SECRET=… anycli snov -- email verify --email …` returns real data — proves field names, env injection, live token exchange, and real async timing all match | **Yes** (paid Snov account: client_id + client_secret) |
| **L3** `provider-gen --check` + both repos' suites | bundle strict-decodes; five projections regenerate clean; helio-cli + integration-service unit suites green | No |
| **L4** singleton + seed + `heliox tool snov -- …` | seed BOTH secrets via `POST /internal/test-only/connections/seed` (multi-field manual-credential user-token row is seedable), then run through the real token gateway → anycli → live Snov API | **Yes** (same paid creds, seeded) |
| **L5** api_key key-entry connect path | open connect link → paste client_id + client_secret in the real drawer → `GET /connections` shows connected/configured → one **unseeded** live `heliox tool snov` run succeeds. Agent-drivable (agent-browser); human lane 3 fallback | **Yes** (same paid creds) |

**Layers needing externally supplied credentials: L2, L4, L5** — all satisfied
by one paid Snov account's `client_id`/`client_secret` from the account pool.
L1 and L3 are fully self-contained (no creds, no live API).

**L5 note:** this is the **key-entry** L5 path (not OAuth consent) — the
master-plan §2 api_key checklist. No `oauth_connected` system event; the
completion signal is the successful unseeded live run through the real drawer.

---

## 8. Open items to resolve at stage-2 build (not blockers)

1. **Exact balance endpoint path** (`GET /v1/get-balance` vs a `/v2/user/…`
   path) — confirm against live docs when wiring `account balance`; it's the
   connectivity check, so pin it precisely.
2. **Async poll budget** — interval + max attempts + overall timeout for
   start/result loops, chosen against the 60 req/min limit; make it a named
   constant, surfaced as a `--timeout` flag with a sane default.
3. **Credit-cost disclosure** — decide the `--json` field name that reports
   credits consumed (if Snov returns it) so the assistant can budget.
