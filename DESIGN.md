# Tool design — SurveyMonkey

Scratch design for the `surveymonkey` external tool provider (298-plan row 211,
Wave 3, category Forms & Surveys). Committed on branch `tool/surveymonkey`;
the batch lead strips it at batch-end. English only.

## Naming (three axes — master plan §3)

| Axis | Value | Notes |
|---|---|---|
| ① CLI command word | `surveymonkey` | Flat command; not a family, so **no** `tool.group`. `heliox tool surveymonkey -- …` |
| ② anycli tool id | `surveymonkey` | Brand is a single token; community-standard single word. `definitions/tools/surveymonkey.json` |
| ③ provider catalog key | `surveymonkey` | Mechanical id→key (no dashes to convert). Bundle dir `integrations/providers/surveymonkey/` |

**② == ③, so no `toolToProvider` entry is added** (`resolver.go`'s map is for
divergences only; identity holds here). Go package name = `surveymonkey`
(no dashes, no leading digit) → `internal/tools/surveymonkey/`.

## Catalog-vs-official-docs verification (independent judgment)

The catalog lanes SurveyMonkey `oauth_review`. SurveyMonkey is **not** in
`oauth-audit.md` (that audit re-laned only tools that were `api_key` before it;
SurveyMonkey was already `oauth_review`), so there is no audit row to reconcile.
Checked against the official docs — the lane is **correct, no divergence to record**:

- **Multi-tenant authorization-code OAuth exists.** Register an app at
  `https://developer.surveymonkey.com/apps/`, get `client_id`/`client_secret`,
  run the RFC-6749 authorization-code three-step flow. One app can be authorized
  by arbitrary SurveyMonkey accounts (one access token per authorized account).
- **A human review/publish gate stands before external accounts can authorize.**
  A *draft* app has a 90-day window and is usable only against the developer's
  own account; to let arbitrary customer accounts authorize, the app must be
  **deployed** — a **Public** deployment goes through SurveyMonkey's App
  Directory review. Additionally, the write scopes (`surveys_write`,
  `responses_write`) "require SurveyMonkey's approval to use in a Public app."
  Review gate before third-party consent ⇒ `oauth_review` per the audit rubric.

Hidden-first still applies: dev-mode (draft/private) app creation gates L4;
public-review clearance gates only the visible flip.

## API surface this tool wraps and why

Official REST API: base `https://api.surveymonkey.com`, all `/v3/*`, Bearer token,
JSON. Docs: `api.surveymonkey.com/v3/docs`, `github.com/SurveyMonkey/public_api_docs`.

What an AI teammate actually does with SurveyMonkey is **read and analyze survey
results** — "summarize the NPS survey," "how many people answered Q3," "pull the
latest responses." So v1 wraps the read path end to end (list → structure →
responses → identity), read-only, plus a generic GET escape hatch:

| anycli subcommand | Method + endpoint | Why |
|---|---|---|
| `survey list` | `GET /v3/surveys` (paginated `page`/`per_page`) | Discover surveys by title/id |
| `survey get --id` | `GET /v3/surveys/{id}` | Survey metadata (title, counts, dates) |
| `survey details --id` | `GET /v3/surveys/{id}/details` | Pages + questions + answer-option ids (the map needed to read responses) |
| `response list --survey <id>` | `GET /v3/surveys/{id}/responses/bulk` (`page`/`per_page`, `status`) | Bulk responses: question ids → selected answer/choice ids |
| `response get --survey <id> --id <rid>` | `GET /v3/surveys/{id}/responses/{rid}/details` | One full response |
| `collector list --survey <id>` | `GET /v3/surveys/{id}/collectors` | Which collectors gathered responses |
| `me` | `GET /v3/users/me` | Identity / team id / plan (also the bundle identity endpoint) |
| `fetch --path <p>` | `GET /v3/<p>` | Generic read escape hatch (notion `fetch` precedent) for anything not modeled |

