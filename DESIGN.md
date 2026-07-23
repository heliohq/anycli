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

Return unions carry a success arm (`... on PostActionSuccess { post { … } }`)
and one or more error arms. The schema reference exposes a **base
`MutationError { message }`** plus sibling error types **`NotFoundError`,
`RestProxyError { message link code }`, and `VoidMutationError`** — but the
published reference does **not** render explicit `implements` clauses, so it is
**unconfirmed whether `MutationError` is a GraphQL interface** those siblings
satisfy (verified against the official schema reference, 2026-07-22). This is a
stage-2 gate, because it decides whether one error arm is enough:

- If `MutationError` **is** an interface implemented by the sibling error
  types, a single `... on MutationError { message }` selection is sufficient
  and surfaces every failure.
- If it is **not** (they are independent concrete types), selecting only
  `... on MutationError` would let a `NotFoundError` / `RestProxyError` /
  `VoidMutationError` response fall through as a non-error — a silent-fallback
  violation of the fail-fast rule, and a false sense of coverage for the L1
  "error arm" test. In that case the selection MUST enumerate **every** error
  arm and the L1 test MUST assert each arm surfaces a message.

`deletePost` returns its **own** `DeletePostPayload` (`DeletePostSuccess { id }`
+ error), a distinct shape from `PostActionPayload` — handle it separately.

## 2. Divergence from the catalog / OAuth audit (record per task contract)

The catalog lanes Buffer **oauth_review** with the note *"new-app registration
is restricted"*, and §6 lists it under "API access regressions" (3-hold on
closed app registration). **That rationale is stale for the surface we build
against.** Official docs show the *new* GraphQL API has **self-serve** OAuth
app registration (client_id/client_secret issued in Settings → API) with
mandatory PKCE — which under the audit rubric is an **oauth_light** shape
(self-serve, no human review/partner/publish gate), not oauth_review.

On the official auth guide's own terms, the underlying auth shape is
**oauth_light**: self-serve confidential-client registration in Settings → API
issues `client_id` + `client_secret` immediately, the flow is Authorization
Code + PKCE (S256), and the guide describes **no** review, partner-program,
verification, or publish gate — it states OAuth lets you "build apps that
access Buffer accounts on behalf of your users," i.e. multi-tenant use is
presented as available, not gated. Under the oauth-audit rubric (self-serve
registration, no review gate) that is squarely **oauth_light**.

**Official-docs divergence (recorded per task contract, verified 2026-07-22
against developers.buffer.com/guides/authentication.html):** an earlier draft
asserted third-party/multi-tenant OAuth is "reported as not yet fully enabled."
I found **no source** for that enablement restriction, and the primary evidence
points the other way (self-serve registration, no gate). The honest recording
is: **auth shape is oauth_light; the only real residual gate is public-beta
stability** of the new GraphQL API — not an enablement or verification lock.

One softer caveat remains, on the fallback credential:

- The **personal/static API key** fallback is self-serve, owner-only,
  account-scoped (`Authorization: Bearer`). Its expiry is **user-selectable at
  creation — 7 / 30 / 60 / 90 days or 1 year, with 30 days merely the
  default** (Help Center article 859; the same article's stray "expires 30 days
  after creation" line is contradicted by its own selectable-option list, so
  the selectable set is authoritative). A 1-year key is a tolerable manual-token
  bundle — the worst case is an **annual** re-paste, not the monthly burden an
  earlier draft claimed.

**Verdict recorded for DESIGN.md / wave-board:** the auth shape is
**oauth_light**; Buffer is **provisionally held at oauth_review only pending
stage-1 confirmation of public-beta stability** — not because of any documented
review or enablement gate. The hidden-first posture makes this
over-conservatism harmless (a hidden tool ships and is L4/L5-testable
regardless of lane), so the lane is **amendable to oauth_light** via the §6
catalog-amendment log the moment stage-1/L2 confirms a third-party OAuth client
registers and an external account authorizes it on the stable API. If stage-1
instead finds the beta is not usable for a shared client (instability, not an
enablement lock), the fallbacks are (a) ship as an **api_key manual-token**
bundle on the personal key (user-selectable expiry, 1-year max — a mild annual
re-paste), or (b) **swap Buffer out** (Hootsuite already covers the same
social-scheduling category in Wave 2). This DESIGN recommends proceeding on
PKCE OAuth as the primary path and names (a)/(b) as the explicit pre-verify
fallbacks.

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
buffer post list     --org <id>             # posts(input:{organizationId}); organizationId REQUIRED
                     [--channel <id>]       #   → PostsFiltersInput.channelIds (optional filter)
                     [--status <s>]         #   → PostsFiltersInput.status (values unconfirmed, see below)
