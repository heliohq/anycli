# Tool design: Customer.io (`customer-io`)

**Catalog row:** #127 · anycli id `customer-io` · provider key `customer_io` · auth lane `api_key` · wave 2 · Marketing.
**Branches:** anycli `tool/customer-io` (this worktree), Helio `tool/customer-io` (`2helio/.claude/worktrees/tool-customer-io`).
**Status of this doc:** per-tool design scratch file (master plan §2); the batch lead strips it at batch end.

## 1. Verification against official docs (independent, not inherited)

Sources verified directly:

- Official App API OpenAPI spec (the JSON the docs site renders): https://docs.customer.io/files/journeys-app.json — full path/operation/security inventory used below.
- App API reference: https://docs.customer.io/integrations/api/app/
- Credentials management: https://docs.customer.io/accounts/settings/managing-credentials/ (key creation UI: https://fly.customer.io/settings/api_credentials?keyType=app)
- Workspaces endpoint release note: https://docs.customer.io/release-notes/2023-10-11-get-workspaces/
- API overview (Track vs App vs Pipelines): https://docs.customer.io/integrations/api/customerio-apis/

**Auth lane verdict: `api_key` CONFIRMED.** The audit row ("no viable multi-tenant path" → api_key) matches the official docs: Customer.io's Journeys APIs expose no OAuth authorization-code flow at all. The spec's only security schemes are `Bearer-Auth` ("The App API uses a bearer authentication scheme. You can generate a bearer token, known as an **App API Key** … in your account settings") and a `ServiceAccount-Auth` variant for transactional send only. The Track API uses HTTP Basic (`site_id:api_key`). No divergence on the lane.

Key API facts (all from the official spec / docs):

| Item | Value |
|---|---|
| App API base | `https://api.customer.io` (US) · `https://api-eu.customer.io` (EU) — both declared in the spec's `servers` |
| App API auth | `Authorization: Bearer <App API Key>`; keys created in Account Settings → API Credentials, **scoped to a workspace at creation**, shown once, permissions fixed at creation |
| Track API | `https://track.customer.io/api/v1` (Basic `site_id:api_key`) — a **separate credential pair**, out of scope for v1 (§1.2) |
| Verification endpoint | `GET /v1/workspaces` — Bearer-authenticated, "Returns a list of workspaces in your account", response `{"workspaces":[{"id":int,"name":string,…}]}` |
| Rate limits | App API 10 req/s general; `POST /v1/campaigns/{broadcast_id}/triggers` limited to one request every 10 s (spec-stated) |
| Transactional send | `POST /v1/send/email` with `transactional_message_id`, `to`, `identifiers`, `message_data` |
| Person search | `GET /v1/customers?email=` (exact email) and `POST /v1/customers` (attribute/segment filter) |

### 1.1 DIVERGENCES and constraints found by checking the repo code against the docs

