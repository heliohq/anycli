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

**One divergence to record — lane label vs. buildable mechanism.** The catalog
row and audit bucket MailerLite in the human-facing **api_key** lane. That lane
label is correct, but on `main` the `manual_api_token` runtime strategy (the
literal "api_key" path) is not buildable for MailerLite: `ValidateRuntimeContract`
(`runtime_contract.go`) requires `AuthAPIKey` bundles to declare
`identity.source: userinfo` — a JSON identity endpoint the verifier GETs. The
MailerLite Connect API has **no `/me`, account, or user-info endpoint** (verified
against developers.mailerlite.com and the official CLI's resource groups — its
sections are Subscribers, Groups, Segments, Fields, Automations, Campaigns, Forms,
Batching, Webhooks, Timezones, Campaign languages, **and an E-commerce API**
(Shops, Products, Categories, Customers, Orders, Carts, Cart Items, Bulk Import);
none returns a stable **account** identity — the nearest thing, a shop id, is a
user-created resource id, not an account identifier, so it cannot key the
connection). With no userinfo endpoint,
the buildable mechanism is the **design-317 `credentials` / `manual_credentials`**
path — exactly what semrush and moz (also opaque keys with no userinfo endpoint)
use. So the bundle sets `auth.type: credentials` even though the lane label is
"api_key"; the wire tool kind stays `api-key`. This is not a lane change, it is
the api_key lane's opaque-key implementation.

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
on demand. The **E-commerce API** (Shops, Products, Categories, Customers,
Orders, Carts, Cart Items, Bulk Import) **exists** but is a deliberate v1
scope cut, not an omission: shop/customer/order sync is a distinct
storefront-integration workflow with its own `--shop`-scoped surface, orthogonal
to the ESP list/campaign core an AI teammate drives day-to-day. Defer to a v2
scope decision once a real e-commerce workflow demands it.

## 3. anycli definition (stage 1 + 2)

**Tool form: `service` type — chosen over a real cli-type alternative.**
An official MailerLite CLI **does exist** and must be engaged honestly, not
denied: `github.com/mailerlite/mailerlite-cli`, published under the MailerLite
GitHub org, documented at <https://developers.mailerlite.com/cli>. Verified
against the repo, it satisfies **all four** stage-1 `cli`-type rubric
conditions (the `github`→`gh` model the skill names):

1. *Official CLI exists* — yes, MailerLite-org owned.
2. *Non-interactive / agent-friendly* — a global `--json` flag on every
   command emits raw JSON; the TUI is an opt-in `dashboard` subcommand, not
   the default.
3. *Env/flag credential injection* — reads `MAILERLITE_API_TOKEN` from the
   environment (the exact env var this design injects for service-type).
4. *Provisionable binary* — pre-built Linux/macOS/Windows release binaries
   plus `go install github.com/mailerlite/mailerlite-cli@latest`.

So the cli-vs-service call is a genuine tradeoff, not a foregone conclusion.
**I still choose `service` type**, on legitimate grounds:

- **CLI immaturity / stability risk.** The binary is very young: created
  2026-02-15, latest tag **v1.0.2 (2026-02-19)**, **6 commits, 3 stars, 0
  forks**, single-author. Its output contract (the `--json` shape we would
  parse) has no track record and could break or be abandoned between our pin
  bumps — a fragile foundation for a shipped tool versus wrapping the stable,
  versioned Connect REST API directly.
- **Runtime image size (master-plan §6 cli-type gate).** Any cli-type
  proposal triggers an image-size check; adding another Go binary to the
  runtime image for a surface we can hit over HTTP with zero footprint is not
  justified while the binary is this immature.
- **Long-term maintenance & consistency.** Service type keeps MailerLite on
  the same HTTP/httptest-fake pattern as 21/23 shipped tools and every ESP
  sibling, so there is one code shape to maintain and no third-party binary
  provisioning to track. If mailerlite-cli matures (stable releases, adoption,
  a committed output contract), revisiting cli-type is a clean future swap.

Net: `service`-type implementation against the Connect HTTP API — chosen
against the real cli-type alternative, not in ignorance of it.

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
  with zero config, like the mongodb/semrush credentials providers).
