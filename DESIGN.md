# Facebook Pages — `heliox tool` provider design

Scratch design for branch `tool/facebook-pages` (stripped at batch-end by the
batch lead). Catalog row 201: anycli id `facebook-pages`, provider key
`facebook_pages`, auth lane `oauth_review`, Wave 3, category Social & Media.

## 0. Verdict summary (independent check vs catalog/audit)

- **Lane `oauth_review` — CONFIRMED against official docs.** The Pages API is
  Graph-API based and uses Facebook Login OAuth 2.0. The permissions a Page
  teammate needs — `pages_show_list`, `pages_read_engagement`,
  `pages_read_user_content`, `pages_manage_posts`, `pages_manage_engagement`,
  `read_insights` — start at **Standard Access** (Pages the developer/app admin
  manages, plus roled test users) and require **Meta App Review + Business
  Verification → Advanced Access** before the app may act on Pages managed by
  users the developer has no admin relationship with. That human review gate is
  exactly the `oauth_review` rubric. Facebook Pages was **born `oauth_review`**
  in the catalog — it was never in the `api_key` lane, so the 2026-07-21 OAuth
  audit (which only re-laned api_key rows) has no verdict row for it; this
  independent check upholds the catalog lane directly against Meta's docs.
- **This is a Meta-family OAuth tool and reuses the Meta-family capability
  meta-ads (row 134) introduced — `long_lived_exchange: facebook` — with zero
  new Helio-side capability.** Facebook Login issues a short-lived user token
  (~1–2 h); the app upgrades it to a ~60-day long-lived user token via
  `GET /oauth/access_token?grant_type=fb_exchange_token`. There is no RFC-6749
  refresh grant. This is identical to meta-ads — same host (`graph.facebook.com`),
  same grant, same `refresh_lease: none` / re-consent-at-~60-days lifecycle. So
  the bundle rides the exact reviewed enum meta-ads adds; **no capability growth
  is owned by this tool** (contrast Instagram, which needed a *second* enum value
  `instagram` for its distinct `ig_exchange_token`/`graph.instagram.com` host and
  a genuine refresh grant — Facebook has neither).
- **The one material divergence to record — and it lives entirely inside the
  anycli service, not the Helio bundle: the Page access-token two-hop.** Unlike
  meta-ads (where the user token operates on ad accounts directly), Page-scoped
  actions — above all **publishing** — require a **Page access token**, a
  distinct token per Page returned by `GET /me/accounts`. The connection stores
  the **user** token; the service derives the Page token on demand (§1/§2). This
  keeps the credential model identical to meta-ads (connection == Facebook user)
  while absorbing the two-hop as a pure service-implementation concern — no
  extra stored credential, no per-Page connection, no Helio capability.