1. **Bearer scheme vs the declarative manual-token verifier (blocks L5, not L1–L4).** `integration-service/service/manual_token_verifier.go` sets the declared header to the **raw token** (`req.Header.Set(definition.APIKey.Header, token)`), and `apiKeyManifest` carries only `header` + `setup_url` — no value scheme. Customer.io accepts only `Authorization: Bearer <key>`, so connect-time verification against `/v1/workspaces` would send `Authorization: <key>` and get 401. Same gap already recorded by the Instantly branch (its DESIGN.md §4.1): the fix is the **shared, reviewed capability** `auth.api_key.scheme: raw | bearer` (default `raw`) across `apiKeyManifest`, `model.APIKeyPolicy`, provider-gen validation, and the verifier — NOT a per-provider adapter. **Batch-lead coordination item; land once before the batch's api_key L5 sweep.** L1–L4 are unaffected (the anycli service builds its own Bearer header; the L4 seed endpoint bypasses the verifier).
2. **Single-secret constraint excludes the Track API.** The manual-credential plane stores exactly one secret per connection (`resolveManualSecret` enforces the design-317 D5 single-field schema; `credential.fields` values are a closed set where only `token.access_token` carries a secret). Customer.io's Track API needs a *second* credential pair (`site_id` + Track API key, Basic auth). Therefore v1 wraps the **App API only** (§2); Track-side writes (identify/track events, attribute updates, manual segment add/remove at `track.customer.io/api/v1/segments/{id}/add_customers`) are out of scope until the platform grows multi-secret support. This is a deliberate scope cut, not an oversight.
3. **Region (US/EU).** The App API key itself does not encode its region, and the bundle's `identity.url` is a single fixed URL. v1 ships **US-region** (`https://api.customer.io/v1/workspaces`); an EU-region account's key would fail connect-time verification with 401. The anycli service still supports `--region eu` (so L2 and the runtime work for EU once connect does), but Helio-side EU connect is a **known limitation**, same class as BoldSign's US-only note. Follow-up options (batch-lead/platform): a reviewed `identity.fallback_urls` capability or per-region verification retry. Do NOT silently retry inside a per-provider adapter.
4. **Identity stable key is the workspace name, not id.** `/v1/workspaces` returns workspace `id` as an **integer**, and `jsonPointerString` (declarative_identity.go) accepts strings only — so the bundle uses `stable_key: /workspaces/0/name` (string, human-readable label too). Risks accepted and to be verified at L2/L5: (a) App API keys are workspace-scoped at creation, so the expectation is the list contains that workspace (verify with a real key at L2 — if a workspace-scoped key still returns *all* account workspaces, index 0 ordering must be confirmed stable, or the platform needs integer-stringify support in `jsonPointerString` as another reviewed capability); (b) renaming the workspace changes the account key → reconnect creates a new connection row (acceptable; same class as login-name keys elsewhere).

## 2. What an AI teammate does with Customer.io → API surface wrapped

Customer.io is the messaging-automation home of a marketing teammate. The real jobs: look up a person and their messaging history ("did Jane get the onboarding email? why not?"), report on campaign/newsletter/broadcast performance, trigger an API broadcast to a prepared audience, manage segments, send a transactional email, and pull delivery exports for analysis. That is the App API's read-and-manage surface. Building emails (Design Studio), asset management, workspace administration, reporting webhooks, and product-side event instrumentation (Track/Pipelines) are not teammate work — excluded.

Wrapped endpoints (all present in the official spec):

| Endpoint | Why |
|---|---|
| `GET /v1/customers?email=` · `POST /v1/customers` | find a person by email; filter people by segment/attributes |
| `GET /v1/customers/{id}/attributes` | person profile detail |
| `GET /v1/customers/{id}/segments` | which segments a person is in |
| `GET /v1/customers/{id}/messages` | per-person delivery history (the "did she get it" question) |
| `GET /v1/customers/{id}/activities` | per-person activity log |
| `GET /v1/campaigns` · `GET /v1/campaigns/{id}` | campaign inventory/detail |
| `GET /v1/campaigns/{id}/metrics` (+`/links`) · `GET /v1/campaigns/{id}/journey_metrics` | campaign performance reporting |
| `GET /v1/broadcasts` · `GET /v1/broadcasts/{id}` · `GET /v1/broadcasts/{id}/metrics` | API-triggered broadcast inventory + performance |
| `POST /v1/campaigns/{broadcast_id}/triggers` | trigger a broadcast (liquid `data`, audience per UI/emails/ids/filter) |
| `GET /v1/campaigns/{broadcast_id}/triggers/{trigger_id}` (+`/errors`) | trigger status + per-recipient errors |
| `GET /v1/segments` · `POST /v1/segments` · `GET /v1/segments/{id}` · `DELETE /v1/segments/{id}` | segment inventory + manual-segment lifecycle |
| `GET /v1/segments/{id}/customer_count` · `/membership` · `/used_by` | segment size, members, dependencies |
| `GET /v1/newsletters` · `GET /v1/newsletters/{id}` · `GET /v1/newsletters/{id}/metrics` (+`/links`) | newsletter reporting |
| `GET /v1/transactional` · `GET /v1/transactional/{id}` · `GET /v1/transactional/{id}/metrics` | transactional template inventory + performance |
| `POST /v1/send/email` | send a transactional email |
| `GET /v1/messages` · `GET /v1/messages/{id}` | workspace-wide delivery search (filter by state/type/time) |
| `POST /v1/exports/customers` · `POST /v1/exports/deliveries` · `GET /v1/exports` · `GET /v1/exports/{id}` · `GET /v1/exports/{id}/download` | bulk people/delivery data for analysis |
| `GET /v1/workspaces` | workspace listing; doubles as the `whoami`-style connectivity check |

