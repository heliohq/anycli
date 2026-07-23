# Klaviyo tool — per-tool design (tool/klaviyo)

Catalog row 125 · anycli id `klaviyo` · provider key `klaviyo` · auth lane `oauth_light` · Wave 2 · Marketing.
Scratch design per master plan §2; the batch lead strips this file at batch end.

## 0. Verification against official docs (independent judgment)

Everything below was checked against Klaviyo's official developer docs
(developers.klaviyo.com), not inherited from the catalog or the audit:

- **Auth lane `oauth_light` is CONFIRMED.** OAuth app creation is self-serve
  in the Klaviyo UI (owner/admin/manager roles); the install URL can be shared
  with any Klaviyo account. App review applies only to App Marketplace
  listing, which we do not need. Official page: `set_up_oauth` /
  `create_a_public_oauth_app`. No divergence from the audit verdict
  (oauth-audit.md row 125) or the catalog row.
- **Token endpoint host matters.** Since 2025-03-31 Klaviyo blocks token and
  revoke traffic on `www.klaviyo.com`; token/revoke MUST use `a.klaviyo.com`.
  Authorize stays on `www.klaviyo.com`.
- **PKCE S256 is mandatory** for both public and confidential clients — the
  bundle must set `pkce: s256`, unlike most shipped bundles (`none`).
- **Token endpoint client auth is HTTP Basic + form body** (JSON bodies fail)
  → `token_exchange_style: form_basic`. Precedent bundles mostly use
  `form_secret`/`json_basic`; `form_basic` is a reviewed enum value already.
- **Token semantics:** access token ~1 h (trust `expires_in`); refresh token
  valid until uninstall, revoked after 90 days of non-use; refresh limited to
  10 calls/min; refresh issues new tokens but there is **no
  rotation-revocation** (old tokens stay valid until natural expiry), so
  concurrent refreshes are harmless → `refresh_lease: none` (matches the
  google/microsoft precedent). `invalid_grant` on refresh means uninstall.
- **Tokens bind to a Klaviyo *account* (workspace), not a user.**
- **API surface:** base `https://a.klaviyo.com/api/...`, JSON:API request /
  response conventions, dated-revision versioning via a **`revision` header**
  (latest stable `2026-07-15`; the reference marks the header *required* on
  every endpoint). Cursor pagination `page[cursor]` with top-level
  `links.next`. Errors are a JSON:API `errors` array (verified live with
  curl: 401 `not_authenticated` / `authentication_failed`).
- **`accounts:read` scope is added by default to every OAuth app** and Klaviyo
  itself recommends `GET /api/accounts` as the post-install identity call.

## 1. What an AI teammate does with Klaviyo → API surface

Klaviyo is the e-commerce marketing hub: audience (profiles/lists/segments),
messaging (campaigns/flows/templates), and analytics (metrics/events/
reporting). An AI teammate's real jobs:

1. **Audience ops** — look up a customer's profile and consent state, add or
   subscribe people to lists, suppress/unsubscribe on request, inspect
   segment membership.
2. **Campaign/flow visibility & control** — list campaigns and their status,
   read a campaign's messages, trigger a ready campaign's send, check which
   flows are live and toggle a flow's status (draft/manual/live).
3. **Performance reporting** — "how did last week's campaign do?" via the
   Reporting API (campaign/flow values & series), plus metric aggregates for
   ad-hoc questions (revenue, opens over time).
4. **Event plumbing** — inspect recent events for a profile, create a custom
   event to trigger a flow.

Wrapped endpoints (all under `https://a.klaviyo.com/api`, header
`revision: 2026-07-15`, pinned as a package const):

