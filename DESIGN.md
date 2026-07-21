# Mixpanel — per-tool design (`heliox tool mixpanel`)

**Batch:** Wave 2 · Analytics · row 117
**Auth lane:** `api_key` (Service Account Basic auth) — audit-confirmed
**Naming axes:** ① CLI word `mixpanel` · ② anycli id `mixpanel` · ③ provider key `mixpanel` (all identical → **no `toolToProvider` entry**, no resolver divergence)
**Tool form:** `service` type (no official non-interactive Mixpanel CLI to wrap)
**Go package:** `internal/tools/mixpanel/`
**Status:** design only. Hidden-first when implemented (`presentation.visible: false`).

---

## 1. What an AI teammate does with Mixpanel, and which API surface that maps to

Mixpanel is a product-analytics platform. An AI teammate's job against it is **read/query product analytics** — "how did signups trend last week", "what's the activation funnel conversion", "which events fire most", "pull the retention curve for the mobile cohort". It does **not** ingest events (that is the app's own instrumentation job, and uses a different credential — see §6). So this tool wraps Mixpanel's **Query API**, plus a thin slice of the **Lexicon Schemas API** (documented event/property schemas — a discovery *aid*, not the full inventory; see the caveat below) and the **Raw Data Export API** (bounded event pulls). `mixpanel events-names` (GET `/api/query/events/names`) is the **primary event-name discovery primitive** — it returns the actively-firing event names; lexicon supplements it with descriptions/metadata where authors created schemas.

**Event discovery uses `events-names`, not lexicon.** Mixpanel's Lexicon Schemas API returns **only entities that have an authored schema** (created via the Schema API, CSV import, or UI metadata edits) — it is explicitly "a subset of the data that appears in Lexicon UI," and per Mixpanel's docs "entities seen in your Lexicon UI that were sent within the last 30 days without a Schema will not be reflected in the Lexicon Schema payload." So a project that never authored schemas returns a **partial or empty** list even though events are actively firing. An AI must not read an empty/short lexicon as "this project has no events." The authoritative event-name source is `mixpanel events-names`; lexicon is a documentation overlay only. This caveat is stated in the AI-facing sub-doc (§8).

**Identity is derived from the credential, not from a connect-time verifier.** `account_key`/label come from the stored fields (`project_id`, `service_account_username`) — see §4 — so there is no static identity URL, and no region-scoped verifier to get wrong (Finding-1 resolution). The `mixpanel me` verb still exists as a **runtime** identity/auth probe the AI or user can run right after connecting (it is region-aware like every other call); a wrong secret surfaces there as a distinct `401` (§2.4).

Official API inventory (verified against `docs.mixpanel.com/reference/overview`, redirected from `developer.mixpanel.com`):

| Mixpanel API | Host (US) | This tool uses it? | Why |
|---|---|---|---|
| **Query** | `mixpanel.com/api/query/…` | **Yes — core** | The calculated data powering the Mixpanel web app: Insights, Segmentation, Funnels, Retention, Events, Cohorts, Engage (people). This is what an AI reads. |
| **Lexicon Schemas** | `mixpanel.com/api/app/projects/…` (region-scoped) | **Yes — documentation overlay** | Lists **only** event/property definitions that have an authored schema (subset of the Lexicon UI; schemaless-but-active events are omitted). A supplement to `events-names`, not the discovery primitive. |
| **Raw Data Export** | `data.mixpanel.com/api/2.0/export` | **Yes — bounded** | Raw event stream for a date range/filter. Large & JSONL-streamed; exposed with mandatory date bounds and a documented "this can be big" caveat. |
| **App / `me`** | `mixpanel.com/api/app/me` (region-scoped) | **Yes — runtime probe** | Runtime identity/auth check (region-aware); **not** a connect-time verifier — identity is credential-derived (§4). |
| Ingestion (`api.mixpanel.com`) | — | **No** | Writing events; uses a **project token**, not the service account. Out of scope for a read/analytics tool (§6). |
| Data Pipelines / Warehouse Connectors / GDPR / Feature Flags | — | **No** | Ops/admin surfaces outside an AI teammate's analytics workflow. |

**Region matters and is not cosmetic.** Mixpanel enforces data residency: the same logical API lives on three host families, and a US-host call against an EU-resident project simply fails. This applies to **every** surface — Query, Export, **and the App API** (`/api/app/me`, Lexicon Schemas), all confirmed region-scoped against `docs.mixpanel.com/reference/overview`. So region is a first-class credential input, not a constant:

