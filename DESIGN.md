# Buffer — per-tool design (tool/buffer)

Scratch design for the `buffer` external tool provider, per the
`helio-tool-provider` pipeline and the 008-300 master rollout plan.
Buffer is catalog row 131, Wave **3-hold** (final Wave-3 batch), category
Marketing, provider key `buffer`. This file is committed on branch
`tool/buffer` and stripped by the batch lead at batch end.

## 0. Naming axes (§3 master plan)

| Axis | Value | Notes |
|---|---|---|
| ① CLI command word | `buffer` | flat command; no family group |
| ② anycli tool id | `buffer` | `definitions/tools/buffer.json` |
| ③ provider catalog key | `buffer` | `integrations/providers/buffer/` |

② == ③, so **no `toolToProvider` entry** is added and no resolver divergence
exists. Go package: `internal/tools/buffer/` (id has no dashes/leading digit).

## 1. Why Buffer, and which of its two API surfaces

An AI teammate's real job with Buffer is **social-media publishing**: draft a
post, schedule it into a channel's queue or at a specific time, list/edit/delete
queued posts, capture ideas, and enumerate the connected channels it can post
to. That maps to Buffer's post/channel/idea/organization model.

Buffer exposes **two** API surfaces, and this is the single most load-bearing
finding of the research (verified against official docs, not the catalog):

- **Legacy REST API** (`api.bufferapp.com/1/`, authorize at
  `bufferapp.com/oauth2/authorize`): OAuth 2.0, but **new developer-app
  registration is closed** — you cannot obtain a `client_id`. This is exactly
  the "closed app registration" the master-plan §6 risk and the catalog's
  oauth_review note were written against. **We do not build against this
  surface.**
- **New GraphQL API** (`https://api.buffer.com`, public beta): a single
  GraphQL endpoint (POST to the base URL, no `/graphql` path) with a unified
  `account → organizations → channels → posts/ideas` data model, an
  Authorization-Code **+ PKCE** OAuth flow with **self-serve app registration**
  in Settings → API (`publish.buffer.com/settings/api`), and a personal/static
  API-key alternative. **This is the surface we wrap.**

Sources:
- Auth guide: https://developers.buffer.com/guides/authentication.html
- Data model: https://developers.buffer.com/guides/data-model.html
- Schema reference: https://developers.buffer.com/reference.html
- Examples: https://developers.buffer.com/examples/
- First post: https://developers.buffer.com/guides/your-first-post.html
- Help Center (personal keys, closed legacy registration):
  https://support.buffer.com/article/859-does-buffer-have-an-api

### Endpoints the tool wraps (new GraphQL API)

All operations are one `POST https://api.buffer.com` with a GraphQL body and
`Authorization: Bearer <token>`.

Queries:
- `account { id email name avatar timezone organizations { id name } }`
- `channels(input: { organizationId })` → `{ id name service displayName avatar isDisconnected }`
- posts listing per channel (schema `posts` query) for read/list verbs.

Mutations:
- `createPost(input: CreatePostInput!): PostActionPayload!` — `text`,
  `channelId`, `schedulingType`, `mode` (`addToQueue` | `customScheduled` +
  `dueAt`), `saveToDraft`, `imageUrl`, per-service `metadata`.
- `editPost(input: EditPostInput!): PostActionPayload!`
- `deletePost(input: DeletePostInput!): DeletePostPayload!`
- `createIdea(input: CreateIdeaInput!): CreateIdeaPayload!` (idea belongs to an
  organization, not a channel).

Return unions carry `... on PostActionSuccess { post { … } }` and
`... on MutationError { message }`; the service always selects the error arm so
a GraphQL-level failure surfaces a message.

## 2. Divergence from the catalog / OAuth audit (record per task contract)

The catalog lanes Buffer **oauth_review** with the note *"new-app registration
is restricted"*, and §6 lists it under "API access regressions" (3-hold on
closed app registration). **That rationale is stale for the surface we build
against.** Official docs show the *new* GraphQL API has **self-serve** OAuth
app registration (client_id/client_secret issued in Settings → API) with
mandatory PKCE — which under the audit rubric is an **oauth_light** shape
(self-serve, no human review/partner/publish gate), not oauth_review.

