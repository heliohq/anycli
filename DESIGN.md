# Tool design: TikTok

Per-tool design for the `tiktok` external tool provider behind `heliox tool`.
Scratch file on branch `tool/tiktok`; the batch lead strips it at batch-end.

- **Catalog row:** #202 TikTok — anycli id `tiktok`, provider key `tiktok`,
  auth lane `oauth_review`, wave 3, category Social & Media
  (`docs/design/008-300-integrations-rollout-plan.md` §4).
- **OAuth audit:** TikTok does **not** appear in
  `oauth-audit.md` — that audit only re-laned tools that were `api_key`
  before 2026-07-21. TikTok was already `oauth_review` in the seed catalog,
  so the audit left it untouched. Nothing to reconcile there.
- **Divergence recorded here (official docs vs. repo assumptions):** the audit
  lane is correct, but TikTok's OAuth is **not** a vanilla authorization-code
  flow. It uses `client_key` (not `client_id`) as the client identifier in the
  authorize URL, the token exchange, the refresh, and the revoke; and it wants
  a **comma-separated** `scope` in the authorize URL. Neither is expressible in
  today's `standard_oauth` capability set (both hardcode `client_id` and
  space-joined scopes). This is a stage-1 non-standard-auth flag, resolved by a
  narrow reviewed capability growth — details in §5. Verified against official
  docs (§2).

---

## 1. What an AI teammate does with TikTok, and the API surface that serves it

TikTok row 202 is a **Social & Media** provider — the creator-facing content
account, the sibling of `youtube` and `instagram`, **not** the ads plane
(TikTok Marketing/Business API is a separate product and is out of scope; the
catalog's ad providers are `meta_ads` / `google_ads`, and TikTok Ads is not in
the 298). So the teammate's jobs are creator-account jobs:

| Teammate intent | Official API surface | Endpoint(s) |
|---|---|---|
| "Who is this account / how big is it?" | **Display API — User Info** | `GET /v2/user/info/?fields=…` |
| "List / inspect my recent videos and their metrics" | **Display API — List Videos, Query Videos** | `POST /v2/video/list/`, `POST /v2/video/query/` |
| "Can I post right now? (privacy options, limits)" | **Content Posting API — Query Creator Info** | `POST /v2/post/publish/creator_info/query/` |
| "Post this video to my profile" | **Content Posting API — Direct Post** | `POST /v2/post/publish/video/init/` |
| "Upload this as a draft to finish in the app" | **Content Posting API — Upload (inbox)** | `POST /v2/post/publish/inbox/video/init/` |
| "Did my post finish processing?" | **Content Posting API — Get Post Status** | `POST /v2/post/publish/status/fetch/` |

All of these live on host **`open.tiktokapis.com`** under the **`/v2`** REST
surface, JSON request/response, `Authorization: Bearer <user access token>`.
Auth is **TikTok Login Kit** (OAuth 2.0). We deliberately wrap this content
surface and skip the Research API (vetted-researcher gated), the Data
Portability API, and Local Services / commerce scopes — none map to a general
creator teammate.

**Why these and not more:** the six jobs above are the full read+publish loop a
teammate needs for a creator account. Direct Post (`video.publish`) and Upload
(`video.upload`) are the two scopes that require TikTok's app audit before they
work against arbitrary external accounts (§4) — which is exactly what puts this
tool in the `oauth_review` lane. Read-only jobs (`user.info.*`, `video.list`)
work under a sandbox/unaudited app against the developer's own account, so the
hidden bundle is fully L1–L4 testable before audit clears.

---

## 2. Verified official API facts (source of truth)

All from `developers.tiktok.com` (fetched 2026-07-22):

**Token endpoint** — `POST https://open.tiktokapis.com/v2/oauth/token/`,
`Content-Type: application/x-www-form-urlencoded`. Authorization-code params:
`client_key`, `client_secret`, `code` (must be URL-decoded), `grant_type=authorization_code`,
`redirect_uri`, and `code_verifier` (**only** required for mobile/desktop PKCE).

**Authorize endpoint** — `https://www.tiktok.com/v2/auth/authorize/`, params:
`client_key`, `scope` (**comma-separated**), `redirect_uri`, `state`,
`response_type=code`.

