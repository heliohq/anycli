# SavvyCal — per-tool design (catalog row 201)

**Status:** scratch design for the `tool/savvycal` branch (batch lead strips this file at batch-end).
**Catalog row:** #201 — SavvyCal · anycli id `savvycal` · provider key `savvycal` · auth lane `oauth_light` · wave 3 · Scheduling & eSign.
**Verified against:** official docs at https://developers.savvycal.com/ (Docusaurus "SavvyCal Meetings" REST API reference + `/authentication`), cross-checked with the pre-migration endpoint reference (`savvycal.com/docs/api/…`, now 301-redirected into the new site; parameter tables recovered from archived copies and re-confirmed against the live endpoint pages' method/path/status-code renders).

## 1. Scope: which API surface and why

SavvyCal is a scheduling-link product. The API surface (base `https://api.savvycal.com`, all under `/v1`, `Authorization: Bearer <token>`) has six categories: Events, Scheduling Links, Current User, Time Zones, Webhooks, Workflows. An AI teammate's real jobs with SavvyCal are:

1. **Answer "what's on the calendar / what got booked"** — list and inspect events scheduled through SavvyCal.
2. **Hand out and manage the user's scheduling links** — find the right link to share in a chat/email, create or tweak links, enable/disable them.
3. **Propose concrete times** — read a link's available slots so the assistant can say "Tuesday 10:00 or 14:30 work" instead of pasting a bare URL.
4. **Book or cancel on someone's behalf** — create an event on a link for a named scheduler (matching an available slot), or cancel an existing event with a reason.

That maps to wrapping **Events + Scheduling Links + Current User** in v1:

| Verb | Method + path (verified) | Notes |
|---|---|---|
| whoami | `GET /v1/me` | identity check; also the bundle's identity endpoint |
| list events | `GET /v1/events` | `state=confirmed\|canceled\|all` (default confirmed), `period=past\|upcoming\|all` (default upcoming), cursor pagination `before`/`after`/`limit` (default 20, max 100) |
| get event | `GET /v1/events/:event_id` | |
| create event | `POST /v1/links/:link_id/events` | required: `display_name`, `email`, `start_at`, `end_at`, `time_zone`; optional `fields[]` (id/type/label/value), `metadata` object. Must match an available slot. Conferencing info (e.g. Zoom) may attach late |
| cancel event | `POST /v1/events/:event_id/cancel` | optional `cancel_reason` |
| list links | `GET /v1/links` | cursor pagination `before`/`after`/`limit` |
| get link | `GET /v1/links/:link_id` | |
| create link | `POST /v1/links` (personal) / `POST /v1/scopes/:scope_slug/links` (team/individual scope) | body: `name`, `private_name`, `description`, `type` (`recurring` = multi-use, default \| `single` = single-use) |
| update link | `PATCH /v1/links/:link_id` | same fields as create |
| toggle link | `POST /v1/links/:link_id/toggle` | active ↔ disabled |
| duplicate link | `POST /v1/links/:link_id/duplicate` | |
| delete link | `DELETE /v1/links/:link_id` | |
| list slots | `GET /v1/links/:link_id/slots` | `from` (default now), `until` (default +7d), ISO-8601; returns `[{start_at, end_at, duration, rank}]`; ranked availability semantics: rank N is cumulative (filter `rank === N`, not `<= N`) |

**Deliberately excluded from v1:**

- **Webhooks** (5 endpoints) — server-callback plumbing, not an agent verb; Helio has no receiver wired to this tool. Add later only if an event-driven design asks for it.
- **Workflows** (3 endpoints) — read-only automation-config listing; no actionable agent job.
- **Time Zones** (`GET /v1/time_zones`, `GET /v1/time_zones/*segments`) — generic localized IANA data the model already knows; no differentiated value.

Pagination shape everywhere is `{"entries": [...], "metadata": {"before", "after", "limit"}}` (opaque cursors). The API is explicitly additive — new fields may appear in payloads; the implementation must pass provider JSON through rather than re-projecting into a closed struct.

## 2. anycli definition