| Command group | Endpoints |
|---|---|
| `account get` | `GET /accounts` |
| `profile list\|get` | `GET /profiles`, `GET /profiles/{id}` (supports `--filter`, e.g. `equals(email,"x@y.com")`) |
| `profile create\|update` | `POST /profiles`, `PATCH /profiles/{id}` |
| `profile subscribe\|unsubscribe` | `POST /profile-subscription-bulk-create-jobs`, `POST /profile-subscription-bulk-delete-jobs` (single-profile convenience over the bulk-job API; 202 job receipt passthrough) |
| `profile suppress\|unsuppress` | `POST /profile-suppression-bulk-create-jobs`, `POST /profile-suppression-bulk-delete-jobs` |
| `list list\|get\|create` | `GET /lists`, `GET /lists/{id}`, `POST /lists` |
| `list profiles` | `GET /lists/{id}/profiles` |
| `list add-profiles\|remove-profiles` | `POST /lists/{id}/relationships/profiles`, `DELETE /lists/{id}/relationships/profiles` |
| `segment list\|get\|profiles` | `GET /segments`, `GET /segments/{id}`, `GET /segments/{id}/profiles` |
| `campaign list\|get\|messages\|send` | `GET /campaigns` (**required** `filter=equals(messages.channel,'email'\|'sms'\|'mobile_push')` — surfaced as `--channel`, default `email`), `GET /campaigns/{id}`, `GET /campaigns/{id}/campaign-messages`, `POST /campaign-send-jobs` |
| `flow list\|get\|status` | `GET /flows`, `GET /flows/{id}`, `PATCH /flows/{id}` (status: draft/manual/live) |
| `metric list\|get\|aggregate` | `GET /metrics`, `GET /metrics/{id}`, `POST /metric-aggregates` |
| `event list\|get\|create` | `GET /events`, `GET /events/{id}`, `POST /events` |
| `template list\|get` | `GET /templates`, `GET /templates/{id}` |
| `report campaign\|flow` | `POST /campaign-values-reports`, `POST /campaign-series-reports`, `POST /flow-values-reports`, `POST /flow-series-reports` (`--series` flag selects series vs values) |

Deliberately out of v1: catalogs, coupons, images, webhooks, forms, reviews,
custom objects, campaign *creation* (multi-step create→message→template→
assign; low agent value vs. reading + sending prepared campaigns). Add later
by extending the cobra tree.

## 2. anycli definition & service (stage 1–2)

**Stage-1 rubric: `service` type.** No official Klaviyo CLI exists at all —
the cli-type conditions fail at the first test. Service implementation
against the REST API, like 21 of 23 existing definitions.

`definitions/tools/klaviyo.json`:

```json
{
  "name": "klaviyo",
  "type": "service",
  "description": "Klaviyo as a tool (OAuth 2.0 access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "KLAVIYO_ACCESS_TOKEN"}
      }
    ]
  }
}
```

- Package `internal/tools/klaviyo/` (id has no dashes — package name == id),
  registered in `internal/tools/register.go` as
  `RegisterService("klaviyo", &klaviyo.Service{})` — **registration rides the
  batch-end merge**; the definition JSON and package merge freely.
- Service struct copies the notion/bitly shape: `BaseURL` (tests point at
  httptest), `HC`, `Out`, `Err`; cobra tree with `SilenceUsage/Errors`; exit
  codes 0 / 1 (typed `apiError` from the JSON:API `errors` array: first
  error's `code`/`title`/`detail` + HTTP status) / 2 (usage).
- **Auth header selection (explicit, tested):** `Authorization: Bearer <tok>`
  normally; if the injected credential starts with Klaviyo's documented
  private-key prefix `pk_`, send `Authorization: Klaviyo-API-Key <tok>`
  instead. This is keyed on Klaviyo's own documented key format (not a silent
  fallback) and makes the L2 harness runnable with a self-serve private key
  before the lane-1 OAuth app exists.
- Every request carries `revision: 2026-07-15` (package const, single owner)
  and `Accept: application/json`; writes add
  `Content-Type: application/json`.
