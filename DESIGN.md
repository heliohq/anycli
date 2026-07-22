# Meta Ads — `heliox tool` provider design

Scratch design for branch `tool/meta-ads` (stripped at batch-end by the batch
lead). Catalog row 134: anycli id `meta-ads`, provider key `meta_ads`, auth
lane `oauth_review`, Wave 2, category Marketing.

## 0. Verdict summary (independent check vs catalog/audit)

- **Lane `oauth_review` — CONFIRMED against official docs.** The Marketing API
  is Graph-API based and uses Facebook Login OAuth 2.0. The permissions an ad
  teammate needs — `ads_read`, `ads_management`, `business_management`,
  `read_insights` — start at **Standard Access** (own/admin assets only) and
  require **App Review + Business Verification → Advanced Access** before the
  app may act on *other* businesses' ad accounts. That human review/verification
  gate is exactly the `oauth_review` rubric. `business_management` and
  `ads_management` explicitly need App Review before use outside your own
  Business Manager.
- **One material divergence to record: Meta has no OAuth2 `refresh_token`
  grant.** Facebook Login issues a short-lived user token (~1–2 h); you upgrade
  it to a long-lived token (~60 days) via a *second* call
  `GET /oauth/access_token?grant_type=fb_exchange_token`. There is no RFC-6749
  refresh grant. This is a Meta-family trait (Instagram uses `ig_exchange_token`,
  Threads `th_exchange_token`), so it wants a **reviewed capability on the
  standard exchanger**, not a per-tool adapter (§4). The catalog/audit did not
  capture this; it is the one place this tool leaves the plain `standard_oauth`
  golden path.
- Everything else matches: `service`-type anycli definition (no official
  agent-friendly CLI), assistant-owned OAuth connection, hidden-first rollout.

Sources verified: Meta Marketing API get-started + Ads Insights docs; Facebook
Login "Get Long-Lived Tokens"; Meta "Ads Management Standard Access" blog;
`graph.facebook.com/v{ver}` versioning (rolling ~2-year deprecation).

## 1. API surface wrapped — and why

An AI teammate on Meta Ads does four things a marketer asks for: **read
performance** ("how did last week's campaigns do"), **inspect structure** (which
campaigns / ad sets / ads exist and their status/budget), **change spend state**
(pause a losing ad set, resume, adjust a daily budget), and **discover which ad
account** to operate on. That maps to the Graph API's ad-object hierarchy
**Ad Account → Campaign → Ad Set → Ad**, plus the Insights edge.

Base URL: `https://graph.facebook.com/v{VERSION}` with `VERSION` pinned as a
single service constant (a recent stable version, e.g. `v23.0`; Meta deprecates
versions on a ~2-year clock, so it is one maintained constant, never
per-request-defaulted — matching the "no silent fallback" rule).

Endpoints wrapped (all are Graph nodes/edges; auth via `Authorization: Bearer`):

| Capability | Method + edge | Permission |
|---|---|---|
| List ad accounts | `GET /me/adaccounts?fields=id,name,account_status,currency,amount_spent` | `ads_read` |
| List campaigns | `GET /act_{ACCT}/campaigns?fields=…` | `ads_read` |
| Get one object | `GET /{object_id}?fields=…` (campaign/adset/ad) | `ads_read` |
| List ad sets | `GET /act_{ACCT}/adsets?fields=…` | `ads_read` |
| List ads | `GET /act_{ACCT}/ads?fields=…` | `ads_read` |
| List creatives | `GET /act_{ACCT}/adcreatives?fields=…` | `ads_read` |
| Insights (reporting) | `GET /{act_or_object}/insights?level=&date_preset=&fields=&time_range=` | `ads_read` + `read_insights` |
| Update status / budget | `POST /{object_id}` with `status` / `daily_budget` / `lifetime_budget` | `ads_management` |
| Create campaign | `POST /act_{ACCT}/campaigns` | `ads_management` |

Why these and not more: the Marketing API has hundreds of endpoints, but the
campaign/adset/ad/insights core covers the full read + lifecycle a teammate is
asked to drive. Creative composition, audience building, and pixel/catalog
management are deferred — high-friction, write-heavy, and better added once the
read+control loop is proven hidden. Insights is the highest-value read (it is
what "how are my ads doing" resolves to) so it ships in the first cut with
`level`, `date_preset`/`time_range`, and `fields` passthrough.

