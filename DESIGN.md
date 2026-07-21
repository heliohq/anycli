# Tool design: Mailchimp

**Catalog row:** #51 · Product: Mailchimp · anycli id `mailchimp` · provider key `mailchimp` · auth lane `oauth_light` · Wave 1 · Email & Messaging.
**Branches:** anycli `tool/mailchimp` (this worktree), Helio `tool/mailchimp`.
**Scratch file:** stripped by the batch lead at batch end.

## 1. Independent verification of the auth lane (official docs)

Verified against the official Mailchimp developer docs
(`https://mailchimp.com/developer/marketing/guides/access-user-data-oauth-2/` and
`https://mailchimp.com/developer/marketing/docs/fundamentals/`):

- **OAuth2 authorization-code flow, self-serve registration.** Apps are created on the
  account's "Registered Apps" page (no review program); `client_id`/`client_secret` are
  shown once at creation. **The `oauth_light` lane in the catalog and the oauth-audit
  verdict are correct** — no divergence on the lane itself.
- Authorize: `GET https://login.mailchimp.com/oauth2/authorize?response_type=code&client_id=…&redirect_uri=…`.
- Token: `POST https://login.mailchimp.com/oauth2/token`, **form-encoded**, client auth
  **in the body** (`grant_type=authorization_code`, `client_id`, `client_secret`,
  `redirect_uri`, `code`) → maps to `token_exchange_style: form_secret`.
- **No PKCE** documented → `pkce: none`. **No scopes** — access is all-or-nothing per
  account → wire `scopes` empty; use `display_scopes` capability slugs for the consent
  page (bitly precedent).
