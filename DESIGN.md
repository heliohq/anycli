# Tool design: Pipedrive

Scratch design for the `tool/pipedrive` branch (batch lead strips this file at
batch-end). Catalog row: #17 — Pipedrive / anycli id `pipedrive` / provider key
`pipedrive` / auth lane `oauth_light` / Wave 1 / CRM
(`docs/design/008-300-integrations-rollout-plan.md`).

## 0. Verification against official docs (independent judgment)

All auth facts below were re-verified against Pipedrive's official developer
docs (pipedrive.readme.io / developers.pipedrive.com), not inherited from the
catalog or the audit.

- **Audit verdict `oauth_light`: CONFIRMED.** Standard OAuth 2.0
  authorization-code flow. A private (unlisted) app registered in Developer Hub
  goes live without human review — only an automated OAuth callback check —
  and can be installed by any Pipedrive company via its direct install link
  (official: "Registering a private app"). Marketplace review applies only to
  public listings, which we do not need.
- **Authorize URL** — `GET https://oauth.pipedrive.com/oauth/authorize` with
  `client_id`, `redirect_uri`, `state` only. **There is no `scope` query
  parameter**: scopes are fixed on the app registration in Developer Hub, and
  the consent screen shows the app's configured scopes. Bundle therefore uses
  `display_scopes` (Notion precedent), not `scopes`.
