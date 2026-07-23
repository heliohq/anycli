# Tool design: PostHog (`posthog`)

Catalog row #121 — Product: PostHog · anycli id `posthog` · provider key `posthog` ·
auth lane `oauth_light` · wave 2 · category Analytics.

Scratch design doc for branch `tool/posthog` (batch lead strips it at batch end).
Everything below was verified against PostHog's official docs
(https://posthog.com/docs/api, https://posthog.com/docs/api/oauth), the live RFC 8414
metadata at `https://oauth.posthog.com/.well-known/oauth-authorization-server`, and the
PostHog open-source server (`PostHog/posthog`: `posthog/settings/web.py` OAUTH2_PROVIDER,
`posthog/api/oauth/views.py`, `services/oauth-proxy/`) — not inherited from the catalog
or the audit.

## 1. Naming (master plan §3)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `posthog` (flat command, no group) | — (no `tool.command`) |
| ② anycli tool id | `posthog` | `definitions/tools/posthog.json` |
| ③ provider catalog key | `posthog` | `integrations/providers/posthog/` |

②==③, so **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.
Go package: `internal/tools/posthog/`.

## 2. Verified provider facts (and divergences from the catalog/audit)

### OAuth server (live metadata + source, 2026-07-21)

- Endpoints (region-agnostic proxy, recommended by official docs for multi-region
  integrations; also per-region on `us.posthog.com` / `eu.posthog.com`):
  - authorize: `https://oauth.posthog.com/oauth/authorize/`
  - token: `https://oauth.posthog.com/oauth/token/`
  - revoke: `https://oauth.posthog.com/oauth/revoke/`
  - userinfo: `https://oauth.posthog.com/oauth/userinfo/` (the proxy tries US then EU —
    `services/oauth-proxy/src/handlers/passthrough.ts`)
- `token_endpoint_auth_methods_supported`: `["none", "client_secret_post"]`,
  `code_challenge_methods_supported`: `["S256"]`, response type `code` only.
- **PKCE is mandatory for every flow** — `OAUTH2_PROVIDER["PKCE_REQUIRED"] = True`
  ("We require PKCE for all OAuth flows - including confidential clients",
  `posthog/settings/web.py`).
- Client registration: **CIMD** (Client ID Metadata Document,
  draft-parecki-oauth-client-id-metadata-document) is the documented third-party path:
  the `client_id` IS an HTTPS URL we control, hosting a JSON doc with `client_id`,
  `client_name`, `redirect_uris`, optional `logo_uri`. No registration step at all.
  PostHog caches the doc per our `Cache-Control: max-age`.
- DCR also exists (`/oauth/register/`, open self-serve). **Probed live**: it issues
  `"token_endpoint_auth_method": "none"` clients and returns **no `client_secret`**.
  So there is **no self-serve confidential-client path at all** — `client_secret_post`
  is for PostHog first-party apps only.
- Token semantics (`posthog/settings/web.py`, `posthog/api/oauth/views.py`):
  - access token `pha_…`: 1 hour standard; **7 days for DCR/CIMD clients**
    (`EXTENDED_ACCESS_TOKEN_EXPIRE_SECONDS`); opaque, DB-backed.
  - refresh token `phr_…`: 30 days, **rotated on every refresh**
    (`ROTATE_REFRESH_TOKEN: True`) with **reuse protection** — reusing an old refresh
    token after a 2-minute grace window revokes the whole session.
  - refresh grant: `refresh_token` + `client_id` (+ nothing else for public clients).
  - Both prefixes are auto-revoked if caught by GitHub secret scanning — never commit
    a real token anywhere.
- Scopes: OAuth scopes mirror personal-API-key scopes, `resource:read|write` form
  (197 in live metadata; e.g. `query:read`, `insight:read`, `feature_flag:write`,
  `user:read`). `openid` is the default scope; `openid profile email` drive userinfo
  claims. A CIMD doc may declare a `com.posthog.scopes` ceiling — anything we request
  at authorize time must be inside it.
- Verification: optional, not required to go live; unverified apps show an
  "unverified application" notice on the consent screen; email
  team-growth@posthog.com to get verified (cosmetic — do before the visible flip if
  we care, non-blocking).

### Divergences to record

1. **Audit verdict "oauth_light" is confirmed but understated**: there is literally no
   app registration (CIMD). Human lane 1's job for this tool is **hosting one static
   JSON file** on a Helio domain + config append — no provider console, no wait.
2. **Helio's `standard_oauth` lane cannot express PostHog today.** The generated
   config contract hard-requires `oauth.client_id` **and** `oauth.client_secret`
   (`cmd/provider-gen/validate.go` `validateConfigContract` default branch), and every
   `token_exchange_style` (`form_secret|form_basic|json_basic`) transmits a client
   secret (`service/oauth_exchange.go`). PostHog third-party clients are
   **secretless public clients (auth method `none`) with mandatory PKCE S256**.
   Per the skill's own guidance (`references/provider-yaml.md`: grow the reviewed
   generic capability set rather than write an adapter), this tool needs one new
   reviewed enum value — see §5.3. This is a **hard prerequisite for L4-refresh/L5**,
   not an adapter case and not a reason to change lanes.
