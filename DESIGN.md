# Instagram — `heliox tool` provider design

Scratch design for branch `tool/instagram` (stripped at batch-end by the batch
lead). Catalog row 200: anycli id `instagram`, provider key `instagram`, auth
lane `oauth_review`, Wave 3, category Social & Media.

## 0. Verdict summary (independent check vs catalog/audit)

- **Lane `oauth_review` — CONFIRMED against official docs.** The
  Instagram-teammate permissions (`instagram_business_basic`,
  `instagram_business_content_publish`, `instagram_business_manage_comments`,
  `instagram_business_manage_insights`) start at **Standard Access** (own /
  test-user accounts only) and require **Meta App Review → Advanced Access**
  before the app may act on accounts the developer does not own — Meta's
  documented review is 2–4 weeks per submission, per-permission screencast +
  written justification. That human review gate is exactly the `oauth_review`
  rubric. Instagram was born `oauth_review` in the catalog (not touched by the
  2026-07-21 api_key audit) — the audit only re-laned api_key rows, and
  Instagram was never one; this independent check upholds the lane.
- **This is a Meta-family OAuth tool and reuses the Meta-family capability the
  meta-ads tool introduces** — but with **two material divergences from
  meta-ads/Facebook that the catalog did not capture** (§3):
  1. **Different long-lived exchange grant + host.** Facebook upgrades a
     short-lived token via `fb_exchange_token` on `graph.facebook.com`;
     Instagram uses **`ig_exchange_token` on `graph.instagram.com`**. So the
     `long_lived_exchange` reviewed enum that meta-ads adds
     (`none|facebook`) needs **one more value, `instagram`** — a distinct host
     and grant_type, not a reuse of `facebook`.
  2. **Instagram HAS a real long-lived-token refresh; Facebook does not.**
     Facebook has no refresh grant at all (meta-ads ships `refresh_lease: none`
     and re-consents at ~60 days). Instagram exposes
     **`GET graph.instagram.com/refresh_access_token?grant_type=ig_refresh_token`**,
     which rolls a ≥24h-old, non-expired 60-day token forward another 60 days
     with **no client_secret and no `refresh_token`** (it re-submits the current
     access token itself). This does **not** fit the RFC-6749 `refresh_lease`
     path, so it is a second, scoped, family-shaped capability decision (§4).
- Everything else matches the catalog: `service`-type anycli definition (no
  official agent-friendly CLI), assistant-owned OAuth connection, hidden-first
  rollout, ②==③ (`instagram`/`instagram`) so **no `toolToProvider` divergence
  entry** is needed.

