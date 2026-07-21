# Help Scout tool design (`help-scout` / `help_scout`)

**Status:** scratch per-tool design for branch `tool/help-scout`; the batch lead strips this file at batch-end.
**Catalog row:** #22 · Help Scout · anycli id `help-scout` · provider key `help_scout` · lane `oauth_light` · wave 1 · Support.
**Audit note:** Help Scout has no row in `docs/design/008-300-integrations-rollout-plan/oauth-audit.md` — that audit's scope was the 250 tools sitting in the `api_key` lane pre-audit, and Help Scout was already `oauth_light` in the seed catalog. The lane was therefore re-verified directly against official docs for this design (§4): confirmed `oauth_light` — self-serve app creation ("Your Profile > My Apps > Create My App"), standard authorization-code flow, no review gate before arbitrary Help Scout accounts can authorize.

## 1. Naming (master plan §3)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `help-scout` (flat command, no group) | no `tool.command` needed — flat providers render under their anycli id |
| ② anycli tool id | `help-scout` | `definitions/tools/help-scout.json` |
| ③ provider catalog key | `help_scout` | `integrations/providers/help_scout/` |

- Go package: `internal/tools/helpscout/` (dashes dropped, per the `microsoft-calendar` → `microsoftcalendar` precedent).
- ②↔③ divergence: mechanical dash↔underscore pair. One entry `"help-scout": "help_scout"` in `helio-cli/internal/toolcred/resolver.go` `toolToProvider` (batch-end shared surface; never mapped at call sites). This is one of the 24 entries the master plan §3 already budgets.

## 2. Which official API, and why

**API:** Help Scout **Mailbox API 2.0** (docs now brand it "Inbox API"), base `https://api.helpscout.net/v2`, JSON + HAL (`_embedded` arrays, `page` object, `_links`), OAuth2 bearer auth. Docs: https://developer.helpscout.com/mailbox-api/.

**Out of scope (v1), with reasons:**
- **Docs API** (knowledge base) — separate API with its own API-key auth, not covered by the Mailbox OAuth token. A future `help_scout_docs`-style extension is a separate decision; do not conflate auth planes.
- **Webhooks** — Helio has no provider-webhook ingest surface for tools; presence-over-polling is served by `conversation list --modified-since` / `query`.
- **Reports** endpoints — plan-gated on the Help Scout side and analytics-shaped; add on demand.
- **Workflows / conversation delete / customer async delete** — destructive or admin-shaped; an AI teammate should not need them in v1.

**What an AI teammate actually does with Help Scout** (drives the surface):
1. Triage a queue: list/filter/search conversations (by inbox, status, tag, assignee, `modifiedSince`, Lucene-style `query`).
2. Read a conversation with its threads before answering.
3. Answer: reply to the customer, or leave an internal note for the human team; optionally set status/assignee in the same call (the reply endpoint supports `status`/`assignTo`).
4. Update conversation state: status (active/closed/pending/spam), assignee, subject; manage tags; snooze.
5. Look up / maintain customer records tied to conversations.
6. Draft consistently: read saved replies.
7. Orient: list inboxes + folders, users, tags.

## 3. anycli definition + service

### 3.1 Stage-1 rubric → `service` type

No official Help Scout CLI exists (only a PHP SDK). → `type: "service"`, implementation in `internal/tools/helpscout/`, registered as `RegisterService("help-scout", &helpscout.Service{})` in `internal/tools/register.go` (registration itself rides the batch-end merge; the package and definition JSON merge freely mid-batch).

### 3.2 `definitions/tools/help-scout.json`

