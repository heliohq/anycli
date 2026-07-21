# ServiceNow — per-tool design (`heliox tool servicenow`)

Batch scratch doc on branch `tool/servicenow` (stripped at batch-end). Drives the
`servicenow` tool per the `helio-tool-provider` pipeline. Catalog row: master plan
§4 row 78 — Product **ServiceNow**, anycli id `servicenow`, provider key
`servicenow`, auth lane **api_key**, wave **2**, category **Support**.

## 0. Verdict check against official docs (independent judgment)

The 2026-07-21 OAuth audit (row 80) keeps ServiceNow in **api_key** with the note
"no viable multi-tenant path". Verified against official docs and **confirmed**:

- ServiceNow OAuth 2.0 is **per-instance**. An OAuth application registry
  (`oauth_entity`) is created **inside each customer's own instance**
  (`https://<instance>.service-now.com/nav_to.do?uri=oauth_entity_list.do`); the
  client id/secret live in that instance. There is no ServiceNow-operated,
  multi-tenant authorization server where one registered Helio app could be
  authorized by arbitrary customer instances. So the rubric's "one registered
  app that arbitrary customer accounts can authorize" fails → **api_key** is
  correct. No divergence to record.

- The concrete api_key credential shape, however, is **not** a plain bearer key
  and **not** a subdomain-less token: ServiceNow credentials are **instance-scoped**
  (the instance host is part of every request URL). This makes ServiceNow a
  **two-input** manual credential — an instance URL **plus** a secret — which the
  current Helio `manual_credentials` D5 single-secret contract does not yet fit.
  This is the central design point (§4).

