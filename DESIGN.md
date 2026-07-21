# Tool design: Acuity Scheduling (`acuity`)

Scratch per-tool design for the 300-integrations rollout (master plan row 200,
Wave 3, category Scheduling & eSign). Branches: anycli `tool/acuity`, Helio
`tool/acuity`. This file is committed on the branch and stripped by the batch
lead at batch end.

## 1. Naming (master plan §3)

| Axis | Value | Notes |
|---|---|---|
| ① CLI command word | `acuity` | flat command, no `tool.group` (no family) |
| ② anycli tool id | `acuity` | `definitions/tools/acuity.json`, Go pkg `internal/tools/acuity/` |
| ③ provider catalog key | `acuity` | `integrations/providers/acuity/provider.yaml` |

② == ③, so **no** `toolToProvider` entry in
`helio-cli/internal/toolcred/resolver.go` — identity mapping applies.

## 2. Auth-lane verification against official docs

Catalog and oauth-audit say `oauth_light`. Verified 2026-07-21 against the
official docs (https://developers.acuityscheduling.com/docs/oauth2 and
/reference/quick-start): **confirmed, with token-semantics nuances** the
generic catalog row does not capture.

- **Registration**: self-serve OAuth2 client-account form at
  `https://acuityscheduling.com/oauth2/register`. No review/approval program
  is documented → `oauth_light` is correct. Lane 1 registers one dev app
  (redirect URI = integration-service callback) and distributes
  client_id/secret as uncommitted local `config/cloud.yaml` entries.
- **Flow**: standard authorization-code.
  - Authorize: `https://acuityscheduling.com/oauth2/authorize`
    (`response_type=code&scope=api-v1&client_id=…&redirect_uri=…`).
  - Token: `POST https://acuityscheduling.com/oauth2/token`,
    `application/x-www-form-urlencoded` with `grant_type=authorization_code`,
    `code`, `redirect_uri`, `client_id`, `client_secret` → maps to
    `token_exchange_style: form_secret`. PKCE is not documented → `pkce: none`.
- **Scope**: exactly one scope exists, `api-v1` (full API access). No
  granular scopes.
- **Token semantics (divergence-worthy nuance)**: the token response is
  `{"access_token": …, "token_type": "Bearer"}` — **no `expires_in`, no
  `refresh_token`, no refresh grant documented**. Acuity access tokens are
  effectively non-expiring. Consequences:
  - bundle `refresh_lease: none`;
  - L4 seeds `access_token` only (Slack-bot-token class in the skill's
    `references/integration-testing.md` "picking a token per provider class");
    there is no refresh path to exercise — the recommended short-`expires_at`
    refresh drill does not apply and must not be faked.
- **Revocation (capability gap — see §5.2)**: the official contract is
  `POST https://acuityscheduling.com/oauth2/disconnect` with form fields
  literally named `access_token`, `client_id`, `client_secret` (official
  OAuth2 doc's curl example: `-d access_token=… -d client_id=… -d
  client_secret=…`). This does **not** fit the declarative revoker as-is:
  `declarativeOAuthRevoker.Revoke`
  (`go-services/integration-service/service/revoke.go`) always sends the
  token in an RFC-7009-style form field named `token`
  (`form := url.Values{"token": {token}}`); the bundle's `revoke.token:
  access_token` is a stored-token *selector* (validate.go oneOf
  `access_token|refresh_token`), not the wire field name. As the wire
  contract stands, the declarative revoker would send `token=…` and per the
  official docs Acuity would not revoke. Resolution path in §5.2.
- **API auth**: `Authorization: Bearer <access_token>` against
  `https://acuityscheduling.com/api/v1/…`. (Basic auth with user-id/API-key
  also exists but is the single-account path; the shipped integration is
  OAuth-only — do not add a second credential shape.)
- **Rate limits / pagination**: not documented. `GET /appointments` caps via
  `max` (default 100) + `minDate`/`maxDate` windows; there is no offset or
  cursor. The CLI exposes `--max` and the date window instead of inventing
  pagination. Re-check empirically at L2.

**Divergence from catalog/audit: none** on the auth lane itself — the audit
verdict (high confidence, single scope `api-v1`, self-serve registration)
matches the official docs exactly. The §5.2 revoke-field-name mismatch is a
Helio-internal capability gap (declarative revoker dialect vs Acuity's
non-RFC-7009 disconnect), not a lane divergence.

## 3. What an AI teammate does with Acuity → wrapped API surface

An assistant works for a business that takes client bookings on Acuity. The
real jobs: "what's on the calendar today/this week", "find/reschedule/cancel
Jane's appointment", "book Jane for a consult Thursday", "what slots are open
before I propose times", "block off Friday afternoon", "look up a client",
plus the lookups those need (appointment types, calendars, intake-form field
ids). API v1 surface wrapped (base `https://acuityscheduling.com/api/v1`):