- **Access tokens never expire; there is no refresh token** ("Mailchimp Marketing access
  tokens do not expire, so you don't need to use a refresh_token") → `refresh_lease: none`;
  L4 seeds `access_token` only (no `refresh_token`/`expires_at` — nothing to refresh).
- **No documented OAuth revoke endpoint** → `disconnect_mode: local_only`.
- **Data-center prefix (`dc`) is required to call the API.** Base URL is
  `https://<dc>.api.mailchimp.com/3.0/`. For OAuth tokens the `dc` comes only from
  `GET https://login.mailchimp.com/oauth2/metadata`, documented with header
  `Authorization: OAuth <access_token>` (note: the guide shows the `OAuth` scheme, not
  `Bearer`). The metadata response carries `dc` (and per long-standing shape:
  `accountname`, `user_id`, `role`, `login{…}`, `api_endpoint`). For API keys the `dc`
  is the key's suffix (`…-us6`).
- **The Marketing API itself accepts both API keys and OAuth tokens identically**, via
  HTTP Basic (`anystring:TOKEN`) or `Authorization: Bearer <TOKEN>` — official
  fundamentals page states both token types "can be used to make authenticated requests
  the same way".
- Rate limits: 10 simultaneous connections, 120 s per-call timeout. Errors are
  problem-detail JSON (`type`/`title`/`status`/`detail`) on non-2xx.

### Recorded divergences / risks vs the generic Helio capability set

Two gaps between Mailchimp's contract and today's `standard_oauth` golden path — both
Helio-side, neither changes the anycli design. Verify both with the real dev app the
moment lane 1 delivers credentials (before the batch-end merge), then pick the
resolution:

1. **Userinfo auth scheme.** `fetchUserInfo`
   (`go-services/integration-service/service/oauth_exchange.go`) hardcodes
   `Authorization: Bearer <token>`; the metadata endpoint is documented with the
   `OAuth` scheme. It is plausible the endpoint also accepts `Bearer` (the Marketing
   API does) but this is **unverified**. If `Bearer` is rejected: grow the generic
   capability with a reviewed enum (e.g. `identity.userinfo_auth_scheme: bearer|oauth`,
   default `bearer`) in provider-gen + `fetchUserInfo`, per the extension contract's
   "grow one reviewed enum value instead of an adapter" guidance. Do NOT write an
   `adapter_mailchimp.go` for this — the gap is not provider-specific logic, only a
   header scheme.
2. **Numeric stable key.** `declarativeIdentityResolver.jsonPointerString`
   (`service/declarative_identity.go`) requires the stable-key JSON-Pointer value to be
   a **string**; Mailchimp's metadata `user_id` / `login/login_id` are JSON numbers.
   Preferred fix: extend `jsonPointerString` to stringify JSON numbers (decode userinfo
   with `json.Number` to avoid float64 precision loss) — generic, benefits every
   upcoming numeric-id provider in the 300-catalog. Fallback if that change is
   rejected: `stable_key: /accountname` (a string), with the documented caveat that an
   account rename mints a new `account_key` and therefore a duplicate connection on
   reconnect. At L5, additionally confirm which metadata field is per-*account* stable
   (`user_id` vs `login/login_id`) when one login owns multiple Mailchimp accounts.

## 2. API surface the tool wraps (and why)

Marketing API v3.0 only (`https://<dc>.api.mailchimp.com/3.0`). Transactional
(Mandrill) is a separate product/API and out of scope. Rationale: an AI teammate's
Mailchimp jobs are (a) keep the audience current (add/update/tag subscribers), (b) look
up who is subscribed and how segments are defined, (c) draft, test, schedule, and send
email campaigns, (d) report on how campaigns performed. That maps to five resource
groups plus search and ping:

| Group | Endpoints |
|---|---|
| health | `GET /ping` |
| audience | `GET /lists`, `GET /lists/{list_id}` |
| member | `GET /lists/{id}/members`, `GET/PUT /lists/{id}/members/{subscriber_hash}` (upsert), `DELETE …` (archive), `POST …/tags` (tag/untag) |
| segment | `GET /lists/{id}/segments`, `GET /lists/{id}/segments/{seg_id}/members` |
| campaign | `GET /campaigns`, `GET /campaigns/{id}`, `POST /campaigns` (type `regular`), `PUT /campaigns/{id}/content`, `POST /campaigns/{id}/actions/{send,test,schedule,unschedule}`, `DELETE /campaigns/{id}` |
| report | `GET /reports`, `GET /reports/{campaign_id}` |
| template | `GET /templates` |
| search | `GET /search-members`, `GET /search-campaigns` |

Deliberately excluded from v1 (add later on demand): classic Automations (legacy),
Customer Journeys (limited API), e-commerce stores, batch operations, webhooks, file
manager. Excluding them keeps the surface at ~24 operations, in line with existing
service tools.

`subscriber_hash` is the MD5 of the lowercase email; the CLI accepts `--email` and
computes the hash client-side (deterministic across API versions), with `--hash` as a
passthrough alternative.

## 3. anycli definition (stage 1–2)

**Type decision: `service`.** Stage-1 rubric: Mailchimp has no official CLI at all, so
the `cli` branch fails at the first criterion. Implement `internal/tools/mailchimp/`
against the Marketing API (the 21-of-23 default).

`definitions/tools/mailchimp.json`:

```json
{
  "name": "mailchimp",
  "type": "service",
  "description": "Mailchimp Marketing (audiences, members, campaigns, reports)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "MAILCHIMP_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single credential field — `dc` is **not** a credential (see below). The definition JSON
merges freely mid-batch; the `RegisterService("mailchimp", &mailchimp.Service{})` line
in `internal/tools/register.go` is a shared surface and rides the batch-end merge.

### DC resolution (the one Mailchimp-specific design point)

The service resolves the API base per invocation, in order:

1. **API-key suffix**: if the token matches `.*-([a-z]{2,4}[0-9]+)$`, base =
   `https://<suffix>.api.mailchimp.com/3.0`. (Covers the L2 harness with a test-account
   API key — the Marketing API accepts API keys and OAuth tokens identically.)