- Everything else matches the catalog: `service`-type anycli definition (no
  official agent-friendly CLI), assistant-owned OAuth connection, hidden-first
  rollout, and a **mechanical** ②↔③ dash→underscore divergence
  (`facebook-pages` → `facebook_pages`) that needs one `toolToProvider` entry
  (or is absorbed by OQ1's normalization if that lands pre-kickoff).

Sources verified (official Meta for Developers docs, cross-checked live):
[Pages API — Get Started](https://developers.facebook.com/docs/pages-api/getting-started/) ·
[Pages API overview](https://developers.facebook.com/docs/pages-api/) ·
[Page access tokens / `/me/accounts`](https://developers.facebook.com/docs/pages-api/getting-started/) ·
[Facebook Login — Get Long-Lived Tokens](https://developers.facebook.com/docs/facebook-login/guides/access-tokens/get-long-lived/) ·
[Graph API `/{page-id}/feed`](https://developers.facebook.com/docs/graph-api/reference/page/feed/) ·
[Permissions reference](https://developers.facebook.com/docs/permissions/).

## 1. API surface wrapped — and why

An AI teammate running a Facebook Page does five things a social manager is
actually asked for: **find which Page to operate on** (a user often admins
several), **read the Page** ("how many followers / what have we posted"),
**publish** ("post this update / link"), **do community management** (read the
comments on a post, reply, hide/delete spam), and **read Page insights** ("how
did the Page do this week"). That maps to the Graph API's **User → Pages →
Page → {feed, post, comments, insights}** hierarchy.

Base URL: `https://graph.facebook.com/v{VERSION}` with `VERSION` pinned as a
single service constant (a recent stable version, e.g. `v23.0`; Meta deprecates
versions on a ~2-year clock, so it is one maintained constant, never
per-request-defaulted — matching the "no silent fallback" rule). Auth is
`Authorization: Bearer <token>` (preferred over `?access_token=` so no token
lands in a URL or log).

Endpoints wrapped:

| Capability | Method + edge | Token used | Permission |
|---|---|---|---|
| List Pages (discovery) | `GET /me/accounts?fields=id,name,category,tasks,access_token` | **user** | `pages_show_list` |
| Get Page profile | `GET /{page_id}?fields=name,about,category,fan_count,followers_count,link,username` | page | `pages_read_engagement` |
| List Page posts | `GET /{page_id}/feed?fields=id,message,created_time,permalink_url,status_type[,shares]` | page | `pages_read_engagement` |
| Get one post | `GET /{post_id}?fields=…` | page | `pages_read_engagement` |
| Publish post | `POST /{page_id}/feed` body `message`[, `link`] | page | `pages_manage_posts` |
| Edit post | `POST /{post_id}` body `message` | page | `pages_manage_posts` |
| Delete post | `DELETE /{post_id}` | page | `pages_manage_posts` |
| List comments | `GET /{post_id}/comments?fields=id,message,from,created_time,like_count` | page | `pages_read_user_content` |
| Reply to comment | `POST /{comment_id}/comments` body `message` | page | `pages_manage_engagement` |
| Hide / unhide comment | `POST /{comment_id}` body `is_hidden=true|false` | page | `pages_manage_engagement` |
| Delete comment | `DELETE /{comment_id}` | page | `pages_manage_engagement` |
| Page insights | `GET /{page_id}/insights?metric=…&period=day[&since=&until=]` | page | `read_insights` + `pages_read_engagement` |

**The Page access-token two-hop is the defining shape and is absorbed inside the
service.** `GET /me/accounts` returns, for each Page the user roles on, its `id`,
`tasks` (e.g. `CREATE_CONTENT`, `MODERATE_CONTENT`, `MANAGE`), and a per-Page
`access_token`. Content creation (`POST /{page_id}/feed`) **requires** that Page
token — the user token is rejected for publishing — and Meta returns Page tokens
derived from a **long-lived** user token as **non-expiring** Page tokens. So:

- The teammate calls `pages list` first (discovery), exactly like meta-ads'
  `accounts list`, then targets a Page with `--page <page_id>` on every other
  command.
- For any Page-scoped command, the service resolves the Page token on demand —
  `GET /{page_id}?fields=access_token` using the stored **user** token — and uses
  it for the actual call. The Page token is **never** surfaced on the command
  line, in `--json` output, or in logs (it is an internal, single-request
  value). This is the "do it the way a human would, but keep secrets out of the
  transcript" call: the assistant reasons in Page ids, not tokens.
- Uniform Page-token use avoids per-endpoint branching (some reads accept the
  user token, but publishing and much comment moderation do not); resolving once
  and using the Page token for every Page-scoped op is always correct and one
  mental model.

Why these and not more: Photo/video/scheduled/Reel publishing, Page settings
(`pages_manage_metadata`), webhooks, and ratings/mentions reads are **deferred**
— text+link publishing plus the read/comment/insights loop is the full core a
Page teammate is asked to drive, and is proven hidden-first before the
write-heavy media surface is added. (Note: pure link-preview posting has been
tightening on Meta's side; `message`+`link` on `/feed` remains the supported
baseline and is what L2 confirms live.)

## 2. anycli definition

- **Type: `service`.** No official non-interactive `--json` CLI exists for the
  Pages API (the `service` default per stage-1 rubric; 21/23 shipped definitions
  are service). HTTP logic lives in `internal/tools/facebookpages/` (package
  `facebookpages` — dashes dropped per the naming rule, matching
  `microsoft-calendar`→`microsoftcalendar`), registered
  `RegisterService("facebook-pages", &facebookpages.Service{})` in
  `internal/tools/register.go`.
- **Definition** `definitions/tools/facebook-pages.json`: `name: "facebook-pages"`,
  one credential binding — `source.field: access_token` (the long-lived **user**
  token) injected as env `FACEBOOK_ACCESS_TOKEN`; the service sends it as
  `Authorization: Bearer` and uses it to derive Page tokens (§1).
- **Command tree** (cobra, grouped by resource — copy the `internal/tools/notion`
  shape: a `BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest`
  server and capture output):

  ```
  facebook-pages pages   list                                  # GET /me/accounts (discovery)
  facebook-pages page    get      --page <page_id> [--fields ...]

  facebook-pages post    list     --page <page_id> [--limit N] [--after CURSOR] [--fields ...]
  facebook-pages post    get      --page <page_id> <post_id> [--fields ...]
  facebook-pages post    create   --page <page_id> --message ... [--link URL]     # -> post id
  facebook-pages post    update   --page <page_id> <post_id> --message ...
  facebook-pages post    delete   --page <page_id> <post_id>

  facebook-pages comment list     --page <page_id> <post_id> [--fields ...]
  facebook-pages comment reply    --page <page_id> <comment_id> --message ...
  facebook-pages comment hide     --page <page_id> <comment_id> [--hidden true|false]
  facebook-pages comment delete   --page <page_id> <comment_id>

  facebook-pages insights          --page <page_id> [--metrics page_impressions,page_post_engagements,page_fans]
                                   [--period day] [--since UNIX] [--until UNIX]
  ```

  **`--page <page_id>` is a required flag on every command except `pages list`,
  not connection state.** A Facebook user commonly admins several Pages; binding
  one Page into the connection would be wrong (mirrors meta-ads' `--account`).
  `pages list` is the discovery command the assistant runs first; the connection
  identity is the **Facebook user**, not a Page (§3). The service transparently
  swaps the user token for the Page token behind that `--page` value.

- **JSON output & errors** (notion contract): `--json` emits a structured
  envelope; default is human-readable. Exit codes **0** success, **1**
  runtime/API failure, **2** usage/parse. Graph errors deserialize to a typed
  `apiError` from the standard envelope
  `{"error":{"message,type,code,error_subcode,fbtrace_id}}` and render in both
  plain and `--json` form. `code:190` (OAuthException / expired or revoked
  token) is surfaced as a distinct "reconnect needed" message so the assistant
  prompts re-auth rather than retrying blindly; `code:200`/`error_subcode`
  permission errors (a Page the user lacks `CREATE_CONTENT` on, or a missing
  Advanced-Access scope) render a distinct "insufficient Page permission"
  message rather than a generic failure. The two-hop's first leg failing (the
  Page token fetch) is reported as such, so the assistant can distinguish "wrong
  Page id / no access to that Page" from "the post call itself failed."

## 3. Credential fields & auth flow (oauth_review lane, verified)

**Registration model.** One Meta App (developers.facebook.com) with **Facebook
Login** configured and the Pages permissions declared; client credentials are
the **App ID / App Secret**. Redirect URIs are registered in the dashboard.
Dev/test-mode apps work against the developer's own Pages and roled **test
users** *without* review (gates only L4 / own-asset L5); acting on Pages managed
by arbitrary external users needs Advanced Access via **App Review + Business
Verification** (gates the **visible flip** only — hidden-first decouples it from
dev). This is the same App-Review track meta-ads and Instagram sit behind.

**OAuth endpoints (official):**
- **Authorize:** `https://www.facebook.com/v{VER}/dialog/oauth` — params
  `client_id, redirect_uri, response_type=code, state,
  scope=pages_show_list,pages_read_engagement,pages_read_user_content,pages_manage_posts,pages_manage_engagement,read_insights`.
- **Code → short-lived user token:**
  `GET https://graph.facebook.com/v{VER}/oauth/access_token` with
  `client_id, client_secret, redirect_uri, code`. Response is **JSON**
  `{access_token, token_type:"bearer", expires_in}` — **no `refresh_token`**.
- **Short → long-lived (~60-day) user token:**
  `GET https://graph.facebook.com/v{VER}/oauth/access_token?grant_type=fb_exchange_token&client_id=…&client_secret=…&fb_exchange_token=<short>`.
- **Page tokens** are not part of the OAuth exchange — they are fetched at
  runtime by the service via `GET /me/accounts` / `GET /{page_id}?fields=access_token`
  using the stored long-lived user token (§1). Page tokens minted from a
  long-lived user token **do not expire** (they lapse only if the user token is
  invalidated, the user changes their password, or admin access is removed).

**Token semantics that drive the bundle:**
- There is **no RFC-6749 `refresh_token` grant** — identical to meta-ads. The
  stored, usable credential is the ~60-day long-lived **user** token; when it
  lapses the connection re-consents. `refresh_lease: none`. The token gateway
  serves the stored long-lived user token directly (no refresh-and-write-back).
- The `fb_exchange_token` short→long upgrade is performed once during connect by
  the shared exchanger's `long_lived_exchange: facebook` capability — the same
  reviewed enum value meta-ads introduced (§4). No Instagram-style refresh loop.

**Credential fields (bundle `credential:`):** `access_token: token.access_token`
(long-lived user token), `account_key: connection.account_key`.

**Scopes** requested: `pages_show_list`, `pages_read_engagement`,
`pages_read_user_content`, `pages_manage_posts`, `pages_manage_engagement`,
`read_insights`. (`pages_manage_metadata` is omitted — Page settings/webhooks
are deferred, §1. A read-only first cut could drop the two `manage` scopes, but
publish + moderation is the core teammate value, so they are in.)

## 4. Helio provider bundle plan (`integrations/providers/facebook_pages/provider.yaml`)

**Three axes.** ① CLI command word `facebook-pages` (flat; no `tool.group` — a
future `meta` family group covering Facebook Pages / Instagram / Meta Ads is
deferred per master-plan open question 2, same posture as meta-ads and
instagram). ② anycli id `facebook-pages`. ③ provider key `facebook_pages`. The
②↔③ dash→underscore divergence is **mechanical** — add
`"facebook-pages": "facebook_pages"` to `helio-cli/internal/toolcred/resolver.go`
`toolToProvider` (or it is absorbed by OQ1's normalization if that lands
pre-kickoff; today the map is required).

**Hidden-first:** `presentation.visible: false`. Bundle sketch (mirrors the
meta-ads as-built bundle, §7 of that design — same host, same capability, same
identity/disconnect resolution):

```yaml
schema: helio.provider/v1
key: facebook_pages
go_name: FacebookPages
presentation:
  name: Facebook Pages
  description_key: facebook_pages
  consent_domain: facebook.com
  visible: false
auth:
  type: oauth
  owner: assistant                     # Pages are shared business assets
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.facebook.com/v23.0/dialog/oauth
    token_url: https://graph.facebook.com/v23.0/oauth/access_token
    token_exchange_style: form_secret
    pkce: none
    scopes: [pages_show_list, pages_read_engagement, pages_read_user_content,
             pages_manage_posts, pages_manage_engagement, read_insights]
    single_active_token: false
    refresh_lease: none                # no refresh grant; long-lived 60d then re-consent
    long_lived_exchange: facebook      # REUSE of meta-ads' reviewed enum value
identity:
  source: userinfo
  url: https://graph.facebook.com/v23.0/me   # generator forbids a query string here
  stable_key: /id                            # Facebook user id (Graph returns default id,name)
  label_candidates: [/name, /id]
connection:
  mode: isolated
  disconnect_mode: local_only          # Meta revoke is DELETE /{user-id}/permissions —
                                       # not expressible by the declarative RFC-7009 revoker
  runtime_strategy: standard_oauth
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: facebook-pages
  kind: oauth
```

**No capability growth is owned by this tool.** `long_lived_exchange: facebook`
is the closed reviewed enum value meta-ads (Wave 2) lands on the standard
exchanger (`model.OAuthLongLivedExchange = none|facebook`, threaded manifest →
validator → Go catalog projection → applied in `standardOAuthExchanger.Exchange`
after the code grant). Facebook Pages is Wave 3, so that value is expected to be
on `main` already; this bundle simply references it.

- **Sequencing note (batch lead):** if, for any reason, Facebook Pages develops
  **before** meta-ads/instagram have merged `long_lived_exchange` to `main`, the
  enum must land with this tool's batch (identical patch to meta-ads' §4/§7).
  Verify at stage 1 whether `OAuthLongLivedExchange`'s `facebook` value is on the
  worktree base before assuming pure reuse. It is a **reuse**, not a re-invention
  — do not add a second enum value or a `service/adapter_facebook.go`; the whole
  point of meta-ads establishing this as a *family* capability is that Facebook
  Pages, Instagram, and WhatsApp share it.

- **`disconnect_mode: local_only` (not `provider_revoke`), settled by
  precedent.** Meta revocation is `DELETE /{user-id}/permissions`, which the
  declarative RFC-7009 revoker (POST form `token=<…>` to a fixed URL) structurally
  cannot express — different method, user-id in the path, token as Bearer not a
  `token=` form field. Meta-ads resolved to `local_only` for exactly this reason
  (§7 as-built); Facebook Pages inherits that resolution. A DELETE-based
  declarative revoker is deliberately out of scope (it would be a second
  unreviewed capability).

**Config (lane 1 lands, per Config Sync):** `oauth.client_id` /
`oauth.client_secret` (App ID / App Secret) into integration-service config in
**both** `config/` and the `deploy/` Helm Secret, together (a partially
configured provider fails startup; both-absent renders `configured:false` and is
safe hidden). The **same Meta app** can carry Facebook Login + Marketing +
Instagram products, but each Helio provider bundle takes its **own** client
id/secret config entry — do not share one config field across the three
providers.

**UI icon:** `ui/helio-app/src/integrations/icons/facebook_pages.svg` + manual
append in `providerIcons.ts` (Facebook "f" glyph, visually distinct from the
`meta_ads` and `instagram` marks; never generated). Add the `facebook_pages`
i18n label/description alongside.

## 5. Test plan → five layers (external-credential needs flagged)

| Layer | What runs | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — `httptest` fake Graph API: assert Bearer injection, `pages list`→`/me/accounts` shape, **the Page-token two-hop** (`GET /{page_id}?fields=access_token` fires with the user token, and the subsequent op carries the *Page* token — assert the header actually swapped, and that the page token never appears in stdout/`--json`), `--page` required-flag error, publish `POST /{page_id}/feed` body, comment hide `is_hidden`, insights query params, error-envelope + `--json` rendering, exit codes 0/1/2, and the `code:190` / `code:200` mapped messages. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<long-lived user token> anycli facebook-pages -- pages list`, then `post create --page <id> --message "…"`, `post list`, `comment list`, `insights` against the **real** Graph API. Proves the two-hop, field names, injection, and request shapes match live — especially that publishing genuinely needs and works with the derived Page token. | **Yes** — a real Meta app + a long-lived user token with the Page scopes on a real Page the tester admins. |
| **L3** generate + suites | `provider-gen` + `provider-gen --check` (five projections); the `long_lived_exchange: facebook` reference validates against the existing enum; helio-cli + integration-service unit suites incl. the `toolToProvider` `facebook-pages`→`facebook_pages` divergence test. Branch is *expected* to fail `--check` in CI until batch-end (do not commit local regens). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"facebook_pages"`, `access_token:<long-lived user token>` (oauth user-token providers are seedable), then `heliox tool facebook-pages -- pages list` and `post list --page <id>`. **Note:** with no refresh grant, seed `access_token` only (omit `refresh_token`/short `expires_at`) — the standard refresh-and-write-back path (A3) does not apply; L4 proves the token-gateway→anycli→two-hop path only. | **Yes** — real long-lived user token (from lane-1 dev app), plus dev App ID/secret in local uncommitted `config/cloud.yaml`. |
| **L5** full connect | Hidden, pre-flip: `heliox tool facebook-pages auth` → Facebook Login consent on the dev app (grant the Page scopes, select the Page) → `oauth_connected` event → unseeded live `facebook-pages pages list` + a `post create` on an **own** Page. Verifies the `fb_exchange_token` long-lived upgrade fires during the real exchange and that the two-hop publishes for real. | **Yes, human-in-the-loop** — Meta 2FA/consent + the Page-selection consent dialog defeat agent automation. Runs on **own** Pages under Standard Access (no review needed for L5 itself); **review clearance gates only the subsequent visible flip**. |

**Externally-supplied credentials required at L2, L4, L5** (a real Meta app +
long-lived user token + a Facebook Page the tester admins, from the test-account
pool). L1/L3 need none. Lane-1 dev-app creation gates L4; Advanced-Access review
+ Business Verification gates only the visible flip.

## 6. Open risks

- **Advanced Access / Business Verification tail.** Standard Access covers all
  dev, L1–L4, and an own-Page L5; the visible flip waits on App Review + Business
  Verification (multi-day to multi-week; the Pages `manage`/`read_user_content`
  scopes each need per-scope screencast justification). Hidden-first means zero
  code waste while it clears. Track in the wave board. This is the shared Meta
  review track — batching Facebook Pages' registration with meta-ads/instagram
  under one Business Verification is a lane-1 efficiency worth noting.
- **Page-token two-hop is the review-worthy artifact of *this* tool.** It carries
  no Helio capability, but it is the one non-obvious correctness point: L1 must
  prove the token actually swaps and never leaks, and L2 must prove publishing
  fails with the user token and succeeds with the derived Page token. A reviewer
  should look at the swap seam, not just the command tree.
- **Publishing surface tightening.** Meta has periodically restricted link-
  preview and certain Page-publishing behaviors. The `message`(+`link`) `/feed`
  baseline is the durable core; media/scheduled/Reel publishing is deferred
  precisely so a mid-review API change touches the smallest surface. Confirm the
  `/feed` contract live at L2 against the pinned `v23.0`.
- **Version pin maintenance.** `v23.0` (or the then-current stable) is a single
  service constant carrying Meta's ~2-year deprecation clock; note it for the
  maintenance board and keep it aligned with the meta-ads/instagram services, do
  not scatter it per-request.
- **`long_lived_exchange` dependency ordering.** Pure reuse *iff* meta-ads/
  instagram have merged the enum to `main` first (expected: they are Wave 2 /
  earlier Wave 3). If not, the enum patch lands with this batch — verify at
  stage 1 (§4 sequencing note).
```
