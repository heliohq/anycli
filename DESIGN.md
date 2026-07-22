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
responses → identity), read-only, plus a generic GET escape hatch. Each wrapped
endpoint is mapped to the **exact scope it requires** (verified against
SurveyMonkey's official scope table — see the correction note below):

| anycli subcommand | Method + endpoint | Required scope | Why |
|---|---|---|---|
| `survey list` | `GET /v3/surveys` (paginated `page`/`per_page`) | `surveys_read` | Discover surveys by title/id |
| `survey get --id` | `GET /v3/surveys/{id}` | `surveys_read` | Survey metadata (title, counts, dates) |
| `survey details --id` | `GET /v3/surveys/{id}/details` | `surveys_read` | Pages + questions + answer-option ids (the map needed to interpret responses) |
| `response list --survey <id>` | `GET /v3/surveys/{id}/responses/bulk` (`page`/`per_page`, `status`) | **`responses_read_detail`** to read answers (`responses_read` alone returns only counts/metadata, no answer content) | Bulk responses: question ids → selected answer/choice ids |
| `response get --survey <id> --id <rid>` | `GET /v3/surveys/{id}/responses/{rid}/details` | **`responses_read_detail`** | One full response with all answers |
| `collector list --survey <id>` | `GET /v3/surveys/{id}/collectors` | **`collectors_read`** | Which collectors gathered responses |
| `me` | `GET /v3/users/me` | `users_read` | Identity / team id / plan (also the bundle identity endpoint) |
| `fetch --path <p>` | `GET /v3/<p>` | (varies by path) | Generic read escape hatch (notion `fetch` precedent) for anything not modeled |

**Write is deliberately out of v1**: `surveys_write`/`responses_write` require a
separate SurveyMonkey approval and widen the review surface with no matching
teammate demand. Revisit as a follow-up if needed.

### Scope correction — reading answers is a PAID-plan capability (baseline, not an edge case)

An earlier draft of this design was **factually wrong** about the scope split. The
official SurveyMonkey scope table is unambiguous:

- `responses_read` — "View if surveys in your account have responses and their
  metadata." Returns **no answer content** — only counts and response metadata (id,
  date, status, ip, etc.). Per the official scope table, **not** paid-gated (No).
- `responses_read_detail` — "View answers along with responses and answer counts and
  trends." This is the scope that returns actual answers (choice ids included), and
  per the official table it is the **only** read scope flagged **"Requires Paid
  SurveyMonkey Account? Yes."**
- `collectors_read` — "View collectors for your surveys and those shared with you."
  A **distinct** scope, and **not** paid-gated (No). `/v3/surveys/{id}/collectors`
  returns `1014` (permission not granted) only when the scope itself was not granted,
  never because of plan.

The real split is therefore **answers-vs-no-answers** (only answer reads are paid-
gated, via `responses_read_detail`), **not** free-text-vs-choice-ids, and **not**
"everything but structure is paid." `collector list` is free (it needs only the free
`collectors_read` scope); only `response list` / `response get` answer reads depend on
the paid `responses_read_detail`.

**Two distinct 403 error codes — do not conflate them (verified against the official
error table in `_overview.md`).** SurveyMonkey has a dedicated plan-gate code that is
separate from the scope-not-granted code:

| Code | Official message | Fires when | Service maps to |
|---|---|---|---|
| `1014` | "Permission has not been granted by the user to make this request." | The token lacks the scope the endpoint needs — e.g. `responses_read_detail` was requested **optional** and a free-plan account could not grant it, or `collectors_read` was absent. | On an answer/detail endpoint → "reading survey answers requires the `responses_read_detail` permission, which needs a paid SurveyMonkey plan — reconnect after upgrading." Otherwise → "the connected account has not granted the permission this request needs." |
| `1015` | "The user does not have the required plan to make this request." | The scope is granted but the account's **plan** does not permit the operation at call time (e.g. an account that downgraded after granting, or endpoint-level plan enforcement). | "the connected SurveyMonkey account's plan does not permit this request — a paid plan is required." |

An earlier draft named `1014` as *the* paid-plan gate and never mentioned `1015`.
That is wrong twice over: (a) the dedicated plan-gate code is `1015`, so a genuine
plan-gated call returning `1015` would have fallen straight through to an opaque 403;
and (b) for the actual free-plan answer-read path here the code is in fact `1014` (the
optional paid scope is simply ungranted, since a free plan cannot make it available to
grant). The service therefore maps **both** `1014` (on answer/detail endpoints) **and**
`1015` to a paid-plan-aware message, so neither falls through to an opaque 403.