**Token response** — `open_id`, `scope` (comma-separated granted list),
`access_token`, `expires_in` (**86400** = 24 h), `refresh_token`,
`refresh_expires_in` (**31536000** = 365 d), `token_type=Bearer`.

**Refresh** — same token endpoint, `client_key` + `client_secret` +
`grant_type=refresh_token` + `refresh_token`. **Refresh tokens rotate**: the
returned `refresh_token` "may be different than the one passed in" and "you must
use the newly-returned token if the value is different" → the token gateway must
write back the rotated refresh token.

**Revoke** — `POST https://open.tiktokapis.com/v2/oauth/revoke/` with
`client_key` + `client_secret` + `token`.

**User Info** — `GET https://open.tiktokapis.com/v2/user/info/?fields=open_id,union_id,display_name,avatar_url`,
`Authorization: Bearer`. Response is wrapped: `{"data":{"user":{…}},"error":{…}}`,
so `open_id` is at JSON pointer `/data/user/open_id`. The `fields` query
parameter is **mandatory** (no fields ⇒ error).

**Scopes** (from the Scopes Reference): `user.info.basic` (open_id, avatar,
display name), `user.info.profile` (profile links, bio, is_verified),
`user.info.stats` (follower/following/likes/video counts), `video.list` (read
the user's public videos), `video.publish` (Direct Post), `video.upload` (draft
to the creator's inbox). Publish/upload require app audit before external use.

**Two non-standard shapes vs. our `standard_oauth`:** (a) client identifier is
`client_key`, not `client_id`, on all four OAuth calls; (b) authorize `scope` is
comma-separated. Handled in §5. `scope` separator is the one item to
re-confirm at L2 stage-1 (TikTok has historically accepted comma; if a space
also works, the capability collapses to just the `client_key` rename).

---

## 3. anycli definition

**Type: `service`** (stage-1 rubric). No official TikTok CLI exists; the surface
is REST JSON at `open.tiktokapis.com/v2`. Falls squarely in the 21/23 service
majority (copy `internal/tools/notion/` shape: resource-grouped cobra tree,
`BaseURL`/`HC`/`Out`/`Err` for httptest, exit-code contract 0/1/2, `--json`
error envelope).

- Definition file: `definitions/tools/tiktok.json` (axis ② = `tiktok`).
- Service package: `internal/tools/tiktok/` (Go pkg name `tiktok`; no digit/dash
  normalization needed), `RegisterService("tiktok", &tiktok.Service{})`.

**Credential injection** (definition `auth.credentials`), mirroring `x.json`
(access_token + user_id):

```json
{
  "name": "tiktok",
  "type": "service",
  "description": "TikTok as a tool (creator content: profile, videos, posting)",
  "auth": {
    "credentials": [
      { "source": {"field": "access_token"}, "inject": {"type": "env", "env_var": "TIKTOK_ACCESS_TOKEN"} },
      { "source": {"field": "open_id"},      "inject": {"type": "env", "env_var": "TIKTOK_OPEN_ID"} }
    ]
  }
}
```

The `/v2` endpoints identify the user by the bearer token, so `access_token`
alone is functionally sufficient; `open_id` is injected as a convenience/echo
value (the resolver maps it from the connection account key, §6) and for any
call that logs/labels the acting account.

**Subcommand tree** (verbs mapped to §1 jobs; all `--json`):

```
tiktok user info [--fields open_id,union_id,display_name,avatar_url,follower_count,…]
tiktok video list [--cursor N] [--max-count 20]
tiktok video query --ids id1,id2 [--fields …]
tiktok creator info                      # Query Creator Info (posting prereqs/limits)
tiktok post video  --title T (--file PATH | --url URL) [--privacy PUBLIC_TO_EVERYONE|…] [--draft]
                                         # --draft ⇒ inbox/upload; default ⇒ direct post
tiktok post status --publish-id PID      # Get Post Status
```

**JSON output shape:** provider-neutral, TikTok's `{data,error}` wrapper
unwrapped to a flat object per command (e.g. `user info` → the user object;
`video list` → `{videos:[…], cursor, has_more}`; `post video` → `{publish_id}`;
`post status` → `{status, …}`). API errors (`error.code != "ok"`, or non-2xx)
render through the `apiError` path → exit 1 + `--json` error envelope; usage/parse
errors exit 2.

**Axes** — ① CLI word `tiktok` (flat, ungrouped; no `tool.command`), ② anycli id
`tiktok`, ③ provider key `tiktok`. **No ②↔③ divergence ⇒ no `toolToProvider`
entry, no resolver change.**

---

## 4. Credentials & auth flow (oauth_review lane, verified)

**Registration model:** TikTok for Developers → create an app → add the products
**Login Kit**, **Display API**, and **Content Posting API**. The app issues
`client_key` + `client_secret`. Apps start in **sandbox** mode: dev/test works
against the developer's own account and a small allowlist of test users **before
review**. Going live (arbitrary external creators) requires **app review /
audit**, and `video.publish` / `video.upload` specifically require passing the
content-posting audit. This is precisely the `oauth_review` gate: **dev-mode
app creation gates L4** (a real sandbox token reaches the live API), **review
clearance gates only the visible flip** — matching master-plan §2 lane 1.

**Flow (server-side confidential client — Helio holds `client_secret`):**
1. Authorize redirect to `https://www.tiktok.com/v2/auth/authorize/` with
   `client_key`, comma-joined `scope`, `redirect_uri`, `state`, `response_type=code`.
2. Callback `code` → token exchange (form, `client_key`+`client_secret` in body)
   → `{open_id, access_token(24h), refresh_token(365d, rotating), scope}`.
3. Access token expires in 24 h ⇒ the token gateway refreshes via
   `grant_type=refresh_token` and **writes back the rotated refresh token**.

**PKCE:** `code_verifier` is required by TikTok only for mobile/desktop public
clients. Helio is a confidential server-side client authenticating with
`client_secret` in the body, so **`pkce: none`** is correct and simplest. (S256
is also accepted by the web flow; not needed.)

**Scopes to request (hidden bundle):** `user.info.basic`, `user.info.profile`,
`user.info.stats`, `video.list`, `video.publish`, `video.upload`. Read scopes
work in sandbox immediately; the two publish/upload scopes are the audit-gated
ones — the tool ships hidden with all six declared, and the visible flip waits
on audit clearance (which also removes the sandbox posting restriction that
forces posted content to private).

**Config fields** (integration-service, both `config/` and `deploy/` per Config
Sync): `oauth.client_id` and `oauth.client_secret` — the bundle's
`required_config_fields`. Note the field **names** stay `oauth.client_id` /
`oauth.client_secret` (Helio's generic config vocabulary); only the **wire
parameter** sent to TikTok is renamed to `client_key` by the §5 capability. The
`client_key` value goes in the `oauth.client_id` config slot.

---

## 5. Helio provider bundle plan (`integrations/providers/tiktok/provider.yaml`)

`standard_oauth` **with a narrow reviewed capability growth**, not an adapter.
Per `references/provider-yaml.md`, before reaching for `service/adapter_*.go`,
check whether the generic capability set should grow one reviewed enum/field —
and here the gap (a renamed client param + scope separator) is a small,
orthogonal, provider-neutral knob, not a bespoke response/lifecycle dialect. So:

**Capability growth in integration-service (§5-scoped, mirrors the
meta-ads/instagram/adobe-sign precedent of growing `standard_oauth`):**
- Add `oauth.client_id_param` (bundle field, default `client_id`) honored by
  `buildOAuthAuthorizeURL` (`service/oauth_start.go` ~L222), `buildTokenRequest`
  (`service/oauth_exchange.go` ~L140), the refresh builder, and the declarative
  revoker. TikTok sets it to `client_key`.
- Add `oauth.scope_separator` (default `" "`) honored where the authorize URL
  joins `DefaultScopes` (`oauth_start.go` ~L227). TikTok sets it to `","`.
  (Collapse to client-param-only if L2 confirms space-joined scopes also work.)
- Both are closed, reviewed manifest fields (extend `cmd/provider-gen/manifest.go`
  `oauthManifest` + `model/catalog.go` + `validate.go`), with unit tests
  asserting the rendered authorize URL / token form carry `client_key` and a
  comma scope. This is smaller and more orthogonal than an adapter and keeps the
  `standard_oauth` golden path intact for every other tool.

Bundle (**axis ①/②/③ all `tiktok`; hidden-first**):

```yaml
schema: helio.provider/v1
key: tiktok
go_name: TikTok

presentation:
  name: TikTok
  description_key: tiktok
  consent_domain: tiktok.com
  visible: false            # hidden-first; flip gated on TikTok audit + L5
  order: <next social slot>

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.tiktok.com/v2/auth/authorize/
    token_url: https://open.tiktokapis.com/v2/oauth/token/
    token_exchange_style: form_secret     # client creds in body…
    client_id_param: client_key           # …but named client_key  (NEW capability)
    scope_separator: ","                  # comma-joined scopes     (NEW capability)
    pkce: none                            # confidential server-side client
    scopes: [user.info.basic, user.info.profile, user.info.stats,
             video.list, video.publish, video.upload]
    single_active_token: false
    refresh_lease: provider               # rotating refresh token → serialize + write-back
    revoke:
      url: https://open.tiktokapis.com/v2/oauth/revoke/
      client_auth: body                   # client_key/client_secret in body
      token: access_token

identity:
  source: userinfo
  url: https://open.tiktokapis.com/v2/user/info/?fields=open_id,union_id,display_name,avatar_url
  stable_key: /data/user/open_id
  label_candidates: [/data/user/display_name, /data/user/open_id]

connection:
  mode: isolated                          # multiple TikTok accounts, one connection each
  disconnect_mode: strategy               # declarative revoke (needs client_key rename); local-delete fallback
  runtime_strategy: standard_oauth

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
    open_id: connection.account_key       # open_id IS the stable account key

tool:
  name: tiktok
  kind: oauth
```

Notes:
- **Identity** uses `userinfo` (not `token_response`) because the token response
  carries only `open_id` — good for the stable key but a poor human label. The
  `?fields=` userinfo GET yields `display_name` for the UI, and the wrapped
  `{data:{user:…}}` shape is reachable by RFC-6901 pointer. `token_response`
  with `stable_key:/open_id` is the fallback if the userinfo call proves flaky
  in sandbox.
- **`open_id`** is TikTok's stable per-user-per-app id ⇒ it is the connection
  `account_key`, so the injected `open_id` credential maps straight from
  `connection.account_key` (no extra metadata capture), same shape as `x`'s
  `user_id`.
- **UI icon:** `ui/helio-app/src/integrations/icons/tiktok.svg` + register in
  `providerIcons.ts` (manual; never generated). **AI-facing doc:** provider
  sub-doc under `agents/plugins/heliox/skills/tool/`.

---

## 6. Test plan — five layers (external-credential needs flagged)

| Layer | What runs | Needs external creds? |
|---|---|---|
| **L1** | `go test ./...` in anycli: `internal/tools/tiktok/` unit tests against an `httptest` fake for `open.tiktokapis.com` — assert request shape (Bearer header, `fields` query, post init body), `{data,error}` unwrap, and both plain + `--json` error rendering. | **No** |
| **L2** | `ANYCLI_CRED_ACCESS_TOKEN=… ANYCLI_CRED_OPEN_ID=… anycli tiktok -- user info` (and `video list`) against the **real** API. Also the stage-1 re-check of the authorize `scope` separator. | **Yes** — a real user access token minted from a **sandbox** TikTok app + a test creator account (lane 2 account pool). |
| **L3** | `provider-gen` + `provider-gen --check`; integration-service unit tests for the new `client_id_param`/`scope_separator` capability (rendered authorize URL carries `client_key` + comma scope; token form carries `client_key`); both repos' suites. | **No** |
| **L4** | singleton + `POST /internal/test-only/connections/seed` (seed `access_token` **and** rotating `refresh_token` with a short `expires_at` to force the 24 h refresh-and-write-back path) → `heliox tool tiktok -- user info`. | **Yes** — a real sandbox access/refresh token pair (gated on **dev-mode app creation**, lane 1). |
| **L5** | One full `heliox tool tiktok auth` → TikTok consent on the sandbox app → `oauth_connected` event → unseeded live run, once before the visible flip. | **Yes** — registered app + real TikTok account consent; **human-in-the-loop** (oauth L5), and the visible flip additionally waits on TikTok **audit clearance**. |

**Rollout:** land hidden (bundle `visible: false` + the two capability fields +
regen); complete L1–L4 while hidden; run L5; flip `visible: true` + regenerate as
the single go-live change **only after** TikTok's app audit clears the
`video.publish`/`video.upload` scopes. Per master-plan §6, a TikTok review stall
just leaves the tool code-complete-hidden — zero waste.
