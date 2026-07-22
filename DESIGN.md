# Tool design: YouTube (`youtube`)

Per-tool design for catalog row 57 (Wave 1, Social & Media). Scratch file on
branch `tool/youtube`; the batch lead strips it at batch end.

Master plan: anycli `docs/design/008-300-integrations-rollout-plan.md` (§2
execution model, §3 naming, row 57). Pipeline: Helio
`.claude/skills/helio-tool-provider/SKILL.md`. Audit:
`008-300-integrations-rollout-plan/oauth-audit.md`.

## 0. Naming (the three axes)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `youtube` — **flat command, no `tool.group`** | bundle `tool` (no `command`/`group`) |
| ② anycli tool id | `youtube` | `definitions/tools/youtube.json`, Go package `internal/tools/youtube/` |
| ③ provider catalog key | `youtube` | `integrations/providers/youtube/` |

- ② == ③ == `youtube`, so **no** `toolToProvider` entry in
  `helio-cli/internal/toolcred/resolver.go`. Nothing to add there.
- **Not** a member of the `google` command group (design 303). §3 of the master
  plan is explicit: "Independent brands under a corporate umbrella follow the
  gmail precedent — key == id, no prefix (`youtube`, `gemini`, `bigquery`,
  `firebase`)." The §3 `MUST declare tool.group: google` rule names only
  Analytics / Search Console / Ads (the Workspace-adjacent data tools); YouTube
  is deliberately in the standalone-brand bucket with gemini/bigquery/firebase,
  which ship as flat `heliox tool <brand>` commands. So `youtube` renders as
  `heliox tool youtube`, **not** `heliox tool google youtube`.
  - Note the one subtlety this resolves: the shipped `gmail` bundle *does* carry
    `group: google` (it is a Workspace app) even though its key == id. YouTube
    reuses gmail's *key-derivation* precedent (no `google_` prefix, key == id)
    but **not** its grouping — because YouTube is a consumer product outside the
    Workspace family, on its own scope/consent surface. This is a naming
    divergence from gmail's `tool.group`, recorded here on purpose.
- Go package name (§3 rule): `youtube` needs no dash-drop or digit-normalization
  — `internal/tools/youtube/`, `RegisterService("youtube", …)`.

## 1. Auth-lane verification against official sources

