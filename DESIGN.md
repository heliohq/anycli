# Tool design — Hootsuite

Scratch design file for branch `tool/hootsuite` (both repos). Batch lead strips
it at batch end. Follows the `helio-tool-provider` pipeline; per-tool decisions
only — no architecture changes.

## 0. Catalog row & naming (master plan §3, §4 row 132)

| Axis | Value |
|---|---|
| ① CLI command word (`tool.command`) | `hootsuite` (flat command; not a grouped family) |
| ② anycli tool id (`definitions/tools/<id>.json`) | `hootsuite` |
| ③ provider catalog key (bundle dir / `key:`) | `hootsuite` |
| Go package (`internal/tools/<pkg>/`) | `hootsuite` |
| `RegisterService` string | `hootsuite` |

② == ③ == ① — **no ②↔③ divergence**, so **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go` (that map holds only divergent pairs).
Auth lane: **`oauth_review`**. Wave: **2**. Category: **Marketing**.

**Lane verification against official docs (independent judgment).** The catalog
lists `oauth_review`; the OAuth audit has no row for Hootsuite because it was
already `oauth_review` pre-audit (the audit only re-laned `api_key` tools).
Verified against the provider's own docs: Hootsuite's REST API uses a
multi-tenant authorization-code OAuth flow (one registered app, arbitrary member
accounts authorize) — but the client id/secret are only visible once **"API
access has been approved"** for the app in the Developer Portal
(`developer.hootsuite.com/docs/getting-started-with-the-rest-api`). That approval
is a human review gate before any external account can authorize, which is
exactly the `oauth_review` rubric ("a human review, partner-program,
verification, or publish gate before external accounts can authorize"). **Catalog
lane confirmed; no divergence to record.** Consequence per master plan §2 lane 1:
a **dev/test-mode** app can be created pre-approval, so dev + L1–L4 + the
batch-end merge are **not** gated on review; only the `presentation.visible: true`
flip waits for API-access approval to clear.

## 1. API surface wrapped, and why

Hootsuite is a social-media management platform: one account fans a post out to
many connected social profiles (X, LinkedIn, Facebook, Instagram, Pinterest,
TikTok, …), on a schedule, with an approval workflow and team/org structure. An
AI teammate's real jobs are: **draft and schedule social posts**, **see what's
already queued**, **unschedule/cancel**, **attach media**, and **discover which
social profiles/teams it may post to**. That maps to a small, stable slice of the
**Hootsuite REST API v1** (base `https://platform.hootsuite.com/v1`, `Bearer`
token, JSON, every response wrapped in a top-level `{"data": …}` envelope).

Endpoints the tool wraps (all verified against `developer.hootsuite.com` /
`apidocs.hootsuite.com`):

| Job | Method + path | Notes |
|---|---|---|
| Identity / whoami | `GET /v1/me` | member `{data:{id,email,fullName,organizationIds}}` — identity stable key + org discovery |
| Member's orgs | `GET /v1/me/organizations` | orgs the member belongs to |
| List social profiles | `GET /v1/socialProfiles` | the target set for any post; `id` feeds `socialProfileIds` |
| Get one social profile | `GET /v1/socialProfiles/{id}` | |
| Profile teams | `GET /v1/socialProfiles/{id}/teams` | team access graph |
| **Schedule / send a post** | `POST /v1/messages` | body `{text, socialProfileIds[], scheduledSendTime, media?, tags?, targeting?, location?, emailNotification?, webhookUrls?}`; `scheduledSendTime` **must** be UTC ISO-8601 ending in `Z`; omit it for "soonest possible" |
| List scheduled/queued posts | `GET /v1/messages` | filters: `state`, `startTime`, `endTime`, `socialProfileIds` |
| Get one message | `GET /v1/messages/{id}` | |
| Unschedule / delete | `DELETE /v1/messages/{id}` | |
| Approve / reject (workflow) | `POST /v1/messages/{id}/approve`, `/reject` | approver flows |
| Generate media upload URL | `POST /v1/media` | `{sizeBytes, mimeType}` → `{id, uploadUrl, expiresAt}`; caller PUTs bytes to `uploadUrl`, then references `id` in a message's `media` |
| Media status | `GET /v1/media/{id}` | poll until `READY` before scheduling |

Why this slice and not more: the analytics/Amplify/enterprise-admin surfaces are
either a separate product (`amplify.hootsuite.com`) or admin-only org management
that an AI teammate won't drive; message scheduling + profile discovery is the
90% path and matches the provider's own "Getting Started" arc (list profiles →
schedule message → verify via `/v1/me`). **Pinterest caveat** (documented): a
Pinterest post cannot be bundled with other profiles and needs an
`extendedInfo{boardId,destinationUrl}` object; the `message schedule` verb
surfaces `--board-id`/`--destination-url` flags for that case. **Media caveat**:
images and video cannot be mixed in one message, and video must be scheduled
≥15 min out.

