# Tool design: Calendly

Catalog row: #41 — Product **Calendly**, anycli id `calendly`, provider key
`calendly`, auth lane `oauth_light`, wave 1, category Scheduling & eSign
(`docs/design/008-300-integrations-rollout-plan.md`).

Scratch per-tool design on branch `tool/calendly`; the batch lead strips this
file at batch end.

## 1. What an AI teammate does with Calendly

The assistant acts as a scheduling aide for its human: answer "when am I
free / what's on my Calendly", share the right booking link (including
single-use links), look up who booked and their answers, cancel a meeting
with a reason, mark no-shows, and — where the account's plan allows — book a
slot directly on an invitee's behalf. Everything is read/act on the user's
existing Calendly setup; the tool does not manage event-type configuration,
webhooks, or org administration.

## 2. Official API surface (verified against developer.calendly.com)

Base URL `https://api.calendly.com`, `Authorization: Bearer <token>`,
JSON, cursor pagination (`pagination.next_page` / `page_token`). Resources
are identified by full URIs (e.g. `https://api.calendly.com/users/XXXX`),
not bare ids.

Endpoints the tool wraps, and why:

| Endpoint | Why |
|---|---|
| `GET /users/me` | whoami: user URI, org URI, `scheduling_url`, timezone — the URI bootstrap every other call needs |
| `GET /event_types?user=…\|organization=…`, `GET /event_types/{uuid}` | discover bookable meeting kinds + their `scheduling_url` to share |
| `GET /event_type_available_times?event_type=…&start_time=…&end_time=…` | open slots for an event type (range ≤ 31 days, must be future) |
| `GET /user_busy_times?user=…&start_time=…&end_time=…` | calendar busy view (range ≤ 7 days) |
| `GET /user_availability_schedules?user=…` | working-hours schedules + date overrides |
| `GET /scheduled_events?user=…\|organization=…` (+`min_start_time`/`max_start_time`/`status`/`invitee_email`), `GET /scheduled_events/{uuid}` | list/inspect booked meetings |
| `GET /scheduled_events/{uuid}/invitees`, `GET …/invitees/{uuid}` | who booked, Q&A answers, `cancel_url`/`reschedule_url` |
| `POST /scheduled_events/{uuid}/cancellation` | cancel with reason |
| `POST /invitee_no_shows`, `DELETE /invitee_no_shows/{uuid}` | mark/unmark no-show |
| `POST /scheduling_links` | mint a single-use booking link (`max_event_count: 1`, `owner_type: EventType`) |
| `POST /invitees` (Scheduling API, 2026) | direct booking: `event_type`, UTC `start_time`, invitee `{name,email,timezone}`, `location` rules; **requires the Calendly account to be on a paid plan** — surface the 403 clearly, do not hide it |
| `GET /organization_memberships?organization=…` | resolve teammates' user URIs (needed for availability/busy on colleagues) |

Deliberately out of scope for v1: webhook subscription CRUD (Helio has no
per-connection receiver in this tool model), event-type/availability writes,
routing forms, contacts, groups, activity log, data-compliance deletes.
There is **no reschedule endpoint** — reschedule = share the invitee's
`reschedule_url` (or cancel + new link); the tool docs must say this.

## 3. Auth: lane verification (oauth_light — CONFIRMED, with nuances)

Verified on official pages (`/creating-an-oauth-app`, `/authentication`,
`/scopes`, `/refresh-token-rotation-guide`, `/getting-started`):

- Developer account is self-serve (GitHub/Google sign-in, separate from the
  Calendly user account). OAuth app creation is immediate — name, kind
  (web/native), environment (**Sandbox** or **Production**), redirect URI,
  scopes; client id/secret issued at creation with **no review step**. The
  audit verdict (`oauth-audit.md` row 41: yes / oauth_light / high) matches
  the official docs. No divergence.
- **Authorize**: `https://auth.calendly.com/oauth/authorize` with
  `client_id`, `response_type=code`, `redirect_uri`, and PKCE
  `code_challenge_method=S256` + `code_challenge` (OAuth 2.1; directed for
  all apps, not enforced for web apps).
- **Token**: `POST https://auth.calendly.com/oauth/token`,
  `application/x-www-form-urlencoded`. Client auth via HTTP Basic
  (`client_id:client_secret`) is the documented/staff-demonstrated method →
  Helio `token_exchange_style: form_basic`. Response: `access_token`,
  `refresh_token`, `expires_in`, `scope`, plus `owner`/`organization` URIs
  (extra fields are harmless to the standard exchanger).
