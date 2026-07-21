# Tool design: Attio (`attio`)

**Catalog row:** #18 · Attio · anycli id `attio` · provider key `attio` · lane `oauth_light` · Wave 1 · CRM.
**Branches:** anycli `tool/attio` (this worktree), Helio `tool/attio`.
**Status of catalog assumptions after independent verification against official docs:** all confirmed; no divergences. Details and citations below.

## 1. What an AI teammate does with Attio, and the API surface that serves it

Attio is a data-model-first CRM: a workspace holds **objects** (standard: `people`, `companies`, `deals`, `users`, `workspaces`; plus custom objects), each object holds **records**, and **lists** overlay records as pipelines/views with per-list **entries** and list-scoped attributes. Collaboration artifacts (**notes**, **tasks**, **comments/threads**) attach to records and entries.

An AI teammate's real jobs: "find the record for Acme / jane@acme.com", "what's the state of this deal", "log a note after the call", "create a follow-up task", "move this deal's stage / update an attribute", "add this new lead as a person + company and put them on our pipeline list", "answer questions across the pipeline (query with filters)". Because Attio schemas are per-workspace (custom objects, custom attributes, select/status options), the teammate also needs **schema introspection** to construct valid payloads — hardcoding attribute slugs would break on real workspaces.

**Wrapped surface** (REST API v2, base `https://api.attio.com`, `Authorization: Bearer <token>`, per https://docs.attio.com/llms.txt endpoint reference):

| Group | Endpoints | Why |
|---|---|---|
| Meta | `GET /v2/self` | whoami / token+workspace identity; also the bundle's identity endpoint |
| Objects | `GET /v2/objects`, `GET /v2/objects/{object}` | discover object slugs (incl. custom) before any record op |
| Attributes | `GET /v2/{target}/{id}/attributes`, `.../attributes/{attribute}`, `.../options`, `.../statuses` (read-only) | discover attribute slugs, select options, deal/status stages — prerequisite for correct writes |
| Records | `POST /v2/objects/records/search` (fuzzy, beta), `POST /v2/objects/{object}/records/query` (filter/sort), `GET/PATCH/DELETE /v2/objects/{object}/records/{record_id}`, `POST /v2/objects/{object}/records` (create), `PUT /v2/objects/{object}/records` (assert/upsert by matching attribute) | the core CRM read/write loop |
| Lists & entries | `GET /v2/lists`, `GET /v2/lists/{list}`, `POST /v2/lists/{list}/entries/query`, `POST /v2/lists/{list}/entries` (create), `PUT` assert-by-parent, `PATCH/DELETE /v2/lists/{list}/entries/{entry_id}` | pipeline work: add to list, change stage, query a pipeline |
| Notes | `GET /v2/notes` (filter by record), `GET/DELETE /v2/notes/{note_id}`, `POST /v2/notes` (markdown/plaintext) | meeting/call logging — a primary assistant write path |
| Tasks | `GET /v2/tasks`, `POST /v2/tasks`, `GET/PATCH/DELETE /v2/tasks/{task_id}` | follow-ups with due dates, assignees, linked records |
| Comments & threads | `GET /v2/threads`, `GET /v2/threads/{thread_id}`, `POST /v2/comments`, `GET/DELETE /v2/comments/{comment_id}` | participate in record discussions |
| Workspace members | `GET /v2/workspace_members`, `GET /v2/workspace_members/{id}` | resolve assignee/actor ids for tasks, notes, `request_as` |

**Deliberately out of scope (v1):** webhooks (runtime has no callback surface; presence-over-polling assistants don't need them for CLI use), meetings/call-recordings (beta, gated by separate `meeting`/`call_recording` scopes, not core CRM), files (upload lifecycle adds little for v1), attribute/object **write** endpoints (schema mutation is an admin act, not a teammate act — read-only introspection is enough and safer), list/object create-update (same reasoning).

**Pagination** (verified: https://docs.attio.com/rest-api/guides/pagination.md): `limit`/`offset` — query params on GET, JSON body fields on POST query endpoints; some newer endpoints (meetings) use cursors, but none we wrap. Surface `--limit`/`--offset` verbatim; do not auto-paginate. Search caps `limit` at 25.

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
attio record   search --query <q> [--objects people,companies] [--limit N]
attio record   query  <object> [--filter <json>] [--sort <json>] [--limit N] [--offset N]
attio record   get|delete <object> <record_id>
attio record   create <object> --values <json>
attio record   update <object> <record_id> --values <json> [--append]   # PATCH; --append maps to the append-multiselect variant
attio record   upsert <object> --values <json> --match <attribute>      # PUT assert
attio list     list | get <list>
attio entry    query <list> [--filter <json>] [--limit N] [--offset N]
attio entry    add <list> --parent-record <id> --parent-object <o> [--values <json>]
attio entry    get|update|remove <list> <entry_id>
attio note     list [--record <object>:<id>] | get <note_id> | create --parent <object>:<id> --title <t> (--markdown <md> | --plaintext <txt>) | delete <note_id>
attio task     list | get <task_id> | create --content <txt> [--deadline <iso>] [--assignee <member_id>] [--record <object>:<id>] | update <task_id> | delete <task_id>
attio thread   list [--record <object>:<id>] | get <thread_id>
attio comment  create (--thread <id> | --record <object>:<id>) --content <txt> | get <id> | delete <id>
attio member   list | get <member_id>
```

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
| **L1** | anycli `go test ./...`: `internal/tools/attio/` unit tests against httptest fakes — request shapes for every subcommand (incl. POST-body vs query-param pagination, `--append` → append-variant path, upsert `matching_attribute`), bearer injection, exit codes, plain + `--json` error envelopes, missing-token guard | none |
| **L2** | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real token> anycli attio -- whoami / object list / record search --query … / record create+update+delete round-trip / note create / task create` against the live API. Mandatory before tagging | **YES — test account pool (lane 2):** a real Attio workspace + an access token. Simplest source pre-OAuth-app: a workspace API key from the Attio settings (same bearer semantics as an OAuth token), else a token minted from the lane-1 dev app |
| **L3** | local `go run ./cmd/provider-gen` + `--check` against the branch bundle (validation only, per §2 — expected red in CI until batch-end regen); helio-cli built with uncommitted `replace github.com/heliohq/anycli => <this worktree>`; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service suite | none |
| **L4** | singleton (`env: dev`), `POST /internal/test-only/connections/seed` with `provider: "attio"`, real ids, and `access_token` **only** (no `refresh_token`/`expires_at` — non-expiring token class, no refresh path to exercise); then `heliox tool attio -- whoami` + one record read reaching the live API | **YES — lane 1:** dev OAuth app registered at build.attio.com; a real access token minted from it (uncommitted local `config/cloud.yaml` entries for client id/secret) |
| **L5** | `heliox tool attio auth` → connect link → real Attio consent at app.attio.com → `oauth_connected` system event on the originating channel → one unseeded live command. Human-in-the-loop (oauth lane, lane 3), after batch-end merge + lane-1 config landing; gates the visible flip | **YES:** lane-1 prod-config landing (`config/` + `deploy/` Secret) and a human consent session on the pool workspace |

**Definition of done** tracks the master plan: L1–L5 green, docs published, icon registered, then `visible: true` + regenerate as the single go-live change.

## 6. Divergence log

None. Official docs confirm the catalog lane (`oauth_light`: self-serve registration, no review program), the audit's endpoints (authorize `app.attio.com/authorize`, token `app.attio.com/oauth/token`), and the standard_oauth fit. The only nuance worth recording: Attio issues **no refresh tokens** and sends **no scope query parameter** — both are properties the Notion bundle already models (`refresh_lease: none`, dashboard-configured scopes as `display_scopes`), so no schema or adapter work follows.
