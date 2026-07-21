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
| `GET /event_type_available_times?event_type=…&start_time=…&end_time=…` | open slots for an event type — range ≤ ~1 week per the API reference, must be future (see the range-cap note below) |
| `GET /user_busy_times?user=…&start_time=…&end_time=…` | calendar busy view (range ≤ 7 days) |
| `GET /user_availability_schedules?user=…` | working-hours schedules + date overrides |
| `GET /scheduled_events?user=…\|organization=…` (+`min_start_time`/`max_start_time`/`status`/`invitee_email`), `GET /scheduled_events/{uuid}` | list/inspect booked meetings |
| `GET /scheduled_events/{uuid}/invitees`, `GET …/invitees/{uuid}` | who booked, Q&A answers, `cancel_url`/`reschedule_url` |
| `POST /scheduled_events/{uuid}/cancellation` | cancel with reason |
| `POST /invitee_no_shows`, `DELETE /invitee_no_shows/{uuid}` | mark/unmark no-show |
| `POST /scheduling_links` | mint a single-use booking link (`max_event_count: 1`, `owner_type: EventType`) |
| `POST /invitees` (Scheduling API, 2026) | direct booking: `event_type`, UTC `start_time`, invitee `{name,email,timezone}`, `location` rules; **requires the Calendly account to be on a paid plan** — surface the 403 clearly, do not hide it |
| `GET /organization_memberships?organization=…` | resolve teammates' user URIs (needed for availability/busy on colleagues) |

