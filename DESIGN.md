# Tool design: Hunter.io (`hunter`)

Per-tool scratch design for the 300-integrations rollout (master plan
`docs/design/008-300-integrations-rollout-plan.md`, catalog row 70, Wave 2,
Sales Engagement). Branches: anycli `tool/hunter`, Helio `tool/hunter`.
This file is stripped by the batch lead at batch end.

## 0. Catalog row, audit verdict, and independent verification

| Axis | Value |
|---|---|
| ① CLI command word | `hunter` (flat, no group) |
| ② anycli tool id | `hunter` (definitions/tools/hunter.json) |
| ③ provider catalog key | `hunter` (integrations/providers/hunter/) |
| Auth lane | `api_key` |

All three axes are identical, so **no `toolToProvider` entry** is needed in
`helio-cli/internal/toolcred/resolver.go` (identity holds; the plan's 24 new
entries are the dashed ids only — `hunter` is not one of them).

**Audit verdict verified against official docs — no divergence.** The oauth
audit row 70 says "no viable multi-tenant path / stays api_key". Confirmed
independently from https://hunter.io/api-documentation/v2: Hunter's API
authenticates exclusively with per-account API keys — passed as the `api_key`
query parameter, the `X-API-KEY` header, or `Authorization: Bearer <key>`.
There is no third-party authorization-code OAuth flow of any kind (no app
registry, no consent screen), so the `api_key` lane stands. Registration
model: any Hunter account (free tier included) creates keys self-serve in the
dashboard at https://hunter.io/api-keys (multiple keys per account are
supported); a key is a long-lived bearer secret with no scopes and no expiry —
it grants the account's full API surface and is revoked only by deleting it in
the dashboard. Quotas are per-account (searches vs. verifications pools,
monthly `reset_date`), not per-key.

## 1. What an AI teammate does with Hunter.io, and the API surface wrapped

Hunter is a prospecting/enrichment utility. The realistic agent jobs are:

1. **Find someone's email** — "get me the email of Jane Doe at stripe.com"
   (Email Finder), "list emails at that company" (Domain Search), "how many
   emails does Hunter have for this domain?" (Email Count).
2. **Verify an email before outreach** — "is this address deliverable?"
   (Email Verifier).
3. **Enrich a contact/company** — "who is behind this email / what's this
   company?" (Person / Company / Combined Enrichment), "what's the domain for
   company X?" (Domain Finder).
4. **Build prospect lists** — "find SaaS companies in France" (Discover),
   then save/manage prospects (Leads + Leads Lists CRUD).
5. **Check quota** — "how many searches do I have left?" (Account).

Wrapped endpoints (base `https://api.hunter.io/v2`), all verified against the
official v2 reference:

| Subcommand | Method + path | Notes |
|---|---|---|
| `domain-search` | GET `/domain-search` | 1 credit / 1–10 emails; free-plan `limit+offset<=10` |
| `email-finder` | GET `/email-finder` | domain/company + name or linkedin_handle |
| `email-verifier` | GET `/email-verifier` | may return **202** while verification runs (see §2 exit contract) |
| `email-count` | GET `/email-count` | free |
| `domain-finder` | GET `/domain-finder` | beta, free; company name → domain |
| `discover` | POST `/discover` | company search: natural-language `query` or structured filters |
| `enrich person` | GET `/people/find` | by email or linkedin_handle |
| `enrich company` | GET `/companies/find` | by domain |
| `enrich combined` | GET `/combined/find` | person + company |
| `lead list/get/create/update/delete` | GET/POST/PUT/DELETE `/leads[/:id]` | free CRUD |
| `lead-list list/get/create/update/delete` | GET/POST/PUT/DELETE `/leads_lists[/:id]` | free CRUD |
| `account` | GET `/v2/account` | free; plan, searches/verifications used vs available, reset_date |

**Deliberately out of v1:**
- **Email Sequences (Campaigns) API** — an outreach-execution surface gated on
  paid all-in-one plans; low agent value relative to the finding/verifying
  core, and its CRUD shapes could not be fully verified from the public doc.
  Add later as a `campaign` group if demand appears.
- **Saved Discover views** (`/discover/views`) and **custom attributes** —
  dashboard-management surfaces, not agent tasks.
- **API-keys management endpoints** — Helio must never manage the user's keys.
- **`POST /domain-search` (location filter)** and Discover premium filters
  beyond passthrough — v1 documents the limitation instead of forking the
  request shape; `discover --filters` raw-JSON passthrough still reaches every
  filter the account's plan allows.