2. **OAuth metadata**: otherwise `GET https://login.mailchimp.com/oauth2/metadata` with
   `Authorization: OAuth <token>`, use the returned `api_endpoint` + `/3.0`. One extra
   round-trip per invocation; acceptable (heliox spawns a fresh process per command, so
   there is nothing to cache into), and it keeps the Helio credential contract at the
   standard closed allow-list — the alternative (projecting `dc` through the token
   gateway) would require widening the reviewed credential-source set
   (`token.access_token` / `connection.account_key` / …) for one provider, which the
   extension contract calls out as a security boundary, not a convenience limit.

Struct follows the bitly/notion testability shape: `BaseURL` (Marketing API override),
`MetadataURL` (metadata override), `HC`, `Out`, `Err`, so unit tests point both at
`httptest` servers.

### Command tree and output

```
mailchimp ping
mailchimp audience list|get <list_id>
mailchimp member list <list_id> [--status …] | get <list_id> --email|--hash
                | upsert <list_id> --email … [--status subscribed|…] [--status-if-new …] [--merge JSON] [--tags a,b]
                | archive <list_id> --email|--hash
                | tag <list_id> --email|--hash --add a,b --remove c
mailchimp segment list <list_id> | members <list_id> <segment_id>
mailchimp campaign list [--status …] | get <id>
                  | create --list <list_id> [--segment <seg_id>] --subject … --from-name … --reply-to … [--title …]
                  | set-content <id> --html …|--html-file …|--template <template_id> [--plain-text …]
                  | send <id> | test <id> --emails a@b,c@d | schedule <id> --at RFC3339 | unschedule <id> | delete <id>
mailchimp report list | get <campaign_id>
mailchimp template list
mailchimp search members --query … | campaigns --query …
mailchimp list-* pagination via --count/--offset and --fields projection passthrough
```

Output contract = bitly precedent: **provider JSON passthrough on stdout** (+ newline);
API requests send `Authorization: Bearer <token>` (official Bearer support). Exit
codes: 0 success, 1 runtime/API failure (typed apiError rendering Mailchimp's
problem-detail `title`/`status`/`detail`), 2 usage/parse errors; `--json` structured
error envelope per the notion precedent. 401 additionally prints a
credential-rejected hint. Action endpoints returning `204 No Content`
(send/schedule/unschedule) emit a small `{"ok":true,"action":"send","id":"…"}` receipt
so agents always get JSON.

## 4. Helio provider bundle plan

**Naming axes (master plan §3):** ① CLI command word `mailchimp` (flat command, no
group — independent brand, no family); ② anycli id `mailchimp`; ③ provider key
`mailchimp`. ② == ③, so **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go` and no `toolGroups` involvement.

`integrations/providers/mailchimp/provider.yaml` (hidden-first):

```yaml
schema: helio.provider/v1
key: mailchimp
go_name: Mailchimp

presentation:
  name: Mailchimp
  description_key: mailchimp
  consent_domain: mailchimp.com
  visible: false          # hidden-first; flip is the go-live change after L5
  order: 130

auth:
  type: oauth
  owner: individual       # a person connects a Mailchimp account; gmail/bitly precedent
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://login.mailchimp.com/oauth2/authorize
    token_url: https://login.mailchimp.com/oauth2/token
    token_exchange_style: form_secret   # form body, client id+secret in body
    pkce: none
    authorize_params: {}
    # Mailchimp has no wire-level scopes (all-or-nothing); display-only slugs,
    # bitly/notion precedent, rendered via i18n tools.scopes.<slug>.
    display_scopes: [manage_audiences, manage_campaigns, view_reports]
    single_active_token: false
    refresh_lease: none   # tokens never expire; no refresh token exists

identity:
  source: userinfo
  url: https://login.mailchimp.com/oauth2/metadata   # §1 gap 1: verify Bearer works, else add userinfo_auth_scheme enum
  stable_key: /user_id                               # §1 gap 2: needs numeric stringify; fallback /accountname
  label_candidates: [/accountname, /login/email, /dc]