**Required OAuth scope per endpoint** (transcribed verbatim from the official
Scope Catalog's "Provides access to" mapping — the scope set in §3/§5 is
*derived* from this, not from a read/write-twin shortcut): `GET /users/me` →
`users:read`; `GET /event_types*` **and `GET /event_type_available_times`** →
`event_types:read` (the catalog lists `event_type_available_times` under
`event_types:read`, **not** `availability:read`); `GET /user_busy_times` and
`GET /user_availability_schedules` → `availability:read`;
`GET /scheduled_events*`, `GET /scheduled_events/{uuid}/invitees`, and
`…/invitees/{uuid}` → `scheduled_events:read` — **the catalog maps invitee reads
under `scheduled_events:read`, and the `scheduled_events:write` we request
implicitly includes `scheduled_events:read` via the catalog's `:write` ⊇ `:read`
hierarchy rule, so invitee reads need no separate scope** (there is no
`invitees:read` catalog scope — see §3/§7 #5); `POST …/cancellation`,
`POST`/`DELETE` no-show, and Scheduling-API `POST /invitees` →
`scheduled_events:write` (**the catalog explicitly enumerates `POST /invitees`
under `scheduled_events:write`** — this booking mapping is catalog-verified, not
assumed); `POST /scheduling_links` → `scheduling_links:write`;
`GET /organization_memberships` → `organizations:read`.

**Range-cap note (Calendly's own docs are internally inconsistent — verified).**
For `event_type_available_times` the current *guide* pages say 31 days
(`/schedule-events-with-ai-agents`: "up to 31 days of available times per
request"; `/view-event-type-and-user-calendar-availability-data`: "cannot be a
range greater than 31 days"), but the **API reference** (Stoplight
`api-docs` "List Event Type Available Times") and Calendly support/community
consistently state the span **"can be no greater than 1 week"** (7 days). The
guide and the reference contradict each other; the reference is the endpoint
contract, so the tool and docs treat **~7 days as the effective cap** for this
endpoint and let the live API's own validation be authoritative. `user_busy_times`
is unambiguously ≤ 7 days. The tool enforces **nothing** client-side beyond
passing params through — it never rejects a range locally — but the AI-facing
doc (§5) tells the assistant to chunk into ≤ ~1-week windows so it does not emit
31-day ranges that the API rejects. L2 records the **observed live cap for both
endpoints** (see §6).

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
  re-authorization. **This does not require any Calendly-specific service code
  and does not change the `standard_oauth` bundle shape** — the shared refresh
  path already handles single-use rotation correctly for a
  `refresh_lease: none` provider, which is what Calendly ships (see the
  concurrency analysis below).

  **Why `refresh_lease: none` is correct here (and matches the whole
  rotating-refresh class).** Two facts settle it:
  - **A3 strict write-back** (`service/token_refresh.go:60-66`): a successful
    refresh is not returned to a caller until the rotated pair is persisted;
    write-back failure returns an error rather than handing out an
    unpersisted token. So after any *successful* refresh, the persisted pair
    is always the rotated one — a losing refresh can never overwrite a good
    token with a stale one it failed to persist.
  - **Transient/permanent classification** (`service/token_refresh.go:165`):
    `invalid_grant` is classified permanent → surfaced as "reconnect
    required"; anything else is transient → retry.

  Put together, the *only* residual failure under `refresh_lease: none` is a
  genuine **concurrent** refresh: two parallel `heliox tool` calls both read
  the same live refresh token, both POST it, one wins (rotates + persists via
  A3), and the **loser POSTs the now-spent token → `invalid_grant` → one
  spurious "reconnect required"** for that single call. This is **not a
  permanent brick**: the winner's rotated token is persisted and valid, so the
  connection self-heals (or, at worst, the user re-connects once). A true
  unrecoverable brick would require Calendly to additionally perform
  OAuth-2.1/BCP (RFC 9700) **refresh-token-reuse detection with family
  revocation** — revoking the *winner's* freshly issued token when it sees the
  spent one replayed. That behavior is plausible given Calendly's 2026
  single-use enforcement, but **it is not stated or cited in Calendly's
  official docs**, so this design does not assert it and does not size a
  service change against it.

  This residual concurrent-refresh risk is **identical for every rotating-refresh
  provider in this program** — Pennylane (audit row 184, "refresh-token
  rotation"), Airtable (row 13, refresh tokens + PKCE), PandaDoc (row 44),
  Google/Notion, etc. — and all of them ship `standard_oauth` with
  `refresh_lease: none`. `refresh_lease: none` is the **program default for
  the whole class**, not a "durable-token" special case; Calendly matches its
  siblings rather than becoming a lone exception. A per-credential refresh
  *serialization* lease (`OAuthLeaseCredential`) *would* close the
  concurrent-race window for the entire rotating class, but that is a
  program-wide capability decision the batch lead / master plan owns — **not a
  change a single tool branch should make to the shared `standard_oauth`
  runtime contract** (which governs ~140 providers and which master-plan §5
  pins to "zero service code"). It is recorded as a master-plan open question
  in §7, not shipped here.
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
  via `/users/me` (needs `users:read`) 403s and L5 fails.
  **Scope set — derived per-endpoint from the official Scope Catalog's
  "Provides access to" mapping, not a read/write-twin heuristic:**
  `users:read` (`GET /users/me`), `event_types:read` (`GET /event_types*` **and
  `GET /event_type_available_times`** — the catalog lists available-times under
  `event_types:read`, not `availability:read`), `availability:read`
  (`GET /user_busy_times` / `GET /user_availability_schedules` — **confirmed a
  real catalog scope, not assumed**), `scheduled_events:write` (`POST
  …/cancellation`, the `POST`/`DELETE` no-show writes, and the Scheduling-API
  `POST /invitees` — **the catalog explicitly enumerates `POST /invitees` under
  `scheduled_events:write`**; and the catalog's documented hierarchy rule — "a
  `:write` scope implicitly includes the corresponding `:read` scope within the
  same domain" — makes this also cover the read-only `GET /scheduled_events*`
  list/get **and the invitee reads `GET /scheduled_events/{uuid}/invitees` /
  `…/invitees/{uuid}`, which the catalog maps under `scheduled_events:read`**),
  `scheduling_links:write` (`POST /scheduling_links`), `organizations:read`
  (`GET /organization_memberships`). **Six scopes, no `invitees:read`** — see
  the note next.
- **No `invitees:read` scope — the catalog is authoritative and it is not a
  valid scope string (see §7 #5).** Calendly's `/scopes` page is internally
  inconsistent: its **"Choosing Scopes" worked example** names `invitees:read`
  ("Required to associate meetings with people and contact details"), but the
  page's **Scope Catalog** — the authoritative list of valid scope *strings* —
  does **not** contain `invitees:read` at all, and instead maps
  `GET /scheduled_events/{uuid}/invitees` and `…/invitees/{uuid}` under
  `scheduled_events:read`. This is decisive on two counts. **(1) Coverage:** the
  `scheduled_events:write` we already request implicitly includes
  `scheduled_events:read` (hierarchy rule), which the catalog says *is* the
  invitee-read scope — so invitee reads are fully covered without a separate
  scope. **(2) Wire safety:** these scopes ship on the wire (§3 above), and
  Calendly's authorize endpoint **strictly validates the `scope` param and
  rejects any unknown/unlisted string** with a hard "The requested scope is
  invalid, unknown, or malformed" error (the documented `openid` rejection is
  the precedent). Because `invitees:read` is not a catalog string, sending it
  would not silently no-op — it would **break the entire `/oauth/authorize`
  request and brick the connect flow for every account**, including the tool's
  headline `event invitees` capability. So v1 **excludes** `invitees:read`
  entirely. L2 (which uses a bearer PAT) cannot exercise the wire `scope`-param
  validation and so structurally cannot settle wire acceptance; the invitee
  verbs are instead confirmed at L2 by asserting they return 2xx under the
  requested six-scope set (`scheduled_events:write` ⊇ `scheduled_events:read`),
  and the wire-acceptance of the whole set is confirmed at L5's real consent.
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
- `availability slots --event-type <id> --from <ts> --to <ts>` (chunk to ≤ ~1 week; no client-side enforcement — the API validates)
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
    # granular-scopes "no access until approved" model. Send scopes on the wire
    # (google_calendar precedent) AND tick the identical set at app
    # registration; display_scopes is consent-page disclosure only.
    scopes: [users:read, event_types:read, availability:read,
             scheduled_events:write, scheduling_links:write,
             organizations:read]
    display_scopes: [users:read, event_types:read, availability:read,
                     scheduled_events:write, scheduling_links:write,
                     organizations:read]
    single_active_token: false
    refresh_lease: none                # program default for standard_oauth, incl. the rotating-refresh class (Pennylane/Airtable/PandaDoc); single-use rotation is handled by A3 strict write-back — see §3
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
  runtime_strategy: standard_oauth     # generic exchanger + userinfo identity; no provider adapter, no service-code change

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

**Zero service-side code — Calendly is a pure `standard_oauth` bundle.**
The generic exchanger, RFC 6901 declarative identity, and declarative revoke
cover it end to end: no provider adapter (`adapter_*.go`), no exchange/identity
dialect, **and no edit to the shared `standard_oauth` runtime contract**. The
contract (`go-services/integration-service/model/runtime_contract.go:37-42`)
pins `refreshLeaseScope: OAuthLeaseNone` for `standard_oauth`, and Calendly
declares exactly that — so `provider-gen --check` passes as-is. Single-use
refresh-token rotation needs no new tuple: the shared refresh path's A3 strict
write-back (`service/token_refresh.go:60-66`) already persists the rotated pair
before returning it, and `invalid_grant` already classifies as
"reconnect required" (`:165`). This keeps Calendly on the master-plan §5
invariant — *"`standard_oauth` bundles need zero service code"* — exactly like
its rotating-refresh siblings (Pennylane, Airtable, PandaDoc). The
per-credential refresh lease that would harden the whole rotating class against
the concurrent-race window is deliberately **not** selected here; it is
escalated to the master plan as an open question (§7) so the batch lead owns
the shared-surface decision program-wide.

Other Helio artifacts (all batch-end shared surfaces except the yaml itself,
which also rides batch end): icon `ui/helio-app/src/integrations/icons/calendly.svg`
+ `providerIcons.ts` entry; AI docs `agents/plugins/heliox/skills/tool/calendly.md`
(must cover URI-vs-UUID, the no-reschedule-endpoint rule, the paid-plan gate
on `book create`, and the range cap: **tell the assistant `availability slots`
takes up to ~7 days per request — the API reference caps it at 1 week even
though a guide page says 31 — so it chunks longer windows; `availability busy`
is ≤7 days**) + plugin bump/publish; client id/secret appended by lane 1 to
integration-service config in `config/` + `deploy/` Helm Secret together.
`provider-gen` is run **locally only for validation** on this branch
(projections not committed, per master plan §2); L4 builds helio-cli with a
local, uncommitted `go.mod` `replace` pointing at this anycli worktree.

## 6. Test plan (five layers)

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes asserting Bearer header, request paths/query (URI expansion, `me` resolution, range params **passed through unmodified — no client-side range rejection**), pagination passthrough, cancel/no-show/link/book request bodies, 401-rejected mapping, plain + `--json` error rendering | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli calendly -- me` then each verb family against the live API. **Assert every verb family returns 2xx with *exactly* the §3 requested six-scope set** — in particular `event invitees`, which confirms the catalog is right that invitee reads fall under `scheduled_events:read` (⊆ the requested `scheduled_events:write`) and need no `invitees:read`. Note: L2 uses a bearer PAT, so it cannot exercise the authorize endpoint's wire `scope`-param validation — wire acceptance of the set is a **L5** concern, not L2's. **Also record the observed live range cap for both `availability slots` (event_type_available_times) and `availability busy` (user_busy_times)** — resolve the guide-vs-reference 31d/7d contradiction against the live API and note it | **yes** — a Calendly **Personal Access Token** from the lane-2 test account, created with the §3 six-scope set (new PATs are scoped; an unscoped PAT has no access). PAT suffices because anycli only sees a bearer token |
| L3 | local `go run ./cmd/provider-gen` + `--check` against this branch's bundle — **passes as-is** (`refresh_lease: none` satisfies the `standard_oauth` contract; no contract edit, no new contract test). Also run helio-cli + integration-service unit suites with the local `replace` | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` with `provider: "calendly"`, real `access_token` **and** `refresh_token` from the lane-1 **sandbox app**, deliberately short `expires_at` → forces the A3 refresh; then `heliox tool calendly -- me`. Run the tool **twice**: the second call proves the rotated (single-use) refresh token was persisted by A3 strict write-back — this is the Calendly-specific L4 assertion, not optional | **yes** — lane-1 sandbox app client id/secret (uncommitted local `config/cloud.yaml`) + a token pair minted from it on the lane-2 test account |
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
3. **Range cap — guide vs. API reference contradiction (divergence recorded)**:
   for `event_type_available_times` the *guide* pages say 31 days but the *API
   reference* (Stoplight `api-docs`) and Calendly support/community say the span
   **"can be no greater than 1 week"** (7 days). Per the "official docs win /
   endpoint contract is authoritative" rule, the tool and the AI-facing doc
   treat **~7 days** as the effective cap for this endpoint (and ≤7 days for
   `user_busy_times`), the tool enforces nothing client-side, and **L2 records
   the observed live cap for both endpoints** to settle the contradiction
   empirically. (This reverses an earlier draft that took the guide's 31-day
   figure at face value.)
4. **Per-credential refresh lease for the rotating-refresh class — escalate to
   the master plan (§6 open question), do NOT ship on this branch.** Calendly,
   Pennylane, Airtable, PandaDoc (and any other single-use rotating provider)
   share one residual failure under `refresh_lease: none`: a genuinely
   *concurrent* refresh burns the loser's token → one spurious "reconnect
   required" (§3). Selecting `OAuthLeaseCredential` on the shared
   `standard_oauth` contract would serialize refreshes per credential and close
   that window **for the whole class at once** — but it is a program-wide edit
   to a contract governing ~140 providers, and master-plan §5 pins
   `standard_oauth` to "zero service code". So it belongs to the batch lead as
   a master-plan amendment/open-question covering the entire rotating class,
   **not** to a single tool branch. Calendly ships `none` (matching every
   sibling) today; if the program later adopts the class-wide lease, Calendly
   flips its one field along with the rest of the class. Recommendation: raise
   this in the master plan's §6 before Wave 1's rotating-provider batches flip
   visible.
5. **`invitees:read` scope — EXCLUDE it; the Scope Catalog is authoritative and
   it is not a valid scope string (divergence recorded).** Calendly's `/scopes`
   page contradicts itself: its **"Choosing Scopes" worked example** names
   `invitees:read` ("Required to associate meetings with people and contact
   details"), but its **Scope Catalog** — the authoritative list of valid scope
   *strings* — does **not** contain `invitees:read` at all and maps List Event
   Invitees (`GET /scheduled_events/{uuid}/invitees`, `…/invitees/{uuid}`) under
   `scheduled_events:read`. Two facts make exclusion the only correct choice, not
   a fail-safe "request it anyway":
   - **It is redundant.** The catalog's hierarchy rule ("`:write` implicitly
     includes the corresponding `:read` within the same domain") means the
     `scheduled_events:write` we already request grants `scheduled_events:read`,
     which the catalog says *is* the invitee-read scope. Invitee reads are fully
     covered without any invitee-specific scope.
   - **It is actively harmful on the wire.** These scopes ship in the authorize
     `scope` param (§3/§5). Calendly **strictly validates** that param and
     rejects any unknown/unlisted string with a hard "The requested scope is
     invalid, unknown, or malformed" error (the documented `openid` rejection is
     the precedent). An unlisted scope does **not** silently no-op — it breaks
     the whole `/oauth/authorize` request and **bricks the connect flow for every
     account**, including the headline `event invitees` capability. So the
     earlier "harmless if redundant, load-bearing if required" framing was
     falsified on both halves.
   The shipped six-scope set (`users:read, event_types:read, availability:read,
   scheduled_events:write, scheduling_links:write, organizations:read`) is
   capability-complete. Note the arbiter: **L2 uses a bearer PAT and never
   exercises the authorize endpoint's wire `scope`-param validation**, so it
   cannot settle wire acceptance — that only surfaces at **L5** (real consent).
   L2's job is narrower: confirm `event invitees` returns 2xx under the six-scope
   set, empirically proving invitee reads ride `scheduled_events:read`.

### Resolved during review revision

- **Refresh-lease vs. `standard_oauth` contract (major finding from review)**:
  an earlier draft made Calendly the **first-ever** selector of
  `refresh_lease: credential` and, to ship it, relaxed the shared
  `standard_oauth` runtime contract — a committed, non-generated service edit
  changing refresh-validation semantics for ~140 providers, justified by an
  **unsubstantiated "permanent brick" claim**. Three problems (all verified):
  (a) *Necessity* — under plain single-use rotation + A3 strict write-back a
  concurrent race yields **one spurious `invalid_grant`/reconnect**, not a
  brick; a true brick needs OAuth-2.1/BCP reuse-detection *family revocation*
  that Calendly's official docs never state, so the load-bearing mechanism was
  missing. (b) *Consistency* — sibling rotating providers in the same catalog
  (Pennylane row 184 "refresh-token rotation", Airtable row 13, PandaDoc row
  44) all ship `standard_oauth` with `refresh_lease: none`; Calendly was framed
  as a lone exception without reconciling them. (c) *Process* — master-plan §5
  states `standard_oauth` bundles need **zero** service code, which a
  shared-contract edit violates. **Resolved by adopting the minimal orthogonal
  design (reviewer option A)**: Calendly ships `refresh_lease: none` like every
  sibling, relies on A3 strict write-back (`token_refresh.go:60-66`) +
  transient/permanent classification (`:165`), and the contract edit is
  **dropped entirely**. The class-wide per-credential-lease idea is escalated
  to the master plan (§7 open question 4), not shipped here. §3/§5/§6 now claim
  zero service code honestly.
- **Scopes (was OQ "wire `scope` param")**: `/scopes` documents a wire-level
  space-separated `scope` param and a granular-scopes "no access until
  approved" model, so scopes ship **on the wire** (`auth.oauth.scopes`) plus
  the same set at app registration — see §3/§5. No longer open.
- **`invitees:read` scope (two review passes — first added, then correctly
  removed)**: an intermediate draft, worried the invitee resource had no
  coverage, **added `invitees:read`** to the scope set. A later review verified
  against the authoritative Scope Catalog that this was wrong on two counts:
  (a) `invitees:read` is **not a valid catalog scope string** — the catalog maps
  `GET /scheduled_events/{uuid}/invitees` and `…/invitees/{uuid}` under
  `scheduled_events:read`, which the requested `scheduled_events:write` already
  grants via the `:write` ⊇ `:read` hierarchy rule, so invitee reads are covered
  without it; and (b) because scopes ship on the wire, Calendly's strict
  authorize-endpoint validation would **reject the unlisted string and brick the
  entire connect flow** (same failure class as the documented `openid`
  rejection), not silently no-op. **Resolved by dropping `invitees:read`** from
  `auth.oauth.scopes` + `display_scopes` (§5), the §3 derivation, the §2
  per-endpoint table, and the §7 #5 rationale — leaving the capability-complete
  six-scope set (`users:read`, `event_types:read`, `availability:read`,
  `scheduled_events:write`, `scheduling_links:write`, `organizations:read`). The
  same verification pass corrected the §2/§3 mapping of
  `GET /event_type_available_times` to `event_types:read` (the catalog lists it
  there, not under `availability:read`), and confirmed from the catalog that
  `availability:read` is a real scope and that the no-show writes **and the
  Scheduling-API `POST /invitees`** are catalog-enumerated under
  `scheduled_events:write` (the booking mapping is catalog-verified, not an
  assumption pending L2).
- **Disconnect revoke (was OQ "disconnect revoke")**: Calendly officially
  documents Revoke Access/Refresh Token
  (`POST https://auth.calendly.com/oauth/revoke`, `client_id`/`client_secret`/
  `token`), so v1 ships `disconnect_mode: provider_revoke` with a declarative
  `revoke:` block (`client_auth: form`) — see §5. No longer open.