3. **Region split is real and lives on the API side, not the OAuth side.** OAuth is
   region-agnostic through `oauth.posthog.com`, but REST calls must hit the user's own
   region (`us.posthog.com` / `eu.posthog.com`); nothing in the closed
   `CredentialSource` set (`model/catalog.go`) can carry a region today. The anycli
   service resolves the region at runtime (§4.5) so the Helio side needs zero new
   credential plumbing.

### REST API facts

- Private endpoints: `https://us.posthog.com` / `https://eu.posthog.com`
  (self-hosted: own domain). Auth: `Authorization: Bearer <token>` — works identically
  for OAuth access tokens (`pha_`) and personal API keys (`phx_`), which is what makes
  the L2 harness runnable on a personal key.
- Query endpoint: `POST /api/projects/:project_id/query/` with
  `{"query": {"kind": "HogQLQuery", "query": "select …"}}`; default 100 rows, `LIMIT`
  up to 50k; keyset pagination on `timestamp` (OFFSET → HTTP 400 for personal keys);
  explicitly not for bulk export.
- Rate limits (org-wide): analytics 240/min & 1,200/h; `query` 1,200/h class of its
  own (docs page states the /query budget separately); CRUD 480/min & 4,800/h.
  Surface 429 bodies verbatim; no client-side retry loops (agents decide).
- Public capture endpoints (`/i/v0/e`) use the **project token**, not user auth —
  out of scope for this tool (Helio assistants read/operate analytics; they don't
  instrument apps through a CLI).

## 3. What an AI teammate does with PostHog → wrapped surface

Driving jobs: "how did the launch do?", "query product analytics ad hoc (HogQL)",
"check/flip a feature flag", "annotate the deploy", "look up what we track", "pull an
insight/dashboard the team already built", "check experiment results", "who is this
user / which cohort".

Wrapped endpoints (all under `/api/projects/:project_id/…` unless noted):