- **Token URL** — `POST https://oauth.pipedrive.com/oauth/token`,
  `application/x-www-form-urlencoded` body, client auth via HTTP **Basic**
  header `base64(client_id:client_secret)` (body credentials are officially
  discouraged) → `token_exchange_style: form_basic`, `pkce: none` (PKCE is not
  part of Pipedrive's documented flow).
- **Token response** — `access_token`, `token_type: "Bearer"`,
  `expires_in` (~3599 s), `refresh_token`, `scope`, and **`api_domain`** (the
  per-company base URL, e.g. `https://acme.pipedrive.com`). Authorization code
  expires in 5 minutes.
- **Token semantics** — access token lives ~60 minutes; refresh token is
  **non-rotating** (the same refresh token is reissued) with a rolling 60-day
  inactivity expiry → `refresh_lease: none`, and the token gateway's standard
  refresh path (A3) is exercised on every ~hourly expiry.
- **Revocation** — **no token-revocation endpoint exists in official docs**
  (the OAuth reference documents only `/oauth/authorize` and `/oauth/token`;
  the separate "app uninstallation" page covers the reverse direction —
  Pipedrive notifying the app). Third-party blogs describe an
  `https://oauth.pipedrive.com/oauth/revoke` endpoint, but it is undocumented
  officially, so we do **not** build on it: `disconnect_mode: local_only`
  (Notion precedent). Divergence recorded here per the prompt's instruction.
- **API base URL** — after auth, **all API calls must go to
  `{api_domain}`** (`https://{company}.pipedrive.com/api/...`) with
  `Authorization: Bearer <access_token>`. There is no fixed-host userinfo URL
  usable before you know `api_domain` — this drives the identity and
  credential-projection decisions in §3.
- **API v1 sunset is imminent** — official changelog: v1 endpoints with direct
  v2 equivalents (Deals, Persons, Organizations, Activities, Products,
  Pipelines, Stages, itemSearch, Fields) are deprecated, sunset extended to
  **2026-07-31** — ten days from now. The tool is therefore **v2-first**
  (`{api_domain}/api/v2/...`, cursor pagination, PATCH for updates). Leads,
  Notes, and Users have **no v2 equivalents and are not in the deprecated
  set**; they stay on `{api_domain}/api/v1/...`.
- **Scopes** (official "Scopes and permission explanations"): `base` is always
  granted; granular scopes come in `:read`/`:full` pairs. Notes have no
  dedicated scope — they ride `deals:*` / `contacts:*`. Pipelines/stages are
  readable under `deals:read`; creating them needs `admin` (out of scope).

## 1. What an AI teammate does with Pipedrive → API surface

An AI teammate acting as a sales/CRM assistant needs to: look up and update
deals as conversations progress (move stage, mark won/lost), find and maintain
people and organizations, log and schedule activities (calls, meetings,
tasks), capture leads, leave notes, and search across the CRM. It does not
administer the account (no pipeline/stage/custom-field/user management, no
webhooks, no mail sync) and should not bulk-delete CRM records.

Wrapped endpoints (v2 unless marked v1):

| Area | Endpoints | Why |
|---|---|---|
| Deals | `GET/POST /api/v2/deals`, `GET/PATCH /api/v2/deals/{id}`, `GET /api/v2/deals/search` | Core object; list filters (`person_id`, `org_id`, `pipeline_id`, `stage_id`, `status`, `owner_id`) replace the deprecated v1 nested listings; stage moves and won/lost are `PATCH` field updates (`stage_id`, `status`, `lost_reason`) |
| Persons | `GET/POST /api/v2/persons`, `GET/PATCH /api/v2/persons/{id}`, `GET /api/v2/persons/search` | Contact maintenance |
| Organizations | `GET/POST /api/v2/organizations`, `GET/PATCH /api/v2/organizations/{id}`, `GET /api/v2/organizations/search` | Account maintenance |
| Activities | `GET/POST /api/v2/activities`, `GET/PATCH/DELETE /api/v2/activities/{id}` | Schedule / complete (`done`) calls, meetings, tasks |
| Leads | `GET/POST /api/v1/leads`, `GET/PATCH/DELETE /api/v1/leads/{id}` | Lead capture; v1-only (no v2 exists) |
| Notes | `GET/POST /api/v1/notes`, `GET/PUT/DELETE /api/v1/notes/{id}` | Logging context on deals/persons/orgs; v1-only |
| Pipelines/Stages | `GET /api/v2/pipelines`, `GET /api/v2/stages` (+ `/{id}`) | Read-only — needed to interpret and move deals |
| Users | `GET /api/v1/users/me`, `GET /api/v1/users` | Identity check + owner assignment; v1-only |
| Search | `GET /api/v2/itemSearch` | Cross-entity lookup ("find the Acme deal") |

Deliberately excluded from v1 of the tool: person/org/deal `DELETE`
(destructive; deals are closed as lost, not deleted), products, mail threads,
projects, goals, webhooks, files, custom-field management. All can be added
later behind the same command without a new provider.

## 2. anycli definition (stage 1–2)

**Type decision: `service`.** Stage-1 rubric: Pipedrive ships no official CLI
binary at all, so the `cli` branch is impossible; implement
`internal/tools/pipedrive/` (package name = id with no dashes, so identical)
against the HTTP API, following the `notion` reference shape (cobra tree per
`Execute`, `HC` + output writers injectable, exit 0/1/2 contract, `--json`
error envelope).

`definitions/tools/pipedrive.json`:

```json
{
  "name": "pipedrive",
  "type": "service",
  "description": "Pipedrive CRM as a tool (OAuth)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "PIPEDRIVE_ACCESS_TOKEN"}
      },
      {
        "source": {"field": "api_domain"},
        "inject": {"type": "env", "env_var": "PIPEDRIVE_API_DOMAIN"}
      }
    ]
  }
}
```

Two bindings follow the `linkedin` / `x` two-field precedent. The
**`api_domain` credential IS the base URL**: the service builds every request
from `PIPEDRIVE_API_DOMAIN` (validated: non-empty, `https://` URL; missing or
malformed → exit 1 with an explicit message, no fallback host). This doubles
as the natural test seam — unit tests inject an `httptest.Server` URL as the
`api_domain` credential instead of a separate `BaseURL` field.

### Command tree (verbs)

```
pipedrive deal      list|get|create|update|search
pipedrive person    list|get|create|update|search
pipedrive org       list|get|create|update|search
pipedrive activity  list|get|create|update|delete
pipedrive lead      list|get|create|update|delete
pipedrive note      list|get|add|update|delete
pipedrive pipeline  list|get
pipedrive stage     list|get
pipedrive user      me|list
pipedrive search    --term <q> [--types deal,person,organization,lead]
```

Flags mirror the API: entity flags (`--title`, `--value`, `--currency`,
`--stage-id`, `--status won|lost|open`, `--lost-reason`, `--person-id`,
`--org-id`, `--owner-id`, `--deal-id`, `--type`, `--subject`, `--due-date`,
`--done`, `--content`, …), pagination `--cursor`/`--limit` on v2 lists
(passthrough of `additional_data.next_cursor`) and `--start`/`--limit` on the
v1-only lists (leads/notes/users, offset pagination — kept as the provider
defines it, not papered over).

### JSON output shape

Per anycli design 003 §3 conventions: success writes Pipedrive's JSON response
**verbatim** to stdout (`{"success":true,"data":...,"additional_data":{...}}`
— cursor lives in `additional_data.next_cursor` on v2); failure writes a
one-line error including Pipedrive's error code/message to stderr and exits 1;
usage/parse errors exit 2. No re-shaping of provider payloads.

### Registration

`RegisterService("pipedrive", &pipedrive.Service{})` in
`internal/tools/register.go` — `register.go` is one of the seven shared
surfaces the master plan (§2) serializes to batch-end. It is nonetheless
**committed on this isolated tool branch on purpose**: L2–L4 all run the real
`anycli pipedrive -- …` binary, and without this line the built binary does not
dispatch `pipedrive` at all, so on-branch harness testing is impossible. The
batch lead re-merges this one-line addition (alongside the sibling tool
branches' entries) at batch-end and resolves the expected conflict there — flag
it in the batch-end sweep. The definition JSON and `internal/tools/pipedrive/`
are generation-inert and merge freely on this branch.

## 3. Helio provider bundle plan (stages 4–8)

Naming axes (master plan §3): ① CLI command `pipedrive` (flat command, no
group — independent brand), ② anycli id `pipedrive`, ③ provider key
`pipedrive`. **② == ③, so no `toolToProvider` entry in
`helio-cli/internal/toolcred/resolver.go`** — zero divergence to register.

`integrations/providers/pipedrive/provider.yaml` (hidden-first):

```yaml
schema: helio.provider/v1
key: pipedrive
go_name: Pipedrive

presentation:
  name: Pipedrive
  description_key: pipedrive
  consent_domain: pipedrive.com
  visible: false          # hidden-first; flip is the separate go-live change
  order: 130              # after existing 23; batch lead may renumber

auth:
  type: oauth
  owner: individual       # the human's Pipedrive login, delegated to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://oauth.pipedrive.com/oauth/authorize
    token_url: https://oauth.pipedrive.com/oauth/token
    token_exchange_style: form_basic   # form body + HTTP Basic client auth (official)
    pkce: none
    display_scopes: [base, deals:full, contacts:full, activities:full, leads:full, search:read, users:read]
    single_active_token: false
    refresh_lease: none

identity:
  source: token_response
  stable_key: /api_domain
  label_candidates: [/api_domain]

connection:
  mode: isolated
  disconnect_mode: local_only          # no official revoke endpoint (§0)
  runtime_strategy: standard_oauth     # zero service-side Go

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
    api_domain: connection.account_key

tool:
  name: pipedrive
  kind: oauth
```

Design notes on the two non-obvious choices:

- **Identity from `token_response` with `stable_key: /api_domain`.** Pipedrive
  has no fixed-host userinfo endpoint (`/users/me` only exists under the
  per-company `api_domain`, and the declarative identity resolver takes a
  fixed URL), and the token response carries no user id — only `api_domain`.
  So the connection is keyed by company domain, exactly parallel to Notion's
  workspace-level `workspace_id` key. Consequence: one connection per
  Pipedrive company per assistant (two humans of the same company connecting
  the same assistant would upsert one row) — acceptable at `mode: isolated`,
  same trade-off Notion already ships. Label is the company URL.
- **`api_domain: connection.account_key` projection.** The credential
  allowlist is closed (`token.access_token`, `connection.account_key`,
  `connection.metadata.person_urn` — provider-extension contract §"封闭允许
  列表"); there is no way to project an arbitrary token-response field.
  Because the stable key IS `/api_domain`, `connection.account_key` carries
  the base URL, and mapping a second credential name onto it follows the
  shipped `x` precedent (`user_id: connection.account_key`). Zero adapter,
  zero allowlist growth. Known limitation: Pipedrive documents that a company
  domain can change; the persisted account_key then goes stale and API calls
  fail until the user reconnects — fail-explicit, no silent fallback host.

**No service-side code** (stage 6): `standard_oauth` covers exchange
(form_basic), identity (token_response pointer), and no-op revoke. Lane 1
registers a **private Developer Hub app** with callback URL + the scope set
`deals:full, contacts:full, activities:full, leads:full, search:read,
users:read` (`base` is implicit). Scope choice is deliberate and up-front:
changing a Pipedrive app's scopes later forces "App permissions have changed"
re-authorization on every existing connection. Client id/secret land as
per-provider appends in `config/` + the `deploy/` Helm Secret together (Config
Sync rule), timed per lane 1; dev credentials arrive as uncommitted local
`config/cloud.yaml` entries for L4. No `experiment` gating.

**Icon** (stage 7): `ui/helio-app/src/integrations/icons/pipedrive.svg` +
manual `providerIcons.ts` registration (batch-end shared surface).

**AI docs** (stage 8): provider sub-doc
`agents/plugins/heliox/skills/tool/pipedrive.md` (command tree, v2 cursor
pagination, deal stage/status semantics, leads-vs-deals guidance); plugin
version bump + marketplace publish ride the batch-end merge.

**Batch-end shared surfaces recap**: anycli tag + `helio-cli/go.mod` pin bump,
the five provider-gen projections, `providerIcons.ts` append, plugin publish —
none committed from this branch. The one exception is the anycli `register.go`
entry, which **is** committed here (see §2 Registration: it is required for
on-branch L2–L4 binary dispatch); the batch lead re-merges it and resolves the
expected conflict at batch-end.

**Batch-end service-test coupling (not a §2-listed shared surface — flag it).**
Committing the regenerated `provider_catalog.gen.go` at batch-end trips a
hand-maintained tripwire test: `go-services/integration-service/service/provider_registry_test.go`
`TestDefaultProviderRegistryIsComplete` enumerates every catalog provider in a
hardcoded `wantStrategies` map and asserts `len(catalog) == len(wantStrategies)`.
Adding Pipedrive makes the catalog 24 while the map still lists 23, so the batch
lead MUST add `model.ProviderPipedrive: model.RuntimeStrategyStandardOAuth` to
that map **in the same commit as the projection regen** — committing it earlier
(without the regen) or omitting it (with the regen) both turn main red. This is
the one integration-service Go change a `standard_oauth` provider needs; it does
not contradict "zero service *code*" (no new adapter — the generic
`composeProviderRegistration` path handles standard_oauth), only the "no new Go"
shorthand in L3 below. On-branch, this branch stays green because the projection
is not committed; the failure surfaces only when the regen is applied locally.

On-branch validation uses locally run
`provider-gen` (+ `--check`) and a locally uncommitted
`replace github.com/heliohq/anycli => <this worktree>` in `helio-cli/go.mod`;
neither is committed, and this branch is expected to fail `provider-gen
--check` in CI until batch end.

## 4. Test plan (five layers)

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes assert `Authorization: Bearer` header injection, base URL taken verbatim from the `api_domain` credential, `/api/v2` paths + `PATCH` for updates, v1 paths for leads/notes/users, `cursor`/`limit` passthrough, verbatim success envelope, `{"success":false,...}` error rendering (plain + `--json`), missing/malformed `api_domain` or `access_token` → exit 1, usage errors → exit 2. TDD: tests first, per anycli AGENTS.md | none |
| L2 | Dev harness against the real API: `ANYCLI_CRED_ACCESS_TOKEN=<real Bearer> ANYCLI_CRED_API_DOMAIN=https://<company>.pipedrive.com anycli pipedrive -- deal list` (+ person search, activity create/done, note add, itemSearch). The Bearer token is minted by hand-driving the dev app's authorize URL + a curl token exchange — the tool speaks OAuth Bearer only; the personal `api_token` path is deliberately not implemented, so L2 cannot shortcut through it | **YES** — lane-1 dev app client id/secret + lane-2 test Pipedrive account |
| L3 | Local `go run ./cmd/provider-gen` + `--check` against the branch bundle (run only, projections NOT committed); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with the local `replace`; integration-service unit suite (synthetic standard_oauth coverage applies; the only Go change is the batch-end `wantStrategies` map entry in `service/provider_registry_test.go` — see the batch-end service-test coupling note in §3) | none |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with provider `pipedrive`, real `access_token` + `refresh_token` and a deliberately short `expires_at`, so `heliox tool pipedrive -- deal list` is forced through the token gateway's refresh-and-write-back path (form_basic refresh against the live token URL; Pipedrive's ~60-min expiry makes this the steady-state path, so it must be exercised). Note the seed payload has no `api_domain` field — `account_key` in the seed request must be set to the real `https://<company>.pipedrive.com` value since the credential projection reads it as the base URL. Real seeded org/assistant identities per the skill's L4 notes | **YES** — dev app client id/secret in local uncommitted `config/cloud.yaml` + real token pair from the test account |
| L5 | Human-in-the-loop (oauth lane, lane 3): `heliox tool pipedrive auth` → connect link → real Pipedrive consent on the test account → `oauth_connected` event on the originating channel → one unseeded live run. Gates the visible flip; runs in the post-merge batch sweep | **YES** — human consent session + test account |

Definition of done tracks the master plan: L1–L5 green, docs published, icon
registered, then `presentation.visible: true` + regenerate as the single
go-live change.