| Region | Query / App host | Export host |
|---|---|---|
| US (default) | `mixpanel.com` | `data.mixpanel.com` |
| EU | `eu.mixpanel.com` | `data-eu.mixpanel.com` |
| India | `in.mixpanel.com` | `data-in.mixpanel.com` |

Because the App API is *also* region-scoped, host selection cannot be a static string anywhere — including for `me`/`lexicon`. The region-aware **anycli service** builds the host from `MIXPANEL_REGION` for **all** surfaces uniformly (Query, Export, App). This is exactly why identity is credential-derived rather than fetched from a fixed verifier URL (§4/§5): a single declarative `identity.url` cannot honor residency, so we don't use one.

**Rate limits (verified, `docs.mixpanel.com/reference/segmentation-query`):** the Query API caps at **60 queries/hour** with **max 5 concurrent queries**. An AI teammate firing successive analytics calls will hit this readily, so `429` is surfaced as a **distinct, retryable** signal (with `Retry-After` when present) rather than a permanent failure (§2.4), and the limits are documented in the AI-facing sub-doc (§8).

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
| `mixpanel events-names` | `GET /api/query/events/names` | **primary event-name discovery** — actively-firing event names (use this, not lexicon, to learn what events exist) |
| `mixpanel funnels list` | `GET /api/query/funnels/list` | saved funnels (id + name) |
| `mixpanel funnels run` | `GET /api/query/funnels` | `--funnel-id` (req), `--from`, `--to`, `--on`, `--where` |
| `mixpanel retention` | `GET /api/query/retention` | `--from`, `--to`, `--born-event`, `--event`, `--retention-type`, `--interval`, `--unit` |
| `mixpanel retention-frequency` | `GET /api/query/retention/addiction` | frequency ("addiction") view |
| `mixpanel insights` | `GET /api/query/insights` | `--bookmark-id` (req) — fetch a saved Insights report |
| `mixpanel cohorts list` | `POST /api/query/cohorts/list` | list saved cohorts (id + name) |
| `mixpanel engage` | `POST /api/query/engage` | `--where`, `--output-properties`, `--page` — query People/user profiles |
| `mixpanel lexicon list` | `GET /api/app/projects/{project_id}/schemas` | list **authored** event/property schemas only (documentation overlay — omits schemaless active events; not the discovery primitive, see §1) |
| `mixpanel export` | `GET /api/2.0/export` (export host) | `--from`, `--to` (req), `--event`, `--where`, `--limit` — bounded raw JSONL event export |
| `mixpanel me` | `GET /api/app/me` | runtime identity/auth probe (region-aware); **not** a connect-time verifier — identity is credential-derived (§4) |

**HTTP method is not uniform — verified against the official OpenAPI (`docs.mixpanel.com/reference/*`).** Most of the Query surface is `GET` with all parameters in the query string, but **`engage` and `cohorts/list` are `POST`** endpoints whose analytical parameters travel in a request **body**, not the query string. `project_id` (and optional `workspace_id`) stay in the query string even for the POST endpoints — only the analytical params move to the body. The two families:

- **GET query-string endpoints** — `segmentation`, `events`, `events-names`, `funnels`, `funnels/list`, `retention`, `retention/addiction`, `insights`, `lexicon` (schemas), `export`, `me`. Every parameter (including `project_id`) is a URL query-string parameter; the service builds a `GET` with no body. (`insights` is `GET` — re-confirmed against `docs.mixpanel.com/reference/insights-query`; `bookmark_id` and `project_id` are query-string params.)
- **POST request-body endpoints** — `engage` (`POST /api/query/engage`) and `cohorts/list` (`POST /api/query/cohorts/list`). For `engage`, `project_id` stays in the query string while `--where` / `--output-properties` / `--page` are assembled into a form-encoded (`application/x-www-form-urlencoded`) request **body** per the official spec (`output_properties` as a JSON-array field, `page` as an integer). For `cohorts/list`, `project_id` is a query-string param and the request is issued as `POST` (Mixpanel defines it POST even though the current surface exposes no body params). The service selects `GET` vs `POST` and body assembly per command; it is **not** a uniform GET query-string pass-through.