Two caveats keep this from being a clean flip to oauth_light, and are the
reason Buffer legitimately sits in 3-hold behind a pre-verify gate:

1. The GraphQL API is **public beta**, and third-party/multi-tenant OAuth
   (one Helio app that arbitrary customer Buffer accounts authorize) is
   reported as **not yet fully enabled** — the enablement state is the real
   gate, functionally equivalent to a review/verification gate.
2. The **personal/static API key** fallback is self-serve but **expires 30
   days after creation** (owner-only, account-scoped, `Authorization: Bearer`).
   That is a poor multi-tenant credential (every user re-pastes a key monthly).

**Verdict recorded for DESIGN.md / wave-board:** keep the **oauth_review** lane
*classification* conservatively for the 3-hold pre-verify (public-beta +
not-fully-enabled third-party OAuth is a genuine enablement gate), but the
underlying *auth shape* is standard PKCE OAuth, **amendable to oauth_light**
via the §6 catalog-amendment log the moment stage-1/L2 confirms a third-party
OAuth client can be created and an external account can authorize it. If
stage-1 finds third-party OAuth is still not enabled for new clients, the
decision is binary: (a) ship as an **api_key manual-token** bundle on the
30-day personal key (documented expiry pain), or (b) **swap Buffer out** via
risk #2 (Hootsuite already covers the same social-scheduling category in
Wave 2). This DESIGN recommends (proceed on PKCE OAuth) as the primary path and
names (a)/(b) as the explicit pre-verify fallbacks.

## 3. anycli definition

**Type: `service`** (stage-1 rubric). No official non-interactive `--json` CLI
to wrap; the surface is an HTTP (GraphQL) API. Implement in
`internal/tools/buffer/` against `https://api.buffer.com`, following the
`notion` reference shape: a cobra tree, a `BaseURL`/`HC`/`Out`/`Err` struct so
tests point at an `httptest` server, exit-code contract (0 ok, 1 runtime/API,
2 usage), and a `--json` structured-error envelope. Because everything is one
POST, the service holds a tiny GraphQL helper (build query/variables, POST,
decode `data`/`errors` + mutation-union `MutationError.message`).

Injected credential: `access_token` → env `BUFFER_ACCESS_TOKEN`, sent as
`Authorization: Bearer`. Single credential field; no `user_id` needed (unlike
X) because Buffer scopes by token, and org/channel are addressed by id.

Proposed cobra tree (resource-grouped, agent-facing verbs):

```
buffer account get                          # account { id email name organizations }
buffer org list                             # organizations under the account
buffer channel list  --org <id>             # channels(input:{organizationId})
buffer post list     --channel <id> [--status queued|sent|draft]
buffer post create   --channel <id> --text <s>
                     [--mode addToQueue|customScheduled] [--due-at <RFC3339>]
                     [--draft] [--image-url <url>] [--metadata-json <json>]
buffer post edit     --id <id> [--text <s>] [--due-at <ts>]
buffer post delete   --id <id>
buffer idea create   --org <id> --text <s>
```

**JSON output shape (`--json`, agent-neutral):** each command prints a single
JSON object with the provider-neutral fields flattened out of GraphQL (never
the raw GraphQL envelope). E.g. `post create` →
`{"id","text","dueAt","channelId","status"}`; `channel list` →
`{"channels":[{"id","name","service","displayName","isDisconnected"}]}`;
errors → `{"error":{"code","message"}}` on stderr with exit 1. Timestamps pass
through as RFC3339 (Buffer uses `dueAt` ISO-8601).

Constraint to encode from the changelog: `assets.videos` and
`metadata.{service}.linkAttachment` are mutually exclusive (video ignored if
both) — surface as a usage error (exit 2) rather than silently dropping.

## 4. Credential fields & the exact auth flow

