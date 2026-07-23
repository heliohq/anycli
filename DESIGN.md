# Tool design: Salesforce (`salesforce`)

**Catalog row:** #15 · Salesforce · anycli id `salesforce` · provider key `salesforce` · lane `oauth_review` · Wave 1 · CRM.
**Branches:** anycli `tool/salesforce` (this worktree) · Helio `tool/salesforce`.
**Status:** design (scratch file — batch lead strips it at batch-end).

All claims below were verified against Salesforce's official documentation on 2026-07-21
(help.salesforce.com OAuth flow pages, developer.salesforce.com REST API Developer Guide
v67.0 / Summer '26) and against the current repo code (integration-service
`standard_oauth` capability set, token gateway, provider-gen validation, anycli built-in
service conventions per design 003 §3).

---

## 1. Naming (master plan §3)

| Axis | Value | Notes |
|---|---|---|
| ① CLI command word | `salesforce` | flat command, no `tool.group` (independent brand) |
| ② anycli tool id | `salesforce` | `definitions/tools/salesforce.json`, Go pkg `internal/tools/salesforce/` |
| ③ provider catalog key | `salesforce` | `integrations/providers/salesforce/provider.yaml` |

② == ③, so **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.

## 2. Auth lane verification (oauth_review — confirmed, with sharpened meaning)

Salesforce predates the 2026-07-21 OAuth audit's api_key sweep (it was oauth_review from
the seed catalog), so `oauth-audit.md` carries no row for it. Verified independently:

- **Protocol.** OAuth 2.0 web server (authorization-code) flow, PKCE S256 supported and
  officially recommended. Authorize `GET /services/oauth2/authorize`, token
  `POST /services/oauth2/token` (form-encoded; `client_secret` in the POST body or HTTP
  Basic — body wins when both present). For a multi-org app that does not know the
  customer's My Domain in advance, `https://login.salesforce.com` is the generic
  production host; sandboxes use `https://test.salesforce.com` (out of scope for v1, see
  §8 limitations).
- **Registration model.** One Connected App / External Client App (ECA) registered in a
  Salesforce org we control (Developer Edition or partner org). Spring '26 turned off
  default Connected App creation and positions **External Client Apps** as the
  go-forward shape — lane 1 should register an ECA, not a legacy Connected App.
- **Why oauth_review (verified 2026):** Salesforce's September 2025 security hardening
  ("Block uninstalled connected apps by default") means users in an arbitrary customer
  org can **no longer self-authorize** a third-party app: the customer org's admin must
  install/approve the app first (System Administrators hold the
  "Approve Uninstalled Connected Apps" permission by default). Frictionless external
  distribution therefore runs through **AppExchange listing + Salesforce security
  review** — a genuine heavy review program. This matches the catalog's oauth_review
  assignment; the review clock gates only the visible flip. Divergence from a naive
  "one app, any org authorizes" reading: even after our AppExchange review clears,
  per-org admin installation/approval remains part of the customer connect UX. Recorded
  here per the master plan's guidance; no catalog change needed.
- **Dev/L4/L5 unaffected:** in our own dev org we are the admin, so the dev-mode app
  authorizes immediately — lane 1's app creation (not review) gates L4/L5, exactly the
  §2 execution model.
- **Token semantics (all verified in official flow docs):**
  - Token response fields: `access_token`, `refresh_token` (only when the
    `refresh_token` scope is granted), `instance_url`, `id` (identity URL),
    `signature`, `scope`, `id_token`, `token_type: Bearer`, `issued_at`.
    **No `expires_in`.** Access-token lifetime = the org's session timeout
    (default 2 h, configurable down to 15 min). See §6 gap (b).
  - `instance_url` is the org's My Domain URL and is the **mandatory** API base:
    Spring '26 hostname enforcement ended legacy instance-hostname redirects — API
    calls must target My Domain URLs. See §6 gap (a).
  - Refresh: `grant_type=refresh_token` at the same token endpoint; returns a new
    `access_token` + `instance_url`; the refresh token is **not rotated** (stays
    valid until revoked) → `single_active_token: false`, `refresh_lease: none`.
  - Revoke: `POST https://login.salesforce.com/services/oauth2/revoke` with `token=`
    (accepts refresh or access token) → `disconnect_mode: provider_revoke`.
  - Userinfo: `GET https://login.salesforce.com/services/oauth2/userinfo` with the
    bearer token (requires `openid` scope). Returns `user_id`, `organization_id`,
    `preferred_username` (Salesforce username, globally unique), `email`, `name`.