### 2.4 JSON output shape
Pass Mixpanel's JSON response through on stdout verbatim + newline (notion/bitly convention) — Query API responses are already structured JSON, regardless of whether the request was `GET` (query-string) or `POST` (request-body, per the method split above). `mixpanel export` streams JSONL (one event object per line) as Mixpanel returns it; the caller is told in help text that this is line-delimited and unbounded-by-default, hence the required date window. Errors use notion's typed envelope: a non-2xx Mixpanel response maps to `apiError` → exit **1** with `{"error":{...}}` (and `--json` renders the structured envelope); usage/parse errors exit **2**; success exits **0**. Two HTTP statuses are surfaced **distinctly** from the generic `4xx` bucket, because the AI must react differently to each:

- **`401`/`403`** → a `credential` error kind — a credential-rejection signal (wrong/expired secret, or the service account lacks project role). Permanent until the credential is fixed.
- **`429`** → a `rateLimit` error kind — the Query API caps at **60 queries/hour, 5 concurrent** (verified, `docs.mixpanel.com/reference/segmentation-query`), which an AI firing successive analytics queries hits readily. The envelope carries the `Retry-After` value when Mixpanel returns one (the `error` object includes `retry_after_seconds`) so the host can **back off and retry** rather than treat the call as permanently failed. This is a `transient` failure, not a dead one.

Both still exit **1** (they are `apiError`s), but the distinct error kind lets the host/AI choose "fix the credential" vs. "wait and retry" vs. "give up."

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
  source: strategy          # credential-derived — no connect-time network call, no static host (mongodb-precedented)
  # account_key = the project the credential is scoped to; label = the service-account username.
  # Both are declared here from named INPUT fields. IMPORTANT: source:strategy / no-verifier is the
  # mongodb precedent, but declarative input-field-sourced stable_key/label is a NEW integration-service
  # capability — mongodb declares NEITHER (it derives account_key inside strategy code from its single
  # packed secret). See §5 dependency (a). project_id is numeric → also §5 dependency (b).
  stable_key: project_id            # from the project_id input field (numeric → §5 (b))
  label: service_account_username   # from the username input field

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

## 5. Integration-service dependencies to confirm at stage 1

**Precedent, corrected.** Design-317 `manual_credentials` as shipped (mongodb) enforces a **single-secret** shape: exactly one `credential_input` field, stored directly as the token payload (`token.access_token`), with `identity.source: strategy` (credential-derived, no connect-time network call) and **no** declarative `stable_key`/`label` — mongodb derives its `account_key` (the DSN host) *inside strategy code* from the single packed secret. The only other `manual_*` api_key bundle in this program, **DataForSEO**, is *also* single-field (login:password packed into one `api_credentials` field → one `token.access_token`) and additionally uses `manual_api_token` + `identity.source: userinfo` (verify-first), **not** `source: strategy`; it is not on `main` (it lives in a parallel worktree). So **neither landed bundle is multi-field, and neither sources `stable_key`/`label` from named input fields.** The shape §4 proposes — **multi-field storage** (4 discrete `credential.fields` projections) with **`stable_key`/`label` sourced from named input fields** — therefore has **no landed precedent**. It is a **new** integration-service capability. An earlier draft's "already precedented by DataForSEO / HubSpot" framing was false and is corrected here.

**Subtract-before-adding: the zero-new-capability alternative (mongodb pattern).** Per the repo's mandatory rule, the landed alternative is evaluated first. mongodb's pattern applied to Mixpanel: collect the credential as **one** packed secret field, store it as the single `token.access_token`, and have the anycli service parse the four values out of it — with the strategy deriving `account_key = project_id` exactly as mongodb derives `account_key = DSN host` from its packed secret. This needs **zero** new integration-service capability and **zero** numeric-stable-key work (account_key is a strategy-derived *string*, never a declaratively-typed key). It is the pure-reuse baseline, and it is the fallback if the batch prefers strict subtraction.

**Why this design chooses multi-field anyway (explicit rationale, not default reuse).** Two Mixpanel-specific reasons, weighed against the mongodb pattern:

1. **No combined artifact to paste.** mongodb's single field works because the Atlas console hands the user one copy-pasteable DSN. Mixpanel exposes username, secret, project_id, and region as **four separate values in different UI locations** with no combined string, so a single packed field forces the user to hand-assemble a synthetic, order-sensitive `username:secret:project_id:region` string — error-prone. Four labeled inputs match how Mixpanel actually presents the credential.
2. **Secret hygiene.** `project_id` and `region` are **non-secret**. The mongodb-pack stores them inside the secret blob (mislabeling non-secret data as secret, hidden from the connection view). Multi-field stores them as non-secret labeled fields, so `account_key = project_id` and the region are inspectable in `GET /connections` without decrypting a secret.

