# Tool design: Formstack

Per-tool design for catalog row 216 (master plan `docs/design/008-300-integrations-rollout-plan.md`,
wave 3, Forms & Surveys). Scratch file on branch `tool/formstack`; the batch lead strips it at
batch end.

| Axis | Value | Notes |
|---|---|---|
| ① CLI command word | `formstack` | Flat command (`heliox tool formstack`); no group, no `tool.command` |
| ② anycli tool id | `formstack` | `definitions/tools/formstack.json`; Go package `internal/tools/formstack/` |
| ③ provider catalog key | `formstack` | `integrations/providers/formstack/` |

② == ③, a mechanical identity — **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go`.

Catalog lane: `oauth_light`. **Verified against official docs — the lane holds, with one
generation caveat and one integration-service decode risk (§3.4, §3.5).** Formstack has no
row in `oauth-audit.md` because the audit scoped only the 250 pre-audit `api_key` tools;
Formstack was seeded `oauth_light` directly, so this doc is the first documented verification
of its lane.

## 1. Official API surface

### 1.1 Two API generations — we target classic v2

Formstack ships two distinct API generations, and the difference is load-bearing for the auth
lane:

- **API v2 (classic)** — base `https://www.formstack.com/api/v2/`, singular resource paths
  (`/form.json`, `/form/:id/submission.json`). Auth is **OAuth2 authorization-code** (multi-
  tenant: any Formstack user can authorize a registered app). Docs live under the `v2.0`
  version of the developer portal: `https://developers.formstack.com/v2.0/reference/...`
  (the unversioned `/reference/...` URLs now serve v2025 and 404/redirect for v2 slugs).
- **API v2025** — base `https://www.formstack.com/api/v2025/`, plural paths (`/forms/{id}`).
  Auth is **Personal Access Tokens only** (`fs_pat_...`); **no OAuth2 is documented for
  v2025**. PATs are user-created, expire in 30/60/90 days, and cannot be minted by a
  third-party app — i.e. v2025 alone would put Formstack in the `api_key` lane with
  expiring, manually rotated keys.

**Decision: wrap API v2.** It is the only generation with a multi-tenant OAuth flow (the
`oauth_light` lane), it is still fully documented and served, and its resource surface covers
everything an assistant needs. Divergence to record: the v2025 push means v2 is de facto
legacy-track; if Formstack ever sunsets v2 OAuth, this provider degrades to an api_key/PAT
model (bundle change, not a CLI rewrite — paths and shapes would move, service layer is
isolated). No sunset is announced anywhere in the official docs or changelog as of
2026-07-21.

Verified v2 facts (from `developers.formstack.com/v2.0/reference/*`):

- Auth header `Authorization: Bearer <access-token>`; JSON responses (`.json` suffix or none).
- Rate limit **14,400 calls per access token per day**; standard HTTP status codes
  (401 invalid credentials, 403 no access, 429 rate limited).
- Requests: URL-encoded params by default, JSON body with `Content-Type: application/json`.

### 1.2 Full v2 resource inventory (what exists)

Forms (list/create/get/update/delete/copy), Fields (per-form list/create, get/update/delete),
Submissions (per-form list/create, get/update/delete), Partial submissions (read/delete),
Folders (CRUD), Confirmation emails (CRUD), Notification emails (CRUD), Webhooks (per-form
list/create, get/update/delete). No user/identity endpoint exists in v2 — identity comes from
the token response (`user_id`), see §3.3.

### 1.3 What the tool wraps and why

Driven by what an AI teammate actually does with Formstack — forms are where structured data
*enters* an org, and the assistant's jobs are overwhelmingly: find the right form, understand
its fields, pull and filter responses, occasionally push a submission or wire responses to an
external URL. Selected surface:

| Assistant job | Endpoint(s) |
|---|---|
| "Find the client intake form" | `GET /form.json` (search, folder filter, pagination), `GET /folder.json` |
| "What does this form ask? / interpret field ids" | `GET /form/:id` (includes submission count, form URL), `GET /form/:id/field.json` |
| "Pull yesterday's responses / responses where email=X" | `GET /form/:id/submission.json` (min_time/max_time, search_field_x/search_value_x, sort, pagination, data) |
| "What did submission N say?" | `GET /submission/:id` |
| "Submit this on my behalf / log an entry" | `POST /form/:id/submission` (field_x values) |
| "Delete that test entry" | `DELETE /submission/:id` |
| "Stand up a quick RSVP form and give me the link" | `POST /form`, `POST /form/:id/field`, `POST /form/:id/copy` |
| "Push new responses to this URL" | `GET/POST /form/:id/webhook`, `DELETE /webhook/:id` |

