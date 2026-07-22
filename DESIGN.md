# Bluesky — per-tool integration design

**Catalog row:** #204 Bluesky · anycli id `bluesky` · provider key `bluesky` ·
audit lane `oauth_light` · Wave 3 · Social & Media.

**Branches:** anycli `tool/bluesky`, Helio `tool/bluesky`.

**Scratch file** — committed on the branch for the batch lead; stripped at
batch-end. English-only, TDD, hidden-first per the pipeline skill and the master
plan (`docs/design/008-300-integrations-rollout-plan.md`).

---

## 0. Headline decision: lane diverges from the audit (oauth_light → api_key)

The 2026-07-21 OAuth audit put Bluesky at `oauth_light` (row 206 in that file),
reasoning that atproto OAuth "requires no registration or review at all": the
`client_id` is a public HTTPS URL to a self-hosted client-metadata JSON, and any
client authorizes against any user's PDS via authorization-code + PKCE/PAR/DPoP.
**That narrow fact is correct — but the audit rubric's own escape clause applies:
"OAuth that is per-instance ... or otherwise impractical for a shared client
keeps the tool in `api_key`."** Verified against the official atproto OAuth spec
(<https://docs.bsky.app/docs/advanced-guides/oauth-client>), the flow is
impractical for Helio's shared-client bundle model on three independent axes,
each individually disqualifying for `standard_oauth`:

1. **DPoP is mandatory on every request.** atproto access tokens are
   sender-constrained: each API call carries an `Authorization: DPoP <token>`
   header *plus* a freshly-minted, per-request DPoP proof JWT signed by a
   session keypair, using server-issued nonces. Helio's token gateway serves a
   plain bearer string and anycli injects it as a static header — there is no
   seam for holding a session private key and signing a proof per request.
2. **PAR is mandatory.** The authorize step must first POST parameters to a
   `pushed_authorization_request_endpoint` to obtain a `request_uri`. The
   `standardOAuthExchanger` (`token_exchange_style: form|json × basic|secret`)
   has no PAR step.
3. **Per-user authorization-server discovery.** There is no fixed
   `authorize_url`/`token_url`. The client resolves the account handle → DID →
   DID document → PDS, then reads `/.well-known/oauth-protected-resource` and
   `/.well-known/oauth-authorization-server` to discover per-user endpoints. The
   bundle's `oauth.authorize_url`/`token_url` are static strings.

The atproto docs themselves caution OAuth "is not recommended for fully headless
clients." An AI teammate posting on a user's behalf is exactly a headless
confidential automation.

**Official-doc-supported alternative — App Passwords (the recommended bot
path).** Bluesky ships a first-class credential model for third-party/automated
clients: the user generates an **App Password** in Settings → Privacy and
Security → App Passwords (self-serve, revocable independently of the main
password), then the client calls `com.atproto.server.createSession` with
`{identifier, password}` and receives `{did, handle, email, accessJwt,
refreshJwt}`. Confirmed against
<https://docs.bsky.app/docs/api/com-atproto-server-create-session> and the
Get-Started guide: **createSession's `accessJwt` is a plain
`Authorization: Bearer` token — DPoP does NOT apply to the app-password path**
(DPoP is an OAuth-only requirement). This sidesteps all three blockers above and
maps cleanly onto Helio's existing `manual_credentials` + api_key infrastructure
(the `mongodb`/`zoominfo`/`servicenow` precedent: a static secret the user
pastes, stored in Vault, served verbatim by the token gateway).

**Recommendation:** implement Bluesky as **api_key lane, `manual_credentials`
bundle** carrying two fields — `identifier` + `app_password`. Record this as an
auth-shape divergence in the §6 amendment log (same class as Mastodon /
Bill.com / NetSuite, which the plan already flags as non-standard-auth). Lane,
row, id, key, and the 298-total invariant are unchanged; only the auth **type**
moves oauth_light → api_key (no OAuth app registration needed — lane-1 work for
this tool drops to zero, and there is no review clock).

Rejected: building a bespoke atproto-OAuth adapter (DPoP keypair custody +
proof minting inside anycli, PAR, per-PDS discovery, token-gateway changes to
carry a DPoP JWK). That is a multi-week infra project outside the pipeline's
"a standard OAuth provider needs zero service code" budget, and the app-password
path is the documented, lower-friction, widely-used bot integration — strictly
better for our use case.

### 0a. Implementation divergence from §2–§4 (verified against `main`, 2026-07-22)

§3/§4 below assumed **multi-field manual credentials** (`identifier` +
`app_password` as two separate `credential_input.fields` projected to two
`token.*` credential sources) were already-shipped capability. Verified against
the actual integration-service on `main`: **they are not.** The manual-credential
plane is single-secret only —
- `service/manual_credential.go` `resolveManualSecret` hard-rejects any bundle
  with `len(credential_input.fields) != 1` (the generator enforces exactly one
  required field);
- `model.CredentialSource` is a **closed enum** — the only token source is
  `token.access_token` (one secret); there is no `token.identifier` /
  `token.app_password`;
- `writeUserTokenCredential` persists a single `AccessToken` string.

Multi-field would require new `CredentialSource` enum values + token-gateway
resolve arms + a multi-secret vault payload + a generator D5 relaxation — the
"distinct vault credential kind" the master plan flags as NetSuite-class, and it
would duplicate the in-flight multi-field work on peer branches. Out of budget.

**Adopted minimal shape (works end-to-end on `main`, zero gateway/enum growth):**
a **single combined secret** `identifier:app_password` stored via the existing
`token.access_token` source (the `mongodb` single-secret precedent). Split on the
first `:` in two places:
- the **anycli service** reads one env var `BLUESKY_CREDENTIALS` and splits it
  into identifier + app password before `createSession`;
- a new **`blueskySessionVerifier`** (integration-service) splits the same
  secret at connect time, calls `createSession`, and derives `did` (stable key)
  + `handle` (label) — the §3 "session verifier" intent, unchanged.

Identifier is a handle or email; app passwords are `xxxx-xxxx-xxxx-xxxx`; neither
contains `:`, so the first-colon split is unambiguous. `pds_host` is dropped from
v1 (bsky.social only), consistent with the §0/§1 "flagship default, override
later" stance. Lane/row/id/key and the createSession verification (did/handle)
are all preserved; only the credential **field count** collapses 2→1 to fit the
shipped single-secret plane. When multi-field manual credentials land on `main`,
splitting the combined field back into two is a follow-up.

---

## 1. Official API surface wrapped, and why

Bluesky's API is the **AT Protocol XRPC** surface. Two Lexicon families matter:

- `com.atproto.*` — repository/account plane on the **user's PDS** (writes:
  create/delete records, upload blobs; session: createSession/refreshSession).
