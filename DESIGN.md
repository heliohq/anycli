# Tool design: Front (`front`)

Scratch design doc for the `tool/front` batch branch (both repos). The batch
lead strips this at batch end. Catalog row 23: **Front | `front` | `front` |
oauth_review | wave 1 | Support**.

## 0. Naming axes (all three identical — no divergence)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `front` | bundle `tool.name` (flat, no `group`) |
| ② anycli tool id | `front` | `definitions/tools/front.json` |
| ③ provider catalog key | `front` | `integrations/providers/front/` dir + `key:` |

id == key, so **no `toolToProvider` entry** is added in
`helio-cli/internal/toolcred/resolver.go` (identity mapping via
`ProviderFor`/`ToolFor` already covers it). Go package: `front`
(`internal/tools/front/`), `RegisterService("front", …)`.

## 1. What this tool wraps and why

Front is a shared-inbox / customer-communication platform: teams triage
customer conversations (email, SMS, chat, social) from shared inboxes,
assign/tag/snooze them, reply, and coordinate internally with private
comments. An AI teammate on Front does exactly what a human agent does:
reads the queue, decides what matters, drafts or sends replies, leaves an
internal comment for a colleague, and tags/assigns/archives.

Official API surface wrapped: **Front Core API**, base
`https://api2.frontapp.com`, JSON over HTTPS, `Authorization: Bearer <token>`.
Docs: <https://dev.frontapp.com/reference/introduction>. The endpoints are
chosen to cover the agent triage loop, nothing speculative:

| Resource / verb | Endpoint | Why an AI teammate needs it |
|---|---|---|
| conversation list | `GET /conversations` (also `GET /inboxes/{id}/conversations`) | read the queue |
| conversation search | `GET /conversations/search/{query}` | find by keyword/contact/status |
| conversation get | `GET /conversations/{id}` | read one conversation's metadata |
| conversation update | `PATCH /conversations/{id}` | status change: archive/reopen (`open`/`archived`), snooze, mark spam |
| conversation assign | `PUT /conversations/{id}/assignee` | assign to a teammate / unassign |
| conversation messages | `GET /conversations/{id}/messages` | read the thread |
| conversation comments | `GET /conversations/{id}/comments` | read internal discussion |
| tag / untag conversation | `POST` / `DELETE /conversations/{id}/tags` | triage labelling |
| reply (send message) | `POST /conversations/{id}/messages` | send an outbound reply into an existing conversation |
| create draft reply | `POST /conversations/{id}/drafts` | draft a reply for a human to review/send (safer default than send) |
| add comment | `POST /conversations/{id}/comments` | internal note, @mention a teammate |
| contact list/get/create | `GET/POST /contacts`, `GET /contacts/{id}` | look up / create the customer record |
| inbox list | `GET /inboxes` | discover which inboxes exist |
| teammate list | `GET /teammates` | resolve assignee targets |
| tag list | `GET /tags` | resolve tag ids for tagging |
| token identity | `GET /me` | connection identity (see §4) + a debug `front me` |

Deliberately out of scope for v1 (can grow later, not needed for the triage
loop): channel CRUD, inbox/channel creation, message import, rules/analytics,
knowledge base, teammate/tag mutation, contact-group management, attachments
upload. New *outbound* conversations (not replies) require a `channel_id`
(`POST /channels/{channel_id}/messages`) and are held back — the AI-teammate
default is replying within an existing conversation, and drafting rather than
auto-sending.

## 2. anycli definition

**Type: `service`** (stage-1 rubric). No official Front CLI exists; the only
integration path is the HTTP Core API. So HTTP logic lives in
`internal/tools/front/`, following the `internal/tools/notion/` reference
shape: a cobra tree grouped by resource, a `BaseURL`/`HC`/`Out`/`Err` struct
so unit tests point at an `httptest.Server` and capture output, and the
documented exit-code contract (0 success; 1 runtime/API failure via a typed
`apiError`; 2 usage/parse errors) with a `--json` structured error envelope.

Command tree (verbs chosen to mirror the endpoint table):

```
front conversation list   [--inbox <id>] [--q <search>] [--status open|archived] [--limit N] [--page-token <cursor>]
front conversation get     --id <cnv_id>
front conversation update   --id <cnv_id> [--status open|archived|deleted] [--assignee <teammate_id|null>] [--tag-add <id>...] [--tag-remove <id>...]
front conversation messages --id <cnv_id> [--limit N] [--page-token <cursor>]
front conversation comments --id <cnv_id>
front message send   --conversation <cnv_id> --body <html/text> [--author <channel_id>] [--text]
front draft create   --conversation <cnv_id> --body <text> [--author <channel_id>]
front comment add    --conversation <cnv_id> --body <text>
front contact list   [--q <query>] [--limit N] [--page-token <cursor>]
front contact get    --id <cnt_id|alt:email>
front contact create --name <n> --handle <email/phone> --source email|phone|...
front inbox list
front teammate list
front tag list
front me
```