## 2. anycli definition (stage-1 rubric: `service` type)

`cli` type fails the rubric immediately — Hunter ships no official CLI. So:
**`service` type**, package `internal/tools/hunter/` (id has no dashes; Go
package name == id), registered as `RegisterService("hunter", &hunter.Service{})`
in `internal/tools/register.go` (registration is written on this branch for
L1/L2; the registry file itself is a batch-end shared surface the batch lead
merges).

`definitions/tools/hunter.json`:

```json
{
  "name": "hunter",
  "type": "service",
  "description": "Hunter.io email finding, verification, and lead enrichment (API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "HUNTER_API_KEY"}
      }
    ]
  }
}
```

The credential field name `api_key` matches the Helio bundle's
`credential.fields` key (§4) — the token gateway's provider-neutral
`credential` map is keyed by these names.

### Service shape (bitly is the api-key-adjacent precedent to copy)

`Service{BaseURL, HC, Out, Err}` struct so tests point at `httptest.Server`;
cobra tree with `SilenceUsage/SilenceErrors`; exit codes 0 success / 1
runtime-API failure / 2 usage-parse errors; provider JSON emitted to stdout
verbatim + newline (`--json` accepted for uniformity, always on).

- **Auth on the wire: `X-API-KEY: <key>` header on every request** — never the
  `api_key` query parameter (keys must not leak into logs/URLs), matching the
  header the Helio bundle declares for verification.
- **Error dialect**: non-2xx with envelope
  `{"errors":[{"id","code","details"}]}` (verified live: bad key →
  401 `authentication_failed`). Error helper extracts `errors[0]`
  `details`/`id`, falls back to raw body. **401 → `execution.RejectCredential`**
  (stale-credential feedback loop), everything else non-2xx → plain error.
  Notable Hunter quirk to document in the sub-doc: **403 = rate limited,
  429 = monthly quota exhausted** (inverted from common conventions) — both
  are plain errors, not credential rejections.
- **Email Verifier 202**: a 202 is inside the 2xx success passthrough — emit
  the body verbatim, exit 0. The body's `data.status` tells the agent the
  verification is still running; re-polling the same command is the documented
  protocol and costs one request. No client-side polling loop (agents decide).
- Subcommand naming mirrors the official endpoint names (`domain-search`,
  `email-finder`, …) so the agent can map Hunter's docs 1:1; the only grouping
  is `enrich person|company|combined` (three `/…/find` endpoints, one verb)
  and `lead` / `lead-list` CRUD nouns.
- Flag → query-param mapping is mechanical kebab→snake (`--first-name` →
  `first_name`; `--linkedin-handle` → `linkedin_handle`). `lead list` filter
  flags pass through Hunter's `*`/`~`/string filter semantics as raw strings.
  `discover` takes `--query` (natural language) and/or `--filters` (raw JSON
  object merged into the POST body); `lead create/update` take explicit flags
  for the common fields plus `--attributes` raw-JSON for the rest.

File layout (~1 file per resource group, bitly-style):
`hunter.go` (root + Execute), `client.go` (call/emit/error helpers),
`search.go` (domain-search, email-count, domain-finder), `finder.go`
(email-finder), `verifier.go` (email-verifier), `discover.go`, `enrich.go`,
`lead.go`, `leadlist.go`, `account.go` — each with its `_test.go`.

## 3. Credential fields and auth flow (verified)

- **One credential field**: the API key. No client id/secret, no refresh, no
  expiry, no scopes. `auth.required_config_fields` is empty — an api_key
  provider needs zero Helio-side server config, so the bundle is safe to ship
  hidden with nothing in `config/`/`deploy/` (no lane-1 dependency at all).
- **Connect flow** (existing `manual_api_token` machinery, zero new service
  code): user pastes the key into the connect form (implicit single-token
  default schema — no `credential_input` block needed); integration-service's
  `declarativeManualTokenVerifier` GETs the bundle's identity endpoint with
  the bundle's header before any Vault write; the key is stored as the user
  token payload and projected to the token gateway as `token.access_token`.
- **Identity verification endpoint**: `GET https://api.hunter.io/v2/account`
  with `X-API-KEY: <key>` — free of charge, returns
  `{"data":{"email","first_name","last_name","plan_name","reset_date",
  "requests":{"searches":{...},"verifications":{...}},…}}`; invalid key →
  401 `authentication_failed` (verified live). Stable key `/data/email`
  (account email is the account identity; keys are per-account), label
  `/data/email`.
