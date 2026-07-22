# Twitch — per-tool design (`heliox tool twitch`)

Scratch design for the Twitch external tool provider, per the
`helio-tool-provider` pipeline. Batch lead strips this at batch-end.

- **Catalog row 206.** anycli id `twitch` (axis ②) · provider key `twitch`
  (axis ③) · CLI word `twitch` (axis ①) · auth lane `oauth_light` · Wave 3 ·
  Social & Media.
- **Naming: identity across all three axes** (`twitch` == `twitch` ==
  `twitch`). No `toolToProvider` divergence entry — the resolver map is only
  for id≠key pairs (design 308). Nothing to add to
  `helio-cli/internal/toolcred/resolver.go`.
- **Go package name (stage 2):** `internal/tools/twitch/` — the id has no
  dashes or leading digit, so package == id.

## 1. Auth-lane verification against official docs

The master catalog and the OAuth audit both assign `twitch` to `oauth_light`.
Twitch is **not** a row in `oauth-audit.md` (that file re-audited only the
250 pre-audit `api_key` tools); Twitch was `oauth_light` from the original
catalog. Verified independently against Twitch's official docs:

- **Registration is fully self-serve, no review gate.** Apps are created in
  the Twitch Developer Console (`https://dev.twitch.tv/console/apps`); the
  developer receives a Client ID immediately and generates a Client Secret,
  and registers one or more OAuth Redirect URLs. Any Twitch user can then run
  the authorization-code consent for that one registered app — there is **no
  publisher verification, marketplace listing, or app-review gate** before
  external accounts can authorize. This matches the `oauth_light` rubric
  (multi-tenant authorization-code OAuth, self-serve, no review).
  Source: `https://dev.twitch.tv/docs/authentication/register-app/`.
- **Verdict: `oauth_light` confirmed — no divergence from the catalog.**
  The only friction is exact redirect-URI registration and standard Helix
  rate limits; neither is a review clock. Twitch does gate *some* elevated
  activities (e.g. sending chat requires the app/user to meet Twitch chat
  eligibility, and higher rate tiers exist), but nothing gates the OAuth
  connect itself, so hidden-first + a normal L5 consent applies.

## 2. Official API surface this tool wraps, and why

**Base:** `https://api.twitch.tv/helix` (the Helix REST API). Every Helix
request carries **both** `Authorization: Bearer <user-token>` **and**
`Client-Id: <client_id>` — the missing-`Client-Id` requirement is the single
most load-bearing fact of this design (see §5).

Driven by what an AI teammate actually does for a streamer / community
manager — read channel state, update stream metadata, see who is live and
who follows, pull clips/VODs, and talk in chat — the tool wraps a focused,
resource-grouped slice of Helix (not the whole surface):

| Verb group | Helix endpoint | Method | Scope (user token) |
|---|---|---|---|
| `user` (self / lookup) | `/helix/users` | GET | none (public) |
| `channel get` | `/helix/channels` | GET | none |
| `channel update` | `/helix/channels` | PATCH | `channel:manage:broadcast` |
| `stream list` | `/helix/streams` | GET | none |
| `stream followed` | `/helix/streams/followed` | GET | `user:read:follows` |
| `search channels` | `/helix/search/channels` | GET | none |
| `clip list` | `/helix/clips` | GET | none |
| `clip create` | `/helix/clips` | POST | `clips:edit` |
| `video list` | `/helix/videos` | GET | none |
| `follower list` | `/helix/channels/followers` | GET | `moderator:read:followers` |
| `subscriber list` | `/helix/subscriptions` | GET | `channel:read:subscriptions` |
| `chat send` | `/helix/chat/messages` | POST | `user:write:chat` |
| `chatters` | `/helix/chat/chatters` | GET | `moderator:read:chatters` |

**Why these and not more.** This set covers the three things a Twitch
assistant is asked to do — *observe* (streams/search/clips/videos/followers/
subscribers), *curate the channel* (title/game/tags via PATCH channels,
create clips), and *participate* (send a chat message, list chatters). Left
out on purpose for a first cut: EventSub subscriptions (push/webhook plane —
not a request/response tool shape), channel-points custom rewards
(`/helix/channel_points/custom_rewards` — narrower audience; a clean
follow-up `reward` group), moderation/ban/timeout writes (elevated,
higher-risk), and ad/commercial controls. These are additive verb groups, not
reasons to reshape the tool.