| Command | Method + path | Scope |
|---|---|---|
| `whoami` | `GET /api/users/@me` (also prints resolved region host) | `user:read` |
| `project list` | `GET /api/projects/` | `project:read` |
| `query run` | `POST …/query/` (HogQL string or raw query JSON passthrough) | `query:read` |
| `insight list\|get` | `GET …/insights/`, `GET …/insights/:id/` | `insight:read` |
| `dashboard list\|get` | `GET …/dashboards/`, `GET …/dashboards/:id/` | `dashboard:read` |
| `flag list\|get\|create\|update\|toggle` | `GET/POST …/feature_flags/`, `GET/PATCH …/feature_flags/:id/` (`toggle` = PATCH `active`) | `feature_flag:read/write` |
| `annotation list\|create` | `GET/POST …/annotations/` (mark deploys/launches) | `annotation:read/write` |
| `person list\|get` | `GET …/persons/?search=`, `GET …/persons/:id/` | `person:read` |
| `cohort list` | `GET …/cohorts/` | `cohort:read` |
| `experiment list\|get` | `GET …/experiments/`, `GET …/experiments/:id/` | `experiment:read` |
| `recording list` | `GET …/session_recordings/` (metadata only) | `session_recording:read` |
| `event-definition list` | `GET …/event_definitions/?search=` (HogQL authoring aid) | `event_definition:read` |
| `property-definition list` | `GET …/property_definitions/?search=` | `property_definition:read` |

Deliberately v1-excluded: event capture (project-token auth), insight/dashboard
creation (HogQL `query run` covers ad hoc analysis; creating shared assets is a human
act), surveys/error-tracking/data-warehouse CRUD (later, additive), person deletion
(destructive, no agent job needs it).

Write set is intentionally small: `feature_flag:write` + `annotation:write` only.

## 4. anycli implementation

### 4.1 Stage-1 rubric → `service` type

No official PostHog CLI wraps this surface (the wizard CLI is a setup tool, not an API
client). `service` type, like 21/23 existing definitions.

### 4.2 Definition (`definitions/tools/posthog.json`)

```json
{
  "name": "posthog",
  "type": "service",
  "description": "PostHog product analytics as a tool (OAuth access token or personal API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "POSTHOG_ACCESS_TOKEN"}
      }
    ]
  }
}
```

`access_token` is the **only** credential binding. A binding's `Source.Field` is
projected into the public `ToolManifest.CredentialFields` unconditionally
(`manifest.go`), and helio-cli's `TestGeneratedToolProvidersMatchPinnedAnyCLI`
requires every manifest field to be projected by the Helio provider bundle — so
declaring an "optional" `api_host` binding would force a Helio-side projection
that has no source (§2: nothing in the closed `CredentialSource` set carries a
region today). The region host is therefore resolved at runtime (§4.5, US→EU
probe), not supplied as a credential. `POSTHOG_API_HOST` remains an
**environment override** the service reads directly from its env
(`posthog.go`) — used by the httptest/`client_test.go` seam and reachable by a
self-hosted runtime that sets the raw env var — but it is not an anycli
credential and not projected by Helio.

### 4.3 Package layout (`internal/tools/posthog/`, notion/bitly shape)

`Service{BaseURL, HC, Out, Err}` (BaseURL override = httptest seam; when set it also
disables region discovery), duck-typed `Execute(ctx, args, env)`, registered as
`RegisterService("posthog", &posthog.Service{})` in `internal/tools/register.go`
(registration line rides the batch-end merge; the definition + package merge freely).

Files: `posthog.go` (root + exec contract), `client.go` (HTTP + auth header + region
resolution + typed `apiError`), `query.go`, `project.go`, `insight_dashboard.go`,
`flag.go`, `annotation.go`, `person_cohort.go`, `experiment.go`, `recording.go`,
`definitions.go` (+ `_test.go` mirrors, `harness_test.go`).

### 4.4 CLI/JSON contract

- Passthrough of the provider JSON body on stdout + trailing newline (bitly precedent);
  list commands pass through PostHog's `{"count", "next", "previous", "results"}`
  pagination envelope untouched; `--limit/--offset`-style paging is exposed as flags
  that map to query params.
- `query run --project <id> (--hogql "<sql>" | --query-json <file|->)`: `--hogql`
  wraps the string as `{"query":{"kind":"HogQLQuery","query":…}}`; `--query-json` is a
  raw query-node passthrough for advanced kinds (TrendsQuery, FunnelsQuery).