`conversation update` collapses the status/assignee/tag endpoints behind one
verb but issues the distinct underlying calls (`PATCH /conversations/{id}`,
`PUT …/assignee`, `POST`/`DELETE …/tags`) so the agent sees one intent.

**JSON output shape.** Every command emits a provider-neutral envelope on
stdout (never the raw Front body), matching the other service tools:

```json
{ "data": [ … normalized items … ], "next_page_token": "<cursor|empty>" }
```

Front paginates with a cursor exposed as `_pagination.next` (an absolute URL)
plus `?limit=`; the service extracts the opaque cursor and re-exposes it as
`next_page_token`, accepted back via `--page-token`, so agents never handle
Front URLs. Single-object commands emit `{ "data": { … } }`. Errors go to
stderr as `{ "error": { "code": …, "message": …, "status": <http> } }` and set
exit 1.

**Auth binding** (definition `auth.credentials`): one binding, source field
`access_token` → inject as env `FRONT_TOKEN`; the service reads `FRONT_TOKEN`
and sends `Authorization: Bearer $FRONT_TOKEN`. This is token-shape-agnostic:
the same bearer works whether it is an OAuth access token (production/token
gateway) or a Settings→Developers **API token** (handy for L2 harness runs).

```json
{
  "name": "front",
  "type": "service",
  "description": "Front shared inbox (conversations, replies, comments, contacts)",
  "auth": {
    "credentials": [
      { "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "FRONT_TOKEN"} }
    ]
  }
}
```

Unit tests (TDD, L1): build an `httptest.Server` Front fake; assert request
method/path/query, the injected `Authorization: Bearer` header, request-body
shape for send/draft/comment/create, cursor extraction, and both plain-text
and `--json` error rendering. Never hit the real API from a unit test.

## 3. Credential fields & OAuth flow (oauth_review lane — verified)

**Registration model (matches the audit, verified against official docs).**
App creation is self-serve: **Settings → Company → Developers → Create app**,
then the **OAuth** feature tab supplies Client ID, Client Secret, redirect
URLs. *"By default, apps are private to the Front instance they are created
in."* Making the app available to **all** Front customers requires publishing
to the App Store via the **"Partnering with Front"** submission/review track —
that publisher review is what puts Front in **oauth_review**. Consistent with
the rollout plan's lane semantics, review clearance gates **only the visible
flip**; dev/L4/L5 run against a private (dev-instance) app with no review.
(Docs: <https://dev.frontapp.com/docs/create-and-manage-apps>,
<https://dev.frontapp.com/docs/oauth>.)

**Admin constraint to record.** *"End users authorizing OAuth apps must be
admins. This applies to both private and public OAuth apps."* So the L5
consent (and any real connect) must be performed by a Front **company admin**.

**Flow (authorization-code, no PKCE, HTTP Basic client auth):**
- authorize: `https://app.frontapp.com/oauth/authorize`
  params `response_type=code`, `client_id`, `redirect_uri`, `state`.
  **No `scope` param** — Front does not take scopes on the authorize request;
  the app's capabilities (features / namespaces `Global|Shared|Private` /
  permissions `Read|Write|Delete|Send`) are configured on the app itself in
  the Front console, not requested per-authorize. This is the one shape
  divergence from a textbook OAuth provider and it is fine: the bundle simply
  omits `oauth.scopes` and uses `display_scopes` for UI copy only.
- token exchange: `POST https://app.frontapp.com/oauth/token`, **HTTP Basic**
  auth `Authorization: Basic base64(client_id:client_secret)`, form body
  `grant_type=authorization_code`, `code`, `redirect_uri`. → maps to
  `token_exchange_style: form_basic`, `pkce: none`.
- refresh: same endpoint, Basic auth, `grant_type=refresh_token`,
  `refresh_token`.
- token semantics: **access token expires in 60 min** (401 when expired);
  **refresh token valid 6 months**; the *same* refresh token is returned for
  most of the window, and **a new refresh token is returned in the last 24h**
  of validity (rotation that resets the 6-month clock). Response fields:
  `access_token`, `refresh_token`, `expires_at`, `token_type=Bearer`.

**Refresh handling / capability check.** `standard_oauth`'s refresh path
already writes back whatever `refresh_token` the token response carries, so
Front's lazy rotation (new token only in the last 24h) needs **no capability
growth** — `refresh_lease: none`. The 60-min expiry means every cached token
naturally exercises the gateway's refresh-and-write-back path (A3).

**Disconnect.** Front documents **no token-revocation endpoint** (the only
introspection URL in the docs is for channel identification, not revoke). So
`disconnect_mode: local_only` (Helio drops its stored credential; the app
authorization is removed by the admin inside Front) — mirroring `notion`, not
gmail's `provider_revoke`. No `revoke:` block in the bundle.