- **Scopes** (from the OpenID discovery document): request
  `openid email profile api refresh_token`. `api` covers the full REST/data surface;
  `refresh_token` (synonym `offline_access`) is required for refresh-token issuance;
  the OIDC trio feeds identity resolution.

## 3. What an AI teammate does with Salesforce → API surface

An AI teammate acting as a sales/ops colleague needs to: look up customers before a
meeting ("what do we know about Acme?"), answer pipeline questions ("open opportunities
closing this quarter"), keep CRM hygiene (create/update leads, contacts, opportunities,
log calls/tasks after meetings), and triage cases. That maps to a small, stable subset
of the **Salesforce Platform REST API** (`/services/data/vXX.0/…` at the connection's
`instance_url`):

| Need | Endpoint |
|---|---|
| Ad-hoc questions over any object | `GET /query?q=<SOQL>` (+ `nextRecordsUrl` paging), `GET /queryAll` (includes deleted/archived) |
| Cross-object fuzzy find ("Acme") | `GET /parameterizedSearch?q=…&sobject=…` (SOSL without hand-writing FIND syntax) |
| Read one record | `GET /sobjects/{type}/{id}` (optional `?fields=`) |
| Create / update / delete | `POST /sobjects/{type}` · `PATCH /sobjects/{type}/{id}` · `DELETE /sobjects/{type}/{id}` |
| Upsert by external id | `PATCH /sobjects/{type}/{extField}/{value}` |
| Discover objects/fields (so the model writes valid SOQL and payloads) | `GET /sobjects` · `GET /sobjects/{type}/describe` (trimmed projection — full describe is hundreds of KB) |
| Who am I / which org | `GET {instance_url}/services/oauth2/userinfo` |
| Org API budget | `GET /limits` |