- **Scopes / token semantics:** none. The token is all-or-nothing per
  account, non-expiring, permanently bound to its creating user. No refresh
  cycle → the token gateway serves it directly (seed `access_token` only in
  L4; omit `refresh_token`/`expires_at`).
- **Entry path:** the user pastes the token through the write-only
  `POST /connections/credentials` connect UI; it is stored in Vault and never
  touches the bundle (per `references/provider-yaml.md`).

**Verification / identity — the one design decision, grounded on `main`.**
There is **no** generic declarative verify-only capability on `main` to inherit,
and `identity.source: verifier` / a `verify_url` manifest field **do not exist**
(the `IdentitySource` enum in `model/catalog.go` is only
`userinfo | token_response | strategy`; provider-gen strict-decode rejects unknown
fields). The credentials path is nailed down by `ValidateRuntimeContract`:
`AuthCredentials` **requires** `identity.source: strategy` ("no provider-side
verification endpoint") and forbids a userinfo/verify URL. So identity for a
credentials provider is produced by a **compiled deriver**, selected in
`composeProviderRegistration` (`service/provider_registry.go`), not by a
declarative verify stanza.

The on-`main` `manual_credentials` case hard-binds `dsnHostIdentityDeriver{}`,
which `url.Parse`s the secret and extracts a DSN host. A MailerLite token is an
opaque Bearer string with **no host** — that deriver returns
`manualCredentialFormatError` ("requires a connection string with a host") and
**every Connect fails**. So MailerLite cannot reuse the mongodb deriver verbatim.
This is the **semrush/moz** shape (opaque key, no userinfo endpoint, needs a
bespoke deriver) — and semrush/moz are on unmerged per-tool branches, not `main`.
There is therefore **MailerLite-owned integration-service work** here, budgeted
with its own unit tests (mirroring tasks #184/#199 for semrush/moz), not
inherited coverage.

Two buildable variants, both `auth.type: credentials` +
`identity.source: strategy` + `runtime_strategy: manual_credentials`, both
adding a compiled deriver keyed by `definition.Provider` in the
`RuntimeStrategyManualCredentials` arm (defaulting to `dsnHostIdentityDeriver`
for mongodb, selecting the MailerLite deriver for `provider == mailerlite`):

- **Primary — no-verify (`mailerliteKeyIdentityDeriver`, minimal).** Mirrors the
  mongodb no-verify contract but for an opaque key: `Verify` performs **no**
  provider HTTP call, derives the account key/label from the token, and stores
  as-is; a bad token surfaces at first use via AnyCLI's `CredentialRejected`
  classification (stale feedback, the accepted design-317 OQ1 trade). Smallest
  change; one deriver + one unit test.
- **Opt-in — verify-on-connect (`mailerliteAPIVerifier`, mirrors
  `semrushAPIUnitsVerifier`).** Same bundle, but the deriver additionally GETs
  `GET /subscribers?limit=0` with the Bearer token: `200` ⇒ store,
  `401 {"message":"Unauthenticated."}` / `403` ⇒ reject at connect with real
  feedback (`invalid_provider_credential`, the code `ManualCredentialService`
  already maps for 401/403). MailerLite gives a cheap real check, so this is the
  better UX — but it is **explicit MailerLite-owned capability growth**
  (a compiled verifier + its unit tests), not a reuse of anything on `main`.

**account_key / label derivation (both variants).** No API account identifier
exists, but a **static constant** (e.g. `"mailerlite"`) is wrong: with
`identity.source: strategy` the account key is per-connection, feeds the
`(org_id, provider, account_key)` model, and is the human-readable label — a
global literal collides and carries no information. Follow semrush: derive a
**non-reversible last-4-characters** key/label from the pasted token
(e.g. label `MailerLite ••••ab12`, key `ab12`). The deriver returns
`(identity, label, accountKey)` and the secret never enters the identity map,
so Connection metadata stays secret-free. Field names and the exact label
format are confirmed against `service/manual_credential.go`
(`defaultAccountName(label, accountKey, provider)`) and the semrush verifier
before pinning.

## 5. Helio provider bundle plan

Directory `integrations/providers/mailerlite/provider.yaml` (axis ③
`key: mailerlite`). **Axes ①/②/③ are all `mailerlite`** — no `tool.command`
group, no `tool.group`, and **no `toolToProvider` entry** (id == key; the
resolver's identity default applies). Register nothing in
`resolver.go`. Hidden-first.

Sketch — the mongodb/semrush `credentials` + `manual_credentials` shape
verbatim (the only credentials shape provider-gen strict-decode accepts on
`main`); the compiled deriver from §4 supplies identity, so the bundle carries
**no** verify stanza:

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
  type: credentials         # design 317 D5 opaque-key path (no userinfo endpoint)
  owner: individual
  credential_input:
    fields:
      - name: api_token     # exactly one required field (D5 single-secret storage)
        label_key: mailerlite_api_token
        secret: true
        required: true
        placeholder: "eyJ0eXAiOi…"     # MailerLite tokens are long opaque strings
    setup_url: https://www.mailerlite.com/help/where-to-find-the-mailerlite-api-key-groupid-and-documentation

identity:
  source: strategy          # compiled deriver (§4); credentials REQUIRES strategy

connection:
  mode: isolated
  disconnect_mode: local_only   # no v3 token-revoke API; dashboard-managed
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_token: token.access_token       # single secret via existing UpsertUserToken path
    account_key: connection.account_key # last-4-chars key from the deriver (§4)

tool:
  name: mailerlite
  kind: api-key             # wire-compat value; clients route the drawer by auth_type
```

- `required_config_fields` is **empty** → `configured: true` with zero
  environment config; **nothing lands in `config/` or `deploy/`** for this
  provider (no client id/secret). This removes the §2 seventh-shared-surface
  (oauth config) work entirely.
- The bundle is declaratively identical to mongodb/semrush; the MailerLite-specific
  work is the **compiled deriver** wired in `service/provider_registry.go`
  (§4) plus its unit test — not a manifest field. Diff the bundle against
  `integrations/providers/mongodb/provider.yaml` at build time (both are
  `credentials`/`manual_credentials`/`identity.source: strategy`); the only
  intended differences are the key/name/labels and `placeholder`.
- **UI icon** (not generated): `ui/helio-app/src/integrations/icons/mailerlite.svg`
  + hand-register in `providerIcons.ts`; add `tools.desc.mailerlite` /
  `mailerlite_api_token` i18n strings across locales.

## 6. Test plan → five layers

| Layer | For MailerLite | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — httptest fakes per verb: path/method/query, `Authorization: Bearer` from `MAILERLITE_API_TOKEN`, `--json` vs plain output, `401`/`422` error envelopes. | No |
| **L2** harness real-API | `ANYCLI_CRED_API_TOKEN=<real> anycli mailerlite -- subscriber count` and a `campaign list --status sent` against `connect.mailerlite.com` — proves field name, header injection, and request shape match the live API. **Mandatory before pin bump.** | **Yes** — one real MailerLite token (free tier works; account pool lane 2). |
| **L3** projections + suites | From `go-services/integration-service`: `provider-gen` then `provider-gen --check`; then `helio-cli` (with local `replace` → this anycli branch) `go build ./...` + `go test ./cmd/heliox/cmds/tool/`, and the integration-service unit suite **including a new MailerLite-owned deriver test** (`mailerlite` identity/verify deriver — mirrors `manual_semrush_verifier_test.go`; no coverage is inherited from semrush/moz, which are on unmerged branches) plus a bundle-load assertion if the suite enumerates bundles. | No |
| **L4** singleton + seed | `make run-singleton`; `POST /internal/test-only/connections/seed` with `provider:"mailerlite"`, a **real** token as `access_token` (no refresh_token/expires_at — non-expiring key), real org/assistant identities; then `heliox tool mailerlite -- subscriber count` returns live data through the token gateway. | **Yes** — same real token as L2. |
| **L5** connect flow (credentials path) | Pre-flip, hidden: open the connect link → paste the token in the real connect UI (`POST /connections/credentials`) → deriver stores it (no-verify variant) **or** GETs `GET /subscribers?limit=0` first (verify-on-connect variant) → connection shows connected/`configured` in `GET /connections` with a `••••<last4>` account label → one **unseeded** `heliox tool mailerlite -- subscriber list` through the real token gateway succeeds. Agent-drivable (agent-browser) per master-plan §2 api_key L5; human fallback on UI breakage. | **Yes** — real token pasted through the UI (account pool). |

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