- **JSON output:** provider JSON:API passthrough on stdout + newline (bitly
  convention — Klaviyo responses are already well-formed JSON with `data` /
  `links`); `--json` accepted for uniformity and switches *error* rendering
  to the structured envelope (notion convention).
- **Shared flags:** `--cursor` → `page[cursor]`, `--page-size` → `page[size]`
  (1–100), `--filter` (raw JSON:API filter passthrough), `--fields`,
  `--include`, `--sort` where the endpoint supports them; `campaign list
  --channel email|sms|mobile_push` (required-filter ergonomics). 429s are
  reported, not retried (agent decides; rate limits are per-account
  burst/steady).

TDD per anycli AGENTS.md: httptest fakes asserting method/path/query, both
auth header schemes, revision header, body shapes (e.g. subscription job's
`data.attributes.profiles.data[]` + optional list relationship), pagination
flag mapping, error rendering both plain and `--json`, plus a
`harness_test.go` like siblings.

## 3. Credential fields & auth flow (verified oauth_light)

- Credential field: `access_token` (OAuth bearer). Acquisition/refresh is
  entirely Helio-side (token gateway); anycli only injects
  `KLAVIYO_ACCESS_TOKEN`.
- Flow: authorization-code + PKCE S256 (mandatory), self-serve app,
  space-separated scopes, `state` owned by integration-service. Authorize
  `https://www.klaviyo.com/oauth/authorize`; token
  `https://a.klaviyo.com/oauth/token` (Basic client auth + form body);
  revoke `https://a.klaviyo.com/oauth/revoke` (same auth; body `token` +
  optional `token_type_hint`). Authorization codes expire in 5 min; redirect
  URI must exactly match the allowlist at klaviyo.com/manage-apps.
- integration-service's `standardOAuthExchanger` already covers all of this
  (`form_basic` + `pkce: s256`) — **zero service-side Go expected**.
- Scopes (least-permissive set matching §1, space-separated):
  `accounts:read campaigns:read campaigns:write events:read events:write
  flows:read flows:write lists:read lists:write metrics:read profiles:read
  profiles:write segments:read subscriptions:write templates:read`
  (`accounts:read` is force-included by Klaviyo; `subscriptions:write` +
  `lists:write` + `profiles:write` are the documented requirement for the
  bulk subscribe job).

## 4. Helio provider bundle plan (hidden-first)

Naming axes: ① `klaviyo` (flat command, `tool.command` omitted = defaults to
name) · ② `klaviyo` · ③ `klaviyo`. **Id == key → no `toolToProvider` entry,
no group.** Bundle `integrations/providers/klaviyo/provider.yaml`:

```yaml
schema: helio.provider/v1
key: klaviyo
go_name: Klaviyo

presentation:
  name: Klaviyo
  description_key: klaviyo
  consent_domain: klaviyo.com
  visible: false            # hidden-first; flip is the go-live change
  order: 130                # Marketing block; batch lead may renumber

auth:
  type: oauth
  owner: assistant          # token binds to a Klaviyo account (workspace), notion precedent
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://www.klaviyo.com/oauth/authorize
    token_url: https://a.klaviyo.com/oauth/token   # NOT www — blocked since 2025-03-31
    token_exchange_style: form_basic               # Basic auth + form body (JSON fails)
    pkce: s256                                     # mandatory for all client types
    scopes: [accounts:read, campaigns:read, campaigns:write, events:read,
             events:write, flows:read, flows:write, lists:read, lists:write,
             metrics:read, profiles:read, profiles:write, segments:read,
             subscriptions:write, templates:read]
    single_active_token: false
    refresh_lease: none      # no rotation-revocation; concurrent refresh harmless
    revoke:
      url: https://a.klaviyo.com/oauth/revoke
      client_auth: basic
      token: refresh_token   # revoking the refresh token ends the grant
      fallback_token: access_token
      token_type_hint: none  # generator: must be none when fallback_token set

identity:
  source: userinfo
  url: https://a.klaviyo.com/api/accounts
  stable_key: /data/0/id     # account id; JSON pointer array indices supported
  label_candidates: [/data/0/attributes/contact_information/organization_name, /data/0/id]

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
  name: klaviyo
  kind: oauth
```