**API version:** pin `v65.0` (Winter '26 — GA on every production org as of 2026-07;
latest is v67.0) as a package constant, overridable per-invocation with
`--api-version`. Versioned paths keep working for years (versions 21.0–30.0 only
retired in Summer '25), so a pinned floor is safe.

Deliberately **out of v1**: Bulk API 2.0 (async job lifecycle — poor fit for one-shot
CLI calls), Analytics/Reports, Chatter, Apex execution (arbitrary code execution via a
teammate credential is a security decision, not a CLI feature), Metadata/Tooling API
(dev workflows belong to the official `sf` CLI, not a CRM teammate).

## 4. anycli definition (stage 1–2)

**Type: `service`.** The stage-1 rubric requires ALL cli-type conditions to hold. An
official CLI exists (`sf`, formerly sfdx) and is `--json`-capable, but it fails two of
the four conditions: (1) credentials are not env/flag-injectable per invocation — `sf`
requires a stateful `org login` step that persists auth to `~/.sfdx`, incompatible with
anycli's per-call credential-map injection; (2) it is a Node.js bundle in the 100+ MB
class, exactly the runtime-image-size risk the master plan §6 flags, and its center of
gravity is dev workflows (deploy/apex/orgs), not CRM record operations. → `service`
type, like 21 of 23 existing definitions.

`definitions/tools/salesforce.json`:

```json
{
  "name": "salesforce",
  "type": "service",
  "description": "Salesforce CRM as a tool (SOQL queries, record CRUD, search, describe)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SALESFORCE_ACCESS_TOKEN"}
      },
      {
        "source": {"field": "instance_url"},
        "inject": {"type": "env", "env_var": "SALESFORCE_INSTANCE_URL"}
      }
    ]
  }
}
```

Two credential fields, LinkedIn-precedent shape (`access_token` + `person_urn` there;
`access_token` + `instance_url` here). `instance_url` is not a secret but it IS
per-connection state, so it rides the credential map, not a flag.

**Package:** `internal/tools/salesforce/` (id has no dashes → package name is the id),
registered in `internal/tools/register.go` `init()` as
`RegisterService("salesforce", &salesforce.Service{})` (registration line rides the
batch-end merge; the package + definition JSON merge freely mid-batch).

**Service struct** (design 003 §3 template, notion as reference implementation):

```go
type Service struct {
    BaseURL string       // overrides SALESFORCE_INSTANCE_URL; tests → httptest server
    HC      *http.Client // nil = http.DefaultClient
    Out, Err io.Writer   // nil = process streams
}
```

`Execute(ctx, args, env)` reads `SALESFORCE_ACCESS_TOKEN` + `SALESFORCE_INSTANCE_URL`
from the resolved env map (missing either → exit 1 with explicit message, JSON envelope
under `--json`), builds the cobra tree per call, no interactive prompts.

**Subcommand tree** (resource-grouped; every command supports `--json`):

```
salesforce query   <soql> [--all] [--max-records N]      # GET /query | /queryAll, follows nextRecordsUrl up to cap
salesforce search  <term> [--objects Account,Contact] [--fields ...] [--limit N]   # GET /parameterizedSearch
salesforce record get     <sobject> <id> [--fields a,b]
salesforce record create  <sobject> --data '<json>'      # or --data @file / stdin
salesforce record update  <sobject> <id> --data '<json>'
salesforce record delete  <sobject> <id>
salesforce record upsert  <sobject> <ext-id-field> <value> --data '<json>'
salesforce sobject list   [--custom-only|--standard-only]  # trimmed: name,label,custom,queryable
salesforce sobject describe <sobject> [--field-names-only]  # trimmed: field name,label,type,picklist values,required,updateable
salesforce whoami                                          # GET {instance}/services/oauth2/userinfo
salesforce limits                                          # GET /limits (DailyApiRequests etc.)
```

**JSON output shape** (built-in service conventions, 003 §3): success writes the
provider's JSON response verbatim to stdout (query results as the raw
`{totalSize, done, records: [...]}` envelope with pages concatenated into `records`;
create returns `{id, success, errors}`; update/delete are 204 → emit
`{"success": true, "id": "..."}`). `sobject list/describe` emit a **trimmed**
projection (full describe payloads are pathological for context windows); a
`--raw` escape hatch returns the untrimmed body. Exit codes: 0 success, 1 runtime/API
error (typed `apiError` carrying Salesforce's `[{"errorCode","message"}]` array — note
Salesforce error bodies are JSON **arrays**, not objects), 2 usage/parse errors.
Errors render to stderr, structured envelope under `--json` (notion precedent).

## 5. Helio provider bundle (stage 4)

`integrations/providers/salesforce/provider.yaml`, **hidden-first**:

```yaml
schema: helio.provider/v1
key: salesforce
go_name: Salesforce

presentation:
  name: Salesforce
  description_key: salesforce
  consent_domain: salesforce.com
  visible: false          # flip gated on L5 + AppExchange review clearance
  order: <batch-assigned>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://login.salesforce.com/services/oauth2/authorize
    token_url: https://login.salesforce.com/services/oauth2/token
    token_exchange_style: form_secret
    pkce: s256                    # officially recommended; exchanger supports it
    scopes: [openid, email, profile, api, refresh_token]
    display_scopes: [openid, email, profile, api, refresh_token]
    single_active_token: false
    refresh_lease: none           # refresh token is not rotated on use
    revoke:
      url: https://login.salesforce.com/services/oauth2/revoke
      client_auth: none
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://login.salesforce.com/services/oauth2/userinfo
  stable_key: /user_id                     # 18-char user id, globally unique
  label_candidates: [/preferred_username, /email, /user_id]

connection:
  mode: isolated
  disconnect_mode: provider_revoke
  runtime_strategy: standard_oauth         # + the two capability extensions in §6

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    instance_url: connection.metadata.instance_url   # NEW source — see §6 (a)
    account_key: connection.account_key

tool:
  name: salesforce
  kind: oauth
```

The fixed `login.salesforce.com` userinfo host is consistent with the fixed authorize
host: v1 supports production + Developer Edition orgs (both live behind
login.salesforce.com); sandboxes are a later variant (§8).

Icon: `ui/helio-app/src/integrations/icons/salesforce.svg` + manual
`providerIcons.ts` registration (rides batch-end). Docs: new provider sub-doc
`agents/plugins/heliox/skills/tool/salesforce/` (flat-provider precedent: `notion/`,
`x/`), plugin version bump + publish ride the batch-end merge.

