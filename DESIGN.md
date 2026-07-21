# Tool design: Attio (`attio`)

**Catalog row:** #18 · Attio · anycli id `attio` · provider key `attio` · lane `oauth_light` · Wave 1 · CRM.
**Branches:** anycli `tool/attio` (this worktree), Helio `tool/attio`.
**Status of catalog assumptions after independent verification against official docs:** catalog lane and audit endpoints confirmed; no catalog divergences. (Rev 2 corrects three internal spec errors found in review — update verb mapping, search required fields, comment author. Rev 3 corrects the search `--objects` default, which Rev 2 had hardcoded to five objects that error on default/Free-plan workspaces — see §2 and §6.) Details and citations below.

## 1. What an AI teammate does with Attio, and the API surface that serves it

Attio is a data-model-first CRM: a workspace holds **objects** (standard: `people`, `companies`, `deals`, `users`, `workspaces`; plus custom objects), each object holds **records**, and **lists** overlay records as pipelines/views with per-list **entries** and list-scoped attributes. Collaboration artifacts (**notes**, **tasks**, **comments/threads**) attach to records and entries.

An AI teammate's real jobs: "find the record for Acme / jane@acme.com", "what's the state of this deal", "log a note after the call", "create a follow-up task", "move this deal's stage / update an attribute", "add this new lead as a person + company and put them on our pipeline list", "answer questions across the pipeline (query with filters)". Because Attio schemas are per-workspace (custom objects, custom attributes, select/status options), the teammate also needs **schema introspection** to construct valid payloads — hardcoding attribute slugs would break on real workspaces.

