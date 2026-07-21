# MailerLite — per-tool design (`tool/mailerlite`)

Scratch design for the `helio-tool-provider` pipeline. Catalog row 128:
product **MailerLite**, anycli id `mailerlite`, provider key `mailerlite`,
auth lane **api_key**, wave 2, category Marketing. This file is committed on
the `tool/mailerlite` branch and stripped by the batch lead at batch-end.

## 1. Verdict check against official docs

The master-plan catalog and the 2026-07-21 OAuth audit (row 130) both place
MailerLite in the **api_key** lane. I verified this against MailerLite's
official developer docs (developers.mailerlite.com) and it holds — **no
divergence to record**:

- The MailerLite (new / "Connect") API authenticates with a single
  account-issued **Bearer API token** (`Authorization: Bearer <token>`).
  Docs: <https://developers.mailerlite.com/docs/> (Getting started →
  Authentication).
- Tokens are generated **self-serve** in the dashboard (Integrations →
  MailerLite API → "Generate new token"), shown once, with **no scopes and
  no expiry** documented. They are permanently bound to the creating user.
- There is **no multi-tenant authorization-code OAuth** offering — no
  authorize/token endpoints, no registered-app model, no consent screen. So
  a shared Helio-owned client cannot mint per-account tokens; the user pastes
  their own key. That is exactly the audit rubric's "no viable multi-tenant
  path → api_key" case. Confirmed api_key.

MailerLite Classic (accounts created before 2022-03-22) is a **separate**
legacy API (`developers-classic.mailerlite.com`, base
`https://api.mailerlite.com/api/v2`, `X-MailerLite-ApiKey` header). We target
**only the current Connect API** (`https://connect.mailerlite.com/api`);
Classic is out of scope (shrinking cohort, different header/base — a second
provider at most, not this one).

## 2. API surface wrapped, and why

Base URL: `https://connect.mailerlite.com/api`. Every call sends
`Authorization: Bearer <token>`, `Content-Type: application/json`,
`Accept: application/json`. Listing is **cursor**-paged for subscribers
(`limit` default 25, `cursor`) and **page**-paged for campaigns/others
(`limit`, `page`); errors are `401 {"message":"Unauthenticated."}` for a bad
token, `403` when API access is disabled account-wide, `422` for validation.

Driven by what an AI teammate actually does with an ESP — grow/inspect the
list, look up and tag a contact, check and schedule a campaign, read
automation/form performance — the tool wraps these resources:

| Resource | Endpoints (methods) | Why an AI teammate needs it |
|---|---|---|
| **subscribers** | `GET /subscribers` (filter[status], limit, cursor, include=groups); `GET /subscribers/{id\|email}`; `POST /subscribers` (create/upsert, 201/200); `PUT /subscribers/{id}`; `DELETE /subscribers/{id}` (204); count = `GET /subscribers?limit=0`; `GET /subscribers/{id}/activity-log`; `POST /subscribers/{id}/forget` | Core CRM-of-email surface: add/upsert a lead, look someone up, fix a field, count the list, GDPR-forget. |
| **groups** | `GET /groups`; `POST /groups`; `PUT /groups/{id}`; `DELETE /groups/{id}`; `GET /groups/{id}/subscribers`; `POST /subscribers/{sub_id}/groups/{group_id}` (assign); `DELETE /subscribers/{sub_id}/groups/{group_id}` (unassign) | Segmentation by tag/group is how campaigns get targeted — assign/unassign is the everyday action. |
| **segments** | `GET /segments`; `GET /segments/{id}/subscribers` | Read dynamic segments to target/report (segments are rule-defined, read-only via API). |
| **fields** | `GET /fields`; `POST /fields`; `PUT /fields/{id}`; `DELETE /fields/{id}` | Custom fields must be discoverable before a subscriber write can set them. |
| **campaigns** | `GET /campaigns` (filter[status]=sent\|draft\|ready, filter[type], limit, page); `GET /campaigns/{id}`; `POST /campaigns`; `PUT /campaigns/{id}`; `POST /campaigns/{id}/schedule`; `POST /campaigns/{id}/cancel`; `DELETE /campaigns/{id}`; `GET /campaigns/{id}/reports/subscriber-activity` | Draft → schedule → check-report is the campaign loop; status filter answers "what's queued / what sent". |
| **forms** | `GET /forms/{type}` (popup\|embedded\|promotion); `GET /forms/{id}`; `PUT /forms/{id}`; `DELETE /forms/{id}`; `GET /forms/{id}/subscribers` | Signup-form inventory + who came in through which form. |
| **automations** | `GET /automations`; `GET /automations/{id}`; `GET /automations/{id}/activity` | Read-only: which flows exist and their run activity (no create-automation API). |
| **webhooks** | `GET /webhooks`; `POST /webhooks`; `PUT /webhooks/{id}`; `DELETE /webhooks/{id}` | Let the assistant register/inspect event callbacks. |