**Auth type: OAuth 2.0 Authorization Code + PKCE (S256), mandatory for all
clients.**

- Authorize: `https://auth.buffer.com/auth`
- Token: `https://auth.buffer.com/token`
- API base (resource): `https://api.buffer.com`
- Registration: self-serve, Settings → API (`publish.buffer.com/settings/api`).
  Confidential client (our server-side case) → `client_id` + `client_secret`;
  PKCE `code_verifier` sent **in addition to** the secret.
- Scopes: `posts:read posts:write ideas:read ideas:write account:read
  offline_access`. `offline_access` is required to receive a `refresh_token`.
  (`account:write` omitted — the teammate has no need to mutate account
  settings; add only if a concrete verb requires it.)
- Tokens: `token_type: Bearer`; `access_token` `expires_in` ~3600s (1h);
  `refresh_token` only when `offline_access` requested.
- **Refresh-token rotation (single-use):** every successful refresh returns a
  **new** `refresh_token` and **invalidates** the one sent; reusing an old
  refresh token revokes the entire grant. The token gateway MUST persist the
  rotated `refresh_token` on every refresh (the standard A3 refresh-and-
  write-back path in `references/integration-testing.md`). This is normal
  rotation, **not** X-style `single_active_token` — do not set that flag.

Bundle-declared config fields: `[oauth.client_id, oauth.client_secret]`
(supplied by lane 1 into `config/` + the `deploy/` Helm Secret together).

**Identity resolution — the one real capability question.** Buffer has **no
GET userinfo endpoint**; identity comes from a **GraphQL POST**
(`query { account { id email name } }`) to `https://api.buffer.com`. The
`standard_oauth` `declarativeIdentityResolver` only does a **GET** on
`identity.url` (or reads `token_response`). So Buffer cannot use the default
`identity.source: userinfo` GET as-is. Options, in order of preference:

1. **Grow the declarative resolver** with a reviewed capability to issue a
   POST identity request with a fixed body (a `post_userinfo` source carrying
   a static GraphQL query), then extract `stable_key: /data/account/id` and
   `label_candidates: [/data/account/email, /data/account/name]` via the
   existing JSON-Pointer machinery. Preferred: it keeps Buffer on
   `runtime_strategy: standard_oauth` with zero provider-specific Go, and the
   capability is generically useful for other GraphQL providers.
2. If the resolver growth is out of scope for the batch, a **narrow adapter**
   (`service/adapter_buffer.go`, precedent: Slack/X identity adapters) that
   POSTs the account query and returns `(stable_key, label)`.

This DESIGN recommends option 1; the choice is finalized at stage-2 against the
integration-service capability set on the branch base (as several prior tools
did, e.g. the userinfo-metadata deriver for docusign).

## 5. Helio provider bundle plan (`integrations/providers/buffer/provider.yaml`)

Hidden-first (`presentation.visible: false`). Marketing precedent is `bitly`;
OAuth+PKCE precedent is `x`. Planned bundle:

```yaml
schema: helio.provider/v1
key: buffer
go_name: Buffer
presentation:
  name: Buffer
  description_key: buffer
  consent_domain: buffer.com
  visible: false            # 3-hold; flip after L5 + oauth_review clearance
  order: <batch-assigned>
auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://auth.buffer.com/auth
    token_url: https://auth.buffer.com/token
    token_exchange_style: form_secret     # confidential client: secret + PKCE verifier
    pkce: s256
    scopes: [posts:read, posts:write, ideas:read, ideas:write, account:read, offline_access]
    single_active_token: false
    refresh_lease: provider               # rotating single-use refresh; gateway writes back
identity:
  source: post_userinfo                   # capability growth (§4 option 1); else adapter
  url: https://api.buffer.com
  query: 'query { account { id email name } }'
  stable_key: /data/account/id
  label_candidates: [/data/account/email, /data/account/name, /data/account/id]
connection:
  mode: isolated
  disconnect_mode: local_only             # Buffer has no documented token-revoke endpoint
  runtime_strategy: standard_oauth
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: buffer
  kind: oauth
```

