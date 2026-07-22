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
- `POST /emails/batch` — send up to 100 emails in one call (array of email
  objects). **Not full parity with single-send:** the batch endpoint does
  **not** support `attachments` (official docs: "The `attachments` field is not
  supported yet"); it **does** support `scheduled_at` and `tags`. Returns
  `{"data":[{"id":…}, …]}`.
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
tree. The connect-time verification probe (§3) is **`GET /domains`**, not
`/api-keys`: a restricted (sending-only) key returns `401 restricted_api_key`
on `/api-keys` while remaining a valid, send-capable key, so `/api-keys` is a
false-negative liveness probe. `/domains` is the lightweight read a full key is
expected to have. Neither is surfaced as a teammate command.

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
  - `resend email batch   --emails <json-array>` (≤100; **batch does NOT
    support `attachments` — Resend rejects them 422; `scheduled_at`/`tags` OK**)
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
- **Credential reject keys on the parsed error `name`, NOT the raw HTTP
  status.** Verified against Resend's official errors page
  (https://resend.com/docs/api-reference/errors): both statuses are overloaded,
  so a raw-status rule false-rejects valid keys.
  - **403 is not credential-exclusive.** `invalid_api_key` is 403, but so are
    three `validation_error` variants — most importantly **unverified sending
    domain** (`403 validation_error`, message "…domain is not verified…"), plus
    testing-restriction and domain-already-registered. Unverified-domain is the
    *default* state of every new account until DNS verification, so a raw
    `403 → RejectCredential` rule would tear down a **valid** `re_…` key the
    first time an agent sends from an unverified `from`. This is a concrete
    false-positive, not a hypothetical.
  - **401 is not credential-exclusive either.** `missing_api_key` is 401, but so
    is `restricted_api_key` — a *valid* sending-only key.
  - Decision, by parsed `name`:
    - `name ∈ {invalid_api_key, missing_api_key}` → wrap in
      `execution.RejectCredential(...)` (genuinely bad/absent key; token gateway
      marks it rejected).
    - `name == restricted_api_key` → **plain passthrough error, NOT a reject**:
      the key is live, just scoped to sending-only. Tearing it down would brick
      a working send credential.
    - **every other 401/403** — notably `403 validation_error` /
      unverified-domain — → **plain passthrough API error** the agent acts on
      (verify the domain, fix the `from`), never a credential reject.
    - If the body is unparseable (no `name`), fall back to a plain error — do
      **not** reject on bare status, so a malformed-but-non-credential 4xx can't
      brick a good key.
- `429` (rate limit, 10 rps/team) → plain error (not a credential reject);
  passthrough the body so the agent can back off.

Confirm the name-based split in L2 (a valid key sending from an unverified
domain must NOT be rejected).

### 2.4 Unit tests (L1, TDD-first)

`resend_test.go` + per-group `*_test.go` with an `httptest.Server` fake:
assert method/path/query, `Authorization: Bearer …` + `User-Agent` headers,
request JSON body for `email send`/`batch`, passthrough stdout, and both
plain-text and `--json` error rendering. **Credential-reject matrix — pins the
name-based contract of §2.3:**

| Fake response | Expect |
|---|---|
| `403 invalid_api_key` | RejectCredential |
| `401 missing_api_key` | RejectCredential |
| `403 validation_error` (unverified domain, valid key) | **NO reject** — plain passthrough |
| `401 restricted_api_key` (live sending-only key) | **NO reject** — plain passthrough |
| `422 invalid_attachment` / `429 rate_limit_exceeded` | plain passthrough |
| 4xx with unparseable body (no `name`) | plain passthrough, NOT reject |

Never hit the real API from a unit test.

## 3. Helio provider bundle (`integrations/providers/resend/provider.yaml`)

Manual-token (api_key) bundle, **hidden-first** (`presentation.visible: false`).
In the `manual_credentials` family, but — unlike mongodb's DSN-host derivation —
Resend's `re_…` key is structureless, so it selects the shared
`bearer_fingerprint` deriver (knock's, #328) via the closed `identity.deriver`
enum (see the identity decision below). Verify-on-connect is an optional
label-only refinement, not the identity source.

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
  source: strategy          # marks the manual_credentials family (shared value
                            # with mongodb/amplitude/iterable/knock) — this is
                            # NOT the deriver selector and cannot distinguish
                            # derivers on its own.
  deriver: bearer_fingerprint  # THE selector: the closed identity.deriver enum
                            # (default dsn_host) that composeProviderRegistration
                            # switches on via manualCredentialsDeriver(...).
                            # bearer_fingerprint == knock's shared hostless,
                            # whoami-less SHA-256 fingerprint deriver — a re_…
                            # key has no DSN host and no userinfo endpoint, so
                            # dsn_host would reject a valid key (see decision).

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
(unlike Notion's `/workspace_id`).

**Ground truth on this worktree base** (verified in
`go-services/integration-service/service/provider_registry.go`,
`composeProviderRegistration`, lines 88-98): the `manual_credentials` runtime
strategy on this branch's base (`afe2cb51fb`) is wired **unconditionally** to
`dsnHostIdentityDeriver{}`, which `url.Parse`es the secret and derives the
account_key/label from the DSN **host**. It does **not** consult
`identity.source` (or any bundle field) — the base has **no** deriver-selection
switch and **no** fingerprint deriver. An opaque `re_…` bearer key has no host,
so `dsnHostIdentityDeriver` returns `manualCredentialFormatError` ("requires a
connection string with a host") and **would reject a perfectly valid Resend key
at connect time.** So `identity.source: strategy` is **not** the discriminator —
it is mongodb's own value (and amplitude's / iterable's / knock's), so it cannot
distinguish one deriver from another. Following the base's YAML verbatim (source
`strategy` + runtime_strategy `manual_credentials`) gets the DSN host deriver
and the false rejection.

**The selection mechanism already exists — reuse it, do not reinvent it.** The
sibling api_key tools in **this same batch** introduced the single shared
selector: a closed `identity.deriver` enum on the bundle that
`composeProviderRegistration` switches on via `manualCredentialsDeriver(...)`
(default `dsn_host`, so mongodb is unchanged). Its values, already built on
sibling branches:

- amplitude (#228) → `deriver: first_colon_split`
- iterable (#333) → `deriver: region_prefix`
- **knock (#328) → `deriver: bearer_fingerprint`** — a hostless, whoami-less
  secret hashed to a stable, secret-free SHA-256 fingerprint account_key.

`bearer_fingerprint` **is** Resend's case: a structureless `re_…` bearer key
with no DSN host and no userinfo endpoint. So the resend bundle sets
`identity.deriver: bearer_fingerprint` and **converges on knock's already-built
shared deriver** — it does **not** add a resend-specific `opaqueKeyIdentityDeriver`
or a parallel selection field. Resend's own worktree base predates both the
`identity.deriver` enum and the fingerprint deriver, so on this branch they show
as absent; they arrive at the **batch-end capability-growth reconciliation** when
the sibling derivers merge. Resend contributes **zero new integration-service
code** on the identity path as long as knock's `bearer_fingerprint` deriver (or
the equivalent shared fingerprint deriver the batch settles on) lands first;
resend must not fork its own.

**Optional label enhancement — verify-on-connect against `GET /domains`.** The
`bearer_fingerprint` account_key is opaque (a hash), so an optional refinement is
to derive a **human-readable label** from the account: probe
`GET https://api.resend.com/domains` and, on `200`, label from the first domain
name; parsed `name == invalid_api_key` (403) → reject at connect; parsed
`name == restricted_api_key` (401) → **accept** (a live sending-only key
legitimately 401s on reads) and fall back to the fingerprint. This is the same
probe endpoint named in §1 (`/domains`, never `/api-keys` — a restricted key
401s on `/api-keys`). Do this **only** by reusing an existing shared
Bearer-verifier capability if the batch base already has one — **never** add a
Resend-specific `service/adapter_*.go`. If no shared verifier exists, ship the
fingerprint alone; the account_key never depends on it (a restricted key can't
read `/domains`), so this stays a pure label nicety, not a required capability.

**Capability-growth accounting.** Resend adds no new CredentialSource, no
token-gateway change, and — given the shared `identity.deriver` /
`bearer_fingerprint` convergence — no new deriver or selection switch of its own.
The one bundle-level change is `identity.deriver: bearer_fingerprint`, and the
final deriver name / dedup against knock's is deferred to the batch-end
reconciliation the master plan §6 already calls for. Record the reuse (not a new
capability) in the batch capability-growth ledger. The key still rides
`token.access_token`, exactly like mongodb's DSN. Seedable at L4 (api_key auth
type is seedable; only minted providers like github are rejected).

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
  sends; `--scheduled-at` for later), and call out that **`email batch` does
  NOT support `attachments`** (single-send only — a batch send with
  `attachments` returns an opaque 422); bump plugin version + publish at batch
  end.
- **Generation:** `provider-gen` + `--check` from
  `go-services/integration-service`; five projections commit together at
  batch-end (never on this branch — expected to fail `--check` in CI until the
  batch lead's canonical regen).

## 4. Test plan — five layers

| Layer | What runs | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...` — `resend` service + definition unit tests vs `httptest` fakes (headers incl. User-Agent, send/batch body, 401/403/422/429 rendering, `--json` envelope) | No |
| **L2** | `ANYCLI_CRED_API_KEY=re_… anycli resend -- email send --from "onboarding@<verified-domain>" --to <inbox> --subject hi --text hi`, plus `email get <id>`, `domain list` against the **real** api.resend.com | **Yes** — a real Resend API key + a verified sending domain + a deliverable test inbox (account pool). Confirms field names, the User-Agent 403 behavior, that the error `name` field is populated (so §2.3's name-based reject fires correctly), and that a send from an **unverified** `from` returns `403 validation_error` as a **passthrough** error (NOT a credential reject) |
| **L3** | `provider-gen --check` + both repos' unit suites; `helio-cli` build with a local uncommitted `go.mod replace` → anycli branch, then `go test ./cmd/heliox/cmds/tool/` | No |
| **L4** | singleton + `POST /internal/test-only/connections/seed` (provider `resend`, `access_token`=real `re_…`) → `heliox tool resend -- email send …` reaches the live API through the token gateway | **Yes** — same real key seeded (non-expiring key, seed `access_token` only, no refresh/expires_at) + real seeded org/assistant/user identities |
| **L5** | Hidden-still: open connect link → paste `re_…` through the real connect UI (`POST /connections/credentials`) → appears connected in `GET /connections` → one **unseeded** live `email send` succeeds. This is the **api_key key-entry L5 path** (master plan §2), agent-drivable via agent-browser, human fallback | **Yes** — real key + verified domain + test inbox |

L2/L4/L5 require externally supplied credentials (Resend API key from a test
account with a verified domain and a deliverable inbox); L1/L3 are hermetic.

**Rollout:** land hidden → pin bump ships the `resend` definition → L1–L4 pass
hidden → L5 key-entry sweep → flip `presentation.visible: true` + regenerate as
the single go-live change.
