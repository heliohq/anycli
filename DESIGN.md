# Lemlist — `heliox tool lemlist` design

Per-tool design for the Lemlist external tool provider, per the
`helio-tool-provider` pipeline and the 298-integrations master plan
(`docs/design/008-300-integrations-rollout-plan.md`).

- **Catalog row:** 71 — Lemlist, Sales Engagement, **Wave 2**.
- **Naming (three axes, all identical → no `toolToProvider` entry):**
  - ① CLI command word: `lemlist` (flat command, no group)
  - ② anycli tool id: `lemlist` (`definitions/tools/lemlist.json`)
  - ③ provider catalog key: `lemlist` (`integrations/providers/lemlist/`)
  - Go package (stage 2): `internal/tools/lemlist/` (no digit/dash normalization needed).
- **Auth lane:** `api_key` (verified below; audit verdict refined but lane retained — see §2).

---

## 1. Official API surface wrapped, and why

**Product.** Lemlist is a cold-outreach / sales-engagement platform (email +
LinkedIn + calls multichannel sequences). An AI teammate's realistic jobs:
enroll/inspect leads in campaigns, read reply/open/click activity, drive
campaign start/pause, manage the unsubscribe list, and read team/sender/credit
state.

**API.** Lemlist REST API, base URL **`https://api.lemlist.com/api`**; every
route is `…/api/{endpoint}`. There is a v2 OpenAPI spec at
`https://developer.lemlist.com/api-reference/openapi/v2.json` — exact
methods/paths below are from the docs index (page titles) and MUST be confirmed
against that spec at L1/L2 implementation time; the resource/verb decomposition
is what is load-bearing here, not the literal path strings.

**Scoped subcommand tree** (service type — cobra tree grouped by resource,
`notion` as the shape precedent). Read-first, with the small set of write/state
verbs an outreach teammate actually needs:

| Group | Verb | Endpoint (confirm at impl) | Why an AI teammate needs it |
|---|---|---|---|
| `team` | `get` | `GET /team` | Account context; **doubles as the identity/verify endpoint** (§3). |
| `team` | `senders` | `GET /team/senders` | List sending members + their campaigns. |
| `team` | `credits` | `GET /team/credits` | Remaining enrichment/send credits before acting. |
| `campaign` | `list` | `GET /campaigns` | Enumerate campaigns. |
| `campaign` | `get` | `GET /campaigns/{id}` | Inspect one campaign. |
| `campaign` | `stats` | `GET /campaigns/{id}/stats` | Open/click/reply/bounce numbers for reporting. |
| `campaign` | `start` / `pause` | `POST /campaigns/{id}/start` · `/pause` | Control an in-flight sequence. |
| `lead` | `add` | `POST /campaigns/{campaignId}/leads` | Enroll a lead (email + variables) into a campaign. |
| `lead` | `get` | `GET /leads/{email}` (or `/campaigns/{id}/leads`) | Look up a lead's state/stats. |
| `lead` | `update` | `PATCH` lead / custom variables | Fix or personalize a lead's fields. |
| `lead` | `unsubscribe` / `delete` | `POST …/unsubscribe` · `DELETE …/leads` | Stop contacting / remove a lead. |
| `lead` | `mark-interested` / `mark-not-interested` | lead status endpoints | Update pipeline disposition after a reply. |
| `activity` | `list` | `GET /activities` | Read the event stream (opens, clicks, replies, bounces). |
| `unsubscribe` | `add` / `list` / `delete` | `/unsubscribes` (email or domain) | Manage the suppression list (compliance-critical). |

Deliberately **excluded** from the first cut (keep the surface tight; add later
if an assistant workflow demands it): schedule CRUD, CSV/export-job polling,
voice-message audio upload, CRM-import batch jobs, activity-recording deletion.
These are heavy/stateful and not core to the "run outreach from chat" loop.

**Output shape.** `--json` on every leaf: pass through Lemlist's JSON, lightly
normalized (list endpoints emit a JSON array/`{data:[…]}`). Exit-code contract
copied from `notion`: `0` success, `1` runtime/API failure (typed `apiError`
with a `--json` structured error envelope carrying HTTP status + Lemlist error
body), `2` usage/parse error. No interactive prompts (anycli AGENTS rule).

---

## 2. Auth lane — verification vs the audit verdict (divergence recorded)

**Audit verdict (oauth-audit.md row 73):** "no viable multi-tenant path" →
`api_key`, no evidence URL (compact note).

**What the official docs actually say** (verified 2026-07-22):

1. **API-key HTTP Basic auth is the documented, primary, self-serve auth.**
   `https://developer.lemlist.com/api-reference/getting-started/authentication`
   states verbatim *"We use BASIC authentication NOT bearer"*: username always
   **empty**, password is the API key — i.e. `Authorization: Basic base64(":"+key)`
   (`curl -u ":YOUR_API_KEY"`). The key is generated self-serve in
   **Settings → Integrations → Generate** (each customer mints their own).