Deliberately **out of scope** for v1: the batch endpoint
(`POST /api/batch`) — it multiplexes the same resource calls and adds a
request-envelope surface an agent rarely needs one-shot; the CLI's per-verb
calls cover the same ground more legibly. Add later if a real workflow needs
bulk. `import` (`POST /subscribers/import`) is async/job-shaped and can wait
on demand.

## 3. anycli definition (stage 1 + 2)

**Tool form: `service` type.** No official MailerLite CLI exists; the surface
is a plain JSON REST API with Bearer auth. Fails the `cli`-type rubric
(no provisionable `--json` binary), so it is a `service`-type implementation
against the HTTP API — matching 21/23 shipped tools and every ESP sibling.

**Definition** `definitions/tools/mailerlite.json` (axis ②
`name: "mailerlite"`):

```json
{
  "name": "mailerlite",
  "type": "service",
  "description": "MailerLite email marketing as a tool (Connect API, Bearer token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_token"},
        "inject": {"type": "env", "env_var": "MAILERLITE_API_TOKEN"}
      }
    ]
  }
}
```

`source.field: api_token` is the resolver-supplied credential-map key; the
Helio bundle projects it (see §5 `credential.fields`). Env injection
(`MAILERLITE_API_TOKEN`) mirrors bitly/notion (`BITLY_ACCESS_TOKEN` /
`NOTION_TOKEN`) — the service reads it, sends `Authorization: Bearer $env`.

**Go package** `internal/tools/mailerlite/` (id has no dashes → package name
`mailerlite`), registered `RegisterService("mailerlite", &mailerlite.Service{})`
in `internal/tools/register.go`. Copy the **notion** service shape:
`BaseURL`/`HC`/`Out`/`Err` struct (so tests point `BaseURL` at an
`httptest.Server` and capture stdout/stderr), a cobra tree grouped by
resource, and the documented exit-code contract:

- **0** success; **1** runtime/API failure (typed `apiError` carrying HTTP
  status + MailerLite `message`/`errors`); **2** usage/parse error.
- `--json` on every command: success prints the provider payload (or a
  `{data,meta}` envelope for lists); errors print a structured
  `{"error":{...}}` envelope. Default (non-`--json`) prints a compact
  human line.

**Command tree (verbs):**

```
mailerlite subscriber list|get|create|update|delete|count|activity|forget
mailerlite group    list|create|update|delete|subscribers|assign|unassign
mailerlite segment  list|subscribers
mailerlite field    list|create|update|delete
mailerlite campaign list|get|create|update|schedule|cancel|delete|report
mailerlite form     list|get|update|delete|subscribers
mailerlite automation list|get|activity
mailerlite webhook  list|create|update|delete
```

