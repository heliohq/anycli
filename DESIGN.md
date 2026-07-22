# Tool design: Kit (ConvertKit)

Per-tool design for the `helio-tool-provider` pipeline. Catalog row 129
(master plan §4): product **Kit (ConvertKit)**, anycli id `kit`, provider key
`kit`, auth lane **oauth_review**, wave 2, category Marketing. Scratch file on
branch `tool/kit`; the batch lead strips it at batch end.

All facts below were verified against Kit's official developer docs
(`https://developers.kit.com`, V4 API) on 2026-07-22, not inherited from the
catalog. Divergences from the catalog/audit are called out inline.

---

## 1. Naming (three axes)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `kit` | bundle `tool.command` (unset → flat `heliox tool kit`) |
| ② anycli tool id | `kit` | `definitions/tools/kit.json`, `RegisterService("kit", …)` |
| ③ provider catalog key | `kit` | `integrations/providers/kit/` dir + `key:` |

All three coincide (`kit`), so **no `toolToProvider` divergence entry** is
needed — verified: `helio-cli/internal/toolcred/resolver.go` has no `kit` key
and none is required. Go package is `internal/tools/kit/` (id has no dashes).
Not a grouped family (no `tool.group`); flat command like `notion`/`slack`.

Note on branding: the product renamed ConvertKit → **Kit** in 2024; the API is
now served from `api.kit.com` (the legacy `api.convertkit.com` V3 host is
deprecated). We build against **V4** only. UI/user-facing name: "Kit".

---

## 2. Official API surface wrapped, and why

**Base URL:** `https://api.kit.com/v4` (OpenAPI `servers: [{url: https://api.kit.com}]`, all paths prefixed `/v4`).
**Auth on API calls:** `Authorization: Bearer <access_token>` (OAuth). (API-key
mode uses `X-Kit-Api-Key`, but Kit officially forbids API keys for public
apps — see §4; Helio uses OAuth.)
**Content type:** `application/json`. **Pagination:** cursor-based
(`per_page`, `after`/`before` cursors). **Consistency:** eventually consistent
+ bulk/async endpoints exist for large writes.

Kit is a creator email-marketing platform. What an AI teammate actually does
with it drives endpoint selection: manage the subscriber list, tag/segment
audiences, draft & schedule broadcasts (newsletters), enroll subscribers into
sequences (automations), and read growth/engagement stats. The wrapped surface
is therefore organized by resource:

| Resource group | Endpoints (verified in V4 OpenAPI index) | Why an AI teammate needs it |
|---|---|---|
| **account** | `GET /account`, `GET /account/creator_profile`, `GET /account/email_stats`, `GET /account/growth_stats`, `GET/PUT /account/colors` | Identity (OAuth whoami) + report on list health/performance |
| **subscribers** | `GET /subscribers` (+ engagement/date/state/tag filters), `GET /subscribers/{id}`, `POST /subscribers`, `POST /bulk/subscribers`, `PUT /subscribers/{id}`, `POST /subscribers/{id}/unsubscribe`, `GET /subscribers/{id}/tags`, `GET /subscribers/{id}/stats` | Add/lookup/update contacts; the core list operations |
| **tags** | `GET /tags`, `POST /tags`, `POST /bulk/tags`, `PUT /tags/{id}`, `POST /tags/{id}/subscribers/{sid}` and `…/subscribers?email=`, `DELETE …`, bulk tag/untag, `GET /tags/{id}/subscribers` | Segment the audience; tagging is Kit's automation trigger primitive |
| **broadcasts** | `GET /broadcasts`, `POST /broadcasts`, `GET /broadcasts/{id}`, `PUT /broadcasts/{id}`, `DELETE …`, `GET /broadcasts/{id}/stats`, `GET /broadcasts/stats`, `GET /broadcasts/{id}/clicks` | Draft/schedule newsletters; read open/click stats — the highest-value teammate action |
| **sequences** | `GET /sequences`, `POST /sequences`, `GET/PUT/DELETE /sequences/{id}`, `POST /sequences/{id}/subscribers` (+ `?email=`), `GET /sequences/{id}/subscribers` | Enroll subscribers into automations |
| **sequence-emails** | `GET/POST/PUT/DELETE /sequences/{id}/emails…` | Author the emails inside a sequence |
| **custom-fields** | `GET /custom_fields`, `POST` (+bulk), `PUT/DELETE`, bulk value updates | Read/write the arbitrary per-subscriber data model |
| **forms** | `GET /forms`, `POST /forms/{id}/subscribers` (+`?email=`), bulk, `GET /forms/{id}/subscribers` | Subscribe contacts via a form (Kit's canonical opt-in path) |
| **segments** | `GET /segments` | Enumerate saved segments for broadcast targeting |
| **purchases** | `GET /purchases`, `GET /purchases/{id}`, `POST /purchases` | Commerce events feeding automations (OAuth-only endpoint) |
| **snippets / posts / email-templates / webhooks** | `GET/POST/PUT` snippets; `GET` posts; `GET /email_templates`; `GET/POST/DELETE /webhooks` | Secondary: reusable content, published posts, templates, event webhooks |

**Scope decision for the first ship:** implement the high-value, agent-natural
subset first — `account` (identity + stats), `subscriber`, `tag`, `broadcast`,
`sequence`, `custom-field`, `form`, `segment`. Defer `snippet`, `post`,
`email-template`, `webhook`, `purchase`, `sequence-email` to a follow-up verb
pass (they are lower-frequency for a teammate and add surface without changing
the auth/identity contract). This keeps the service file within the code-health
size budget while covering the actions the assistant will actually take.

---

## 3. anycli definition

**Type: `service`** (stage-1 rubric). There is no official, non-interactive,
`--json`-capable Kit CLI binary to wrap, so the `cli` type is disqualified;
implement HTTP against V4 in `internal/tools/kit/`. This matches 21/23 existing
definitions and the `notion` reference shape.

`definitions/tools/kit.json`:

```json
{
  "name": "kit",
  "type": "service",
  "description": "Kit (ConvertKit) as a tool — creator email marketing (OAuth)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "KIT_ACCESS_TOKEN"}
      }
    ]
  }
}
```

The service reads `KIT_ACCESS_TOKEN` and sends `Authorization: Bearer $KIT_ACCESS_TOKEN`
on every request (mirrors `notion` injecting `access_token`→`NOTION_TOKEN`).
anycli stays OAuth-ignorant: it only knows the field name and the injection.

**Service package (`internal/tools/kit/`, copy the `notion` shape):** a
`BaseURL`/`HC`/`Out`/`Err` struct so tests can point at an `httptest` server
and capture stdout/stderr; a cobra tree grouped by resource; global `--json`
flag; the documented exit-code contract (0 success, 1 runtime/API failure via a
typed `apiError`, 2 usage/parse errors) with a `--json` structured error
envelope.

Command tree (verbs are agent-facing; noun-verb grouping like `notion`):

```
kit account get                       # GET /v4/account  (whoami + plan)
kit account stats [--growth|--email]  # GET /v4/account/{growth_stats|email_stats}
kit subscriber list [--status --tag-id --created-after …]
kit subscriber get <id>
kit subscriber create --email --first-name --fields k=v
kit subscriber update <id> [...]
kit subscriber unsubscribe <id>
kit tag list
kit tag create --name
kit tag add    --tag-id --subscriber-id | --email
kit tag remove --tag-id --subscriber-id | --email
kit broadcast list
kit broadcast get <id>
kit broadcast create --subject --content [--send-at --public --tag-id --segment-id]
kit broadcast update <id> [...]
kit broadcast stats <id>
kit sequence list
kit sequence add --sequence-id --subscriber-id | --email
kit custom-field list
kit custom-field create --label
kit form list
kit form add --form-id --email
kit segment list
```

**JSON output shape:** provider-neutral, one envelope per command. Success:
the resource object/array lifted from Kit's `{ "<resource>": {…} }` or
`{ "<resources>": [...], "pagination": {…} }` wrapper into a stable
`{ "data": …, "pagination": {…}? }` shape, so the agent gets consistent keys
regardless of Kit's per-endpoint wrapper name. Error (`--json`): the standard
typed envelope `{ "error": { "code", "message", "details"? } }`, exit 1.
Follow `notion`'s renderer precedent exactly — do not invent a new convention
(003 §3 output conventions are the contract).

**Kit response-shape notes for the service impl (verified):**
- List endpoints wrap collections and return `pagination` with cursor fields
  (`per_page`, `has_next_page`, `end_cursor`/`start_cursor`). Surface a
  `--after`/`--limit` flag pair; do not auto-paginate the whole list by
  default (unbounded fan-out).
- Tag/form/sequence membership has both an id-based and an
  `…/subscribers?email=` variant — expose `--subscriber-id` XOR `--email`.
- `POST /bulk/*` and `POST /purchases` require OAuth (not API-key) per Kit
  docs — fine, we are OAuth-only.

**TDD (L1):** `httptest.Server` fakes asserting request method/path/body, the
injected `Authorization: Bearer` header, cursor pagination handling, and both
plain-text and `--json` error rendering. Never hit the live API from a unit
test. Include a fake for `GET /v4/account` (identity) and one write
(`POST /v4/broadcasts`) and one list with pagination (`GET /v4/subscribers`).

---

## 4. Credential fields & auth flow (oauth_review — verified)

**Audit verdict re-verified and CONFIRMED.** Audit row 131: Kit V4 supports
OAuth 2.0 authorization-code (+refresh, +PKCE); API keys are officially
unsupported for public integrations; distribution to arbitrary creators
requires **Kit App Store submission + review + approval** before publishing.
Official confirmation: `developers.kit.com/api-reference/authentication`
("OAuth 2.0 for apps available for all creators in the Kit App Store" vs "API
keys … for your own account … We do not offer any official support for apps or
public integrations that rely upon API keys") and
`developers.kit.com/kit-app-store/going-live` (app-review checklist: "Developers
must use OAuth for user authentication instead of API keys", "Submit for
approval", reviewers test not-logged-in / logged-in / new-signup OAuth flows;
Kit emails approval before publish). → **oauth_review is correct.** The review
gate is on the *visible flip* only (hidden-first); dev/L1–L4 are unblocked.

**Registration model:** self-serve app creation in the Kit account (create app
→ enable API access → configure Authorization URL, Redirect URI(s), and the
"Secure application" toggle). A registered-but-unpublished app works for the
developer's own account and test accounts (enough for L4/L5); publishing to all
creators is the reviewed step. This is a standard **confidential web-server
client** for Helio's token gateway — so Helio uses the **refresh-token flow
(client_secret), not PKCE**. ("Secure application" ON = confidential; PKCE is
only for SPA/mobile where the secret can't be kept.)

**OAuth endpoints (verified, all under `api.kit.com/v4`):**
- authorize: `https://api.kit.com/v4/oauth/authorize`
  (params `client_id`, `response_type=code`, `redirect_uri`, optional `scope`,
  `state`)
- token: `POST https://api.kit.com/v4/oauth/token`, grant `authorization_code`
  then `refresh_token`. **Verified 2026-07-22 against the official curls:**
  - the **authorization_code** exchange sends `-H 'Content-Type: application/json'`
    with a JSON body carrying **both** `client_id` **and** `client_secret` (plus
    `grant_type`, `code`, `redirect_uri`);
  - the **refresh** call also documents `Content-Type: application/json` but its
    body carries **only** `client_id` — no `client_secret`. (Kit's published
    refresh curl is itself buggy: it shows `"code"` where the field should be
    `"refresh_token"`, so it is not a reliable source for exact body fields.)
  Content-type is documented as JSON, but the chosen bundle encoding is
  `form_secret` (form body, `client_secret_post`), NOT a JSON style — see §5 Q1
  for the architectural reason (the shared refresh path is form-only by
  construction, and Doorkeeper accepts form on this controller family).
- revoke: `POST https://api.kit.com/v4/oauth/revoke` (RFC 7009; body `token`,
  `client_id`, `client_secret`, optional `token_type_hint`; always `200`).
  **Documented content-type here is `application/x-www-form-urlencoded`** —
  direct proof that Kit's OAuth stack (Doorkeeper) accepts form-encoded bodies
  on the same controller family. This is exactly what makes `form_secret` safe
  for the token endpoint too (§5 Q1).

**Scopes:** the only scope today is `public` (default when `scope` is omitted);
Kit's docs say "Fine-grained access control via scopes coming soon." So the
bundle declares `display_scopes: [public]` (or omits `scopes` and relies on the
default). Do **not** invent scope strings.

**Token semantics (verified from the token response):**
```json
{ "access_token":"…", "token_type":"Bearer", "expires_in":172800,
  "refresh_token":"…", "scope":"public", "created_at":1710270147 }
```
`expires_in` is **provider-driven and variable**, not a fixed 48h: the initial
authorization_code grant returns `172800` (48h), but the documented refresh
example returns `expires_in: 7200` (2h). A **rotating, single-use
`refresh_token`** is issued on every exchange (the submitted refresh token is
revoked; reuse returns `invalid_grant`).

**Both of those are handled by the standard refresh path with
`refresh_lease: none` — no special lease scope is warranted.** Verified against
`service/token_refresh.go` on the worktree base:
- **Variable `expires_in` → dynamic `expires_at`:** the refresh leg runs through
  `golang.org/x/oauth2`, whose `Token.Expiry` is derived from each response's
  `expires_in`; `token_refresh.go:53-54` writes `refreshed.Expiry = &newTok.Expiry`
  whenever the new token carries an expiry. The connection's `expires_at` is
  therefore re-derived per response regardless of lease scope. Nothing about a
  variable lifetime requires `refresh_lease: provider`.
- **Rotating single-use `refresh_token` → write-back:** `token_refresh.go:50`
  does `RefreshToken: firstNonEmpty(newTok.RefreshToken, td.RefreshToken)`, i.e.
  it persists the freshly rotated refresh token (and logs
  `refresh_token_rotated`). Every shipped Google app rotates the same way and
  ships `refresh_lease: none` (`gmail/provider.yaml:31`).

`OAuthLeaseProvider` is **not** a "provider-driven expiry" flag — it is a global
per-**app** serialization lock reserved for single-active-token providers, where
one connection's refresh invalidates *every other connection's* token. Its only
user is X, via the bespoke `RuntimeStrategyXExclusiveGrant`
(`provider_catalog.gen.go`), paired with `single_active_token: true`. Kit issues
**one independent token per connection** (`single_active_token: false`), so
there is no cross-connection invalidation to serialize. The correct scope is
therefore `none`, matching every shipped standard-OAuth refresher.

**Contract-level fact (see §5 Q2):** the `standard_oauth` runtime contract pins
`refresh_lease` to `none` as a hard scalar and enforces it by strict equality
(`model/runtime_contract.go:42`, `:224-231`, run from `provider-gen`
`validate.go:405` via `ValidateRuntimeContract`). `refresh_lease: provider`
under `standard_oauth` is not a legal bundle at all — it fails `provider-gen
--check` (the L3 gate) and integration-service startup. So `none` is both the
semantically correct choice **and** the only value the contract accepts.

**Identity (verified `GET /v4/account`):**
```
{ "user": { "email": str, "id": int },
  "account": { "id": int, "name": str, "plan_type": str,
               "primary_email_address": str, "timezone": {…}, … } }
```
→ `identity.source: userinfo`, `url: https://api.kit.com/v4/account`,
`stable_key: /account/id`, `label_candidates: [/account/name, /account/primary_email_address, /user/email, /account/id]`.

`account.id` is an **integer**, and the declarative identity resolver on the
`tool/kit` worktree base **cannot read a numeric stable key** — this is required
capability growth, not a pre-solved case. Verified on the base:
`service/declarative_identity.go:103` `jsonPointerString` does a plain
`value.(string)` type assertion and returns `ok=false` for any non-string JSON
value; the resolver then errors `provider "kit" identity has no string value at
stable key "/account/id"` (`declarative_identity.go:36`). There is **no numeric
coercion anywhere in integration-service** on the base, and there is **no
HubSpot bundle on `main`** (HubSpot lives on an unmerged batch branch that added
its own coercion + tests). The earlier claim that this "is already supported on
main (added for HubSpot)" is therefore **false on the base** and must not be
relied on.

**Consequence:** Kit needs numeric-stable-key coercion added to the declarative
identity resolver (widen `jsonPointerString`, or add a numeric-aware stable-key
reader, with `json.Number`/int handling and unit tests), delivered in the same
change as this bundle — exactly as the HubSpot branch did for its own integer
`/portalId`. Kit exposes **no stable string key** to fall back to
(`primary_email_address` and `user.email` are mutable; `account.name` is a
display label), so the integer `/account/id` is the only durable AccountKey and
coercion is a genuine dependency — we do **not** carry a `stable_key` the
resolver cannot read. Because of this, Kit is **not** a zero-service-code
provider (see §5).

**Credential fields stored by Helio:** `access_token`, `refresh_token`,
`expires_at` (from `expires_in`), `account_key = connection.account_key`
(the resolved `/account/id`). The user token never touches the bundle; it
enters via the OAuth callback and lives in Vault.

---

## 5. Helio provider bundle plan (`integrations/providers/kit/provider.yaml`)

Ships **hidden-first** (`presentation.visible: false`) — decouples the App
Store review clock from the anycli pin. Pure `standard_oauth` **strategy**; no
provider-specific Go adapter and **no new exchanger capability** are warranted.
Kit is a textbook Doorkeeper-style authorization-code server — token exchange,
refresh, revoke, and client authentication all sit inside the existing
`standard_oauth` capability set. The token exchange uses the existing
`form_secret` enum value (standard OAuth2 `client_secret_post`); §5 Q1 explains
why that is both the documented-compatible and the architecturally coherent
choice over a new `json_secret` value.

**One genuine capability dependency remains:** numeric-stable-key coercion in the
declarative identity resolver (§4 identity), because Kit's `/account/id` is an
integer and there is no coercion on the branch base. That is a small, tested
widening of a shared reader — not a bespoke adapter — but it does mean Kit is
**not** a zero-service-code provider. The lease scope and disconnect mode are
plain `standard_oauth` contract values (`refresh_lease: none`,
`disconnect_mode: provider_revoke`), not growth.

```yaml
schema: helio.provider/v1
key: kit
go_name: Kit

presentation:
  name: Kit
  description_key: kit
  consent_domain: kit.com
  visible: false          # hidden-first; flip on go-live after L5 + App Store review
  order: <batch-assigned>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://api.kit.com/v4/oauth/authorize
    token_url: https://api.kit.com/v4/oauth/token
    token_exchange_style: form_secret   # existing enum; client_secret_post, coherent across auth-code + refresh (§5 Q1)
    pkce: none                          # confidential web-server client
    display_scopes: [public]
    single_active_token: false
    refresh_lease: none                 # standard_oauth contract value; rotating refresh handled by the standard path (§4, §5 Q2)
    revoke:                             # RFC-7009 form-POST; contract requires this block when disconnect_mode=provider_revoke
      url: https://api.kit.com/v4/oauth/revoke
      client_auth: form                 # client_id+client_secret in the form body (OAuthRevokeClientAuthForm)
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://api.kit.com/v4/account
  stable_key: /account/id             # integer → needs numeric-stable-key coercion (§4 identity, capability growth)
  label_candidates: [/account/name, /account/primary_email_address, /user/email, /account/id]

connection:
  mode: isolated
  disconnect_mode: provider_revoke  # standard_oauth contract value; declarative RFC-7009 revoke via auth.oauth.revoke above
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
  name: kit
  kind: oauth
```

**Config (Config Sync hard rule):** `oauth.client_id` / `oauth.client_secret`
land in integration-service config in BOTH `config/` (local) and the Helm
Secret under `deploy/` together, as lane-1 per-provider appends. A fully-unset
Kit provider renders `configured: false` (Connect disabled) and is safe hidden;
a *partial* config fails startup — so id+secret always land in the same change,
before Kit's L5.

**UI icon (manual, never generated):**
`ui/helio-app/src/integrations/icons/kit.svg` + register in
`ui/helio-app/src/integrations/providerIcons.ts`. Add the `kit`
`description_key` i18n label(s).

### Capability questions to resolve against `main` (flag at stage 1)

1. **`token_exchange_style` — ship the existing `form_secret`, not a new
   `json_secret`.** The closed enum is `form_secret | form_basic | json_basic`
   (`validate.go:154-161`; `oauth_exchange.go`). `json_basic` is JSON-body but
   puts the client credentials in an `Authorization: Basic` header
   (`oauth_exchange.go:106-125`), so none of the three sends
   `client_id`+`client_secret` **in a JSON body** — the exact shape Kit's
   documented authorization_code curl uses. An earlier draft concluded we must
   add a `json_secret` enum value and ship it as the default. **That is wrong,
   for a decisive architectural reason found by reading the refresh path:** the
   token **refresh** leg (`token_refresh.go:148-169`) does not use the custom
   `buildTokenRequest` at all — it uses `golang.org/x/oauth2`
   (`oauth2.Config.TokenSource(...).Token()`), which **always** encodes the body
   as `application/x-www-form-urlencoded`. `TokenExchangeStyle` there only toggles
   credential placement (body vs. `Authorization: Basic`, via
   `UsesHTTPBasicClientAuth()`) — it can never make the refresh body JSON. Kit
   uses rotating refresh (`refresh_lease: none`, serviced by the standard refresh
   path — see §5 Q2), so it **will** hit this form-only refresh path. Adding
   `json_secret` would make only the
   authorization_code leg JSON while the refresh leg stays form — an incoherent
   split that still could not honor the documented JSON content-type on refresh.
   The coherent choice is one encoding for both legs: **`form_secret`** (form
   body, standard OAuth2 `client_secret_post`).
   - **Is form-encoding safe against Kit's `/oauth/token`?** Yes, with high
     confidence: Kit's own `/oauth/revoke` example is `application/x-www-form-
     urlencoded` (§4), which proves the Doorkeeper OAuth controller family accepts
     form bodies; and every refreshing OAuth provider Helio already ships
     (`x`, `keap`, `signnow`, `hootsuite`) exchanges through this same form path
     against its real endpoint. The documented JSON content-type on `/oauth/token`
     is an example, not a rejection of form (`client_secret_post` is standard
     OAuth2). We record this divergence-from-docs explicitly and **confirm it
     empirically at L2** (a real authorization_code→token exchange with
     `form_secret`) before leaving the harness.
   - **Contingency, not up-front growth:** `json_secret` is introduced **only if**
     L2 proves Kit's `/oauth/token` authorization_code leg rejects form-encoding
     (highly unlikely). Even then it would fix only the auth-code leg — the
     form-only refresh leg would still have to be validated separately — so the
     enum buys little. Per "subtract before adding" and integration-service's
     rule that a new standard provider should not need a new capability unless
     the closed set genuinely cannot express it, we do not grow the shared,
     security-sensitive exchanger ahead of L2 evidence. Either way no bespoke
     adapter.

   *Review-finding resolution (verified against the official curls on
   2026-07-22).* The review is **accepted**: default flipped from `json_secret`
   to `form_secret`; `json_secret` demoted to an L2-gated contingency; the
   earlier §4/§5 claim that "both the authorization_code and refresh curl
   examples carry client_id+client_secret in a JSON body" is corrected (the
   refresh curl carries **only** `client_id`, and is itself buggy — it prints
   `"code"` in place of `"refresh_token"`). One factual refinement to the review:
   it stated "there is NO documented Kit example showing client_id+client_secret
   in a JSON body" — the **authorization_code** exchange curl is exactly such an
   example (Content-Type `application/json`, both creds in the body). That does
   not change the conclusion, because (a) Doorkeeper accepts form on the same
   stack and (b) the refresh leg is form-only regardless, so `form_secret`
   remains the correct, coherent default and no capability growth is justified
   up front.
2. **`refresh_lease` — ship `none`, the only legal `standard_oauth` value; no
   allowlist to join.** An earlier draft framed this as "check whether `main`
   gates `provider` leasing by an explicit provider allowlist; if so, add
   `kit`." **That premise is false.** `refresh_lease` under `standard_oauth` is
   not an allowlist — it is a single pinned scalar
   (`model/runtime_contract.go:42`: `refreshLeaseScope: OAuthLeaseNone`) enforced
   by strict equality (`:224-231`, run from `provider-gen`
   `validate.go:405` via `ValidateRuntimeContract`). There is no "add kit to the
   allowlist" operation; the contract accepts exactly `none` for this strategy.
   And `none` is also *semantically* correct here (§4): the rotating single-use
   refresh token and the variable `expires_in` are both serviced by the standard
   refresh path (`token_refresh.go` `firstNonEmpty` write-back + oauth2-derived
   `Expiry`), exactly as every shipped Google app does. `OAuthLeaseProvider` is a
   global per-**app** serialization lock reserved for `single_active_token: true`
   providers where one refresh invalidates every other connection's token — its
   sole user is X via the bespoke `RuntimeStrategyXExclusiveGrant`. Kit issues
   one independent token per connection, so there is nothing to serialize. If a
   *single-connection* refresh-concurrency race were ever a genuine concern it
   would map to `credential` scope (a per-credential key), **never** `provider`,
   and would still require deliberately widening the `standard_oauth` contract —
   which we are not doing. **No enum growth, no adapter.**

   *(The keap/signnow/hootsuite precedent is about a different axis: those tools
   grew a `refresh_lease` allowed-set on a **manual/bespoke** strategy contract,
   not the `standard_oauth` one, which has always pinned `none`.)*
3. **`disconnect_mode: provider_revoke` + declarative revoker — fully expressible
   today, no fallback needed.** Kit's revoke is a standard RFC-7009 form-POST
   carrying `token`/`client_id`/`client_secret`. Two corrections to the earlier
   draft:
   - `disconnect_mode: strategy` is **illegal** under `standard_oauth`: the
     contract permits only `provider_revoke` and `local_only`
     (`runtime_contract.go:41`; enforced by `ValidateRuntimeContract` and
     `validate.go:402`). `strategy` is reserved for bespoke adapter strategies
     (discord/github) and would fail `provider-gen --check`.
   - A declarative RFC-7009 revoke under `standard_oauth` uses
     `disconnect_mode: provider_revoke` **plus** an `auth.oauth.revoke` block,
     which `validate.go:503-505` *requires* when `provider_revoke` is set.
   The worry that "the existing revoker only supports Bearer-token revoke" is
   **unfounded**: the revoke enum already supports form client-auth with client
   creds in the body — `client_auth: form` (`OAuthRevokeClientAuthForm`,
   `model/catalog.go`; `validate.go:480` accepts `none|basic|form`). So we set
   `client_auth: form`, `token: refresh_token`, `fallback_token: access_token`,
   modeled directly on `integrations/providers/gmail/provider.yaml`'s revoke
   block (which uses `client_auth: none` because Google's revoke needs no client
   creds; Kit needs them, hence `form`). **No `local_only` fallback, no revoker
   growth.**

**Service-code footprint.** Item 1 (`form_secret`) and items 2–3 sit entirely
inside existing capabilities. The **one** genuine growth is
numeric-stable-key coercion in the declarative identity resolver (§4 identity):
Kit's `/account/id` is an integer and the base resolver's `jsonPointerString`
cannot read it. That is a small, tested widening of a shared reader — not a
bespoke `service/adapter_*.go`. So Kit is a **near-zero-service-code** provider:
one shared-resolver capability + the anycli service, and **no** provider-specific
adapter.

---

## 6. Test plan (five layers)

| Layer | What proves Kit works | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/kit` unit tests vs `httptest` fakes: request method/path/body, injected `Authorization: Bearer`, cursor pagination, `--json` + plain error rendering. Definition JSON strict-decodes. | No |
| **L2** dev harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<real> anycli kit -- account get` and `… -- subscriber list` / `-- broadcast list` against live `api.kit.com/v4`. **Confirm the token exchange** — run one real authorization_code→token exchange with the dev app's client creds to prove the default `form_secret` (form body, `client_secret_post`) shape is accepted by `/oauth/token` end-to-end (capability Q1). Only if that exchange is rejected do we introduce `json_secret`. | **Yes** — a real Kit account access token (from the account pool) + the dev app client_id/secret |
| **L3** generation + suites | `provider-gen` then `provider-gen --check` (five projections regen together, run locally only — not committed on the tool branch); `helio-cli` + `integration-service` unit suites green, incl. the numeric-stable-key coercion tests (§4 identity — the one capability growth; Q1–Q3 add no growth). Branch is *expected* to fail `provider-gen --check` in CI until the batch-end merge. | No |
| **L4** singleton + seeded creds | `make run-singleton` (env=dev) → `POST /internal/test-only/connections/seed` for provider `kit` with a real dedicated-account `access_token` + `refresh_token` and a deliberately short `expires_at` (force the gateway refresh-and-write-back path, since Kit tokens expire in 48h) → `heliox tool kit -- account get` returns real account JSON via the token gateway. Requires the pinned/`replace`d anycli carrying the `kit` definition. | **Yes** — real Kit access+refresh token; dev app client creds in local uncommitted `config/cloud.yaml` for the refresh exercise |
| **L5** full connect flow (pre-flip, human-in-the-loop) | `heliox tool kit auth` → connect link → **real Kit OAuth consent on a dev/unpublished app** → `oauth_connected` system event fires → unseeded `heliox tool kit -- account get` succeeds through the freshly created connection. oauth_review ⇒ human consent (lane 3); this validates the connect UX the L4 seed bypasses. | **Yes** — a real Kit creator account for live consent; the registered (unpublished-OK) OAuth app |

**Gating (master plan §2):** dev-mode app creation (lane 1) gates L4/L5 — a
real token and client creds only exist once the Kit app is registered. Kit App
Store **review clearance gates only the `visible: true` flip**, never dev, L4,
or the batch-end merge. Go-live = L5 pass **and** App Store approval, then flip
`presentation.visible: true` + regenerate as the single change.

## 7. Definition-of-done checklist (this tool)

- [ ] anycli `kit.json` + `internal/tools/kit/` service + L1 tests green;
      `RegisterService("kit", …)` in `register.go` (rides batch-end merge).
- [ ] L2 real-API harness pass, incl. the token-exchange-style confirmation.
- [ ] Bundle `integrations/providers/kit/provider.yaml` (hidden,
      `refresh_lease: none`, `disconnect_mode: provider_revoke` + `auth.oauth.revoke`)
      + numeric-stable-key coercion in the declarative identity resolver with
      tests (§4 identity — the sole capability growth).
- [ ] `oauth.client_id`/`client_secret` appended to `config/` + `deploy/`
      (lane 1, before L5).
- [ ] UI icon `kit.svg` + `providerIcons.ts` + i18n label.
- [ ] AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` (batch publish).
- [ ] L3/L4 validated on-branch (local regen + `replace` build).
- [ ] L5 + App Store approval → `visible: true` + regenerate (go-live change).
