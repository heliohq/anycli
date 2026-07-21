# Tool design: Outreach (`outreach`)

**Status:** Stage-1 design (pre-implementation). Scratch file on branch `tool/outreach`; the batch lead strips it at batch-end.
**Catalog row:** #66 — Outreach | anycli id `outreach` | provider key `outreach` | lane `oauth_light` | wave 2 | Sales Engagement.
**Official docs verified:** 2026-07-21 — https://developers.outreach.io/api (overview), https://developers.outreach.io/api/oauth (OAuth), https://developers.outreach.io/api/getting-started (scopes/limits), https://developers.outreach.io/api/making-requests (JSON:API conventions).

## 1. Auth-lane verification (independent, against official docs)

Outreach was **not** in the 2026-07-21 oauth-audit table (that audit covered only pre-audit `api_key` tools); it sat in `oauth_light` already. Verified independently:

- **Registration model:** self-serve. Apps are created in the Outreach admin UI (Settings → Integrations → Your apps) / developer portal; the API-access feature gives redirect URIs + scope selection. **Development credentials are provisioned immediately**, no review.
- **Multi-tenant reach of dev credentials:** the official OAuth page states *"Application's development credentials can be authorized by any user from any org."* Limits: unlimited use for up to 10 users of the owning org; users from **other orgs must re-authorize weekly**, the consent screen carries an "unreviewed app" warning, and Outreach periodically expires grants/revokes tokens on dev credentials. **Production credentials** (no weekly re-consent, no warning) are issued only after the app goes through the publish/review process.
- **Verdict: `oauth_light` stands** for dev, L4, L5, and hidden shipping — self-serve registration, standard authorization-code flow, arbitrary orgs can authorize. **Divergence note recorded:** the weekly re-consent + periodic token revocation on dev credentials is a real GA-quality gap; before (or soon after) the visible flip, lane 1 should submit the app through Outreach's publish process to obtain production credentials. This is a publish-review clock on *polish*, not a gate on code-complete — closer to a light `oauth_review` tail than the catalog row admits. Flag to the batch lead / wave board.
- **Flow parameters (official):**
  - Authorize: `https://api.outreach.io/oauth/authorize?client_id&redirect_uri&response_type=code&scope&state` (scopes space-separated).
  - Token: `POST https://api.outreach.io/oauth/token`, **form body** with `client_id`, `client_secret`, `redirect_uri`, `grant_type` (`authorization_code` / `refresh_token`) → `token_exchange_style: form_secret`. No PKCE documented → `pkce: none`.
  - Token response: `{access_token, token_type: "Bearer", expires_in: 7200, refresh_token, scope, created_at}`.
  - Lifetimes: access token **2 h**; refresh token **14 days**, **rotating** — a new refresh token is issued with each refresh and docs say to always use the most recent one. Max 100 refresh tokens per user/app pair; new access tokens throttled to **one per user per 60 s** (429 beyond).
  - **No RFC 7009 revoke endpoint documented** → `disconnect_mode: local_only`, no `revoke` block.
