# Mixpanel — per-tool design (`heliox tool mixpanel`)

**Batch:** Wave 2 · Analytics · row 117
**Auth lane:** `api_key` (Service Account Basic auth) — audit-confirmed
**Naming axes:** ① CLI word `mixpanel` · ② anycli id `mixpanel` · ③ provider key `mixpanel` (all identical → **no `toolToProvider` entry**, no resolver divergence)
**Tool form:** `service` type (no official non-interactive Mixpanel CLI to wrap)
**Go package:** `internal/tools/mixpanel/`
**Status:** design only. Hidden-first when implemented (`presentation.visible: false`).

---

## 1. What an AI teammate does with Mixpanel, and which API surface that maps to

Mixpanel is a product-analytics platform. An AI teammate's job against it is **read/query product analytics** — "how did signups trend last week", "what's the activation funnel conversion", "which events fire most", "pull the retention curve for the mobile cohort". It does **not** ingest events (that is the app's own instrumentation job, and uses a different credential — see §6). So this tool wraps Mixpanel's **Query API**, plus a thin slice of the **Lexicon Schemas API** (so the AI can discover what events/properties exist before querying) and the **Raw Data Export API** (bounded event pulls). Verification/identity rides the **App API** `/api/app/me`.

Official API inventory (verified against `docs.mixpanel.com/reference/overview`, redirected from `developer.mixpanel.com`):

| Mixpanel API | Host (US) | This tool uses it? | Why |
|---|---|---|---|
| **Query** | `mixpanel.com/api/query/…` | **Yes — core** | The calculated data powering the Mixpanel web app: Insights, Segmentation, Funnels, Retention, Events, Cohorts, Engage (people). This is what an AI reads. |
| **Lexicon Schemas** | `mixpanel.com/api/app/projects/…` | **Yes — discovery** | Lets the AI list event/property definitions before writing a query, instead of guessing event names. |
| **Raw Data Export** | `data.mixpanel.com/api/2.0/export` | **Yes — bounded** | Raw event stream for a date range/filter. Large & JSONL-streamed; exposed with mandatory date bounds and a documented "this can be big" caveat. |
| **App / `me`** | `mixpanel.com/api/app/me` | **Yes — verify only** | Connect-time credential verification + identity label. |
| Ingestion (`api.mixpanel.com`) | — | **No** | Writing events; uses a **project token**, not the service account. Out of scope for a read/analytics tool (§6). |
| Data Pipelines / Warehouse Connectors / GDPR / Feature Flags | — | **No** | Ops/admin surfaces outside an AI teammate's analytics workflow. |

**Region matters and is not cosmetic.** Mixpanel enforces data residency: the same logical API lives on three host families, and a US-host call against an EU-resident project simply fails. So region is a first-class credential input, not a constant:

| Region | Query / App host | Export host |
|---|---|---|
| US (default) | `mixpanel.com` | `data.mixpanel.com` |
| EU | `eu.mixpanel.com` | `data-eu.mixpanel.com` |
| India | `in.mixpanel.com` | `data-in.mixpanel.com` |

## 2. anycli definition

### 2.1 Type & id
`service` type — there is no official, non-interactive, `--json`-capable Mixpanel binary to provision into the runtime image, so we implement the HTTP surface directly (matching 21/23 existing definitions). Definition file `definitions/tools/mixpanel.json`; service registered as `RegisterService("mixpanel", &mixpanel.Service{})` in `internal/tools/register.go`; Go package `internal/tools/mixpanel/` (id has no dashes/leading digit, so package name == id).

### 2.2 Credential bindings (definition `auth.credentials`)
Mixpanel Query/Export/App APIs authenticate with **HTTP Basic auth** where the username is the service-account username and the password is the service-account secret (`--user "<username>:<secret>"`, per `docs.mixpanel.com/reference/service-accounts`). The service builds the `Authorization: Basic …` header itself and selects the host from region, so the definition injects four discrete fields as env vars and the service package owns all wire assembly:

