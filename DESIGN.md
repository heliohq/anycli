# Resend — per-tool design (`heliox tool resend`)

Scratch design for the `tool/resend` batch branch. Batch lead strips this file
at batch-end. Covers: API surface, anycli definition, credential/auth flow,
Helio provider bundle, and the five-layer test plan.

## 0. Naming (the three axes) & lane

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `resend` | bundle `tool.command` (unset — flat, not a group) |
| ② anycli tool id | `resend` | `definitions/tools/resend.json`, `RegisterService("resend", …)` |
| ③ provider catalog key | `resend` | `integrations/providers/resend/` dir name + `key:` |

②==③ (identity), so **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go` — `ProviderFor("resend")` falls
through to the identity return. Go package: `internal/tools/resend/` (id has no
dashes/leading digit → package name `resend`).

**Auth lane: `api_key`.** Catalog row 54 = `api_key`; OAuth-audit row 54 =
"no viable multi-tenant path → api_key". Independently verified against
official docs: Resend has **no OAuth flow of any kind** — the entire API
authenticates with a team-scoped bearer API key (`re_…`). There is no authorize
endpoint, no token endpoint, no 3-legged consent. The audit verdict and catalog
lane both hold; no divergence to record in DESIGN.md. Sibling ESPs Postmark
(row 53) and SendGrid (row 52) are `api_key` for the same reason — Resend is
consistent with that precedent.

## 1. What an AI teammate does with Resend → API surface wrapped

Resend is a developer-first transactional + broadcast email platform. The
dominant AI-teammate job is **programmatic email sending** ("email the customer
the summary", "send the report to finance@…", "schedule this reminder for
tomorrow 9am"), with a secondary marketing-list job (manage audiences/contacts,
draft and send a broadcast). The tool wraps the official REST API at
`https://api.resend.com` (docs: https://resend.com/docs/api-reference/introduction).

Endpoint groups, by AI relevance:

**Emails (primary — the reason this tool exists)**
- `POST /emails` — send one email. Body: `from` (required, `Name <addr>` form),
  `to` (string|array, max 50), `subject` (required), `html`, `text`, `cc`,
  `bcc`, `reply_to`, `scheduled_at` (ISO-8601 or natural language e.g.
  "in 1 min"), `attachments[]` (`content` base64 / `path`, `filename`,
  `content_type`, `content_id`), `tags[]` (`name`/`value`), `headers{}`.
  Optional `Idempotency-Key` request header (unique per request, 24h TTL,
  ≤256 chars) — surfaced as a flag so retried agent sends don't double-deliver.
  Success: `{"id": "<uuid>"}`.
- `POST /emails/batch` — send up to 100 emails in one call (array of the same
  objects). Returns `{"data":[{"id":…}, …]}`.
- `GET /emails/{id}` — retrieve a sent email's delivery status.
- `PATCH /emails/{id}` — reschedule a not-yet-sent email (`scheduled_at`).
- `POST /emails/{id}/cancel` — cancel a scheduled email.

**Domains (read — sender-address discovery/verification)**
- `GET /domains`, `GET /domains/{id}` — an agent needs to know which verified
  sending domains exist before it can pick a valid `from`. Read is in-scope.
- `POST /domains`, `PATCH`, `DELETE`, `POST /domains/{id}/verify` — account
  configuration ops. Included as explicit verbs but low-frequency; a teammate
  rarely provisions DNS. Kept for completeness, gated behind clear subcommands.

**Audiences / Contacts / Broadcasts (secondary — newsletter workflows)**
- Audiences: `POST /audiences`, `GET /audiences`, `GET /audiences/{id}`,
  `DELETE /audiences/{id}`.
- Contacts (nested under an audience): `POST /audiences/{aid}/contacts`,
  `GET /audiences/{aid}/contacts`, `GET …/contacts/{id}`, `PATCH …`, `DELETE …`.
- Broadcasts: `POST /broadcasts`, `GET /broadcasts`, `GET /broadcasts/{id}`,
  `PATCH /broadcasts/{id}`, `POST /broadcasts/{id}/send`, `DELETE …`.

**API keys — deliberately NOT exposed as AI verbs.** `POST /api-keys` /
`DELETE /api-keys/{id}` are credential-management/privilege-escalation surface
(an assistant could mint itself a full-access key). Excluded from the command
tree. `GET /api-keys` is used only as the connect-time verification probe
(§3), not surfaced as a teammate command.

Scope decision: ship **`email` (send/batch/get/update/cancel)** + **`domain`
(list/get, plus create/verify/update/delete)** in v1 as the core, and
**`audience` / `contact` / `broadcast`** groups in the same v1 since they are
thin passthroughs over the same client. Send is the load-bearing path; the
rest ride the same auth + client for zero marginal cost.