```json
{
  "name": "help-scout",
  "type": "service",
  "description": "Help Scout shared-inbox support tool (conversations, replies, customers)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "HELPSCOUT_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single credential field `access_token`, matching the bundle's credential projection (§5). No `binary`/`source` (service type), no `before`/`after` rules.

### 3.3 Service shape (bitly/notion precedent)

`Service{BaseURL, HC, Out, Err}` struct so tests point at `httptest.Server`; `DefaultBaseURL = "https://api.helpscout.net/v2"`; `EnvAccessToken = "HELPSCOUT_ACCESS_TOKEN"`; auth header `Authorization: Bearer <token>`. Exit-code contract per the notion reference: 0 success, 1 runtime/API error (typed `apiError`, 401 classified as credential rejection), 2 usage/parse errors; `--json` structured error envelope. 429 responses surface `X-RateLimit-Retry-After` in the error message (no auto-retry — fail fast, the caller decides).

**Output:** provider-JSON passthrough on stdout (bitly precedent) — HAL envelopes (`_embedded`, `page`, `_links`) pass through untouched so pagination stays visible to the agent. Create endpoints answer `201` with an empty body + `Resource-Id` header; those commands emit a small JSON receipt `{"id": <Resource-Id>, "status": "created"}` so the agent always gets the new id on stdout.

### 3.4 Subcommand tree (v1)

```
help-scout
├── conversation
│   ├── list      --mailbox --folder --status --tag --assigned-to --modified-since
│   │             --number --query --sort-field --sort-order --page --embed-threads
│   ├── get       <id> [--embed-threads]
│   ├── create    --mailbox --subject --customer-email|--customer-id --type email|phone|chat
│   │             (--text for the initial thread) [--status active|closed|pending, default active]
│   │             [--assign-to] [--tags]
│   ├── update    <id> [--status] [--assign-to|--unassign] [--subject]      (JSON-Patch PATCH ops)
│   ├── tag       <id> --tags t1,t2            (PUT /conversations/{id}/tags — replaces the set)
│   ├── snooze    <id> --until <ISO8601> [--unsnooze-on-customer-reply=true]
│   │             (PUT /conversations/{id}/snooze — dedicated endpoint, NOT a PATCH path)
│   └── unsnooze  <id>                          (DELETE /conversations/{id}/snooze)
├── thread
│   ├── list      <conversation-id> [--page]
│   ├── reply     <conversation-id> --text ... [--customer-id] [--status] [--assign-to] [--draft] [--cc] [--bcc]
│   └── note      <conversation-id> --text ...
├── customer
│   ├── list      [--first-name --last-name --mailbox --modified-since --query --page]
│   ├── get       <id>
│   ├── create    --first-name --last-name --email [...]
│   └── update    <id> [fields]
├── inbox
│   ├── list
│   ├── get       <id>
│   └── folders   <id>
├── saved-reply
│   ├── list      --inbox <id>
│   └── get       --inbox <id> <reply-id>
├── tag  list     [--page]
└── user
    ├── list      [--email --mailbox --page]
    └── me