Sources: [Inbound REST API Keys — ServiceNow Community](https://www.servicenow.com/community/developer-advocate-blog/inbound-rest-api-keys/ba-p/2854924),
[Table API — Now Support Portal FAQ](https://support.servicenow.com/kb?id=kb_article_view&sysparm_article=KB0534905),
[ServiceNow REST API reference](http://servicenow.rest/).

## 1. What an AI teammate does with ServiceNow → the API surface

ServiceNow is an enterprise ITSM / workflow platform. A Helio teammate in the
**Support** category realistically:

- Reads/queries **incidents, problems, change requests, tasks** and their state,
  priority, assignment, and work notes.
- **Creates** an incident (logs a ticket out of a channel conversation).
- **Updates** a record — change state, add a work note/comment, (re)assign.
- Looks up **users / groups / CMDB CIs** to resolve references.
- Searches the **knowledge base** (`kb_knowledge`) and reads catalog requests
  (`sc_request`, `sc_req_item`).

Every one of these is a CRUD operation on a ServiceNow **table**, so the tool
wraps the **Table API** — the single generic REST surface that reads and writes
records in any table with immediate effect:

```
https://<instance>.service-now.com/api/now/table/{table}
```

GET (list/query + single get), POST (create), PATCH (update), DELETE, driven by
the `sysparm_*` query language (`sysparm_query` encoded query, `sysparm_limit`,
`sysparm_offset`, `sysparm_fields`, `sysparm_display_value`). This one surface
covers the entire list above generically; a thin **incident** convenience group
and a raw **api** escape hatch cover ergonomics and everything else (Aggregate
API, Import Set API, Attachment API) without per-endpoint code. This is the
notion precedent: a generic `api` passthrough + a few resource-shaped
subcommands, not a hand-modeled endpoint per object.

**Not wrapped** (out of scope for v1, reachable via `api` if needed): Aggregate
API stats, Import Set staging, Attachment upload/download (binary), Service
Catalog ordering. These are lower-frequency for a teammate and add binary/side-effect
complexity; the raw `api` verb keeps them reachable.

## 2. anycli definition

### 2.1 Form decision — `service` type

`service` type (HTTP against the Table API). Stage-1 `cli` rubric fails: there is
no official, non-interactive, `--json`-capable ServiceNow binary provisionable
into the runtime image (the ServiceNow CLI / `sn` SDK tooling is dev-workstation
oriented and interactive). 21/23 shipped definitions are `service`; ServiceNow
joins them. Package `internal/tools/servicenow/`, `RegisterService("servicenow", …)`.

### 2.2 The base-URL-from-credential shape (first-class precedent: mongodb)

Every other `service` tool has a **constant** `BaseURL`. ServiceNow's target host
is per-connection — it comes from the credential, exactly like `mongodb`, whose
connection target is the user-supplied connection string, not a constant. The
service reads the instance base URL from an injected credential field and builds
`<instance_url>/api/now/table/...` at request time. `BaseURL` is derived, not
hardcoded; tests point it at an `httptest.Server` by overriding the resolved base.

### 2.3 Command tree (verbs / subcommands)

```
servicenow table query <table> [--query <encoded>] [--fields a,b] [--limit N]
                                [--offset N] [--display-value all|true|false]   # GET list
servicenow table get   <table> <sys_id> [--fields a,b] [--display-value …]      # GET one
servicenow table create <table> --data '<json>'                                 # POST
servicenow table update <table> <sys_id> --data '<json>'                        # PATCH
servicenow table delete <table> <sys_id>                                        # DELETE

servicenow incident list   [--query <encoded>] [--limit N] [--fields …]         # sugar → table incident
servicenow incident get    <number|sys_id>
servicenow incident create --short-description <s> [--data '<json>']
servicenow incident update <number|sys_id> --data '<json>'
servicenow incident resolve <number|sys_id> --close-notes <s> [--code <s>]      # state + close fields

servicenow whoami                                                               # verify + identity echo
servicenow api <METHOD> <path> [--body '<json>'] [--query k=v]…                 # raw escape hatch
```

- `incident get/update/resolve` accept a human `number` (INC0010001) and resolve
  it to `sys_id` via a `sysparm_query=number=<n>&sysparm_limit=1` lookup — the AI
  and humans speak incident numbers, not sys_ids.
- `api` mirrors notion's raw verb: any `/api/now/...` path, method, body, query.
  It **rejects** attempts to override the auth header (`x-sn-apikey`) via a raw
  header (notion's `--header Authorization … "cannot be overridden"` precedent).

### 2.4 JSON output shape & exit codes (notion contract)

- Success: exit **0**, stdout is JSON. Table API responses wrap payloads in
  `{"result": …}`; the tool unwraps to the bare `result` (array for query, object
  for get/create/update) so agents consume `[…]` / `{…}` directly. `delete` prints
  `{"deleted": true, "sys_id": "…"}`.
- API/runtime failure: exit **1**, structured `--json` error envelope
  `{"error":{"status":<http>,"message":"…","detail":"…"}}` built from ServiceNow's
  own `{"error":{"message","detail"},"status":"failure"}` body.
- Usage/parse error: exit **2** (bad flags, malformed `--data`, missing `--query`).

A `capturedRequest` + `httptest`-backed `run()` harness (notion shape) asserts
request method/path/query, the injected `x-sn-apikey` header, the derived base
URL, and both plain and `--json` error rendering. Never hit the real API from a
unit test.

### 2.5 Auth block (two injected credential fields)

anycli has **no** single-secret restriction — a definition may bind multiple
credential fields. ServiceNow binds two:

```json
"auth": {
  "credentials": [
    { "source": { "field": "instance_url" },
      "inject": { "type": "env", "env_var": "SERVICENOW_INSTANCE_URL" } },
    { "source": { "field": "api_key" },
      "inject": { "type": "env", "env_var": "SERVICENOW_API_KEY" } }
  ]
}
```

The service reads `SERVICENOW_INSTANCE_URL` (→ derived base URL) and
`SERVICENOW_API_KEY` (→ `x-sn-apikey` request header). The credential-field names
`instance_url` / `api_key` are the contract the Helio bundle projects into (§4).

## 3. Credential model & exact auth flow (api_key lane, verified)

### 3.1 Primary credential: Inbound REST API Key (`x-sn-apikey`)

Verified against official docs. ServiceNow inbound API-key auth
(plugin **`com.glide.tokenbased_auth`**, "API Key and HMAC Authentication",
GA since the **Washington** release):

1. Admin activates the plugin, then creates an **Inbound Authentication Profile**
   (System Web Services → API Access Policies) with Auth Parameter = the
   `x-sn-apikey` **Auth Header** record.
2. Admin creates a **REST API Key** linked to a dedicated integration **user**
   (the user's roles scope what the key can read/write; `Auth Scope = useraccount`).
3. Admin attaches the profile to a **REST API Access Policy** targeting
   `/now/table/` (optionally by method/resource).
4. Requests send the key in the header: `x-sn-apikey: <key>`.

This is a **single revocable secret**, scoped by an admin-controlled user's roles,
with no expiry/refresh — the ideal api_key shape. It is the bundle default.

**Caveat, recorded in the AI-facing doc:** creating an API Access Policy on a
resource **locks that resource to the configured auth method(s)** — other methods
(e.g. Basic) must be explicitly re-added as profiles. The setup doc must state
this so a teammate's admin doesn't accidentally lock out existing integrations.

### 3.2 Secondary shape (documented, not the bundle default): Basic auth

`-u <user>:<password>` works on every instance and needs no plugin, but (a) it
stores a password, (b) it is two secrets packed into one, and (c) it is blocked
once API Access Policies are enabled. We do **not** make it the default. If a
customer's instance predates Washington or cannot enable the plugin, Basic is a
fallback the `api`/service layer could accept by treating `api_key` as
`user:password` and emitting an `Authorization: Basic` header instead of
`x-sn-apikey` — deferred; v1 ships the API-key header only. Flagged here so the
credential kind is not silently assumed.

### 3.3 Verify-first (recommended) vs no-verify

Unlike mongodb (no HTTPS identity endpoint → no-verify), ServiceNow **has** a
concrete per-instance endpoint. Connect verifies the key against the instance
with a minimal read:

```
GET <instance_url>/api/now/table/sys_user?sysparm_limit=1&sysparm_fields=user_name
x-sn-apikey: <key>
```

200 → good; 401/403 → reject at connect time (`invalid_provider_credential`),
matching `manual_api_token`'s verify-first UX rather than mongodb's stale-feedback
loop. (If the integration user lacks `sys_user` read, the doc names an alternate
low-privilege table to point the verify at; the verify table is a reviewed
constant, not user input.) No-verify is the fallback if the capability cost is
rejected in review.

### 3.4 Account key / identity

`account_key` = the **normalized instance base URL** (`https://<instance>.service-now.com`,
scheme+host, no trailing slash), derived from the user-typed instance field —
human-readable (mongodb OQ2), stable, unique per instance, and doubles as the
value anycli needs for the base URL. Multiple instances per assistant
(dev/test/prod) are naturally distinct connections selected with `--account`.

## 4. Helio provider bundle plan

### 4.1 Naming (three axes — all identical, simplest case)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `servicenow` (flat, not a group) | bundle `tool.command` (defaults to name) |
| ② anycli tool id | `servicenow` | `definitions/tools/servicenow.json` |
| ③ provider catalog key | `servicenow` | bundle dir `integrations/providers/servicenow/` |

②≡③ → **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.
Go package `servicenow` (no dash normalization needed).

### 4.2 `provider.yaml` (hidden-first) — shape

Modeled on `mongodb` (`auth.type: credentials`, `runtime_strategy: manual_credentials`,
`identity.source: strategy`, secret projected via `token.access_token`), extended
to two inputs:

```yaml
schema: helio.provider/v1
key: servicenow
go_name: ServiceNow
presentation:
  name: ServiceNow
  description_key: servicenow
  consent_domain: service-now.com
  visible: false            # hidden-first; flip is the single go-live change
auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: instance_url
        label_key: servicenow_instance_url
        secret: false
        placeholder: "https://acme.service-now.com"
        required: true
      - name: api_key
        label_key: servicenow_api_key
        secret: true
        placeholder: "REST API Key (x-sn-apikey)"
        required: true
    setup_url: https://www.servicenow.com/community/developer-advocate-blog/inbound-rest-api-keys/ba-p/2854924
identity:
  source: strategy          # account_key derived from instance_url (+ verify)
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
    api_key: token.access_token           # the secret
    instance_url: connection.account_key   # the instance base URL (== account_key)
tool:
  name: servicenow
  kind: api-key            # wire-compat value (317 D2), auth_type routes the drawer
```

Note: `credential.fields` uses **only existing** closed-vocabulary sources
(`token.access_token`, `connection.account_key`) — **no new `CredentialSource`**.
The token gateway hands anycli `{api_key, instance_url}`, matching §2.5.

### 4.3 Required integration-service capability growth (the real work)

The current `manual_credentials` contract forbids this bundle today. Three bounded
changes, all in `go-services/integration-service`, gated by tests:

1. **Relax D5 to "endpoint + secret".**
   `model/runtime_contract.go` `validateCredentialInputSchema` currently hard-fails
   `len(Fields) != 1 || !Fields[0].Required` (single-secret storage). Grow it to also
   accept exactly **one `secret:true, required:true`** field **plus** exactly
   **one `secret:false, required:true`** "endpoint" field. Storage stays
   single-secret (the secret → token payload); the non-secret field → `account_key`.
   `cmd/provider-gen/validate.go` `validateCredentialInput` allows the second field
   (it already permits N fields at the YAML layer; the count/secret rule is the
   model contract above).

2. **Carry the non-secret field through Connect.**
   `service/manual_credential.go` `resolveManualSecret` returns only the secret
   today. Extend it (or add a sibling) to also return the endpoint field's value,
   and thread that value into the identity deriver + `account_key` on Create/Update.
   The `values` map already arrives keyed by `credential_input.fields[].name`, so
   `{instance_url, api_key}` is available — the change is selecting the secret by
   `Secret==true` and passing the endpoint field on, instead of asserting exactly
   one key.

3. **Instance identity deriver (+ optional verify).**
   `service/provider_registry.go` composes `manual_credentials` with
   `dsnHostIdentityDeriver` (parses the secret). Add an
   `instanceIdentityDeriver` selected when the schema is endpoint+secret: it
   normalizes the instance_url field → `account_key`/label, and (verify-first
   variant, §3.3) issues the `sys_user?sysparm_limit=1` probe with `x-sn-apikey`
   before the Vault write, mapping 401/403 → `invalid_provider_credential`.

This is the **same instance-scoped pattern** the OAuth side already took: zendesk
(instance/subdomain-scoped OAuth) and salesforce (`instance_url` metadata capture).
ServiceNow brings it to the manual-credential side, and it **generalizes** to the
master-plan OQ3 "self-hosted URL + token" providers (Jenkins, Metabase,
Rocket.Chat, Mattermost) — build it as a reusable "endpoint + token" capability,
not a `servicenow` special-case. No token-gateway or Vault schema change (single
secret + existing `account_key` slot).

### 4.4 Service side, config, icon, docs

- **No OAuth config.** `manual_credentials` needs zero `config/` + `deploy/`
  secrets (no Helio client id/secret) — human lane 1 is not on this tool's critical
  path. Nothing to Config-Sync.
- **UI icon:** `ui/helio-app/src/integrations/icons/servicenow.svg` + register in
  `providerIcons.ts` (manual, never generated).
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` covering: connect (instance URL + REST API
  Key), the API-Access-Policy lockout caveat (§3.1), `table`/`incident`/`api`
  verbs, encoded-query syntax (`^` = AND), and that `number` is accepted for
  incidents. Bump plugin version + publish at batch end.
- **Generate:** `provider-gen` + `--check`; the five projections commit together
  with the bundle (batch-end).

## 5. Test plan — five layers

| Layer | What runs | External creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: `internal/tools/servicenow/` unit tests against an `httptest` fake Table API — asserts method/path/`sysparm_*` query, injected `x-sn-apikey`, derived base URL from `instance_url`, `{result}` unwrapping, number→sys_id lookup, incident sugar, `api` auth-header-override rejection, and 0/1/2 exit codes + `--json` error envelope. | **No** |
| **L2** | `anycli servicenow -- table query incident --limit 1` (and create/update/whoami) via the dev harness against a **real ServiceNow Personal Developer Instance** (developer.servicenow.com PDI — free, self-serve), creds from `ANYCLI_CRED_INSTANCE_URL` + `ANYCLI_CRED_API_KEY`. Proves field names, `x-sn-apikey` injection, and request shape match the live API. | **Yes** — PDI + configured REST API Key |
| **L3** | `provider-gen --check` + both repos' unit suites, **including** the new integration-service capability-growth tests (§4.3: endpoint+secret schema accepted, secret selected by `Secret==true`, `account_key` = instance URL, verify 401→reject). | No |
| **L4** | singleton + `POST /internal/test-only/connections/seed` with `{provider:"servicenow", account_key:"https://<pdi>.service-now.com", access_token:"<api key>"}` (non-expiring secret → seed `access_token` only, omit refresh/expiry), then `heliox tool servicenow -- table query incident --limit 1`. Seed sets `account_key` directly, so the token gateway projects `{api_key, instance_url}` and the run reaches the live PDI. | **Yes** — same PDI + key reused for the seeded token to reach the live API |
| **L5** | api_key key-entry path (master plan §2): `heliox tool servicenow auth` → open connect link → enter **instance URL + API key** in the two-field form → connection shows connected/configured in `GET /connections` → one **unseeded** live command succeeds. Agent-drivable (agent-browser), human fallback. This is the only layer that exercises the **new two-field connect form + instance-URL→account_key derivation + verify-first** — the L4 seed bypasses all of it. Run once while still hidden, then flip `visible: true` + regenerate. | **Yes** — PDI + key from the account pool |

**Externally-supplied credentials needed:** a ServiceNow **Personal Developer
Instance** (free self-serve at developer.servicenow.com; account-pool lane, wave 2)
with the `com.glide.tokenbased_auth` plugin active and a REST API Key +
Inbound Auth Profile (`x-sn-apikey`) + Access Policy configured against `/now/table/`.
The same PDI + key serve L2, L4, and L5. api_key L5 is agent-drivable, so no
human-consent session is required (unlike the oauth lanes).

## 6. Risks / open decisions

- **Capability growth is the gating item, not the endpoints.** The Table API
  wrapping is routine; the reviewed work is the endpoint+secret `manual_credentials`
  relaxation (§4.3). Land it as the reusable OQ3 capability. If review rejects
  verify-first, fall back to no-verify (mongodb parity) — the bundle still ships,
  bad keys just surface at first use.
- **Washington-release dependency.** Inbound API keys require the
  `com.glide.tokenbased_auth` plugin. ServiceNow's forced-upgrade cadence keeps
  supported instances at N/N-1, so this is safe in practice; the doc names the
  plugin and the Basic-auth deferral (§3.2) for pre-Washington edge cases.
- **Access-Policy lockout footgun.** Documented in the AI-facing doc (§3.1) so a
  teammate's admin re-adds existing auth profiles when creating the policy.