**`broadcaster_id` convenience.** Most channel-scoped endpoints require a
`broadcaster_id` query param that is the caller's own user id. The service
resolves "self" by calling Get Users (which returns the authenticated user
when no `id`/`login` is given) and caching that id for the process, so
`channel get`, `channel update`, `subscriber list`, `follower list`, and
`chatters` work without the AI having to first look up its own id. Explicit
`--broadcaster-id` / `--login` overrides target another channel.

## 3. anycli definition (stage 1–2)

- **Tool form: `service` type** (the default; 21/23 current definitions are
  service). No official Twitch CLI binary is non-interactive, `--json`-
  capable, and image-provisionable, so the `cli` rubric fails — implement
  against the Helix REST API in `internal/tools/twitch/`.
- **Definition** `definitions/tools/twitch.json` (`name: "twitch"`,
  `type: "service"`). **Two** credential bindings (this is the non-standard
  bit — most service tools inject one):

```json
{
  "name": "twitch",
  "type": "service",
  "description": "Twitch as a tool (Helix API / OAuth user token)",
  "auth": {
    "credentials": [
      { "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "TWITCH_TOKEN"} },
      { "source": {"field": "client_id"},
        "inject": {"type": "env", "env_var": "TWITCH_CLIENT_ID"} }
    ]
  }
}
```

  The service sets `Authorization: Bearer $TWITCH_TOKEN` **and**
  `Client-Id: $TWITCH_CLIENT_ID` on every Helix request. Registered in
  `internal/tools/register.go` as `RegisterService("twitch", &twitch.Service{})`.

- **Service shape (copy `internal/tools/notion/`):** a cobra tree grouped by
  resource (`user`, `channel`, `stream`, `search`, `clip`, `video`,
  `follower`, `subscriber`, `chat`) with a `BaseURL`/`HC`/`Out`/`Err` struct
  so unit tests point at an `httptest` server and capture stdout/stderr.
  Exit-code contract per notion: `0` success, `1` runtime/API failure (typed
  `apiError`, `--json` error envelope), `2` usage/parse error. Twitch's error
  body is `{"error","status","message"}` — map `status`→exit path and surface
  `message`.
- **Pagination:** Helix uses opaque cursor pagination
  (`pagination.cursor` + `?after=`). List verbs accept `--first N` and
  `--after <cursor>` and echo the returned cursor in the JSON envelope so the
  AI can page.
- **JSON output shape:** provider-neutral. List verbs emit
  `{"data":[...],"cursor":"<next or empty>"}`; single-object verbs emit the
  Helix object unwrapped from its `data[0]` array (Helix wraps even singletons
  in `data`). `--json` errors emit `{"error":{"code","message"}}`.
- **L1/L2 (TDD, anycli AGENTS.md):** httptest fakes assert request path,
  method, the injected `Authorization: Bearer` **and** `Client-Id` headers,
  query params (`broadcaster_id`, `first`, `after`), and both plaintext and
  `--json` error rendering. L2 runs the dev harness against the **real**
  Helix API with `ANYCLI_CRED_ACCESS_TOKEN` + `ANYCLI_CRED_CLIENT_ID` from a
  real Twitch app + user token before the pin bump.

## 4. Exact OAuth flow (verified against official docs)

Standard OAuth 2.0 **authorization-code grant** (confidential client with a
stored secret). Endpoints (`id.twitch.tv`, distinct from the `api.twitch.tv`
Helix host):

- **Authorize:** `https://id.twitch.tv/oauth2/authorize`
  — params: `client_id`, `redirect_uri`, `response_type=code`,
  `scope` (space-delimited, URL-encoded), `state` (CSRF), optional
  `force_verify`.
- **Token:** `https://id.twitch.tv/oauth2/token`
  — POST `x-www-form-urlencoded` body: `client_id`, `client_secret`, `code`,
  `grant_type=authorization_code`, `redirect_uri`. **`client_secret` is in
  the POST body; there is no HTTP Basic variant** → `token_exchange_style:
  form_secret` (same as bitly). **PKCE is not part of Twitch's auth-code
  flow** → `pkce: none`.