## 2. anycli definition — `service` type

**Form decision (skill stage 1): `service` type.** There is no official
Hootsuite CLI to wrap (fails the `cli`-type gate), so this is an HTTP `service`
against the REST API — the 21-of-23 default. Reference implementation to copy the
shape of: `internal/tools/notion/` (cobra tree grouped by resource, injectable
`BaseURL`/`HC`/`Out`/`Err`, `--json` structured error envelope, exit codes 0/1/2).

`definitions/tools/hootsuite.json`:

```json
{
  "name": "hootsuite",
  "type": "service",
  "description": "Hootsuite as a tool (schedule and manage social posts; OAuth 2.0 user token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "HOOTSUITE_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single credential field — unlike `x`/`linkedin` (which inject an extra
`user_id`/`person_urn`), the Hootsuite service resolves its own actor via
`GET /v1/me`, so only the bearer `access_token` is injected. Field name
`access_token` matches the bundle's `credential.fields.access_token` projection.

**Subcommand tree (`internal/tools/hootsuite/`):**

```
hootsuite me                                  GET /v1/me
hootsuite org list                            GET /v1/me/organizations
hootsuite profile list                        GET /v1/socialProfiles
hootsuite profile get <id>                    GET /v1/socialProfiles/{id}
hootsuite profile teams <id>                  GET /v1/socialProfiles/{id}/teams
hootsuite message schedule                    POST /v1/messages
    --text <s> --profile <id>... [--send-time <RFC3339Z>]
    [--tag <s>...] [--email-notification] [--media-id <id>...]
    [--board-id <id> --destination-url <url>]   # Pinterest
hootsuite message list                        GET /v1/messages
    [--state <s>] [--start <RFC3339Z>] [--end <RFC3339Z>] [--profile <id>...]
hootsuite message get <id>                     GET /v1/messages/{id}
hootsuite message delete <id>                  DELETE /v1/messages/{id}
hootsuite message approve <id>                 POST /v1/messages/{id}/approve
hootsuite message reject <id> [--reason <s>]   POST /v1/messages/{id}/reject
hootsuite media create --size-bytes <n> --mime-type <s>   POST /v1/media
hootsuite media get <id>                       GET /v1/media/{id}
```

**JSON output shape.** Provider-neutral for agents: unwrap Hootsuite's `{"data":
…}` envelope and print the inner value (object or array) as the command result;
lists print the array directly. On failure, exit non-zero with the notion-style
`--json` error envelope `{"error":{"code","message","status"}}` derived from
Hootsuite's error body (which carries numeric error codes, e.g. `1005` token
could not be retrieved, `1006` token removal failed) and the HTTP status. Client
input errors (bad flags, non-`Z` send time) exit `2`; API/runtime failures exit
`1`; success `0`. `--send-time` is validated to be UTC ending in `Z` before the
POST (fail fast — Hootsuite rejects ambiguous/offset timestamps).

**Validate-before-leaving-anycli (L2):** run the dev harness against the real API
with a real token: `ANYCLI_CRED_ACCESS_TOKEN=<tok> anycli hootsuite -- profile
list` and `… -- me`. Unit tests (L1) use `httptest` fakes asserting request path,
`Authorization: Bearer` header, JSON body shape for `message schedule`, envelope
unwrapping, and both plain/`--json` error rendering — never the live API.

## 3. Credential fields & the exact OAuth flow (oauth_review lane)

**Registration model.** Create an app in the Hootsuite Developer Portal; request
REST API access; on approval the app's **REST API Client ID** and **REST API
Client Secret** become visible. Redirect/callback URI is registered with the app
(default is the Postman callback; changeable on request). This approval step is
the `oauth_review` gate (dev-mode app usable before approval for L1–L4).

**Endpoints (verified):**
- Authorize: `GET https://platform.hootsuite.com/oauth2/auth`
  — params `response_type=code`, `client_id`, `redirect_uri`, `scope=offline`,
  `state` (≥8 chars if supplied).
- Token / refresh: `POST https://platform.hootsuite.com/oauth2/token`
  — client authenticates with **HTTP Basic** (`client_id:client_secret`); body is
  form-encoded `grant_type=authorization_code&code=…&redirect_uri=…` then
  `grant_type=refresh_token&refresh_token=…` on refresh.

**Scope.** The only valid scope value is **`offline`** (its presence is what
returns a refresh token). Token response echoes `"scope":"offline"`.