## 2. anycli definition

- **Type: `service`.** No official non-interactive `--json` CLI exists for the
  Marketing API (the `service` default per stage-1 rubric; 21/23 shipped
  definitions are service). HTTP logic lives in `internal/tools/metaads/`
  (package `metaads` — dashes dropped per the naming rule), registered
  `RegisterService("meta-ads", &metaads.Service{})` in `internal/tools/register.go`.
- **Definition** `definitions/tools/meta-ads.json`: `name: "meta-ads"`, one
  credential binding — `source.field: access_token` injected as env
  `META_ACCESS_TOKEN`. The service sends it as `Authorization: Bearer <token>`
  (Graph API accepts the Bearer header; preferred over `?access_token=` so the
  token never lands in a URL/log).

- **Command tree** (cobra, grouped by resource — copy the `internal/tools/notion`
  shape: `BaseURL`/`HC`/`Out`/`Err` struct so tests point at `httptest`):

  ```
  meta-ads accounts list
  meta-ads campaign list   --account act_<id> [--status ACTIVE] [--fields ...] [--limit N] [--after CURSOR]
  meta-ads campaign get    <campaign_id> [--fields ...]
  meta-ads campaign create --account act_<id> --name ... --objective ... --status PAUSED
  meta-ads campaign update <campaign_id> [--status PAUSED|ACTIVE] [--daily-budget CENTS] [--name ...]
  meta-ads adset   list    --account act_<id> [--campaign <id>] [--fields ...]
  meta-ads adset   get     <adset_id>
  meta-ads adset   update  <adset_id> [--status ...] [--daily-budget CENTS]
  meta-ads ad      list    --account act_<id> [--adset <id>] [--fields ...]
  meta-ads ad      get     <ad_id>
  meta-ads ad      update  <ad_id> [--status ...]
  meta-ads creative list   --account act_<id>
  meta-ads insights        --account act_<id> [--object <id>] [--level account|campaign|adset|ad]
                           [--date-preset last_30d | --time-range '{"since":..,"until":..}'] [--fields ...]
  ```

  **Account targeting is a per-command `--account act_<id>` flag, not connection
  state.** A Facebook user commonly has access to many ad accounts across
  several businesses; binding one account into the connection would be wrong.
  `accounts list` (`GET /me/adaccounts`) is the discovery command the assistant
  runs first, then passes `--account` explicitly. The connection identity is the
  **Facebook user**, not an ad account (§3).

- **JSON output & errors** (notion contract): `--json` emits a structured
  envelope; default is human-readable. Exit codes **0** success, **1**
  runtime/API failure, **2** usage/parse. Graph API errors deserialize to a
  typed `apiError` from the standard envelope
  `{"error":{"message,type,code,error_subcode,fbtrace_id}}` and render in both
  plain and `--json` form. `code:190` (OAuthException / expired token) is
  surfaced as a distinct "reconnect needed" message so the assistant can prompt
  re-auth rather than retry blindly.

## 3. Credential fields & auth flow (oauth_review lane, verified)

**Registration model.** One Meta App (developers.facebook.com) with the
Marketing API product added; Facebook Login provides the authorize/token
endpoints. Client credentials are the App ID / App Secret. Dev/test-mode apps
work against the developer's own Business Manager assets *without* review
(gates only L4/L5-on-own-assets); acting on external customers' ad accounts
needs Advanced Access via App Review + Business Verification (gates the **visible
flip** only — hidden-first decouples it from dev).

**OAuth endpoints (official):**
- Authorize: `https://www.facebook.com/v{VER}/dialog/oauth` — params
  `client_id, redirect_uri, response_type=code, state, scope=ads_read,ads_management,business_management,read_insights`.
- Token (code → short-lived): `GET https://graph.facebook.com/v{VER}/oauth/access_token`
  with `client_id, client_secret, redirect_uri, code`. Response is **JSON**
  `{access_token, token_type:"bearer", expires_in}` — **no `refresh_token`**.
- Long-lived upgrade (short → ~60-day):
  `GET https://graph.facebook.com/v{VER}/oauth/access_token?grant_type=fb_exchange_token&client_id&client_secret&fb_exchange_token=<short>`.