Sources verified (official Meta for Developers docs):
[Instagram API with Instagram Login](https://developers.facebook.com/docs/instagram-platform/instagram-api-with-instagram-login/) ·
[Business Login for Instagram](https://developers.facebook.com/docs/instagram-platform/instagram-api-with-instagram-login/business-login) ·
[Content Publishing](https://developers.facebook.com/docs/instagram-platform/content-publishing/) ·
[Insights](https://developers.facebook.com/docs/instagram-platform/insights/) ·
[Refresh Access Token](https://developers.facebook.com/docs/instagram-platform/reference/refresh_access_token/) ·
[Access Token](https://developers.facebook.com/docs/instagram-platform/reference/access_token/) ·
[Overview](https://developers.facebook.com/docs/instagram-platform/overview/).

**API choice — "Instagram API with Instagram Login", not the Facebook-Login
path or the retired Basic Display API.** Two current paths exist to reach an
Instagram professional account: (a) *Instagram API with Facebook Login* — via a
Facebook Page linked to the IG account, tokens on `graph.facebook.com`; and (b)
*Instagram API with Instagram Login* (a.k.a. Instagram Business/Direct Login,
launched July 2024) — the user authenticates **directly on Instagram**, tokens
on `graph.instagram.com`, **no linked Facebook Page required**. The legacy
*Instagram Basic Display API* was **deprecated 2024-12-04** and is out. This
design wraps **(b)**: it is the lower-friction connect UX for a teammate (no
"which Facebook Page?" detour), it is the path Meta is steering new apps to, and
it maps cleanly to one connection == one Instagram professional account. Mixing
hosts is the single most common failure mode ("Invalid OAuth access token –
Cannot parse access token" when an Instagram-Login token is sent to
`graph.facebook.com`), so the service pins `graph.instagram.com` as its one API
host constant.

## 1. API surface wrapped — and why

An AI teammate managing an Instagram business/creator account does five things a
social manager is actually asked for: **read the account** ("how many followers
/ what's posted"), **read & measure media** (list posts, per-post reach /
engagement), **publish** ("post this image with this caption"), **do community
management** (read comments, reply, hide/delete), and **read account-level
insights** ("how did the account do this week"). That maps to the Instagram User
node (`/me`) and its `media`, `comments`, and `insights` edges plus the
content-publishing container flow.

Base URL: `https://graph.instagram.com/v{VERSION}` with `VERSION` pinned as a
single service constant (a recent stable Graph version, e.g. `v23.0`; Meta
deprecates versions on a ~2-year clock, so it is one maintained constant, never
per-request-defaulted — matching the "no silent fallback" rule). Auth is
`Authorization: Bearer <token>` (preferred over `?access_token=` so the token
never lands in a URL or log).

Endpoints wrapped:

| Capability | Method + edge | Permission |
|---|---|---|
| Account / profile | `GET /me?fields=user_id,username,name,biography,followers_count,follows_count,media_count,profile_picture_url` | `instagram_business_basic` |
| List media | `GET /me/media?fields=id,caption,media_type,media_url,permalink,timestamp,like_count,comments_count` | `instagram_business_basic` |
| Get one media | `GET /{media_id}?fields=…` | `instagram_business_basic` |
| Media insights | `GET /{media_id}/insights?metric=reach,likes,comments,saved,shares` | `instagram_business_manage_insights` |
| Account insights | `GET /me/insights?metric=reach,follower_count,profile_views&period=day[&since=&until=]` | `instagram_business_manage_insights` |
| Create media container | `POST /me/media` body `image_url`/`video_url`, `caption`, `media_type` | `instagram_business_content_publish` |
| Poll container status | `GET /{container_id}?fields=status_code` | `instagram_business_content_publish` |
| Publish container | `POST /me/media_publish` body `creation_id={container_id}` | `instagram_business_content_publish` |
| List comments | `GET /{media_id}/comments?fields=id,text,username,timestamp,like_count,replies{...}` | `instagram_business_manage_comments` |
| Reply to comment | `POST /{comment_id}/replies` body `message` | `instagram_business_manage_comments` |
| Hide / unhide comment | `POST /{comment_id}` body `hide=true|false` | `instagram_business_manage_comments` |
| Delete comment | `DELETE /{comment_id}` | `instagram_business_manage_comments` |

**Content publishing is deliberately the async 3-step, exposed — not hidden
behind one magic verb.** Instagram publishing is container-based: `POST /me/media`
returns a container id, Instagram then processes the media **asynchronously**
(downloads the public URL, transcodes video, generates thumbnails), and only
once `status_code == FINISHED` may you `POST /me/media_publish`. Containers
**expire after 24h**; media must be at a **publicly reachable URL** at publish
time; accounts are capped at **50 published posts / 24h**. Rather than bake in a
fixed sleep-and-hope (the failure mode of every "one-call" wrapper), the tool
exposes `publish create` → `publish status` → `publish finish` so the assistant
polls and decides — the Helio "agency over automation / do it the way a human
would" principle. `media_type` supports feed images plus `REELS`/`STORIES`
(video), passed through as a flag.

Why not more: DMs (`instagram_business_manage_messages`), mentions/tags, and
hashtag search are **deferred**. Messaging is a heavier review + a different
interaction model (webhooks), and tags/hashtag-search are read niceties; the
read + publish + comment + insights loop above is the full core a teammate is
asked to drive and is proven hidden first. Ads and tagging are **not available**
on the Instagram-Login path at all (Meta's documented limitation).

## 2. anycli definition

- **Type: `service`.** No official non-interactive `--json` CLI exists for the
  Instagram Platform API (the `service` default per stage-1 rubric; 21/23
  shipped definitions are service). HTTP logic lives in
  `internal/tools/instagram/` (package `instagram` — id has no dashes, so the
  package name equals the id), registered
  `RegisterService("instagram", &instagram.Service{})` in
  `internal/tools/register.go`.
- **Definition** `definitions/tools/instagram.json`: `name: "instagram"`, one
  credential binding — `source.field: access_token` injected as env
  `INSTAGRAM_ACCESS_TOKEN`; the service sends it as `Authorization: Bearer`.
- **Command tree** (cobra, grouped by resource — copy the
  `internal/tools/notion` shape: `BaseURL`/`HC`/`Out`/`Err` struct so tests
  point at an `httptest` server and capture output):

  ```
  instagram account get                         # GET /me?fields=...
  instagram media   list  [--limit N] [--after CURSOR] [--fields ...]   # GET /me/media
  instagram media   get   <media_id> [--fields ...]
  instagram media   insights <media_id> [--metrics reach,likes,comments,saved,shares]

  instagram publish create --image-url URL | --video-url URL
                           [--caption ...] [--media-type IMAGE|REELS|STORIES]   # -> container id
  instagram publish status <container_id>       # status_code: IN_PROGRESS|FINISHED|ERROR|EXPIRED|PUBLISHED
  instagram publish finish <container_id>       # POST /me/media_publish creation_id=<container_id>

  instagram comment list   <media_id> [--fields ...]
  instagram comment reply  <comment_id> --message ...
  instagram comment hide   <comment_id> [--hidden true|false]
  instagram comment delete <comment_id>

  instagram insights       [--metrics reach,follower_count,profile_views]
                           [--period day] [--since UNIX] [--until UNIX]   # GET /me/insights
  ```

  **The connection targets one Instagram professional account — `/me` is that
  account.** Unlike Meta Ads (where one Facebook user spans many ad accounts and
  the account is a per-command flag), an Instagram-Login token *is* scoped to a
  single IG professional account, so `/me` needs no account selector. The
  connection identity is that IG account (§3).

- **JSON output & errors** (notion contract): `--json` emits a structured
  envelope; default is human-readable. Exit codes **0** success, **1**
  runtime/API failure, **2** usage/parse. Graph errors deserialize to a typed
  `apiError` from the standard envelope
  `{"error":{"message,type,code,error_subcode,fbtrace_id}}` (Instagram uses the
  same Graph error shape) and render in both plain and `--json` form.
  `code:190` (OAuthException / expired or revoked token) is surfaced as a
  distinct "reconnect needed" message so the assistant prompts re-auth rather
  than retrying blindly. The publish path additionally maps a container
  `status_code` of `ERROR`/`EXPIRED` to exit 1 with a clear message (do not let
  `publish finish` fire on a non-`FINISHED` container).

## 3. Credential fields & auth flow (oauth_review lane, verified)

**Registration model.** One Meta App (developers.facebook.com) with the
**Instagram** product added and *Business Login* / *Instagram API with Instagram
Login* configured; client credentials are the **Instagram app id / app secret**
(the App-Dashboard "Instagram app ID", distinct from the Facebook App ID).
Redirect URIs are registered in the dashboard. Dev/test-mode apps work against
the developer's own IG professional account and explicitly-listed **test users**
*without* review (gates only L4 / own-asset L5); acting on other users' accounts
needs Advanced Access via **App Review** (gates the **visible flip** only —
hidden-first decouples it from dev).

**OAuth endpoints (official), a three-host flow:**
- **Authorize:** `https://www.instagram.com/oauth/authorize` — params
  `client_id, redirect_uri, response_type=code, state,
  scope=instagram_business_basic,instagram_business_content_publish,instagram_business_manage_comments,instagram_business_manage_insights`.
- **Code → short-lived token:** `POST https://api.instagram.com/oauth/access_token`
  (form body: `client_id, client_secret, grant_type=authorization_code,
  redirect_uri, code`). Response is **JSON** carrying `access_token` (a
  ~1-hour short-lived IG User token), the **`user_id`** (Instagram App-scoped
  User ID), and the granted `permissions`.
  - **Verified response-shape divergence (review fix).** Meta's **current
    official Business Login doc** returns these fields **wrapped in a
    single-element `data` array** —
    `{"data":[{"access_token":...,"user_id":"...","permissions":...}]}` — with
    `user_id` a **string**, *not* the flat `{access_token,user_id}` this design
    originally assumed. Community implementations of the same July-2024 flow
    still parse the **flat** top-level shape, i.e. Meta rolls the envelope out
    **inconsistently per app**. A static identity pointer therefore cannot cover
    both shapes. Resolution: the shared code exchanger
    (`integration-service/service/oauth_exchange.go`, `unwrapDataEnvelope`)
    **normalizes** a `data[0]`-wrapped body to the top level whenever the flat
    `access_token` is absent, so one flat contract (`access_token` + identity
    `/user_id`) serves both wrapped and flat apps. Absent that fix the standard
    exchanger errored `token endpoint returned no access_token` and the whole
    connect flow broke on wrapped apps. **L5 must still capture the raw
    code-exchange body once against a live app** to confirm the shape; the
    normalization already tolerates either outcome.
- **Short → long-lived (~60 days):**
  `GET https://graph.instagram.com/access_token?grant_type=ig_exchange_token&client_secret=<secret>&access_token=<short>`
  → JSON `{access_token, token_type:"bearer", expires_in≈5184000}`.
- **Refresh long-lived (roll forward another ~60 days):**
  `GET https://graph.instagram.com/refresh_access_token?grant_type=ig_refresh_token&access_token=<long>`
  — valid when the token is **≥24h old and not yet expired**; **no
  client_secret**, **no `refresh_token`** (the current access token is the
  input). Tokens not refreshed within 60 days expire and cannot be refreshed
  (re-consent required); a password change or app-revoke also invalidates.

**Token semantics that drive the bundle:**
- There is **no RFC-6749 `refresh_token` grant**. The usable credential is the
  long-lived (~60-day) token obtained via `ig_exchange_token`.
- Instagram **can** roll that token forward indefinitely via `ig_refresh_token`
  (unlike Facebook). This is a genuine, unattended-friendly refresh, but its
  wire shape (GET, no secret, token-as-input, `graph.instagram.com` host) is
  **not** the standard exchanger's refresh path — see §4 for how it is modeled.

**Credential fields (bundle `credential:`):** `access_token: token.access_token`,
`account_key: connection.account_key`.

**Scopes** requested: `instagram_business_basic`,
`instagram_business_content_publish`, `instagram_business_manage_comments`,
`instagram_business_manage_insights`. (`instagram_business_manage_messages` is
omitted — DMs are deferred, §1.) The **new** scope names are used; the legacy
`business_*` values were **deprecated 2025-01-27** and must not appear.

## 4. Helio provider bundle plan (`integrations/providers/instagram/provider.yaml`)

**Three axes.** ① CLI command word `instagram` (flat; no `tool.group` — a future
`meta` family group covering Facebook Pages / Instagram is deferred per
master-plan open question 2, same posture as meta-ads). ② anycli id
`instagram`. ③ provider key `instagram`. **②==③, so no `toolToProvider`
entry** and no resolver change (contrast meta-ads' `meta-ads`→`meta_ads`).

**Hidden-first:** `presentation.visible: false`. Bundle sketch:

```yaml
schema: helio.provider/v1
key: instagram
go_name: Instagram
presentation:
  name: Instagram
  description_key: instagram
  consent_domain: instagram.com
  visible: false
auth:
  type: oauth
  owner: assistant                     # a brand's IG account is a shared team asset (cf. meta-ads)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.instagram.com/oauth/authorize
    token_url: https://api.instagram.com/oauth/access_token
    token_exchange_style: form_secret  # code→short-lived: form body carries client_secret
    pkce: none                         # confidential client; Instagram Login uses client_secret
    scopes:
      - instagram_business_basic
      - instagram_business_content_publish
      - instagram_business_manage_comments
      - instagram_business_manage_insights
    single_active_token: false
    long_lived_exchange: instagram     # NEW enum value on the meta-ads capability — see below
    long_lived_refresh: instagram      # NEW reviewed capability — see below (fallback: refresh_lease: none)
identity:
  source: token_response               # code→token response carries user_id (App-scoped IG user id)
  stable_key: /user_id
  label_candidates: [/user_id]
connection:
  mode: isolated
  disconnect_mode: local_only          # IG revoke is DELETE /me/permissions — declarative RFC-7009 revoker can't express it
  runtime_strategy: standard_oauth
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: instagram
  kind: oauth
```

**Capability growth #1 — `long_lived_exchange: instagram`.** meta-ads introduces
the reviewed enum `long_lived_exchange: none|facebook` on the
`standardOAuthExchanger` (after the `authorization_code` grant, upgrade the
short-lived token to the long-lived one so the stored credential is the ~60-day
token). Instagram needs **one more enum value**, `instagram`, because the
upgrade is a **different host + grant_type** (`ig_exchange_token` on
`graph.instagram.com`, vs `fb_exchange_token` on `graph.facebook.com`). This is
the orthogonal, family-minded move the skill asks for — grow the closed reviewed
enum rather than fork a `service/adapter_instagram.go`. **Cross-batch
dependency:** the `none|facebook` enum ships with the meta-ads batch (Wave 2);
this tool's `instagram` value assumes that enum exists. If Instagram's batch
lands **before** meta-ads', this branch introduces the enum
(`none|facebook|instagram`) and meta-ads reuses it — either order is fine, but
the batch lead must sequence the `provider-gen` model change so the enum is a
superset, not a conflict.

**Capability growth #2 — the long-lived refresh (`long_lived_refresh:
instagram`).** This is the real Instagram-specific decision and the one place
the design must choose:

- **Primary recommendation — add `long_lived_refresh: none|instagram`**, a
  second reviewed enum on the standard exchanger that the token gateway's
  refresh-and-write-back path (A3) invokes when the stored long-lived token is
  near expiry: `GET graph.instagram.com/refresh_access_token?grant_type=ig_refresh_token&access_token=<current>`,
  storing the returned token + new `expires_at`. It is deliberately **not**
  folded into RFC-6749 `refresh_lease` because it has no `refresh_token` and no
  client_secret — conflating them would overload `refresh_lease` with a
  discriminator, the exact "not orthogonal" smell the code-health rule warns
  against. It **is** family-shaped: Instagram and Threads share the `ig_/th_`
  refresh grant, so a closed enum (mirroring `long_lived_exchange`) is the right
  altitude. This gives an **unattended teammate a connection that survives
  indefinitely** — the correct product behavior, and Instagram's genuine
  advantage over Facebook.
- **Zero-capability fallback (unblocks hidden-merge if the refresh-enum review
  slips) — `refresh_lease: none` + drop `long_lived_refresh`.** Store the
  60-day token; when it lapses, re-consent. This is exactly meta-ads/Facebook's
  shipped behavior, needs **no** capability beyond growth #1, and does not
  reshape the bundle. Ship this if growth #2's review trails the batch; flip to
  the refresh enum as a fast-follow. **The design's position:** target the
  refresh (primary) because unattended survival is the whole point of a
  teammate connection; fall back only to keep the hidden merge moving.

**Identity — `token_response` / `/user_id` (primary), userinfo as the L2-gated
alternative.** The code→token exchange response already carries `user_id` (the
App-scoped IG user id), giving a deterministic, always-present stable key with
**no extra GET** — cleaner than meta-ads' userinfo `/me`. The trade-off is the
**label**: a human-friendly `@username` needs `GET /me?fields=username`, but the
generator **forbids a query string on `identity.url`** (meta-ads hit this
exact rule), and whether `graph.instagram.com/me` returns `username` among its
**default** fields (no `?fields`) is unverified. So the first cut labels by
`user_id`; **L2 must check** whether default `/me` yields `username` — if it
does, switch to `identity.source: userinfo`, `url:
https://graph.instagram.com/me`, `stable_key: /user_id`, `label_candidates:
[/username, /user_id]`. Recorded as an as-built decision, not a blocker.

**`disconnect_mode: local_only`.** Instagram revocation is
`DELETE /me/permissions` (path-embedded user, DELETE method, Bearer token) — the
declarative RFC-7009 revoker (POST `token=<...>` form to a fixed URL)
structurally cannot express it, identical to meta-ads' as-built. No `revoke:`
block (the generator forbids one under `local_only`). A DELETE-based declarative
revoker is out of scope — it would be a third unreviewed capability.

**Config (lane 1 lands, per Config Sync):** `oauth.client_id` /
`oauth.client_secret` (Instagram app id / app secret) into integration-service
config in **both** `config/` and the `deploy/` Helm Secret, together (a
partially-configured provider fails startup; both-absent renders
`configured:false` and is safe hidden).

**UI icon:** `ui/helio-app/src/integrations/icons/instagram.svg` + manual append
in `providerIcons.ts` (Instagram glyph; never generated).

## 5. Test plan → five layers (external-credential needs flagged)

| Layer | What runs | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — `httptest` fake `graph.instagram.com`: assert Bearer injection, `/me`/`/me/media` field params, `publish create`→container-id parse, `publish status` `status_code` decode + `ERROR/EXPIRED`→exit 1, `publish finish` `creation_id` body, `comment reply/hide/delete` shapes, insights query params, error-envelope + `--json` rendering, exit codes 0/1/2, and the `code:190` reconnect path. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<long-lived IG token> anycli instagram -- account get` then `media list`, `media insights <id>`, `insights`, and a full `publish create`→`publish status`→`publish finish` against the **real** `graph.instagram.com`. Proves host, field names, injection, and the container flow match live. **Verify here** whether default `/me` returns `username` (identity decision, §4). | **Yes** — a real Meta app with the Instagram product + a long-lived IG User token on a real **professional** account (personal accounts are unsupported), plus a publicly-hosted test image URL. |
| **L3** generate + suites | `provider-gen` + `provider-gen --check` (five projections); the new `long_lived_exchange: instagram` (and, if taken, `long_lived_refresh: instagram`) enum values validate; helio-cli + integration-service unit suites. No `toolToProvider` entry to test (②==③). Branch is *expected* to fail `--check` in CI until batch-end (do not commit local regens). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"instagram"`, `access_token:<long-lived>` (oauth user-token providers are seedable), then `heliox tool instagram -- account get` / `media list` / `insights`. **If** growth #2 (refresh) is taken, seed a short `expires_at` to force the gateway's `ig_refresh_token` write-back path; **if** the fallback (`refresh_lease: none`) is taken, seed `access_token` only (no refresh cycle to exercise), and L4 proves only the token-gateway→anycli path. | **Yes** — real long-lived token (from lane-1 dev app) + dev Instagram app id/secret in local uncommitted `config/cloud.yaml`. |
| **L5** full connect | Hidden, pre-flip: `heliox tool instagram auth` → Instagram Business Login consent on the dev app (own / test-user IG professional account) → `oauth_connected` event → unseeded live `instagram account get`. Verifies the `ig_exchange_token` long-lived upgrade fires during the real exchange. | **Yes, human-in-the-loop** — Instagram 2FA/consent defeats agent automation. Runs on **own / test-user** assets under Standard Access (no review needed for L5 itself); **App Review clearance gates only the subsequent visible flip**. |

**Externally-supplied credentials required at L2, L4, L5** (a real Meta app with
the Instagram product, a long-lived token, a real IG professional account, and a
public image URL for the publish path). L1/L3 need none. Lane-1 dev-app creation
gates L4; Advanced-Access App Review gates only the visible flip.

## 6. Open risks

- **App Review / Advanced Access tail.** Standard Access covers all dev, L1–L4,
  and own/test-user L5; the visible flip waits on Meta App Review (per-permission
  screencast + written justification, ~2–4 weeks each, and
  `instagram_business_content_publish` is a scrutinized write scope). Hidden-first
  means zero code waste while it clears. Track on the wave board.
- **Two capability-growth artifacts, one required.** Growth #1
  (`long_lived_exchange: instagram`) is **required** and cross-batch-coupled to
  meta-ads' enum (sequence the superset). Growth #2 (`long_lived_refresh:
  instagram`) is the **recommended** but droppable refresh capability; if its
  review trails the batch, ship the `refresh_lease: none` fallback and
  fast-follow. Neither reshapes the bundle. Batch-end review should look at both
  as **Meta/Instagram-family** capabilities (Instagram + Threads reuse
  `ig_refresh_token`), not instagram one-offs.
- **Container-flow ergonomics.** 24h container expiry, public-URL hosting, async
  `status_code`, and the 50-posts/24h cap are provider constraints the *tool*
  surfaces (poll + clear errors) rather than hides; the assistant, not a baked-in
  sleep, drives the wait. Confirm the `status_code` state set at L2.
- **Host-mixing footgun.** The three-host OAuth flow (`www.instagram.com`
  authorize, `api.instagram.com` code-exchange, `graph.instagram.com`
  exchange/refresh/API) plus the API host must stay exactly as specified;
  sending an Instagram-Login token to `graph.facebook.com` yields the classic
  "Cannot parse access token". Pin `graph.instagram.com` as the one service API
  constant.
- **Version pin maintenance.** `v23.0` (or the then-current stable) is a single
  service constant carrying Meta's ~2-year deprecation clock; several Instagram
  insights metrics were deprecated at v21 (2025-01-08), so keep metric names
  matched to the pinned version. Note it for the maintenance board; do not
  scatter it per-request.

## 7. Review-fix as-built notes

- **Code-exchange envelope (Finding 1, major) — RESOLVED.** See §3: Meta's
  current official Business Login exchange returns the `data[]`-wrapped shape
  (inconsistently per app), which broke the standard exchanger. The shared
  exchanger now normalizes `data[0]` to top level (`unwrapDataEnvelope`), so
  `identity.source: token_response` / `stable_key: /user_id` serves both flat
  and wrapped apps unchanged. Still needs a one-off L5 capture of the raw body
  to confirm which shape the live dev app emits (the code tolerates either).
- **Long-lived refresh (Finding 2, minor) — DOCUMENTED, follow-up tracked.** The
  shipped bundle takes the §4 growth-#2 zero-capability fallback:
  `refresh_lease: none`, no `long_lived_refresh`. Consequence, now stated
  explicitly in `provider.yaml` and the AI-facing doc: a connection's ~60-day
  token is **not** auto-refreshed and silently lapses, requiring a manual
  reconnect. Wiring `long_lived_refresh: instagram`
  (`GET graph.instagram.com/refresh_access_token?grant_type=ig_refresh_token`)
  is a tracked follow-up **before broad GA**, not just before the visible flip.
- **Account-insight defaults (Finding 3, minor) — FIXED.** `profile_views` was
  deprecated in Graph v22.0 while the service pins v23.0, so the old default
  `reach,follower_count,profile_views` shipped a deprecated metric. The anycli
  default is now `reach,follower_count`, and a new `--metric-type` flag
  (passthrough, no default) lets the assistant supply `total_value` for the v22+
  metrics that require it — previously impossible without dropping to the raw
  API. The AI-facing doc calls out that account insights are version-sensitive
  and may need `--metric-type total_value`.