- `app.bsky.*` — the Bluesky AppView (reads: timeline, feeds, search, profiles,
  notifications). For an authenticated client these are called against the PDS,
  which **proxies** them to the AppView, so one host + one bearer token serves
  everything (docs: "make all requests via the PDS and proxying").

**Host.** Call everything against the entryway/PDS at **`https://bsky.social`**
(default). For self-hosted-PDS accounts the correct host is the PDS service
endpoint published in the account's DID document; v1 defaults to `bsky.social`
and exposes an optional `--pds` / stored `pds_host` override (the same
"flagship-instance default, override later" stance the plan takes for Mastodon).
`createSession`'s response includes `didDoc`, so a later hardening pass can
auto-resolve the PDS with zero extra user input.

**What an AI teammate actually does with Bluesky** drives the endpoint choice —
post, read the timeline, search, look up people, engage, and check mentions:

| Capability | XRPC method(s) | Lexicon / collection |
|---|---|---|
| Authenticate a run | `com.atproto.server.createSession` | — (returns accessJwt/did/handle) |
| Create a post (text, reply, links, images) | `com.atproto.repo.createRecord` (+ `com.atproto.repo.uploadBlob` for images) | `app.bsky.feed.post` |
| Delete a post | `com.atproto.repo.deleteRecord` | `app.bsky.feed.post` |
| Read a post / thread | `app.bsky.feed.getPostThread`, `app.bsky.feed.getPosts` | — |
| Home timeline | `app.bsky.feed.getTimeline` | — |
| Someone's posts | `app.bsky.feed.getAuthorFeed` | — |
| Search posts | `app.bsky.feed.searchPosts` | — |
| Get a profile | `app.bsky.actor.getProfile` | — |
| Search people | `app.bsky.actor.searchActors` | — |
| Follow / unfollow | `createRecord` / `deleteRecord` | `app.bsky.graph.follow` |
| Like / repost | `createRecord` | `app.bsky.feed.like`, `app.bsky.feed.repost` |
| Notifications (mentions/replies) | `app.bsky.notification.listNotifications` | — |