Explicitly NOT wrapped in v1: Track API (see §1.1-2), Pipelines/CDP API, Design Studio, Assets, Snippets, Collections, Subscription Center, Reporting Webhooks, Imports, ESP Suppression, Objects, Data Index, Opt-outs, `send/push|sms|in_app|inbox_message` (email is the teammate channel; add others if usage demands), newsletter/broadcast/campaign content editing (`PUT …/actions/…`, variants, translations).

## 3. anycli definition

**Stage-1 rubric: `service` type.** Customer.io ships no official CLI binary, so the `cli`-type conditions fail at the first test. Service implementation against the App API, like 21 of 23 existing definitions.

`definitions/tools/customer-io.json` (bitly is the minimal precedent):

```json
{
  "name": "customer-io",
  "type": "service",
  "description": "Customer.io messaging automation as a tool (App API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "CUSTOMERIO_APP_API_KEY"}
      }
    ]
  }
}
```

Single credential field `access_token` (matches the token-gateway projection `credential.fields.access_token: token.access_token`; harness feeds it via `ANYCLI_CRED_ACCESS_TOKEN`). The service composes `Authorization: Bearer <key>` itself. No Track API auth path — a second credential would be a silent parallel path the platform cannot fill (§1.1-2).

**Package:** `internal/tools/customerio/` (id with dashes dropped, per master plan §3 Go-package rule; `microsoft-calendar`→`microsoftcalendar` precedent). Registered at batch end as `RegisterService("customer-io", &customerio.Service{})` in `internal/tools/register.go` (shared surface — batch lead merges). Struct follows the notion/bitly shape: `BaseURL` (default `https://api.customer.io`), `HC`, `Out`, `Err`; exit codes 0 success / 1 runtime-API (typed `apiError`) / 2 usage; `--json` structured error envelope.

**Region:** persistent flag `--region us|eu` (default `us`) maps to `https://api.customer.io` / `https://api-eu.customer.io` (both official). Explicit flag, no autodetection fallback (no silent-downgrade rule). `BaseURL` override in tests takes precedence.

**Subcommand tree** (cobra, resource-grouped per notion precedent; all flags, no interactivity):