**Write is deliberately out of v1**: `surveys_write`/`responses_write` require a
separate SurveyMonkey approval and widen the review surface with no matching
teammate demand. Revisit as a follow-up if needed.

Reading full free-text answer bodies (not just answer-option ids) needs
`responses_read_detail`, which is a **paid-plan** scope — noted below and in the
AI-facing doc so the assistant explains the cap rather than failing opaquely.

## anycli definition (stage-1 rubric → `service` type)

`service` type. No official SurveyMonkey CLI binary exists; the integration is a
REST API behind a Bearer token — none of the `cli`-type conditions
(`github`→`gh`) hold. Reference implementation to copy: `internal/tools/notion/`
(cobra tree grouped by resource, `BaseURL`/`HC`/`Out`/`Err` struct for httptest,
exit-code contract 0/1/2, `--json` error envelope).

Definition JSON (`definitions/tools/surveymonkey.json`):

```json
{
  "name": "surveymonkey",
  "type": "service",
  "description": "SurveyMonkey surveys and responses (OAuth token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SURVEYMONKEY_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Service reads `SURVEYMONKEY_ACCESS_TOKEN` and sends `Authorization: Bearer <t>`
plus `Accept: application/json`.

**JSON output shape.** Pass the provider's own JSON through on stdout (provider-
neutral, agent-consumable per built-in service conventions 003 §3). List
endpoints return SurveyMonkey's `{ "data": [...], "page", "per_page", "total",
"links": {...} }` envelope — surface it as-is so the agent can page via
`--page`/`--per-page`; do not invent a wrapper. Errors: exit 1 on API failure
with a typed `apiError`, exit 2 on usage/parse; `--json` emits the structured
error envelope; SurveyMonkey error bodies are `{ "error": { "id", "name",
"message", "http_status_code" } }` — map into the envelope.

Pagination/rate-limit notes for the impl: list endpoints are 1-based `page` +
`per_page`, follow `links.next` until absent; app-global daily rate limits
surface as `429` with `X-Ratelimit-App-Global-Day-Remaining` (no documented
`Retry-After`) — treat `429` as a distinct exit-1 apiError message.

## Credential fields & exact auth flow (`oauth_review` lane, verified)

Standard OAuth 2.0 authorization-code (RFC 6749). **No PKCE, no refresh tokens** —
the exchange yields a single **long-lived, non-expiring** access token, valid only
in combination with the app's `client_id` and only for the one authorized account.

- **Registration model:** self-serve draft app at
  `developer.surveymonkey.com/apps/` (client id/secret issued immediately);
  deploy Private (own org) or Public (App Directory review) to go beyond the
  90-day draft window and serve arbitrary accounts. Dev-mode/draft creation is
  enough for L4/L5; public review gates only the visible flip.
- **Authorize:** `GET https://api.surveymonkey.com/oauth/authorize`
  `?response_type=code&client_id=…&redirect_uri=…&scope=…`
- **Token exchange:** `POST https://api.surveymonkey.com/oauth/token` with
  `client_id`, `client_secret`, `code`, `redirect_uri`,
  `grant_type=authorization_code` as **form-encoded body** params (client secret
  in the body, not HTTP Basic) → `token_exchange_style: form_secret`.
- **Token semantics:** response is `{ "access_token", "token_type": "bearer" }`
  with **no `refresh_token` and no `expires_in`** — the token does not expire
  unless revoked. So the bundle sets `refresh_lease: none`, and the credential
  carries `access_token` only (no refresh). SurveyMonkey exposes **no public
  token-revoke endpoint** (users disconnect apps from account settings) →
  `disconnect_mode: local_only`.
- **Scopes requested (read-only v1):** `surveys_read`, `responses_read`,
  `users_read` (all free-plan scopes). Optional `responses_read_detail` for full
  free-text answer bodies is **paid-plan gated** — request it but document that
  connect/analysis on a free SurveyMonkey plan is limited to answer-option ids.