Catalog row 57 says `oauth_review`. The `oauth-audit.md` has **no YouTube row**
— that audit scoped only the 250 tools sitting in `api_key` before it; YouTube
was `oauth_review` in the seed catalog from the start (§4 note: "Google
Analytics, Google Ads, Search Console, and YouTube carry Google
sensitive/restricted scopes (review lane) even though Gemini (AI Studio key)
does not"). Independently verified against official docs:

- **Registration model:** one multi-tenant OAuth 2.0 app in the Google Cloud
  console — the *same* Google app family/registration lane 1 already runs for
  `gmail` / `google_calendar` / Search Console / Analytics. Self-serve client
  creation; dev/test-mode works pre-verification (test-user allowlist +
  "unverified app" warning + user cap), so lane-1 dev-app creation gates L4
  exactly as the plan describes.
- **Protocol:** standard Google OAuth 2.0 authorization-code flow. Authorize
  `https://accounts.google.com/o/oauth2/v2/auth`, token
  `https://oauth2.googleapis.com/token`, form-with-secret exchange, no PKCE
  (matches the whole shipped Google family). YouTube Data API v3 has **no**
  API-key path for private/user data — API keys work only for public read of
  public resources, which is not what an authenticated teammate tool does.
- **Scopes (verified, `developers.google.com/youtube/v3/guides/auth`):**
  - `…/auth/youtube.readonly` — read-only view of the user's account
    (**sensitive**).
  - `…/auth/youtube.force-ssl` — **broad**: see, edit, and permanently delete
    the user's videos, ratings, comments, and captions (**sensitive**). Superset
    of `youtube` and `youtube.readonly` for our surface.
  - `…/auth/youtube.upload` — upload video files (**sensitive**).
  - `…/auth/youtube` — manage account (largely superseded by force-ssl).
  - `…/auth/youtubepartner`, `…/auth/youtube.channel-memberships.creator` —
    partner/membership niches (not requested).
  - **This tool requests exactly `youtube.force-ssl`** (plus the `openid email
    profile` identity trio). force-ssl is a single scope that covers every read
    *and* write verb this tool ships (video-metadata edit, ratings, the full
    comment moderation loop, playlist curation, subscription reads), so
    least-privilege is one broad-but-necessary scope rather than
    readonly+force-ssl overlap. `youtube.upload` is **not** requested (see §2
    exclusions).
- **Review verdict:** `youtube.force-ssl` is a **sensitive** scope → Google
  OAuth app verification is required before arbitrary external accounts can
  grant it without the unverified-app warning and the 100-test-user cap.
  Sensitive, **not restricted** — no CASA third-party security assessment (the
  restricted list is Gmail/Drive-content/Fit/etc.), so this is the cheaper end
  of the review lane, same class as Search Console's `webmasters`. **→
  `oauth_review` confirmed.** Recorded alternative not chosen: a readonly-only
  variant (drop every write verb, request only `youtube.readonly`) — but
  `youtube.readonly` is *itself* sensitive, so it would **not** drop to
  `oauth_light`, and it would throw away the comment-moderation / playlist /
  metadata write loop that is the core teammate value. Keep force-ssl + review.
- **Token semantics:** standard Google — ~1 h access token; long-lived refresh
  token only when `access_type=offline` is on the authorize request;
  `prompt=consent` forces a fresh refresh token on reconnect; refresh tokens are
  **not** rotated on use (no single-active-token lease — the gateway retains the
  stored refresh token); revoke via `https://oauth2.googleapis.com/revoke`.
  Byte-identical to the shipped `gmail` bundle → `standard_oauth`, zero
  service-side adapter.

## 2. What an AI teammate does with YouTube → API surface

All verbs are on base `https://www.googleapis.com/youtube/v3`. YouTube Data API
v3 uses a mandatory `part` parameter on reads (which resource sections to hydrate
— `snippet`, `contentDetails`, `statistics`, `status`, `replies`, …); the
service defaults `part` per verb and lets `--part` override.

Driving use cases, in order of teammate value:

1. **Channel & audience context** — "how's our channel doing" (subscriber /
   view / video counts), "what did we publish recently".
2. **Research / discovery** — search videos & channels, pull a video's stats and
   metadata, list a channel's uploads.
3. **Community management** — read the comment threads on a video, reply to a
   comment, moderate (hold / publish / reject / mark-as-spam / delete) — the
   highest-frequency real teammate loop.
4. **Playlist curation** — list / create / update / delete playlists; add and
   remove videos.
5. **Video metadata management** — update a video's title / description / tags /
   category / privacy; like/dislike a video.
6. **Subscriptions** — list who the channel subscribes to.

Wrapped surface:

| Verb | Method + path | part / notes |
|---|---|---|
| channels get | `GET /channels` | `--mine` \| `--id` \| `--for-handle` \| `--for-username`; part `snippet,statistics,contentDetails,status` |
| search | `GET /search` | `--query`, `--type video\|channel\|playlist`, `--channel`, `--order`, `--published-after/-before`, `--region`, paging; **100-unit** cost |
| videos get | `GET /videos` | `--id` (comma list); part `snippet,statistics,contentDetails,status` |
| videos mine | `GET /search?forMine=true&type=video` | the assistant's own uploads |
| videos update | `PUT /videos?part=snippet,status` | `--id` + any of `--title/--description/--tags/--category-id/--privacy`; read-modify-write (fetch current snippet first, API replaces the whole part) |
| videos rate | `POST /videos/rate` | `--id --rating like\|dislike\|none`; empty 204 body |
| playlists list | `GET /playlists` | `--mine` \| `--channel`; part `snippet,contentDetails,status` |
| playlists create | `POST /playlists?part=snippet,status` | `--title [--description] [--privacy]` |
| playlists update | `PUT /playlists?part=snippet,status` | `--id` + fields |
| playlists delete | `DELETE /playlists` | `--id` |
| playlist-items list | `GET /playlistItems` | `--playlist`; part `snippet,contentDetails` |
| playlist-items add | `POST /playlistItems?part=snippet` | `--playlist --video [--position]` |
| playlist-items remove | `DELETE /playlistItems` | `--id` (the playlistItem id, not the video id) |
| comments list | `GET /commentThreads` | `--video`, `--order time\|relevance`; part `snippet,replies` (top-level threads) |
| comments replies | `GET /comments` | `--parent <commentId>`; replies under one top-level comment |
| comments reply | `POST /comments?part=snippet` | `--parent <commentId> --text` |
| comments update | `PUT /comments?part=snippet` | `--id --text` (own comment) |
| comments delete | `DELETE /comments` | `--id` |
| comments moderate | `POST /comments/setModerationStatus` | `--id --status heldForReview\|published\|rejected [--ban-author]` |
| comments spam | `POST /comments/markAsSpam` | `--id` |
| subscriptions list | `GET /subscriptions` | `--mine` \| `--channel`; part `snippet` |

Deliberate exclusions (recorded):

- **Video upload** (`videos.insert`, scope `youtube.upload`): the resumable
  multipart upload of a large binary file to
  `https://www.googleapis.com/upload/youtube/v3/videos` is a distinct protocol
  (session-init + chunked PUT) disproportionate to a JSON-passthrough tool, and
  it adds a second sensitive scope + its own review facet. Excluded from v1;
  scope list omits `youtube.upload`. Flagged as a possible follow-up if teammate
  demand appears.
- **captions** (download/upload of caption binaries), **channelBanners**,
  **thumbnails.set** (binary upload), **watermarks**, **liveBroadcasts /
  liveStreams** (the Live Streaming API is a separate operational surface),
  **members / membershipsLevels** (`youtubepartner`), and **activities** (low
  signal) — all out of scope for a general teammate tool; keeps the requested
  scope set to the single `youtube.force-ssl`.

API facts the implementation must honor:

- **`part` is mandatory on every read** and shapes the response; the service
  sends a sensible default per verb and passes `--part` through verbatim (no
  re-invented vocabulary — same discipline as the calendar/search-console
  tools).
- **Quota:** default project quota is 10,000 units/day; `search` costs 100
  units, most reads 1 unit, writes ~50. On `403` with reason `quotaExceeded`
  surface Google's error verbatim — **no** client-side throttling or retry loop.
- **`videos.update` / `playlists.update` replace the whole named `part`**: the
  service must GET the current `snippet` and merge caller-supplied fields before
  the PUT, or unspecified fields are cleared. Documented in the service.
- **Paging** is `pageToken` / `nextPageToken` with `maxResults` 1–50 (default
  5); the tool exposes `--max` and `--page`, echoes `nextPageToken`.
- `search` returns lightweight results whose ids live under
  `id.videoId` / `id.channelId` / `id.playlistId` (not a flat `id`) — the
  service normalizes this.

## 3. anycli definition and service

**Stage-1 form decision: `service` type.** No official YouTube CLI binary exists
(`gcloud` does not cover the Data API; community CLIs fail the "official" bar),
so the `cli`-type rubric fails at its first clause. Implement
`internal/tools/youtube/` against the HTTP API. Matches the 21-of-23 precedent
and the whole Google family.

`definitions/tools/youtube.json`:

```json
{
  "name": "youtube",
  "type": "service",
  "description": "YouTube as a tool (OAuth 2.0 user access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "YOUTUBE_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single credential binding `access_token` → `YOUTUBE_ACCESS_TOKEN`, matching the
bundle's `credential.fields` and the gmail precedent (`GMAIL_ACCESS_TOKEN`).
Registered as `RegisterService("youtube", &youtube.Service{})` in
`internal/tools/register.go` — registration rides the **batch-end** merge; the
definition JSON + service package merge freely mid-batch.

**Service shape** (`internal/tools/youtube/`, copying the `notion` / `gmail`
skeleton): `Service{BaseURL, HC, Out, Err}` so tests point `BaseURL` at an
`httptest.Server`; `Execute(ctx, args, env)` fails fast when
`YOUTUBE_ACCESS_TOKEN` is unset; cobra root `youtube` with `SilenceUsage`,
persistent `--json`; runnable resource groups (unknown subcommand = failure, not
help-with-exit-0); documented exit-code contract (0 success, 1 runtime/API via a
typed `apiError`, 2 usage/parse). Single overridable `BaseURL`
(`https://www.googleapis.com/youtube/v3`) — no second base is needed once upload
is excluded.

Command tree:

```
youtube channels get      [--mine | --id ID,.. | --for-handle @h | --for-username u] [--part ...]
youtube search            --query Q [--type video|channel|playlist] [--channel ID]
                          [--order relevance|date|rating|viewCount|title]
                          [--published-after RFC3339] [--published-before RFC3339]
                          [--region CC] [--max N] [--page TOKEN]
youtube videos get        --id ID,.. [--part ...]
youtube videos mine       [--max N] [--page TOKEN]
youtube videos update     --id ID [--title T] [--description D] [--tags a,b,c]
                          [--category-id N] [--privacy public|unlisted|private]
youtube videos rate       --id ID --rating like|dislike|none
youtube playlists list    [--mine | --channel ID] [--max N] [--page TOKEN] [--part ...]
youtube playlists create  --title T [--description D] [--privacy public|unlisted|private]
youtube playlists update  --id ID [--title T] [--description D] [--privacy ...]
youtube playlists delete  --id ID
youtube playlist-items list   --playlist ID [--max N] [--page TOKEN]
youtube playlist-items add    --playlist ID --video ID [--position N]
youtube playlist-items remove --id PLAYLIST_ITEM_ID
youtube comments list     --video ID [--order time|relevance] [--max N] [--page TOKEN]
youtube comments replies  --parent COMMENT_ID [--max N] [--page TOKEN]
youtube comments reply    --parent COMMENT_ID --text BODY
youtube comments update   --id COMMENT_ID --text BODY
youtube comments delete   --id COMMENT_ID
youtube comments moderate --id COMMENT_ID --status heldForReview|published|rejected [--ban-author]
youtube comments spam     --id COMMENT_ID
youtube subscriptions list [--mine | --channel ID] [--max N] [--page TOKEN]
```

**JSON output shape** (`--json`): the provider response normalized, not
re-modeled. List verbs → `{"items":[...],"nextPageToken":"..."}` with YouTube's
`kind`/`etag` envelope stripped and `search` ids flattened to a top-level
`id`/`kind`. Single-resource gets echo the resource object. Mutations that return
a body (create/update) echo it; empty-body mutations (`videos rate`, deletes,
`setModerationStatus`, `markAsSpam`) → `{"ok":true,"id":...}`. Default (no
`--json`) is a compact human summary (e.g. channel line `title — subs / views /
videos`; comment threads as author + text lines). Exit-code contract and a
`--json` structured error envelope match notion/gmail.

## 4. Helio provider bundle plan

`integrations/providers/youtube/provider.yaml` — clones the shipped `gmail`
bundle (both are Google `standard_oauth` with userinfo identity); differs only in
key/name/order/scope and the flat (ungrouped) `tool` block:

```yaml
schema: helio.provider/v1
key: youtube
go_name: YouTube

presentation:
  name: YouTube
  description_key: youtube
  consent_domain: accounts.google.com
  visible: false          # hidden-first; flip is the separate go-live change
  order: <batch lead assigns — next free Social & Media slot>

auth:
  type: oauth
  owner: individual        # the provider sees a person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://accounts.google.com/o/oauth2/v2/auth
    token_url: https://oauth2.googleapis.com/token
    token_exchange_style: form_secret
    pkce: none
    authorize_params:
      access_type: offline     # required: yields a refresh token
      prompt: consent          # required: fresh refresh token on reconnect
    scopes:
      - openid
      - email
      - profile
      - https://www.googleapis.com/auth/youtube.force-ssl
    display_scopes: [openid, email, profile, youtube.force-ssl]
    single_active_token: false
    refresh_lease: none
    revoke:
      url: https://oauth2.googleapis.com/revoke
      client_auth: none
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://openidconnect.googleapis.com/v1/userinfo
  stable_key: /sub
  label_candidates: [/email, /name, /sub]

connection:
  mode: isolated
  disconnect_mode: provider_revoke
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
  name: youtube            # anycli id + flat CLI command; no `command`/`group`
  kind: oauth
```

Bundle notes / decisions:

- **Identity is the Google account (`/sub`), not the channel.** Consistent with
  the rest of the Google family and stable across channel renames; the human
  label falls back to email/name. (A single Google login can own multiple
  YouTube channels; `--mine`/`--channel` at the tool layer selects among them —
  the *connection* identity stays the Google account.)
- **No `experiment` gating.** gmail (the key==id precedent) ships GA with no
  design-090 flag; youtube is a flat standalone brand, **not** part of the
  `google_tools`-gated Workspace group, so it does not inherit that flag.
  Hidden-first (`visible: false`) is the rollout control. If the batch wants a
  preview gate, that is a batch-lead call — flag noted, not assumed.
- **Zero integration-service code.** Pure `standard_oauth` + userinfo identity +
  `provider_revoke`; nothing about YouTube leaves the standard Google shape that
  gmail already proves, so **no adapter and no capability growth**.
- **Config (lane 1 / batch-end, Config Sync):** `oauth.client_id` /
  `oauth.client_secret` for key `youtube` land in `config/` **and** the
  `deploy/` Helm Secret together, before this provider's L5. Because Google
  verification is per-app-per-scope-set, whether the `youtube.force-ssl` scope
  is added to Helio's existing Google OAuth app or a separate client is a lane-1
  decision — the bundle is agnostic (it names only config-field slots); flag to
  the batch lead that the Google verification submission must include
  `youtube.force-ssl`.

Helio-side companions (all batch-end unless noted):

- **No `toolToProvider` entry** (② == ③).
- The five `provider-gen` projections — single batch-end regen, **never**
  committed from this branch.
- `provider_registry_test.go`: hand-add
  `model.ProviderYoutube: model.RuntimeStrategyStandardOAuth` to the
  `wantStrategies` map in
  `go-services/integration-service/service/provider_registry_test.go`.
  `provider-gen` generates the `ProviderYoutube` const in
  `provider_catalog.gen.go` but does **not** touch this hand-written test, so
  `TestDefaultProviderRegistryIsComplete` fails after the regen until this line
  lands. It cannot compile on the tool branch (the const only exists post-regen)
  → batch-end-coupled exactly like the projections.
- Icon `ui/helio-app/src/integrations/icons/youtube.svg` + manual
  `providerIcons.ts` registration.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` + one plugin
  version bump per batch.

## 5. Test plan (the five layers)

| Layer | What runs here | External credentials |
|---|---|---|
| L1 | anycli unit tests, httptest fake for `https://www.googleapis.com/youtube/v3` (TDD, tests first): assert `Authorization: Bearer` injection; `part` defaults + `--part` passthrough; `search` id-flattening (`id.videoId` → top-level) and 100-unit verbs; `videos update` read-modify-write (GET snippet then PUT); paging (`pageToken`/`maxResults`, `nextPageToken` echo); empty-body mutations (`rate`, deletes, `setModerationStatus`, `markAsSpam`) → `{"ok":true}`; the comment moderation verbs; `403 quotaExceeded` → exit-1 verbatim (no retry); `--json` vs plain rendering; exit codes 0/1/2; unknown-subcommand failure. `go test ./...` green. | None |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli youtube -- channels get --mine`, then `search --query … --max 3`, `videos get --id …`, `comments list --video …`, one `playlists create` + `playlist-items add` + `playlist-items remove` + `playlists delete` round-trip on a scratch playlist, and one `comments reply` + `comments delete` round-trip on a test video the account owns. Mandatory before the pin bump. | **Yes** — lane 1/2: a Google account that owns a YouTube channel, and a `youtube.force-ssl`-scoped access token minted from the dev-mode app (OAuth Playground against the dev client works). |
| L3 | Local-only `provider-gen` + `provider-gen --check` against the branch bundle (regens **not** committed; branch expectedly red on this CI check until batch-end); helio-cli built with a **locally uncommitted** `go.mod` `replace github.com/heliohq/anycli => ../../../anycli/.claude/worktrees/tool-youtube`; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service unit suite. | None |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with `provider: youtube`, real `access_token` **and** `refresh_token`, deliberately short `expires_at` to force the gateway's refresh-and-write-back path (YouTube/Google have a real ~1 h cycle); then `heliox tool youtube -- channels get --mine` must return live data. Real seeded org/user/assistant ids per the skill's L4 notes. | **Yes** — lane-1 dev client id/secret as uncommitted local `config/cloud.yaml` entries + a real token pair from the L2 account. |
| L5 | Human-in-the-loop (oauth lane): `heliox tool youtube auth` → connect link → real Google consent on the pool account → `oauth_connected` system event on the originating channel → one unseeded live run. Runs after batch-end merge + lane-1 config landing; the **visible flip** additionally waits on Google sensitive-scope (`youtube.force-ssl`) verification clearance (oauth_review). | **Yes** — human consent session on the pooled Google/YouTube account; verified dev app. |

Definition of done (master plan §2): all five layers green, docs published, icon
registered, then `visible: true` + regenerate as the single go-live change —
which for this tool additionally waits on Google verification clearance.

## 6. Recorded divergences / open flags (for the batch lead)

1. **Grouping divergence from gmail (intentional).** YouTube reuses gmail's
   key==id derivation but is a **flat, ungrouped** command (`heliox tool
   youtube`), not `tool.group: google`. Rationale in §0; matches
   gemini/bigquery/firebase, per master-plan §3.
2. **oauth-audit.md has no YouTube row** — not a divergence: the audit scoped
   only pre-audit api_key tools. Lane source is the catalog; independently
   re-verified here (§1) and upheld as `oauth_review`.
3. **Sensitive scope, not restricted** — Google OAuth *verification* only, no
   CASA assessment; cheaper end of the review lane (same class as Search
   Console `webmasters`).
4. **`youtube.upload` deliberately excluded** (§2) — keeps the requested scope
   to the single `youtube.force-ssl` and avoids the resumable-upload protocol.
   Revisit only on demand.
5. **Shared vs separate Google OAuth app** for the `youtube.force-ssl` scope
   addition — lane-1 / batch-lead decision (§4); the verification submission
   must include this scope.
6. **`provider_registry_test.go` batch-end line** required (§4) — cannot compile
   on-branch; couples to the regen exactly like the projections.