**Token semantics (verified against official docs).** Access token `token_type:
bearer`, `expires_in: 3599` (~1 hour) — **short-lived, must refresh**. A
`refresh_token` is returned; it has **no expiry but is single-use** — every
`grant_type=refresh_token` exchange returns a **new** `refresh_token` that must
replace the stored one (documented rotation; drives the `refresh_lease:
credential` decision in the mapping below and the A3 strict-write-back
requirement). No PKCE documented (→ `pkce: none`). No standalone REST **revoke**
endpoint exists (revocation is App-Directory-uninstall or dashboard-side) →
disconnect is `local_only`.

**Mapping to `standard_oauth` (the golden path — no provider Go).** The token
exchange is form-body + Basic client auth → `token_exchange_style: form_basic`
(existing enum, `model/catalog.go`). This composes `standardOAuthExchanger` +
`declarativeIdentityResolver` with no compiled per-provider adapter. The one
non-declarative dependency is the `refresh_lease: credential` contract growth
below — an integration-service contract-table edit shared by every rotating-token
`standard_oauth` provider, still not a Hootsuite-specific adapter.

- **`refresh_lease: credential` — an owned capability prerequisite of this batch,
  not a contingency.** Hootsuite's official auth docs
  (`developer.hootsuite.com/docs/api-authentication`) state the refresh token
  **"can only be used once"** and every refresh returns a **new** `refresh_token`
  that must replace the stored one — i.e. Hootsuite **rotates** the refresh token
  on every refresh. This is documented behavior, not something to discover at
  L2/L5. The decision rule ("if the provider rotates the refresh token, grow
  `standard_oauth` and set `refresh_lease: credential`") is therefore **already
  triggered by the docs**, so the correct default is **`refresh_lease:
  credential`**, not `none`. `none` does **not** mean "never refresh" — the
  exchanger still refreshes an expired token — but it also does **not** serialize
  concurrent refreshes: under Helio's horizontal-scale mandate, two token-gateway
  replicas refreshing the same connection would each POST the same single-use
  refresh token; one wins and writes back the new pair, the other burns an
  already-consumed token and returns a transient 5xx, and the A3 strict-write-back
  edge (refreshed-but-unpersisted) risks bricking the connection. The
  `credential`-scoped lease (`leaseKey = "refresh:<provider>:<credentialID>"`,
  `service/token_refresh.go`) exists precisely to serialize this per connection.
  - **Blocking prerequisite (owned by this tool's batch, do NOT assume it landed):**
    the `standard_oauth` runtime contract in `model/runtime_contract.go` pins
    `refreshLeaseScope: OAuthLeaseNone` and `ValidateRuntimeContract` enforces it
    with an **exact-match** check (line ~224), so `refresh_lease: credential` fails
    `provider-gen --check` **today**. Verified on this branch point: neither the
    `keap` nor the `signnow` bundle exists and the contract still pins
    `OAuthLeaseNone`, so the "already-in-flight §4a growth may be on `main`"
    assumption is **false** — treat the growth as an explicit prerequisite this
    batch must land, not inherit. The growth is: make `RuntimeStrategyStandardOAuth`
    accept an **allowed set** `{OAuthLeaseNone, OAuthLeaseCredential}` for
    `refreshLeaseScope` (the enum value `OAuthLeaseCredential = "credential"`
    already exists in `model/catalog.go`, and the credential-scoped lease path is
    already implemented in `token_refresh.go` — only the contract table rejects it).
    This is an integration-service contract-table edit with its own test, **not** a
    per-provider Go adapter. If a sibling tool in the same batch lands the identical
    growth first, reuse it (don't re-add); but plan and test as if this batch owns it.
- **Identity.** The token response has no member identifier, so identity is
  resolved via a userinfo GET: `identity.source: userinfo`,
  `url: https://platform.hootsuite.com/v1/me`, `stable_key: /data/id`,
  `label_candidates: [/data/email, /data/fullName, /data/id]` (RFC-6901 pointers
  into the `{data:{…}}` envelope). `standard_oauth` permits `userinfo` identity.

**What never enters the bundle** (per `provider-yaml.md`): client id/secret live
only in integration-service config — `config/` locally and the Helm Secret under
`deploy/`, landed together (Config Sync). Bundle declares only
`required_config_fields: [oauth.client_id, oauth.client_secret]`.

## 4. Helio provider bundle plan — `integrations/providers/hootsuite/provider.yaml`

Hidden-first (`presentation.visible: false`). Proposed manifest:

```yaml
schema: helio.provider/v1
key: hootsuite
go_name: Hootsuite

presentation:
  name: Hootsuite
  description_key: hootsuite
  consent_domain: hootsuite.com
  visible: false            # hidden-first; flip is the single go-live change after review clears + L5
  order: <next-free>        # batch lead assigns to avoid collisions

auth:
  type: oauth
  owner: assistant          # the AI teammate holds the social-posting connection (notion/slack precedent)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://platform.hootsuite.com/oauth2/auth
    token_url: https://platform.hootsuite.com/oauth2/token
    token_exchange_style: form_basic
    pkce: none
    scopes: [offline]
    single_active_token: false
    refresh_lease: credential  # Hootsuite rotates the single-use refresh token every refresh (docs-confirmed); serialize concurrent replica refreshes per connection. Requires the §3 standard_oauth contract growth (owned by this batch).

identity:
  source: userinfo
  url: https://platform.hootsuite.com/v1/me
  stable_key: /data/id
  label_candidates: [/data/email, /data/fullName, /data/id]

connection:
  mode: isolated
  disconnect_mode: local_only   # no public REST revoke endpoint
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
  name: hootsuite
  kind: oauth
```

Adjacent required artifacts (batch-end merged, per §2 shared-surface rules):
- **Generation:** `provider-gen` + `provider-gen --check` from
  `go-services/integration-service`; the **five** projections commit together with
  the bundle (never hand-edited, never committed on the tool branch — batch lead
  produces the one canonical regen).
- **Service code:** no per-provider adapter. `standard_oauth` + `form_basic` +
  `userinfo` identity is fully declarative; no `service/adapter_*.go`. **One
  required integration-service edit (not a per-provider adapter):** the §3
  `refresh_lease` contract growth — `RuntimeStrategyStandardOAuth` in
  `model/runtime_contract.go` must accept `{OAuthLeaseNone, OAuthLeaseCredential}`
  (currently pins `OAuthLeaseNone`, exact-match). This is a blocking prerequisite
  of the bundle, owned by this batch; land it with a contract test. Do **not**
  assume `keap`/`signnow` already landed it — verified absent at this branch point.
- **Config:** `oauth.client_id` / `oauth.client_secret` appended to `config/` and
  the `deploy/` Helm Secret **together** (partial config fails service startup;
  fully-absent renders `configured:false` and is safe hidden).
- **UI icon:** `ui/helio-app/src/integrations/icons/hootsuite.svg` +
  register in `ui/helio-app/src/integrations/providerIcons.ts` (manual, never
  generated). `description_key: hootsuite` needs an i18n label entry.
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/`; bump plugin version + publish
  (one publish per batch).

## 5. Test plan — five layers

| Layer | Concretely for Hootsuite | External creds needed |
|---|---|---|
| **L1** anycli unit | `go test ./...` — `httptest` fakes for `me`, `profile list`, `message schedule` (assert path, `Bearer` header, JSON body, `Z`-suffix send-time validation, envelope unwrap, `--json` error from Hootsuite numeric codes 1005/1006). | No — fakes only |
| **L2** real-API harness | `ANYCLI_CRED_ACCESS_TOKEN=<tok> anycli hootsuite -- me` and `-- profile list`, and a `message schedule` to a sandbox profile with a future `scheduledSendTime`, then `message delete`. Confirms field names + injection match live API. **Also confirms the docs-stated refresh-token rotation** (POST `grant_type=refresh_token` twice with the first response's `refresh_token`; the second must fail because the token was consumed, and the first response must carry a new `refresh_token`). The default is `refresh_lease: credential` per §3; downgrade to `none` **only** if L2 empirically shows the refresh token is reusable (non-rotating), contradicting the docs. | **Yes** — real Hootsuite access token from the test-account pool (lane 2) |
| **L3** generation + suites | `provider-gen --check` green; `helio-cli` + `integration-service` unit suites pass with a local `go.mod replace` pointing at this anycli branch. Bundle expected to fail `--check` in CI on-branch until batch-end (§2) — do not commit local regens. | No |
| **L4** singleton + seeded creds | `POST /internal/test-only/connections/seed` with `provider:"hootsuite"`, real `org_id`/`owner_user_id`/`assistant_id` from a seeded AI-user fixture, seeding **both** `access_token` and `refresh_token` with a short `expires_at` so the next call forces the token-gateway refresh-and-write-back path (Hootsuite tokens are ~1h expiring — the expiring-OAuth guidance, not the bot-token shortcut). Then `heliox tool hootsuite -- profile list` returns live data. Runs against the **hidden** provider as-is. | **Yes** — a real access+refresh token pair from the test account |
| **L5** full connect flow | Once, hidden, before the visible flip: `heliox tool hootsuite auth` → complete Hootsuite OAuth consent on the dev/sandbox app → `oauth_connected` system event fires on the originating channel → run an unseeded `hootsuite -- message schedule …` through the new connection. Human-in-the-loop (oauth L5, master plan §2 lane 3). | **Yes** — a real Hootsuite login + a registered dev-mode app (lane 1) |

**Blocked-on-humans summary:** dev-mode app registration (lane 1) gates L4/L5;
API-access **approval** (the `oauth_review` review clearance) gates **only** the
`visible: true` flip, never dev/L4/merge. Test account with connected social
profiles (lane 2) gates L2/L4/L5.