## 2. anycli definition

### 2.1 Tool form — `service` type

`service` per the stage-1 rubric. `cli` type is rejected: there is no official
Resend binary to wrap — Resend ships language SDKs (node/python/go/…) and a
REST API, no agent-friendly `--json` CLI that could be provisioned into the
runtime image. So implement `service` type in `internal/tools/resend/` against
the HTTP API (matches 21/23 existing definitions; direct structural precedent
is `internal/tools/bitly/` — a Bearer-token api_key service).

### 2.2 `definitions/tools/resend.json`

```json
{
  "name": "resend",
  "type": "service",
  "description": "Resend as a tool (send transactional & broadcast email via API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "RESEND_API_KEY"}
      }
    ]
  }
}
```

Single credential field `api_key`, injected as env `RESEND_API_KEY`. The
service reads it and sets `Authorization: Bearer <key>` on every request.

### 2.3 Service implementation (`internal/tools/resend/`)

Copy the `bitly` shape (`Service{BaseURL, HC, Out, Err}` so tests point at an
`httptest.Server`; cobra tree; a `call(ctx, method, path, query, payload)`
helper; passthrough emit). Notion's grouped tree is the model for the
resource grouping.

- `DefaultBaseURL = "https://api.resend.com"`.
- **`User-Agent` is mandatory**: Resend rejects requests with a missing
  User-Agent with `403`. Set an explicit `User-Agent: helio-anycli/resend`
  header in `call` (Go's default `Go-http-client/1.1` would technically pass,
  but relying on it is fragile — set it explicitly). Flag this in the L2 run.
- Cobra tree (`--` passthrough from heliox):
  - `resend email send   --from --to --subject [--html|--text] [--cc --bcc --reply-to --scheduled-at --attachments <json> --tags <json> --headers <json> --idempotency-key]`
  - `resend email batch   --emails <json-array>` (≤100)
  - `resend email get     <id>`
  - `resend email update  <id> --scheduled-at <ts>`
  - `resend email cancel  <id>`
  - `resend domain list` / `resend domain get <id>` / `resend domain create --name --region` / `resend domain verify <id>` / `resend domain update <id> …` / `resend domain delete <id>`
  - `resend audience list|get|create|delete`
  - `resend contact list|get|create|update|delete --audience <aid> …`
  - `resend broadcast list|get|create|update|send|delete`
- Complex/structured inputs (`attachments`, `tags`, `headers`, batch `emails`)
  taken as raw-JSON flags validated client-side (bitly's `decodeJSONFlag`
  pattern) — keeps the surface agent-friendly without modeling every nested
  field.

**JSON output shape:** passthrough — write Resend's JSON response body to
stdout verbatim + newline (bitly/notion `emit`). No client-side reshaping.
`--json` persistent flag accepted for uniformity (output is always JSON).

**Exit-code / error contract** (bitly/notion precedent):
- `0` success; `1` runtime/API failure via typed `apiError`; `2` usage/parse.
- Non-2xx → typed error carrying Resend's error body. Resend errors are JSON
  `{"statusCode":<n>,"name":"<slug>","message":"<text>"}` — `apiMessage`
  extracts `message` (fallback `name`, fallback raw body). Confirm exact field
  names in the L1 fake and the L2 real run.
- **`401`** (missing key) **and `403`** (invalid key) → wrap in
  `execution.RejectCredential(...)` so the token gateway marks the credential
  rejected (bitly only maps 401; Resend uses 403 for a *bad* key and 401 for a
  *missing* one, so map **both**). Verify this 401-vs-403 split in L2.
- `429` (rate limit, 10 rps/team) → plain error (not a credential reject);
  passthrough the body so the agent can back off.

### 2.4 Unit tests (L1, TDD-first)

`resend_test.go` + per-group `*_test.go` with an `httptest.Server` fake:
assert method/path/query, `Authorization: Bearer …` + `User-Agent` headers,
request JSON body for `email send`/`batch`, passthrough stdout, and both
plain-text and `--json` error rendering for 401/403/422/429. Never hit the
real API from a unit test.

## 3. Helio provider bundle (`integrations/providers/resend/provider.yaml`)

Manual-token (api_key) bundle, **hidden-first** (`presentation.visible: false`).
Modeled on the manual-credential precedent (`mongodb`), but Resend *does* have
a validatable HTTPS endpoint, so prefer verify-on-connect over mongodb's
no-verify.