```

Notes:
- `thread reply` requires `customer` in the body: default it to the conversation's `primaryCustomer` (one extra GET) when `--customer-id` is omitted — the agent-friendly path; `--customer-id` overrides.
- `conversation create`: the API requires `status` (allowed values on create: `active`/`closed`/`pending` — narrower than update, which also accepts `spam` via `/status` replace). The CLI defaults `--status` to `active` when omitted and always sends the field; `--status` values are validated client-side against the create set.
- `conversation update` uses the API's JSON-Patch dialect; the full set of supported paths per official docs (fetched 2026-07-21) is `/subject`, `/primaryCustomer.id`, `/draft`, `/mailboxId`, `/status`, `/assignTo` — flags compile to `replace`/`remove` ops on `/status`, `/assignTo`, `/subject` so the agent never writes patch JSON. `/snoozedUntil` is **not** a PATCH path; snoozing has its own endpoint (next bullet).
- `conversation snooze` is the dedicated endpoint `PUT /v2/conversations/{id}/snooze` (204 empty on success). Body requires **both** fields: `snoozedUntil` (ISO 8601, must be in the future, not after year 2100) and `unsnoozeOnCustomerReply` (boolean). `--until` maps to `snoozedUntil`; `--unsnooze-on-customer-reply` defaults to `true` (the human-typical "wake me if the customer replies" mode) since the API gives the field no default. `conversation unsnooze` is `DELETE /v2/conversations/{id}/snooze` (204; conversation returns to its home folder and reactivates if needed). Both emit a small JSON receipt (`{"id": <id>, "status": "snoozed"|"unsnoozed"}`) on the empty-204 response, mirroring the 201 `Resource-Id` receipt convention in §3.3.
- `--query` passes Help Scout's Lucene-style search string through verbatim (documented in the AI-facing sub-doc with examples like `modifiedAt:[NOW-1HOUR TO *]`, `assigned:"Unassigned"`).
- Everything is non-interactive; all input from flags (AGENTS.md rule).

## 4. Auth: verified `oauth_light` facts (official docs)

Source: https://developer.helpscout.com/mailbox-api/overview/authentication/ (fetched 2026-07-21).

- **Registration:** self-serve, "My Apps" under the user profile; no review gate; redirect URL fixed at app creation. → `oauth_light` confirmed.
- **Flow:** authorization code. Authorize: `https://secure.helpscout.net/authentication/authorizeClientApplication` (documented params: `client_id`, optional `state`). Token: `https://api.helpscout.net/v2/oauth2/token`.
- **Exchange:** `grant_type=authorization_code` + `code` + `client_id` + `client_secret` in the request body (form/JSON/query all accepted; no HTTP Basic documented) → `token_exchange_style: form_secret`.
- **Scopes: none.** No scope parameter exists anywhere in the flow; access is all-or-nothing, bounded by the authorizing user's own Help Scout permissions. The bundle therefore declares `display_scopes` only (consent-page disclosure slugs, bitly/notion precedent), never wire `scopes`.
- **PKCE:** not documented → `pkce: none`.
- **Token lifetime:** access tokens expire in 172800 s (2 days), signaled by 401. **Refresh tokens rotate**: `grant_type=refresh_token` returns a new access+refresh pair each time. `standard_oauth`'s refresh path writes back the returned pair (the microsoft_* bundles are the shipped rotating-refresh precedent), and the runtime contract pins `standard_oauth` to `refresh_lease: none` / `single_active_token: false` (`go-services/integration-service/model/runtime_contract.go`) — acceptable here because rotation is per-credential (one connection's refresh does not invalidate other users' tokens under the same app). Docs also warn token length varies — no fixed-size storage assumptions (Vault storage is variable-length already).
- **Revocation:** no RFC-7009 revoke endpoint documented → `disconnect_mode: local_only` (microsoft_outlook precedent: local_only forbids a `revoke` block).
- **client_credentials** flow exists for own-account apps — irrelevant to Helio's multi-tenant model; not used.

### Divergences / risks to verify at L5 (recorded per the independent-judgment rule)

1. **Authorize URL extra params.** Helio's `oauth_start.go` always appends `redirect_uri`, `response_type=code`, and `state`; Help Scout documents only `client_id` + `state` (redirect is pinned at app registration). Expectation: extra params are ignored. If Help Scout ever rejects them, that is a generic-capability gap (an "omit redirect params" enum on the reviewed capability set), not a per-provider adapter. Verify on the first real authorize (L5, and earlier at L4 app-creation time with a manual browser hit).
2. **Token-exchange `redirect_uri`.** `oauth_exchange.go` sends `redirect_uri` in the exchange body; Help Scout does not require it. Docs say POST bodies accept form params generally — expected to be ignored. Verified implicitly by L5.
3. **Numeric identity id.** `GET /v2/users/me` returns `id` as a JSON **number**; `jsonPointerString` (declarative_identity.go) accepts only string values, so `/id` cannot be the stable key without an adapter. Use `/email` (string, present on the resource-owner payload) as `stable_key`. Trade-off, accepted for v1: a user changing their Help Scout email re-keys the connection (new `account_key` → shows as a new account). If this ever matters, the generic fix is number-to-string coercion in the declarative resolver (a reviewed capability), not a Help Scout adapter.

## 5. Helio provider bundle plan

`integrations/providers/help_scout/provider.yaml` (held to the batch-end merge with the single `provider-gen` run; do NOT commit locally regenerated projections):

