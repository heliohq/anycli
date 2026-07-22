# Tool design: Close (CRM)

Scratch per-tool design for the `close` provider, produced per the
`helio-tool-provider` pipeline skill and the 298-integrations master plan
(`docs/design/008-300-integrations-rollout-plan.md`). This file lives on the
`tool/close` branch only; the batch lead strips it at batch end.

- Catalog row: **#60** — Product **Close**, anycli id **close**, provider key
  **close**, auth lane **oauth_review**, wave **2**, category **CRM**.
- OAuth audit verdict: row 62 — OAuth supported **yes**, recommended lane
  **oauth_review**, confidence **high**
  (`docs/design/008-300-integrations-rollout-plan/oauth-audit.md`).

## 1. Independent verification vs. catalog & audit

Verified against Close's official developer docs (fetched 2026-07-22):

- OAuth flow guide: `https://developer.close.com/api/overview/oauth-authentication.md`
- OAuth app creation: `https://developer.close.com/integrations/create-an-oauth-app.md`
- API key auth: `https://developer.close.com/api/overview/api-key-authentication.md`

**Divergence check: none.** The official docs agree with the catalog and the
audit on every load-bearing point:

- Close offers a **multi-tenant OAuth 2.0 authorization-code** flow — one
  registered app, and any Close organization can authorize it. This satisfies
  the audit rubric for an OAuth lane (not api_key).