These are genuine UX/hygiene wins, not a reflex to keep the nicer shape. If the batch weights strict subtraction higher, fall back to the mongodb single-secret pack above — it ships with no integration-service change.

**Two distinct new capabilities, each its own stage-1 confirmation item.** They are **orthogonal** — do not treat either as "free" because the other landed:

- **(a) Multi-field `manual_credentials` + input-field-sourced `stable_key`/`label`.** Store a structured 4-field credential in the token payload, project each value to its own `credential.fields` entry, and source `stable_key`/`label` from named input fields (`project_id` / `service_account_username`). **No landed precedent** (mongodb and DataForSEO are both single-field). If not on `main` at stage 1, the batch implements + unit-tests it (multi-field storage in the seed/credential write path + input-field `stable_key`/`label` for `manual_credentials`) **before** Mixpanel's L4.
- **(b) Numeric `stable_key` for `project_id`.** Mixpanel's `project_id` is numeric, so a declarative `stable_key: project_id` requires the integration-service to accept a **numeric** stable key. This is the **HubSpot** capability (numeric `hub_id` stable key) and is orthogonal to (a): it concerns the *value type* of the stable key, not multi-field storage or input-field sourcing. HubSpot's numeric-stable-key work is **not yet on `main`** (parallel worktree); confirm its landing status at stage 1, and if unlanded it is a separate prerequisite the batch must land, with unit tests, before Mixpanel's L4. (The mongodb-pattern fallback dodges (b) entirely, since account_key would be a strategy-derived string.)

**Confirm both (a) and (b) against `main` at stage 1.** If both are already landed, Mixpanel needs bundle + anycli service only. If either is missing, the batch lands it — with unit tests — before Mixpanel's L4. The chosen shape is multi-field (rationale above); the mongodb single-secret pack remains the documented zero-capability fallback.

**Why no connect-time verifier — the deliberate design cut (Finding-1 / Finding-2 resolution).** An earlier draft used `identity.source: verifier` posting to a fixed `https://mixpanel.com/api/app/me`. That is unshippable and was removed, for two independent reasons verified against Mixpanel's own docs:

1. **Residency defect.** The App API (`/api/app/me`, Lexicon Schemas) is region-scoped — EU service accounts must hit `eu.mixpanel.com/api/app/me`, India `in.mixpanel.com/api/app/me` (confirmed, `docs.mixpanel.com/reference/overview`). A single declarative `identity.url` cannot interpolate the `region` input field, so a static-host verifier would silently target the **wrong host** for every EU/India project — by the design's own residency argument (§1), those calls fail. Making the verifier region-aware would require a *third* capability (verifier-host templating from an input field) that no cited precedent covers, and it would be **untestable** under a US-only account pool (the defect would ship uncaught). We do not add region host-templating; we remove the need for it.
2. **Undocumented response shape.** Mixpanel does not publish the `/api/app/me` response schema (checked `docs.mixpanel.com/reference/service-accounts` and the OpenAPI index — no field list). So the earlier `label_candidates: /results/name` pointer was an unverifiable assumption. Sourcing `label` from a known **input field** (`service_account_username`) is deterministic and needs no response contract.

Removing the verifier honors §1's residency argument **more** completely (region is now honored on *every* surface at runtime, including `me`/`lexicon`, via the region-aware anycli service — not just query/export), and it is strictly subtractive: one fewer integration-service capability, no static host, no assumption about an undocumented body. The cost — a wrong secret is not caught at connect time — matches the **mongodb `source: strategy` precedent** exactly and is mitigated because (a) `mixpanel me` is an explicit post-connect auth probe the AI/user can run immediately, and (b) a bad secret surfaces as a **distinct `401`** (§2.4), not a silent misread.

`account_key` = `project_id` (stable, human-readable, never a hash — mongodb-host precedent); `label` = the service-account username, both from the connect-time input. `mode: isolated` so each connected Mixpanel project is its own connection. **Region (us/eu/in) is fully supported in v1** — the region-aware anycli service builds the host from `MIXPANEL_REGION` for all surfaces, and L1 unit tests assert host selection for all three families (see §7), so EU/India correctness is proven without a live non-US credential.

## 6. Explicitly out of scope (v1)

