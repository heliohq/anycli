# Beehiiv — `heliox tool` provider design

Scratch design doc for the `tool/beehiiv` branch (batch lead strips this at
batch-end). Written against the `helio-tool-provider` pipeline skill, the
298-integrations master plan (row 130), and the 2026-07-21 OAuth audit
(row 132), all verified against beehiiv's official developer docs.

## 1. Naming axes (master plan §3)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `beehiiv` | bundle `tool.command` (unset → flat command; no family) |
| ② anycli tool id | `beehiiv` | `definitions/tools/beehiiv.json` |
| ③ provider catalog key | `beehiiv` | `integrations/providers/beehiiv/` |
| Go package | `beehiiv` | `internal/tools/beehiiv/` |

All three axes are identical — **no `toolToProvider` divergence entry** is
needed (ProviderFor/ToolFor identity holds). No grouped family. Category:
Marketing, Wave 2, lane `oauth_review`.

## 2. What an AI teammate does with beehiiv (drives the API surface)

beehiiv is a newsletter/publishing platform. An AI teammate's realistic jobs:

- **Grow/maintain the list** — add a subscriber captured from a conversation
  or form, look a subscriber up by email, update their tier/custom fields,
  bulk-add a batch. This is the primary *write* path.
- **Report on the newsletter** — list recent posts with stats
  (opens/clicks/recipients), pull a single post's performance, enumerate
  audience segments and their sizes.
- **Discover context** — list the publications the connection can see, read a
  publication's settings, list custom fields and tiers so writes reference
  valid names/ids, enroll a subscriber into an automation journey.

That maps to a read-heavy tool with a focused set of subscriber mutations —
exactly what the beehiiv v2 REST API exposes. We do **not** try to author/send
posts via the API (post creation/sending is a first-class app authoring flow,
not a stable public write endpoint), so posts are read-only in the tool.

## 3. Official API surface wrapped

- **Base URL:** `https://api.beehiiv.com/v2` (verified).
- **Everything is publication-scoped:** almost every resource path is
  `/publications/{publicationId}/…`, where `publicationId` matches
  `^pub_[0-9a-fA-F-]+$`. The teammate discovers ids via `publication list`
  first, then passes `--publication-id` to resource verbs. `GET /publications`
  returns only the publications the credential can see (key/scope-restricted).
- **Auth on API requests:** `Authorization: Bearer <token>` (verified) — the
  same header shape for both an API key and an OAuth access token, so the
  anycli service is auth-mechanism-agnostic: it injects one bearer token.

Endpoints the tool wraps (all verified against the api-reference except where
marked *inferred* — those are confirmed at anycli stage 1 before coding):

| Verb group | Method + path | Scope |
|---|---|---|
| `publication list` | `GET /publications` | `publications:read` |
| `publication get` | `GET /publications/{pub}` | `publications:read` |
| `post list` | `GET /publications/{pub}/posts` (`expand`, `status`, `limit`, `page`, `order_by`) | `posts:read` |
| `post get` | `GET /publications/{pub}/posts/{postId}` *(inferred: standard REST show)* | `posts:read` |
| `subscription get-by-email` | `GET /publications/{pub}/subscriptions/by_email/{email}` (URL-encoded email) | `subscriptions:read` |
| `subscription list` | `GET /publications/{pub}/subscriptions` *(inferred: index of the create path)* | `subscriptions:read` |
| `subscription create` | `POST /publications/{pub}/subscriptions` (body below) | `subscriptions:write` |
| `subscription update` | `PATCH /publications/{pub}/subscriptions/{subId}` *(inferred)* | `subscriptions:write` |
| `segment list` | `GET /publications/{pub}/segments` | `segments:read` |
| `custom-field list` | `GET /publications/{pub}/custom_fields` | `custom_fields:read` |
| `tier list` | `GET /publications/{pub}/tiers` | `tiers:read` |
| `automation list` | `GET /publications/{pub}/automations` | `automations:read` |

`subscription create` verified body: `email` (required) plus optional
`reactivate_existing`, `send_welcome_email`, `utm_*`, `referring_site`,
`referral_code`, `custom_fields[]`, `double_opt_override` (`on|off|not_set`),
`tier` (`free|premium`), `premium_tier_ids[]`, `stripe_customer_id`,
`automation_ids[]`, `newsletter_list_ids[]`, `complimentary_gift_id`.

Stage-1 rule: any endpoint marked *inferred* is confirmed against the live
api-reference page before its verb is implemented; if a show/list/update path
does not exist, that verb is dropped rather than faked (no silent fallback).