Two provider-specific details the service must handle so the AI doesn't have to:

- **Post record shape.** `app.bsky.feed.post` records need `$type`, `text`,
  `createdAt` (RFC-3339). Rich text (mentions, links, hashtags) requires
  **facets** with UTF-8 **byte** offsets — the service computes facets from
  plain text (detect URLs → `app.bsky.richtext.facet#link`; `@handle` → resolve
  DID → `#mention`) so the AI passes plain `--text`. v1 scope: auto-detect links
  and hashtags; mentions best-effort (resolve handle, skip on failure) — never
  fail a post because a facet couldn't resolve.
- **Images.** `--image PATH` → `uploadBlob` → embed the returned blob ref in
  `app.bsky.embed.images` with `--alt` alt-text (accessibility; default empty
  with a warning). Cap at 4 images per post (provider limit).

`at://` URIs and `cid`s are opaque handles the AI passes back verbatim for
delete/like/reply — the service echoes them in output.

---

## 2. anycli definition (axis ② id: `bluesky`)

**Type: `service`** (stage-1 rubric). No official Bluesky CLI binary exists that
is non-interactive, `--json`-capable, and provisionable into the runtime image;
the integration is HTTP against XRPC. Lives in `internal/tools/bluesky/`
(package `bluesky` — id has no dashes, no leading digit, so the package name is
the id verbatim), registered `RegisterService("bluesky", &bluesky.Service{})`
in `internal/tools/register.go`. Copy the `notion` service shape:
`BaseURL`/`HC`/`Out`/`Err` struct for httptest injection, cobra tree grouped by
resource, exit-code contract (0 success, 1 runtime/API failure via typed
`apiError`, 2 usage/parse), `--json` structured error envelope.

**Definition `definitions/tools/bluesky.json`** — two credential fields injected
as env, service reads them and runs createSession internally:

```json
{
  "name": "bluesky",
  "type": "service",
  "description": "Bluesky as a tool (AT Protocol, app-password session)",
  "auth": {
    "credentials": [
      { "source": {"field": "identifier"},
        "inject": {"type": "env", "env_var": "BLUESKY_IDENTIFIER"} },
      { "source": {"field": "app_password"},
        "inject": {"type": "env", "env_var": "BLUESKY_APP_PASSWORD"} },
      { "source": {"field": "pds_host"},
        "inject": {"type": "env", "env_var": "BLUESKY_PDS_HOST"} }
    ]
  }
}
```

`pds_host` is optional (empty → `https://bsky.social`); it is a projected field,
not a user-entered secret (see §3). anycli treats an absent optional field as
empty, matching existing multi-field definitions.

**Command tree (verbs).** Grouped by resource, `--json` on every leaf:

```
bluesky whoami                                   # createSession + getProfile(self)
bluesky post create --text "…" [--reply-to at://…] [--quote at://…]
                    [--image PATH --alt "…"]… [--lang en]
bluesky post delete --uri at://…
bluesky post get    --uri at://…                 # getPostThread
bluesky timeline           [--limit N] [--cursor C]
bluesky feed author --actor <handle|did> [--limit N] [--cursor C]
bluesky search posts  --q "…" [--limit N] [--cursor C]
bluesky search actors --q "…" [--limit N]
bluesky profile get   --actor <handle|did>
bluesky follow   --actor <handle|did>
bluesky unfollow --uri at://…                    # the follow record uri
bluesky like     --uri at://… --cid <cid>
bluesky repost   --uri at://… --cid <cid>
bluesky notifications list [--limit N] [--cursor C]
```