**Decision: request the paid scope as OPTIONAL and keep a free-plan connection
useful.** `responses_read_detail` is requested **optional** (see the bundle section for
where optionality is configured), so a free-plan account can still complete OAuth and
use the free scopes. Consequence to document honestly (design, bundle, AI-facing doc):
**reading survey answers via the SurveyMonkey API requires the connected account to be
on a paid SurveyMonkey plan.** A free-plan connection can still `survey list` /
`survey get` / `survey details` (structure), `collector list`, and see response
**counts/metadata** via `response list`; only answer reads fail — with `1014` (the
ungranted optional scope) or, if a plan gate trips at call time, `1015` — surfaced as a
clear "reading answers requires a paid SurveyMonkey plan" message rather than an opaque
403. A metadata-only v1 that dropped the paid scope entirely was considered and
rejected: it cannot deliver the tool's core function.

> **Divergence recorded (independent verification).** The review suggested softening
> the free narrative to "Basic (free) plans can read answers for up to ~25
> responses/survey." I could **not** confirm any such API allowance in the official
> docs: the scope table marks `responses_read_detail` "Requires Paid … Yes" with no
> free-tier API carve-out, and the ~25-response figure is a SurveyMonkey **web-UI**
> product limit, not documented API behavior. Per the "follow official docs" rule this
> design does not assert a free API answer-read allowance; it states only what the API
> scope table supports (answers require the paid scope; free plans get structure +
> counts/metadata). The blanket "any answer read returns `1014` until upgrade" wording
> is corrected above to the precise `1014`/`1015` split rather than to an unverifiable
> 25-response claim.

### Multi-datacenter / region routing — `access_url` (correctness gap, now handled)

SurveyMonkey is **multi-datacenter**. The token-exchange response includes an
`access_url` field giving the correct API host for that account, and accounts served
from a non-default datacenter (e.g. EU data residency) use a host other than
`https://api.surveymonkey.com`. Hitting the wrong host returns error **1018** ("the
user does not have permission to access the host in this region"). An earlier draft
hardcoded `https://api.surveymonkey.com` everywhere and never mentioned `access_url` —
for any non-default-region account, identity resolution at connect **and** every
subsequent API call would fail with `1018`.

**v1 decision: scope to the default (US) host, and document the non-default-region
limitation as a known cap.** The token exchange itself runs on the default host
(`https://api.surveymonkey.com/oauth/token`) and works for all accounts; only the
per-account API host diverges. For default-region accounts `access_url ==
https://api.surveymonkey.com`, so a hardcoded base URL is correct. For a
non-default-region (e.g. EU) account, connect fails fast at identity resolution — the
anycli service maps `1018` to an explicit "this SurveyMonkey account is served from a
region not supported in v1" error rather than a silent/opaque failure (no silent
fallback, per the repo Architecture rule).

**Follow-up (deferred, and it IS integration-service capability growth):** full
region support means capturing `access_url` from the token-exchange response into
connection metadata and threading a per-account base URL into both the anycli service
(via a credential/env field) and the identity `userinfo` call. That is exactly the
kind of integration-service growth this design otherwise avoids, so it is called out
here as a follow-up to open only if non-default-region demand appears — not silently
assumed away. See the corrected capability note in the bundle section.

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
  `collectors_read`, `users_read` (all free, marked **required** in the app dashboard's
  OAuth settings) plus `responses_read_detail` (paid, marked **optional**). Note
  "required/optional" here is SurveyMonkey's app-settings terminology, distinct from
  "necessary to read answers." Rationale per the scope correction above:
  `responses_read_detail` is the only paid-gated read scope and the only one that
  returns answer content, so it is marked **optional** — that keeps a free-plan account
  from being blocked at the consent step (a *required* paid scope would force it to
  upgrade before it could OAuth at all), while paid accounts still grant it and get
  answers. `collectors_read` is a **free** scope needed for `collector list` (absent it,
  `1014`). `responses_read` lets free-plan connections see response counts/metadata. On
  a free/unentitled connection, answer reads return `1014` (optional paid scope
  ungranted) or `1015` (plan gate at call time); the service maps both to a "reading
  answers requires a paid SurveyMonkey plan" message. No write scopes.