- **Identity:** `GET https://api.surveymonkey.com/v3/users/me` (requires
  `users_read`) → stable key `/id`; labels `/username`, `/email`, `/id`.

## Helio provider bundle plan (`standard_oauth`, hidden-first)

`integrations/providers/surveymonkey/provider.yaml` — a plain `standard_oauth`
bundle; **no `service/adapter_*.go`** (standard exchange body, standard bearer
identity via a separate userinfo GET — inside the closed capability set, so zero
provider-specific Go). `presentation.visible: false` at first.

```yaml
schema: helio.provider/v1
key: surveymonkey
go_name: SurveyMonkey

presentation:
  name: SurveyMonkey
  description_key: surveymonkey
  consent_domain: surveymonkey.com
  visible: false          # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://api.surveymonkey.com/oauth/authorize
    token_url: https://api.surveymonkey.com/oauth/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [surveys_read, responses_read, users_read]
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://api.surveymonkey.com/v3/users/me
  stable_key: /id
  label_candidates: [/username, /email, /id]

connection:
  mode: isolated
  disconnect_mode: local_only
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
  name: surveymonkey
  kind: oauth
```

Config: `oauth.client_id`/`oauth.client_secret` land in integration-service
config — `config/` locally **and** the Helm Secret under `deploy/` together
(Config Sync hard rule), as lane-1 per-provider appends; id and secret always in
the same change (a partially-configured provider fails startup). No experiment
flag (GA path). UI icon `ui/helio-app/src/integrations/icons/surveymonkey.svg`
+ manual `providerIcons.ts` register; i18n `description_key: surveymonkey`.

**Capability check:** `standard_oauth` + `form_secret` + `pkce: none` +
`refresh_lease: none` + `identity.source: userinfo` + `disconnect_mode:
local_only` are all existing reviewed enum values (notion uses `json_basic`+
`token_response`; linkedin uses `userinfo`+`form_secret`). **No integration-
service capability growth expected.** Confirm the `refresh_lease: none` +
non-expiring-token combination is accepted by `provider_configuration.go`
validation at implementation time; if not, that is the only possible narrow
growth — flagged now, not mid-wave.

## Test plan → five layers (external-credential needs marked)

| Layer | What | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — httptest fake for `/v3/surveys`, `/details`, `/responses/bulk`, `/users/me`; assert Bearer injection, `page`/`per_page` params, `429`/`error` mapping, `--json` envelope, exit codes 0/1/2 | No |
| **L2** dev harness | `ANYCLI_CRED_ACCESS_TOKEN=<t> anycli surveymonkey -- survey list` (and `me`, `response list`) against the **real** API — mandatory before pin bump | **Yes** — a real SurveyMonkey OAuth access token from a test account |
| **L3** generate + suites | `provider-gen` + `provider-gen --check`; helio-cli + integration-service unit suites; helio-cli build with local `replace` → anycli branch | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with **`access_token` only** (non-expiring, no refresh → Slack-bot-token pattern, omit `refresh_token`/`expires_at`), then `heliox tool surveymonkey -- survey list` through the real token gateway | **Yes** — a real token minted from the registered dev app (dev-app creation gates L4) |
| **L5** connect flow | `heliox tool surveymonkey auth` → consent on the dev/Private app → `oauth_connected` event → unseeded live run; once, hidden, before the visible flip. Human-in-the-loop (oauth_review, human lane 3) | **Yes** — real SurveyMonkey account + registered app; public-review clearance additionally gates the flip |

Rollout: land hidden in the Wave-3 batch; L1–L4 while hidden; L5 as the per-batch
sweep; flip `visible: true` + regenerate as the single go-live change once L5
passes and (for the public deployment) SurveyMonkey review clears.

## Sources

- SurveyMonkey OAuth + API overview: https://github.com/SurveyMonkey/public_api_docs/blob/main/includes/_overview.md
- API docs portal: https://api.surveymonkey.com/v3/docs
- App registration: https://developer.surveymonkey.com/apps/