| Endpoint | Why |
|---|---|
| `GET /appointments`, `GET /appointments/:id` | read the schedule; filters minDate/maxDate/calendarID/appointmentTypeID/email/firstName/lastName/canceled/max/direction/excludeForms |
| `POST /appointments` | book (client-validated by default; `admin=true` bypasses availability checks and allows `notes`; `noEmail=true` suppresses notifications) |
| `PUT /appointments/:id` | edit the allowed field set (names, email, phone, notes, labels, intake fields — exact allowed set verified at implementation from the reference page) |
| `PUT /appointments/:id/cancel`, `PUT /appointments/:id/reschedule` | cancel / move; reschedule takes `datetime` (+ optional `calendarID`) |
| `GET /availability/dates`, `GET /availability/times` | find open slots before proposing/booking (`month`/`date` + `appointmentTypeID`, optional `calendarID`, `timezone`) |
| `GET /appointment-types` | resolve type names → ids and durations |
| `GET /calendars` | resolve calendar names → ids |
| `GET /clients`, `POST /clients`, `PUT /clients`, `DELETE /clients` | client lookup/CRUD |
| `GET /forms` | intake-form field ids needed for booking `fields` |
| `GET /labels` | labels usable on appointment update |
| `GET /blocks`, `POST /blocks`, `DELETE /blocks/:id` | block off time |
| `GET /me` | account identity (also the bundle's userinfo endpoint) |

Deliberately **out of v1**: products/orders/certificates/addons (commerce
tail), `GET /availability/classes` and class check-times (class businesses;
add on demand), `GET /meta`. Cheap to add later inside the same service.

## 4. anycli definition

**Stage-1 rubric: `service` type.** No official Acuity CLI exists at all, so
the `cli`-type conditions fail at the first test. Plain REST + Bearer header →
built-in service like 21 of 23 existing definitions.

`definitions/tools/acuity.json`:

```json
{
  "name": "acuity",
  "type": "service",
  "description": "Acuity Scheduling as a tool (appointments, availability, clients)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "ACUITY_ACCESS_TOKEN"}
      }
    ]
  }
}
```

**Package**: `internal/tools/acuity/` (id has no dashes; pkg name = id),
registered as `RegisterService("acuity", &acuity.Service{})` in
`internal/tools/register.go` — registration rides the batch-end merge; the
definition JSON and package merge freely mid-batch.

**Service shape** (bitly/notion precedent): `Service{BaseURL, HC, Out, Err}`;
`DefaultBaseURL = "https://acuityscheduling.com/api/v1"`; exit codes 0
success / 1 runtime-API failure (typed apiError) / 2 usage; `--json`
structured error envelope; success output is passthrough of the provider JSON
body + newline (Acuity returns JSON everywhere). Acuity errors are non-2xx
with `{"status_code", "message", "error"}` — surface all three; 401 maps to
the credential-rejected message.

**Cobra tree** (resource-grouped, non-interactive, flags only):

```
acuity appointment list   [--min-date --max-date --calendar-id --type-id
                           --email --first-name --last-name --canceled
                           --exclude-forms --max --direction]
acuity appointment get <id>
acuity appointment create --type-id --datetime --first-name --last-name
                          [--email --phone --timezone --calendar-id --notes
                           --field id=value ... --admin --no-email]
acuity appointment update <id>  [allowed-set flags]
acuity appointment reschedule <id> --datetime [--calendar-id --admin --no-email]
acuity appointment cancel <id> [--note --admin --no-email]
acuity availability dates --type-id --month   [--calendar-id --timezone]
acuity availability times --type-id --date    [--calendar-id --timezone]
acuity type list
acuity calendar list
acuity client list [--search]
acuity client create --first-name --last-name [--email --phone --notes]
acuity client update --first-name --last-name [...]   # PUT /clients keys on name
acuity client delete --first-name --last-name [--email]
acuity form list
acuity label list
acuity block list [--min-date --max-date --calendar-id --max]
acuity block create --start --end [--calendar-id --notes]
acuity block delete <id>
acuity me
```

`--datetime` is passed through verbatim (provider parses via strtotime in the
business/calendar timezone); the command help documents ISO-8601 as the safe
form. `--admin` maps to `?admin=true`, `--no-email` to `?noEmail=true`.

## 5. Credential fields and auth flow end-to-end

- One credential field: `access_token` (vault-stored user token, delegated to
  the assistant), injected as `ACUITY_ACCESS_TOKEN`.
- Helio flow: `heliox tool acuity auth` → connect intent under provider key
  `acuity` → integration-service `standard_oauth` runtime strategy performs
  authorize + `form_secret` code exchange → identity via userinfo `GET /me` →
  token stored; token gateway serves it non-expiring (no refresh lease).
  Disconnect: see §5.2 — the declarative revoker's wire contract does not
  match Acuity's documented disconnect field name as-is.
- No adapter (`service/adapter_*.go`) is needed — everything fits the
  declarative `standard_oauth` capability set, **except two generic gaps**
  (§5.1 numeric identity, §5.2 revoke field name).

### 5.1 Finding: numeric userinfo id vs declarative identity resolver

`GET /me` returns `{"id": 12345, "email": …, "name": …, "timezone": …,
"schedulingPage": …}` — `id` is a JSON **number**. Helio's
`go-services/integration-service/service/declarative_identity.go`
`jsonPointerString` only accepts values that type-assert to `string`, so
`identity.stable_key: /id` would resolve-then-drop the value today (verified
in code on the Helio worktree).

Preferred fix (per skill guidance "grow the generic capability, not an
adapter"): extend `jsonPointerString` to stringify JSON numbers
(`strconv.FormatFloat`-with-integer-normalization, unit-tested) — numeric
account ids are common across upcoming providers, this is not
Acuity-specific. With that landed: `stable_key: /id`,
`label_candidates: [/email, /name]`.

Fallback if the batch lead rejects touching integration-service in this
batch: `stable_key: /email` (string; slightly weaker — email is mutable) with
the same label candidates, and file the numeric-stringify change separately.
The `/me` field shape itself must be re-confirmed live at L2 (the official
reference page does not render the full schema; shape corroborated from
secondary sources + the official OAuth doc's `GET /api/v1/me` example).

### 5.2 Finding: revoke form field name — `access_token` vs RFC-7009 `token`

Verified in code and against official docs (2026-07-21):

- Acuity's documented disconnect contract requires the form field to be
  literally named `access_token` (plus `client_id`, `client_secret`); no
  RFC-7009 `token` field is documented.
- Helio's `declarativeOAuthRevoker.Revoke`
  (`go-services/integration-service/service/revoke.go`) unconditionally
  builds `form := url.Values{"token": {token}}`; the bundle's `revoke.token`
  key only selects *which stored token* is sent
  (`cmd/provider-gen/validate.go` restricts it to
  `access_token|refresh_token`). There is no knob for the wire field name.

So a `provider_revoke` bundle as originally drafted would POST `token=…` and,
per the official contract, Acuity would not revoke — and the L5 check
("`/me` with the old token starts 401ing") would fail.

**Resolution path** (decide with the batch lead before the bundle rides the
batch-end merge):

1. **Empirically test at L2** whether `/oauth2/disconnect` also accepts the
   RFC-7009 `token` field name. This is undocumented — do not assume; test
   with a throwaway token: POST `token=…&client_id=…&client_secret=…`, then
   confirm `/me` with that token 401s. If it revokes, the existing
   declarative revoker fits unchanged and the bundle keeps
   `disconnect_mode: provider_revoke` (record the empirical result in the
   provider sub-doc).
2. **If it does not**, either:
   - grow the generic capability with a reviewed enum — e.g. a
     `token_field: token|access_token` policy knob on `auth.oauth.revoke`
     (default `token`), threaded through validate.go, the revoker, and unit
     tests — same "grow the generic, not an adapter" rationale as the §5.1
     numeric-stringify fix; numeric ids and non-RFC-7009 revoke dialects are
     both likely to recur across upcoming providers; or
   - ship with `disconnect_mode: local_only` and **no** `revoke` block (the
     generator forbids a revoke block with `local_only` —
     validate.go: "standard_oauth with disconnect_mode local_only forbids
     auth.oauth.revoke"), matching the notion/bitly precedent for providers
     the declarative dialect can't reach, and document the divergence.

Until resolved, the §6 draft below carries the conservative `local_only`
shape; flipping to `provider_revoke` (+ revoke block) is a two-line bundle
change once path 1 or 2 lands.

## 6. Helio provider bundle plan (hidden-first)

`integrations/providers/acuity/provider.yaml` draft:

```yaml
schema: helio.provider/v1
key: acuity
go_name: Acuity

presentation:
  name: Acuity Scheduling
  description_key: acuity
  consent_domain: acuityscheduling.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <assigned by batch lead>

auth:
  type: oauth
  owner: assistant          # business scheduling account (slack/notion class),
                            # not a personal-identity account (linkedin/x class)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://acuityscheduling.com/oauth2/authorize
    token_url: https://acuityscheduling.com/oauth2/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [api-v1]
    single_active_token: false
    refresh_lease: none
    # No revoke block while disconnect_mode is local_only (generator forbids
    # the pair). If §5.2 path 1 or 2 resolves in favor of provider revoke,
    # add:
    #   revoke:
    #     url: https://acuityscheduling.com/oauth2/disconnect
    #     client_auth: form
    #     token: access_token        # stored-token selector, not wire field
    #     token_field: access_token  # only if the §5.2 capability knob lands
    # and flip disconnect_mode to provider_revoke.

identity:
  source: userinfo
  url: https://acuityscheduling.com/api/v1/me
  stable_key: /id           # requires numeric-stringify fix (§5.1); else /email
  label_candidates: [/email, /name]

connection:
  mode: isolated
  disconnect_mode: local_only   # conservative default pending §5.2 resolution
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
  name: acuity
  kind: oauth
```

Batch-end / shared-surface items (NOT merged mid-batch, per master plan §2):
`register.go` entry, anycli tag + `helio-cli/go.mod` pin bump, the single
`provider-gen` run (five projections — run locally for validation only, never
committed from this branch), icon
`ui/helio-app/src/integrations/icons/acuity.svg` + `providerIcons.ts` append,
provider sub-doc under `agents/plugins/heliox/skills/tool/` + plugin version
bump/publish. OAuth config appends (client id/secret in `config/` +
`deploy/` Helm Secret together, Config Sync rule) are lane-1-owned and must
land before L5. Local builds of helio-cli use an uncommitted `go.mod`
`replace github.com/heliohq/anycli => <anycli worktree>`.

## 7. Test plan (five layers)

| Layer | What runs for acuity | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes asserting request paths/queries/bodies for every subcommand, `Authorization: Bearer` injection, provider-JSON passthrough, error envelope (401 vs generic non-2xx), exit codes, `--field id=value` parsing, `--admin`/`--no-email` query mapping. TDD: tests first. | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real token> anycli acuity -- me`, then `type list`, `availability times`, an `appointment create --admin` + `cancel` round-trip on the test account. Also confirms `/me` field shape (§5.1), the §5.2 revoke-field-name probe (`/oauth2/disconnect` with `token=` on a throwaway token), and undocumented rate/pagination behavior. | **yes** — a real Acuity test account (lane 2) AND the lane-1 dev OAuth app: the only token shape the tool speaks is a Bearer OAuth token, so L2 needs one manual code→token exchange (curl) against the dev app to mint `ANYCLI_CRED_ACCESS_TOKEN`. |
| L3 | Local `go run ./cmd/provider-gen` + `--check` against the branch bundle (projections not committed; branch CI expected red on `--check` until batch-end regen); anycli + helio-cli + integration-service unit suites. If the §5.1 numeric-stringify and/or §5.2 `token_field` changes land, their unit tests ride integration-service's suite here. | none |
| L4 | Singleton, `POST /internal/test-only/connections/seed` with provider `acuity` and **`access_token` only** (non-expiring class — no `refresh_token`/`expires_at`, no refresh drill), then `heliox tool acuity -- appointment list` reaching the live API through the real token gateway. helio-cli built with the local `replace`. | **yes** — same real token as L2 (minted from the lane-1 dev app). |
| L5 | Human-in-the-loop (oauth lane 3), post-batch-merge, pre-flip: `heliox tool acuity auth` → connect link → real Acuity login/consent (`scope=api-v1`) → `oauth_connected` event → one unseeded live run; verify identity label (email/name) in the integrations UI. If the bundle ships `provider_revoke` (§5.2 resolved), verify disconnect actually revokes at the provider (`/me` with the old token starts 401ing); if it ships `local_only`, verify local disconnect only and record the documented divergence. | **yes** — lane-1 config append landed (client id/secret in integration-service config) + the lane-2 test account, human consent session. |

## 8. Open items / risks

1. **Numeric stable key** (§5.1) — decide fix-in-generic vs `/email` fallback
   with the batch lead before the bundle rides the batch-end merge.
2. **Revoke form field name** (§5.2) — the declarative revoker sends RFC-7009
   `token=`; Acuity documents `access_token=`. L2 probes whether the
   RFC-7009 name is also accepted; otherwise grow the generic
   (`token_field` enum) or ship `local_only` (notion/bitly precedent).
   Decision recorded with the batch lead before the batch-end merge; the §6
   draft carries `local_only` until then.
3. `/me` response schema and the `PUT /appointments/:id` allowed field set are
   verified live at L2/implementation (official reference pages do not render
   full schemas; machine index `llms.txt` omits them).
3. Non-expiring tokens mean a revoked-at-provider token is only discovered on
   a 401 at call time; the service's 401 error message should tell the
   assistant to reconnect (`heliox tool acuity auth`).
4. No documented rate limit — keep the harness runs polite; note any observed
   429 behavior at L2 in the provider sub-doc.