- **Event ingestion** (`/import`, `/track`, `/engage#profile-set`): uses a **project token**, a different credential from the service account, and is the app's instrumentation job, not an AI-teammate analytics action. Not wired.
- **GDPR data-retrieval, Warehouse Connectors, Data Pipelines, Feature Flags admin:** ops surfaces, not analytics reads.
- If a future need for writes appears, it is a separate credential field + command group, designed then — not smuggled into the read tool.

## 7. Test plan — five layers

| Layer | What runs | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in `internal/tools/mixpanel/`: httptest fakes for `/api/query/segmentation`, `/events`, `/events/names`, `/funnels(/list)`, `/retention`, `/insights`, `/cohorts/list`, `/engage`, `/api/app/projects/*/schemas`, `data.*/export`, `/api/app/me`. Assert: **host is built from `region` for all three families** — `us`→`mixpanel.com`/`data.mixpanel.com`, `eu`→`eu.mixpanel.com`/`data-eu.mixpanel.com`, `in`→`in.mixpanel.com`/`data-in.mixpanel.com` — across query, export, **and** app (`me`/`lexicon`) surfaces (this is where EU/India correctness is proven without a live non-US credential); `Authorization: Basic base64(user:secret)` header; `project_id` query param present on every query call; POST-body assembly for `engage`/`cohorts/list`; JSONL passthrough for export. Error envelopes: plaintext + `--json` for `401`/`403` (`credential` kind) **and `429` (`rateLimit` kind, `retry_after_seconds` populated from `Retry-After`)** distinct from the generic `4xx`/`5xx` bucket. Exit-code contract 0/1/2. | **No** |
| **L2** harness real API | `make build-harness` then `ANYCLI_CRED_SERVICE_ACCOUNT_USERNAME=… ANYCLI_CRED_SERVICE_ACCOUNT_SECRET=… ANYCLI_CRED_PROJECT_ID=… ANYCLI_CRED_REGION=us anycli mixpanel -- segmentation --event "…" --from … --to …` against a real Mixpanel project. Mandatory before pin bump — proves field names, Basic-auth assembly, host/region, and `project_id` injection match the live API. | **Yes** — real service account + a project with events (account pool) |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (five projections); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service unit suite (incl. any §5 capability tests). On-branch: local `replace` in `helio-cli/go.mod` → anycli branch; local regen not committed (batch lead owns the canonical regen). | **No** |
| **L4** singleton + seed | Singleton in `env: dev`; `POST /internal/test-only/connections/seed` with the 4-field Mixpanel credential (multi-field seed — the §5 capability dependency; confirm the seed write path stores all four and derives `account_key = project_id` via `source: strategy`), then `heliox tool mixpanel -- me` (the auth probe) and one real query. Proves token-gateway → anycli path serves the structured credential and reaches the live API. Non-expiring credential ⇒ seed the pair only, no `refresh_token`/`expires_at`. | **Yes** — same real service account seeded |
| **L5** connect flow (api_key key-entry path) | Hidden tool, before visible flip: open the connect link → enter username/secret/project_id/region through the real connect UI (stored via `POST /connections/credentials`; **identity is credential-derived — no connect-time verifier call**, so a correct-shape entry connects immediately) → connection shows connected/configured with `account_key = project_id` in `GET /connections` → one **unseeded** live `heliox tool mixpanel -- me` (auth check) + one query through the real token gateway succeeds; a deliberately-wrong secret is confirmed to surface as a **distinct `401`** on that first call (not a silent connect). Agent-drivable (api_key L5 lane); human fallback on UI breakage. | **Yes** — real service account (account pool) |

**Externally-supplied credentials needed:** L2, L4, L5 — one real Mixpanel Service Account (username + secret) plus a `project_id` for a project that has event data, US region for the pool default. Mixpanel's free tier supports service accounts, so the account-pool lane can procure this without a paid plan.

## 8. Definition of done (per master plan §2)

