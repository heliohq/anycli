# Tool design — Typefully (`typefully`)

Scratch design doc for the `tool/typefully` batch branch. The batch lead strips
this at batch-end. Scope: one external tool provider behind `heliox tool`, per
the `helio-tool-provider` pipeline. Catalog row 207: id `typefully`, key
`typefully`, auth lane `api_key`, Wave 3, Social & Media.

## 0. Naming axes (SKILL.md §"three naming axes" / master plan §3)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `typefully` (flat, not grouped) | bundle `tool.command` (omitted — equals name) |
| ② anycli tool id | `typefully` | `definitions/tools/typefully.json` + `RegisterService` |
| ③ provider catalog key | `typefully` | `integrations/providers/typefully/` dir + `key:` |

All three are the identical single token → **no `toolToProvider` divergence
entry** in `helio-cli/internal/toolcred/resolver.go` (identity mapping holds, as
for `slack`/`gmail`/`instantly`). Go package name is `typefully` (no dashes, no
leading digit) → `internal/tools/typefully/`.

## 1. What an AI teammate does with Typefully → which API surface

Typefully is a social-writing/scheduling tool (X, LinkedIn, Mastodon, Threads,
Bluesky, Substack). A Helio teammate's real jobs, in priority order:

1. **Draft and schedule/publish posts & threads** — "draft a thread about X and
   schedule it for tomorrow 9am", "publish this now", "add it to the next free
   queue slot". This is the core, highest-frequency use.
2. **Review what's drafted/queued/published** — "what's scheduled this week",
   "did the launch thread go out", "list my draft ideas".
3. **Light housekeeping** — reschedule/edit/delete a draft, tag drafts,
   inspect the posting schedule, pull basic X post/follower analytics.

