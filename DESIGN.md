# Tool design: Reddit (`reddit`)

Per-tool design for catalog row 60 (Wave 1, Social & Media). Scratch file on
branch `tool/reddit`; the batch lead strips it at batch end.

- Axis ① CLI command word: `reddit` (flat command, no `tool.group`)
- Axis ② anycli tool id: `reddit` (`definitions/tools/reddit.json`, Go package
  `internal/tools/reddit/`)
- Axis ③ provider catalog key: `reddit` (`integrations/providers/reddit/`)
- ② == ③, so **no** `toolToProvider` entry in
  `helio-cli/internal/toolcred/resolver.go`.
- Catalog auth lane: `oauth_light` — **challenged below; recommend
  re-laning to `oauth_review`** (registration-approval gate), recorded per the
  divergence rule.

## 1. Auth lane verification against official sources (DIVERGENCE)

The audit file (`docs/design/008-300-integrations-rollout-plan/oauth-audit.md`)
has **no Reddit row** — its scope was only the 250 tools that sat in `api_key`
before the audit; Reddit was `oauth_light` from the seed catalog, so no audit
verdict exists. I verified the lane independently:

**The OAuth protocol itself is standard and light** (archived-but-canonical
official wiki, `github.com/reddit-archive/reddit/wiki/OAuth2`):

- Authorization-code flow. Authorize `https://www.reddit.com/api/v1/authorize`
  (params `client_id`, `response_type=code`, `state` required, exact-match
  `redirect_uri`, `duration`, `scope` space-separated).
- Token endpoint `https://www.reddit.com/api/v1/access_token`; exchange and
  refresh use **HTTP Basic** client auth (`client_id:client_secret`) with a
  form-encoded body → bundle `token_exchange_style: form_basic`. PKCE is not
  part of Reddit's documented flow → `pkce: none`; `state` is mandatory.
- `duration=permanent` on the authorize request is required to receive a
  `refresh_token`. Access tokens expire after **1 hour**. The refresh grant
  (`grant_type=refresh_token`) returns a new `access_token` only — **no new
  refresh token**; the original refresh token is reused until revoked.
- Revocation: `https://www.reddit.com/api/v1/revoke_token` (Basic client auth,
  form `token` + optional `token_type_hint`); revoking the refresh token also
  revokes its access tokens.
- API base for authed calls is `https://oauth.reddit.com` (token endpoints live
  on `www.reddit.com`).

**But the registration model is no longer self-serve.** Reddit's Responsible
Builder Policy (announced on r/redditdev in late 2025) closed self-service
Data API access: a **new OAuth client now requires prior approval** through
Reddit's request form (categories: developer / researcher / moderator), with
multi-week, unpredictable turnaround; already-issued clients were
grandfathered. Reddit's official Help Center language matches: the Data API is
for "approved developers", and "the information you provide … during Reddit's
App Review determines eligibility and approval"
(`support.reddithelp.com/hc/en-us/articles/14945211791892`). Commercial use
additionally "requires permission and a contract" (free tier is
non-commercial, 100 QPM per OAuth client id averaged over 10 minutes).