L1–L5 green · AI-facing sub-doc published under `agents/plugins/heliox/skills/tool/` (new `mixpanel` page: the verb table; that `project_id`/region are connection-time not per-call; export's bounded-JSONL caveat; **use `events-names` — not `lexicon` — for event-name discovery, and never read an empty/short `lexicon` result as "no events" since it lists only authored schemas**; and the **Query API rate limits — 60 queries/hour, max 5 concurrent — with guidance to back off on a `429`/`rateLimit` error rather than retry immediately or give up**) + plugin version bump (batch-end) · UI icon `ui/helio-app/src/integrations/icons/mixpanel.svg` + `providerIcons.ts` registration · then the visible flip (`presentation.visible: true` + regenerate) as the single go-live change. No `toolToProvider` entry (axes identical). Until the flip: code-complete (hidden).

---

## 9. Implementation divergence — single-secret packed credential (recorded per master plan §2)

**As-designed:** §2.2/§4/§5 specify **four discrete credential fields**
(`service_account_username`, `service_account_secret`, `project_id`, `region`),
each injected as its own env var and **projected to its own `credential.fields`
entry**, with `stable_key`/`label` sourced from named input fields — flagged in
§5 as a **new** integration-service capability the batch would land.

**As-built:** a **single packed JSON credential** (`token.access_token`) —
i.e. the mongodb single-secret pack that §5 documented as the sanctioned
zero-capability fallback. Verified against the actual repo code (the
instruction's "check the repo yourself, nothing is exempt" mandate), the
multi-field shape is **more invasive than §5 framed**, on three counts the
design under-weighted:

1. **Closed `CredentialSource` allowlist.** `model/catalog.go` defines a closed
   allowlist of token-gateway sources; the only token-derived one is
   `token.access_token`. Four per-field projections would require **adding four
   entries to a security-sensitive closed vocabulary** plus token-gateway
   plumbing — not "projection config."
2. **D5 single-secret invariant.** `model.validateCredentialInputSchema` and the
   `RuntimeStrategyManualCredentials` runtime contract **assert exactly one
   required field** (design 317 D5/D8, "no new CredentialSource"). Multi-field
   relaxes a *documented, asserted* invariant.
3. **`manualTokenVerifier.Verify(secret string)`** takes a single secret; the
   whole manual-credentials path is single-secret end to end.

Given the repo's **subtract-before-adding** hard rule, and that a sibling batch
tool (**amplitude**, same analytics/api_key lane) concurrently landed the
**deriver-selection** mechanism for exactly this packed-secret shape, the
packed-JSON fallback is the correct, lowest-conflict, allowlist-respecting
choice. It is functionally equivalent: `account_key = project_id` is still
surfaced (via `mixpanelCredentialDeriver` → `connection.account_key`,
human-readable, never a hash), and identity is still credential-derived with no
connect-time verifier (the §5 Finding-1/Finding-2 residency argument holds
unchanged). Net effect on §5: **both new capabilities (a) and (b) are dropped**
— (b) numeric `stable_key` disappears entirely because a form-field value is
always a string, and (a) collapses to one deriver + one-line deriver selection.

Consequent shape changes (supersede §2.2 and §4 where they conflict):

- **anycli definition:** one binding — `credentials` → `MIXPANEL_CREDENTIALS`
  (a JSON object `{username, secret, project_id, region}`); the service parses
  the four values (region defaults to `us`). Region-aware host selection,
  Basic-auth assembly, GET/POST split, JSONL export, and typed
  `credential`/`rateLimit` error kinds are all unchanged.
- **bundle:** one required secret `credentials` field with a JSON placeholder;
  `identity.source: strategy`; `credential.fields: {credentials:
  token.access_token, account_key: connection.account_key}`.
- **integration-service:** `mixpanelCredentialDeriver` (JSON → account_key =
  project_id, label = username, secret excluded from identity) selected by
  `manualCredentialsDeriver(definition)`.

**Trade-off accepted:** the connect form is one JSON field rather than four
labeled inputs, and `region` is inside the secret blob rather than an
inspectable non-secret field. The AI-facing sub-doc + the field placeholder
carry the exact JSON shape, and `project_id` remains inspectable as
`account_key`.

---

### Sources
- Service accounts / Basic auth: https://docs.mixpanel.com/reference/service-accounts · https://docs.mixpanel.com/docs/orgs-and-projects/service-accounts
- API overview, hosts, data residency (Query/Export/**App API** all region-scoped): https://docs.mixpanel.com/reference/overview
- Segmentation query (path, `project_id`, params, rate limits — 60/hr, 5 concurrent): https://docs.mixpanel.com/reference/segmentation-query
- Lexicon (Schemas API returns only authored schemas — "subset of the data that appears in Lexicon UI"): https://docs.mixpanel.com/docs/data-governance/lexicon
- Query API overview: https://docs.mixpanel.com/reference/query-api