- **Disconnect**: `local_only` — Hunter has no token-revocation API, and
  deleting the user's dashboard-managed key on their behalf would be wrong.

## 4. Helio provider bundle plan

`integrations/providers/hunter/provider.yaml` (axis ③ = directory name =
`hunter`), hidden-first:

```yaml
schema: helio.provider/v1
key: hunter
go_name: Hunter

presentation:
  name: Hunter
  description_key: hunter
  consent_domain: hunter.io
  # Hidden-first (master plan §2): flip gate = anycli pin ships the hunter
  # tool, icon + i18n land, and L5 key-entry sweep passes. Pick an unoccupied
  # presentation.order at flip time.
  visible: false

auth:
  type: api_key
  owner: individual
  api_key:
    header: X-API-KEY
    setup_url: https://hunter.io/api-keys

identity:
  source: userinfo
  url: https://api.hunter.io/v2/account
  stable_key: /data/email
  label_candidates: [/data/email]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_key: token.access_token
    account_key: connection.account_key

tool:
  name: hunter
  kind: api-key
```

Notes:
- `tool.command` omitted → defaults to `hunter` (axis ①). No `tool.group` —
  independent brand, flat command.
- `kind: api-key` is the wire-compat value (mongodb precedent).
- This revives the currently-dormant `manual_api_token` runtime strategy
  (integration-service AGENTS.md: "no live provider" since Figma moved to
  MCP) — hunter becomes its first live provider; the `acme` synthetic tests
  already cover the path, so no service code changes are expected.
- Batch-end shared surfaces this tool touches: `register.go` entry, anycli
  pin bump, one `provider-gen` run (5 projections — **run locally for
  validation only, never committed on this branch**), icon
  `ui/helio-app/src/integrations/icons/hunter.svg` + `providerIcons.ts`
  append, i18n `tools.desc.hunter`, sub-doc
  `agents/plugins/heliox/skills/tool/hunter/hunter.md` + plugin version bump.
  No `toolToProvider` entry, no OAuth config append (lane 1 not involved).

## 5. Test plan (five layers)

| Layer | What runs | External credential needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes asserting `X-API-KEY` header injection, query/body shape per subcommand, JSON passthrough, 401→RejectCredential, 403/429 plain-error rendering, 202 verifier passthrough, exit codes | none |
| L2 | `make build-harness`; `ANYCLI_CRED_API_KEY=<real key> anycli hunter -- account` then `-- domain-search --domain stripe.com`, `-- email-verifier --email …` against the live API | **YES — real Hunter API key from the account pool** (free tier: 25 searches / 50 verifications per month — enough; the documented `test-api-key` dummy key exercises domain-search/email-finder/email-verifier without burning quota but does NOT replace the mandatory real-key run) |
| L3 | local `go run ./cmd/provider-gen` + `--check` against this branch's bundle (regens NOT committed; branch expected red in CI until batch-end regen), integration-service + helio-cli suites, heliox built with a local **uncommitted** `go.mod replace` pointing at this anycli worktree | none |
| L4 | singleton, `POST /internal/test-only/connections/seed` with `provider: "hunter"` and `access_token: <real key>` — **seed `access_token` only, no `refresh_token`/`expires_at`** (non-expiring key, no refresh cycle) — then unmocked `heliox tool hunter -- account` through the real token gateway | **YES — same real key** |
| L5 | api_key key-entry sweep (agent-drivable per master plan §2 lane 3): open connect link → paste key through the real connect UI (`POST /connections/credentials`, verified against `GET /v2/account`) → connection shows connected in `GET /connections` → one **unseeded** live `heliox tool hunter -- account` | **YES — same real key**; runs post-batch-merge, gates the visible flip |

L1 is written first (TDD per anycli AGENTS.md) — every service file lands with
its failing test before implementation.

## 6. Open items / risks

- Free-tier quota is small (25 searches/mo); L2+L4+L5 should prefer the free
  `account`, `email-count`, `domain-finder`, and `lead` endpoints for repeated
  runs and spend paid-credit calls (`domain-search`, `email-finder`) once each.
- `/data/email` as stable key means two keys of the same Hunter account
  upsert the same connection `(org, assistant, provider, account_key)` —
  correct behavior (quota and identity are account-level).
- Discover filter surface is plan-gated (premium filters, Data Platform
  `no_discover_access` 403); v1 passes filters through and surfaces Hunter's
  own error rather than pre-validating plans client-side.