```
customer-io person search      --email <e> | --filter <json>  [--start <cursor>] [--limit N]
customer-io person get         --id <customer_id> [--id-type id|email|cio_id]
customer-io person segments    --id <customer_id> [--id-type …]
customer-io person messages    --id <customer_id> [--id-type …] [--limit N] [--start <cursor>]
customer-io person activities  --id <customer_id> [--id-type …] [--type <t>] [--limit N]
customer-io campaign list
customer-io campaign get       --id <campaign_id>
customer-io campaign metrics   --id <campaign_id> [--period hours|days|weeks|months] [--steps N] [--type <t>] [--links] [--journey]
customer-io broadcast list
customer-io broadcast get      --id <broadcast_id>
customer-io broadcast metrics  --id <broadcast_id> [--period …] [--steps N]
customer-io broadcast trigger  --id <broadcast_id> [--data <json>] [--emails <e,…> | --ids <id,…> | --per-user-data <json> | --data-file-url <url>]
customer-io broadcast status   --id <broadcast_id> --trigger <trigger_id> [--errors]
customer-io segment list
customer-io segment get        --id <segment_id> [--count] [--used-by]
customer-io segment create     --name <n> [--description <d>]
customer-io segment delete     --id <segment_id>
customer-io segment members    --id <segment_id> [--start <cursor>] [--limit N]
customer-io newsletter list
customer-io newsletter get     --id <newsletter_id>
customer-io newsletter metrics --id <newsletter_id> [--period …] [--steps N] [--links]
customer-io transactional list
customer-io transactional get  --id <transactional_id>
customer-io transactional metrics --id <transactional_id> [--period …]
customer-io send email         --transactional-id <id> --to <email> --identifier <id|email=…> [--message-data <json>] [--from <e>] [--subject <s>] [--body <html>] [--plaintext-body <t>] [--bcc <e>] [--reply-to <e>] [--disable-message-retention] [--queue-draft]
customer-io message list       [--state <s>] [--type <t>] [--start <cursor>] [--limit N]
customer-io message get        --id <message_id>
customer-io export deliveries  --newsletter <id> | --campaign <id> | --action <id> [--start <ts>] [--end <ts>] [--metric <m>]
customer-io export people      [--filter <json>]
customer-io export list
customer-io export get         --id <export_id> [--download --out <path>]
customer-io workspace list
```

**JSON output shape:** provider JSON passthrough on stdout for every JSON-returning endpoint (`{"customers":[…]}`, `{"campaigns":[…]}`, `{"metrics":…}`, trigger receipts `{"id":…}` etc.) — agents get the documented provider schema, not an invented one. `export get --download` follows the export's `download_url` and writes to `--out`, emitting a receipt `{"ok":true,"path":"…","bytes":N}` (bitly `qr image` precedent). Errors: non-2xx → typed `apiError` with status + provider error body; `--json` renders the structured error envelope; exit 1. Usage errors exit 2. Help text documents the broadcast-trigger 1-per-10s rate limit and the 10 req/s App API limit.

## 4. Helio provider bundle plan

**Naming axes (master plan §3):** ① CLI command word `customer-io` (flat provider, no group; command == tool.name) · ② anycli id `customer-io` (suffix kept — bare "customer" is generic, per §3 derivation rule) · ③ provider key `customer_io`. ②↔③ is a **mechanical dash↔underscore divergence** → one `toolToProvider` entry `"customer-io": "customer_io"` in `helio-cli/internal/toolcred/resolver.go` (batch-end shared surface; goes away if master-plan OQ6 mechanical normalization lands, but is required today).

`integrations/providers/customer_io/provider.yaml` (hidden-first; `manual_api_token`, zero service-side Go given the §1.1-1 shared `scheme: bearer` capability):

```yaml
schema: helio.provider/v1
key: customer_io
go_name: CustomerIO

presentation:
  name: Customer.io
  description_key: customer_io
  consent_domain: customer.io
  visible: false          # hidden-first; flip is the single go-live change
  order: <batch-lead assigned>

auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    scheme: bearer        # REQUIRES the shared verifier capability (§1.1-1); not per-provider code
    setup_url: https://fly.customer.io/settings/api_credentials?keyType=app

identity:
  source: userinfo
  url: https://api.customer.io/v1/workspaces   # US region; EU limitation recorded in §1.1-3
  stable_key: /workspaces/0/name               # id is an integer — unusable by jsonPointerString (§1.1-4)
  label_candidates: [/workspaces/0/name]

connection:
  mode: isolated
  disconnect_mode: local_only                  # no key-revocation API; keys are managed in the Customer.io UI
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: customer-io
  kind: api-key
```

`required_config_fields` stays empty by contract (`manual_api_token` forbids server config fields — validateConfigContract), so there are no `config/` / `deploy/` appends and **no lane-1 app registration**: nothing gates L4/L5 except the lane-2 account-pool key and the §1.1-1 verifier capability. The user's key enters only through the write-only `POST /connections/credentials` path into Vault.