This drives wrapping the **Typefully v2 REST API** (`https://api.typefully.com/v2`),
verified against the official reference (https://typefully.com/docs/api) and the
Help Center article (https://support.typefully.com/en/articles/8718287-typefully-api).

### Version decision — v2, not v1 (divergence from the catalog's implicit v1)

The 2026-07-21 OAuth audit row (209) and older third-party integrations (Zapier,
Pipedream, Nango) describe a **v1** surface: base `https://api.typefully.com/v1`,
header **`x-api-key: Bearer <key>`** (the header value itself carried the
`Bearer` prefix), endpoints `/drafts/create/`, `/drafts/recently-published/`,
`/drafts/recently-scheduled/`, `/notifications/`. Per the official v1→v2
migration guide (https://support.typefully.com/en/articles/13133296):

- **Creating new v1 API keys is disabled.** Any key a Helio user generates today
  is a **v2** key.
- v1 endpoints keep working only until **15 June 2026** (past by the time this
  ships in a 2026-H2 wave).
- v2 (released Dec 2025) **renames** the auth header from `x-api-key` to
  `Authorization` while keeping the `Bearer` scheme (v1 `x-api-key: Bearer <key>`
  → v2 **`Authorization: Bearer <key>`**), is **social-set-scoped**, and replaces
  the `threadify` flag with explicit `posts` arrays. v1 keys cannot be used with
  v2.

**Recorded divergence:** we build against **v2 + `Authorization: Bearer`**, not
the v1 shape any stale reference implies. This does **not** change the audit's
lane verdict — see §4. If the batch's stage-1 recheck finds a Helio user still on
a live v1 key before the cutover, that is a transitional edge, not a reason to
target the sunsetting surface.

### Endpoints wrapped (v2) and why

Everything is scoped under a **social set** (`{social_set_id}`), so identity /
target discovery comes first, then drafts (the point), then a thin support tail.

| Group | Endpoint | Why (teammate task) |
|---|---|---|
| identity | `GET /v2/me` | Connect-time verify + "who am I" (see §4). |
| social-set | `GET /v2/social-sets` | Discover the `social_set_id` every other call needs; list connected accounts. |
| social-set | `GET /v2/social-sets/{id}/` | Which platforms are connected, publishing quota. |
| draft | `POST /v2/social-sets/{id}/drafts` | **Create + schedule + publish** (one endpoint). `platforms` body; `publish_at` = `"now"` \| `"next-free-slot"` \| ISO-8601. |
| draft | `GET /v2/social-sets/{id}/drafts` | List/filter (`status`, `tag`, `order_by`, `limit`, `offset`) — the "what's queued/published" job. |
| draft | `GET /v2/social-sets/{id}/drafts/{draft_id}` | Read one draft's full content + `status`/`publish_state`. |
| draft | `PATCH /v2/social-sets/{id}/drafts/{draft_id}` | Edit / reschedule / publish an existing draft. |
| draft | `DELETE /v2/social-sets/{id}/drafts/{draft_id}` | Remove a draft (204). |
| tag | `GET` / `POST /v2/social-sets/{id}/tags` | List/create tags used to filter drafts. |
| queue | `GET /v2/social-sets/{id}/queue` (`start_date`,`end_date`, ≤62d) | "Show me the posting queue." |
| queue | `GET` / `PUT /v2/social-sets/{id}/queue/schedule` | Inspect / replace the recurring slot schedule (PUT needs ADMIN). |
| analytics | `GET /v2/social-sets/{id}/analytics/{platform}/posts` \| `/followers` | Basic X post/follower metrics (X only). |
| media | `POST /v2/social-sets/{id}/media/upload` + `GET .../media/{media_id}` | Attach an image: presigned-PUT upload then poll status, referenced by `media_id` in a draft. |
| linkedin | `GET /v2/social-sets/{id}/linkedin/organizations/resolve` | Resolve an org URL to a mention when drafting LinkedIn company posts. |
| comments | `GET /v2/social-sets/{id}/drafts/{draft_id}/comment-threads` | Read reviewer comments on a draft. |

**Deliberately out of scope for v1:** the v1-only `/notifications/` endpoint
(inbox/activity feed). v2 has **no** notifications endpoint — it moved to
outbound webhooks (Draft Created/Published/Scheduled/…), which are a
push-to-a-URL mechanism, not something a stateless passthrough CLI polls. A
Helio teammate gets the same signal by listing drafts with `status`/`publish_state`
filters, so we do not wrap notifications or webhook management. Recorded so the
next reader does not "restore" a v1 parity that v2 dropped.

### Async publish nuance (must surface to the agent)

`publish_at: "now"` returns `201` immediately but publishes **asynchronously**:
the response `publish_state` starts non-terminal (`null` → `in_progress` →
`finished`) and the caller polls `GET .../drafts/{draft_id}` until
`publish_state == "finished"`. The service does **not** hide this behind a
blocking wait (would fight the non-interactive, one-call-one-result contract);
the AI-facing doc (§5) tells the agent to poll.

**`finished` is not success.** The official v2 reference is explicit:
`publish_state == "finished"` means the job is *done*, not that it *succeeded*.
`publish_state` is a distinct axis from `status` — `status` does not flip to
`publishing` while an immediate publish is in flight. So after `finished`, the
agent must read the draft's **`status`** (a `published` vs. `error` outcome) and
the **per-platform published URLs** (`x_published_url`, `linkedin_published_url`,
`mastodon_published_url`, …; `null` on a platform that did not post) to
distinguish full success from a partial/failed publish, and report the real
outcome — *which* platform(s) failed — rather than treating `finished` as
success. Reporting a silently-failed X/LinkedIn post as published would violate
Helio's hard rule against silent-success paths (fail fast, surface the real
cause). The service already returns `status`/`publish_state` from
`GET .../drafts/{draft_id}` and filters lists by `status`, so this is a
doc/guidance fix, not a new API surface. §5 carries it into the shipped sub-doc.

## 2. anycli definition (stage 1–2)

**Tool form: `service` type.** No official Typefully CLI binary exists (fails the
`cli`-type rubric in SKILL.md stage 1: no official agent-friendly binary to
provision). Implement as a built-in HTTP service under
`internal/tools/typefully/` against the v2 REST API — matching 21/23 existing
definitions and the near-identical `instantly` precedent.

### Definition JSON — `definitions/tools/typefully.json`

Single credential field, injected as an env var (mirrors `instantly.json` /
`bitly.json`):

```json
{
  "name": "typefully",
  "type": "service",
  "description": "Typefully social-writing tool (drafts, scheduling/publishing, queue, tags, X analytics)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "TYPEFULLY_API_KEY"}
      }
    ]
  }
}
```

The service reads `TYPEFULLY_API_KEY` and sends `Authorization: Bearer <key>` on
every request (matching `instantly`'s Bearer scheme). Registered in
`internal/tools/register.go` via `RegisterService("typefully", …)` — the one
shared-registry line that lands at batch-end, not mid-batch.

### Command tree (verbs)

Cobra tree, non-interactive, every command `--json` (raw provider JSON
passthrough, snake_case, matching official docs 1:1 — the built-in service
convention). The social set is a required, explicit input on scoped commands.

```
typefully
  me                    GET  /v2/me
  social-set
    list                GET  /v2/social-sets                       [--limit --offset]
    get                 GET  /v2/social-sets/{id}/                  --social-set <id>
  draft
    list                GET  .../drafts        --social-set <id> [--status --tag --order-by --limit --offset]
    get                 GET  .../drafts/{id}   --social-set <id> --id <draft> [--exclude-comment-markers]
    create              POST .../drafts        --social-set <id> --data '<json>'   (or typed flags, see below)
    update              PATCH .../drafts/{id}  --social-set <id> --id <draft> --data '<json>'
    delete              DELETE .../drafts/{id} --social-set <id> --id <draft>
  tag
    list                GET  .../tags          --social-set <id>
    create              POST .../tags          --social-set <id> --name <name>
  queue
    view                GET  .../queue         --social-set <id> --start-date --end-date
    schedule-get        GET  .../queue/schedule --social-set <id>
    schedule-set        PUT  .../queue/schedule --social-set <id> --data '<json>'
  analytics
    posts               GET  .../analytics/{platform}/posts     --social-set <id> --platform x --start-date --end-date [--include-replies --limit --offset]
    followers           GET  .../analytics/{platform}/followers  --social-set <id> --platform x [--start-date --end-date]
  media
    upload              POST .../media/upload  --social-set <id> --file <path>     (gets presigned URL, PUTs bytes, returns media_id)
    status              GET  .../media/{id}    --social-set <id> --id <media>
  linkedin
    resolve-org         GET  .../linkedin/organizations/resolve --social-set <id> --organization-url <url>
  comment
    threads             GET  .../drafts/{id}/comment-threads --social-set <id> --id <draft> [--platform --status --limit]
```

**Body strategy for `draft create`/`update` and `queue schedule-set`:** the
`platforms` and `rules` bodies are deeply nested and platform-specific; a
`--data '<raw json>'` passthrough (the `instantly campaign create` precedent) is
the honest, low-surface choice. Add **thin conveniences** on `draft create` for
the 80% path — `--text` (single post), repeatable `--text` or `--thread-file`
for a thread, `--platform x` (default), `--publish-at now|next-free-slot|<iso>`,
`--title`, repeatable `--tag` — which the service assembles into the `platforms`
body. Anything richer goes through `--data`. `--data` and typed flags are
mutually exclusive (usage error, exit 2).

### JSON output shape

- Success: provider JSON verbatim on stdout + newline; exit 0.
- List commands: pass through the `{results, count, limit, offset, next, previous}`
  envelope; `--limit`/`--offset` map to query params (defaults: `limit` 10, max
  50; analytics posts `limit` 25, max 100 — mirror provider defaults, do not
  re-cap).
- Errors: non-2xx → structured error on stderr (JSON under `--json`, else text),
  exit 1; usage/flag errors exit 2. `401`/`403` → `CredentialRejected`
  classification (see §4). `429` is a rate-limit runtime error (exit 1); surface
  the `X-RateLimit-*` reset info in the message, do not auto-retry.

## 3. Credential fields & auth flow

**Credential:** one field — the Typefully API key (a v2 key). Non-expiring,
user-scoped bearer token; carries the creating user's permissions.

- anycli `source.field`: `access_token` → env `TYPEFULLY_API_KEY`.
- Helio token payload: `token.access_token` → projected by the token gateway as
  the `access_token` anycli injects. No new `CredentialSource` kind (reuses the
  `manual_api_token` path, same as `instantly`).

**Registration model (verified against official docs):** user creates the key in
**Settings → API** (`https://typefully.com/?settings=api`). No OAuth app, no
client id/secret, no redirect URI, no review — the `api_key` lane. Enabling
**Development mode** there surfaces the social-set / draft / media IDs the agent
needs. No wire-level scopes to request (the key inherits the user's account
permissions), so no scope-grant step — simpler than `instantly` (which needs
`workspaces:read`).

**Auth flow (L5 key-entry path):** open connect link → paste key into the
connect drawer → integration-service verifies it against `GET /v2/me`
(`Authorization: Bearer`) via the write-only `POST /connections/credentials`
→ stored in Vault → connection shows connected/configured → runtime injects it.

## 4. Helio provider bundle plan (`integrations/providers/typefully/provider.yaml`)

`api_key` / `manual_api_token` bundle — the `instantly` precedent almost exactly,
with a **cleaner identity story** because `GET /v2/me` returns a stable
account-scoped id (instantly needed `workspaces:read`; Typefully needs nothing
extra). No integration-service Go adapter, no capability growth: the existing
`manual_api_token` runtime strategy + `api_key` Bearer-scheme connect-time
verifier + declarative userinfo identity resolver cover it. **Hidden-first**
(`presentation.visible: false`).

```yaml
schema: helio.provider/v1
key: typefully
go_name: Typefully

presentation:
  name: Typefully
  description_key: typefully
  consent_domain: typefully.com
  # Hidden-first (300-integrations master plan §Hidden-first rollout). Flip to
  # visible only after ALL of: the anycli typefully tool ships in the pinned
  # AnyCLI + heliox rebuild/runtime image; a reviewed brand icon lands in
  # helio-app; tools.desc.typefully ships in all locales; and the L5 key-entry
  # connect flow is verified end to end. Pick an unoccupied order when flipping.
  visible: false

# Typefully v2 uses user-scoped bearer API keys created in Settings -> API
# (https://typefully.com/?settings=api). No third-party OAuth exists (the 2026
# OAuth audit row 209 confirmed: no viable multi-tenant authorization-code
# flow), so this is an api_key / manual_api_token provider: the user pastes a
# key, verified against GET /v2/me before any Vault write.
auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    scheme: bearer            # send "Authorization: Bearer <key>" at verify time
    setup_url: https://typefully.com/?settings=api

# GET /v2/me takes no params, resolves the account from the key, and returns a
# stable string id plus label fields. Verifies the key at connect time.
identity:
  source: userinfo
  url: https://api.typefully.com/v2/me
  stable_key: /id
  label_candidates: [/api_key_label, /name, /email, /id]

connection:
  mode: isolated
  disconnect_mode: local_only   # no self-revoke API; user deletes the key in Settings
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: typefully
  kind: api-key
```

**Config (`required_config_fields`):** none. `api_key` bundles carry no
client id/secret, so there is **no** integration-service `config/` + `deploy/`
Secret append for this provider (the seventh shared surface / lane-1 landing does
not apply — that lane is oauth-only). Nothing to keep in Config-Sync here.

Supporting shared-surface edits (all batch-end): UI icon
`ui/helio-app/src/integrations/icons/typefully.svg` + `providerIcons.ts` append;
i18n `tools.desc.typefully` (+ any `description_key`) strings; AI sub-doc (§5);
the five `provider-gen` projections regenerated once by the batch lead.

## 5. AI-facing docs (stage 8)

`agents/plugins/heliox/skills/tool/typefully/typefully.md`, structured like
`instantly.md`: flat-provider preamble → **Connect** (key from Settings → API,
enable Development mode to see IDs, no scopes to pick) → **command groups** with
the two load-bearing gotchas spelled out:

1. **Discover the social set first** — call `social-set list`, take `/results/0/id`
   (or the one matching the user's account), pass it as `--social-set` to
   everything else.
2. **Publish-now is async, and `finished` ≠ success** — `draft create
   --publish-at now` returns `201` with a non-terminal `publish_state`; poll
   `draft get --id <draft>` until `publish_state == "finished"`. `finished` only
   means the job is done, **not** that it succeeded: then read the draft's
   `status` (`published` vs. `error`) and the per-platform published URLs
   (`x_published_url`, `linkedin_published_url`, …; `null` = that platform did
   not post) to tell full success from a partial/failed publish, and report
   which platform(s) failed instead of assuming success. `next-free-slot` and
   ISO datetimes schedule instead (no poll needed).

## 6. Test plan — five layers (SKILL.md §"Testing layers")

| Layer | What | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` green. httptest fake asserts: `me`/`social-set`/`draft` route to correct v2 paths; `Authorization: Bearer` header sent; `publish_at`/`status`/`limit`/`offset` map to the right body/query; `--data` vs typed-flag exclusivity → exit 2; 401/403 → CredentialRejected + exit 1; 429 → exit 1 rate-limit message; `--json` vs text error rendering; list envelope passthrough. | No (fakes) |
| **L2** harness real-API | `TYPEFULLY_API_KEY=… ./bin/anycli typefully -- me` and `-- social-set list`, then a real `draft create`/`get`/`delete` round-trip against a live Typefully account. Confirms v2 auth + paths + async publish behavior. | **Yes** — a real Typefully API key (v2), from the test-account pool (lane 2). Needs a paid tier if the account requires it for API/scheduling. |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (bundle projects into all five files, directory-key equality, HTTPS identity/setup URLs, reviewed `manual_api_token` strategy); helio-cli + integration-service unit suites green. `helio-cli/go.mod` local uncommitted `replace` → anycli branch; **no committed regen/pin** (batch-lead owns those). | No |
| **L4** singleton + seed | `make run-singleton` + `POST /internal/test-only/connections/seed` a Typefully key, then `heliox tool typefully -- me` / `-- social-set list` through the real token gateway (hidden tool runs as cobra-hidden). Success = the seeded key reaches the live v2 API and returns the account. | **Yes** — same real key seeded (bypasses connect UI). |
| **L5** full connect | One end-to-end **api_key key-entry** run (master plan §2 api_key L5 path): open connect link → paste key in the real connect UI → verified against `GET /v2/me` → connection shows connected in `GET /connections` → one **unseeded** live `typefully -- me` (or `social-set list`) through the token gateway succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. Gates the visible flip. | **Yes** — a real key pasted through the UI (account pool). |

**Credential-supplied layers:** L2, L4, L5 (all need one real Typefully v2 API
key from the account pool). L1 and L3 are hermetic. No OAuth app registration
(lane 1) is needed at all — `api_key` lane.

## 7. Rollout (stage 10)

Land hidden (bundle `visible: false`) in the Wave 3 batch; batch-end merge brings
the anycli tag + pin bump + one `provider-gen` run + `providerIcons.ts` +
plugin-docs publish. Run L1–L4 hidden, then the api_key L5 sweep; flip
`presentation.visible: true` + regenerate as the single go-live change once L5
passes and the icon/i18n/docs are in. No review-clearance gate (api_key lane).

## 8. Divergences recorded (independent-judgment checks)

- **Catalog/audit implied v1; official docs mandate v2.** New keys are v2-only;
  v1 sunsets 15 Jun 2026. We target v2 + `Authorization: Bearer` (not v1
  `X-API-KEY`). Audit's `api_key` **lane verdict stands** — v2 still offers no
  multi-tenant authorization-code OAuth (keys are user-scoped, self-serve, no
  app/redirect/review). §1, §3.
- **v2 dropped notifications.** The v1 `/notifications/` endpoint has no v2
  successor (webhooks replaced it); not wrapped. §1.
- **No resolver divergence, no config append.** id==key==`typefully`; api_key
  bundle carries no client id/secret → no `toolToProvider` entry and no
  integration-service config/deploy Secret work. §0, §4.