**Token semantics that drive the bundle:** there is no refresh grant. The
usable credential is the long-lived (~60-day) token; when it lapses the
connection re-consents. So `refresh_lease: none` and the connection is a
~60-day-lived grant. The token gateway serves the stored long-lived token
directly (no refresh-and-write-back cycle).

**Credential fields (bundle `credential:`):** `access_token: token.access_token`,
`account_key: connection.account_key`.

**Scopes** requested: `ads_read`, `ads_management`, `business_management`,
`read_insights`. (A read-only first cut could drop `ads_management`, but the
control loop — pause/budget — is core to the teammate value, so it is in.)

## 4. Helio provider bundle plan (`integrations/providers/meta_ads/provider.yaml`)

**Three axes.** ① CLI command word `meta-ads` (flat; no `tool.group` — a future
`meta` family group covering Facebook Pages/Instagram is deferred per master-plan
open question 2). ② anycli id `meta-ads`. ③ provider key `meta_ads`. The ②↔③
dash→underscore divergence is **mechanical** — add `"meta-ads": "meta_ads"` to
`helio-cli/internal/toolcred/resolver.go` `toolToProvider` (or it is absorbed by
OQ1's normalization if that lands pre-kickoff; today the map is required).

**Hidden-first:** `presentation.visible: false`. Bundle sketch:

```yaml
schema: helio.provider/v1
key: meta_ads
go_name: MetaAds
presentation:
  name: Meta Ads
  description_key: meta_ads
  consent_domain: facebook.com
  visible: false
auth:
  type: oauth
  owner: assistant                     # ad accounts are shared business assets
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.facebook.com/v23.0/dialog/oauth
    token_url: https://graph.facebook.com/v23.0/oauth/access_token
    token_exchange_style: form_secret  # Meta accepts form-encoded; body ignored for GET-style, client_secret carried
    pkce: none
    scopes: [ads_read, ads_management, business_management, read_insights]
    single_active_token: false
    refresh_lease: none                # no refresh grant; long-lived 60d then re-consent
    long_lived_exchange: facebook      # NEW reviewed capability — see below
identity:
  source: userinfo
  url: https://graph.facebook.com/v23.0/me?fields=id,name
  stable_key: /id
  label_candidates: [/name, /id]
connection:
  mode: isolated
  disconnect_mode: provider_revoke     # DELETE /me/permissions; fall back to local_only if the declarative revoker can't express it
  runtime_strategy: standard_oauth
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: meta-ads
  kind: oauth
```

**The one capability growth — `long_lived_exchange: facebook`.** The `standard_oauth`
exchanger currently composes a code→token exchange and (where configured) an
RFC-6749 refresh. Meta needs a **third, reviewed** move: after the
`authorization_code` exchange, perform the `fb_exchange_token` upgrade so the
stored credential is the ~60-day token rather than the ~1-hour one. Per the
skill's guidance ("first check whether the gap is really provider-specific or
whether the generic `standard_oauth` capability set should grow one more reviewed
enum value"), this is a **Meta-family** concern, not a Meta-Ads one — Instagram
(row 200), Facebook Pages (row 201), and WhatsApp (row 4) share the exact same
short→long token model. So the orthogonal decomposition is a closed reviewed
enum on the exchanger (`long_lived_exchange: none|facebook`), not a bespoke
`service/adapter_meta.go`. This mirrors prior capability-growth precedents in
this program (keap `refresh_lease`, salesforce `instance_url` capture, posthog
`form_public`) rather than the four legacy hand-compiled adapters (slack/discord/
linkedin/x). Identity (`/me` via the declarative resolver) and revoke
(`DELETE /me/permissions`) stay on the golden path — only the long-lived upgrade
is new. If review of that enum finds the upgrade too provider-shaped to
generalize, the fallback is a narrow `adapter_meta.go`; the bundle shape above is
unchanged either way.

**Config (lane 1 lands, per Config Sync):** `oauth.client_id` / `oauth.client_secret`
(App ID / App Secret) into integration-service config in **both** `config/` and
the `deploy/` Helm Secret, together (a partially-configured provider fails
startup; both-absent renders `configured:false` and is safe hidden).

**UI icon:** `ui/helio-app/src/integrations/icons/meta_ads.svg` + manual append
in `providerIcons.ts` (Meta/Facebook glyph; never generated).

## 5. Test plan → five layers (external-credential needs flagged)

| Layer | What runs | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — `httptest` fake Graph API: assert Bearer injection, `act_<id>` path building, `--account` required-flag error, insights query params, error-envelope + `--json` rendering, exit codes 0/1/2. Fake the `code:190` path. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<long-lived token> anycli meta-ads -- accounts list` then `campaign list --account act_… ` and `insights …` against the **real** Graph API. Proves field names/injection/request shape match live. | **Yes** — a real Meta app + a long-lived user (or system-user) token with `ads_read`/`read_insights` on a real ad account. |
| **L3** generate + suites | `provider-gen` + `provider-gen --check` (five projections); the new `long_lived_exchange` enum validates; helio-cli + integration-service unit suites incl. the `toolToProvider` divergence test. Branch is *expected* to fail `--check` in CI until batch-end (do not commit local regens). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"meta_ads"`, `access_token:<long-lived>` (oauth user-token providers are seedable), then `heliox tool meta-ads -- accounts list`/`insights`. **Note:** with no refresh grant, seed `access_token` only (omit `refresh_token`/short `expires_at`) — the standard refresh-and-write-back path (A3) does not apply; L4 proves the token-gateway→anycli path only. | **Yes** — real long-lived token (from lane-1 dev app), plus dev App ID/secret in local uncommitted `config/cloud.yaml`. |
| **L5** full connect | Hidden, pre-flip: `heliox tool meta-ads auth` → Facebook Login consent on the dev app → `oauth_connected` event → unseeded live `meta-ads accounts list`. Verifies the long-lived upgrade fires during the real exchange. | **Yes, human-in-the-loop** — Meta 2FA/consent defeats agent automation. Runs on **own** Business Manager assets under Standard Access (no review needed for L5 itself); **review clearance gates only the subsequent visible flip**. |

**Externally-supplied credentials required at L2, L4, L5** (a real Meta app +
long-lived token + an ad account in the test-account pool). L1/L3 need none.
Lane-1 dev-app creation gates L4; Advanced-Access review gates only the visible
flip.

## 6. Open risks

- **Advanced Access / Business Verification tail.** Standard Access is enough
  for all dev, L1–L4, and an own-asset L5; the visible flip waits on App Review
  + Business Verification (multi-day to multi-week). Hidden-first means zero code
  waste while it clears. Track in the wave board.
- **`long_lived_exchange` enum is the review artifact of this tool.** It is the
  first Meta-family OAuth tool in the plan, so it establishes the shared exchange
  capability the other three Meta tools reuse; batch-end review should look at it
  as a family capability, not a meta-ads one-off.
- **Version pin maintenance.** `v23.0` (or the then-current stable) is a single
  service constant carrying Meta's ~2-year deprecation clock; note it for the
  maintenance board, do not scatter it per-request.

## 7. As-built notes (implementation deltas vs the §4 sketch)

Two bundle fields resolved differently from the §4 sketch during
implementation; both were anticipated by the design and neither changes the
verdict (lane `oauth_review` still CONFIRMED, no official-docs contradiction):

- **`disconnect_mode: local_only`, not `provider_revoke`.** Meta's revocation is
  `DELETE /{user-id}/permissions` (or `/me/permissions`), which the declarative
  RFC-7009 revoker (POST form `token=<...>` to a fixed URL) structurally cannot
  express — a different HTTP method, a path with the user id embedded, and the
  token carried as Bearer/query rather than a `token=` form field. Per §4's own
  fallback clause the bundle ships `local_only` (no `revoke:` block; the
  generator forbids one under `local_only`). A DELETE-based declarative revoker
  is deliberately out of scope — adding it would be a second unreviewed
  capability beyond the sanctioned `long_lived_exchange`.
- **`identity.url` carries no query.** The generator forbids a query string on
  `identity.url`, so it is `https://graph.facebook.com/v23.0/me` (not
  `…/me?fields=id,name`). Graph's User node returns its default fields (`id`,
  `name`) for the bearer token, so `stable_key: /id` and `label: /name` resolve
  unchanged.
- **`long_lived_exchange` landed as a closed reviewed enum on the standard
  exchanger** (`model.OAuthLongLivedExchange` = `none|facebook`), threaded
  through the bundle manifest → validator → Go catalog projection, and applied
  in `standardOAuthExchanger.Exchange` after the code grant. This is the
  Meta-family capability the other three Meta tools (Instagram, Facebook Pages,
  WhatsApp) reuse — not a meta-ads one-off.