- **Token response:** `{access_token, refresh_token, expires_in,
  scope:[...], token_type:"bearer"}`. User tokens expire (`expires_in` is
  seconds, ~4h) and a `refresh_token` is issued for confidential clients →
  `refresh_lease: provider` (the gateway refreshes via the refresh token and
  persists whatever new `refresh_token` Twitch returns).
- **Refresh:** `https://id.twitch.tv/oauth2/token` with
  `grant_type=refresh_token` — handled entirely by the `standard_oauth`
  gateway; no provider code.
- **Revoke (optional):** `https://id.twitch.tv/oauth2/revoke` (POST
  `client_id` + `token`). See disconnect note in §5.

**Credential fields:** the connection stores the user access token; the
public `client_id` is *config*, not a token.

## 5. Helio provider bundle + the one capability growth

Bundle `integrations/providers/twitch/provider.yaml`, **`presentation.visible:
false`** (hidden-first). Sketch:

```yaml
schema: helio.provider/v1
key: twitch
go_name: Twitch
presentation:
  name: Twitch
  description_key: twitch
  consent_domain: twitch.tv
  visible: false            # hidden-first; flip only after L5
  order: <batch-assigned>
auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://id.twitch.tv/oauth2/authorize
    token_url: https://id.twitch.tv/oauth2/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [channel:manage:broadcast, channel:read:subscriptions,
             moderator:read:followers, moderator:read:chatters,
             user:read:follows, clips:edit, user:write:chat]
    single_active_token: false
    refresh_lease: provider
identity:
  source: userinfo
  url: https://id.twitch.tv/oauth2/validate
  stable_key: /user_id
  label_candidates: [/login, /user_id]
connection:
  mode: isolated
  disconnect_mode: local_only     # see disconnect note
  runtime_strategy: standard_oauth
resources: {selection: none, discovery: none, enforcement: none}
credential:
  fields:
    access_token: token.access_token
    client_id: config.oauth.client_id   # NEW CredentialSource — see below
    account_key: connection.account_key
tool: {name: twitch, kind: oauth}
```

### 5a. Identity — zero capability growth (the clean part)