Under the audit rubric ("a human review … gate before external accounts can
authorize → `oauth_review`"), **Reddit belongs in `oauth_review`**, not
`oauth_light`. Caveats: `support.reddithelp.com` blocks automated fetch (403),
and the primary r/redditdev announcement was corroborated via its Help Center
paraphrase plus multiple secondary sources, not fetched verbatim — lane 1 must
confirm the exact approval path when it submits the registration.
**Recommendation for the batch lead:** amend the catalog row to `oauth_review`
(scheduling change only — registration/approval submitted at kickoff, visible
flip gated on approval + the commercial-use decision); everything technical
below is unchanged, since the wire protocol is a plain `standard_oauth` shape
either way. Also flag the commercial-terms question (Helio agents acting for
users of a paid product is plausibly "commercial use", i.e. contract +
per-call pricing territory) to lane 1 / business sign-off.

## 2. What an AI teammate does with Reddit → API surface

Driving use cases, in order of value:

1. **Community / brand monitoring & research** — watch subreddits, search for
   product or topic mentions, read a discussion (post + comment tree), check
   subreddit rules before participating. (Reddit search cannot search
   comments and has no date-range filter; the tool exposes what the API has.)
2. **Participation** — submit a text/link post, reply to posts and comments,
   edit or delete the assistant's own content.
3. **Inbox** — read mentions/replies/private messages, mark read, send a
   message (community-manager loop).
4. **Account context** — who am I, which subreddits am I subscribed to.

Deliberate exclusions:

- **Voting** (`vote` scope): Reddit's rules prohibit automated/programmatic
  voting ("votes must be cast by humans"); an agent tool must not offer it.
- Mod tooling (`mod*` scopes), wiki, flair management, save/hide — not
  teammate work; keep scope list minimal (least-privilege consent screen).

Endpoints wrapped (all on `https://oauth.reddit.com`):

| Verb | Endpoint | Scope |
|---|---|---|
| identity | `GET /api/v1/me` | `identity` |
| subreddit about / rules | `GET /r/{sub}/about`, `GET /r/{sub}/about/rules` | `read` |
| subreddit posts | `GET /r/{sub}/{hot|new|top|rising}` | `read` |
| search | `GET /search`, `GET /r/{sub}/search?restrict_sr=1` | `read` |
| post + comment tree | `GET /comments/{article}` | `read` |
| thing lookup | `GET /api/info?id=t3_…` | `read` |
| submit post | `POST /api/submit` (`api_type=json`) | `submit` |
| comment / reply | `POST /api/comment` (`api_type=json`) | `submit` |
| edit own text | `POST /api/editusertext` | `edit` |
| delete own thing | `POST /api/del` | `edit` |
| user about / posts / comments | `GET /user/{name}/about|submitted|comments` | `history` |
| inbox / unread / mentions | `GET /message/inbox|unread|mentions` | `privatemessages` |
| mark read | `POST /api/read_message` | `privatemessages` |
| send message | `POST /api/compose` | `privatemessages` |
| my subscriptions | `GET /subreddits/mine/subscriber` | `mysubreddits` |

Requested scopes: `identity read submit edit history mysubreddits
privatemessages` (7 of 19; no `vote`, no mod scopes).

## 3. anycli definition

**Stage-1 form decision: `service` type.** No official Reddit CLI exists
(snoo/PRAW are libraries; community CLIs fail the "official" bar), so the
`cli`-type rubric fails at the first clause. Implement
`internal/tools/reddit/` against the HTTP API, registered as
`RegisterService("reddit", &reddit.Service{})` in
`internal/tools/register.go` (registration rides the batch-end merge; the
package + definition JSON merge freely).

`definitions/tools/reddit.json`:

```json
{
  "name": "reddit",
  "type": "service",
  "description": "Reddit as a tool (OAuth 2.0 user token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "REDDIT_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single credential binding: unlike X (which needs `user_id` in request paths),
Reddit's own-account endpoints are self-relative (`/api/v1/me`,
`/message/inbox`), and other-user reads take the username as an argument.

**Service shape** (copy `internal/tools/x` / `notion` conventions: struct with
`APIBase`/`HC`/`Out`/`Err` for httptest injection, cobra tree with
`SilenceUsage`, persistent `--json`, exit codes 0 success / 1 API-runtime via
typed `apiError` / 2 usage):

```
reddit me
reddit subreddit about <name>
reddit subreddit rules <name>
reddit subreddit posts <name> [--sort hot|new|top|rising] [--time hour|day|week|month|year|all] [--limit N] [--after CURSOR]
reddit search --query Q [--subreddit S] [--sort relevance|hot|top|new|comments] [--time …] [--limit N] [--after CURSOR]
reddit post get <id>
reddit post comments <id> [--sort best|top|new] [--depth N] [--limit N]
reddit post create --subreddit S --title T (--text BODY | --url URL)
reddit post edit <fullname> --text BODY
reddit post delete <fullname>
reddit comment create --parent <fullname> --text BODY
reddit comment edit <fullname> --text BODY
reddit comment delete <fullname>
reddit user about <name>
reddit user posts <name> [--limit N] [--after CURSOR]
reddit user comments <name> [--limit N] [--after CURSOR]
reddit inbox list [--filter all|unread|mentions] [--limit N]
reddit inbox mark-read <fullname>...
reddit message send --to USER --subject S --text BODY
reddit subs list [--limit N] [--after CURSOR]
```

**JSON output shape.** Default: terse human-readable lines. `--json`:
provider-neutral objects with Reddit's `Listing`/`kind`+`data` envelopes
stripped. Listings emit JSONL of flat items —
`{id, fullname, title, author, subreddit, score, num_comments, created_utc,
permalink, url, selftext}` for posts, `{id, fullname, author, body, score,
parent_id, depth, created_utc}` for comments — followed by a final
`{"after": "…"}` cursor object when more pages exist. `post comments`
flattens the tree with `depth`/`parent_id` and surfaces unexpanded `more`
stubs as `{"kind":"more","count":N,"parent_id":…}` rather than silently
dropping them. Write commands echo the created thing (`id`, `fullname`,
`permalink`).

**Reddit dialect gotchas the service must own:**

- **User-Agent is mandatory**: descriptive, unique, format
  `<platform>:<app-id>:<version> (by /u/<username>)`; generic/default UAs are
  blocked or throttled. Constant in `service.go`, e.g.
  `helio:heliox-reddit-tool:v1 (by /u/<lane-1 app-owner account>)` — fill the
  handle at implementation from lane 1's registered app.
- **200-with-errors dialect**: form endpoints (`/api/submit`, `/api/comment`,
  `/api/compose`) must send `api_type=json` and check the `json.errors` array
  on an HTTP-200 response; a non-empty array is exit-1 `apiError`.
- **Rate limits**: 100 QPM per client id; on 429 surface
  `X-Ratelimit-Remaining`/`-Reset` in the error message; never auto-retry in
  a loop.
- Raw JSON quirk: pass `raw_json=1` on GETs so `&`/`<`/`>` come back
  unescaped.

## 4. Helio provider bundle (hidden-first)

`integrations/providers/reddit/provider.yaml` — a plain `standard_oauth`
bundle; no service adapter expected:

```yaml
schema: helio.provider/v1
key: reddit
go_name: Reddit

presentation:
  name: Reddit
  description_key: reddit
  consent_domain: reddit.com
  visible: false          # hidden-first; flip is the go-live change
  order: <batch lead assigns>

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.reddit.com/api/v1/authorize
    token_url: https://www.reddit.com/api/v1/access_token
    token_exchange_style: form_basic
    pkce: none
    authorize_params:
      duration: permanent      # required: yields the refresh token
    scopes: [identity, read, submit, edit, history, mysubreddits, privatemessages]
    single_active_token: false
    refresh_lease: none
    revoke:
      url: https://www.reddit.com/api/v1/revoke_token
      client_auth: basic
      token: refresh_token
      token_type_hint: refresh_token

identity:
  source: userinfo
  url: https://oauth.reddit.com/api/v1/me
  stable_key: /id
  label_candidates: [/name, /id]

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
  name: reddit
  kind: oauth
```

Notes / risks:

- `/api/v1/me` returns a flat JSON object (`id`, `name`) — JSON Pointers
  `/id` and `/name` are correct; requires the `identity` scope (in the list).
- Refresh semantics fit the standard exchanger: like Google, Reddit's refresh
  response carries no new refresh token, so the gateway must retain the
  stored one (existing behavior for `google_*`).
- **UA risk at connect time (flagged for implementation):**
  integration-service's outbound OAuth/userinfo client sets no `User-Agent`
  (grep of `go-services/integration-service/service/` finds none), so the
  userinfo identity fetch — and possibly token exchange/refresh — goes out as
  `Go-http-client`, which Reddit throttles/blocks. If L5 hits this, the right
  fix is provider-neutral: a descriptive Helio `User-Agent` on
  integration-service's outbound HTTP client (or one reviewed optional
  `identity` header field), **not** a Reddit adapter.
- No icon in the bundle: `ui/helio-app/src/integrations/icons/reddit.svg` +
  manual `providerIcons.ts` registration ride the batch-end merge.
- No `experiment` gating planned (GA once visible), unless the commercial-use
  question (§1) forces a design-090 flag.
- Config: lane 1 lands `oauth.client_id`/`oauth.client_secret` for key
  `reddit` in `config/` + the `deploy/` Helm Secret together (Config Sync);
  dev values arrive as uncommitted local `config/cloud.yaml` entries for L4.

## 5. Test plan (five layers)

| Layer | Plan | External credentials needed |
|---|---|---|
| L1 | anycli unit tests, httptest fakes per subcommand (TDD, tests first): assert path/query/form shape incl. `api_type=json` + `raw_json=1`, `Authorization: Bearer` header, the constant User-Agent header, listing/envelope stripping, `json.errors` 200-dialect → exit 1, 429 → rate-limit error with headers, `--json` and plain rendering, exit codes 0/1/2. `go test ./...` green. | None |
| L2 | Dev harness against the real API: `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli reddit -- me`, then `subreddit posts`, `search`, one `post create`+`post delete` round-trip in a private test subreddit, `inbox list`. | **Yes** — registered Reddit app (lane 1, approval-gated per §1) + pool test account (lane 2). Once the app exists, a parallel `script`-type app on the test account can mint tokens via the password grant for repeatable L2 runs without a browser. |
| L3 | Local-only `go run ./cmd/provider-gen` + `--check` against the branch bundle (regens NOT committed, per master plan §2); helio-cli built with an uncommitted local `replace github.com/heliohq/anycli => ../anycli-worktree` in `go.mod`; both repos' unit suites green. Branch CI is expected red on `provider-gen --check` until the batch-end regen. | None |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with real `access_token` **and** `refresh_token` and a deliberately short `expires_at` (Reddit has a real 1-hour refresh cycle — force the gateway's refresh-and-write-back path), then `heliox tool reddit -- me` and one read command returning live data. Real seeded org/user/assistant ids per the skill's L4 notes. | **Yes** — real tokens minted from the lane-1 dev app; dev `client_id`/`client_secret` as uncommitted local `config/cloud.yaml` entries (needed for the refresh exercise). |
| L5 | Human-in-the-loop (oauth lane): `heliox tool reddit auth` → connect link → real Reddit consent on the pool account → `oauth_connected` system event on the originating channel → one unseeded live run. Gates the visible flip; per §1, also gated on Reddit's app approval (and the commercial-terms decision) if the lane moves to `oauth_review`. | **Yes** — lane-1 app config landed in `config/` + `deploy/`, pool account, human consent session (lane 3). |

## 6. Batch-end checklist handoff (for the batch lead)

- `register.go` entry `RegisterService("reddit", …)`; anycli tag; helio-cli
  pin bump (drop the local `replace`).
- Bundle merge + single canonical `provider-gen` run (five projections).
- No `toolToProvider` entry (② == ③).
- `providerIcons.ts` append + `icons/reddit.svg`.
- Provider sub-doc under `agents/plugins/heliox/skills/tool/` + plugin version
  bump + marketplace publish.
- Catalog amendment PR proposal: row 60 `oauth_light` → `oauth_review` (§1).