```yaml
schema: helio.provider/v1
key: resend
go_name: Resend

presentation:
  name: Resend
  description_key: resend
  consent_domain: resend.com
  visible: false            # hidden-first; flip is the single go-live change

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: resend_api_key
        secret: true
        placeholder: "re_..."
        required: true
    setup_url: https://resend.com/api-keys

identity:
  source: strategy          # no team-identity userinfo endpoint (see below)

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
    api_key: token.access_token      # single secret through UpsertUserToken
    account_key: connection.account_key

tool:
  name: resend
  kind: api-key
```

**Identity / verify decision (the one capability question).** Resend keys are
**team-scoped** and the API exposes **no `/me`/userinfo endpoint** returning a
stable team id or name — so there is no clean field to use as `stable_key`
(unlike Notion's `/workspace_id`). Two viable shapes:

1. **No-verify (mongodb precedent, safest default for hidden-first):**
   `identity.source: strategy`, store the key without a provider round-trip; a
   bad key surfaces at first `heliox tool resend` call via
   `RejectCredential` (401/403). Account label is a static/user-supplied label.
   Zero integration-service capability growth. **Recommended for the initial
   hidden landing.**
2. **Verify-on-connect (loops/tally Bearer-verifier precedent):** if the batch
   base already carries a generic Bearer-scheme identity/verifier capability,
   reuse it against `GET https://api.resend.com/domains` (200 = valid key,
   401/403 = reject) to fail a bad key at connect time and, optionally, derive
   the label from the first domain name. **Only reuse an existing shared
   capability — do NOT add a Resend-specific adapter** (`service/adapter_*.go`
   is reserved for non-standard response/lifecycle shapes; a bearer key with a
   standard GET probe is not one). If no such capability exists on the base,
   ship shape (1) and let verification land when the shared verifier does.

Either way: **no new CredentialSource, no token-gateway change** — the key
rides `token.access_token`, exactly like mongodb's DSN. Seedable at L4 (api_key
auth type is seedable; only minted providers like github are rejected).

`auth.required_config_fields` is **empty** — there is no OAuth client id/secret,
so integration-service needs **zero `config/` + `deploy/` credential appends**
for this provider (it renders `configured: true` with no env). This removes the
lane-1 config-landing gate that OAuth tools carry.

### 3.1 Other Helio-side artifacts

- **Resolver:** none (②==③).
- **UI icon:** `ui/helio-app/src/integrations/icons/resend.svg` + register in
  `providerIcons.ts` (manual, never generated). Resend brand mark (black
  wordmark/logo).
- **i18n:** `tools.desc.resend` + `resend_api_key` field label across all
  locales.
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` — emphasize `email send` (from must be a
  verified-domain address; `to` ≤50; use `--idempotency-key` for retriable
  sends; `--scheduled-at` for later), bump plugin version + publish at batch end.
- **Generation:** `provider-gen` + `--check` from
  `go-services/integration-service`; five projections commit together at
  batch-end (never on this branch — expected to fail `--check` in CI until the
  batch lead's canonical regen).

## 4. Test plan — five layers

| Layer | What runs | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...` — `resend` service + definition unit tests vs `httptest` fakes (headers incl. User-Agent, send/batch body, 401/403/422/429 rendering, `--json` envelope) | No |
| **L2** | `ANYCLI_CRED_API_KEY=re_… anycli resend -- email send --from "onboarding@<verified-domain>" --to <inbox> --subject hi --text hi`, plus `email get <id>`, `domain list` against the **real** api.resend.com | **Yes** — a real Resend API key + a verified sending domain + a deliverable test inbox (account pool). Confirms field names, the User-Agent 403 behavior, and the 401(missing)-vs-403(invalid) split |
| **L3** | `provider-gen --check` + both repos' unit suites; `helio-cli` build with a local uncommitted `go.mod replace` → anycli branch, then `go test ./cmd/heliox/cmds/tool/` | No |
| **L4** | singleton + `POST /internal/test-only/connections/seed` (provider `resend`, `access_token`=real `re_…`) → `heliox tool resend -- email send …` reaches the live API through the token gateway | **Yes** — same real key seeded (non-expiring key, seed `access_token` only, no refresh/expires_at) + real seeded org/assistant/user identities |
| **L5** | Hidden-still: open connect link → paste `re_…` through the real connect UI (`POST /connections/credentials`) → appears connected in `GET /connections` → one **unseeded** live `email send` succeeds. This is the **api_key key-entry L5 path** (master plan §2), agent-drivable via agent-browser, human fallback | **Yes** — real key + verified domain + test inbox |

L2/L4/L5 require externally supplied credentials (Resend API key from a test
account with a verified domain and a deliverable inbox); L1/L3 are hermetic.

**Rollout:** land hidden → pin bump ships the `resend` definition → L1–L4 pass
hidden → L5 key-entry sweep → flip `presentation.visible: true` + regenerate as
the single go-live change.
