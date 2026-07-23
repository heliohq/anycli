# Tool design: Instantly (`instantly`)

Per-tool design for the 300-integrations rollout (master plan row 72, Wave 2,
Sales Engagement). Scratch file on `tool/instantly`; the batch lead strips it
at batch end.

- **Product**: Instantly (instantly.ai) — cold-email outreach: campaigns,
  sending accounts + warmup, lead management, unified inbox (Unibox), email
  verification, deliverability analytics.
- **Naming axes**: ① CLI command `instantly` (flat, no group) · ② anycli id
  `instantly` · ③ provider key `instantly`. ② == ③, so **no
  `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.
  Go package: `internal/tools/instantly/`.
- **Catalog row**: `| 72 | Instantly | instantly | instantly | api_key | 2 |
  Sales Engagement |` — verified consistent with the naming rules (§3).

## 1. Official API surface (verified against developer.instantly.ai)

Source of truth: https://developer.instantly.ai/ (docs index at
`/llms.txt`), OpenAPI spec https://api.instantly.ai/openapi/api_v2.json
(126 paths, fetched and inspected 2026-07-21).

- **API v2 only.** v1 is deprecated (planned removal 2025) and shares the
  rate-limit budget with v2; we target v2 exclusively. Base URL
  `https://api.instantly.ai`, all paths under `/api/v2/`.