**Known risk — identity fetch and the `revision` header.** The declarative
`fetchUserInfo` sends only `Authorization` + `Accept`; Klaviyo's reference
marks `revision` required on `/api/accounts`, but the versioning policy doc
is silent on omission behavior (only *retired* revisions are documented to
fall forward), and it cannot be probed unauthenticated (auth is checked
first — verified by curl). **Verify at L4 with a real dev-app token**: plain
`GET /api/accounts` with Bearer only. If Klaviyo rejects the missing header,
the fix is a *generic* reviewed capability, not an adapter: add an optional
`identity.headers` map to the manifest/generator and have `fetchUserInfo`
send it (per the skill: grow the standard_oauth capability set rather than
fork `service/adapter_*.go`). Record the outcome here before batch end.

Other Helio-side artifacts (batch-end surfaces, per master plan §2):
- Config: `integration.providers.klaviyo.oauth.client_id/client_secret` in
  `config/` + the Helm Secret in `deploy/` — landed by lane 1 (id and secret
  together; dev values stay uncommitted in local `config/cloud.yaml`).
- Icon: `ui/helio-app/src/integrations/icons/klaviyo.svg` + manual
  `providerIcons.ts` registration.
- Docs: `agents/plugins/heliox/skills/tool/klaviyo.md` provider sub-doc;
  plugin bump + publish ride the batch-end merge.
- provider-gen: run locally for validation only; the five projections are
  **not** committed on this branch.

## 5. Test plan (five layers)

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes for every command group — request shape, both auth schemes (Bearer vs `pk_` → `Klaviyo-API-Key`), `revision` header on every request, campaign required-channel filter, subscription-job body, pagination mapping, JSON:API error → exit 1 + `--json` envelope, usage → exit 2 | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<cred> anycli klaviyo -- account get`, then one command per group against the live API (`profile list`, `list list`, `campaign list --channel email`, `metric list`, `report campaign …`). Runnable early with a test account's private key (`pk_…`) thanks to the dual-scheme header; re-run at least `account get` with a real OAuth bearer once the dev app exists | **yes** — test-account private API key (lane 2), later a dev-app OAuth token |
| L3 | local-only `provider-gen` + `provider-gen --check` against this bundle; `helio-cli` built with an uncommitted `go.mod` `replace github.com/heliohq/anycli => /Users/wenfeng/workspace/helio/anycli/.claude/worktrees/tool-klaviyo`; both repos' unit suites. Branch CI is expected red on `provider-gen --check` until batch end — do not commit regens | none |
| L4 | singleton (`env: dev`), seed via `POST /internal/test-only/connections/seed` with real `access_token` **and** `refresh_token` minted from the lane-1 dev app and a deliberately short `expires_at` (forces the A3 refresh-and-write-back path — Klaviyo's ~1 h tokens make refresh load-bearing); then `heliox tool klaviyo -- account get` etc. through the real token gateway. Also the §4 identity-header probe | **yes** — lane-1 dev app client id/secret (uncommitted local `config/cloud.yaml`) + tokens minted from it |
| L5 | human-in-the-loop (lane 3): `heliox tool klaviyo auth` → connect link → real Klaviyo consent (owner/admin role required to install) → `oauth_connected` event on the channel → one unseeded live run. Gated on lane 1's committed config append (`config/` + `deploy/` together) | **yes** — real Klaviyo test account + human consent session |

Definition of done follows master plan §2: L1–L4 green on-branch, batch-end
merge (registration, pin bump, canonical regen, icon, docs), L5 sweep, then
the visible flip as the single go-live change.
