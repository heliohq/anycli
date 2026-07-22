# Pinterest — `heliox tool` provider design

Scratch design for branch `tool/pinterest` (stripped at batch-end by the batch
lead). Catalog row 203: anycli id `pinterest`, provider key `pinterest`, auth
lane `oauth_review`, Wave 3, category Social & Media.

## 0. Verdict summary (independent check vs catalog / audit)

- **Lane `oauth_review` — CONFIRMED against official docs.** Pinterest was born
  `oauth_review` in the catalog; it is **not** one of the 90 rows the 2026-07-21
  api_key audit re-laned, and it carries no row in `oauth-audit.md` (the audit
  scoped only tools that sat in `api_key` before it). The independent check
  upholds the lane: to run the standard **Authorization Code** flow against
  *other people's* Pinterest accounts, an app must move past **Trial access** —
  Pinterest reviews every app-access request ("Submit your request for trial
  access… reviewed each business day," then a further **Standard access** tier
  for higher limits) before the app may act on accounts the developer does not
  own. That human review gate before external accounts can authorize is exactly
  the `oauth_review` rubric ("a human review… before external accounts can
  authorize"). Trial/test-mode apps work against the developer's own account and
  a 24h test token **without** review — so review gates only the **visible
  flip**, not dev / L4, which is the hidden-first premise. Confirmed.
- **Standard `standard_oauth` bundle — no service-side adapter needed.**
  Pinterest OAuth is a textbook RFC-6749 authorization-code + refresh-token flow
  with **HTTP Basic client auth** — wire-identical to the already-shipped
  **Reddit** bundle (`token_exchange_style: form_basic`, `refresh_lease: none`,
  refresh-token grant). The generic `standard_oauth` runtime strategy covers it;
  no `service/adapter_*.go`.
- **②==③ (`pinterest`/`pinterest`) — no `toolToProvider` divergence entry.**
  Mechanical identity mapping; nothing added to
  `helio-cli/internal/toolcred/resolver.go`.
- **`service`-type anycli definition.** No official non-interactive `--json`
  Pinterest CLI exists to wrap (the stage-1 rubric default; 21/23 shipped
  definitions are service).
- **One capability-growth risk flagged for L2 (§3, §4):** Pinterest's
  *continuous refresh token* may require a `refresh_on=true` **token-request body
  param** that `standard_oauth`'s exchanger has no mechanism to inject
  (`AuthorizeParams` appends to the authorize URL only, not the token body). See
  §4 for why this is *likely* moot (legacy tokens are retired, so continuous is
  effectively default) and the fallback if L2 proves otherwise.

Sources verified (official Pinterest Developers docs, 2026-07):
[Set up authentication and authorization](https://developers.pinterest.com/docs/getting-started/set-up-authentication-and-authorization/) ·
[Connect your app / access tiers](https://developers.pinterest.com/docs/getting-started/connect-app/) ·
[Create boards and pins](https://developers.pinterest.com/docs/work-with-organic-content-and-users/create-boards-and-pins/) ·
[API v5 reference (api.pinterest.com/v5)](https://developers.pinterest.com/docs/api/v5/) ·
[Token debugger](https://developers.pinterest.com/docs/developer-tools/token-debugger/).

## 1. API surface wrapped — and why

An AI teammate managing a Pinterest business/creator account does what a social
manager is actually asked for: **read the account** ("whose account is this, how
many followers / pins"), **organize** ("what boards exist, make a board"),
**publish** ("pin this image to this board with this title + link"), **read
content** (list/inspect pins on a board), and **clean up** (delete a stale pin).
That maps to the v5 `user_account`, `boards` (+ board `sections`), and `pins`
resources.

Base URL: **`https://api.pinterest.com/v5`** (pinned as one service constant —
Pinterest carries the version in the path, not a header, so there is no
per-request default to leak). Auth is `Authorization: Bearer <access_token>`
(the `pina`-prefixed user token), never `?access_token=` in the query string, so
the token never lands in a URL or log.

Endpoints wrapped:

| Capability | Method + path | Scope |
|---|---|---|
| Account / profile (also identity, §3) | `GET /user_account` | `user_accounts:read` |
| List boards | `GET /boards?page_size=&bookmark=` | `boards:read` |
| Get one board | `GET /boards/{board_id}` | `boards:read` |
| Create board | `POST /boards` body `{name, description, privacy}` | `boards:write` |
| Delete board | `DELETE /boards/{board_id}` | `boards:write` |
| List board sections | `GET /boards/{board_id}/sections` | `boards:read` |
| Create board section | `POST /boards/{board_id}/sections` body `{name}` | `boards:write` |
| List pins on a board | `GET /boards/{board_id}/pins` | `pins:read` |
| List pins (account) | `GET /pins?page_size=&bookmark=` | `pins:read` |
| Get one pin | `GET /pins/{pin_id}` | `pins:read` |
| Create pin | `POST /pins` body `{board_id, media_source, title, description, link, board_section_id?}` | `pins:write` |
| Delete pin | `DELETE /pins/{pin_id}` | `pins:write` |

**Create-pin is image-URL-first (`media_source.source_type: image_url`), the
lower-friction shape — video is deferred.** Pinterest's `POST /pins` requires a
`board_id` on every pin and a `media_source`; the simplest, most reliable
`media_source` for an agent is `{"source_type":"image_url","url":"https://…"}`
(a publicly reachable image URL), mirroring the Instagram design's "public URL"
publish model. **Video pins** need the extra async `POST /media` register →
upload → poll flow (like Instagram containers) and are **deferred** — the
image-pin + board-management loop above is the full core a teammate is asked to
drive and is provable hidden-first. Ads, catalogs, analytics/insights, and the
`biz_access`/`billing` scopes are **out of scope** (they are ads-manager
surfaces, a different review posture and audience).

**Pagination is Pinterest's `bookmark` cursor, surfaced not hidden.** List
endpoints return an opaque `bookmark`; the tool exposes `--bookmark` /
`--page-size` and echoes the returned `bookmark` so the assistant drives paging
(no hidden auto-follow that could fan out unboundedly — the Code Health
efficiency lens + Helio "agency over automation").

## 2. anycli definition

- **Type: `service`** (per §0). HTTP logic lives in `internal/tools/pinterest/`
  (package `pinterest` — id has no dashes, so package name == id), registered
  `RegisterService("pinterest", &pinterest.Service{})` in
  `internal/tools/register.go`.
- **Definition** `definitions/tools/pinterest.json`: `name: "pinterest"`, one
  credential binding — `source.field: access_token` injected as env
  `PINTEREST_ACCESS_TOKEN`; the service sends it as `Authorization: Bearer`.
- **Command tree** (cobra, grouped by resource — copy the `internal/tools/notion`
  shape: a `BaseURL`/`HC`/`Out`/`Err` struct so unit tests point at an
  `httptest` server and capture stdout/stderr):

  ```
  pinterest account get                              # GET /user_account

  pinterest board  list   [--page-size N] [--bookmark C]
  pinterest board  get     <board_id>
  pinterest board  create  --name NAME [--description ...] [--privacy PUBLIC|PROTECTED|SECRET]
  pinterest board  delete  <board_id>
  pinterest board  sections <board_id>               # GET  /boards/{id}/sections
  pinterest board  add-section <board_id> --name NAME  # POST /boards/{id}/sections
  pinterest board  pins    <board_id> [--page-size N] [--bookmark C]

  pinterest pin    list    [--page-size N] [--bookmark C]
  pinterest pin    get      <pin_id>
  pinterest pin    create   --board-id ID --image-url URL
                            [--title ...] [--description ...] [--link ...] [--section-id ID]
  pinterest pin    delete   <pin_id>
  ```

  **One connection == one Pinterest account.** An authorization-code token is
  scoped to the single account that authorized it; `GET /user_account` is that
  account, so no account selector flag is needed (contrast Meta Ads' per-command
  ad-account flag). The connection identity is that account (§3).

- **JSON output & errors** (notion contract): `--json` emits a structured
  envelope; default is human-readable. Exit codes **0** success, **1**
  runtime/API failure (typed `apiError`), **2** usage/parse. Pinterest error
  bodies (`{"code":<int>,"message":"..."}`) deserialize to the typed `apiError`
  and render in both plain and `--json` form. A **401** (expired/revoked token)
  is surfaced as a distinct "reconnect needed" message so the assistant prompts
  re-auth rather than retrying blindly; a **429** (rate limit) is surfaced with
  its `message` so the assistant backs off rather than hammering.

## 3. Credential fields & auth flow (oauth_review lane, verified)

**Registration model.** One app registered at developers.pinterest.com yields an
**app ID** (client_id) + **secret key** (client_secret) and a set of registered
**redirect URIs** (exact-match enforced). New apps start at **Trial access**
(reviewed each business day) and can mint a **24h test token** to exercise
endpoints against the developer's own account before wiring the full flow;
higher volume / acting on external accounts moves to **Standard access**. Dev/
trial gates L4 + own-asset L5; the external-account review gates the **visible
flip** only — hidden-first decouples it from dev.

**OAuth endpoints (official), Basic-auth client:**
- **Authorize:** `GET https://www.pinterest.com/oauth/` — params
  `client_id, redirect_uri, response_type=code, scope, state`
  (scopes comma- or space-separated).
- **Code → token:** `POST https://api.pinterest.com/v5/oauth/token`, client
  authenticated with **HTTP Basic** (`Authorization: Basic base64(client_id:client_secret)`),
  `application/x-www-form-urlencoded` body
  `grant_type=authorization_code&code=<code>&redirect_uri=<uri>`. Response is
  JSON: `access_token` (**`pina`**-prefixed), `refresh_token` (**`pinr`**-prefixed),
  `token_type:"bearer"`, `expires_in` (**2592000** = 30 days),
  `refresh_token_expires_in`, `scope`.
- **Refresh:** same token endpoint + Basic auth, body
  `grant_type=refresh_token&refresh_token=<pinr…>`. Standard RFC-6749 refresh —
  returns a fresh 30-day access token.

**Token semantics that drive the bundle:**
- Access token **30 days**; a real refresh cycle exists → seed with a short
  `expires_at` at L4 to force the token-gateway refresh-and-write-back path (A3).
- **Continuous refresh token model.** Pinterest has **retired the legacy 365-day
  hard-limit refresh token** and now issues only the **continuous refresh token
  (60-day expiry, refreshable indefinitely)**. Practically the refresh token is
  long-lived and each `refresh_token` grant rolls the window forward — a standard
  refresh, `refresh_lease: none` (independent concurrent refreshes; Pinterest is
  not single-active-token). **Verify at L2 whether the token response rotates
  the `refresh_token`** (returns a new `pinr…` each refresh): if it does, the
  token gateway must persist the rotated value back (standard write-back of
  `refresh_token` when present — confirm the shared exchanger does so for this
  provider, as it must already for rotating-refresh providers in the catalog).
- **`refresh_on=true` caveat — the one capability risk (see §4).** Pinterest
  docs note `refresh_on` / `everlasting_refresh` "are now available to all apps"
  and "later, continuous refresh will become the default." Because the legacy
  token is already retired, continuous is *effectively* the only kind issued, so
  omitting `refresh_on` should still yield the 60-day continuous token. L2 must
  confirm this on a live exchange; if `refresh_on=true` must still be sent in the
  **token-request body**, that is a genuine (small) capability gap — §4.

**Credential fields (bundle `credential:`):** `access_token: token.access_token`,
`account_key: connection.account_key`.

**Scopes requested** (read + write for the wrapped surface):
`user_accounts:read, boards:read, boards:write, pins:read, pins:write`. The
`*:read_secret` / `*:write_secret` variants (secret boards/pins), `ads:*`,
`catalogs:*`, `billing:*`, `biz_access:*` are **omitted** — out of the §1 scope.

**Disconnect.** Pinterest v5 documents **no public OAuth token-revoke
endpoint** — so `disconnect_mode: local_only` (delete the stored credential;
no provider-side revoke call), matching the "no silent fallback / don't invent a
revoke that doesn't exist" posture (bitly precedent). If a revoke endpoint is
confirmed at L5, it can later flip to `provider_revoke` with a `revoke:` block
(reddit shape) — additive, not a break.

## 4. Helio provider bundle plan (`integrations/providers/pinterest/provider.yaml`)

**Three axes.** ① CLI command word `pinterest` (flat; **no** `tool.group` — a
future `pinterest`-family grouping is unwarranted, single tool). ② anycli id
`pinterest`. ③ provider key `pinterest`. **②==③ → no `toolToProvider` entry**,
no resolver change.

**Hidden-first:** `presentation.visible: false`. Bundle sketch (Reddit is the
direct precedent — same `form_basic` + refresh + `standard_oauth`):

```yaml
schema: helio.provider/v1
key: pinterest
go_name: Pinterest

presentation:
  name: Pinterest
  description_key: pinterest
  consent_domain: pinterest.com
  visible: false            # hidden-first; visible flip (+ order) is the go-live change

auth:
  type: oauth
  owner: individual         # the provider sees a person; the connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.pinterest.com/oauth/
    token_url: https://api.pinterest.com/v5/oauth/token
    token_exchange_style: form_basic   # HTTP Basic client auth + form-encoded body
    pkce: none                         # not part of Pinterest's documented flow; state is mandatory
    scopes: [user_accounts:read, boards:read, boards:write, pins:read, pins:write]
    display_scopes: [user_accounts:read, boards:read, boards:write, pins:read, pins:write]
    single_active_token: false
    refresh_lease: none                # standard refresh_token grant; independent concurrent refreshes

identity:
  source: userinfo
  url: https://api.pinterest.com/v5/user_account
  stable_key: /id                      # immutable account id (verify present at L2; /username is label)
  label_candidates: [/username, /business_name, /id]

connection:
  mode: isolated
  disconnect_mode: local_only          # no documented Pinterest OAuth revoke endpoint
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
  name: pinterest
  kind: oauth
```

**Config (Config Sync hard rule).** `oauth.client_id` / `oauth.client_secret`
land in integration-service config for **both** `config/` (local) and the Helm
Secret under `deploy/` in the same change; because `token_exchange_style:
form_basic` **requires a configured secret before an authorize session can be
minted** (`TokenExchangeStyle.UsesHTTPBasicClientAuth()`), a partially-configured
Pinterest provider fails startup — id and secret must land together (lane 1
per master-plan §2). Both absent → `configured: false`, Connect disabled, safe
to ship hidden.

**The one capability decision (only if L2 forces it).** If — and only if — L2
proves `refresh_on=true` must be sent in the **token-request body** to obtain the
continuous refresh token, `standard_oauth` needs a small reviewed addition: a
`token_params` map on `OAuthEndpoints` (sibling to the existing
`AuthorizeParams`, catalog.go:255) that the shared exchanger merges into the
`oauth_exchange.go` form body for both the code and refresh grants. This is a
generic, reviewed enum-style growth (one closed field), **not** a
Pinterest-specific `adapter_*.go` — flagged at stage 1 per master-plan risk
"Flag adapter/credential-kind candidates at stage 1, not mid-wave." Default
expectation: **not needed** (legacy tokens retired → continuous is default), so
the bundle above ships as-is and this stays a contingency.

**Other artifacts (batch-end merged, not in this design):**
`ui/helio-app/src/integrations/icons/pinterest.svg` + `providerIcons.ts` append;
AI-facing sub-doc under `agents/plugins/heliox/skills/tool/`; i18n
`description_key: pinterest` label.

## 5. Test plan → the five layers

| Layer | Pinterest specifics | Needs external creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/pinterest` unit tests: `httptest` fake for `/user_account`, `/boards*`, `/pins*`; assert path + `Authorization: Bearer` injection; assert `--json` vs plain rendering and the typed-error path (401 "reconnect", 429 backoff, 400 create-pin "valid image url" body); assert `bookmark`/`page_size` are forwarded and the returned `bookmark` echoed. No network. | No |
| **L2** `anycli pinterest -- …` harness vs REAL api.pinterest.com/v5 | `ANYCLI_CRED_ACCESS_TOKEN=pina_…` from a trial-access app's own account (24h test token is sufficient). Run `account get`, `board list`, `board create`, `pin create --image-url …`, `pin get`, `pin delete`. **Also captures the two verification items:** (a) raw `/user_account` body to confirm `/id` presence for `stable_key`; (b) a live code+refresh exchange to confirm the continuous-refresh shape and whether `refresh_on=true` is required + whether `refresh_token` rotates. | **Yes** (trial-access app + own Pinterest account) |
| **L3** `provider-gen --check` + both repos' unit suites | provider-gen strict-decodes the bundle, validates HTTPS URLs + `form_basic` secret requirement + `standard_oauth` reviewed strategy; helio-cli `cmd/heliox/cmds/tool` test sees the hidden provider (skipped from the pinned-anycli visible check until the pin ships). Run locally on-branch with `provider-gen`/`--check` + a `go.mod` `replace` to the anycli branch (not committed). | No |
| **L4** singleton + seed + `heliox tool pinterest -- …` | `POST /internal/test-only/connections/seed` with a real seeded assistant/org identity, `provider: pinterest`, seed **both** `access_token` and `refresh_token` with a short `expires_at` so the next call forces the token-gateway refresh-and-write-back (A3) — Pinterest has a real refresh cycle. Then `heliox tool pinterest -- account get` returns live data. | **Yes** (real Pinterest token from the L2 account) |
| **L5** full connect flow, once, still hidden, before visible flip | `heliox tool pinterest auth` → consent on `www.pinterest.com/oauth/` for the trial/standard app → confirm `oauth_connected` system event → unseeded live `pinterest account get` through the new connection. Human-in-the-loop (oauth L5 per master-plan §2 lane 3). Also the point to confirm whether a revoke endpoint exists (would flip `disconnect_mode`). | **Yes** (human consent on a real Pinterest account) |

**Externally-supplied-credential layers: L2, L4, L5** (all need a registered
Pinterest app + a real account; L1/L3 are hermetic). Per master-plan §2, dev-mode
(trial) app creation gates L4 and must precede the on-branch L4 run; review
clearance gates only the visible flip.