- Exit codes per the notion contract: 0 success; 1 runtime/API failure (typed
  `apiError`: HTTP status + PostHog `{"type","code","detail"}` body surfaced verbatim,
  plus a `--json` structured error envelope); 2 usage/parse errors.
- 401 → credential-rejected wording (feeds heliox stale-credential feedback); 429 →
  body passthrough with exit 1, no retry.
- Non-interactive throughout; every input is a flag.

### 4.5 Region resolution (the one PostHog-specific mechanism)

Order per invocation:
1. `POSTHOG_API_HOST` env, if set (an environment override read directly from the
   process env — httptest/`client_test.go` seam, or a self-hosted runtime that
   sets the raw var; it is not an anycli credential binding, see §4.2) — used
   as-is, no probe.
2. Probe `GET https://us.posthog.com/api/users/@me` with the Bearer token: any
   response **except 401** (200, 403 scope-denied, …) proves the token is known to
   that region → use it. On 401, probe EU the same way. Both 401 → explicit
   "token not recognized in US or EU" error, exit 1. (Same try-both strategy PostHog's
   own oauth-proxy uses for userinfo.)
Resolved host is cached for the process lifetime (one heliox invocation = one probe at
most, and zero when the first API call is itself `whoami`-shaped).

## 5. Helio provider bundle plan

### 5.1 `integrations/providers/posthog/provider.yaml` (hidden-first)

```yaml
schema: helio.provider/v1
key: posthog
go_name: PostHog

presentation:
  name: PostHog
  description_key: posthog
  consent_domain: posthog.com
  visible: false          # flip only after L5 (master plan: single go-live change)
  order: <pick unoccupied at batch end>

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id]   # CIMD URL; NO client secret exists
  oauth:
    authorize_url: https://oauth.posthog.com/oauth/authorize/
    token_url: https://oauth.posthog.com/oauth/token/
    token_exchange_style: form_public          # NEW reviewed enum value — §5.3
    pkce: s256                                 # mandatory server-side (PKCE_REQUIRED)
    authorize_params: {}
    scopes:
      - openid                                  # userinfo /sub = AccountKey source
      - email
      - profile
      - user:read
      - organization:read
      - project:read
      - query:read
      - insight:read
      - dashboard:read
      - feature_flag:read
      - feature_flag:write
      - annotation:read
      - annotation:write
      - person:read
      - cohort:read
      - experiment:read
      - session_recording:read
      - event_definition:read
      - property_definition:read
    display_scopes: [read_analytics, run_queries, manage_feature_flags, manage_annotations]
    single_active_token: false
    refresh_lease: credential   # rotation + reuse protection ⇒ serialize refreshes
    revoke:
      url: https://oauth.posthog.com/oauth/revoke/
      client_auth: none
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://oauth.posthog.com/oauth/userinfo/   # proxy tries US then EU
  stable_key: /sub
  label_candidates: [/email, /sub]

connection:
  mode: isolated
  disconnect_mode: provider_revoke   # PostHog-side revoke via the revoke block above
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
  name: posthog
  kind: oauth
```

Notes:

- `disconnect_mode: provider_revoke` is required, not optional, given the design's
  intent to revoke on the PostHog side (`https://oauth.posthog.com/oauth/revoke/`).
  The generator contract couples the two fields: for `runtime_strategy: standard_oauth`,
  `validateOAuthRevocation` (`go-services/integration-service/cmd/provider-gen/validate.go`,
  the `switch m.Connection.DisconnectMode` block) **forbids** an `auth.oauth.revoke`
  block when `disconnect_mode` is `local_only` and **requires** one when it is
  `provider_revoke`. `local_only` means "do not call provider revoke," so pairing it
  with a revoke block is incoherent and fails `provider-gen --check` — the exact L3 gate
  the §6 test plan and master plan §2 rely on. We keep the `revoke:` block and set
  `provider_revoke`; L3 must confirm `provider-gen --check` passes on the corrected
  bundle before the batch-end merge. The revoke call itself is `client_auth: none`
  (public client — no client secret to present), targeting the rotated `refresh_token`
  with `access_token` as fallback.