**Session handling inside the service.** Each `heliox tool bluesky -- …`
invocation is a fresh process. The service calls `createSession` once at start
(cheap; the docs explicitly bless "a single session and not bother with
refreshing" for one-off requests), caches `accessJwt`+`did` in-memory for that
process, and issues the command against `bsky.social` (or `pds_host`) with
`Authorization: Bearer <accessJwt>`. No `refreshSession` needed — a process
never outlives the short accessJwt window in practice; if a call returns
`ExpiredToken`, refresh once and retry (bounded), else surface the typed error.
`refreshJwt` is never persisted (Helio stores the app password, not JWTs).

**JSON output shape.** Provider-neutral, thin, agent-consumable — never raw
XRPC passthrough. Examples:

- `post create` →
  `{"uri":"at://did:plc:…/app.bsky.feed.post/…","cid":"bafy…","handle":"alice.bsky.social"}`
- `timeline` / `search posts` →
  `{"posts":[{"uri","cid","author":{"did","handle","display_name"},"text","created_at","reply_count","repost_count","like_count"}],"cursor":"…"}`
- `profile get` →
  `{"did","handle","display_name","description","followers_count","follows_count","posts_count"}`
- error →
  `{"error":{"code":"api_error","message":"…","status":400,"provider_error":"InvalidRequest"}}`
  (exit 1).

**TDD (L1).** httptest fake serving `createSession` + each XRPC method; assert
request shape (JSON body, injected Bearer header, byte-offset facets on a post
with a URL/hashtag, uploadBlob multipart + embed wiring, `at://` echo on delete),
plus both plain-text and `--json` error rendering. Never hit the live API in a
unit test.

---

## 3. Credential fields & auth flow (Helio side)

**Credential the user supplies:** two fields.

| Field | Label | Secret | Notes |
|---|---|---|---|
| `identifier` | Handle or email | no | e.g. `alice.bsky.social` or the account email |
| `app_password` | App password | yes | `xxxx-xxxx-xxxx-xxxx` from Settings → App Passwords — **not** the main password |

`setup_url` → the App Passwords settings page
(`https://bsky.app/settings/app-passwords`) so the connect drawer tells the user
exactly where to mint one.

**Auth flow at connect time.** No OAuth redirect. The user pastes identifier +
app password into the connect drawer → `POST /connections/credentials`
(write-only, stored in Vault). Identity + verification is a **session verifier**
(Option A capability): integration-service calls
`POST https://bsky.social/xrpc/com.atproto.server.createSession` with the pasted
`{identifier, password}`; on `200` it reads `did` (stable account key) and
`handle` (human label), on `401` it rejects the credential (`InvalidRequest` /
`AuthFactorTokenRequired`). This is strictly better than mongodb's no-verify
stance because Bluesky *has* a verifiable identity endpoint — the pattern mirrors
`crisp`/`servicenow` verifiers that call a provider endpoint to derive identity.

**Auth flow at runtime.** Token gateway serves the stored `{identifier,
app_password}` (+ projected `pds_host`) verbatim as the provider-neutral
credential map; anycli injects them as env; the service runs createSession →
command. No refresh cycle on Helio's side — the app password is a long-lived
static secret (revoked only when the user deletes it in Bluesky settings).

---

## 4. Helio provider bundle plan (`integrations/providers/bluesky/provider.yaml`)

**Three naming axes** — all identity, zero divergence, so **no
`toolToProvider` entry** is added:

| Axis | Value |
|---|---|
| ① CLI command word | `bluesky` (flat, ungrouped) |
| ② anycli tool id | `bluesky` |
| ③ provider catalog key | `bluesky` |

**Bundle shape** (manual_credentials, hidden-first), modeled on `mongodb` but
multi-field + verified:

```yaml
schema: helio.provider/v1
key: bluesky
go_name: Bluesky

presentation:
  name: Bluesky
  description_key: bluesky
  consent_domain: bsky.app
  visible: false            # hidden-first; flip is the single go-live change

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: identifier
        label_key: bluesky_identifier
        secret: false
        placeholder: "alice.bsky.social"
        required: true
      - name: app_password
        label_key: bluesky_app_password
        secret: true
        placeholder: "xxxx-xxxx-xxxx-xxxx"
        required: true
    setup_url: https://bsky.app/settings/app-passwords

identity:
  source: strategy          # blueskySessionVerifier derives did + handle

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    identifier: token.identifier
    app_password: token.app_password
    pds_host: connection.pds_host   # optional; empty ⇒ bsky.social
    account_key: connection.account_key
tool:
  name: bluesky
  kind: api-key
```

**Capability growth (integration-service).** Requires a `blueskySessionVerifier`
identity/verifier strategy (calls createSession, extracts `did`→stable_key,
`handle`→label). Before implementing, verify on `main` whether an existing
multi-field manual-credentials verifier can be reused/parameterized (the
`zoominfo`/`servicenow`/`crisp` verifiers are near-neighbors — prefer growing a
reviewed enum value over a new adapter). Multi-field `credential_input` +
two-field `token.*` projection is already-shipped capability (zoominfo/mixpanel);
only the verifier is potentially new.

**No integration-service OAuth config, no lane-1 app registration, no
`deploy/` Secret append** — the api_key lane needs none. Config Sync hard rule
is trivially satisfied (nothing to sync).

**Other hidden-first artifacts** (batch-end merge): `internal/tools/register.go`
entry, UI icon `ui/helio-app/src/integrations/icons/bluesky.svg` +
`providerIcons.ts` append, i18n `tools.desc.bluesky` + the two `label_key`
strings across all 9 locales, AI-facing sub-doc under
`agents/plugins/heliox/skills/tool/`, one plugin version bump.

---

## 5. Test plan — five layers

| Layer | What it proves for bluesky | External creds needed |
|---|---|---|
| **L1** | `go test ./...` in anycli: httptest fake for createSession + every XRPC leaf; asserts Bearer injection, JSON body shape, **byte-offset facet** computation, uploadBlob→embed wiring, `at://` echo, `--json` error envelope, exit codes. | none |
| **L2** | Dev harness against the **real** API: `BLUESKY_IDENTIFIER=… BLUESKY_APP_PASSWORD=… anycli bluesky -- whoami` / `post create` / `timeline` / `search posts`. Proves the real createSession→Bearer→XRPC path, facet byte offsets, and blob upload against live `bsky.social`. | **Yes** — a real Bluesky test account + app password (account pool). |
| **L3** | `provider-gen` + `--check` (five projections regenerate clean, directory-key equality, the new `blueskySessionVerifier` enum accepted) + both repos' unit suites (helio-cli build with local `replace` → anycli branch; integration-service verifier test). | none |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with `identifier`+`app_password` fields (api_key providers are seedable), then `heliox tool bluesky -- whoami` reaches live `bsky.social` through the real token gateway. Success = real profile data, not a mock. | **Yes** — the same test-account app password, seeded (a valid app password is required for createSession to succeed against the live API). |
| **L5** | Full connect path once, still hidden: `heliox tool bluesky auth` → connect drawer → paste identifier + app password → `blueskySessionVerifier` verifies (did/handle captured, connection shows connected) → one **unseeded** live command through the token gateway. This is the **api_key key-entry L5 path** (master plan §2), not the OAuth consent path. | **Yes** — real account; agent-drivable via agent-browser (human fallback on UI breakage). No OAuth consent, no 2FA-on-consent obstacle. |

**Credential-gated layers:** L2, L4, L5 all need one real Bluesky account with an
app password from the account pool (free, self-serve — no paid tier, no partner
review, no app registration). L1/L3 are fully offline.

Only after L5 passes does `presentation.visible` flip to `true` + regenerate as
the single go-live change (skill stage 10).

---

## 6. Open items for the batch lead

- **§6 amendment-log entry required:** record the oauth_light → api_key
  auth-shape divergence (DPoP/PAR/per-PDS OAuth impractical for the shared-client
  model; app-password path adopted). Lane totals shift by one
  (oauth_light −1, api_key +1); row/id/key/wave and the 298 total unchanged.
  Removes one entry from lane-1's registration queue (no OAuth app needed).
- **Reuse check before coding:** confirm on `main` whether an existing
  multi-field manual verifier covers "call endpoint, read two JSON pointers for
  stable_key+label" so `blueskySessionVerifier` is a parameterization, not a new
  adapter.
- **pds_host default:** ship `bsky.social`-only in v1; didDoc-based PDS
  auto-resolution is a follow-up, not a blocker (covers third-party-PDS users).