connection:
  mode: isolated
  disconnect_mode: local_only     # no documented provider revoke endpoint
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
  name: mailchimp
  kind: oauth
```

No service-side adapter expected (see §1 for the two possible generic-capability
changes). Other Helio artifacts, all per the pipeline stages:

- **Config (lane 1 output)**: `integration.providers.mailchimp.oauth.client_id/client_secret`
  in `config/` + the Helm Secret under `deploy/` together (Config Sync rule); dev app's
  redirect URI must exactly match the singleton callback (Mailchimp requires exact
  match; HTTPS recommended but not enforced, so a localhost callback works for dev).
- **Icon**: `ui/helio-app/src/integrations/icons/mailchimp.svg`; the
  `providerIcons.ts` registration rides the batch-end merge.
- **AI docs**: `agents/plugins/heliox/skills/tool/mailchimp/mailchimp.md` (notion/x
  layout); plugin version bump + marketplace publish ride the batch-end merge.
- **provider-gen**: run `go run ./cmd/provider-gen` + `--check` locally for validation
  only; the five projections are NOT committed on this branch (batch lead owns the one
  canonical regen). This branch is expected to fail `provider-gen --check` in CI until
  batch end.
- **helio-cli build for L4**: local, uncommitted `replace github.com/heliohq/anycli =>
  ../../../anycli/.claude/worktrees/tool-mailchimp` in `helio-cli/go.mod`.

## 5. Test plan (five layers)

| Layer | What runs for Mailchimp | External credentials? |
|---|---|---|
| L1 | anycli `go test ./...`; httptest fakes assert: Bearer header on API calls; `OAuth` scheme on metadata call; dc resolution order (api-key suffix beats metadata; metadata `api_endpoint` used verbatim); MD5(lowercase(email)) hashing; request bodies for member upsert / tag / campaign create / set-content / schedule; 204-receipt envelopes; problem-detail error rendering + exit codes 0/1/2; `--json` error envelope. TDD: tests first. | No |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<api-key> anycli mailchimp -- ping` then real runs of audience/member/campaign/report verbs against the test account (API-key suffix path), plus at least one run with a real **OAuth token** once the dev app exists, to exercise the metadata dc path live. | **Yes** — lane 2 test account (API key); OAuth token additionally needs lane 1 dev app |
| L3 | Local `provider-gen` + `--check` against the branch bundle; `helio-cli` build + `go test ./cmd/heliox/cmds/tool/` with the local `replace`; integration-service unit suite (incl. any new `userinfo_auth_scheme` / numeric-stable-key tests from §1, written first). Nothing committed from the regen. | No |
| L4 | Singleton (`env: dev`); `POST /internal/test-only/connections/seed` with provider `mailchimp`, real OAuth access token, **`access_token` only — no `refresh_token`/`expires_at`** (non-expiring token class, Slack-bot precedent in the skill's L4 notes); then `heliox tool mailchimp -- ping` and `-- audience list` must return live data through the real token gateway. | **Yes** — real OAuth token minted from lane 1 dev app |
| L5 | Human-in-the-loop (lane 3, per-batch sweep, after batch-end merge, provider still hidden): `heliox tool mailchimp auth` → connect link → real Mailchimp consent → `oauth_connected` event on the channel → unseeded live run. Also confirms §1 gap 1/2 resolutions end-to-end (identity label shows the account name; account_key stable). Then the visible flip + regen as the single go-live change. | **Yes** — lane 2 account + lane 1 registered app |

## 6. Open items

1. §1 gap 1 (metadata `Bearer` acceptance) — resolve empirically at first L2-with-OAuth
   run; capability enum change in integration-service if needed.
2. §1 gap 2 (numeric stable key) — propose the generic `json.Number` stringify in the
   Helio worktree (tests first); fallback `/accountname`.
3. Confirm per-account stability of `user_id` vs `login/login_id` for multi-account
   logins at L5.
4. `display_scopes` slugs need matching i18n keys (`tools.scopes.manage_audiences`, …)
   in helio-app.