- New OAuth apps default to **Private** ("only members of your own Close
  organization can authorize them"). Letting arbitrary Close customers
  authorize requires **requesting Close make the app public** — a human review
  gate. Per the rubric ("a human review, partner-program, verification, or
  publish gate before external accounts can authorize → `oauth_review`") this
  is squarely **oauth_review**, matching audit row 62. The dev-mode (private)
  app is fully usable inside our own org, so dev + L4 do not wait on review —
  only the visible flip does (master plan §2, hidden-first).

No `DESIGN.md` divergence to record: catalog auth lane, provider key, and
anycli id all hold as written.

## 2. Official API surface wrapped, and why

Close is a sales-team CRM. What an AI teammate actually does in Close —
triage inbound leads, look up a contact/company before a call, log activity
(notes/calls/emails), advance opportunities through pipeline stages, and manage
follow-up tasks — maps onto the Close REST API v1 at
`https://api.close.com/api/v1/`. The tool wraps exactly the object families
that back those jobs, and no more:

| Resource | Endpoints (base `/api/v1`) | Why an AI teammate needs it |
|---|---|---|
| Lead (company/account) | `GET/POST /lead/`, `GET/PUT/DELETE /lead/{id}/` | The central CRM object; create/inspect/update accounts. |
| Contact (person) | `GET/POST /contact/`, `GET/PUT/DELETE /contact/{id}/` | People on a lead — emails/phones for outreach. |
| Opportunity (deal) | `GET/POST /opportunity/`, `GET/PUT/DELETE /opportunity/{id}/` | Pipeline value/stage; advance or report on deals. |
| Activity | Read all types: `GET /activity/`, `GET /activity/{type}/{id}/` (type ∈ note\|call\|email\|sms\|meeting). Write v1: `POST /activity/note\|call\|email/`, `DELETE /activity/{type}/{id}/` (sms/meeting writes deferred, §3). | Log and read the interaction history that drives follow-ups. |
| Task | `GET/POST /task/`, `GET/PUT/DELETE /task/{id}/` | Follow-up reminders the assistant creates/completes. |
| Search | `POST /data/search/` (advanced query DSL); `GET /lead/?query=` (simple) | Find leads/contacts by structured or free-text query — the entry point for most agent workflows. |
| Me / User | `GET /me/`, `GET /user/`, `GET /user/{id}/` | Identity + org resolution (also the identity/verify endpoint, §4). |

Design bias (anycli AGENTS.md + built-in service conventions 003 §3): every
subcommand targets non-interactive `--json`-friendly output, uses cursor/skip
pagination flags Close exposes (`_limit`, `_skip`, and `has_more`/cursor on
search), and never prompts. Write verbs take flags or a `--body @file.json`
raw-JSON escape hatch for fields the flag set does not cover (custom fields via
`custom.<id>` keys, which vary per org).

Endpoints deliberately **out of scope** for v1: bulk actions, email
sequences/sending, reporting/export, webhooks, and org/settings
administration — none are core to the teammate loop above and each is a large
surface better added later if demand appears.

## 3. anycli definition

**Tool form — `service` type (stage-1 rubric).** There is no official Close
CLI to wrap, so the `cli`-type criteria fail outright. Implement a `service`
type against the REST API, matching 21 of 23 existing definitions and the
`internal/tools/notion/` reference shape.

`definitions/tools/close.json`:

```json
{
  "name": "close",
  "type": "service",
  "description": "Close CRM as a tool (OAuth or API key): leads, contacts, opportunities, activities, tasks, search",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "CLOSE_ACCESS_TOKEN"}
      }
    ]
  }
}
```

The service reads `CLOSE_ACCESS_TOKEN` and sends
`Authorization: Bearer <token>` on every request (verified: OAuth tokens use
`token_type: "Bearer"` and `curl … -H "Authorization: Bearer ACCESS_TOKEN"`).
The credential map carries only the resolved bearer token; anycli knows nothing
about OAuth/refresh (that is the Helio token gateway's job).

> Note on the two Close auth schemes: an API **key** authenticates as HTTP
> Basic (`key` as username, empty password), but an **OAuth access token**
> authenticates as `Bearer`. Because Helio drives Close through OAuth
> (oauth_review lane), the service uses the Bearer scheme exclusively. A
> single injected `access_token` field covers it; no Basic-auth path is built.

**Go package:** `internal/tools/close/` (id has no dashes/leading digit, so the
package name is just `close`), registered as
`RegisterService("close", &close.Service{})` in `internal/tools/register.go`.

**Command tree** (cobra, grouped by resource, mirroring notion's shape):

```
close lead        list | get | create | update | delete
close contact     list | get | create | update | delete
close opportunity list | get | create | update | delete
close activity    list | note-add | call-log | email-log | get | delete
                  # list/get/delete cover all activity types (note/call/email/
                  # sms/meeting); write verbs ship note/call/email only in v1 —
                  # sms-send and meeting-log writes are deferred to a later
                  # revision (§2 advertises the full read family).
close task        list | get | create | update | complete | delete
close search      -- --query '<smart-query>' [--type lead|contact]
close me
```

**JSON output shape (003 conventions).** Default output is pretty text; `--json`
emits the provider payload verbatim for `get`, and for `list`/`search` a
provider-neutral envelope:

```json
{ "data": [ … ], "has_more": true, "cursor": "…" }
```

Errors follow the notion exit-code contract: `0` success, `1` runtime/API
failure rendered as a typed `apiError` (Close returns
`{"error": "...", "field-errors": {…}}` on 4xx — mapped into the `--json`
error envelope), `2` usage/parse error. Unit tests point `BaseURL`/`HC` at an
`httptest.Server` and assert request shape, the injected `Bearer` header, and
both text and `--json` error rendering — never the live API.

## 4. Credentials & the exact auth flow

**Lane: oauth_review, standard authorization-code — verified.** All values
below are quoted from the official OAuth flow guide.

- **Authorize:** `https://app.close.com/oauth2/authorize/`
  params `client_id`, `response_type=code`, `redirect_uri` (HTTPS/TLS
  mandatory — plain `http://` rejected), `scope`, `state`.
- **Token exchange + refresh:** `https://api.close.com/oauth2/token/`
  POST `application/x-www-form-urlencoded`; `client_id` + `client_secret`
  passed **in the body** (not Basic). Exchange body:
  `grant_type=authorization_code`, `code`, `client_id`, `client_secret`.
- **Scopes:** fixed `all.full_access offline_access` (all OAuth apps get the
  same; `offline_access` is what makes `refresh_token` come back).
- **PKCE:** not documented / not supported → **off**.
- **Access token:** `expires_in: 3600` (1 hour) → short-lived; the token
  gateway must refresh, so L4 seeds a short `expires_at` to force the
  refresh-and-write-back path.
- **Refresh:** POST token endpoint with `grant_type=refresh_token`,
  `refresh_token`, `client_id`, `client_secret`. **Rotating** — a new refresh
  token is returned each time and the old one is revoked; the gateway must
  persist the rotated value.
- **Revoke (disconnect):** `https://api.close.com/oauth2/revoke/`
  POST form `client_id`, `client_secret`, `token`.
- **Identity / verify:** `GET https://api.close.com/api/v1/me/` returns
  `id`, `first_name`, `last_name`, `email`, and `organizations[]`
  (`{id, name}`) — used for the connection identity + account label.

### Helio provider bundle (`integrations/providers/close/provider.yaml`)

Fully standard OAuth shape → **`connection.runtime_strategy: standard_oauth`,
zero service adapter.** Every axis of the standard exchanger maps cleanly:

- `token_exchange_style: form_secret` — form-encoded body with client
  id/secret in the body (matches the verified request format).
- `refresh_lease: credential` (`OAuthLeaseCredential`) — **required** because
  Close rotates its refresh token on every refresh: the official OAuth guide
  states "the authorization server issues a new Refresh Token and the client
  must discard the old" and "revokes the old Refresh Token after issuing a new
  one." `RefreshLeaseScope` exists precisely to serialize refreshes across
  replicas (catalog.go comment: "defines how refreshes are serialized across
  replicas"); omitting it defaults to `none` (independent concurrent
  refreshes). Under the repo's mandatory "design for horizontal scale" rule,
  two runtime replicas hitting the 1h-expired token concurrently would both
  refresh, Close revokes the loser's now-stale refresh token, and with A3
  strict write-back (integration-service CLAUDE.md: a permanent connection
  brick for rotation-type providers) and no per-credential lease to force
  `reloadRefreshSnapshot`, the connection can be permanently bricked. The
  credential-scoped lease keys on the credential id (`refresh:<provider>:<credID>`,
  token_refresh.go) so concurrent refreshes serialize and the loser reloads the
  rotated snapshot instead of replaying a revoked token. This matches the
  same-program rotating providers keap (task #152) and signnow (#189), which
  both set `refresh_lease: credential`.
- `auth.oauth.pkce: false`.
- `auth.oauth.authorize_url: https://app.close.com/oauth2/authorize/`
- `auth.oauth.token_url: https://api.close.com/oauth2/token/`
- `auth.oauth.scopes: [all.full_access, offline_access]`
- `identity.source: userinfo` — separate `GET /api/v1/me/`; RFC-6901 JSON
  Pointer `stable_key` from `/id`, `label_candidates` `[/email,
  /organizations/0/name, /first_name]`.
- `connection.disconnect_mode: revoke` — declarative revoker against
  `https://api.close.com/oauth2/revoke/`, form body `client_id`,
  `client_secret`, `token`.
- `auth.required_config_fields: [oauth.client_id, oauth.client_secret]`.
- `presentation.visible: false` (hidden-first).

**Capability-growth check:** no new capability code is needed — form_secret
exchange, userinfo identity, declarative revoke, and the credential-scoped
refresh lease are all inside the existing closed capability set. But this is
**not** the pipedrive/hubspot shape on the refresh axis: those set
`refresh_lease: none` because their refresh tokens do **not** rotate. Close's
rotating refresh forces the `refresh_lease: credential` selection (above),
matching keap/signnow — an established, expected decision, not a gap. So the
work is a config selection on an existing field, not a generic-enum growth. If
bundle review still surfaces a genuine gap, grow the generic enum rather than
add a `close`-specific `service/adapter_*.go` (per provider-yaml.md guidance).
Do **not** reach for an adapter speculatively.

> L4 caveat: the L4 "seed a short `expires_at` to force the refresh path" check
> is single-threaded and exercises refresh + rotated write-back, but does **not**
> exercise the concurrent-refresh race that `refresh_lease: credential` guards.
> The lease is validated by the standard-exchanger contract test + the
> integration-service refresh-lease unit tests, not by L4/L5.

### Three naming axes (all identical → no divergence)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `close` | bundle `tool.command` (flat, ungrouped) |
| ② anycli tool id | `close` | `definitions/tools/close.json` |
| ③ provider catalog key | `close` | bundle dir `integrations/providers/close/` |

Because ② == ③, **no `toolToProvider` entry** is added to
`helio-cli/internal/toolcred/resolver.go` (identity holds; `ProviderFor`/
`ToolFor` return `close` unchanged). Not a grouped family → no `tool.group`, no
`toolGroups` edit.

### Config (Config Sync hard rule)

Lane-1 registers a Close OAuth app (dev/private mode is enough for dev + L4)
and lands `client_id` + `client_secret` **together** into integration-service
config — `config/` locally *and* the Helm Secret under `deploy/` in the same
change. A provider with **all** config fields absent renders
`configured: false` (safe hidden); a **partially** configured one fails
service startup — so id and secret never land apart, and both must precede the
provider's L5 run.

## 5. Test plan — five layers

| Layer | What runs for `close` | External creds needed? |
|---|---|---|
| **L1** | `go test ./...` in anycli: `internal/tools/close/` unit tests against an `httptest` fake — request shape per resource, injected `Bearer` header, pagination flags, `--json` success + error (`{"error":…, "field-errors":…}`) rendering, exit codes 0/1/2. | No — fakes only. |
| **L2** | `make build-harness` then `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli close -- me` and one `search`/`lead list`/`note-add` round trip against the **real** `api.close.com`. Mandatory before the pin bump. | **Yes** — a real Close account token (from the test-account pool). An OAuth access token or, for L2 convenience, a Close API key works since both hit the same API (but the shipped path is OAuth Bearer). |
| **L3** | `provider-gen` + `provider-gen --check` (from `go-services/integration-service`); anycli `go test ./...`; `helio-cli` build with a local `replace` → `go build ./... && go test ./cmd/heliox/cmds/tool/`. Expect `--check` to fail in CI on-branch until the batch-end regen (do not commit local regens). | No. |
| **L4** | Singleton (`env: dev`) + `POST /internal/test-only/connections/seed` with a real seeded assistant identity, `provider: close`, seed `access_token` + `refresh_token` + a short `expires_at` (forces the gateway refresh path, since Close tokens live 1h), then `heliox tool close -- me` returns live data through the token gateway. | **Yes** — a real Close OAuth access + refresh token pair from the dev app + test account. Depends on lane-1 dev-app creation. |
| **L5** | Once, hidden, before the visible flip: `heliox tool close auth` → complete Close consent on the dev/private app → assert `oauth_connected` system event → run one unseeded `heliox tool close -- me` through the new connection. | **Yes** — human-in-the-loop OAuth consent on a real Close account (oauth L5, master-plan lane 3). |

**Rollout:** land hidden; complete L1–L4 while hidden; run L5; then — and only
after Close approves the app for public use (oauth_review gate) — flip
`presentation.visible: true` + regenerate as the single go-live change.

## 6. Open items for the implementer

- Confirm search cursor semantics at L2: `POST /data/search/` returns a
  `cursor` for the next page vs. `GET /lead/?_skip=` offset paging — the
  `list`/`search` envelope should surface whichever the endpoint actually
  returns rather than assuming one.
- Custom fields are per-org (`custom.<field_id>` keys). Keep them out of the
  typed flag set; expose them only through the `--body @file.json` raw-JSON
  escape hatch on `create`/`update`.
- Re-confirm at stage 1 that fixed scope `all.full_access offline_access` is
  acceptable (Close grants no narrower scope self-serve); if least-privilege is
  later required it is a "contact Close" request, not a bundle change.