- **Access-token lifetime**: the official OAuth guide
  (`/how-to-access-calendly-data-on-behalf-of-authenticated-users`) documents a
  **2-hour** access-token lifetime (refresh tokens "don't expire until they are
  used"). Operative rule is unchanged: treat `expires_in` from each token
  response as authoritative — do not hard-code the 2h; the documented figure is
  orientation only.
- **Refresh rotation (the critical nuance)**: refresh tokens are
  **single-use** and rotate on every `grant_type=refresh_token` call;
  Calendly enforces this for all integrations by **August 31, 2026**. Reuse
  of a spent token → `invalid_grant` (400/401), unrecoverable without
  re-authorization. Helio's 227 A3 refresh path already persists the rotated
  refresh token with strict write-back (`token_refresh.go`: write-back
  failure returns an error rather than handing out an unpersisted token), and
  `refresh_lease: credential` serializes concurrent refreshes per credential
  so parallel `heliox tool` calls cannot burn the token — both are required
  for Calendly, not optional.
- **Scopes are wire-level AND app-level** (verified on `/scopes`): the official
  Scopes page documents a **space-separated `scope` param on the authorize
  request** (example: `…/oauth/authorize?…&scope=scheduled_events:read
  webhooks:write`), directs apps to "request the minimum set of scopes needed,"
  and — on the granular-scopes model — states newly created apps get **no API
  access until scopes are explicitly requested and approved**. So the bundle
  **sends `scopes:` on the wire** (google_calendar precedent — the generator
  supports wire scopes) *and* lane 1 ticks the identical set at app
  registration; `display_scopes` is retained only for consent-page disclosure.
  Omitting wire scopes risks a zero-scope token where even identity resolution
  via `/users/me` (needs `users:read`) 403s and L5 fails. Scope set (write
  scopes include their read twin): `users:read`, `event_types:read`,
  `availability:read`, `scheduled_events:write`, `scheduling_links:write`,
  `organizations:read`.
- Personal Access Tokens exist (also scoped for new tokens); irrelevant to
  the connect flow but ideal for L2.

Lane-1 registration notes: sandbox app allows `http://localhost` redirect
URIs; production app requires HTTPS redirect. Client secret is shown once at
creation — captured immediately into the uncommitted local
`config/cloud.yaml` for L4 and the config/deploy landing for L5.

## 4. anycli definition (stage 1–2)

**`service` type** — stage-1 rubric: Calendly ships no official CLI, so the
`cli` conditions fail; implement `internal/tools/calendly/` against the REST
API (matches 21/23 precedent).

`definitions/tools/calendly.json`:

```json
{
  "name": "calendly",
  "type": "service",
  "description": "Calendly as a tool (OAuth 2.0 access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "CALENDLY_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Package `internal/tools/calendly` (id has no dashes; package == id),
registered in `internal/tools/register.go` `init()` (batch-end shared
surface). Shape copied from `internal/tools/bitly` / `notion`: `Service`
struct with `BaseURL`/`HC`/`Out`/`Err` for httptest injection; Bearer auth;
exit codes 0 success / 1 runtime-API failure / 2 usage errors; raw provider
JSON passthrough on stdout; `--json` structured error envelope; 401 marks
the credential rejected.

Calendly-specific helper: flags accept either a bare UUID or a full resource
URI; a normalizer expands UUIDs to the canonical
`https://api.calendly.com/<collection>/<uuid>` form the API requires. `me`
is accepted wherever a user URI is expected (resolved via one cached
`GET /users/me` per invocation).

Cobra tree (`heliox tool calendly -- …`):

- `me` — current user (+ org URI, scheduling_url)
- `event-type list [--user me|<uri>] [--org]` / `event-type get <id>`
- `availability slots --event-type <id> --from <ts> --to <ts>` (≤31 days)
- `availability busy [--user me|<uri>] --from <ts> --to <ts>` (≤7 days)
- `availability schedule list [--user me|<uri>]`
- `event list [--user me|<uri>] [--org] [--status active|canceled] [--invitee-email e] [--from ts] [--to ts]`
- `event get <id>` / `event invitees <id>`
- `event cancel <id> [--reason text]`
- `invitee no-show <invitee-id>` / `invitee no-show <no-show-id> --undo`
- `link create --event-type <id>` (single-use scheduling link)
- `book create --event-type <id> --start <ts> --name n --email e --timezone tz [--location-kind k] [--location v] [--guest e]...` (Scheduling API; document the paid-plan 403)
- `org members [--email filter]`
- `--page-token` / `--count` passthrough on list verbs (cursor pagination)

## 5. Helio provider bundle (stage 4)

Axes (§3 master plan): ① CLI command `calendly` (flat, no group), ② anycli
id `calendly`, ③ provider key `calendly` — all identical, so **no
`toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.

`integrations/providers/calendly/provider.yaml` (hidden-first):

```yaml
schema: helio.provider/v1
key: calendly
go_name: Calendly

presentation:
  name: Calendly
  description_key: calendly
  consent_domain: calendly.com
  visible: false          # hidden-first; flip is the go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual       # the provider sees a person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://auth.calendly.com/oauth/authorize
    token_url: https://auth.calendly.com/oauth/token
    token_exchange_style: form_basic   # documented Basic client auth on token endpoint
    pkce: s256                         # OAuth 2.1; Calendly directs S256 for all apps
    authorize_params: {}
    # Official /scopes documents a space-separated wire `scope` param and a
    # granular-scopes model (new apps have zero API access until scopes are
    # requested/approved). Send scopes on the wire (google_calendar precedent)
    # AND tick the identical set at app registration; display_scopes is
    # consent-page disclosure only.
    scopes: [users:read, event_types:read, availability:read,
             scheduled_events:write, scheduling_links:write, organizations:read]
    display_scopes: [users:read, event_types:read, availability:read,
                     scheduled_events:write, scheduling_links:write,
                     organizations:read]
    single_active_token: false
    refresh_lease: credential          # single-use rotating refresh tokens (enforced 2026-08-31)
    revoke:
      url: https://auth.calendly.com/oauth/revoke   # officially documented (Revoke Access/Refresh Token)
      client_auth: form                # revoke body carries client_id + client_secret
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://api.calendly.com/users/me
  stable_key: /resource/uri
  label_candidates: [/resource/name, /resource/email, /resource/slug]

connection:
  mode: isolated
  disconnect_mode: provider_revoke     # Calendly documents OAuth token revoke (see auth.oauth.revoke)
  runtime_strategy: standard_oauth     # no adapter: standard token JSON + userinfo identity

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: calendly
  kind: oauth
```

No service-side adapter: token response and identity are fully standard.
Other Helio artifacts (all batch-end shared surfaces except the yaml itself,
which also rides batch end): icon `ui/helio-app/src/integrations/icons/calendly.svg`
+ `providerIcons.ts` entry; AI docs `agents/plugins/heliox/skills/tool/calendly.md`
(must cover URI-vs-UUID, the no-reschedule-endpoint rule, the paid-plan gate
on `book create`, and range limits) + plugin bump/publish; client id/secret
appended by lane 1 to integration-service config in `config/` + `deploy/`
Helm Secret together. `provider-gen` is run **locally only for validation**
on this branch (projections not committed, per master plan §2); L4 builds
helio-cli with a local, uncommitted `go.mod` `replace` pointing at this
anycli worktree.

## 6. Test plan (five layers)

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes asserting Bearer header, request paths/query (URI expansion, `me` resolution, range params), pagination passthrough, cancel/no-show/link/book request bodies, 401-rejected mapping, plain + `--json` error rendering | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli calendly -- me` then each verb family against the live API | **yes** — a Calendly **Personal Access Token** from the lane-2 test account, created with the §3 scope set (new PATs are scoped; an unscoped PAT has no access). PAT suffices because anycli only sees a bearer token |
| L3 | local `go run ./cmd/provider-gen` + `--check` against this branch's bundle; helio-cli + integration-service unit suites with the local `replace` | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` with `provider: "calendly"`, real `access_token` **and** `refresh_token` from the lane-1 **sandbox app**, deliberately short `expires_at` → forces the A3 refresh; then `heliox tool calendly -- me`. Run the tool **twice**: the second call proves the rotated (single-use) refresh token was persisted — this is the Calendly-specific L4 assertion, not optional | **yes** — lane-1 sandbox app client id/secret (uncommitted local `config/cloud.yaml`) + a token pair minted from it on the lane-2 test account |
| L5 | `heliox tool calendly auth` → connect link → real consent on the test account → `oauth_connected` event → unseeded `heliox tool calendly -- me` and one write (e.g. `link create`); confirm the consent screen shows the §3 scope set | **yes** — human-in-the-loop (lane 3), production-app config landed in `config/` + `deploy/` |

## 7. Open questions / divergences

1. **No lane divergence**: official docs confirm `oauth_light` (self-serve
   registration, immediate credentials, no review). Nothing to escalate.
2. **`book create` plan gate**: the Scheduling API requires the connected
   Calendly account to be paid. Keep the verb, surface Calendly's 403
   verbatim, and document it — do not silently degrade (no-silent-fallback
   rule). If the lane-2 test account is free-tier, L2/L4 for this one verb
   are consent-blocked and it is exercised in L5 or with a paid test
   account.
3. **Range limits — re-verified, not drifting**: the current official guides
   (`/schedule-events-with-ai-agents`: "can retrieve up to 31 days of available
   times per request"; `/view-event-type-and-user-calendar-availability-data`:
   `event_type_available_times` "cannot be a range greater than 31 days",
   `user_busy_times` "cannot be a range greater than 7 days") both confirm
   **≤31 days for `event_type_available_times`** and **≤7 days for
   `user_busy_times`**. A review pass claimed both endpoints share a 7-day cap;
   that is **not** what the official pages say and is rejected here. §2/§4 keep
   31/7. The tool enforces nothing client-side beyond passing params through;
   let the API's own validation error surface, and re-confirm the live caps at
   L2. Recorded as a divergence-from-review per the "official docs win" rule.

### Resolved during review revision

- **Scopes (was OQ "wire `scope` param")**: `/scopes` documents a wire-level
  space-separated `scope` param and a granular-scopes "no access until
  approved" model, so scopes ship **on the wire** (`auth.oauth.scopes`) plus
  the same set at app registration — see §3/§5. No longer open.
- **Disconnect revoke (was OQ "disconnect revoke")**: Calendly officially
  documents Revoke Access/Refresh Token
  (`POST https://auth.calendly.com/oauth/revoke`, `client_id`/`client_secret`/
  `token`), so v1 ships `disconnect_mode: provider_revoke` with a declarative
  `revoke:` block (`client_auth: form`) — see §5. No longer open.
