# Salesloft — per-tool design (batch scratch file)

**Tool:** Salesloft · anycli id `salesloft` · provider key `salesloft` · auth lane `oauth_light` (verified, see §2)
**Branches:** anycli `tool/salesloft`, Helio `tool/salesloft`
**Status:** design for implementation on these branches; this file is stripped by the batch lead at batch end.

## 1. What an AI teammate does with Salesloft, and the API surface that serves it

Salesloft is a sales-engagement platform: sequenced outreach (cadences), prospect
(person) and account records, tasks, and a full activity feed (emails, calls,
notes). An AI teammate acting as an SDR/AE assistant needs to:

1. **Look up and maintain prospects/accounts** — find a person by email before
   emailing or enrolling them, create/update person and account records.
2. **Drive cadences** — list available cadences, enroll a person into a cadence
   (the single highest-value write in the product), check membership state.
3. **Manage tasks** — list due tasks for the rep, create follow-up tasks,
   update/complete them.
4. **Log and read activity** — add notes to a person/account, review recent
   emails/calls/activity history to brief the rep or decide next steps.

All of this is Salesloft **API v2** (base `https://api.salesloft.com/v2/`;
re-confirm the host at L2 — the doc pages render paths only). Auth is
`Authorization: Bearer <access_token>`; requests/responses are JSON with a
`data` payload key and `metadata` (paging/filter echo) on list endpoints;
errors are `error` (string, 403/404) or `errors` (field→[]string map, 422).
Pagination: `page` (1-based), `per_page` (default 25, max 100), metadata
returns `current_page`/`next_page`/`total_pages`/`total_count`. Sorting:
`sort_by` + `sort_direction` (ASC/DESC). Rate limit: 600 cost/minute per
**team** (shared across the team's integrations), headers
`x-ratelimit-endpoint-cost` / `x-ratelimit-remaining-minute`; deep pages
(>100) cost extra — the tool caps `--page` guidance in docs and prefers
`updated_at` filters.

### Wrapped endpoints (v1 scope)

| Verb | Endpoint | Why |
|---|---|---|
| `me` | `GET /v2/me` | identity, sanity check, and the bundle's identity probe |
| `user list` / `user get` | `GET /v2/users`, `GET /v2/users/{id}` | resolve teammates for `user_id` params |
| `person list` / `get` / `create` / `update` | `GET/POST /v2/people`, `GET/PUT /v2/people/{id}` | prospect lookup (filter by `email_addresses`, `updated_at`) and upkeep |
| `account list` / `get` / `create` / `update` | `GET/POST /v2/accounts`, `GET/PUT /v2/accounts/{id}` | account context and upkeep |
| `cadence list` / `get` | `GET /v2/cadences`, `GET /v2/cadences/{id}` | discover cadences to enroll into |
| `cadence add-person` | `POST /v2/cadence_memberships` (`person_id`, `cadence_id`, optional `user_id`) | the core write: enroll a prospect |
| `cadence memberships` | `GET /v2/cadence_memberships` (filter `person_id`/`cadence_id`) | check enrollment state |
| `task list` / `get` / `create` / `update` | `GET/POST /v2/tasks`, `GET/PUT /v2/tasks/{id}` | rep's task queue; create/complete follow-ups |
| `note list` / `note create` | `GET/POST /v2/notes` (`associated_with_type` Person/Account) | log qualitative context |
| `activity list` | `GET /v2/activity_histories` | unified recent-engagement feed |
| `email list` / `email get` | `GET /v2/activities/emails`, `.../{id}` | review outreach that happened |
| `call list` | `GET /v2/activities/calls` | review call activity |

Deliberately **out** of v1: deletes (destructive, rarely needed by a teammate,
and each adds a `*:delete` scope), bulk jobs, imports/exports, webhook
subscriptions, conversations/transcriptions, meetings/calendar (covered by the
calendar providers), and admin surfaces (custom roles, settings). Exact
request-body field lists for create/update verbs are taken from the per-endpoint
reference pages during implementation (the doc site renders schemas
client-side; the static fetches above confirmed method/path/required params
only — treat per-field shape as an implementation-time verification item
against the live reference + L2).

## 2. Auth lane verification (independent, against official docs)

Catalog row 65 says `oauth_light`. **Salesloft has no row in
`oauth-audit.md`** — the 2026-07-21 audit scoped only tools that sat in the
`api_key` lane pre-audit, and Salesloft was `oauth_light` from the start. So
the lane was verified directly against the official docs
(`developers.salesloft.com/docs/platform/api-basics/oauth-authentication/`):

- **Registration is self-serve**: accounts.salesloft.com → Your Applications →
  OAuth Applications → Create New. Client id ("Application Id"), secret, and an
  Integration Id are issued immediately. No review gate is documented for app
  creation or for arbitrary external teams to authorize; review exists only
  for the optional partner program listing. Salesloft explicitly steers
  integrations to OAuth ("Partner Applications submitted using API Keys will
  not be approved").
- **Flow is standard multi-tenant authorization-code**: one registered app,
  any Salesloft user on any team can authorize.

**Verdict: `oauth_light` confirmed.** No divergence from the catalog.

### Flow parameters (from official docs)

- Authorize: `GET https://accounts.salesloft.com/oauth/authorize`
  with `client_id`, `redirect_uri`, `response_type=code`. No `scope`
  parameter is documented on the authorize request — **scopes are selected on
  the app at registration time** (e.g. `people:read`, `cadences:write`
  tokens; granted set echoed back in the token response `scope` field, with a
  separate "Privileged Scopes" tier we don't need).
- Token: `POST https://accounts.salesloft.com/oauth/token` with
  `client_id`, `client_secret`, `code`, `grant_type=authorization_code`,
  `redirect_uri`. Response: `access_token`, `token_type: bearer`,
  `expires_in: 7200` (2 h), `refresh_token`, `scope`, `created_at`.
- Refresh: same URL, `grant_type=refresh_token` + client id/secret +
  refresh token. **Refresh tokens rotate: "upon receipt of a refresh token,
  all old refresh tokens are revoked"** — the new refresh token must be
  persisted on every refresh. No documented refresh-token expiry.
- **PKCE:** not documented (no `code_challenge` params anywhere) → `pkce: none`.
- **Client auth style:** docs show credentials **in the request body** (their
  examples use a JSON body; the `invalid_grant` troubleshooting explicitly
  warns that parameters belong in the body, *not* headers — i.e. Basic auth is
  the wrong shape here). The endpoint is a standard Doorkeeper-style provider
  (`created_at` in the token response), which accepts RFC 6749 form-encoded
  token requests, so the bundle uses **`token_exchange_style: form_secret`**
  (form body + secret in body). Risk noted in §6: if the live endpoint turns
  out to strictly require a JSON body with in-body credentials, that
  combination (json + body secret) is outside the current
  `form_secret|form_basic|json_basic` enum — the fix would be one new reviewed
  enum value in provider-gen/standard_oauth, **not** a per-provider adapter.
  Verified at first L4 token-gateway refresh and at L5.
- **Revocation:** no official revoke endpoint documented →
  `disconnect_mode: local_only` (Notion/Microsoft precedent).

### Scopes requested at app registration (lane 1 input)

`me:read` (if listed), `users:read`, `people:read`, `people:write`,
`accounts:read`, `accounts:write`, `cadences:read`,
`cadence_memberships:read`, `cadence_memberships:write`, `tasks:read`,
`tasks:write`, `notes:read`, `notes:write`, `activities:read`,
`emails:read`, `calls:read`.

Exact scope-token spellings are whatever the app-creation screen offers
(the docs list the `<resource>:<read|write|delete>` convention but the full
menu is only visible in-product) — lane 1 records the actual granted set at
registration and it lands in the bundle's `display_scopes`. No `*:delete`
and no Privileged Scopes.

## 3. anycli definition

**Stage-1 rubric: `service` type.** Salesloft ships no official CLI binary at
all, so the `cli`-type conditions fail at the first test. Service
implementation against API v2.

`definitions/tools/salesloft.json`:

```json
{
  "name": "salesloft",
  "type": "service",
  "description": "Salesloft sales engagement (people, cadences, tasks, activity)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SALESLOFT_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Implementation `internal/tools/salesloft/` (id has no dash; package name =
id), registered in `internal/tools/register.go` `init()` as
`RegisterService("salesloft", &salesloft.Service{})` — **registration rides
the batch-end merge; the definition JSON and package merge freely**.

**Shape: copy `internal/tools/notion/`** (the reference service): a cobra tree
grouped by resource with a `BaseURL`/`HC`/`Out`/`Err` struct so tests point at
httptest; exit codes 0 (success), 1 (runtime/API failure via typed `apiError`),
2 (usage/parse); `--json` structured error envelope.

Command tree (§1 verbs): `me`, `user list|get`, `person list|get|create|update`,
`account list|get|create|update`, `cadence list|get|add-person|memberships`,
`task list|get|create|update`, `note list|create`, `activity list`,
`email list|get`, `call list`.

**JSON output shape:** pass through Salesloft's envelope — `data` object/array
plus, for lists, `metadata.paging` (`current_page`, `next_page`,
`total_pages`, `total_count` when present) so the agent can page without a
second vocabulary. Shared list flags: `--page`, `--per-page` (≤100),
`--updated-since` (maps to the `updated_at` gte filter — the
rate-limit-friendly incremental pattern Salesloft recommends over deep
paging), `--sort-by`, `--sort-direction`. Resource filters as documented per
endpoint (e.g. `person list --email <addr>` → `email_addresses[]`,
`cadence memberships --person-id/--cadence-id`). API 422s render the
`errors` field map verbatim inside the error envelope; 403/404 render the
`error` string. Rate-limit hits surface `x-ratelimit-remaining-minute` in the
error message.

**TDD:** every subcommand gets an httptest-fake test asserting method, path,
query/body shape, `Authorization: Bearer` injection from
`SALESLOFT_ACCESS_TOKEN`, and both plain and `--json` error rendering.
Never hit the real API from unit tests.

## 4. Helio provider bundle plan

**Naming axes:** ① CLI command word `salesloft` (flat command, no group; no
`tool.command` needed) · ② anycli id `salesloft` · ③ provider key
`salesloft`. All three identical → **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go`.

`integrations/providers/salesloft/provider.yaml` (hidden-first; held to the
batch-end merge with the single provider-gen run):

```yaml
schema: helio.provider/v1
key: salesloft
go_name: Salesloft

presentation:
  name: Salesloft
  description_key: salesloft
  consent_domain: salesloft.com
  visible: false          # hidden-first; flip is the single go-live change
  order: <batch-lead assigned>

auth:
  type: oauth
  owner: individual        # per-user Salesloft seat (Google/Microsoft precedent)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://accounts.salesloft.com/oauth/authorize
    token_url: https://accounts.salesloft.com/oauth/token
    token_exchange_style: form_secret   # see §2 risk note
    pkce: none
    authorize_params: {}
    # No wire-level scope parameter on the authorize request — scopes are
    # fixed on the registered app (bitly/notion display-only precedent).
    # Final slugs = the scope set lane 1 actually grants at registration (§2).
    display_scopes: [people:read, people:write, accounts:read, accounts:write,
      cadences:read, cadence_memberships:write, tasks:read, tasks:write,
      notes:read, notes:write, activities:read, emails:read, calls:read]
    single_active_token: false
    refresh_lease: none    # fixed by the standard_oauth runtime contract

identity:
  source: userinfo
  url: https://api.salesloft.com/v2/me
  stable_key: /data/guid           # /v2/me wraps the user in a data envelope
  label_candidates: [/data/email, /data/guid]

connection:
  mode: isolated
  disconnect_mode: local_only      # no documented revoke endpoint
  runtime_strategy: standard_oauth # zero service-side Go

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: salesloft
  kind: oauth
```

Notes:

- **`standard_oauth`, no adapter.** Response shapes are standard; the only
  non-vanilla trait (rotating refresh tokens) is a token-gateway write-back
  concern, not a bundle capability gap.
- **Identity pointers** (`/data/guid`, `/data/email`) assume `/v2/me` returns
  the standard `{"data": {...}}` envelope with `guid`/`email` fields —
  verified with a real token at L2 before the bundle lands (the static doc
  page doesn't render the schema).
- **Refresh-rotation write-back:** the seeded short-expiry L4 run (§5) must
  show a *second* successful refresh after the first, proving the gateway
  persisted the rotated refresh token — with Salesloft, replaying a stale
  refresh token fails hard because rotation revokes old tokens.
- **Config:** lane 1 lands `oauth.client_id`/`oauth.client_secret` for
  `salesloft` in both `config/` and the Helm Secret under `deploy/` together
  (Config Sync rule), before this provider's L5. Dev credentials arrive as
  uncommitted local `config/cloud.yaml` entries for on-branch L4.
- **Icon:** `ui/helio-app/src/integrations/icons/salesloft.svg` + manual
  `providerIcons.ts` registration (rides batch-end merge).
- **Docs:** provider sub-doc under `agents/plugins/heliox/skills/tool/`
  (one plugin bump/publish per batch).
- No `experiment` gate (not a preview tool).

## 5. Test plan — five layers

| Layer | What runs here | External credentials needed |
|---|---|---|
| **L1** | anycli `go test ./...`: per-subcommand httptest fakes (request shape, Bearer injection, envelope passthrough, 403/404 `error` vs 422 `errors` rendering, exit codes, `--json` envelope) | None |
| **L2** | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real token> anycli salesloft -- me`, then `person list --per-page 3`, `cadence list`, one write (`note create` on a sandbox person), verifying live request/response shape, the `/v2/me` data-envelope fields the identity block points at, and the base host. Token minted from the lane-1 dev app's "Test Authorization" button (dashboard) + manual code exchange | **Yes** — Salesloft test account (lane 2) + registered dev app (lane 1); OAuth-only, no static API key path for our shape |
| **L3** | Local (uncommitted) `provider-gen` + `provider-gen --check` against the branch bundle; helio-cli build/tests with a local uncommitted `go.mod` `replace` pointing at the anycli worktree; integration-service unit suite | None |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with real `access_token` + `refresh_token` and a **deliberately short `expires_at`** → `heliox tool salesloft -- me` forces the gateway refresh path; run twice to prove rotated-refresh-token write-back (§4). Dev client id/secret in local uncommitted `config/cloud.yaml` | **Yes** — real token pair from the dev app + test account |
| **L5** | Per-batch human sweep (oauth lane): `heliox tool salesloft auth` → connect link → real Salesloft consent → `oauth_connected` event → one unseeded live command. Requires lane 1's committed config append to have landed | **Yes** — human consent on the pooled test account |

Definition of done per master plan §2: L1–L5 green, docs published, icon
registered, then the visible flip (`visible: true` + regen) as its own change.

## 6. Risks / open verification items

1. **Token-exchange style** (§2): `form_secret` chosen on RFC/Doorkeeper
   grounds; official examples show a JSON body. Verify at the first real
   exchange (L4 refresh / L5 authorize). Contingency: add a reviewed
   `json_secret`-style enum value to provider-gen + `standardOAuthExchanger`
   (generic capability growth, per the skill's guidance), not an adapter.
2. **Refresh rotation vs concurrent runtimes**: rotation revokes old refresh
   tokens; if two runtimes race a refresh under `refresh_lease: none`, the
   loser's token dies. The gateway's per-credential serialization is assumed
   sufficient (Google-precedent path); the double-refresh L4 check covers the
   sequential case. Escalate to the batch lead if racing is observed — that is
   a gateway-level fix, not per-provider.
3. **Scope-token spellings and `/v2/me` schema** are only fully visible
   in-product/at runtime (docs render client-side): captured at lane-1
   registration and L2 respectively; `display_scopes` and identity pointers
   finalized then.
4. **Team-shared rate limit** (600 cost/min per team): a busy customer team's
   other integrations share the budget; the tool's docs steer agents to
   `--updated-since` polling over deep pagination.
5. **Redirect URI**: registration requires a non-Salesloft redirect URI —
   lane 1 registers the standard Helio callback used by every
   `standard_oauth` provider.