2. **An OAuth 2.0 authorization-code flow DOES exist** — contradicting the bare
   "no viable multi-tenant path" wording. Endpoints:
   `…/api/v1/auth/oauth2/authorize` and `…/api/v1/auth/oauth2/token`
   (`client_id`/`client_secret`/`code`/`refresh_token`). **However**, client
   registration is **not self-serve**: `client_id`/`client_secret` are issued
   via a partner/contact request, and the flow is positioned primarily for
   Lemlist's own **MCP server** (browser-consent PKCE for LLM clients) — a
   different integration path than our anycli passthrough.
3. A legacy `?access_token=<key>` **query-parameter** auth is mentioned in
   Help-Center/community sources, but Lemlist's current official developer
   authentication page documents **only** the Basic-auth header and its Help
   Center explicitly recommends against the query-param method for security.
   Treat query-param auth as an undocumented fallback, not a primary path.

**Divergence recorded, lane RETAINED as `api_key`.** Under the audit's own
rubric a partner-gated authorization-code flow would map to `oauth_review`, not
"no path" — so the verdict's *wording* is inaccurate. But `api_key` remains the
correct **lane** because:
- it is the officially documented, **self-serve**, no-review path (each customer
  generates their own key — zero lane-1 app registration, agent-drivable L5);
- the OAuth path has **no self-serve client registration** (partner/contact-gated)
  and mainly serves Lemlist's MCP, so it buys us nothing over the key;
- the whole Wave-2 batch scheduling for this tool assumes `api_key` (no review
  clock, api_key L5 sweep).

Net: **keep row 71 `api_key`**; this DESIGN is the recorded amendment that the
"no viable multi-tenant path" note should read "OAuth exists but is
partner-gated / not self-serve; `api_key` is the self-serve path." No catalog
lane change, no wave/number change.

---

## 3. anycli definition

**Type: `service`.** No official Lemlist CLI exists → the default service type
against the HTTP API (rubric: `cli` type only when an official, non-interactive,
`--json`, env-injectable, image-provisionable binary exists — none here).

**Credential shape — Basic auth is handled inside the service, not by header
templating.** Because Lemlist's key is the Basic-auth *password* (empty
username), the definition only injects the raw key as an env var; the service
constructs `req.SetBasicAuth("", key)` per request. This is cleaner than trying
to express Basic-auth-with-empty-user as a static header.

`definitions/tools/lemlist.json`:

```json
{
  "name": "lemlist",
  "type": "service",
  "description": "Lemlist as a tool (cold outreach: campaigns, leads, activities)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "LEMLIST_API_KEY"}
      }
    ]
  }
}
```

- `source.field: api_key` is the resolver-supplied field name; the Helio bundle
  maps it from the stored secret (`credential.fields: api_key: token.access_token`,
  §4), mirroring the `mongodb` precedent (`connection_string: token.access_token`).
- Service impl (`internal/tools/lemlist/`): `client.go` holds
  `BaseURL`/`HC`/`Out`/`Err` + a request helper that reads `LEMLIST_API_KEY` and
  calls `req.SetBasicAuth("", key)`; resource files (`team.go`, `campaign.go`,
  `lead.go`, `activity.go`, `unsubscribe.go`) build the cobra tree. Register in
  `internal/tools/register.go`: `RegisterService("lemlist", &lemlist.Service{})`.

---

## 4. Helio provider bundle plan

`integrations/providers/lemlist/provider.yaml` — hidden-first
(`presentation.visible: false`). Axis ③ key = `lemlist`.

```yaml
schema: helio.provider/v1
key: lemlist
go_name: Lemlist

presentation:
  name: Lemlist
  description_key: lemlist
  consent_domain: lemlist.com
  visible: false     # flip only after anycli pin ships the tool + L5 passes
  # order: <pick unoccupied at flip time>

auth:
  type: api_key
  owner: individual
  api_key:
    # Verify the key at connect time against the team endpoint using Lemlist's
    # documented Basic auth (empty user + key as password). See capability note.
    auth_style: basic_password          # NEW enum value — see §4a
    setup_url: https://developer.lemlist.com/api-reference/getting-started/authentication

identity:
  source: userinfo
  url: https://api.lemlist.com/api/team
  stable_key: /_id                      # confirm field name vs real /team response
  label_candidates: [/name, /_id]

connection:
  mode: isolated
  disconnect_mode: local_only           # api_key: nothing to revoke provider-side
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_key: token.access_token         # single secret via UpsertUserToken path
    account_key: connection.account_key

tool:
  name: lemlist
  kind: api-key
```

### 4a. Connect-time verification — capability decision