- `refresh_lease: credential` is load-bearing — PostHog rotates refresh tokens
  with reuse protection (2-min grace, then full session revocation), so concurrent
  refreshes for one credential must be serialized and the rotated `phr_` must be written
  back (integration-service already writes back `newTok.RefreshToken`,
  `service/token_refresh.go`). Access-token expiry for CIMD clients is 7 days, so
  refreshes are rare in practice; correctness still demands the lease.

### 5.2 CIMD document (replaces app registration — human lane 1)

One static JSON hosted at a stable Helio HTTPS URL (exact home is lane 1's call;
recommendation: a checked-in static asset on a Helio-owned domain, e.g.
`https://helio.im/.well-known/helio-oauth-client.json`):

```json
{
  "client_id": "https://helio.im/.well-known/helio-oauth-client.json",
  "client_name": "Helio",
  "logo_uri": "https://helio.im/<logo>.png",
  "redirect_uris": [
    "https://api.helio.im/oauth/callback",
    "https://rollout.lifecycle.so/oauth/callback",
    "http://localhost:<singleton-port>/oauth/callback"
  ]
}
```

- One document serves every environment: `redirect_uris` must list each environment's
  `oauth.redirect_url` exactly (scheme/host/port/path); http is allowed only for
  loopback (covers the local singleton).
- `oauth.client_id` config value in every env = the document URL itself.
- Serve with a short `Cache-Control: max-age` initially — PostHog caches the doc, and
  redirect-URI additions wait out the TTL.
- If a scope ceiling is declared (`com.posthog.scopes`), it must be a superset of the
  bundle's `scopes` list.
- Config Sync rule still applies (`config/` + `deploy/` land together), but the only
  secret-shaped value is… nothing: `client_id` is a public URL. A provider with all
  config fields absent renders `configured: false`, same hidden-safe behavior as usual.

### 5.3 Required integration-service extension (prerequisite, generic — not an adapter)

Add one reviewed capability to the `standard_oauth` closed set — **public client**:

- `token_exchange_style: form_public`: form-encoded code exchange sending `client_id`,
  `code`, `redirect_uri`, `code_verifier` — **no `client_secret` parameter at all**
  (PostHog's DOT server registers third-party clients with auth method `none`).
- provider-gen validation: `form_public` requires `pkce: s256` and relaxes
  `validateConfigContract` to require only `oauth.client_id`.
- Refresh path (`service/token_refresh.go`, `requestOAuthRefresh`): `form_public`
  must **explicitly** set `endpoint.AuthStyle = oauth2.AuthStyleInParams` (client_id in
  the form body, no `client_secret`, no `Authorization` header) in addition to bypassing
  the Basic-auth credential check. This is not automatic today: the current code sets
  `AuthStyleInHeader` only for Basic-auth styles (`UsesHTTPBasicClientAuth`) and
  otherwise leaves `AuthStyleAuto`. Under `AuthStyleAuto` with an empty `ClientSecret`,
  `golang.org/x/oauth2` may probe HTTP Basic first (client_id + empty password), which
  PostHog's secretless public-client token endpoint (auth method `none`) can reject —
  breaking the mandatory L4(b) rotation/write-back path. Selecting `AuthStyleInParams`
  is what actually pins the secretless body form; the library does not choose it on its
  own. Assert this with the synthetic-provider unit test below: the refresh request must
  carry `client_id` in the form and no `Authorization` header.
- `provider_configuration.go`: configured == `client_id` present for `form_public`
  providers.
- Proven by a synthetic-provider unit test (the existing standard_oauth test pattern);
  Klaviyo-class PKCE providers and future CIMD/DCR providers (this will not be the
  last public-client SaaS) reuse it.

This lands as a normal integration-service change reviewed with the batch — no
`service/adapter_*.go`.

### 5.4 Other Helio artifacts

- Icon: `ui/helio-app/src/integrations/icons/posthog.svg` (official hedgehog mark, SVG)
  + manual `providerIcons.ts` registration (batch-end shared surface).
- i18n: `tools.desc.posthog` + `tools.scopes.{read_analytics,run_queries,manage_feature_flags,manage_annotations}` in all locales.
- AI-facing docs: provider sub-doc `agents/plugins/heliox/skills/tool/posthog.md`
  (region note, project-id discovery via `project list`, HogQL quick reference,
  flag-toggle caution); plugin version bump + publish ride the batch.
- No resolver entry, no group, no experiment flag (GA when visible).

## 6. Test plan (five layers)

| Layer | What runs | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes asserting request paths (`/api/projects/1/query/` etc.), Bearer header injection, HogQL body wrapping, pagination param mapping, exit codes 0/1/2, `--json` error envelope, region-probe logic (fake us 401 → eu 200), `POSTHOG_API_HOST` override, BaseURL-disables-probe | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=phx_… anycli posthog -- whoami / project list / query run --project <id> --hogql "select event, count() from events group by event limit 5" / flag list / annotation create` against the **real** US cloud (the US→EU probe resolves the region; use an EU-region token to exercise the EU branch — `api_host` is no longer a credential binding) | **yes** — a personal API key (`phx_`) from the lane-2 test account, scoped to the §5.1 scope list (proves the wire scopes are the right ones) |
| L3 | local-only `go run ./cmd/provider-gen && … --check` against the branch bundle (projections NOT committed — batch lead owns the canonical regen); helio-cli built against this anycli worktree via a **local, uncommitted** `go.mod` `replace`; both repos' unit suites; new provider-gen tests for `form_public` | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `posthog`). Two passes: (a) seed the `phx_` personal key as `access_token`, no expiry — proves token-gateway→anycli→live-API on the real command path (`heliox tool posthog -- query run …`); (b) after §5.3 lands: mint a real `pha_`/`phr_` pair once via a scratch CIMD flow, seed with short `expires_at` to force the refresh-and-write-back path and verify the **rotated** refresh token is persisted (rotation makes a stale write-back a session-killing bug, so this pass is mandatory here, not optional polish) | **yes** — same personal key; plus one manual OAuth grant for pass (b) |
| L5 | Human lane 3, after batch-end merge, still hidden: `heliox tool posthog auth` → connect link → real PostHog consent (expect the "unverified application" notice) → `oauth_connected` event on the channel → unseeded live `query run`. Prereqs: CIMD doc deployed with this env's redirect_uri; `oauth.client_id` config landed; §5.3 shipped | **yes** — lane-2 PostHog account (US cloud), human completes consent |

Definition of done and the visible flip follow the master plan §2 (flip +
single regen as the go-live change; optionally get the app verified first to drop the
consent-screen notice).

## 7. Risks / open questions

1. **§5.3 sequencing**: the `form_public` extension must merge before this tool's
   L4(b)/L5. If the batch window closes first, the tool lands code-complete (hidden)
   and slips its flip — same shape as a stalled app registration in the plan.
2. **CIMD document hosting ownership**: which surface serves the static JSON
   (marketing site vs backend route) — lane 1 decision; must be HTTPS, stable forever
   (it IS our client identity), and carry a sane `Cache-Control`.
3. **EU-region coverage in L5**: the account pool is presumably US; the region probe
   is unit-tested for EU but a live EU account run is a nice-to-have follow-up.
4. **Scope list breadth**: 16 API scopes on one consent screen is legible but long;
   if product wants it shorter, drop `session_recording:read`/`experiment:read` first
   (commands degrade with a clean 403 passthrough).
5. **Self-hosted PostHog**: reachable today only via the harness/`POSTHOG_API_HOST`;
   a Helio-side connection story for self-hosted instances (api_key auth type variant)
   is explicitly out of scope for this bundle.