**Identity.** The token response carries no identity fields, but
`GET https://api2.frontapp.com/me` returns the **Front company** the token is
scoped to (`{ "id": …, "name": … }`) — OAuth here is company-scoped, not
per-teammate. So identity is resolved via a userinfo GET, not
`token_response`.

## 4. Helio provider bundle (`integrations/providers/front/`, hidden-first)

`standard_oauth`, zero service-side Go (no adapter). Bundle sketch:

```yaml
schema: helio.provider/v1
key: front
go_name: Front

presentation:
  name: Front
  description_key: front
  consent_domain: app.frontapp.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: assistant          # token is company-scoped (not an individual person)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.frontapp.com/oauth/authorize
    token_url: https://app.frontapp.com/oauth/token
    token_exchange_style: form_basic     # POST form body + Basic client auth
    pkce: none
    # no `scopes:` — Front authorize takes no scope param (app-configured perms)
    display_scopes: [read, write, send]  # UI copy only
    single_active_token: false
    refresh_lease: none                  # lazy 6-month rotation; write-back handles it

identity:
  source: userinfo
  url: https://api2.frontapp.com/me
  stable_key: /id
  label_candidates: [/name, /id]

connection:
  mode: isolated
  disconnect_mode: local_only            # no revoke endpoint
  runtime_strategy: standard_oauth

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: front
  kind: oauth
```

Axis naming (§0): `tool.name: front`, dir/`key: front`, no `group`, no
resolver entry.

**Config landing (human lane 1).** `oauth.client_id` / `oauth.client_secret`
land in integration-service config — `config/` locally *and* the Helm Secret
in `deploy/` **together** (Config Sync hard rule; a *partially* configured
provider fails service startup, so both fields land in one change). Absent
both → `configured: false` (Connect disabled), safe to ship hidden.

**Other artifacts (batch-end shared surfaces):**
- UI icon `ui/helio-app/src/integrations/icons/front.svg` + register in
  `providerIcons.ts` (manual, never generated).
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` (front verbs +
  the "draft, don't auto-send by default" guidance), plugin version bump +
  marketplace publish.
- `provider-gen` five projections regenerate together at batch end; the tool
  branch is *expected* to fail `provider-gen --check` in CI until then (do not
  commit local regens).

**No capability growth needed.** `form_basic` + `pkce: none` +
`refresh_lease: none` + `identity.source: userinfo` + `disconnect_mode:
local_only` are all existing `standard_oauth` enum values. If a review of the
current enum set shows `form_basic` is already exercised by another wave-1
provider, this is pure config; if not, it is at most one reviewed enum value,
not an adapter. **No `service/adapter_front.go`** — Front's response shapes
and lifecycle sit inside the closed `standard_oauth` capability set.

## 5. Test plan — five layers

| Layer | What runs for Front | Ext. creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in anycli: httptest Front fake; assert path/query/verb, `Bearer` injection, send/draft/comment/create body shapes, cursor→`next_page_token`, `--json` error envelope, exit codes | **No** |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<front token> anycli front -- conversation list --limit 3` etc. against `api2.frontapp.com`. Fastest with a Settings→Developers **API token** (bearer, no OAuth dance); token is company-scoped like the OAuth token, so it validates field names + request shapes identically | **Yes** — a Front account + API token |
| **L3** gen + suites | `provider-gen` then `provider-gen --check` locally on-branch; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with a local uncommitted `replace` → anycli branch; integration-service unit suite | **No** |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider":"front"`, seeding **both** `access_token` and `refresh_token` from the dev app with a short `expires_at` (60-min real expiry ⇒ next call forces the gateway refresh-and-write-back path), then `heliox tool front -- conversation list` returns real data through the token gateway | **Yes** — dev-app OAuth tokens + Front account (lane 1 output) |
| **L5** full connect | `heliox tool front auth` → consent on the **private/dev** Front app as a **company admin** → `oauth_connected` event fires → unseeded `heliox tool front -- conversation list` succeeds. Run once while still hidden, before the visible flip | **Yes** — dev-app client id/secret + **admin** Front account |

**Externally-supplied credentials required:** L2, L4, L5. L1 and L3 are
self-contained.

**Rollout / visible flip.** oauth_review: after L1–L5 pass hidden, the visible
flip (`presentation.visible: true` + regenerate) additionally waits on **App
Store / "Partnering with Front" review clearance** — which gates only the flip,
never dev, L4, or the batch-end merge. L5 itself runs against the private app
and does not wait on that review.

## 6. Divergences from the prompt/catalog

None. Official docs confirm the catalog's **oauth_review** lane (self-serve
private app; App Store publisher review gates all-customer availability) and
the audit verdict. The only noteworthy API-shape facts — Front's authorize
takes **no `scope` param** (app-configured permissions) and Front exposes **no
revoke endpoint** — are handled within `standard_oauth` config
(`display_scopes` only; `disconnect_mode: local_only`) and require no adapter.