- **Auth**: Bearer-token API keys. Official authorization guide
  (`/getting-started/authorization.md`): header `authorization: Bearer {{key}}`.
  Keys are created in the dashboard (Settings → Integrations → API Keys,
  https://app.instantly.ai/app/settings/integrations), displayed **once**,
  non-recoverable, and **scoped** at creation (per-resource scopes like
  `campaigns:read`, plus wildcards `all:read` … `all:all`). No expiry
  documented; revocation is per-key from the dashboard or
  `DELETE /api/v2/api-keys/{id}`.
- **No third-party OAuth exists.** The `OAuth` endpoint group in the API
  (`/api/v2/oauth/google/init`, `/microsoft/init`, `/session/status/:id`) is
  for connecting Google/Microsoft **mailboxes into Instantly** — it is not an
  authorization path for external apps. The audit verdict ("no viable
  multi-tenant path; stays api_key") is **confirmed** by the official docs.
- **Rate limits** (`/getting-started/rate-limit.md`): workspace-wide across
  all keys and both API versions — 100 req/s and 6,000 req/min; 429 on
  breach. Endpoint exceptions: `GET /api/v2/emails` 20 req/min; test email
  send 10 req/min; AI-label test 500 req/30 days.
- **Pagination**: `limit` + `starting_after` cursor on list endpoints; list
  responses carry `items` + `next_starting_after`.
- **REST deviation**: `POST /api/v2/leads/list` — lead listing is a POST
  because of its complex filter body (documented deviation, not a mistake).
- **Long-running ops** return background jobs polled via
  `GET /api/v2/background-jobs/:id` (bulk lead moves, warmup enable/disable).
- **Plan gate (procurement note)**: the official docs pages do not state a
  plan requirement for API keys, but third-party sources consistently report
  API v2 requires the Hypergrowth plan or higher. Lane-2 test-account
  procurement must budget for a paid workspace; verify at signup.

## 2. What an AI teammate does with Instantly → wrapped surface

An AI GTM/sales teammate: monitors and reports on campaign performance,
manages leads (add, move, update interest after replies), triages and replies
to Unibox responses, watches sending-account health/warmup and deliverability,
and verifies email addresses before adding them. It does **not** buy DFY
domains, delete workspaces, manage workspace members, or rotate API keys.

Wrapped endpoint groups (all under `/api/v2/`):

| Verb group | Endpoints | Why |
|---|---|---|
| `campaign` | `GET/POST /campaigns`, `GET/PATCH /campaigns/{id}`, `POST /campaigns/{id}/activate`, `POST /campaigns/{id}/pause`, `GET /campaigns/{id}/sending-status`, `GET /campaigns/analytics`, `/analytics/overview`, `/analytics/daily`, `/analytics/steps` | Core object; reporting + start/stop are the highest-frequency asks |
| `lead` | `POST /leads/list` (list), `GET/PATCH/DELETE /leads/{id}`, `POST /leads` (create), `POST /leads/add` (bulk ≤1000 to campaign/list), `POST /leads/move` (background job), `POST /leads/update-interest-status` | Pipeline upkeep after replies; bulk import into campaigns |
| `lead-list` | `GET/POST /lead-lists`, `GET/PATCH/DELETE /lead-lists/{id}`, `GET /lead-lists/{id}/verification-stats` | Staging area for leads before campaign assignment |
| `email` | `GET /emails`, `GET /emails/{id}`, `POST /emails/reply` (uses `reply_to_uuid`), `GET /emails/unread/count`, `POST /emails/threads/{thread_id}/mark-as-read` | Unibox triage + reply — the human-inbox half of the loop |
| `account` | `GET /accounts`, `GET /accounts/{email}`, `POST /accounts/{email}/pause`, `/resume`, `POST /accounts/warmup-analytics`, `GET /accounts/analytics/daily` | Sender health / warmup / deliverability monitoring |
| `verify` | `POST /email-verification`, `GET /email-verification/{email}` (poll `pending`) | Hygiene before adding leads |
| `job` | `GET /background-jobs`, `GET /background-jobs/{id}` | Poll bulk-operation completion |
| `api` | generic `instantly api <METHOD> <path> [--body/--query]` escape hatch (notion `api.go` precedent, auth-header override rejected) | Covers the long tail (subsequences, custom tags, block list, webhooks, inbox placement) without bloating the tree |

Deliberately excluded from first-class verbs: DFY email-account orders
(spends money), workspace removal/owner-change/members/groups (destructive
admin), API-key management (credential self-modification), SuperSearch
enrichment (consumes paid credits; revisit on demand), webhooks (Helio has no
inbound path for them). All remain reachable via `api` if a human directs it.

## 3. anycli definition (stage 1–2)

**Type: `service`** per the stage-1 rubric — Instantly ships no official CLI
at all, so the `cli`-type conditions fail at the first test. HTTP
implementation in `internal/tools/instantly/`, registered as
`RegisterService("instantly", &instantly.Service{})` in
`internal/tools/register.go` (registration rides the batch-end merge; the
definition JSON + package merge freely mid-batch).

`definitions/tools/instantly.json`:

```json
{
  "name": "instantly",
  "type": "service",
  "description": "Instantly cold-email platform (campaigns, leads, Unibox, warmup analytics)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "INSTANTLY_API_KEY"}
      }
    ]
  }
}
```

Implementation shape copies `internal/tools/notion/` (the reference) and
`internal/tools/bitly/` (Bearer construction): a `Service` struct with
`BaseURL`/`HC`/`Out`/`Err` for httptest injection; every request sets
`Authorization: Bearer $INSTANTLY_API_KEY`; exit codes 0 success / 1
runtime-API failure (typed `apiError`) / 2 usage errors; `--json` structured
error envelope; fail fast with a clear message when `INSTANTLY_API_KEY` is
unset.

**JSON output**: raw provider JSON passthrough — single objects as returned;
list commands pass through the `{ "items": [...], "next_starting_after": "…" }`
envelope with `--limit` / `--starting-after` flags mapped to the query params
(and to the POST body for `lead list`). No re-shaping: Instantly's v2 is
already strict snake_case REST, and passthrough keeps the tool honest with
provider docs. 429s surface as `apiError` with the status; no client-side
rate limiting in v1 of the tool.

## 4. Helio provider bundle (stage 4)

`integrations/providers/instantly/provider.yaml` — hidden-first:

```yaml
schema: helio.provider/v1
key: instantly
go_name: Instantly

presentation:
  name: Instantly
  description_key: instantly
  consent_domain: instantly.ai
  visible: false        # flip only after L5 key-entry sweep passes
  order: <unoccupied at batch end>

auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization        # value must be "Bearer <key>" — see §4.1
    setup_url: https://app.instantly.ai/app/settings/integrations

identity:
  source: userinfo
  url: https://api.instantly.ai/api/v2/workspaces/current
  stable_key: /id
  label_candidates: [/name, /id]

connection:
  mode: isolated
  disconnect_mode: local_only    # Instantly has no key-revoke-by-itself API we should call
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
  name: instantly
  kind: api-key
```

Identity endpoint choice: `GET /api/v2/workspaces/current` takes no
parameters, resolves the workspace from the key itself, and returns a
`Workspace` object with string `id` (stable key) and `name` (label) —
verified in the OpenAPI schema. Requires the key to carry `workspaces:read`
(or `all:read`/`all:all`); the provider sub-doc and connect-drawer setup link
must tell users to include it. `required_config_fields` is empty by contract
(`manual_api_token` forbids server config fields), so no `config/` / `deploy/`
appends and no lane-1 app registration — nothing gates L4/L5 except the
account-pool key and §4.1.

### 4.1 DIVERGENCE — verifier cannot send `Bearer` today (blocks L5, not L4)

`integration-service/service/manual_token_verifier.go` line 34 sets the
declared header to the **raw token**:
`req.Header.Set(definition.APIKey.Header, token)`. Instantly accepts only
`Authorization: Bearer <key>`, so connect-time verification against
`/workspaces/current` fails 401 with the current declarative verifier, and
`apiKeyManifest` (`header`, `setup_url` only) cannot express a value scheme.

Per the provider-yaml guidance ("grow the generic capability set one reviewed
enum value instead" of a per-provider adapter), the right fix is a **shared
integration-service change**, not an Instantly adapter:

- Add `auth.api_key.scheme: raw | bearer` (default `raw`, preserving the
  synthetic `acme` fixture and any raw-header provider) to `apiKeyManifest`,
  `model.APIKeyPolicy`, provider-gen validation, and the verifier
  (`bearer` → `"Bearer " + token`).
- This is not Instantly-specific: a large share of the 160 api_key-lane tools
  are Bearer-style. **Batch-lead coordination item** — land once, before the
  batch's first api_key L5 sweep.

Scope of the blockage: L1–L4 are unaffected (the anycli service builds the
Bearer header itself, and the L4 seed endpoint writes through
`writeUserTokenCredential` without invoking the manual verifier). Only the
real connect path (L5 key entry → verification) needs the capability.

## 5. Batch-end shared-surface items

- `register.go`: `RegisterService("instantly", …)`.
- anycli tag + `helio-cli/go.mod` pin bump (local uncommitted `replace` for
  on-branch validation only).
- One `provider-gen` run — five projections committed together by the batch
  lead; **not** committed from this branch.
- No `toolToProvider` entry (② == ③).
- Icon: `ui/helio-app/src/integrations/icons/instantly.svg` + manual
  `providerIcons.ts` append.
- Docs: provider sub-doc `agents/plugins/heliox/skills/tool/instantly.md`
  (key creation path, required scopes incl. `workspaces:read`, rate-limit
  cautions, verb reference); plugin version bump + marketplace publish ride
  the batch.

## 6. Test plan (five layers)

| Layer | What runs | External credentials? |
|---|---|---|
| L1 | `go test ./...` in anycli: httptest fakes assert method/path/query/body per verb, `Authorization: Bearer` header injection, pagination flag mapping, `leads/list` POST body, `--json` error envelope, exit codes 0/1/2, missing-env fail-fast | No |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<key> anycli instantly -- campaign list`, then one of each verb group against the **real** API (incl. `verify` pending→done poll and a `job get` on a bulk op). Mind 20 req/min on `email list` | **Yes** — real Instantly API key from the lane-2 account pool (paid-plan workspace; see §1 plan gate) |
| L3 | Local (uncommitted) `provider-gen` + `provider-gen --check` against the branch bundle; `helio-cli` build + `go test ./cmd/heliox/cmds/tool/` with local `replace` to this worktree; integration-service suite | No |
| L4 | Singleton; seed via `POST /internal/test-only/connections/seed` with `provider: instantly`, `access_token: <real key>` — **access_token only, no refresh_token/expires_at** (non-expiring key, Slack-bot-token pattern); then `heliox tool instantly -- campaign list` must return live data through the real token gateway | **Yes** — same real key |
| L5 | api_key key-entry sweep (master plan §2 checklist): connect link → paste key in the real UI (`POST /connections/credentials`) → verification against `/workspaces/current` → connection visible in `GET /connections` → one **unseeded** live run. Agent-drivable via agent-browser; human fallback | **Yes** — real key; **blocked on the §4.1 bearer-scheme verifier capability** |

Definition of done follows the master plan: L1–L5 green, docs published,
icon registered, then `visible: true` + regen as the single go-live change.