- **Scope format (API v2):** `<pluralizedResource>.<read|write|delete|all>` (e.g. `prospects.read`, `sequenceStates.all`). Scopes are **not additive** (`prospects.write` does not include read). Missing scope → 403 `unauthorizedOauthScope`. The registered app must have every scope the bundle requests (authorize may request a subset of the app's scopes) — lane 1 must configure the dev app with the full bundle scope list below.

## 2. What an AI teammate does with Outreach → API surface

Outreach is a sales-engagement system: sequences (cadences) executed over prospects via mailboxes, producing mailings/calls/tasks. The teammate's real jobs: look up and maintain prospects/accounts, **enroll prospects in sequences** (the core write), pause/resume enrollments, work the task list, and report on outcomes (mailings opens/clicks/replies, calls, opportunities).

Wrapped surface — all under `https://api.outreach.io/api/v2` (JSON:API 1.0, `Content-Type: application/vnd.api+json`):

| Resource | Endpoints | Why |
|---|---|---|
| prospects | GET list (+`filter[q]`, `filter[emails]`…), GET/POST/PATCH by id | core CRM object; global search supported |
| accounts | GET list (+`filter[q]`), GET/POST/PATCH | company context for prospects |
| sequences | GET list, GET | pick the cadence to enroll into |
| sequenceStates | GET list, **POST (enroll prospect: relationships prospect+sequence+mailbox)**, POST actions `pause`/`resume`/`finish` | the core sales-engagement write |
| sequenceSteps | GET list (filter by sequence) | inspect cadence content |
| mailboxes | GET list | required relationship for enroll |
| mailings | GET list (filter by prospect/state), GET | email outcomes: delivered/opened/clicked/replied |
| calls | GET list, GET | call activity read |
| tasks | GET list, GET, POST, POST actions (`complete`, `snooze`) | teammate works the task queue |
| opportunities | GET list, GET | pipeline reporting |
| users | GET list, GET | owner resolution / assignment |
| templates | GET list, GET | inspect email templates referenced by steps |
| stages, personas | GET list | ids needed when creating/updating prospects |

Out of scope v1: snippets, callDispositions/callPurposes, webhooks, custom objects, Kaia recordings, S2S auth. Deletes are omitted v1 (destructive; `*.all` scopes are requested only where create/update is needed).

## 3. anycli definition + service

- **Type decision (stage-1 rubric):** `service`. There is no official Outreach CLI at all, so the `cli` criteria fail at the first test. Implement `internal/tools/outreach/` against the REST API (matches 21-of-23 precedent; `internal/tools/notion/` is the shape to copy).
- **Definition** `definitions/tools/outreach.json`:

```json
{
  "name": "outreach",
  "type": "service",
  "description": "Outreach sales engagement as a tool (OAuth 2.0 user token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "OUTREACH_ACCESS_TOKEN"}
      }
    ]
  }
}
```

- **Registration:** `RegisterService("outreach", &outreach.Service{})` in `internal/tools/register.go` — a batch-end shared-surface change; the definition file and `internal/tools/outreach/` merge freely mid-batch.
- **Go package:** `internal/tools/outreach/` (id has no dashes; package name == id).
- **Cobra tree (resource → verb):**

```
outreach prospect   list|get|create|update      (--q, --email, --account-id, --stage-id, --owner-id, field flags)
outreach account    list|get|create|update      (--q, --domain, field flags)
outreach sequence   list|get|steps              (steps = sequenceSteps filtered by sequence id)
outreach enrollment list|add|pause|resume|finish  (sequenceStates; add --prospect-id --sequence-id --mailbox-id)
outreach mailbox    list
outreach mailing    list|get                    (--prospect-id, --state)
outreach call       list|get                    (--prospect-id)
outreach task       list|get|create|complete|snooze  (--due, --note, --prospect-id, --action)
outreach opportunity list|get
outreach user       list|get
outreach template   list|get
outreach stage      list
outreach persona    list
```

`enrollment` is the human word for `sequenceStates` (axis-② id stays `outreach`; this is only a subcommand name). Pause/resume/finish use the documented JSON:API action POSTs (`/sequenceStates/{id}/actions/…` dialect, as with `/tasks/{id}/actions/snooze`).

- **HTTP conventions:** base `https://api.outreach.io/api/v2`, headers `Authorization: Bearer $OUTREACH_ACCESS_TOKEN`, `Content-Type: application/vnd.api+json`, `Accept: application/vnd.api+json`. Struct fields `BaseURL`/`HC`/`Out`/`Err` for httptest injection (notion pattern). Exit codes 0/1/2 with typed `apiError` and `--json` error envelope; surface JSON:API `errors[]` (id/title/detail) and the 403 scope-error id verbatim. Respect 429 by reporting `X-RateLimit-Reset`/`Retry-After` in the error — no silent retry loops.
- **JSON output shape (design 003 conventions):** flatten each JSON:API resource to `{ "id", "type", ...attributes, "<rel>_id": … }` (relationship ids hoisted, e.g. `account_id`, `owner_id`); list commands emit `{ "items": [...], "next_cursor": <opaque page[after] cursor or null>, "count": <when count=true> }`. Pagination flags: `--limit` (`page[size]`, cursor-based — the offset style is deprecated upstream) and `--cursor` (`page[after]`). `--fields` maps to sparse fieldsets.
- **TDD:** unit tests per resource file with `httptest.Server` fakes asserting method/path/query (filter/sort/page syntax), the two JSON:API headers, bearer injection, request body shape for create/patch/enroll/actions, flattening, pagination cursor extraction, and both plain + `--json` error rendering (422 with `source.pointer`, 403 scope error, 429). Never hit the real API from unit tests.

## 4. Helio provider bundle plan

Axes (master plan §3): ① CLI word `outreach` (flat command, no group) · ② anycli id `outreach` · ③ provider key `outreach`. **id == key → no `toolToProvider` entry.**

`integrations/providers/outreach/provider.yaml` (hidden-first):

```yaml
schema: helio.provider/v1
key: outreach
go_name: Outreach

presentation:
  name: Outreach
  description_key: outreach
  consent_domain: outreach.io
  visible: false            # hidden-first; flip is the go-live change
  order: 130                # after current max (x=120); batch lead settles final ordering

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://api.outreach.io/oauth/authorize
    token_url: https://api.outreach.io/oauth/token
    token_exchange_style: form_secret   # secret in form body, per official curl examples
    pkce: none                          # not offered
    scopes:
      - prospects.all
      - accounts.all
      - sequences.read
      - sequenceStates.all
      - sequenceSteps.read
      - mailboxes.read
      - mailings.read
      - calls.read
      - tasks.all
      - opportunities.read
      - users.read
      - templates.read
      - stages.read
      - personas.read
    display_scopes: [prospects.all, accounts.all, sequences.read, sequenceStates.all,
                     tasks.all, mailings.read, calls.read, opportunities.read]
    single_active_token: false
    refresh_lease: none                 # required by the standard_oauth runtime contract (see risk below)
    # no revoke block: Outreach documents no RFC 7009 endpoint → local_only

identity:
  source: userinfo
  url: https://api.outreach.io/api/v2   # API root: returns current OAuth app/token + org attributes
  stable_key: /data/attributes/user/email        # PROVISIONAL — pin at L2 (see verification items)
  label_candidates: [/data/attributes/user/email, /data/attributes/org/shortname]

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
  name: outreach
  kind: oauth
```

Zero integration-service code (`standard_oauth` golden path). Config: `oauth.providers.outreach.{client_id,client_secret}` appended by lane 1 to `config/` + the Helm Secret in `deploy/` together (Config Sync rule); dev credentials ride agents' **uncommitted** local `config/cloud.yaml` until then. Icon: `ui/helio-app/src/integrations/icons/outreach.svg` + manual `providerIcons.ts` registration (batch-end). AI docs: `agents/plugins/heliox/skills/tool/outreach.md` (flat providers get a single sub-doc; google/microsoft use dirs because they are groups) + plugin version bump + publish, riding the batch-end merge.

### Verification items to close before the bundle merges (L2/L4)

1. **Identity JSON pointers.** Official docs confirm `GET /api/v2` returns "information about your current OAuth application and token, as well as attributes related to your organization", but publish no example body; no `/users/me` exists. Pin `stable_key` / `label_candidates` against the real root response at L2 (first real-token harness session; a plain `curl` alongside). Expected candidates: user email + org shortname. If the root turns out to expose only org-level fields (no user identity), fall back to `stable_key: /data/attributes/org/shortname` — account key would then be org-scoped, which is acceptable for per-assistant isolated connections but must be a recorded decision.
2. **Root endpoint header tolerance.** `fetchUserInfo` sends only `Authorization` + `Accept: application/json`; Outreach's 415 guard is documented for `Content-Type` on requests. Confirm at L2 that a body-less GET to `/api/v2` without `Content-Type: application/vnd.api+json` returns 200; if not, the identity fetch needs the generic capability set to grow a reviewed header option (do NOT write a one-off adapter first — per provider-yaml.md guidance).
3. **Scope-string casing** for camelCase resources (`sequenceStates.all`, `sequenceSteps.read`) — verify accepted verbatim at app registration + authorize.

### Risk note: rotating refresh tokens under `refresh_lease: none`

The `standard_oauth` runtime contract (integration-service `model/runtime_contract.go`) fixes `refresh_lease: none`; the `credential` lease scope exists in the enum but no reviewed strategy admits it. Outreach rotates refresh tokens on every refresh and throttles token minting to 1/user/60 s. Single-flight refresh is the common case and the token gateway's A3 strict write-back guarantees the newest refresh token is persisted or the request 5xxs; a cross-replica concurrent refresh would either 429 (throttle) or momentarily race the rotation. L4 explicitly exercises the refresh path (short `expires_at` seed). If L4 shows reuse-invalidated refresh tokens under concurrency, the fix is a reviewed contract change (admit `refresh_lease: credential` for standard_oauth), not an adapter — flag to batch lead if hit.

## 5. Test plan (five layers)

| Layer | What runs | External credentials needed |
|---|---|---|
| L1 | `go test ./...` in anycli: httptest fakes per resource (request shape, JSON:API headers, bearer env injection, flatten/pagination, error envelopes 422/403/429, exit codes) | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real token> anycli outreach -- prospect list --limit 3` etc. against the live API; also pin identity pointers (verification items 1–2) | **YES** — Outreach has no PAT/API-key mode; the token can only be minted through the dev app's OAuth flow. Needs lane-1 dev app (client id/secret + redirect URI) and a pool test account; one manual authorize+exchange yields the L2 bearer (2 h TTL — re-mint per session or keep the refresh token handy) |
| L3 | `go run ./cmd/provider-gen && go run ./cmd/provider-gen --check` locally on-branch (regens NOT committed, per master plan §2); helio-cli build+tests with a local uncommitted `replace github.com/heliohq/anycli => <this worktree>` in `helio-cli/go.mod` | none |
| L4 | singleton; `POST /internal/test-only/connections/seed` with real `access_token` + `refresh_token` and a **short `expires_at`** so the very first `heliox tool outreach -- prospect list` forces the gateway refresh + strict write-back (rotation!); confirm a second call uses the rotated pair | **YES** — dev app client id/secret in local uncommitted `config/cloud.yaml` (lane 1 distributes at app creation) + real token pair from the test account |
| L5 | still hidden: `heliox tool outreach auth` → connect link → real consent on the pool account (expect the "unreviewed app" dev-credential warning) → `oauth_connected` event on the channel → unseeded live run | **YES** — human-in-the-loop consent (lane 3); provider config landed in `config/` + `deploy/` beforehand |

Definition of done per master plan §2: L1–L4 green on-branch pre-merge; bundle + regen + registry entry + icon + docs + pin bump land at batch end; L5 sweep then gates the visible flip. Reminder for the flip decision: dev-credential connections degrade weekly for non-owning orgs — schedule the Outreach publish review (production credentials) around the flip (see §1 divergence note).

## 6. Handoff asks (lane 1 / batch lead)

- Register the Outreach dev app with **exactly** the bundle scope list above and the standard Helio redirect URI; distribute client id/secret as uncommitted local config.
- Test-account pool: one Outreach org with a seat that has prospects/sequences permissions (enrollment needs a mailbox connected to the seat for a real `enrollment add` to succeed end-to-end).
- Wave board: record the production-credential publish-review follow-up (§1) so the flip isn't left on weekly-expiring dev credentials indefinitely.