Axis notes: ① `tool.command` unset (flat, not a family group like
google/microsoft). `refresh_lease: provider` is a candidate value — confirm it
is in the standard_oauth allowed-set at stage-2 (several prior tools grew that
set, e.g. hootsuite/signnow); the intent is "gateway persists the rotated
refresh_token each refresh." `disconnect_mode: local_only` because Buffer
documents no OAuth token-revocation endpoint; if one is found at stage-1, use
`strategy` with a declarative revoker instead.

Also required at batch-end (not this scratch file): UI icon
`ui/helio-app/src/integrations/icons/buffer.svg` + `providerIcons.ts` append,
i18n `tools.<...>.buffer` description string, AI-facing sub-doc under
`agents/plugins/heliox/skills/tool/`.

## 6. Test plan — five layers

| Layer | What it does for Buffer | External creds needed |
|---|---|---|
| **L1** | anycli `go test ./...`: `httptest` fake of `api.buffer.com` GraphQL — assert POST body (query/variables), `Authorization: Bearer` header injection, mutation-union error arm (`MutationError.message`), `--json` vs plain error rendering, exit codes, and the video/linkAttachment mutual-exclusion usage error. | none |
| **L2** | `BUFFER_ACCESS_TOKEN=<key> anycli buffer -- account get` / `channel list --org …` / `post create …` against the **real** `api.buffer.com`. Proves the GraphQL body, scopes, and `dueAt` scheduling actually work live. | **Yes** — a real Buffer account + token from the account pool (a **personal/static API key** is sufficient for L2 since it's a Bearer token on the same endpoint; no app registration needed just to exercise the GraphQL surface). |
| **L3** | `provider-gen --check` + `helio-cli` + integration-service unit suites; includes the identity-resolver capability-growth test (§4). | none |
| **L4** | singleton + `POST /internal/test-only/connections/seed` (provider `buffer`, seed `access_token` **+ `refresh_token`** with a short `expires_at` to force the rotating-refresh write-back path) → `heliox tool buffer -- account get`. Proves token-gateway → anycli. | **Yes** — a real access/refresh token pair; the refresh token must come from a real OAuth grant to exercise rotation (a static personal key has no refresh cycle, so seed it access-only to at least prove the serve path, and additionally seed an OAuth pair once lane 1's dev app exists to prove rotation). |
| **L5** | one full `heliox tool buffer auth` → connect link → **real Buffer OAuth consent (PKCE)** on the dev app → `oauth_connected` event → unseeded live run. Human-in-the-loop (oauth L5, lane 3). Gates the visible flip. | **Yes** — lane-1 registered dev OAuth app (client_id/secret) **and** a real Buffer account to consent with. |

Layers needing externally supplied credentials: **L2, L4, L5** (account-pool
token for L2; dev OAuth app + refresh-capable token for L4/L5). L1/L3 are pure
agent throughput.

## 7. 3-hold pre-verify outcome & residual risks

- **API feasibility: PASS (conditional).** The new GraphQL API is a real,
  documented, self-serve-registerable surface that covers the teammate's
  publish/schedule/channel/idea needs. The legacy-REST "closed registration"
  blocker does **not** apply to it.
- **Auth-shape decision: standard PKCE OAuth** on `standard_oauth`, with one
  capability question (POST-body identity resolution, §4) resolved at stage-2.
  Lane stays **oauth_review** for pre-verify conservatism, **amendable to
  oauth_light** on stage-1/L2 confirmation of third-party OAuth enablement.
- **Residual risk (the one that could still swap Buffer out):** if stage-1
  finds third-party/multi-tenant OAuth clients still cannot be created (public
  beta not yet opened to external apps), the only shippable path is the
  **api_key personal-key** bundle with a **30-day expiry** — a monthly
  re-paste burden that is a weak multi-tenant experience. In that case escalate
  to the §6 catalog-amendment: ship api_key-with-caveat, or swap Buffer out
  (Hootsuite covers the category). Record whichever on the wave-board.