**Stage-1 form decision: `service` type.** No official CLI exists for this API. (`svycal/appointments-cli` on GitHub is the CLI for **SavvyCal Appointments**, a separate headless scheduling-infrastructure product — not the SavvyCal Meetings REST API this provider wraps. It fails the "official CLI for this surface" test, so the cli-type rubric doesn't even reach the other criteria.)

`definitions/tools/savvycal.json`:

```json
{
  "name": "savvycal",
  "type": "service",
  "description": "SavvyCal scheduling as a tool (OAuth user token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SAVVYCAL_ACCESS_TOKEN"}
      }
    ]
  }
}
```

**Implementation:** `internal/tools/savvycal/` (package `savvycal`; id has no dashes, no normalization needed), registered in `internal/tools/register.go` as `RegisterService("savvycal", &savvycal.Service{})` — registration rides the batch-end merge; the definition file and package merge freely mid-batch.

Follow the `bitly`/`notion` service shape: `Service{BaseURL, HC, Out, Err}` struct so tests point at `httptest.Server`; `DefaultBaseURL = "https://api.savvycal.com/v1"`; `Execute(ctx, args, env)` reads `SAVVYCAL_ACCESS_TOKEN`, builds a cobra root with `SilenceUsage/SilenceErrors`, exit codes 0 success / 1 runtime-API failure (typed `apiError`) / 2 usage errors, `--json` structured error envelope, provider-JSON passthrough on stdout.

**Subcommand tree (axis-② verbs):**

```
savvycal me
savvycal event list   [--state confirmed|canceled|all] [--period past|upcoming|all]
                      [--limit N] [--after CUR] [--before CUR]
savvycal event get    <event_id>
savvycal event create <link_id> --display-name NAME --email EMAIL
                      --start ISO --end ISO --time-zone TZ
                      [--field id=value ...] [--metadata JSON]
savvycal event cancel <event_id> [--reason TEXT]
savvycal link list      [--limit N] [--after CUR] [--before CUR]
savvycal link get       <link_id>
savvycal link create    --name NAME [--scope SLUG] [--description D]
                        [--private-name P] [--type recurring|single]
savvycal link update    <link_id> [--name|--description|--private-name|--type]
savvycal link toggle    <link_id>
savvycal link duplicate <link_id>
savvycal link delete    <link_id>
savvycal link slots     <link_id> [--from ISO] [--until ISO]
```

**JSON output:** provider JSON passthrough + trailing newline for every command (list endpoints emit the full `{entries, metadata}` envelope so the agent can page with `--after`). `--json` accepted globally for uniformity (always-on), and errors render both plain-text and a `--json` envelope, per the notion/bitly contract. Error mapping: non-2xx with SavvyCal's `{"errors": {...}}` 422 validation body passed through inside the `apiError`; 401 → credential rejection message.

## 3. Credential fields and auth flow (oauth_light — verified)

Official `/authentication` doc confirms the audit verdict; **no divergence from the catalog**:

- **Registration:** fully self-serve — Settings → Developers → "OAuth Applications": supply name + redirect URI(s), receive client id + client secret. No review/approval step ⇒ `oauth_light` is correct.
- **Authorize:** `https://savvycal.com/oauth/authorize?response_type=code&client_id=…&redirect_uri=…` → callback with `code`.
- **Token:** `POST https://savvycal.com/oauth/token`, form-encoded, `grant_type=authorization_code` with `code`, `client_id`, `client_secret`, `redirect_uri` ⇒ bundle `token_exchange_style: form_secret`.
- **Token semantics:** response `{access_token, refresh_token, expires_in: 7200, token_type: "bearer"}` — 2-hour access tokens with a real refresh cycle; `grant_type=refresh_token` (with `client_id` + `client_secret`) returns the same shape. The token gateway's standard refresh path handles this; `refresh_lease: none`, `single_active_token: false` (nothing documented suggests a new grant revokes prior tokens — confirm empirically at L4/L5 and flip if observed).
- **Scopes:** none exist — the docs define no scope parameter; a grant covers the account. Bundle carries no `scopes:`; `display_scopes` (if the generator requires a non-empty list for consent UI) uses a descriptive placeholder such as `[full_account]` — resolve against generator validation at implementation time.
- **PKCE:** not documented ⇒ `pkce: none` (confidential client with secret).
- **Revoke:** no revocation endpoint documented ⇒ `disconnect_mode: local_only` (Notion precedent).
- **Personal access tokens** (`pt_secret_…`, created at savvycal.com/developers) authenticate identically (`Authorization: Bearer`) — they are the L2 harness credential, **not** a Helio connect path.

Credential field consumed by the tool: `access_token` only (+ `account_key` for the connection row, per the standard bundle credential block).

## 4. Helio provider bundle plan

**Naming axes:** ① CLI command `savvycal` (flat, no `tool.group` — independent brand) · ② anycli id `savvycal` · ③ provider key `savvycal`. All three identical ⇒ **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.

`integrations/providers/savvycal/provider.yaml` sketch (hidden-first):

```yaml
schema: helio.provider/v1
key: savvycal
go_name: SavvyCal

presentation:
  name: SavvyCal
  description_key: savvycal
  consent_domain: savvycal.com
  visible: false            # hidden-first; flip + regen is the go-live change
  order: <next free in Scheduling block>

auth:
  type: oauth
  owner: individual         # the provider sees a person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://savvycal.com/oauth/authorize
    token_url: https://savvycal.com/oauth/token
    token_exchange_style: form_secret
    pkce: none
    display_scopes: [full_account]   # SavvyCal has no scopes; see §3
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://api.savvycal.com/v1/me
  stable_key: /id                       # "user_01ED74…" ULID-style id
  label_candidates: [/email, /display_name, /id]

connection:
  mode: isolated
  disconnect_mode: local_only           # no revoke endpoint documented
  runtime_strategy: standard_oauth      # zero service-side Go; no adapter needed

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: savvycal
  kind: oauth
```

`standard_oauth` fits exactly (standard code flow, form-secret exchange, JSON userinfo identity, declarative no-op revoke) — no `service/adapter_*.go`.

**Shared surfaces riding the batch-end merge** (not this branch): `register.go` entry, anycli tag + `helio-cli/go.mod` pin bump, the one canonical `provider-gen` run (five projections), icon `ui/helio-app/src/integrations/icons/savvycal.svg` + `providerIcons.ts` append, heliox plugin sub-doc under `agents/plugins/heliox/skills/tool/` + version bump/publish, and lane 1's `config/` + `deploy/` Helm-Secret append of the SavvyCal client id/secret (id and secret land together; must precede L5).

**On-branch validation only (never committed):** local `provider-gen` + `--check` run against the branch bundle; `helio-cli/go.mod` `replace github.com/heliohq/anycli => ../path/to/anycli-worktree`; dev app client id/secret as uncommitted `config/cloud.yaml` entries.

## 5. Test plan (five layers)

| Layer | What runs | External credentials needed |
|---|---|---|
| **L1** | `go test ./...` in anycli: TDD-first unit tests in `internal/tools/savvycal/` against `httptest.Server` fakes — assert request method/path/query/body shape, `Authorization: Bearer` injection from `SAVVYCAL_ACCESS_TOKEN`, `{entries, metadata}` passthrough, 401/404/422 error rendering plain + `--json`, exit codes 0/1/2 | none |
| **L2** | dev harness against the real API: `ANYCLI_CRED_ACCESS_TOKEN=pt_secret_… anycli savvycal -- me`, then `link list`, `link slots`, `event list --period all`, and one full `link create → link slots → event create → event cancel → link delete` round trip on a throwaway link | **yes** — SavvyCal test account (lane 2 pool) + a personal access token from savvycal.com/developers (verified interchangeable with OAuth tokens per official auth doc) |
| **L3** | local `provider-gen` + `provider-gen --check` against the branch bundle; anycli suite + `helio-cli` build/tests via the uncommitted `replace`; integration-service suite | none |
| **L4** | singleton + `POST /internal/test-only/connections/seed` with a **real** OAuth `access_token` + `refresh_token` minted from lane 1's dev app, seeded with a short `expires_at` so the first `heliox tool savvycal -- me` forces the token-gateway refresh-and-write-back path (SavvyCal's 2-hour tokens make the refresh path the hot path in production — exercising it at L4 is not optional polish) | **yes** — lane-1 registered SavvyCal OAuth app (client id/secret as uncommitted local `config/cloud.yaml` entries) + test account to mint the token pair |
| **L5** | human-in-the-loop (lane 3, per-batch sweep, oauth path): `heliox tool savvycal auth` → connect link → real SavvyCal consent → `oauth_connected` system event on the originating channel → one unseeded `heliox tool savvycal -- event list` | **yes** — lane-1 config landed in `config/` + `deploy/` Helm Secret; test account for the consent session |

Definition of done per master plan §2: L1–L4 green on-branch before the batch-end merge; L5 + icon + published docs + the visible flip complete the tool. Until the flip, SavvyCal is "code-complete (hidden)".

## 6. Open items / risks

- **Rate limits:** none documented; implement no client-side throttling, surface provider 429s verbatim if they occur.
- **`display_scopes` with a scope-less provider:** confirm what the generator requires when `scopes:` is absent (see §3); adjust the placeholder or omit at implementation time.
- **Token rotation on refresh:** refresh responses include a `refresh_token`; assume rotation (store the returned one — the standard gateway already writes back). Confirm at L4.
- **Slot-match constraint on `event create`:** creation fails unless `start_at/end_at` match an available slot — the AI docs sub-doc must tell the assistant to call `link slots` first; the 422 passthrough makes the failure self-explanatory otherwise.