**Wrapped surface** (REST API v2, base `https://api.attio.com`, `Authorization: Bearer <token>`, per https://docs.attio.com/llms.txt endpoint reference):

| Group | Endpoints | Why |
|---|---|---|
| Meta | `GET /v2/self` | whoami / token+workspace identity; also the bundle's identity endpoint |
| Objects | `GET /v2/objects`, `GET /v2/objects/{object}` | discover object slugs (incl. custom) before any record op |
| Attributes | `GET /v2/{target}/{id}/attributes`, `.../attributes/{attribute}`, `.../options`, `.../statuses` (read-only) | discover attribute slugs, select options, deal/status stages — prerequisite for correct writes |
| Records | `POST /v2/objects/records/search` (fuzzy, beta), `POST /v2/objects/{object}/records/query` (filter/sort), `GET/DELETE /v2/objects/{object}/records/{record_id}`, `PUT /v2/objects/{object}/records/{record_id}` (update, **overwrite** multiselect), `PATCH /v2/objects/{object}/records/{record_id}` (update, **append** multiselect), `POST /v2/objects/{object}/records` (create), `PUT /v2/objects/{object}/records` (assert/upsert by matching attribute) | the core CRM read/write loop |
| Lists & entries | `GET /v2/lists`, `GET /v2/lists/{list}`, `POST /v2/lists/{list}/entries/query`, `POST /v2/lists/{list}/entries` (create), `PUT` assert-by-parent, `PUT /v2/lists/{list}/entries/{entry_id}` (update, **overwrite** multiselect), `PATCH /v2/lists/{list}/entries/{entry_id}` (update, **append** multiselect), `DELETE /v2/lists/{list}/entries/{entry_id}` | pipeline work: add to list, change stage, query a pipeline |
| Notes | `GET /v2/notes` (filter by record), `GET/DELETE /v2/notes/{note_id}`, `POST /v2/notes` (markdown/plaintext) | meeting/call logging — a primary assistant write path |
| Tasks | `GET /v2/tasks`, `POST /v2/tasks`, `GET/PATCH/DELETE /v2/tasks/{task_id}` | follow-ups with due dates, assignees, linked records |
| Comments & threads | `GET /v2/threads`, `GET /v2/threads/{thread_id}`, `POST /v2/comments`, `GET/DELETE /v2/comments/{comment_id}` | participate in record discussions |
| Workspace members | `GET /v2/workspace_members`, `GET /v2/workspace_members/{id}` | resolve assignee/actor ids for tasks, notes, `request_as` |

**Deliberately out of scope (v1):** webhooks (runtime has no callback surface; presence-over-polling assistants don't need them for CLI use), meetings/call-recordings (beta, gated by separate `meeting`/`call_recording` scopes, not core CRM), files (upload lifecycle adds little for v1), attribute/object **write** endpoints (schema mutation is an admin act, not a teammate act — read-only introspection is enough and safer), list/object create-update (same reasoning).

**Pagination** (verified: https://docs.attio.com/rest-api/guides/pagination.md): `limit`/`offset` — query params on GET, JSON body fields on POST query endpoints; some newer endpoints (meetings) use cursors, but none we wrap. Surface `--limit`/`--offset` verbatim; do not auto-paginate. Search is the exception: `limit` only (default/max 25), no offset.

**Rate limits** (verified): 100 read rps / 25 write rps, `429` + `Retry-After`. No client-side throttling in v1; surface the 429 body through the standard error envelope.

## 2. anycli definition

**Stage-1 rubric: `service` type.** Attio ships no official CLI binary at all, so the `cli`-type conditions fail at the first gate. Standard service implementation against the HTTP API.

**Definition** — `definitions/tools/attio.json`:

```json
{
  "name": "attio",
  "type": "service",
  "description": "Attio CRM as a tool (records, lists, notes, tasks via OAuth token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "ATTIO_ACCESS_TOKEN"}
      }
    ]
  }
}
```

**Package:** `internal/tools/attio/` (id has no dashes; package name == id), registered in `internal/tools/register.go` `init()` as `RegisterService("attio", &attio.Service{})` — registration rides the batch-end merge; the definition file merges freely.

**Service shape** — copy `internal/tools/notion/` (the skill's named reference): `Service{BaseURL, HC, Out, Err}` so tests point at `httptest.Server`; exit-code contract 0 = success, 1 = runtime/API error (typed `apiError` carrying Attio's `status_code`/`type`/`code`/`message` error body), 2 = usage/parse; `--json` structured error envelope on stderr, matching the design-003 §3 built-in service conventions.

**Cobra tree (axis-① words inside the tool are resource-grouped, notion-style):**

```
attio whoami
attio object   list | get <object>
attio attribute list --object <o> | --list <l>   # plus: options/statuses via `attribute options|statuses`
attio record   search --query <q> [--objects <o1,o2,...>] [--request-as-member <member_id|email>] [--limit N]
attio record   query  <object> [--filter <json>] [--sort <json>] [--limit N] [--offset N]
attio record   get|delete <object> <record_id>
attio record   create <object> --values <json>
attio record   update <object> <record_id> --values <json> [--append]   # default PUT (overwrite multiselect); --append switches to PATCH (append multiselect)
attio record   upsert <object> --values <json> --match <attribute>      # PUT assert
attio list     list | get <list>
attio entry    query <list> [--filter <json>] [--limit N] [--offset N]
attio entry    add <list> --parent-record <id> --parent-object <o> [--values <json>]
attio entry    get|remove <list> <entry_id>
attio entry    update <list> <entry_id> --values <json> [--append]      # same contract as record update: default PUT (overwrite), --append → PATCH (append)
attio note     list [--record <object>:<id>] | get <note_id> | create --parent <object>:<id> --title <t> (--markdown <md> | --plaintext <txt>) | delete <note_id>
attio task     list | get <task_id> | create --content <txt> [--deadline <iso>] [--assignee <member_id>] [--record <object>:<id>] | update <task_id> | delete <task_id>
attio thread   list [--record <object>:<id>] | get <thread_id>
attio comment  create (--thread <id> | --record <object>:<id>) --content <txt> [--author <member_id>] | get <id> | delete <id>
attio member   list | get <member_id>
```

**Update semantics (record + entry)** — verified against https://docs.attio.com/rest-api/endpoint-reference/records/update-a-record-overwrite-multiselect-values.md and `.../update-a-record-append-multiselect-values.md` (and the list-entry twins under `endpoint-reference/entries/`): Attio exposes the same duality on both paths — `PUT /v2/objects/{object}/records/{record_id}` "the values supplied will overwrite/remove the list of values that already exist (if any)", `PATCH` at the same path "the values supplied will be created and prepended to the list of values that already exist (if any)". The CLI default for `record update` / `entry update` is **PUT (overwrite)** — the intuitive "update replaces what I set" semantics — and `--append` switches to **PATCH (append)**. Both verbs get L1 request-shape assertions.

**Search defaults** — verified against https://docs.attio.com/rest-api/endpoint-reference/records/search-records.md: the body requires `query` (maxLength 256), `objects` (min 1 item, slugs or IDs), and `request_as` (`{type: "workspace"}` or `{type: "workspace-member", workspace_member_id|email_address}`); `limit` is optional (default/max 25); no offset. The endpoint **validates each slug against the workspace's actual objects** and returns `400 invalid_request_error` / `code: value_not_found` (`"Object with slug/ID \"…\" not found."`) for any slug that isn't a present, enabled object. Per Attio's data model (https://docs.attio.com/docs/standard-objects/standard-objects.md and the [standard-objects help reference](https://attio.com/help/reference/managing-your-data/objects/manage-standard-objects)), only **`people` and `companies` are enabled by default in every workspace**; `deals`, `users`, and `workspaces` are *optional* standard objects, disabled until an admin activates them — and a Free-plan workspace caps at 3 objects total, so it may never hold them. Hardcoding the five-object set as the default would therefore make the natural first call `attio record search --query "Acme"` (no `--objects`) error with `value_not_found` on any default-onboarded or Free-plan workspace — the exact opposite of the doc's own "don't hardcode per-workspace schema" principle. CLI contract: **`--objects` defaults to the two always-present objects `people,companies`** when omitted (which also matches the dominant assistant intent: fuzzy-find the person/company behind a name); broader or custom-object search is opt-in via explicit `--objects` (slugs discoverable via `attio object list`). Dynamic resolution from `GET /v2/objects` was considered but rejected for the default path: it adds a synchronous round-trip on the search hot path and would silently fan search out across every custom object — surprising and against the "no silent expansion" rule. `request_as` defaults to `{"type": "workspace"}`, with `--request-as-member <member_id|email>` opting into member-scoped visibility (UUID → `workspace_member_id`, otherwise `email_address`). Both defaults are asserted in L1 request-shape tests, and the `value_not_found` error on a disabled slug is exercised in L2 against the live API.

**Comment author contract** — verified against https://docs.attio.com/rest-api/endpoint-reference/comments/create-a-comment.md: every variant of `POST /v2/comments` requires `format` (enum: `plaintext` only), `content`, and `author` (`{type: "workspace-member", id: <uuid>}`; "other types of actors are not currently supported"). The CLI always sends `format: "plaintext"` and defaults `author` to the connection's `authorized_by_workspace_member_id` resolved via `GET /v2/self` (one extra call, cacheable per token within the process); `--author <member_id>` overrides. If `/v2/self` yields no member id (e.g. a workspace API key not tied to a member), the command fails fast with a message requiring `--author` — no silent fallback.

**JSON output:** default human-readable summaries (record_text, ids, one line per item); `--json` emits the provider's response `data` verbatim (Attio already wraps in `{"data": ...}`) so agents can chain calls. Attio attribute *values* are structured (arrays of typed value objects); write flags take raw JSON (`--values`, `--filter`, `--sort`) rather than inventing a lossy flag-per-attribute DSL — the schema is per-workspace, so JSON passthrough plus `attribute list` introspection is the honest contract.

**TDD:** httptest fakes asserting request path/method/body shape, `Authorization: Bearer` injection, pagination param placement (query vs body), and both plain and `--json` error rendering — per anycli AGENTS.md, tests first; no real API from unit tests.

## 3. Auth flow — oauth_light verified against official docs

Verified 2026-07-21 against https://docs.attio.com/docs/oauth/authorize.md, https://docs.attio.com/docs/oauth/token.md, https://docs.attio.com/rest-api/tutorials/connect-an-app-through-oauth, https://docs.attio.com/rest-api/endpoint-reference/meta/identify.md:

- **Registration:** self-serve app at build.attio.com (developer dashboard); redirect URIs registered there; no review program mentioned for external workspaces to authorize. **Audit verdict (oauth_light) confirmed.**
- **Authorize:** `GET https://app.attio.com/authorize` with `client_id`, `response_type=code`, `redirect_uri` (exact match to a registered URL), `state`. **No `scope` parameter** — scopes are configured per-app in the dashboard's scopes tab, granted wholesale at consent (Notion-style).
- **Token:** `POST https://app.attio.com/oauth/token`, `application/x-www-form-urlencoded`, body `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `client_secret` → **`token_exchange_style: form_secret`**. No PKCE parameters documented → `pkce: none`.
- **Token semantics:** response is `access_token` + `token_type: Bearer` only. **No `refresh_token`, no `expires_in`; only `authorization_code` grant is documented.** `/v2/self` returns `exp: number | null` — tokens are effectively non-expiring workspace-scoped bearer tokens → `refresh_lease: none`; L4 seeds `access_token` only (the Slack-style "non-expiring token" branch of the skill's seeding guidance, not the short-expiry refresh exercise — there is no refresh cycle to exercise).
- **Identity:** `GET https://api.attio.com/v2/self` — `workspace_id` (token is workspace-scoped; also in `sub`), `workspace_name`, `workspace_slug`, `authorized_by_workspace_member_id`, `scope`. Declarative userinfo identity fits.
- **Revocation:** docs index shows `oauth/introspect` but no revoke endpoint → `disconnect_mode: local_only` (Notion precedent).
- **Scopes to configure on the Helio dev/prod app** (dashboard-side; from the `/v2/self` scope vocabulary): `record_permission:read-write`, `object_configuration:read`, `list_entry:read-write`, `list_configuration:read`, `note:read-write`, `task:read-write`, `comment:read-write`, `user_management:read`. These become the bundle's `display_scopes` (presentation only, since no scope param is sent).

**Credential fields** delivered to anycli: `access_token` (+ `account_key` for cache keying), exactly the Notion shape.

## 4. Helio provider bundle plan

**Axes:** ① CLI word `attio` (flat command, no `tool.group` — independent brand); ② anycli id `attio`; ③ provider key `attio`. **All three identical → no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`; no resolver work regardless of open-question-1's outcome.

`integrations/providers/attio/provider.yaml` (hidden-first):

```yaml
schema: helio.provider/v1
key: attio
go_name: Attio

presentation:
  name: Attio
  description_key: attio
  consent_domain: attio.com
  visible: false        # hidden-first; flip is the single go-live change
  order: <assigned at batch end>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.attio.com/authorize
    token_url: https://app.attio.com/oauth/token
    token_exchange_style: form_secret
    pkce: none
    display_scopes: [record_permission, object_configuration, list_entry, note, task, comment, user_management]
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://api.attio.com/v2/self
  stable_key: /workspace_id
  label_candidates: [/workspace_name, /workspace_slug, /workspace_id]

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
  name: attio
  kind: oauth
```

`runtime_strategy: standard_oauth` — zero integration-service Go: form_secret exchange, userinfo identity via JSON Pointer, no-op revoker (`local_only`) are all inside the generic capability set; no adapter candidate flags. Client id/secret land via lane 1 in `config/` + the `deploy/` Helm Secret together (Config Sync rule), never in the bundle.

**Batch-end items ridden by this tool** (not committed on this branch): `register.go` entry, anycli tag + `helio-cli/go.mod` pin bump, one `provider-gen` run (five projections), icon `ui/helio-app/src/integrations/icons/attio.svg` + `providerIcons.ts` append, provider sub-doc under `agents/plugins/heliox/skills/tool/` + plugin version bump/publish. Per the master plan §2, provider-gen regens are run locally for validation only and are **not** committed from this branch; the bundle itself is assembled by the batch lead at batch end.

## 5. Test plan — five layers

| Layer | What runs for attio | External inputs needed |
|---|---|---|
| **L1** | anycli `go test ./...`: `internal/tools/attio/` unit tests against httptest fakes — request shapes for every subcommand (incl. POST-body vs query-param pagination; record/entry update default **PUT** vs `--append` → **PATCH**, both verbs asserted; search body defaults `objects` = `["people","companies"]` (the two always-present standard objects, **not** the disabled-by-default five) + `request_as` = `{"type":"workspace"}` and the `--request-as-member` member variant; comment `author` default from `/v2/self` + `--author` override + `format: plaintext`; upsert `matching_attribute`), bearer injection, exit codes, plain + `--json` error envelopes, missing-token guard | none |
| **L2** | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real token> anycli attio -- whoami / object list / record search --query … / record create+update+delete round-trip / note create / task create` against the live API. Mandatory before tagging | **YES — test account pool (lane 2):** a real Attio workspace + an access token. Simplest source pre-OAuth-app: a workspace API key from the Attio settings (same bearer semantics as an OAuth token), else a token minted from the lane-1 dev app |
| **L3** | local `go run ./cmd/provider-gen` + `--check` against the branch bundle (validation only, per §2 — expected red in CI until batch-end regen); helio-cli built with uncommitted `replace github.com/heliohq/anycli => <this worktree>`; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service suite | none |
| **L4** | singleton (`env: dev`), `POST /internal/test-only/connections/seed` with `provider: "attio"`, real ids, and `access_token` **only** (no `refresh_token`/`expires_at` — non-expiring token class, no refresh path to exercise); then `heliox tool attio -- whoami` + one record read reaching the live API | **YES — lane 1:** dev OAuth app registered at build.attio.com; a real access token minted from it (uncommitted local `config/cloud.yaml` entries for client id/secret) |
| **L5** | `heliox tool attio auth` → connect link → real Attio consent at app.attio.com → `oauth_connected` system event on the originating channel → one unseeded live command. Human-in-the-loop (oauth lane, lane 3), after batch-end merge + lane-1 config landing; gates the visible flip | **YES:** lane-1 prod-config landing (`config/` + `deploy/` Secret) and a human consent session on the pool workspace |

**Definition of done** tracks the master plan: L1–L5 green, docs published, icon registered, then `visible: true` + regenerate as the single go-live change.

## 6. Divergence log

**Catalog/audit divergences:** none. Official docs confirm the catalog lane (`oauth_light`: self-serve registration, no review program), the audit's endpoints (authorize `app.attio.com/authorize`, token `app.attio.com/oauth/token`), and the standard_oauth fit. The only nuance worth recording: Attio issues **no refresh tokens** and sends **no scope query parameter** — both are properties the Notion bundle already models (`refresh_lease: none`, dashboard-configured scopes as `display_scopes`), so no schema or adapter work follows.

**Rev 2 spec corrections (design-internal, not catalog divergences)** — review findings verified against official docs and fixed in §1/§2/§5:
1. **Update verb mapping was inverted.** `PATCH /v2/objects/{object}/records/{record_id}` is the *append-multiselect* variant; `PUT` at the same path is the *overwrite/remove* variant (same duality on `/v2/lists/{list}/entries/{entry_id}`). Rev 1 based `update` on PATCH with a no-op `--append`, leaving overwrite unreachable. Now: default PUT (overwrite), `--append` → PATCH, for both `record update` and `entry update`; PUT-with-id added to the §1 surface table.
2. **Search required fields were missing.** `POST /v2/objects/records/search` requires `objects` (min 1) and `request_as` in addition to `query`. Now: `--objects` defaults (see Rev 3 below) and `request_as` defaults to `{"type":"workspace"}` with `--request-as-member` override.

**Rev 3 spec correction (design-internal)** — one review finding verified against official docs and fixed in §2/§5:
1. **Search default objects were wrong and would error.** Rev 2 defaulted `--objects` to all five standard objects (`people,companies,deals,users,workspaces`) and enshrined that in the L1 contract. But per Attio's data model only `people` and `companies` are enabled by default; `deals`, `users`, and `workspaces` are optional (disabled until an admin activates them, and a Free-plan workspace caps at 3 objects total), and the search endpoint validates slugs — returning `400 invalid_request_error` / `value_not_found` for any object not present. So the natural first call `attio record search --query "…"` (no `--objects`) would have errored on any default-onboarded or Free-plan workspace. Now: `--objects` defaults to the two always-present objects `people,companies` (also the dominant search intent); broader/custom search is opt-in via explicit `--objects` (discoverable via `attio object list`). Dynamic resolution from `GET /v2/objects` was rejected for the default path (extra hot-path round-trip + silent cross-object fan-out). L1 asserts the two-object default; L2 exercises the `value_not_found` error on a disabled slug.
3. **Comment author was unspecified.** `POST /v2/comments` requires `author` (`{type:"workspace-member", id}`) and `format` (only `plaintext`) in every variant. Now: author defaults to `authorized_by_workspace_member_id` from `GET /v2/self`, `--author` overrides, fail-fast when unresolvable.