- **Identity:** `GET <access_url>/v3/users/me` (requires `users_read`) → stable key
  `/id`; labels `/username`, `/email`, `/id`. v1 uses the default host
  `https://api.surveymonkey.com` (see the region note above); non-default-region
  accounts are an explicit known cap.

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
    # `scopes` is just the set requested in the authorize URL. required-vs-optional
    # is NOT expressed here (or anywhere in provider.yaml) — SurveyMonkey resolves it
    # from the app dashboard (developer.surveymonkey.com/apps → app settings): the
    # four free scopes are marked required, responses_read_detail (paid) optional, so
    # free-plan accounts can still complete OAuth. See the scope correction above.
    scopes: [surveys_read, responses_read, responses_read_detail, collectors_read, users_read]
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://api.surveymonkey.com/v3/users/me   # default (US) host; v1 known cap — see region note
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
service capability growth expected for the v1 (default-US-host) scope.** Marking
`responses_read_detail` **optional** is likewise **not** a schema/capability
concern: the `standard_oauth` `scopes` field only enumerates what to request in the
authorize URL, and SurveyMonkey resolves required-vs-optional from the app dashboard
settings — so no per-scope optionality needs expressing in `provider.yaml`. Two
caveats, both flagged now rather than mid-wave:

1. Confirm the `refresh_lease: none` + non-expiring-token combination is accepted
   by `provider_configuration.go` validation at implementation time; if not, that
   is a narrow growth to make.
2. The region follow-up above (capturing `access_url` into connection metadata and
   injecting a per-account base URL for non-default-region accounts) **is**
   integration-service capability growth — deliberately **out of v1 scope** and
   opened only if EU/non-default-region demand appears. v1 does not claim to
   support it; it fails fast (`1018` → explicit error) instead.

## Test plan → five layers (external-credential needs marked)

| Layer | What | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — httptest fake for `/v3/surveys`, `/details`, `/responses/bulk`, `/users/me`; assert Bearer injection, `page`/`per_page` params, `429`/`error` mapping, `--json` envelope, exit codes 0/1/2 | No |
| **L2** dev harness | `ANYCLI_CRED_ACCESS_TOKEN=<t> anycli surveymonkey -- survey list` (and `me`, `response list`, `collector list`) against the **real** API — mandatory before pin bump. To validate answer reads (`response list`/`response get` returning actual answers), the token must come from a **paid** SurveyMonkey account granted `responses_read_detail`; also confirm that on a **free-plan** token an answer read fails with `1014` (optional `responses_read_detail` ungranted), and that the service maps **both** `1014` (answer endpoints) and `1015` (plan gate) to the "reading answers requires a paid SurveyMonkey plan" message — never an opaque 403 | **Yes** — a real SurveyMonkey OAuth token; a **paid** account to exercise answer reads |
| **L3** generate + suites | `provider-gen` + `provider-gen --check`; helio-cli + integration-service unit suites; helio-cli build with local `replace` → anycli branch | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with **`access_token` only** (non-expiring, no refresh → Slack-bot-token pattern, omit `refresh_token`/`expires_at`), then `heliox tool surveymonkey -- survey list` through the real token gateway | **Yes** — a real token minted from the registered dev app (dev-app creation gates L4) |
| **L5** connect flow | `heliox tool surveymonkey auth` → consent on the dev/Private app → `oauth_connected` event → unseeded live run; once, hidden, before the visible flip. Human-in-the-loop (oauth_review, human lane 3) | **Yes** — real SurveyMonkey account + registered app; public-review clearance additionally gates the flip |

Rollout: land hidden in the Wave-3 batch; L1–L4 while hidden; L5 as the per-batch
sweep; flip `visible: true` + regenerate as the single go-live change once L5
passes and (for the public deployment) SurveyMonkey review clears.

## Sources

- SurveyMonkey OAuth + API overview (base URL, multi-datacenter `access_url`, error codes incl. `1014` scope-not-granted / `1015` plan-required / `1018` region): https://github.com/SurveyMonkey/public_api_docs/blob/main/includes/_overview.md
- OAuth scope table + required/optional app-settings model (`responses_read_detail` is the only read scope marked "Requires Paid … Yes"; `surveys_read`/`responses_read`/`collectors_read`/`users_read` = No; required-vs-optional is set in the app dashboard, and "all required scopes must be approved by and available to the user for the OAuth process to succeed"): https://github.com/SurveyMonkey/public_api_docs/blob/main/includes/_overview.md#scopes
- API docs portal: https://api.surveymonkey.com/v3/docs
- App registration: https://developer.surveymonkey.com/apps/
