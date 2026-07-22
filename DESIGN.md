# Cal.com — per-tool design (`heliox tool calcom`)

Scratch design for the `tool/calcom` batch branch. Row 42 of the master plan
catalog (`docs/design/008-300-integrations-rollout-plan.md`): Product **Cal.com**,
anycli id **`calcom`**, provider key **`calcom`**, auth lane **`oauth_review`**,
Wave **1**, category Scheduling & eSign. The batch lead strips this file at
batch end.

Pipeline: Helio `.claude/skills/helio-tool-provider/SKILL.md`. English only; TDD
per anycli AGENTS.md; conventional commits on `tool/calcom` only; do NOT commit
provider-gen projections.

---

## 0. Divergence check vs official docs (independent verification)

The prompt requires verifying the catalog/audit against Cal.com's **official**
API docs, not inheriting assumptions. Results:

- **Auth lane `oauth_review` is CONFIRMED.** The 2026-07-21 OAuth audit verdict
  for Cal.com (oauth-audit.md) reads: "API v2 offers an OAuth authorization-code
  flow for existing Cal.com users, but a newly created OAuth client sits in a
  'pending' state and a Cal.com admin must manually review and accept it before
  the auth flow works ('Client not approved' error otherwise); API keys remain
  the no-review fallback." Official Cal.com third-party OAuth is the newer API v2
  addition (tracked as CAL-5197 / calcom#19456 "OAuth support for third party
  apps to access api v2"). The manual admin-approval gate on a newly registered
  client is exactly the review-clearance gate `oauth_review` encodes. **No lane
  change; the catalog is correct.**

- **Two distinct "OAuth" surfaces exist in Cal.com — pick the right one.** This
  is the #1 trap and must be recorded:
  1. **Platform OAuth Clients** (`POST /v2/oauth-clients`, `x-cal-secret-key`,
     "managed users") — this is Cal.com's *Platform* embedding product where a
     paying platform customer provisions and owns managed end-users under their
     own client. It is **not** a multi-tenant authorization-code flow that
     arbitrary existing Cal.com users can consent to. **We do NOT use this.**
  2. **Third-party OAuth (authorization code)** — authorize at
     `https://app.cal.com/auth/oauth2/authorize`, token exchange at
     `POST https://api.cal.com/v2/auth/oauth2/token`. One registered client;
     any existing Cal.com user clicks Allow and consents to scopes. **This is
     the surface the audit graded and the one this tool integrates.** Several
     third-party integration guides reference stale hosts
     (`api.cal.com/oauth/token`, `app.cal.com/api/auth/oauth/token`) — those are
     wrong for the v2 flow; use the two hosts above.

- **Versioning model divergence CONFIRMED and CORRECTED.** An earlier draft
  modeled `cal-api-version` as a single global constant (`2024-08-13`) "baked
  in" and sent on every call, by analogy to Notion's `Notion-Version`. Checking
  each endpoint's official reference disproves that: Cal.com v2 pins the version
  **per endpoint** — `/v2/slots` requires `2024-09-04` (docs: "must be set to
  2024-09-04; if not set to this value, the endpoint will default to an older
  version"), `/v2/bookings` requires `2024-08-13`, `/v2/event-types` requires
  `2024-06-14`, `/v2/schedules` requires `2024-06-11`. A single `2024-08-13`
  constant would send the wrong version to `/slots` (a core "find available
  time" capability), silently downgrading its response semantics. The design now
  models the version per route (§1 table, §2, §6 L1). The Notion single-global
  analogy is dropped.

- **API-maturity RISK to record (not a lane change).** The v2 third-party OAuth
  is young: the token endpoint `POST /v2/auth/oauth2/token` has an open upstream
  report of returning `404` even from Cal.com's own docs playground
  (calcom#27686), and there is an open refresh-token defect (calcom#24016). This
  does not move the lane, but it makes **L2 (real-API harness) the go/no-go
  gate**: the dev-mode app's real token exchange and refresh must be exercised
  against the live endpoint before this tool leaves `code-complete (hidden)`. If
  the third-party OAuth flow is still unusable at dev time, escalate under
  master-plan §6 "API access regressions" (hold hidden; do not fall back to the
  api_key path without an explicit design amendment — Helio's connect model is
  OAuth-owner `assistant`, and silently swapping to a user-pasted API key would
  contradict the catalog lane and the connect UX). The api_key fallback the
  audit mentions is Cal.com's, not our sanctioned integration path.

---

## 0a. Stage-1 gate RESOLVED (implementation update, 2026-07-22)

Independent re-verification against Cal.com's official docs during implementation
**resolved the §4 stage-1 `/v2/me` version-header gate to the header-free branch —
§4a does NOT run, and there is ZERO integration-service Go change.** The resolution
is a divergence from this doc's original assumption (that the version-gated
`api.cal.com/v2/me` was the only identity option), so it is recorded here:

- **Identity uses `GET https://app.cal.com/api/auth/oauth/me`, not `api.cal.com/v2/me`.**
  Cal.com's official OAuth reference documents a purpose-built OAuth credential
  endpoint — `GET https://app.cal.com/api/auth/oauth/me` with only
  `Authorization: Bearer <token>` — for exactly the "who is this token" question
  identity resolution asks. It is **not** under the version-gated `/v2` surface, so
  it needs **no `cal-api-version` header**. This is the header-free branch: the plain
  `declarativeIdentityResolver` reaches it with only `Authorization` + `Accept` (the
  two headers `fetchUserInfo` already sends), so the §4a shared-contract header
  capability is unnecessary. Subtracting §4a is the orthogonal-minimal outcome:
  identity gets its own dedicated non-versioned endpoint instead of growing the
  shared identity contract to carry a Cal.com version pin.
- **Token response carries no identity, confirming userinfo is required.** The v2
  third-party token exchange returns only `access_token` + `refresh_token` (no `id`,
  no `expires_in`), so Notion-style `identity.source: token_response` is impossible;
  `source: userinfo` against `oauth/me` is the correct and only declarative option.
- **The anycli `me` command is separate and DOES send the version header.** The
  data-plane `calcom me` → `GET api.cal.com/v2/me` sends `cal-api-version: 2024-06-14`
  (anycli sends per-route versions natively). Only integration-service's connect-time
  identity fetch uses the non-versioned `oauth/me` endpoint. Clean separation; no
  code path needs a version header it cannot send.
- **All five per-endpoint versions re-confirmed against each endpoint's official
  reference:** event-types `2024-06-14`, slots `2024-09-04`, bookings `2024-08-13`,
  schedules `2024-06-11`, me `2024-06-14`. (Some third-party blogs claim event-types
  uses `2024-08-13`; the official `/v2/event-types` reference says `2024-06-14` — the
  blogs conflated versions. This doc's §1 table stands.)
- **Modern scopes are UPPERCASE, space-separated** (`BOOKING_READ BOOKING_WRITE
  EVENT_TYPE_READ SCHEDULE_READ PROFILE_READ`), passed via the bundle `scopes:` list
  which the standard authorize-URL builder space-joins into the `scope` param. The
  legacy lowercase `READ_PROFILE`/`READ_BOOKING` scopes are deprecated. `display_scopes`
  (lowercase UI labels) stays separate.

## 1. Official API surface wrapped — and why

**Base:** `https://api.cal.com/v2`. **Auth:** `Authorization: Bearer <access_token>`.
**Required per-request header:** `cal-api-version: <date>` — date-pinned, but
**PER-ENDPOINT, not a single global version.** This is the #1 correctness trap
and it is NOT like Notion's single `Notion-Version` constant. Cal.com v2 pins a
different date per endpoint family, and omitting the header (or sending the
wrong date) does **not** hard-error uniformly: the endpoint **silently defaults
to an older version** with altered response semantics/shape (docs: "must be set
to <date>; if not set to this value, the endpoint will default to an older
version"), and several v2 endpoints `404` outright when the header is absent.
So the version must be **modeled per command/route**, verified from each
endpoint's own official reference. **Response envelope:** v2 wraps payloads as
`{"status":"success","data": …}` (errors as `{"status":"error","error": …}`).

**Per-endpoint `cal-api-version` (confirmed against each endpoint's official
reference; re-confirm at stage 1):**

| Endpoint family | Required `cal-api-version` |
|---|---|
| `/v2/event-types`, `/v2/event-types/{id}` | `2024-06-14` |
| `/v2/slots` | `2024-09-04` |
| `/v2/bookings` (list/get/create/cancel/reschedule) | `2024-08-13` |
| `/v2/schedules` | `2024-06-11` |
| `/v2/me` | `2024-06-14` **only on the header-required branch** — the docs example omits it and v2 may default-to-older / 404 without it; whether calcom's userinfo GET sends it is the §4 stage-1 gate, and if it does it requires §4a capability growth (the anycli service already sends per-route versions, but the connect-time `/v2/me` fetch is integration-service's `declarativeIdentityResolver`, not anycli). |

An AI teammate uses Cal.com as a **scheduling actuator**: inspect what meeting
types the user offers, find open time, book/cancel/reschedule on the user's
behalf, and read the resulting bookings. That maps to a deliberately small,
high-value slice of v2 — not the whole platform/org/webhook surface:

| Verb group | Endpoint(s) | Version | Why an AI teammate needs it |
|---|---|---|---|
| `event-type list` | `GET /v2/event-types` | `2024-06-14` | Enumerate the user's bookable meeting types (id, slug, length) — the entry point; a booking needs an `eventTypeId`. |
| `event-type get` | `GET /v2/event-types/{id}` | `2024-06-14` | Inspect one type's config before booking. |
| `slot list` | `GET /v2/slots` (bounded `start`/`end` range, `eventTypeId`) | `2024-09-04` | Find available times to propose/confirm. Bounded range per Cal.com guidance. Wrong/missing version silently downgrades slot semantics. |
| `booking list` | `GET /v2/bookings` | `2024-08-13` | Review upcoming/past meetings ("what's on my calendar"). |
| `booking get` | `GET /v2/bookings/{bookingUid}` | `2024-08-13` | Detail one booking. |
| `booking create` | `POST /v2/bookings` (`eventTypeId`, `start`, `attendee{name,email,timeZone}`) | `2024-08-13` | Schedule a meeting — the core write. |
| `booking cancel` | `POST /v2/bookings/{bookingUid}/cancel` | `2024-08-13` | Cancel on request. |
| `booking reschedule` | `POST /v2/bookings/{bookingUid}/reschedule` | `2024-08-13` | Move a meeting. |
| `schedule list` | `GET /v2/schedules` | `2024-06-11` | Read availability schedules (working hours). |
| `me` | `GET /v2/me` | `2024-06-14` | Identity/profile; also underpins connect-time identity resolution (§3, §4). |

Explicitly **out of scope** for v1: `/v2/oauth-clients` (Platform managed-user
plane — wrong product, see §0), `/v2/webhooks`, `/v2/credits`, and org/team
admin endpoints. Bookings + event-types + slots + schedules + me is the
orthogonal minimum that lets the assistant actually schedule. The surface can
grow later behind the same bundle without re-review.

Time zones are explicit on every booking (`attendee.timeZone`); the service does
not guess — the caller passes it, and errors surface Cal.com's non-2xx JSON
body verbatim.

## 2. anycli definition

**Type: `service`** (stage-1 rubric). No official, non-interactive,
`--json`-capable Cal.com CLI binary exists to wrap; the v2 REST API is clean and
JSON-native. Matches 21/23 existing definitions. `cli` type is not justified.

- `definitions/tools/calcom.json` — `name: "calcom"`, `type: "service"`,
  one-line description, single credential binding: resolver field
  `access_token` → injected as env `CALCOM_TOKEN` (mirrors notion.json's
  `access_token`→`NOTION_TOKEN`).
- `internal/tools/calcom/` — Go package **`calcom`** (id has no dash, so the
  package name equals the id; §3 naming). Registered in
  `internal/tools/register.go` `init()` as `RegisterService("calcom", &calcom.Service{})`.
  Copy the shape of `internal/tools/notion/`: a cobra tree grouped by resource,
  a `Service{BaseURL, HC, Out, Err}` struct so httptest fakes can drive it, a
  **per-route `cal-api-version` map** (NOT one baked-in constant — a named
  constant per endpoint family: `2024-06-14` event-types/me, `2024-09-04` slots,
  `2024-08-13` bookings, `2024-06-11` schedules; each command sends its own), and
  the documented exit-code contract
  (**0** success; **1** runtime/API failure via typed `apiError` carrying
  Cal.com's `status:error` body; **2** usage/parse/enum errors) with a `--json`
  structured error envelope.

**Command tree** (axis-① word `calcom`; passthrough after `--`):

```
calcom event-type list
calcom event-type get   --id <id>
calcom slot list        --event-type-id <id> --start <ISO> --end <ISO>
calcom booking list     [--status upcoming|past|cancelled]
calcom booking get      --uid <bookingUid>
calcom booking create   --event-type-id <id> --start <ISO> \
                        --attendee-name <n> --attendee-email <e> --attendee-tz <tz> \
                        [--notes <s>] [--metadata <json>]
calcom booking cancel   --uid <bookingUid> [--reason <s>]
calcom booking reschedule --uid <bookingUid> --start <ISO> [--reason <s>]
calcom schedule list
calcom me
```

**JSON output shape.** Every subcommand supports `--json` and emits the
provider-neutral, agent-consumable projection: unwrap Cal.com's `{status,data}`
envelope and print `data` (an object for get/create, an array for list) to
stdout; on failure print the `{status:error,error:{...}}` body through the typed
`apiError` → `--json` error envelope on stderr, non-zero exit. No interactive
prompts (anycli AGENTS.md).

## 3. Naming axes (§3 master plan)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `calcom` | bundle `tool.command` (implicit; equals `tool.name`) |
| ② anycli tool id | `calcom` | `definitions/tools/calcom.json` |
| ③ provider catalog key | `calcom` | bundle dir `integrations/providers/calcom/` |

All three are the community-standard single token `calcom` (§3: "use the
community-standard single token where one exists (`calcom`, …)"). **②==③, so NO
`toolToProvider` entry** is added in `helio-cli/internal/toolcred/resolver.go`
— the identity default already resolves `calcom`→`calcom`. Not a grouped family
(no `tool.group`). Go package `calcom` (no leading digit, no dash).

## 4. Credential fields & the exact auth flow

**Lane: `oauth_review`. OAuth owner: `assistant`** (a shared scheduling identity
per assistant, like Notion; not `individual`). Multi-tenant authorization-code:
one Helio-registered Cal.com client that any existing Cal.com user consents to.

Flow, verified against official docs:
1. **Authorize** — redirect the user to
   `https://app.cal.com/auth/oauth2/authorize?client_id=…&redirect_uri=…&state=…&scope=BOOKING_READ BOOKING_WRITE EVENT_TYPE_READ SCHEDULE_READ PROFILE_READ`.
   User clicks Allow → redirected back with `code`.
2. **Token exchange** — `POST https://api.cal.com/v2/auth/oauth2/token` with
   `grant_type=authorization_code`, `code`, `client_id`, `client_secret`,
   `redirect_uri`. Accepts `application/x-www-form-urlencoded`; confidential
   client authenticates with `client_secret` → **`token_exchange_style:
   form_secret`**. Returns access + refresh tokens.
3. **Refresh** — standard refresh-token grant against the same token endpoint;
   **`refresh_lease: none`** (ordinary per-connection refresh, not X-style
   single-active-token / global lease). Seed a short `expires_at` at L4 to force
   the gateway's refresh-and-write-back path.
4. **Approval gate** — a newly registered client is `pending`; a Cal.com admin
   must accept it or the authorize step returns "Client not approved". This
   gates only the **visible flip** (review clearance), never dev / L4 / merge
   (hidden-first). Dev-mode credentials for L4 come from the registered (even if
   still-pending for *public* review) app per master-plan lane 1.

**Bundle `auth` block (standard_oauth):**
- `type: oauth`, `owner: assistant`,
  `required_config_fields: [oauth.client_id, oauth.client_secret]`.
- `oauth.authorize_url: https://app.cal.com/auth/oauth2/authorize`,
  `oauth.token_url: https://api.cal.com/v2/auth/oauth2/token`,
  `token_exchange_style: form_secret`, `pkce: none`,
  `display_scopes: [booking_read, booking_write, event_type_read, schedule_read, profile_read]`,
  `single_active_token: false`, `refresh_lease: none`.
- `connection: {mode: isolated, disconnect_mode: local_only,
  runtime_strategy: standard_oauth}` (no provider revoke endpoint documented for
  third-party tokens → `local_only`).
- `credential.fields: {access_token: token.access_token, account_key:
  connection.account_key}`.

**Identity resolution** (`identity` block): `source: userinfo`,
`url: https://api.cal.com/v2/me`, `stable_key: /data/id`,
`label_candidates: [/data/email, /data/username]` (RFC-6901 pointers into the v2
`data` envelope).

**The version header on `/v2/me` is NOT a declarative field today — it is a
stage-1 decision that may force shared-contract capability growth (§4a).**
Verified against integration-service `main`, not assumed:
- The declarative identity contract is exactly four fields —
  `ProviderIdentity{Source, URL, StableKey, LabelCandidates}`
  (`model/catalog.go`) / `identityManifest{source, url, stable_key,
  label_candidates}` (`cmd/provider-gen/manifest.go`). There is **no header
  field**, and `provider-gen` strict-decodes (`decoder.KnownFields(true)` in
  `manifest.go`), so a bundle that writes `header:` under `identity:` **fails
  the build** (`TestDecodeManifestRejectsUnknownField`).
- The userinfo fetch (`service/declarative_identity.go` → `fetchUserInfo` in
  `service/oauth_exchange.go`) sets only `Authorization: Bearer` and
  `Accept: application/json`. There is **no mechanism** to send
  `cal-api-version` on the userinfo GET.
- The `Header` field an earlier draft leaned on lives on `APIKeyPolicy` /
  `apiKeyManifest` — the `manual_api_token` verifier lane
  (`service/manual_token_verifier.go`), unrelated to `standard_oauth` userinfo.
  No shipped bundle sends any custom header on the OAuth userinfo GET.

So delivering `/v2/me` identity with a version header is **not free/declarative**;
it is real shared-contract capability growth (§4a). It is still **not** a compiled
`service/adapter_*.go` — this stays standard-shaped OAuth (provider-yaml.md: a new
standard OAuth provider should never need an adapter); the growth is a *generic*
capability on the shared identity contract, reusable by any future provider.

**Stage-1 gate (decides whether §4a runs at all).** At L2, empirically call
`GET https://api.cal.com/v2/me` with only `Authorization: Bearer <token>` (no
`cal-api-version`) against the dev account:
- **If it returns a usable `{status:success, data:{id,email,username}}`
  envelope** → keep the plain `declarativeIdentityResolver`: **no header, no §4a,
  zero integration-service change** — and drop every header claim from this doc.
- **If it `404`s or silently downgrades** → execute §4a and pin `/v2/me` to
  `2024-06-14` (confirm the exact value from the official `/v2/me` reference at
  stage 1). Cal.com's own docs and third-party integration guides state
  `cal-api-version` is required for v2 and that omitting it may `404` or default
  to an older version, so **this is the expected outcome** — but it is confirmed
  empirically at stage 1, not assumed here.

**Nothing secret in the bundle.** `client_id`/`client_secret` land only in
integration-service config — `config/` locally and the Helm Secret in `deploy/`
together (Config Sync hard rule), as lane-1 per-provider appends. All fields
absent ⇒ `configured: false` (Connect disabled), safe to ship hidden; partial
config fails startup, so id+secret land in the same change.

## 4a. integration-service capability growth (CONDITIONAL on the §4 stage-1 gate)

Run this section **only if** the stage-1 `/v2/me` check shows the version header
is required. It is a **generic, shared-contract** fixed-header capability on the
declarative identity path — reusable by any provider, not a Cal.com adapter. The
integration-service AGENTS.md mandates exactly this shape ("a response shape or
lifecycle outside that closed capability set needs a compiled generic capability
… never an unbounded YAML expression"), and it mirrors how sibling Wave-1 tools
carve out a capability task when one is genuinely needed (e.g. hubspot numeric
stable-key, salesforce `instance_url` metadata). Work items:

1. **Manifest + model field** — add a closed fixed-header field (e.g.
   `header_name` + `header_value`, non-secret constant — it is a version pin,
   never a credential) to `identityManifest` (`cmd/provider-gen/manifest.go`)
   and the matching field(s) to `model.ProviderIdentity` (`model/catalog.go`).
2. **Validation** — extend `validateDeclarativeIdentity`
   (`service/declarative_identity.go`): the header is legal only for
   `source: userinfo`; reject it on `token_response` / `strategy`.
3. **Fetch wiring** — thread the header into `fetchUserInfo`
   (`service/oauth_exchange.go`) and its caller `loadDeclarativeIdentity`,
   setting it on the userinfo GET alongside `Authorization` / `Accept`.
4. **Projection** — regenerate all five provider-gen outputs
   (`model/provider_catalog.gen.go`, `ui/helio-app/.../providerCatalog.gen.ts`,
   `helio-sdk/src/connectionProviders.gen.ts`,
   `helio-sdk/src/toolCatalogDefaults.gen.ts`,
   `helio-cli/.../tool/providers_gen.go`); the header value is runtime-contract
   data, so it lands in the Go catalog projection
   `model/provider_catalog.gen.go` (the UI/SDK/heliox projections regenerate
   unchanged for calcom). Then `provider-gen --check`.
5. **Tests** (§6 L1/L3): a generator unit test that the new field decodes and
   projects into the Go catalog, and that an unknown `identity:` field still
   fails `KnownFields(true)`; a `fetchUserInfo` / resolver unit test asserting
   the userinfo GET carries `cal-api-version: 2024-06-14`.

"Zero integration-service code" is therefore true only on the header-free
branch; §5 states both branches explicitly instead of claiming the bundle is
always code-free.

## 5. Helio provider bundle plan (hidden-first)

`integrations/providers/calcom/provider.yaml`, `schema: helio.provider/v1`,
`key: calcom`, `go_name: Calcom`, **`presentation.visible: false`** initially.
`presentation`: name "Cal.com", `description_key: calcom`,
`consent_domain: cal.com`, an `order` slotted with the other Scheduling tools.
`tool: {name: calcom, kind: oauth}`. `resources: {selection|discovery|
enforcement: none}`. Bundle uses `runtime_strategy: standard_oauth` → **no
provider-specific integration-service *adapter*** (the golden path composes the
exchanger, the `declarativeIdentityResolver`, and the no-op revoker). Whether
there is **any** integration-service Go change is decided by the §4 stage-1 gate:
- **Header-free branch** (stage-1 `/v2/me` works without `cal-api-version`):
  **zero integration-service code** — pure declarative bundle.
- **Header-required branch**: §4a adds a *generic, shared-contract* userinfo-header
  capability (manifest field + `ProviderIdentity` + `fetchUserInfo` wiring +
  generator projection), exercised by any provider — still **not** a Cal.com
  adapter. This is the only integration-service Go change, and it is net-new
  shared capability, not a declarative freebie.

Companion (batch-end, ride the single provider-gen run):
- UI icon `ui/helio-app/src/integrations/icons/calcom.svg` + hand-register in
  `ui/helio-app/src/integrations/providerIcons.ts` (never generated).
- i18n `description_key: calcom` label string.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` (what the verbs
  do, ISO-time + timezone discipline, the `booking create` attendee shape).
- Five provider-gen projections regenerated together **locally only** for
  validation — not committed on this branch (master-plan §2; batch lead owns the
  canonical regen + pin bump).

Rollout: deploy hidden → L1–L4 green → L5 one real connect → after admin
approval clears, flip `presentation.visible: true` + regenerate as the single
go-live change (SKILL.md stage 10).

## 6. Test plan — five layers

| Layer | What it proves for calcom | Needs external creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/calcom` unit tests against an `httptest` fake: request shape (path, `Authorization: Bearer`, and the **correct per-endpoint `cal-api-version`** — assert `slot list` sends `2024-09-04`, `booking *` send `2024-08-13`, `event-type *`/`me` send `2024-06-14`, `schedule list` sends `2024-06-11`; a single fixed value would be a bug), `{status,data}` unwrap, booking-create body assembly, `--json` vs plain error rendering, exit codes 0/1/2. Pin the failure mode: a route sending the wrong/absent version is a defect the fake must catch. Never hits the real API. | No |
| **L2** `anycli calcom -- <args>` harness, `ANYCLI_CRED_ACCESS_TOKEN=<tok>` | Real `api.cal.com/v2` calls: `me`, `event-type list`, `slot list`, `booking create`+`cancel`. **Go/no-go gate** (§0): also exercise the real `POST /v2/auth/oauth2/token` exchange + refresh from the dev app to confirm the third-party OAuth endpoint works (calcom#27686 / #24016). | **Yes** — a Cal.com test account access token (account pool, lane 2) + the dev-mode OAuth app (lane 1) |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle strict-decodes, closed field/enum contract, directory==key, HTTPS URLs; helio-cli builds against the local anycli `replace`; resolver identity (no divergence entry) holds. Run locally on-branch; expected to be red in CI until batch-end regen. **Conditional on §4a (header-required branch):** an integration-service generator unit test that the new userinfo-header field decodes + projects into `provider_catalog.gen.go` and that an unknown `identity:` field still fails `KnownFields(true)`; a `fetchUserInfo` / resolver unit test asserting the userinfo GET carries `cal-api-version: 2024-06-14`. On the header-free branch these tests do not exist (no capability added). | No |
| **L4** singleton + `POST /internal/test-only/connections/seed` + `heliox tool calcom -- me` | Seed `access_token`+`refresh_token` with a short `expires_at` for a real seeded assistant identity; the next call forces the token-gateway refresh-and-write-back path, then reaches live `/v2/me`. Proves gateway→anycli wiring end to end (bypasses authorize). | **Yes** — real Cal.com access+refresh tokens; dev OAuth app id/secret in local uncommitted `config/cloud.yaml` for the refresh path |
| **L5** `heliox tool calcom auth` → consent → run | One full authorize→callback→`oauth_connected` event→unseeded live `booking`/`me` run, once before the visible flip. Human-in-the-loop (oauth L5, lane 3): a real Cal.com consent on the **admin-approved** dev app. | **Yes** — real Cal.com account consent; requires the client's pending→approved gate cleared |

L2, L4, L5 depend on externally supplied credentials (test account + registered
dev app). L1 and L3 are fully self-contained. Per master-plan lane 1, dev-app
creation gates L4/L5; per the §0 maturity risk, L2's token-endpoint check is the
gate that decides whether calcom stays in its Wave-1 batch or slips.

## References
- Master plan: `docs/design/008-300-integrations-rollout-plan.md` (row 42; §2 execution, §3 naming).
- Audit verdict: `docs/design/008-300-integrations-rollout-plan/oauth-audit.md` (row 42, oauth_review, high confidence).
- Pipeline: Helio `.claude/skills/helio-tool-provider/SKILL.md` + `references/{anycli-development,provider-yaml,integration-testing}.md`.
- Precedent: anycli `internal/tools/notion/` (service shape, version header, exit-code contract); Helio `integrations/providers/notion/` — precedent **only** for the `owner: assistant` / `standard_oauth` / `refresh_lease: none` bundle shape. Notion uses `identity.source: token_response` (`stable_key: /workspace_id` read straight from the token response); it performs **no** userinfo GET and sends **no** custom header. So no shipped bundle exercises a fixed header on the OAuth userinfo GET — that path (§4a) is net-new shared capability, not established precedent (tie to the §4 stage-1 `/v2/me` check).
- Official Cal.com: API v2 introduction (`cal.com/docs/api-reference/v2/introduction`), OAuth (`cal.com/docs/api-reference/v2/oauth`), calcom#19456 / #27686 / #24016.
