# Tool design: Lusha

Scratch design doc for the `tool/lusha` batch branch (stripped by the batch lead
at batch-end). Scope: one external tool provider behind `heliox tool`, per the
`helio-tool-provider` pipeline. Master plan catalog row **69** (Lusha), Wave 2,
category *Sales Engagement*.

Naming axes (all three identical — no divergence):

| Axis | Value |
|---|---|
| ① CLI command word | `lusha` |
| ② anycli tool id | `lusha` |
| ③ provider catalog key | `lusha` |

Go package (stage-2 rule): `internal/tools/lusha/` (id has no dashes, no leading
digit). No `toolToProvider` entry is needed — id == key, so `ProviderFor`
resolves mechanically.

---

## 1. Auth lane verification (official docs vs catalog/audit)

**Catalog row 69:** `api_key`. **OAuth audit row 71:** stays `api_key` — "no
viable multi-tenant path."

**Verified against official docs — the lane holds, no divergence to record.**
Lusha authenticates every request with an account-level key passed in a custom
`api_key` **header** (not `Authorization: Bearer`). The key is issued in the
Lusha API Hub to Admins/Managers on Premium and Scale plans; several keys can
coexist per account. There is no authorization-code OAuth flow — no
`authorize`/`token` endpoints, no per-user consent, no registered multi-tenant
app that arbitrary customer accounts could authorize. This is exactly the audit
rubric's "no viable multi-tenant path," so Lusha correctly sits in the
`api_key` lane. (Sources: docs.lusha.com "All there is to know about Lusha's
API", docs.lusha.com/guides.)

Consequence for the bundle: user-supplied secret through the write-only
`POST /connections/credentials` API into Vault; `tool.kind: api-key`; and a
manual-secret `runtime_strategy` whose exact enum value is **provisional** until
stage-2 confirms the reviewed strategy set on `integration-service` main (see
finding-3 resolution in §4). The only shipped manual precedent on this branch is
mongodb's `runtime_strategy: manual_credentials` (design 317); a
`manual_api_token` verify-before-write strategy (the Figma-style `X-Figma-Token`
+ `/v1/me` shape from design 227's provider-extension contract) is **not present
on this branch** and must not be assumed. So the strategy is `manual_api_token`
**only if** a shared verifier capability exists; otherwise it is
`manual_credentials`.

---

## 2. API surface wrapped, and why

**Base URL:** `https://api.lusha.com/v3/` (V3 is the current production version;
V2 is operational but deprecating — target V3 only). All requests HTTPS, JSON
in/out, key in the `api_key` header.

**What an AI teammate actually does with Lusha** (Sales Engagement): it already
holds a partial lead — an email, a LinkedIn URL, or a name + company — and needs
the missing coordinates (verified email, direct/mobile phone, title,
firmographics) to act; or it needs to *generate* net-new contacts/companies
matching an ICP and then reveal the good ones; or it needs to check remaining
credits before spending them on a costly prospecting run. Two discovery paths
feed one reveal step, plus a usage check:

| Group / verb | Method + path | Why it's in scope |
|---|---|---|
| `contact enrich` | `POST /v3/contacts/search-and-enrich` | One-shot known-identifier path: turn a real-world identifier (email / linkedinUrl / firstName+lastName+companyName\|companyDomain) into revealed emails + phones in a single call. Drop-in replacement for legacy `GET /v2/person`. Charges twice (api_search + reveal). |
| `contact search` | `POST /v3/contacts/prospecting` | ICP prospecting: filter by job title, seniority, department, location, company size/revenue/industry/technologies/intent/signals → net-new contact `id`s + a `requestId` (name-only preview, no email/phone). Generates leads the assistant did not already have. Charged per result via `api_search`. |
| `contact reveal` | `POST /v3/contacts/enrich` | **The reveal step for prospecting.** Takes up to 100 Lusha contact `ids` (from a `contact search` result) + a `reveal` selector (`emails` / `phones`, omit = both) → full revealed records. Charged per revealed datapoint only (no api_search) — this is the credit-efficient path that makes "search 500 cheaply, reveal only the 80 that matter" possible. Without it, `contact search` returns `id`s that no verb could reveal. |
| `company enrich` | `POST /v3/companies/search-and-enrich` | One-shot: domain / name → firmographics (size, revenue, industry, technologies). Drop-in for legacy `GET /v2/company`. **No `reveal` selector** — the `V3CompaniesSearchAndEnrichRequest` schema is `{companies[], options}` only; companies have no emails/phones, firmographics are returned by default. Charged per successful result via the `reveal_company` action. |
| `company search` | `POST /v3/companies/prospecting` | Same ICP filter model, company-level → company `id`s + `requestId`. Charged `api_search` per result. |
| `company reveal` | `POST /v3/companies/enrich` | Reveal step for company prospecting: up to 100 company `ids` (from a `company search` result) → full firmographics. Optional `reveal` selector is the **firmographic-expansion** enum `employeesByDepartment\|employeesByLocation\|employeesBySeniority\|competitors\|intent` (NOT `emails`/`phones` — that enum belongs only to the contact verbs); omit = base firmographics. Charged per successful result via the `reveal_company` action. |
| `account usage` | `GET /v3/account/usage` | Credits used/remaining/total, plan, rate limits, per-action pricing. Agent-natural pre-flight check before a credit-heavy sweep — and the provider-side **verifier** endpoint (see §4). |

**Divergence recorded (review vs official V3 spec).** The review finding assumed
the reveal endpoint "requires a `requestId` + `contactIds[]` plus
`revealEmails`/`revealPhones` flags." The authoritative V3 OpenAPI bundle
(`docs.lusha.com/_bundle/apis/@v3/openapi.yaml`, schema `V3ContactsEnrichRequest`
/ `V3CompaniesEnrichRequest`) shows the request body is `{ "ids": [<id>…] (1–100,
required), "reveal": ["emails"|"phones"] (optional, omit = both) }` — there is
**no `requestId` in the enrich request** and no `revealEmails`/`revealPhones`
booleans. `requestId` is a **response-only** field (returned by search /
prospecting / enrich for traceability), so the reveal verb needs only the `id`s.
Per the "follow official docs" rule, the verb is modeled on `ids` + `reveal`, and
the search-verb envelope surfaces `request_id` as informational context, not as a
required input to reveal.

**Deliberately deferred (v1):** Signals, Lookalikes, the identifier-based
two-step `POST /v3/contacts/search` + `/v3/companies/search` (the ICP-filter
`prospecting` discovery already covers the "generate net-new" use case; the
identifier-search variant is redundant with `search-and-enrich` for v1), and the
Subscriptions (webhook) endpoints under the `/api/` prefix. Webhooks require a
Helio-hosted callback sink and a subscription lifecycle an AnyCLI passthrough
tool has no place to deliver to; the rest are narrower and can be added as verbs
later without reshaping the tool. Bulk (up to 100 identifiers/ids per call) is
supported by the enrich, reveal, and search-and-enrich endpoints and is exposed
via repeatable `--id` / identifier flags rather than a separate verb.

**Billing note surfaced to the AI (docs, not code) — billing differs by
entity, so state it per endpoint, not as one blanket "enrich = per-datapoint"
rule:**

- **contact reveal** (`POST /v3/contacts/enrich`) → charged **per revealed
  datapoint** (each email or phone), no `api_search`. If `canReveal.credits` is
  0 the datapoint is already revealed and re-enriching it is free.
- **contact enrich** (`POST /v3/contacts/search-and-enrich`) → **two** charges
  per result: one `api_search` + one per revealed datapoint.
- **company reveal** (`POST /v3/companies/enrich`) → charged **per successful
  company result** via the `reveal_company` action — a per-result charge, **not**
  per-datapoint (companies expose no emails/phones to meter individually).
- **company enrich** (`POST /v3/companies/search-and-enrich`) → charged **per
  successful result** via `reveal_company` (same meter as company reveal).
- **`contact search` / `company search`** (`prospecting`) → `api_search` per
  result.

So the credit-efficient contact prospecting flow is **`contact search` (cheap
`api_search` sweep) → filter to ICP → `contact reveal` on the chosen `id`s
(reveal-only, per-datapoint charge)** — never revealing the rows you discard.
The same "reveal only the `id`s that matter" reasoning holds for companies, but
there the reveal charge is per surviving company result, not per datapoint. The
`account usage` verb exists so the assistant can reason about spend (and read
per-action pricing). This goes in the AI-facing sub-doc, not the definition.

---

## 3. anycli definition

**Stage-1 form decision: `service` type.** No official Lusha CLI binary exists;
the integration is a plain REST API. Follows the 21-of-23 default. The service
implementation lives in `internal/tools/lusha/`, registered as
`RegisterService("lusha", &lusha.Service{})` in `internal/tools/register.go`.

**Definition (`definitions/tools/lusha.json`):**

```json
{
  "name": "lusha",
  "type": "service",
  "description": "Lusha B2B contact & company enrichment and prospecting (API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "LUSHA_API_KEY"}
      }
    ]
  }
}
```

The single credential field is `api_key` (matches the resolver-supplied field
name the Helio bundle projects; §4). The service reads `LUSHA_API_KEY` from the
env and sends it as the `api_key` request header — anycli never sees Vault or
the token gateway; it only injects the field the host resolves.

**Service tree** (cobra, resource-grouped, mirroring `internal/tools/notion/`
shape — `BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest`
server; exit codes 0 success / 1 runtime+API failure via typed `apiError` /
2 usage-parse; `--json` structured error envelope):

```
lusha contact enrich   --email | --linkedin-url | --name+--company[-domain]      [--reveal emails,phones]
lusha contact search   --title --seniority --location --industry --company-size … [--page N --size 50]
lusha contact reveal   --id <lusha-id>… (1–100, repeatable)                       [--reveal emails,phones]
lusha company enrich   --domain | --name                                          (no --reveal)
lusha company search   --industry --location --size --revenue --technology …      [--page N --size 50]
lusha company reveal   --id <lusha-id>… (1–100, repeatable)                        [--reveal employeesByDepartment,employeesByLocation,employeesBySeniority,competitors,intent]
lusha account usage
```

`enrich` owns the real-world-identifier one-shot (`search-and-enrich`); `reveal`
owns the ID-based batch reveal (`/v3/{contacts,companies}/enrich`) that consumes
the `id`s a prior `search` returned. Keeping them separate is the orthogonal
split: "I have an email/LinkedIn/name" vs "I have Lusha `id`s from a search."
Both `search` verbs' output must surface those `id`s (plus `request_id`) so the
assistant can feed `reveal`.

**The `--reveal` flag is per-entity, not shared** (verified against the V3
OpenAPI bundle — see the correction note below):

- **Contact verbs** (`contact enrich`, `contact reveal`) — `--reveal` selects
  the PII to reveal: enum `emails,phones`, omit = both. This is the only reveal
  axis the two contact verbs carry.
- **`company enrich`** (`companies/search-and-enrich`) — **no `--reveal` flag at
  all.** Its request schema `V3CompaniesSearchAndEnrichRequest` is
  `{companies[], options}` with no `reveal` field; firmographics come back by
  default. Emitting a reveal selector here would send a field the endpoint
  ignores.
- **`company reveal`** (`companies/enrich`) — `--reveal` is the optional
  **firmographic-expansion** enum
  `employeesByDepartment|employeesByLocation|employeesBySeniority|competitors|intent`;
  omit = base firmographics. It is **not** `emails`/`phones` (companies have no
  contact PII to reveal), so passing `emails`/`phones` here returns a 400
  invalid-enum. Because the expansion enum is niche, the flag is optional and
  most calls omit it.

**Correction (prior draft vs official V3 spec).** An earlier draft applied the
contact-only `--reveal emails,phones` selector to both company verbs. The V3
OpenAPI bundle (`docs.lusha.com/_bundle/apis/@v3/openapi.yaml`, and the
`companies/enrich` reference) contradicts this: `companies/search-and-enrich`
has no `reveal` field, and `companies/enrich`'s `reveal` enum is the
firmographic-expansion set above, never `emails`/`phones`. Per the "follow
official docs" rule the reveal model is split by entity as documented here.

**JSON output shape.** Every verb accepts `--json` and emits a stable envelope
so the assistant parses deterministically regardless of Lusha's nesting:

- `enrich` verbs → `{"data": {<revealed contact|company object>}, "meta":
  {"credits_charged": <n>, "request_id": "…"}}`. Lusha's search-and-enrich
  response carries preview scaffolding (`canReveal`); the service flattens the
  revealed record into `data` and drops preview-only fields.
- `reveal` verbs → `{"data": [<revealed objects>], "meta": {"credits_charged":
  <n>, "request_id": "…"}}` (Lusha returns a `results` array of up to 100
  revealed records + a `billing.creditsCharged` total).
- `search` verbs → `{"data": [<preview objects>], "meta": {"page": N,
  "total": M, "has_more": bool, "request_id": "…"}}`. Preview objects carry the
  Lusha `id` (+ name-only fields, no email/phone) — the assistant collects the
  `id`s of the rows it wants and calls the matching `reveal` verb on them. The
  `id` and `request_id` MUST appear in the envelope; without the `id`s the
  reveal step is impossible.
- `account usage` → `{"data": {"credits": {"used", "remaining", "total"},
  "plan": "…", "pricing": {…}}}` (fields passed through as returned).
- Errors (all verbs) → exit 1 with `{"error": {"code": "…", "message": "…"}}`;
  a `401` from Lusha maps to an auth-class error message pointing the assistant
  at reconnecting.

---

## 4. Helio provider bundle plan (`integrations/providers/lusha/provider.yaml`)

Hidden-first (`presentation.visible: false`). Manual-secret (api_key) shape. The
`runtime_strategy` enum is **provisional** (see the verify/identity decision
below): `manual_api_token` (Figma-style verify-before-write) *if* a shared
verifier capability exists on `integration-service` main, else
`manual_credentials` (the mongodb no-verify precedent, design 317 — the only
shipped manual strategy on this branch). Lusha *does* offer a clean HTTPS verify
signal, so `manual_api_token` is preferred if available.

Planned bundle (final field names pinned against `provider-gen`'s closed schema
during stage-2/5; shape below):

```yaml
schema: helio.provider/v1
key: lusha
go_name: Lusha

presentation:
  name: Lusha
  description_key: lusha
  consent_domain: lusha.com
  visible: false            # flip only after L1–L5 green + anycli pin ships lusha + icon + 9 locales

auth:
  type: api_key
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: lusha_api_key
        secret: true
        required: true
    setup_url: https://docs.lusha.com/user-guide/lushas-api/all-there-is-to-know-about-lushas-api

identity:
  source: strategy          # see verify/identity note below

connection:
  mode: isolated
  disconnect_mode: local_only
  # PROVISIONAL — pin in stage-2 against provider-gen's closed strategy enum:
  #   manual_api_token   if a shared verifier capability exists on integration-service main
  #   manual_credentials otherwise (mongodb precedent, design 317 — the only shipped manual strategy on this branch)
  runtime_strategy: manual_api_token   # or manual_credentials — see verify/identity note

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_key: token.access_token   # single secret via existing UpsertUserToken write path
    account_key: connection.account_key

tool:
  name: lusha
  kind: api-key
```

**Verify + identity — the one real design decision (and what pins the
`runtime_strategy` enum).** *If* a `manual_api_token`-style verifier capability
exists on `integration-service` main, it auto-composes *verify-before-write*: a
provider-declared header + an HTTPS identity endpoint hit at connect time, so a
bad key is rejected before it reaches Vault. Lusha's natural verify endpoint is
`GET /v3/account/usage` (`200` on a valid key, `401` on an invalid/missing one).
This strategy is **not confirmed present on this branch** (`manual_api_token`
appears in no shipped provider bundle here; only mongodb's `manual_credentials`
does), so stage-2 must verify the exact strategy enum in provider-gen before
treating it as fixed — do not fork a parallel verifier capability; grow the
shared set or fall back to option 2 below.

**Path confirmed (minor-finding resolution).** The V3 migration guide renders
this endpoint loosely as `account/usage` ("No change" from V2, no `/v3` shown),
which raised an off-by-version risk — a wrong verify path would turn every
connect into a false `401`/reject (the `manual_api_token` verify-before-write
gate). The authoritative V3 OpenAPI bundle
(`docs.lusha.com/_bundle/apis/@v3/openapi.yaml`) lists the path as
**`GET /v3/account/usage`** (operationId `getAccountUsage`, `securitySchemes:
ApiKeyAuth` = `api_key` header, `401` Unauthorized on bad key). So the DESIGN's
pinned `/v3/account/usage` is correct; L2 still re-probes it live (it is
credit-free) before verify metadata is authored, to catch any doc/live drift.
The endpoint's **5 req/min** rate limit is confirmed in the same spec — a
once-per-connect verify is well within budget.

**But** that endpoint returns credit/plan/pricing data only — it exposes **no
stable per-account identifier or human label** (no email, no account id, no org
name; confirmed against `AccountUsageResponse`). This is the same gap the
Wave-2 api_key tools with a "verifier capability" hit (semrush / moz /
dataforseo / fullstory precedents): the endpoint is a boolean validity oracle,
not an identity source.

Two ways to land it, in preference order:

1. **Verifier capability (preferred).** Use `manual_api_token` verify-before-
   write against `GET /v3/account/usage` for the `200`/`401` gate, but derive
   the connection `account_key`/label from a **constant/synthetic** value (e.g.
   `"lusha"` or a short fingerprint of the key) rather than a JSON-pointer
   identity field — because the key is *account-level*, one connection per
   assistant is the natural model and a rich label adds nothing. This is exactly
   the reusable "verifier" capability the parallel api_key tools grew; confirm
   whether it has already landed on `integration-service` main before writing
   any new capability code (do not fork a parallel one — grow the shared set).
   Also mind the account/usage **5 req/min** rate limit: verify fires once at
   connect, which is well within budget.
2. **No-verify strategy identity (fallback, mongodb precedent).** If the
   verifier capability is not available and growing it is out of this tool's
   scope, fall back to `identity.source: strategy` with no provider-side
   verification (mongodb OQ1): store the key, surface a bad key at first use via
   AnyCLI's `CredentialRejected`. Acceptable but strictly worse UX (stale
   feedback), so only if option 1 is blocked.

The bundle above writes `identity.source: strategy` as a placeholder; stage-2
pins it to whichever of the two the current `integration-service` capability set
supports, and records the choice here.

**`standard_oauth` vs adapter:** neither — this is a manual-secret (api_key)
bundle (`manual_api_token` or `manual_credentials` per the strategy decision
above), zero provider-specific Go on the Helio side (the whole point of the
manual-secret golden path). No `service/adapter_*.go`.

**Config:** none. api_key providers carry no `required_config_fields` (no client
id/secret) — nothing lands in `config/` or the `deploy/` Secret, so Lusha is not
part of lane-1's OAuth-app-registration queue at all. `configured` is true by
construction; the only gate to `done` is L5 (key-entry connect flow) + the
visible flip.

**Icon:** `ui/helio-app/src/integrations/icons/lusha.svg` +
`providerIcons.ts` append (manual, never generated).

---

## 5. Test plan → the five layers

| Layer | Concretely for Lusha | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in anycli. `httptest.Server` fakes for `contacts/search-and-enrich`, `companies/search-and-enrich`, `contacts/prospecting`, `companies/prospecting`, `contacts/enrich`, `companies/enrich`, `account/usage`. Assert: `api_key` header injected; request body shapes (identifier flags → `search-and-enrich` `contacts[]`/`companies[]` body; repeatable `--id` → `enrich` `{ids:[…]}` body; contact `--reveal emails,phones` → `reveal:["emails","phones"]`; `company reveal --reveal competitors,intent` → `reveal:["competitors","intent"]`; `company enrich` emits **no** `reveal` key; an `emails`/`phones` value rejected for company verbs); search-verb envelope surfaces `id` + `request_id`; `--json` envelope for success + `401`/`429` error rendering; exit-code contract (0/1/2). Never hits the real API. | No |
| **L2** dev harness, real API | `make build-harness`; `ANYCLI_CRED_API_KEY=<real key> anycli lusha -- account usage` (cheapest, credit-free, proves header+auth **and re-confirms the `/v3/account/usage` verify path live**), then one `contact enrich --email <known>` (proves the search-and-enrich body + reveal), plus a minimal two-step probe: `contact search` with a tight filter + `--size 1` → take the returned `id` → `contact reveal --id <that-id> --reveal emails` (proves prospecting `id` flows into `/v3/contacts/enrich`). Mandatory before pin bump. | **Yes** — a real Lusha Premium/Scale API key (account pool, lane 2). Burns credits on the enrich/search/reveal calls (kept minimal). |
| **L3** generate + suites | From `integration-service`: `provider-gen` + `provider-gen --check` (five projections regenerate together — validate on-branch, do **not** commit; batch lead owns the canonical regen). `helio-cli/go.mod` local `replace` → anycli branch; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`. `integration-service` unit suite green (esp. if the verifier capability is touched). | No |
| **L4** singleton + seed | `make run-singleton` (`env: dev`); `POST /internal/test-only/connections/seed` with `provider:"lusha"`, `access_token:<real key>` (api_key providers are seedable — user-token class), real seeded assistant/org identities; then `heliox tool lusha -- account usage` and `heliox tool lusha -- contact enrich --email <known>` resolve the key through the real token gateway → anycli → live Lusha API. Non-expiring key → seed `access_token` only, no refresh cycle. | **Yes** — same real key as L2. |
| **L5** full connect flow (pre-flip, once) | api_key **key-entry** path (not OAuth consent): `heliox tool lusha auth` → open connect link → paste the key through the real connect UI → stored via `POST /connections/credentials`, **verified against `GET /v3/account/usage`** (option-1 verifier) → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool lusha -- account usage` through the real token gateway succeeds. Agent-drivable (agent-browser) with human fallback. Gates the visible flip. | **Yes** — same real key. |

**Credential summary:** L1 and L3 need nothing external. L2, L4, L5 all need
**one real Lusha API key** (Premium or Scale plan — the free/lower tiers do not
issue API keys), from the lane-2 account pool. Prefer `account usage` for
liveness checks (no credit cost); reserve enrich calls for the minimum needed to
prove the request/response contract, since each burns account credits.

---

## 6. Definition of done (this tool)

L1–L5 green · AI-facing sub-doc published under
`agents/plugins/heliox/skills/tool/` (with the billing/credits guidance) · icon
registered · `presentation.visible: true` + regenerate as the single go-live
change. Until the flip: code-complete (hidden).

---

## 7. Implementation notes — divergences from this design (as built)

Recorded after implementing against the authoritative V3 OpenAPI bundle
(`docs.lusha.com/_bundle/apis/@v3/openapi.yaml`). Per the "follow official docs"
rule, the code follows the spec where it contradicts this design.

1. **Prospecting request shape — flat filter flags → `--filters <json>`.** §2/§3
   modeled `search` with flat flags (`--title --seniority --location …`). The
   real schemas (`V3ProspectingContactsRequest` / `V3ProspectingCompaniesRequest`
   — note the name order, not `V3Contacts…`) require a nested body:
   `{pagination:{page,size}, filters:{contacts|companies:{include,exclude:{…}}},
   options}`. The contact filter DSL alone carries `jobTitles`, `seniorityIds`,
   `departments`, `countries`, `locations`, `names`, … The verb therefore takes
   the whole `filters` object as raw JSON (`--filters`) plus `--page`/`--size`/
   `--include-partial`, rather than a flag per field. Pagination bounds are
   `page` 0–1000 (default 0) and `size` **10–100 (default 25)** — not 50.
2. **Enrich envelope `data` is an array, not a single object.** §3 said `enrich`
   → `data: {object}`. `search-and-enrich`/`enrich` responses always return a
   `results` **array** (batch-capable to 100), so all list verbs emit
   `data: [...]` for a consistent, honest agent contract; `account usage` stays
   an object. The reveal model (`{ids, reveal}`, per-entity reveal enums, company
   enrich has no reveal) matched the design's corrected §2/§3 and shipped as-is.
3. **Error envelope follows the shipped notion convention**
   `{"error":{"message","kind","status"}}` (stderr, exit 1), not the ad-hoc
   `{"error":{"code","message"}}` §3 sketched — one error shape across tools.
   401/403 map to AnyCLI `RejectCredential` (reconnect signal).
4. **Runtime strategy pinned to `manual_api_token` + `identity.source: strategy`
   (DESIGN §4 option 1).** On this branch neither shipped manual verifier fit a
   bare account-level key: the `manual_api_token` declarative verifier requires a
   stable-key identity field (Lusha's `/v3/account/usage` has none), and
   `manual_credentials` requires a DSN host. So the integration-service grew a
   reusable **constant-identity manual api_key verifier** (verify-before-write
   against the declared endpoint + header, 200/401 gate, constant synthetic
   account key = provider key). This is the same shared "verifier capability" the
   parallel api_key-no-identity tools (semrush / moz / dataforseo / fullstory)
   grow; the batch lead reconciles duplicates. The generator also now permits
   `_` in auth header names (RFC 7230 tchar) — Lusha's real header is `api_key`.
5. **No `toolToProvider` entry** — id == key == `lusha`, resolved mechanically.
6. **L2/L5 require a real Premium/Scale key** (lane-2 account pool) and were not
   run in this worktree; L1 (anycli unit) and L3 (`provider-gen --check` +
   helio-cli replace build + both suites) are green locally.