buffer post create   --channel <id> --text <s>
                     [--mode addToQueue|customScheduled] [--due-at <RFC3339>]
                     [--draft] [--image-url <url>] [--metadata-json <json>]
buffer post edit     --id <id> [--text <s>] [--due-at <ts>]
buffer post delete   --id <id>
buffer idea create   --org <id> --text <s>
```

**`post list` is organization-scoped, not channel-scoped** (verified against
the schema reference, 2026-07-22): the query is
`posts(input: PostsInput!, first, after)` where **`PostsInput.organizationId:
OrganizationId!` is required**; channel filtering is an *optional*
`PostsFiltersInput.channelIds` and status filtering is
`PostsFiltersInput.status`. So `--org` is **required** on `post list` and
`--channel` is demoted to an optional filter — this mirrors `channel list
--org` (channels is likewise org-scoped via `ChannelsInput.organizationId`), so
the two read verbs are now internally consistent. An agent holding only a
channel id resolves its org via `account get` (the account carries
`organizations`) or `channel list --org`, then supplies `--org`.

**`--status` values are unconfirmed.** The schema declares a `PostStatus` enum
but the published reference does not enumerate its members (only `draft` shows
in example responses). Do **not** hardcode `queued|sent|draft` as the accepted
set — read the real `PostStatus` values off the schema at stage-2 before
constraining the flag; until then `--status` passes through as a raw string
into `PostsFiltersInput.status`.

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
  visible: false            # 3-hold; flip after L5 + stage-1 beta-stability clearance
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
    refresh_lease: credential             # rotating single-use refresh; per-connection lock; gateway writes back
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
google/microsoft). `refresh_lease: credential` serializes a refresh
**per-connection** (lease key includes the credentialID), which is the correct
scope for Buffer: each connection has its own independent single-use rotating
refresh token, so the only hazard is a concurrent double-refresh of the *same*
connection — not a provider-wide one. The provider-wide `provider` scope is
reserved for genuinely single-token providers (X). Confirm `credential` is in
the standard_oauth allowed-set at stage-2 (several prior tools grew that set,
e.g. hootsuite/signnow); the intent is "gateway persists the rotated
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
- **Auth-shape decision: oauth_light** — standard Authorization Code + PKCE
  (S256) on `standard_oauth`, self-serve confidential-client registration with
  no review/partner/publish gate per the official auth guide (§2, §4), plus one
  capability question (POST-body identity resolution, §4) resolved at stage-2.
  Lane is **provisionally held at oauth_review only pending stage-1
  confirmation of public-beta stability** — no source supports an enablement
  gate — and is **amendable to oauth_light** on that confirmation.
- **Residual risk (the one that could still swap Buffer out):** if stage-1
  finds the public-beta GraphQL API is not usable for a shared multi-tenant
  client (instability, not a documented enablement lock), the fallback is the
  **api_key personal-key** bundle — self-serve, with **user-selectable expiry
  and a 1-year maximum** (a mild annual re-paste, not the monthly burden an
  earlier draft implied). In that case escalate to the §6 catalog-amendment:
  ship api_key-with-caveat, or swap Buffer out (Hootsuite covers the category).
  Record whichever on the wave-board.

## 8. Stage-2 verification (2026-07-22) — schema confirmed against official docs

Every gate the earlier sections flagged as "unconfirmed" was verified against
developers.buffer.com. Resolutions (and the resulting divergences from the
§1–§7 sketch, which drove implementation):

- **MutationError arm — RESOLVED, single arm suffices.** The official *first
  post* guide (`guides/your-first-post.html`) shows the `createPost` response
  union selected with exactly `... on PostActionSuccess { post { id text dueAt } }`
  plus `... on MutationError { message }`, and explicitly advises "always include
  `... on MutationError { message }` to catch errors." `MutationError` is the
  base error interface these mutations resolve against, so one arm surfaces every
  failure. Implementation additionally selects `__typename` on every mutation
  payload and treats any non-success typename as a failure (surfacing `message`,
  falling back to the typename) — so fail-fast holds even for an error type that
  doesn't implement the interface. No enumeration of NotFoundError /
  RestProxyError / VoidMutationError is required.
- **`posts` query — Relay connection, NOT a flat list (divergence from §3).**
  Verified verbatim (`examples/get-paginated-posts.html`): `posts(after, first,
  input: { organizationId, filter: { status, channelIds } }) { pageInfo {
  startCursor endCursor hasNextPage } edges { node { id text createdAt channelId
  } } }`. Confirmed Post read-fields are `id text createdAt channelId` (NOT
  `dueAt`/`status` in the node — `status` is an input filter only). `--org` is
  required; `--channel` and `--status` are optional filters. Pagination exposed
  as `--first` / `--after`.
- **`filter.status` is `[PostStatus!]` (array of enum), not a scalar string
  (divergence from §3).** Confirmed value `sent` in the official example (plus
  `draft` elsewhere). Passed via a typed GraphQL variable (`$input: PostsInput!`)
  so the server coerces the JSON string to the enum; `--status <s>` maps to
  `filter: { status: ["<s>"] }`. The full `PostStatus` member set is still not
  enumerated in the reference, so the flag stays an un-validated pass-through
  (L2 confirms live values).
- **`createPost` minimal input (refines §1's "assets required" schema-reference
  read).** The working example creates a text-only post with just `text`,
  `channelId`, `schedulingType: automatic`, `mode: addToQueue` — `assets` is NOT
  required in practice. Enum values confirmed: `schedulingType: automatic`;
  `mode ∈ {addToQueue, customScheduled}`; `customScheduled` pairs with `dueAt`
  (ISO-8601 UTC); `saveToDraft: true` for drafts.
- **`assets`/`metadata` are raw JSON pass-throughs, not a cooked `--image-url`
  (divergence from §3).** The `assets` object shape is undocumented on any
  fetchable page (the images example 404s; data-model omits it), so — per the
  no-silent-guess / fail-fast rule — the tool exposes `--assets-json` and
  `--metadata-json` (validated as JSON, exit 2 on parse failure) rather than
  guessing a structure. The changelog `assets.videos` × `metadata.*.linkAttachment`
  mutual-exclusion is enforced as a structural check on the parsed JSON (exit 2).
- **`createIdea` content is `{ title, text }` (refines §3's `--text`-only).**
  Verified (`examples/create-idea.html`): `content: { title, text }`, response
  `... on Idea { id content { title text } }`. Tool exposes `--text` (required)
  and `--title` (optional).
- **Identity query trimmed to verified fields.** The only account fields the
  official docs demonstrate are `id`, `email` (auth guide) and `organizations {
  id name }` (data model). Identity resolution uses `query { account { id email
  } }` → stable_key `/data/account/id`, labels `[/data/account/email,
  /data/account/id]` (the unverified `name` field is dropped from §5's plan).
  `account get` selects `id email organizations { id name }`; `channel list`
  selects the verified `id name service` only.
- **All auth params confirmed unchanged** (`guides/authentication.html`):
  authorize `https://auth.buffer.com/auth`, token `https://auth.buffer.com/token`,
  API base `https://api.buffer.com`, PKCE S256 required for all clients,
  single-use rotating refresh tokens, scopes as in §4.