## 4. anycli definition (stage-1 rubric → `service` type)

`cli` type is rejected: there is no official, provisionable, `--json`
beehiiv binary. Implement **`service`** type against the HTTP API, copying the
`internal/tools/notion/` shape (cobra tree grouped by resource; a
`BaseURL`/`HC`/`Out`/`Err` struct so unit tests point at an `httptest` server;
exit codes 0 success / 1 API-or-runtime failure via typed `apiError` / 2
usage; `--json` structured error envelope).

`definitions/tools/beehiiv.json`:

```json
{
  "name": "beehiiv",
  "type": "service",
  "description": "beehiiv newsletter platform (publications, posts, subscribers, segments)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "BEEHIIV_API_KEY"}
      }
    ]
  }
}
```

Single credential field `access_token` (the resolver-supplied bearer token,
whether an OAuth access token or a manually-entered API key — see §5),
injected as `BEEHIIV_API_KEY`; the service reads it and sets
`Authorization: Bearer`. This mirrors slack/notion's single-field injection.

**Output shape:** every verb prints the provider's JSON envelope unmodified to
stdout on success (`{"data": …}` / list bodies as returned), so the assistant
consumes real beehiiv shapes; errors render as the provider-neutral anycli
`--json` error envelope with the upstream status/message. `register.go` adds
`RegisterService("beehiiv", &beehiiv.Service{})`.

## 5. Credentials & the exact auth flow

### Verified against official docs

The 2026-07-21 audit (row 132) laned beehiiv **`oauth_review`** on the basis
that beehiiv offers a multi-tenant authorization-code OAuth2 flow whose client
registration is **not self-serve** (contact beehiiv Support). Official-doc
verification (`developers.beehiiv.com/oauth2`) **confirms** this and fills in
the exact contract:

- **Endpoints:** authorize `GET https://app.beehiiv.com/oauth/authorize`;
  token `POST https://app.beehiiv.com/oauth/token`. Utilities exist:
  `POST /oauth/revoke`, `POST /oauth/introspect`, `GET /oauth/token/info`.
- **Registration model:** gated — you must contact beehiiv Support to be
  issued a `client_id`/`client_secret`. This is precisely what `oauth_review`
  encodes: it gates the **visible flip**, not dev/L4/the batch-end merge
  (hidden-first). Lane 1 owns the support request. **No divergence from the
  audit** — the lane stands.
- **Token semantics:** token response returns `access_token`, `token_type`
  (Bearer), `expires_in`, and `refresh_token` "when available". Refresh via
  `grant_type=refresh_token`. So there is a real expiry + refresh cycle; the
  token gateway's refresh-and-write-back path applies.
- **Scopes** (space-delimited): default `identify:read`; request the
  read/write pairs the tool needs — `publications`, `posts`, `subscriptions`,
  `segments`, `custom_fields`, `tiers`, `automations` (each `:read`/`:write`).
- **PKCE:** required for public clients; confidential clients use
  `client_secret`. Helio is a confidential server-side client → use
  `client_secret`, PKCE not required.

### Recorded divergence (api_key fallback exists but is NOT what we ship)

beehiiv **also** offers self-serve API keys (Settings → API; Bearer, optionally
publication-scoped). Under the master-plan lane model beehiiv is nonetheless an
**oauth_review** provider — the multi-tenant path is OAuth, and the api_key
path is a single-account fallback. We ship the **standard_oauth** bundle. The
api_key path is recorded here only because it de-risks L2: an agent can run the
L2 harness against the real API with a self-serve API key *before* the OAuth
client is registered (both are `Authorization: Bearer`, so the anycli service
is unchanged). This does not change the shipped bundle or lane.

### Provider bundle credential wiring

`standard_oauth`; `credential.fields.access_token: token.access_token`,
`account_key: connection.account_key`. Nothing secret in the bundle —
`auth.required_config_fields: [oauth.client_id, oauth.client_secret]`; the
support-issued id/secret land in integration-service config (`config/` +
`deploy/` Helm Secret together, per Config Sync) as lane 1's per-provider
append, before beehiiv's L5 run.

## 6. Helio provider bundle plan (hidden-first)

`integrations/providers/beehiiv/provider.yaml`, `presentation.visible: false`:

```yaml
schema: helio.provider/v1
key: beehiiv
go_name: Beehiiv

presentation:
  name: beehiiv
  description_key: beehiiv
  consent_domain: beehiiv.com
  visible: false
  order: <batch-lead assigns>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.beehiiv.com/oauth/authorize
    token_url: https://app.beehiiv.com/oauth/token
    token_exchange_style: form_secret     # client creds in the form body; confirm form vs basic at L5
    pkce: none                            # confidential client uses client_secret
    display_scopes: [publications, posts, subscriptions, segments, custom_fields, tiers, automations]
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://app.beehiiv.com/oauth/token/info   # see identity note
  stable_key: /workspace_id
  label_candidates: [/workspace_name, /workspace_id]

connection:
  mode: isolated
  disconnect_mode: local_only            # /oauth/revoke exists → provider_revoke is a later enhancement
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
  name: beehiiv
  kind: oauth
```

**Identity note (the one open decision — resolve at stage 1/2).** A beehiiv
OAuth token is workspace-scoped and can span multiple publications, so the
stable account identity is the *workspace*, not a single publication. The
recommended `identity.source: userinfo` against `GET /oauth/token/info` (the
token-metadata endpoint under the default `identify:read` scope) needs its
exact JSON keys confirmed against a live response before the stable_key /
label pointers above are final. Two fallbacks if `token/info` does not expose a
stable workspace id: (a) `POST /oauth/introspect`, or (b) `userinfo` against
`GET /publications` extracting the first publication's `/data/0/id` +
`/data/0/name` (weaker — order/emptiness sensitive). This is a bundle-config
choice only; it needs **zero** service code and stays on the `standard_oauth`
golden path.

**standard_oauth fit — no adapter.** beehiiv is a textbook authorization-code
provider: form/JSON token exchange, JSON-pointer identity extraction, bearer
credential projection — all inside the closed `standard_oauth` capability set
(`token_exchange_style` + `declarativeIdentityResolver` + no-op/declarative
revoker). No `service/adapter_*.go`, no integration-service capability growth
is expected. If L5 shows the token endpoint wants Basic client auth, flip
`token_exchange_style: form_basic` — still config-only.

Other stages: UI icon `ui/helio-app/src/integrations/icons/beehiiv.svg` +
`providerIcons.ts` append (manual); AI-facing sub-doc under
`agents/plugins/heliox/skills/tool/`; five projections regenerated by the
batch lead's single `provider-gen` run (never committed on this branch).

## 7. Test plan → five layers

| Layer | For beehiiv | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — `httptest` fakes for each verb: assert path (incl. URL-encoded email), `Authorization: Bearer` injection, publication-scoping, `subscription create` body, and both plaintext + `--json` error rendering. No real API. | No |
| **L2** harness real-API | `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli beehiiv -- publication list` then a real `subscription get-by-email` / `post list --expand stats`. Proves field names + request shape match live API. **Runnable with a self-serve API key** ahead of OAuth registration (both are bearer). | **Yes** — a beehiiv account API key (account pool, lane 2) |
| **L3** generate + suites | `provider-gen --check` (bundle validates: HTTPS urls, reviewed `standard_oauth`, closed enum fields) + both repos' unit suites. Branch is *expected* to fail `--check` in CI until batch-end (bundle serialized to batch lead). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` a real OAuth `access_token` (+`refresh_token`, short `expires_at` to force the gateway refresh path) for a seeded assistant, then `heliox tool beehiiv -- publication list` through the real token gateway. | **Yes** — a real OAuth `access_token` from the registered dev app (lane 1 gates this) |
| **L5** full connect | Once, hidden: `heliox tool beehiiv auth` → consent on beehiiv's OAuth app → `oauth_connected` event → unseeded live run. Confirms authorize/callback/identity extraction/`form_secret` vs `form_basic`. Human-in-the-loop (lane 3). Gates the visible flip together with review clearance. | **Yes** — registered OAuth client (lane 1) + a real beehiiv account for consent (lane 2) |

L1 and L3 need no credentials. L2/L4/L5 need externally supplied credentials:
L2 a self-serve API key, L4 a dev-app OAuth access token, L5 the
support-registered OAuth client plus a consenting account. L4 and L5 are
gated by lane 1 (dev-mode app creation gates L4; review clearance + the
client registration gate the visible flip).

## 8. Summary of divergences / decisions recorded

1. **Audit lane confirmed, not overturned.** Official docs confirm
   `oauth_review` (gated, contact-support client registration). No amendment.
2. **api_key path noted as an L2 de-risking fallback only** — the shipped
   bundle is `standard_oauth`; the lane is unchanged.
3. **Identity endpoint is the single stage-1/2 verification item**
   (`/oauth/token/info` keys), config-only, no service code.
4. **No `toolToProvider` entry, no grouped family, no service adapter, no
   integration-service capability growth** — beehiiv is fully on the
   standard_oauth golden path.