The Helix Get Users identity call would need the `Client-Id` header, but
`declarative_identity.go`'s `fetchUserInfo` sends **only**
`Authorization: Bearer` and supports no extra header. Rather than grow the
identity resolver, use Twitch's **token-validation endpoint** as the
`userinfo` source: `GET https://id.twitch.tv/oauth2/validate` accepts
`Authorization: Bearer <token>` (docs explicitly: "You may also use the
Bearer prefix in place of OAuth"), needs **no `Client-Id` header and no extra
scope**, and returns `{client_id, login, user_id, scopes, expires_in}`. The
stock `declarativeIdentityResolver` handles it verbatim — `stable_key:
/user_id`, `label_candidates: [/login]`. This is the orthogonal win: identity
rides an existing capability, so no `openid`/OIDC scope is added and no
resolver code changes.

### 5b. Runtime — one capability growth: `config.oauth.client_id` source

Every Helix call needs `Client-Id`, and that value is the OAuth app's public
`client_id`, which lives in integration-service config as `oauth.client_id`.
The closed `CredentialSource` allowlist
(`go-services/integration-service/model/catalog.go`) today is only
`token.access_token`, `connection.account_key`,
`connection.metadata.person_urn`, `credential.app_id`, `credential.brand` —
**no `config.*` source**. So projecting `client_id` into the anycli
credential map requires **growing the allowlist by one reviewed enum value**,
`config.oauth.client_id`, plus token-gateway wiring to read the provider's
configured client id and emit it as the `client_id` credential field.

This is the **same class of growth as google_ads' `config.developer_token`
("Option A", catalog task #415)** — a public, non-secret config value the
tool needs at request time, projected through the gateway rather than stored
per-connection. (google_ads' branch is not on this worktree base, so on-branch
this lands as net-new growth; if the sibling branch merges first, the
batch lead reconciles to a shared `config.<field>` source mechanism — flag at
stage 1 so it isn't duplicated.) The generator's credential-source safety
check must accept the new enum; it is safe because `client_id` is public
(it is even returned by the unauthenticated-consent URL).

No compiled `service/adapter_*.go` is needed: exchange (`form_secret`),
refresh (`provider` lease), and identity (declarative `userinfo` against
`/validate`) all fall inside the `standard_oauth` capability set. The **only**
Helio-service change is the `config.oauth.client_id` credential source.

### 5c. Config sync

`oauth.client_id` + `oauth.client_secret` land in integration-service config
in **both** `config/` (local) and the `deploy/` Helm Secret together (Config
Sync hard rule; partially-configured providers fail startup). Both absent →
`configured: false` (Connect disabled), safe to ship hidden.

### 5d. Disconnect note

`disconnect_mode: local_only` is the safe default (matches bitly). Twitch does
offer a standard revoke endpoint (`/oauth2/revoke`, `client_id` + `token` in
body, **no secret**), so `provider_revoke` is a desirable upgrade — but only if
the declarative revoker's client-auth enum set (`none`/`basic`/`form`) can
send `client_id` in the body *without* a client secret. If that shape isn't in
the closed set, keep `local_only` rather than grow the revoker for hygiene;
revocation is not correctness-critical. Decide at stage 1, don't guess.

## 6. Bundle-adjacent Helio artifacts

- **Axis-③ resolver:** none (id == key).
- **UI icon:** `ui/helio-app/src/integrations/icons/twitch.svg` + register in
  `ui/helio-app/src/integrations/providerIcons.ts` (manual, never generated).
- **i18n:** `tools.description.twitch` and (if display scopes surfaced)
  `tools.scopes.<slug>` strings in helio-app locales.
- **AI-facing doc:** a `twitch` provider sub-doc under
  `agents/plugins/heliox/skills/tool/`; bump the heliox plugin version
  (`agents/plugins/scripts/bump-version.sh`) and publish
  (`publish-to-marketplace.sh`) — one publish per batch, riding batch-end.
- **Generation:** from `go-services/integration-service`, `go run
  ./cmd/provider-gen` then `--check`; five projections commit together at
  batch end (do **not** commit local regens on the tool branch — expected to
  fail `--check` in CI until the batch lead's canonical regen).

## 7. Test plan — five layers

| Layer | What runs | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: httptest fakes assert path/method, the injected `Authorization: Bearer` **and** `Client-Id` headers, `broadcaster_id`/`first`/`after` params, cursor echo, and plaintext + `--json` error envelopes. Also a service test that `channel update` resolves self `broadcaster_id` via Get Users when omitted. | No |
| **L2** | Dev harness against the **real** Helix API: `ANYCLI_CRED_ACCESS_TOKEN=<user token> ANYCLI_CRED_CLIENT_ID=<app id> anycli twitch -- user get` / `stream followed` / `channel get`. Proves the two-credential injection and the mandatory `Client-Id` header against the live API before the pin bump. | **Yes** — a real Twitch app (client id) + a user token from a real Twitch account with the declared scopes. |
| **L3** | `provider-gen --check` (must accept the new `config.oauth.client_id` source) + integration-service unit suite (new credential-source projection test: gateway emits `client_id` from config) + `helio-cli` build with a local `go.mod replace` at the anycli branch, `go test ./cmd/heliox/cmds/tool/`. | No |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` for provider `twitch` (seed `access_token` + short `expires_at` + `refresh_token` from a real test account to force the refresh path), then `heliox tool twitch -- user get` reaches live Helix and returns real data. Confirms the seeded token **and** the config-projected `client_id` both flow through the token gateway. | **Yes** — a real Twitch user token/refresh token for the seed. |
| **L5** | Once, hidden, before the visible flip: `heliox tool twitch auth` → Twitch consent on the dev app → `oauth_connected` event on the originating channel → one **unseeded** live `heliox tool twitch -- ...` through the new connection. This is `oauth_light` → **human-in-the-loop** (lane 3): a live Twitch consent on a real account, not agent-drivable. | **Yes** — a real Twitch account for interactive consent. |

**Layers needing externally supplied credentials: L2, L4, L5** (a registered
Twitch dev app for the `client_id`/secret, and a real Twitch user account +
token). L1 and L3 are hermetic. Lane 1 (app registration) gates L4/L5; the
dev app is self-serve so registration is quick and carries no review clock.

## 8. Rollout

Ship hidden. After the anycli pin ships the `twitch` definition and the
`config.oauth.client_id` source lands + regenerates, run L1–L4 hidden, then the
human L5 consent, then flip `presentation.visible: true` + regenerate as the
single go-live change.