The token gateway / data plane needs **zero** integration-service change: it
projects `token.access_token` and anycli does the Basic auth. The only question
is **connect-time verification** (`POST /connections/credentials`).

The existing `declarativeManualTokenVerifier`
(`service/manual_token_verifier.go`) does
`req.Header.Set(definition.APIKey.Header, token)` — a single header with the raw
token as its value. That **cannot** express Lemlist's `Authorization: Basic
base64(":"+key)`, so the current capability does not fit.

- **Preferred — Option A (grow one reviewed capability):** add an
  `auth_style` enum to `APIKeyPolicy` (`header` default = today's behavior;
  `basic_password` = `req.SetBasicAuth("", token)` against `Identity.URL`). Small,
  closed, in-family with the recent verifier growths (crisp keypair, servicenow
  endpoint+secret, amplitude colon-split, semrush/moz query-param). Gives real
  connect-time rejection of bad keys (better UX) using Lemlist's official primary
  auth. Add a unit test in `manual_credential_test.go` for the Basic-auth branch.
- **Fallback — Option B (zero service code, `mongodb` precedent):** use
  `auth.type: credentials` + `identity.source: strategy` (no provider-side
  verification); a bad key surfaces at first `heliox tool` call via anycli's
  `CredentialRejected`. Choose this only if the batch wants to avoid any
  integration-service capability growth.

**Recommendation: Option A.** Verification-at-connect is the better teammate UX
and the capability is minimal and reviewed; multiple Wave-1/2 api_key tools grew
comparable verifier variants. `access_token` query-param verification is
explicitly rejected (discouraged by Lemlist, absent from current official docs).

### 4b. Other Helio-side artifacts

- **Resolver:** no `toolToProvider` entry (②==③==`lemlist`).
- **Config:** api_key manual-token provider needs **no** integration-service
  client id/secret — no `config/` or `deploy/` Secret append (Config Sync rule
  N/A here). This is a lane-1-free tool.
- **UI icon:** `ui/helio-app/src/integrations/icons/lemlist.svg` + register in
  `providerIcons.ts` (manual, never generated).
- **i18n:** `tools.desc.lemlist` (+ any `credential_input` label keys if Option B)
  across all locales.
- **AI-facing docs:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/`, then plugin version bump + marketplace
  publish (rides the batch-end merge).
- **Generation:** `provider-gen` + `--check` from `go-services/integration-service`;
  commit all five projections together with the bundle at batch end.

---

## 5. Test plan → five layers

| Layer | What it proves for Lemlist | External credential? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fake asserts: Basic auth header = `base64(":"+key)` with empty user; `campaign/lead/team/activity/unsubscribe` request shapes; `--json` error envelope; exit codes 0/1/2. | No |
| **L2** dev harness real API | `ANYCLI_CRED_API_KEY=<key> anycli lemlist -- team get` (and `campaign list`, `activity list`) return real data — proves field name, env injection, and Basic-auth request shape match the live API. **Mandatory before pin bump.** | **Yes** — real Lemlist API key (account pool) |
| **L3** `provider-gen --check` + both repos' unit suites | bundle strict-decodes; if Option A, integration-service verifier unit test for the `basic_password` branch passes. On-branch: run `provider-gen` locally + `helio-cli/go.mod` local `replace` → anycli branch (do not commit regen/replace). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider=lemlist`, `access_token=<real key>` (non-expiring key → seed `access_token` only, no refresh/expiry), then `heliox tool lemlist -- team get` reaches the live API through the real token gateway. Seed bypasses the connect UI. | **Yes** — real key + a real seeded assistant/org identity |
| **L5** api_key key-entry sweep | Per master-plan §2 api_key L5 path: open connect link → paste key through the real connect UI (`POST /connections/credentials`, verified against `/api/team`) → `GET /connections` shows connected/configured → **one unseeded** `heliox tool lemlist -- team get` succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. Runs before the visible flip. | **Yes** — real key (account pool) |

Layers needing externally supplied credentials: **L2, L4, L5** (one real Lemlist
API key from the test-account pool; Lemlist has a self-serve trial, so
procurement is low-risk). L1 and L3 need none.

**Rollout:** land hidden → bump anycli pin (one per batch) → L1–L4 while hidden →
api_key L5 sweep → flip `presentation.visible: true` + regenerate as the single
go-live change.

---

## 6. Open items to close at implementation (stage 1/L2)

1. Confirm exact HTTP methods/paths against the v2 OpenAPI spec (doc index gave
   names, not paths) — especially `campaign start/pause`, `lead
   mark-interested`, and `GET /leads/{email}` vs `/campaigns/{id}/leads`.
2. Confirm the `/team` response field names for `identity.stable_key` /
   `label_candidates` (`_id` vs `id`, `name`).
3. Ratify Option A (grow `basic_password` verifier) vs Option B (no-verify,
   mongodb precedent) with the batch lead before writing integration-service code.