```yaml
schema: helio.provider/v1
key: help_scout
go_name: HelpScout

presentation:
  name: Help Scout
  description_key: help_scout
  consent_domain: helpscout.net
  visible: false          # hidden-first; flip + regen is the single go-live change
  order: <assigned by batch lead>

auth:
  type: oauth
  owner: individual       # token acts as the authorizing Help Scout user
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://secure.helpscout.net/authentication/authorizeClientApplication
    token_url: https://api.helpscout.net/v2/oauth2/token
    token_exchange_style: form_secret
    pkce: none
    single_active_token: false
    refresh_lease: none
    # Help Scout has no wire-level scopes (all-or-nothing per authorizing
    # user). Display-only capability slugs, bitly/notion pattern:
    display_scopes: [read_conversations, send_replies, manage_customers]

identity:
  source: userinfo
  url: https://api.helpscout.net/v2/users/me
  stable_key: /email          # /id is a JSON number — see DESIGN §4 risk 3
  label_candidates: [/email]

connection:
  mode: isolated
  disconnect_mode: local_only # no documented revoke endpoint
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
  name: help-scout            # axis ②; flat command (no group)
  kind: oauth
```

- Zero Helio-side Go: `standard_oauth` golden path (exchanger form_secret, declarative userinfo identity, no-op revoker under local_only).
- Config: lane 1 registers the Help Scout app (redirect = Helio's OAuth callback) and lands `help_scout.oauth.client_id/client_secret` in integration-service config — `config/` and the Helm Secret under `deploy/` together (Config Sync rule); dev-mode values arrive as uncommitted local `config/cloud.yaml` entries for the on-branch L4 run.
- No `experiment` gating (GA-track wave-1 support tool; hidden-first is the rollout gate).
- Icon: `ui/helio-app/src/integrations/icons/help_scout.svg` + manual `providerIcons.ts` registration (batch-end shared surface).
- AI-facing doc: new sub-doc `agents/plugins/heliox/skills/tool/help-scout/` (or file per current layout) covering triage/reply/note/search examples and the `query=` syntax; plugin version bump + publish ride the batch-end merge.

## 6. Test plan (five layers)

| Layer | What runs for help-scout | External creds needed? |
|---|---|---|
| **L1** anycli unit | `go test ./internal/tools/helpscout/` + registry/definition tests. httptest fakes asserting: `Authorization: Bearer` injection; request paths/query/body for every subcommand (list filters, JSON-Patch ops for `conversation update`, snooze PUT body with both `snoozedUntil` + `unsnoozeOnCustomerReply`, unsnooze DELETE, create body always carrying `status` incl. the defaulted `active`, reply body incl. defaulted `primaryCustomer`); HAL passthrough; `Resource-Id` receipt on 201-empty-body creates; 401→credential-rejection classification; 429 rendering with retry-after; `--json` error envelope; exit codes 0/1/2. TDD: tests first. | No |
| **L2** dev harness vs real API | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli help-scout -- conversation list --mailbox <id>` etc. Token minted from the lane-1 dev app (a client_credentials token from the same app also works for L2 since it is host-side supplied — but prefer an authorization-code token so L2 exercises the exact token class production serves). Must round-trip real data: list → get → note → reply(draft) → tag → status update. **Gate: real Help Scout test account (lane 2) + registered dev app (lane 1).** | Yes — Help Scout test workspace + dev app token |
| **L3** generation + suites | Local (uncommitted) `go run ./cmd/provider-gen` + `--check` against the branch bundle; helio-cli build/tests with a local uncommitted `go.mod` `replace github.com/heliohq/anycli => <this worktree>`; integration-service unit suite (bundle validation: runtime contract pins, HTTPS URLs, directory-key equality). Branch CI is expected red on `provider-gen --check` until batch-end — do not commit regens. | No |
| **L4** singleton + seed | `make run-singleton` (dev), seed via `POST /internal/test-only/connections/seed` with `provider: "help_scout"`, real `access_token` + `refresh_token` from the dev app and a short `expires_at` so the very next call exercises the refresh-and-write-back path against the rotating refresh pair; then `heliox tool help-scout -- conversation list` must return live data through the real token gateway. Requires lane-1 dev client id/secret in local uncommitted `config/cloud.yaml`. | Yes — dev app client id/secret + a real token pair |
| **L5** full connect flow | Human-in-the-loop (oauth lane): `heliox tool help-scout auth` → connect link → real Help Scout consent → `oauth_connected` event → one unseeded live command. Also confirms §4 risks 1–3 (extra authorize params tolerated; identity extraction lands on `/email`). Runs in the per-batch L5 sweep, gating the visible flip. | Yes — human + real Help Scout account |

Definition of done tracks the master plan: L1–L5 green, docs published, icon registered, then `visible: true` + regen as the single go-live change.