## 6. Where Salesforce exceeds today's `standard_oauth` capability set (the real design decision)

Two verified gaps. Both are Helio-side (integration-service), neither touches anycli.
Per `references/provider-yaml.md`'s explicit guidance — "first check whether the gap is
really provider-specific or whether the generic `standard_oauth` capability set should
grow one more reviewed enum value instead" — the recommendation is **grow the generic
capability set**, because both gaps recur across this catalog. A narrow
`service/adapter_salesforce.go` (LinkedIn `person_urn` precedent) is the fallback if
the batch lead rules the generic extensions out of batch scope.

**(a) Per-tenant API base URL (`instance_url`).** The token response's `instance_url`
is the only supported API host (Spring '26 hostname enforcement). Today the
credential-source enum is closed (`token.access_token`, `connection.account_key`,
`connection.metadata.person_urn`, `credential.app_id`, `credential.brand` —
`cmd/provider-gen/render_symbols.go`), and the only token→metadata capture is the
hard-coded LinkedIn `person_urn` copy in `oauth_callback.go`. Proposal:

  1. A declarative capture in the bundle, e.g.
     `connection.metadata_capture: {instance_url: /instance_url}` — RFC 6901 pointer
     applied to the sanitized token response at callback time (and refreshed values
     re-captured on token refresh, since `instance_url` can change on org migration).
  2. One new reviewed credential source `connection.metadata.instance_url`, served by
     the token gateway exactly as `connection.metadata.person_urn` is today.

  Generic justification: Shopify (shop domain), Zoho CRM/Books (`api_domain`), Zendesk
  (subdomain), Basecamp/Harvest (account id) in this same catalog all need
  connect-time tenant identifiers projected into the credential map.

**(b) No `expires_in` in the token response.** Verified against the official web
server flow doc: the sample response carries `issued_at` but **no `expires_in`**;
actual lifetime is the org session timeout (default 2 h, minimum 15 min). Today
`tokenExchangeResponse.expiry()` returns nil for `ExpiresIn <= 0` and
`needsRefresh()` treats a nil expiry as **non-expiring** (`token_gateway.go`), so the
gateway would serve a dead token forever after ~2 h and anycli/heliox would surface
`INVALID_SESSION_ID` errors with no refresh ever attempted — a silent-downgrade
failure the repo's hard rules forbid. Proposal: a reviewed bundle field, e.g.
`auth.oauth.assumed_ttl_seconds: 900`, applied at exchange AND refresh time when the
provider response lacks `expires_in`; 900 s sits at the minimum configurable session
timeout, so the gateway proactively refreshes well inside any org's real lifetime
(cost: one refresh round-trip per account per 15 min of active use — the refresh
token is durable and non-rotating, so this is safe). Alternative rejected: calling
`/services/oauth2/introspect` per resolution adds a provider round-trip to every
token-gateway hit for exact TTLs we don't need.

**Consequence for the plan:** Salesforce is NOT a zero-service-code
`standard_oauth` bundle; it is one of the master plan §5's budgeted "handful of the
review lane" capability cases. The two extensions land in integration-service with
their own unit tests (synthetic provider fixtures, the existing
`standard_oauth_test.go` pattern) inside this tool's Helio-side branch.

## 7. Test plan (skill five layers)

| Layer | What runs | External credentials needed |
|---|---|---|
| **L1** | anycli `go test ./...`: httptest fakes for query (incl. `nextRecordsUrl` pagination), parameterizedSearch, record CRUD + upsert (201/204/404 paths), describe trimming, whoami, limits; assert `Authorization: Bearer` header, base-URL construction from `SALESFORCE_INSTANCE_URL`, versioned path `/services/data/v65.0/…`, JSON **array** error-body rendering (`[{"errorCode": "INVALID_SESSION_ID", …}]`) in both plain and `--json` envelopes, exit codes 0/1/2, missing-credential (either field) exit 1. TDD: tests first, per anycli AGENTS.md. | none |
| **L2** | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=… ANYCLI_CRED_INSTANCE_URL=… anycli salesforce -- query "SELECT Id,Name FROM Account LIMIT 5"` plus one create→get→update→delete cycle on a scratch record, `whoami`, `sobject describe Account`, against the real API. | **Yes — lane 2**: a Salesforce Developer Edition org (free, self-serve) from the test-account pool; access token minted from the lane-1 dev ECA (or a temporary session id for early harness runs). |
| **L3** | On-branch only (not committed): local `go run ./cmd/provider-gen` + `--check` against the bundle; `helio-cli` built with an uncommitted `go.mod` `replace github.com/heliohq/anycli => <this worktree>`; both repos' unit suites incl. the new integration-service capability tests (§6). Branch CI is expected to fail `provider-gen --check` until the batch lead's canonical regen — do not commit local regens. | none |
| **L4** | Singleton (`make run-singleton`, `env: dev`) + `POST /internal/test-only/connections/seed` with a **real** access_token + refresh_token from the dev org and a deliberately short `expires_at`, so the very next `heliox tool salesforce -- query …` is forced through the gateway's refresh-and-write-back path — this doubles as the regression check for §6 (b)'s assumed-TTL behavior. Seed body must also land `instance_url` in connection metadata (via §6 (a)'s capture path or an explicit seed field — confirm the seed endpoint carries metadata through; extend it in the same branch if not). Success = live data from the real org through the token gateway. | **Yes — lane 1**: dev ECA client id/secret as uncommitted local `config/cloud.yaml` entries, distributed at app creation; real tokens from the pool org. |
| **L5** | Human-in-the-loop (lane 3, per-batch sweep): `heliox tool salesforce auth` → connect link → Salesforce consent on the pool org (we are that org's admin, so the Sept-2025 uninstalled-app block does not bite) → `oauth_connected` system event → one unseeded live command. Gates the visible flip **together with** AppExchange security-review clearance (oauth_review lane). Lane 1's committed config append (id+secret to `config/` + `deploy/` Helm Secret together) must precede this run. | **Yes — lanes 1+3**: registered app config landed; human consent session on a pool-org login. |

## 8. Limitations, risks, and follow-ups (recorded, not blocking)

- **Sandbox orgs unsupported in v1** — fixed `login.salesforce.com` authorize/token/
  userinfo hosts exclude `test.salesforce.com` sandboxes. Fine for the teammate use
  case (CRM of record is production); a sandbox variant would need host selection at
  connect time (new capability, out of scope).
- **Customer-org admin friction** — until AppExchange review clears AND the customer
  admin installs/approves the app, users in that org cannot complete consent
  (Sept 2025 hardening). The connect-failure UX ("app must be installed") should be
  mentioned in the provider sub-doc; nothing to build.
- **Professional Edition orgs** lack API access by default (API add-on required) —
  connect succeeds but `api`-scope calls 403. Doc note only.
- **Org API limits** are consumed per call (`/limits` exposed as a command exactly so
  the teammate can self-throttle); no Helio-side rate handling in v1 beyond surfacing
  Salesforce's 403 `REQUEST_LIMIT_EXCEEDED` error verbatim.
- **`expires_in` re-check at implementation:** External Client Apps expose an optional
  per-app access-token TTL setting; if the lane-1 ECA's token responses turn out to
  include `expires_in` (behavior may differ from the legacy Connected App sample docs),
  §6 (b) shrinks to a no-op safety net — keep `assumed_ttl_seconds` anyway; it is
  harmless when the provider supplies a real TTL (real value wins).

## References (official)

- OAuth 2.0 Web Server Flow — help.salesforce.com `remoteaccess_oauth_web_server_flow.htm`
- OAuth 2.0 Refresh Token Flow — help.salesforce.com `remoteaccess_oauth_refresh_token_flow.htm`
- UserInfo / OpenID discovery — help.salesforce.com `remoteaccess_using_openid_discovery_endpoint.htm`
- REST API Developer Guide v67.0 (Summer '26) — developer.salesforce.com `api_rest` (Versions, Query, SObject Rows, Parameterized Search, Describe, Limits)
- External Client Apps — help.salesforce.com `external_client_apps.htm`
- Sept 2025 connected-app hardening (uninstalled-app blocking, device-flow removal) — Salesforce security advisory coverage, cross-checked against help.salesforce.com OAuth pages
