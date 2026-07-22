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
- token: `POST https://api.kit.com/v4/oauth/token`
  (grant `authorization_code` then `refresh_token`; body carries `client_id` +
  `client_secret` for the confidential flow). **Documented content-type is
  `application/json`** — Kit's refresh example sends
  `-H 'Content-Type: application/json'` with a JSON body. This is the crux of
  the token-exchange-style decision (§5 Q1): the default bundle shape must
  match this JSON body, not a form-encoded one.
- revoke: `POST https://api.kit.com/v4/oauth/revoke` (RFC 7009; body `token`,
  `client_id`, `client_secret`, optional `token_type_hint`; always `200`).
  **Documented content-type here is `application/x-www-form-urlencoded`** — Kit
  deliberately uses a *different* content-type from `/oauth/token`. This
  divergence is decisive: because Kit explicitly form-encodes revoke while
  JSON-encoding token, we must NOT assume the token endpoint silently accepts
  form-encoding.

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
revoked; reuse returns `invalid_grant`). Because the lifetime is set by the
provider per-response, the connection must derive `expires_at` dynamically from
each response's `expires_in` — which is exactly why provider-driven refresh
leasing (`refresh_lease: provider`, like `x`) is correct, NOT `none`
(Notion-style non-expiring) and NOT a hard-coded TTL.

**Identity (verified `GET /v4/account`):**
```
{ "user": { "email": str, "id": int },
  "account": { "id": int, "name": str, "plan_type": str,
               "primary_email_address": str, "timezone": {…}, … } }
```
→ `identity.source: userinfo`, `url: https://api.kit.com/v4/account`,
`stable_key: /account/id`, `label_candidates: [/account/name, /account/primary_email_address, /user/email, /account/id]`.
`account.id` is an **integer** — relies on the numeric stable-key coercion the
standard_oauth identity resolver already supports on `main` (added for HubSpot;
confirm present before coding — if absent it is a known, already-solved
capability, not new work).

**Credential fields stored by Helio:** `access_token`, `refresh_token`,
`expires_at` (from `expires_in`), `account_key = connection.account_key`
(the resolved `/account/id`). The user token never touches the bundle; it
enters via the OAuth callback and lives in Vault.

---

## 5. Helio provider bundle plan (`integrations/providers/kit/provider.yaml`)

Ships **hidden-first** (`presentation.visible: false`) — decouples the App
Store review clock from the anycli pin. Default `standard_oauth`; no
provider-specific Go adapter is warranted (Kit is a textbook Doorkeeper-style
authorization-code server — response shapes and lifecycle stay inside the
`standard_oauth` capability set). The one non-default piece is the token
**exchange content-type**: Kit documents a JSON body (see §5 Q1), which needs a
new `json_secret` enum value on `standardOAuthExchanger`. That is planned
capability growth done up front, not an L2-contingent fallback.

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
    token_exchange_style: json_secret   # documented shape: JSON body, client_id+client_secret in body (§5 Q1 — new enum value)
    pkce: none                          # confidential web-server client
    display_scopes: [public]
    single_active_token: false
    refresh_lease: provider

identity:
  source: userinfo
  url: https://api.kit.com/v4/account
  stable_key: /account/id
  label_candidates: [/account/name, /account/primary_email_address, /user/email, /account/id]

connection:
  mode: isolated
  disconnect_mode: strategy        # Kit exposes RFC-7009 revoke → declarative revoker
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

1. **`token_exchange_style` for Kit's token endpoint — new `json_secret` enum
   value (primary planned work).** The existing closed enum is
   `form_secret | form_basic | json_basic`. Kit's **documented** confidential
   exchange (both the authorization_code and refresh curl examples) sends a
   **JSON body** with `client_id`+`client_secret` **in the body**
   (`Content-Type: application/json`) — verified against
   `developers.kit.com/api-reference/oauth-refresh-token-flow` (2026-07-22),
   which shows `-H 'Content-Type: application/json'`. None of the three enum
   values expresses this (there is no `json_secret`: `json_basic` puts creds in
   an `Authorization: Basic` header, not the JSON body). This is exactly the
   undocumented-assumption trap: Kit *deliberately* form-encodes `/oauth/revoke`
   while JSON-encoding `/oauth/token` (§4), so inferring that the token endpoint
   silently also accepts form-encoding is unsafe. **Plan:** the primary work is
   to add one reviewed enum value **`json_secret`** (JSON body, `client_id` +
   `client_secret` in the body) to `standardOAuthExchanger`, ship it as the
   default (§5 bundle), and confirm it at L2 against the real token endpoint.
   `form_secret` is demoted to a *fallback to consider only if* we want to avoid
   the enum addition — and it would only be viable if L2 proves Kit's Doorkeeper
   server tolerates form-encoding despite the docs; we do not rely on that.
   Either way no bespoke adapter — this is a single reviewed enum growth on the
   shared exchanger.
2. **`refresh_lease: provider` allowed-set.** Prior refreshing OAuth tools
   (keap, signnow, hootsuite) each had to grow the standard_oauth
   `refresh_lease` allowed-set. Check whether `main` still gates `provider`
   leasing by an explicit provider allowlist; if so, add `kit`. If the set has
   since been generalized, no change. (Either way this is config/enum growth,
   not an adapter.)
3. **`disconnect_mode: strategy` + declarative revoker.** Kit's revoke is a
   standard RFC-7009 form-POST with `token`/`client_id`/`client_secret`. Verify
   the `declarativeRevoker` capability can express a body-param revoke with
   client creds; if the existing revoker only supports Bearer-token revoke,
   fall back to `disconnect_mode: local_only` (Notion precedent) for the first
   ship and grow the revoker in a follow-up — do not block the tool on it.

No adapter (`service/adapter_*.go`) is expected. If items 1–3 all sit inside
existing capabilities, this is a **zero-service-code** provider.

---

## 6. Test plan (five layers)

| Layer | What proves Kit works | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/kit` unit tests vs `httptest` fakes: request method/path/body, injected `Authorization: Bearer`, cursor pagination, `--json` + plain error rendering. Definition JSON strict-decodes. | No |
| **L2** dev harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<real> anycli kit -- account get` and `… -- subscriber list` / `-- broadcast list` against live `api.kit.com/v4`. **Confirm the token exchange** — run one real authorization_code→token exchange with the dev app's client creds to prove the documented `json_secret` (JSON body) shape works end-to-end (capability Q1); form-encoding is not the default and is only probed if we later want to avoid the enum addition. | **Yes** — a real Kit account access token (from the account pool) + the dev app client_id/secret |
| **L3** generation + suites | `provider-gen` then `provider-gen --check` (five projections regen together, run locally only — not committed on the tool branch); `helio-cli` + `integration-service` unit suites green, incl. any capability-growth tests (Q1/Q2/Q3). Branch is *expected* to fail `provider-gen --check` in CI until the batch-end merge. | No |
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
- [ ] Bundle `integrations/providers/kit/provider.yaml` (hidden) + any
      capability growth from §5 Q1–Q3 with tests.
- [ ] `oauth.client_id`/`client_secret` appended to `config/` + `deploy/`
      (lane 1, before L5).
- [ ] UI icon `kit.svg` + `providerIcons.ts` + i18n label.
- [ ] AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` (batch publish).
- [ ] L3/L4 validated on-branch (local regen + `replace` build).
- [ ] L5 + App Store approval → `visible: true` + regenerate (go-live change).