Pagination flags mirror the API: `--limit`, `--cursor` (subscribers/groups),
`--page` (campaigns/forms/etc.), `--status` → `filter[status]`, `--type` →
`filter[type]`, `--include groups`. Write verbs take `--email`, `--fields`
(JSON), `--groups`, etc. Keep JSON output provider-neutral and stable
(design 003 §3 conventions) — no reshaping beyond the list envelope.

**L1 tests** (`*_test.go`, httptest fakes, no real API): assert request path
+ method + query for each verb, `Authorization: Bearer` header injection from
`MAILERLITE_API_TOKEN`, and both plain-text and `--json` rendering of a
`401 {"message":"Unauthenticated."}` and a `422` validation error.

## 4. Credential fields & auth flow

- **Credential kind:** single opaque secret — the MailerLite API token. One
  field, `api_token` (bundle field name; injected as `MAILERLITE_API_TOKEN`).
- **Registration model:** self-serve, dashboard-generated, no app
  registration, **no OAuth client** — so **lane 1 does nothing** for this
  tool (no client id/secret to create or land in integration-service config;
  `required_config_fields` is empty and the provider is `configured: true`
  with zero config, like other manual-token api_key providers).
- **Scopes / token semantics:** none. The token is all-or-nothing per
  account, non-expiring, permanently bound to its creating user. No refresh
  cycle → the token gateway serves it directly (seed `access_token` only in
  L4; omit `refresh_token`/`expires_at`).
- **Entry path:** the user pastes the token through the write-only
  `POST /connections/credentials` connect UI; it is stored in Vault and never
  touches the bundle (per `references/provider-yaml.md`).