### Helio-side capability growths this required (integration-service)

Buffer is the first `standard_oauth` provider whose account identity comes from
a **GraphQL POST** and whose refresh token **rotates**. Two reviewed capability
additions (both DESIGN-mandated in §4/§5, both with sibling-branch precedent):

1. **`identity.source: post_userinfo`** — a new declarative identity source that
   POSTs a fixed GraphQL query (`identity.query`) to `identity.url` with Bearer
   auth and extracts `stable_key` / `label_candidates` via the existing JSON
   Pointer machinery (pointers address into the `{ data: { account: … } }`
   envelope). Keeps Buffer on `runtime_strategy: standard_oauth` with zero
   provider-specific Go (DESIGN §4 option 1, chosen over the adapter option). A
   GraphQL `errors` array in the identity response fails Connect fast.
2. **`standard_oauth` refresh-lease allowed-set** — the standard_oauth runtime
   contract previously pinned `refresh_lease: none` exactly; it now accepts
   `{none, credential}`. Buffer selects `refresh_lease: credential` so refreshes
   of its single-use rotating refresh token are serialized **per-connection**
   (the lease key includes the credentialID) across replicas — a concurrent
   double-refresh of the *same* connection would otherwise invalidate its grant,
   while unrelated connections never queue behind each other. The provider-wide
   `provider` scope (one global lock for every connection of a provider) is
   intentionally NOT used: it is reserved for genuinely single-token providers
   (X), and applying it to isolated per-connection tokens like Buffer's would be
   a single-point serialization bottleneck across all orgs/users.

   **Review-finding divergence (recorded per task contract):** an earlier draft
   of this bundle set `refresh_lease: provider`, whose lease key is the
   provider-wide `refresh:buffer`. That serialized every Buffer connection on one
   global lock and contradicted this section's own "per-connection" claim.
   Corrected to `refresh_lease: credential` (per-connection, keyed by
   credentialID), which the token gateway's `acquireRefreshLease` already keys
   correctly.