```json
{
  "name": "mixpanel",
  "type": "service",
  "description": "Mixpanel product analytics — query Insights, Segmentation, Funnels, Retention, Events, Cohorts, People; Lexicon schemas; bounded raw export (Service Account auth)",
  "auth": {
    "credentials": [
      { "source": {"field": "service_account_username"}, "inject": {"type": "env", "env_var": "MIXPANEL_SERVICE_ACCOUNT_USERNAME"} },
      { "source": {"field": "service_account_secret"},   "inject": {"type": "env", "env_var": "MIXPANEL_SERVICE_ACCOUNT_SECRET"} },
      { "source": {"field": "project_id"},               "inject": {"type": "env", "env_var": "MIXPANEL_PROJECT_ID"} },
      { "source": {"field": "region"},                   "inject": {"type": "env", "env_var": "MIXPANEL_REGION"} }
    ]
  }
}
```

The service treats a missing username/secret/project_id as a fatal config error (exit 1); `region` defaults to `us` when empty and is validated against `{us, eu, in}`.

### 2.3 Subcommands / verbs
A resource-grouped cobra tree (notion's shape). `project_id` is injected from the credential, never a per-call flag — the AI passes only the analytical parameters.

| Command | Mixpanel endpoint | Key flags |
|---|---|---|
| `mixpanel segmentation` | `GET /api/query/segmentation` | `--event` (req), `--from`, `--to` (req, `YYYY-MM-DD`), `--on`, `--where`, `--type general\|unique\|average`, `--unit` |
| `mixpanel events` | `GET /api/query/events` | `--event` (repeatable), `--type`, `--unit`, `--interval`/`--from`/`--to` |
| `mixpanel events-names` | `GET /api/query/events/names` | top event names for autodiscovery |
| `mixpanel funnels list` | `GET /api/query/funnels/list` | saved funnels (id + name) |
| `mixpanel funnels run` | `GET /api/query/funnels` | `--funnel-id` (req), `--from`, `--to`, `--on`, `--where` |
| `mixpanel retention` | `GET /api/query/retention` | `--from`, `--to`, `--born-event`, `--event`, `--retention-type`, `--interval`, `--unit` |
| `mixpanel retention-frequency` | `GET /api/query/retention/addiction` | frequency ("addiction") view |
| `mixpanel insights` | `GET /api/query/insights` | `--bookmark-id` (req) — fetch a saved Insights report |
| `mixpanel cohorts list` | `POST /api/query/cohorts/list` | list saved cohorts (id + name) |
| `mixpanel engage` | `POST /api/query/engage` | `--where`, `--output-properties`, `--page` — query People/user profiles |
| `mixpanel lexicon list` | `GET /api/app/projects/{project_id}/schemas` | list event/property data definitions |
| `mixpanel export` | `GET /api/2.0/export` (export host) | `--from`, `--to` (req), `--event`, `--where`, `--limit` — bounded raw JSONL event export |
| `mixpanel me` | `GET /api/app/me` | account/verification echo (also the connect verifier, §4) |

**HTTP method is not uniform — verified against the official OpenAPI (`docs.mixpanel.com/reference/*`).** Most of the Query surface is `GET` with all parameters in the query string, but **`engage` and `cohorts/list` are `POST`** endpoints whose analytical parameters travel in a request **body**, not the query string. `project_id` (and optional `workspace_id`) stay in the query string even for the POST endpoints — only the analytical params move to the body. The two families:

- **GET query-string endpoints** — `segmentation`, `events`, `events-names`, `funnels`, `funnels/list`, `retention`, `retention/addiction`, `insights`, `lexicon` (schemas), `export`, `me`. Every parameter (including `project_id`) is a URL query-string parameter; the service builds a `GET` with no body. (`insights` is `GET` — re-confirmed against `docs.mixpanel.com/reference/insights-query`; `bookmark_id` and `project_id` are query-string params.)
- **POST request-body endpoints** — `engage` (`POST /api/query/engage`) and `cohorts/list` (`POST /api/query/cohorts/list`). For `engage`, `project_id` stays in the query string while `--where` / `--output-properties` / `--page` are assembled into a form-encoded (`application/x-www-form-urlencoded`) request **body** per the official spec (`output_properties` as a JSON-array field, `page` as an integer). For `cohorts/list`, `project_id` is a query-string param and the request is issued as `POST` (Mixpanel defines it POST even though the current surface exposes no body params). The service selects `GET` vs `POST` and body assembly per command; it is **not** a uniform GET query-string pass-through.

### 2.4 JSON output shape
Pass Mixpanel's JSON response through on stdout verbatim + newline (notion/bitly convention) — Query API responses are already structured JSON, regardless of whether the request was `GET` (query-string) or `POST` (request-body, per the method split above). `mixpanel export` streams JSONL (one event object per line) as Mixpanel returns it; the caller is told in help text that this is line-delimited and unbounded-by-default, hence the required date window. Errors use notion's typed envelope: a non-2xx Mixpanel response maps to `apiError` → exit **1** with `{"error":{...}}` (and `--json` renders the structured envelope); usage/parse errors exit **2**; success exits **0**. A `401`/`403` is surfaced distinctly so the host can treat it as a credential-rejection signal.

## 3. Credential fields & exact auth flow

### 3.1 Verified auth model (official docs)
- **Mechanism:** HTTP Basic auth. `curl https://mixpanel.com/api/app/me --user "<serviceaccount_username>:<serviceaccount_secret>"`. Mixpanel accepts both base64 and plaintext for the header, but we send standards-compliant base64.
- **Registration model:** a **Service Account** is a non-human Mixpanel principal created in Organization/Project settings by an Owner/Admin. It can span multiple projects within **one organization**; its per-project **role** (scope) is set explicitly per project/workspace. The secret is shown once at creation and is unrecoverable afterward.
- **Token semantics:** by default service accounts **do not expire** (an optional validity period can be set; after it lapses requests are unauthorized). There is **no refresh cycle** — this is a static credential pair, not OAuth.
- **`project_id` requirement:** the Query API requires `project_id` as a query-string parameter on every call (a service account can see several projects, so the project is not implied by the credential). We capture it once at connect time and inject it on every request.

### 3.2 Why `api_key`, and divergence check vs. the audit
The 2026-07-21 OAuth audit (row 119) kept Mixpanel `api_key`: "no viable multi-tenant path." **Official docs confirm this and I record no divergence.** Mixpanel does expose OAuth, but only for (a) SSO/login and (b) Mixpanel's own first-party integrations — there is **no self-serve, multi-tenant authorization-code OAuth client** that an arbitrary customer's project can authorize for programmatic Query/Export access. The documented programmatic path for exactly our use case (a backend/script reading analytics) is the Service Account. So `api_key` is correct; **axis ②==③==① and no `toolToProvider` mapping is added.**

### 3.3 Connect-time inputs (Helio `credential_input`)
Four fields; this is a **multi-field** credential, which is the one capability point to flag (§5):

| Field | Secret | Required | Notes |
|---|---|---|---|
| `service_account_username` | no | yes | e.g. `helio-reader.1a2b3c.mp-service-account` |
| `service_account_secret` | **yes** | yes | shown once by Mixpanel at creation |
| `project_id` | no | yes | numeric project id from Project Settings |
| `region` | no | no (default `us`) | one of `us` / `eu` / `in`; drives host selection |

`setup_url`: `https://docs.mixpanel.com/docs/orgs-and-projects/service-accounts` (how to create the service account + find project id). The secret enters through the write-only `POST /connections/credentials` API and is stored in Vault; nothing secret touches the bundle.

## 4. Helio provider bundle plan (`integrations/providers/mixpanel/provider.yaml`)

Naming: ① `tool.command` implicit `mixpanel` · ② `tool.name: mixpanel` · ③ dir/`key: mixpanel`. Flat command (no family group). `presentation.visible: false` initially.

```yaml
schema: helio.provider/v1
key: mixpanel
go_name: Mixpanel

presentation:
  name: Mixpanel
  description_key: mixpanel
  consent_domain: mixpanel.com
  visible: false            # hidden-first; flip after anycli pin + L5
  order: <unoccupied>

auth:
  type: credentials
  owner: individual
  credential_input:
    setup_url: https://docs.mixpanel.com/docs/orgs-and-projects/service-accounts
    fields:
      - {name: service_account_username, label_key: mixpanel_service_account_username, secret: false, required: true,
         placeholder: "helio-reader.1a2b3c.mp-service-account"}
      - {name: service_account_secret,   label_key: mixpanel_service_account_secret,   secret: true,  required: true}
      - {name: project_id,               label_key: mixpanel_project_id,               secret: false, required: true,
         placeholder: "3193719"}
      - {name: region,                   label_key: mixpanel_region,                   secret: false, required: false,
         placeholder: "us"}

identity:
  source: verifier          # verify the pair against /api/app/me at connect time
  url: https://mixpanel.com/api/app/me
  # account_key is human-readable and stable: the project the credential is scoped to
  stable_key: project_id
  label_candidates: [/results/name, service_account_username]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources: {selection: none, discovery: none, enforcement: none}

credential:
  fields:
    service_account_username: token.service_account_username
    service_account_secret:   token.service_account_secret
    project_id:               token.project_id
    region:                   token.region
    account_key:              connection.account_key

tool:
  name: mixpanel
  kind: api-key
```

**Adapter?** No compiled `service/adapter_*.go` — Mixpanel is a static-credential provider with no OAuth lifecycle, no revoke endpoint (disconnect is `local_only`: delete the stored credential; Mixpanel has no programmatic service-account revoke by key). It rides the declarative `manual_credentials` strategy.

## 5. The one capability dependency to flag at stage 1

Design-317 `manual_credentials` as originally shipped (mongodb) enforces a **single-secret** constraint: exactly one required field, stored directly as the token payload, and `identity.source: strategy` (no connect-time verification). Mixpanel needs **two extensions**, both already precedented by earlier Wave-2 api_key batches — Mixpanel reuses them rather than inventing anything:

1. **Multi-field `manual_credentials`** — storing a structured credential (4 fields) in the token payload and projecting each to a distinct `credential.fields` entry. Precedent: **DataForSEO** (Basic auth `login`+`password`, task #127 "integration-service capability") and HubSpot's numeric stable-key work. Region/project_id are non-secret companions to the secret.
2. **Connect-time verifier for a typed-credential provider** — `identity.source: verifier` posting the assembled Basic-auth pair to an HTTPS identity endpoint (`/api/app/me`) so a wrong secret is rejected at connect time instead of surfacing as stale first-use failure. Precedent: **Semrush / Moz** ("verifier capability", tasks #184/#199).

If both capabilities have already landed on `main` by Mixpanel's batch, Mixpanel needs **zero** new integration-service code — bundle + anycli service only. If not, the batch must add whichever is missing (multi-field storage in the seed/verify path + `verifier` identity source for `manual_credentials`), with unit tests, before Mixpanel's L4. This is the single stage-1 risk to confirm against `main` before the dev branch starts; it is not a novel capability, only a reuse.

`account_key` = `project_id` (stable, human-readable, never a hash — mongodb-host precedent); label prefers the account name from `/api/app/me`, falling back to the service-account username. `mode: isolated` so each connected Mixpanel project is its own connection.

## 6. Explicitly out of scope (v1)

- **Event ingestion** (`/import`, `/track`, `/engage#profile-set`): uses a **project token**, a different credential from the service account, and is the app's instrumentation job, not an AI-teammate analytics action. Not wired.
- **GDPR data-retrieval, Warehouse Connectors, Data Pipelines, Feature Flags admin:** ops surfaces, not analytics reads.
- If a future need for writes appears, it is a separate credential field + command group, designed then — not smuggled into the read tool.

## 7. Test plan — five layers

| Layer | What runs | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in `internal/tools/mixpanel/`: httptest fakes for `/api/query/segmentation`, `/events`, `/funnels(/list)`, `/retention`, `/insights`, `/cohorts/list`, `/engage`, `/api/app/projects/*/schemas`, `data.*/export`, `/api/app/me`. Assert: host chosen per `region`, `Authorization: Basic base64(user:secret)` header, `project_id` query param present on every query call, JSONL passthrough for export, and both plaintext + `--json` error envelopes for 401/403/4xx/5xx. Exit-code contract 0/1/2. | **No** |
| **L2** harness real API | `make build-harness` then `ANYCLI_CRED_SERVICE_ACCOUNT_USERNAME=… ANYCLI_CRED_SERVICE_ACCOUNT_SECRET=… ANYCLI_CRED_PROJECT_ID=… ANYCLI_CRED_REGION=us anycli mixpanel -- segmentation --event "…" --from … --to …` against a real Mixpanel project. Mandatory before pin bump — proves field names, Basic-auth assembly, host/region, and `project_id` injection match the live API. | **Yes** — real service account + a project with events (account pool) |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (five projections); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service unit suite (incl. any §5 capability tests). On-branch: local `replace` in `helio-cli/go.mod` → anycli branch; local regen not committed (batch lead owns the canonical regen). | **No** |
| **L4** singleton + seed | Singleton in `env: dev`; `POST /internal/test-only/connections/seed` with the 4-field Mixpanel credential (multi-field seed — the §5 capability dependency; confirm the seed write path stores all four), then `heliox tool mixpanel -- me` and one real query. Proves token-gateway → anycli path serves the structured credential and reaches the live API. Non-expiring credential ⇒ seed the pair only, no `refresh_token`/`expires_at`. | **Yes** — same real service account seeded |
| **L5** connect flow (api_key key-entry path) | Hidden tool, before visible flip: open the connect link → enter username/secret/project_id/region through the real connect UI (stored via `POST /connections/credentials`, **verified against `/api/app/me`**) → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool mixpanel` query through the real token gateway succeeds. Agent-drivable (api_key L5 lane); human fallback on UI breakage. | **Yes** — real service account (account pool) |

**Externally-supplied credentials needed:** L2, L4, L5 — one real Mixpanel Service Account (username + secret) plus a `project_id` for a project that has event data, US region for the pool default. Mixpanel's free tier supports service accounts, so the account-pool lane can procure this without a paid plan.

## 8. Definition of done (per master plan §2)

L1–L5 green · AI-facing sub-doc published under `agents/plugins/heliox/skills/tool/` (new `mixpanel` page: the verb table, that `project_id`/region are connection-time not per-call, and export's bounded-JSONL caveat) + plugin version bump (batch-end) · UI icon `ui/helio-app/src/integrations/icons/mixpanel.svg` + `providerIcons.ts` registration · then the visible flip (`presentation.visible: true` + regenerate) as the single go-live change. No `toolToProvider` entry (axes identical). Until the flip: code-complete (hidden).

---

### Sources
- Service accounts / Basic auth: https://docs.mixpanel.com/reference/service-accounts · https://docs.mixpanel.com/docs/orgs-and-projects/service-accounts
- API overview, hosts, data residency: https://docs.mixpanel.com/reference/overview
- Segmentation query (path, `project_id`, params, rate limits): https://docs.mixpanel.com/reference/segmentation-query
- Query API overview: https://docs.mixpanel.com/reference/query-api