**Verification / identity — the one design decision.** MailerLite exposes
**no `/me` / account endpoint**: there is no API way to read an account id or
name, so no natural `stable_key` exists. This is the **semrush / moz /
fullstory** situation, not the mongodb one — unlike a MongoDB DSN (which
can't be checked without connecting), a MailerLite token **can** be validated
with a cheap authenticated GET. Design:

- **Verify** the pasted token at connect time with
  `GET /subscribers?limit=0` (the documented count call — returns `200` +
  `{"total":N}` on a valid token, `401 {"message":"Unauthenticated."}` on a
  bad one, `403` when API access is disabled). This is the bundle's HTTPS
  identity/verification endpoint, sending the fixed `Authorization: Bearer`
  header — reusing the integration-service **api_key verifier capability**
  already shipped for semrush/moz/fullstory (verify-only: `200` ⇒ store,
  non-`200` ⇒ reject at connect with real feedback, no silent store).
- **account_key / label:** since the API yields no account identifier, use a
  **static** provider-stable key (e.g. `mailerlite`) with a human-readable
  label — `isolated` connection mode means one MailerLite connection per
  assistant, so a static key is unambiguous. (Fallback if the shipped
  verifier capability cannot assign a static key: `runtime_strategy:
  manual_credentials` + `identity.source: strategy` like mongodb, i.e.
  no-verify store. Preferred is verify-on-connect — MailerLite gives us a
  real check, so we should use it rather than defer a bad key to first use.)

No adapter Go is needed on either side beyond the generic verifier: MailerLite
is a plain Bearer key with a standard verify GET.

## 5. Helio provider bundle plan

Directory `integrations/providers/mailerlite/provider.yaml` (axis ③
`key: mailerlite`). **Axes ①/②/③ are all `mailerlite`** — no `tool.command`
group, no `tool.group`, and **no `toolToProvider` entry** (id == key; the
resolver's identity default applies). Register nothing in
`resolver.go`. Hidden-first.

Sketch (final field names pinned to the shipped verifier-capability contract
at build time; shape follows the api_key manual-token precedents):

```yaml
schema: helio.provider/v1
key: mailerlite
go_name: MailerLite

presentation:
  name: MailerLite
  description_key: mailerlite
  consent_domain: mailerlite.com
  visible: false            # hidden-first; flip is the single go-live change

auth:
  type: credentials         # api_key / manual-token (wire auth_type routes the drawer)
  owner: individual
  credential_input:
    fields:
      - name: api_token
        label_key: mailerlite_api_token
        secret: true
        required: true
        placeholder: "eyJ0eXAiOi…"     # MailerLite tokens are long opaque strings
    setup_url: https://www.mailerlite.com/help/where-to-find-the-mailerlite-api-key-groupid-and-documentation

identity:
  source: verifier          # verify GET, no natural account id (semrush/moz/fullstory pattern)
  verify_url: https://connect.mailerlite.com/api/subscribers?limit=0
  # static provider-stable account key + human label (no API-derived id)

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_token: token.access_token       # single secret via existing UpsertUserToken path
    account_key: connection.account_key

tool:
  name: mailerlite
  kind: api-key             # wire-compat value; clients route the drawer by auth_type
```

- `required_config_fields` is **empty** → `configured: true` with zero
  environment config; **nothing lands in `config/` or `deploy/`** for this
  provider (no client id/secret). This removes the §2 seventh-shared-surface
  (oauth config) work entirely.
- Exact `identity`/verifier field names must match the
  semrush/moz/fullstory bundles already on `main` — verify against one of
  those at implementation time and copy its verifier stanza verbatim rather
  than inventing keys (strict-decode fails on unknown fields).
- **UI icon** (not generated): `ui/helio-app/src/integrations/icons/mailerlite.svg`
  + hand-register in `providerIcons.ts`; add `tools.desc.mailerlite` /
  `mailerlite_api_token` i18n strings across locales.

## 6. Test plan → five layers

| Layer | For MailerLite | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — httptest fakes per verb: path/method/query, `Authorization: Bearer` from `MAILERLITE_API_TOKEN`, `--json` vs plain output, `401`/`422` error envelopes. | No |
| **L2** harness real-API | `ANYCLI_CRED_API_TOKEN=<real> anycli mailerlite -- subscriber count` and a `campaign list --status sent` against `connect.mailerlite.com` — proves field name, header injection, and request shape match the live API. **Mandatory before pin bump.** | **Yes** — one real MailerLite token (free tier works; account pool lane 2). |
| **L3** projections + suites | From `go-services/integration-service`: `provider-gen` then `provider-gen --check`; then `helio-cli` (with local `replace` → this anycli branch) `go build ./...` + `go test ./cmd/heliox/cmds/tool/`, and integration-service unit suite (verifier capability already covered by semrush/moz tests; add a mailerlite bundle-load assertion if the suite enumerates bundles). | No |
| **L4** singleton + seed | `make run-singleton`; `POST /internal/test-only/connections/seed` with `provider:"mailerlite"`, a **real** token as `access_token` (no refresh_token/expires_at — non-expiring key), real org/assistant identities; then `heliox tool mailerlite -- subscriber count` returns live data through the token gateway. | **Yes** — same real token as L2. |
| **L5** connect flow (api_key path) | Pre-flip, hidden: open the connect link → paste the token in the real connect UI (`POST /connections/credentials`) → verifier hits `GET /subscribers?limit=0` → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool mailerlite -- subscriber list` through the real token gateway succeeds. Agent-drivable (agent-browser) per master-plan §2 api_key L5; human fallback on UI breakage. | **Yes** — real token pasted through the UI (account pool). |

External-credential layers: **L2, L4, L5** (all need one real MailerLite API
token from the account pool). L1 and L3 are hermetic. No OAuth app, no lane-1
client credentials, no consent session — the api_key lane keeps this tool
fully agent-automatable through L5 with a single pasted key.

## 7. Rollout

Ship hidden (`visible: false`) with the batch; anycli code + definition merge
freely mid-batch, bundle + regen + pin bump ride the batch-end merge. After
L1–L5 pass on the seeded/hidden tool, flip `presentation.visible: true` +
regenerate as the single go-live change. No review clearance gate (api_key),
so the visible flip is gated only on its own L5 sweep.