Connect UX notes for the sub-doc/drawer: create the App API key in Account Settings → API Credentials (or Workspace Settings → API and webhook credentials), scope it to the target workspace, grant read permissions for people/campaigns/segments/newsletters/broadcasts/messages/exports plus send permissions for transactional and broadcast triggering (App API key permissions are fixed at creation — an under-scoped key must be re-created).

**Other Helio-side artifacts (batch-end surfaces except where noted):** icon `ui/helio-app/src/integrations/icons/customer_io.svg` + manual `providerIcons.ts` registration; AI-facing sub-doc `agents/plugins/heliox/skills/tool/customer-io.md` (key creation + scoping, person-lookup and reporting flows, broadcast-trigger rate limit, US/EU note); heliox plugin version bump + marketplace publish ride the batch. Per master plan §2: provider-gen projections are run **locally only** for L3 validation and never committed from this branch; `helio-cli/go.mod` gets a local, uncommitted `replace github.com/heliohq/anycli => <this worktree>` for the L4 build.

## 5. Test plan (five layers)

| Layer | What runs for customer-io | External credentials needed |
|---|---|---|
| L1 | `go test ./...` in anycli: httptest fakes assert method/path/query/body per verb (`GET /v1/customers?email=`, `POST /v1/customers` filter body, `POST /v1/campaigns/{id}/triggers` audience-shape variants, `POST /v1/send/email` body incl. `identifiers`, metrics `period/steps` params, export download redirect handling), `Authorization: Bearer` injection, `--region eu` base-URL switch, pagination cursor mapping, provider-JSON passthrough, `--json` error envelope, exit codes 0/1/2, missing-env fail-fast. TDD: tests first (AGENTS.md). | None |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real App API key> anycli customer-io -- workspace list`, then one of each verb group against the real API: `person search --email`, `campaign list` + `campaign metrics`, `segment list`/`create`/`delete`, `message list`, a real `send email` (transactional template, self as recipient), `broadcast trigger` on a test broadcast (mind 1/10s), `export deliveries` round trip. **Also verifies §1.1-4: what `GET /v1/workspaces` returns for a workspace-scoped key** (feeds the bundle's stable-key choice). | **Yes** — Customer.io workspace + App API key from the lane-2 account pool (free trial suffices; a transactional template and an API-triggered broadcast must be set up once in the UI) |
| L3 | Local (uncommitted) `provider-gen` + `provider-gen --check` against the branch bundle (requires the §1.1-1 `scheme` capability to exist in the generator — coordinate with batch lead); anycli suite + `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with the local `replace`; integration-service suite. Branch is expected to fail `--check` in CI until batch-end regen — do not commit local regens. | None |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with provider `customer_io`, seeding `access_token` only (non-expiring key: omit `refresh_token`/`expires_at` — no refresh cycle to exercise, per the skill's provider-class guidance). Then `heliox tool customer-io -- workspace list` and `person search --email <seeded person>` must return live data through the real token gateway. The seed path bypasses the §1.1-1 verifier, so L4 is runnable even before the `scheme: bearer` capability lands. | **Yes** — the same real App API key (lane 2); real seeded org/assistant identities in local Mongo |
| L5 | api_key key-entry sweep (agent-drivable, lane 3): open the connect link → paste the App API key through the real connect UI (`POST /connections/credentials`, verified against `https://api.customer.io/v1/workspaces` with `Authorization: Bearer`) → connection shows connected in `GET /connections` → one **unseeded** live `heliox tool customer-io -- campaign list`. **Blocked until the shared `scheme: bearer` verifier capability lands (§1.1-1).** | **Yes** — real App API key (lane 2) |

Definition of done per master plan §2: L1–L5 green, docs published, icon registered, visible flip shipped (`presentation.visible: true` + regenerate as the single go-live change). Until the flip, code-complete (hidden).