Deliberately **out of scope for v1**: confirmation/notification email CRUD (form-builder
plumbing, humans do this in the UI), partial submissions (niche), folder create/update/delete
(read-only listing suffices for locating forms), submission `PUT` (edit-in-place of a
respondent's answers is a footgun; delete+resubmit covers the rare need), the v2025-only
surfaces (smart lists, portals, themes, prefill URLs, submit actions).

## 2. anycli definition

### 2.1 Stage-1 rubric: `service` type

No official Formstack CLI exists at all, so the `cli`-type test fails at the first clause.
Implement **`service` type** in `internal/tools/formstack/` against the v2 REST API —
matching 21 of 23 existing definitions.

### 2.2 `definitions/tools/formstack.json`

```json
{
  "name": "formstack",
  "type": "service",
  "description": "Formstack as a tool (OAuth 2.0 access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "FORMSTACK_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Registered in `internal/tools/register.go`: `RegisterService("formstack", &formstack.Service{})`
(registry line rides the batch-end merge; the definition file and package merge freely).

### 2.3 Package shape

Copy the `bitly` package shape (most recent minimal precedent; `notion` for the larger
patterns): `Service` struct with `BaseURL` (default `https://www.formstack.com/api/v2`),
`HC`, `Out`, `Err`; cobra tree with `SilenceUsage/SilenceErrors`; a shared `call()` helper
setting `Authorization: Bearer`, `Accept: application/json`, JSON body via
`Content-Type: application/json`; **401 → `execution.RejectCredential`**; other non-2xx → API
error carrying Formstack's `{"error": "..."}"` message. Exit codes: 0 success, 1 runtime/API
failure, 2 usage/parse errors.

### 2.4 Subcommands / verbs

```
formstack form list      [--search q] [--folder id] [--page n] [--per-page n] [--sort id|name-asc|desc]
formstack form get       <form-id>
formstack form fields    <form-id>
formstack form create    --name <name> [--folder id]
formstack form copy      <form-id>
formstack form delete    <form-id>
formstack field get      <field-id>
formstack field create   <form-id> --type <type> --label <label> [--required] [--options a,b,c] [--hidden]
formstack folder list
formstack submission list   <form-id> [--since ts] [--until ts] [--search field=value]... [--page n] [--per-page n] [--sort asc|desc] [--no-data] [--expand-data] [--encryption-password pw]
formstack submission get    <submission-id> [--encryption-password pw]
formstack submission create <form-id> --field id=value... [--read]
formstack submission delete <submission-id>
formstack webhook list    <form-id>
formstack webhook get     <webhook-id>
formstack webhook create  <form-id> --url <url> [--content-type json|form]
formstack webhook delete  <webhook-id>
```

Notes:
- `submission list` sends `data=true` by default (values inline; that is what an assistant
  wants); `--no-data` for metadata-only listing. `--search field=value` is repeatable and maps
  to the API's paired `search_field_{n}`/`search_value_{n}` params (0–10). `--since/--until`
  map to `min_time`/`max_time` (`YYYY-MM-DD [HH:MM:SS]`, US/Eastern per the API — documented
  in command help). `--encryption-password` maps to the `X-FS-ENCRYPTION-PASSWORD` header
  (encrypted forms); it is form data, not a stored credential, so a flag is correct.
- `submission create --field id=value` maps to the API's `field_<id>=<value>` body params.
- `form create` + `field create` cover the "spin up a quick form" job with the common field
  types (`text`, `textarea`, `email`, `number`, `select`, `radio`, `checkbox`, `datetime`,
  `phone`, `name`); advanced layout/logic stays in the Formstack builder (documented in help).
- Destructive verbs (`form delete`, `submission delete`, `webhook delete`) exist in the API
  and are included; `form delete` is a soft delete per the API docs.

### 2.5 JSON output shape

Bitly/notion passthrough contract: every command emits the provider's JSON response verbatim
on stdout (+ trailing newline); Formstack list responses already carry `total`/`pages`
envelopes, which we pass through untouched. A persistent `--json` flag is accepted for
uniformity (always-on). Errors: plain-text one-liner on stderr, plus the typed exit-code
contract above; `--json` error envelope follows the notion precedent.

### 2.6 TDD

Unit tests per command against an `httptest.Server` fake asserting: request method/path/query
(including `.json` suffixes, `search_field_x` pairing, `min_time`/`max_time` passthrough,
`X-FS-ENCRYPTION-PASSWORD` header), injected `Authorization: Bearer` header, body encoding
for create verbs, passthrough output, 401→RejectCredential, non-2xx error rendering. Never
hit the real API from unit tests. Harness (`make build-harness`) run against the real API is
mandatory before the pin bump (L2, §5).

## 3. Auth: `oauth_light` verification against official docs

### 3.1 Registration model — lane confirmed

- App registration is **fully self-serve** in any Formstack account:
  `https://www.formstack.com/admin/apiKey/main` (profile menu → API). No review, partner
  program, or marketplace gate is documented anywhere in the v2 docs. Rubric: self-serve
  registration, multi-tenant authorization-code flow → **`oauth_light` confirmed**.
- Multi-tenant: "The Formstack API uses OAuth2 for authorization. This allows Formstack users
  to authorize a 3rd party (your application) to access their Formstack account" — any
  Formstack user can authorize our one registered app; the token carries that user's own
  in-app form permissions.
- **Bonus for L2**: "Creating an application record will also create an access token" for the
  owning account — the dev harness run needs no OAuth ceremony, just the app's own token from
  the registration page.
- Registration constraint for lane 1: the `redirect_uri` "must have the same domain as the
  application" — the app record's website field must be registered with Helio's callback
  domain, and the token-exchange `redirect_uri` must begin with or match the authorize-time
  one (integration-service already replays the same redirect URI on exchange).
- Account-pool note (lane 2): v2 API access is plan-gated in Formstack (API available on
  higher form plans); the test account must be provisioned on an API-capable plan or trial.

### 3.2 Endpoints and wire shape

| | |
|---|---|
| Authorize | `GET https://www.formstack.com/api/v2/oauth2/authorize` — params `client_id`, `redirect_uri`, `response_type=code`. **No `scope` parameter exists** (access is all-or-nothing at the authorizing user's permission level). **No PKCE.** |
| Token | `POST https://www.formstack.com/api/v2/oauth2/token` — form-encoded body `grant_type=authorization_code`, `client_id`, `client_secret`, `redirect_uri`, `code` → `token_exchange_style: form_secret` (client id+secret in body, exactly the documented curl example). |

Documented token response:

```json
{
  "token_type": "Bearer",
  "access_token": "abcdefg123456",
  "expires_in": "3600",
  "scope": null,
  "refresh_token": "abcdefg123456",
  "user_id": "12345"
}
```

### 3.3 Identity

v2 has **no userinfo endpoint** (no `/user` resource exists in the REST inventory). The token
response carries `user_id` → `identity.source: token_response`, `stable_key: /user_id`,
`label_candidates: [/user_id]`. Label UX is a bare numeric id — accepted (the notion
`workspace_id` fallback precedent); nothing better exists in v2.

### 3.4 Token semantics — the open empirical question

The docs' token example shows `expires_in: "3600"` **and** a `refresh_token`, but:

- **No refresh grant is documented** — the only documented `grant_type` is
  `authorization_code`; the error enum includes `unsupported_grant_type`.
- Long-standing ecosystem behavior treats Formstack v2 access tokens as **long-lived /
  non-expiring** (the v2 overview calls the app's token an "API key" and help-center flows
  treat it as a stable credential); the example response is consistent with generic OAuth2
  server boilerplate.

Resolution path (blocking the L4 seed-strategy choice, not dev): at lane-1 app creation,
run one manual `curl` code exchange and inspect the real response. Two outcomes:

- **Long-lived token (expected)**: bundle stays as §4 (`refresh_lease: none` semantics via
  absent expiry — `tokenResponse.expiry()` returns nil when `expires_in <= 0`); L4 seeds
  `access_token` only (Slack-bot pattern, no refresh cycle to exercise).
- **Really expires in 3600s with a working (undocumented) `refresh_token` grant**: L4 seeds
  `access_token` + `refresh_token` + short `expires_at` to force the gateway refresh path
  (A3); record the confirmed refresh grant in the bundle PR description.

### 3.5 Integration-service decode risk (flag to batch lead)

The documented response encodes `expires_in` as a JSON **string** (`"3600"`). integration-
service `service/oauth_exchange.go` decodes `ExpiresIn int` with a strict `json.Unmarshal`,
which **hard-fails on a string value** ("cannot unmarshal string into Go struct field") and
would abort the entire callback exchange → an L5 blocker *if* the live server really emits a
string. Same empirical check as §3.4 resolves it. If confirmed: the fix belongs in
integration-service generically (tolerant `json.Number`-style decode for `expires_in`), not
in a Formstack adapter — this is exactly the "grow the generic capability, don't fork an
adapter" rule from `references/provider-yaml.md`. Coordinate with the batch lead before the
batch-end merge; if the live server emits an integer (docs example imprecise), no change.

No revocation endpoint is documented → `disconnect_mode: local_only`.

## 4. Helio provider bundle plan

`integrations/providers/formstack/provider.yaml` (hidden-first; `standard_oauth`, zero
service-side Go barring §3.5):

```yaml
schema: helio.provider/v1
key: formstack
go_name: Formstack

presentation:
  name: Formstack
  description_key: formstack
  consent_domain: formstack.com
  visible: false           # hidden-first; flip + regen is the single go-live change
  order: 300               # provisional; batch lead owns final ordering

auth:
  type: oauth
  owner: individual        # the provider sees a person; token carries that user's form permissions
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.formstack.com/api/v2/oauth2/authorize
    token_url: https://www.formstack.com/api/v2/oauth2/token
    token_exchange_style: form_secret
    pkce: none
    authorize_params: {}
    single_active_token: false
    refresh_lease: none
    # Formstack has no wire-level scope parameter (access follows the
    # authorizing user's in-app form permissions). Display-only capability
    # slugs, bitly-precedent; rendered via i18n tools.scopes.<slug>.
    display_scopes: [read_forms, read_submissions, create_submissions, manage_forms, manage_webhooks]

identity:
  source: token_response
  stable_key: /user_id
  label_candidates: [/user_id]

connection:
  mode: isolated
  disconnect_mode: local_only    # no revocation endpoint documented
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
  name: formstack
  kind: oauth
```

- No `experiment` flag (not a preview-gated family; follows batch norm — batch lead may
  override).
- No `tool.command` / `tool.group` — flat standalone brand, not a corporate family.
- Config: `oauth.client_id` + `oauth.client_secret` land as per-provider appends in `config/`
  **and** the Helm Secret under `deploy/` together (lane 1 owns landing; local uncommitted
  `config/cloud.yaml` entries for on-branch L4). All-absent = `configured: false` (safe
  hidden); partial = startup failure — id and secret always land together.
- UI icon: `ui/helio-app/src/integrations/icons/formstack.svg` + manual registration in
  `providerIcons.ts` (batch-end shared surface). i18n: provider description string for
  `description_key: formstack` + `tools.scopes.*` entries for the five display slugs.
- AI-facing docs: new provider sub-doc under `agents/plugins/heliox/skills/tool/` (flat
  provider precedent: `notion/`, `x/`), riding the batch's single plugin version bump +
  marketplace publish.
- Generated projections: run `provider-gen` + `--check` locally on-branch for validation
  only; **do not commit regens** — batch lead produces the canonical regen at batch end.
  helio-cli built against this anycli worktree via a local, uncommitted `go.mod` `replace`.

## 5. Test plan (five layers)

| Layer | What runs | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...` — httptest fakes per §2.6; no real API | none |
| L2 | `anycli formstack -- form list`, `-- submission list <id> --since ...`, `-- submission create ...` against the **real** v2 API via `ANYCLI_CRED_ACCESS_TOKEN=<token>` | **Yes** — Formstack test account on an API-capable plan (lane 2); token comes free with app registration (§3.1), no OAuth ceremony needed |
| L3 | local `provider-gen` + `provider-gen --check` against the branch bundle; helio-cli `go build ./... && go test ./cmd/heliox/cmds/tool/` with the local `replace`; integration-service unit suite | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `formstack`, real seeded token) + `heliox tool formstack -- form list` through the real token gateway | **Yes** — real access token (from the registered dev app, lane 1) reaching the live API; seed shape depends on §3.4 outcome: access-token-only (long-lived) vs access+refresh+short-expiry (refresh path A3) |
| L5 | one full `heliox tool formstack auth` → connect link → real Formstack consent → unseeded live run; confirm `oauth_connected` event; **gates the visible flip** | **Yes** — human-in-the-loop (lane 3), dev app client id/secret landed in integration-service config (lane 1), real test account login |

Layer-specific checks beyond the standard sweep:

- L2 must exercise at least one paired-search listing (`--search field=value`) and one
  `submission create`, since the `search_field_x`/`field_x` param mapping is the most
  API-shape-sensitive code in the service.
- The §3.4/§3.5 manual curl exchange happens at lane-1 app creation, **before** L4 seeding,
  and its outcome is recorded in the batch notes (seed shape + whether the
  `oauth_exchange.go` decode fix is needed before L5).
- L4 negative check: run once with a revoked/garbage seeded token and confirm the 401 →
  `RejectCredential` path surfaces as a credential error, not a generic failure.

## 6. Rollout

Standard hidden-first: bundle lands `visible: false` in the batch-end merge; anycli tag +
pin bump by batch lead; L5 sweep post-merge; flip `visible: true` + regen as the single
go-live change. `oauth_light` — no external review clock; the only Formstack-specific
gating item is the §3.4 token-semantics check at app-creation time.
