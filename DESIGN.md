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
`POST /connections/credentials` API into Vault; `runtime_strategy:
manual_api_token`; `tool.kind: api-key`. This is the Figma-precedent shape
(`X-Figma-Token` + `/v1/me`) described in design 227's provider-extension
contract, adapted to Lusha's header name and verify endpoint.

---

## 2. API surface wrapped, and why

**Base URL:** `https://api.lusha.com/v3/` (V3 is the current production version;
V2 is operational but deprecating — target V3 only). All requests HTTPS, JSON
in/out, key in the `api_key` header.

**What an AI teammate actually does with Lusha** (Sales Engagement): it already
holds a partial lead — an email, a LinkedIn URL, or a name + company — and needs
the missing coordinates (verified email, direct/mobile phone, title,
firmographics) to act; or it needs to *generate* net-new contacts/companies
matching an ICP; or it needs to check remaining credits before spending them on
a costly prospecting run. That maps to three resource groups:

| Group / verb | Method + path | Why it's in scope |
|---|---|---|
| `contact enrich` | `POST /v3/contacts/search-and-enrich` | The core action: turn a known identifier (email / linkedinUrl / firstName+lastName+companyName\|companyDomain / id) into revealed emails + phones. One-shot combined search+enrich — the drop-in replacement for legacy `GET /v2/person`. |
| `company enrich` | `POST /v3/companies/search-and-enrich` | Turn a domain / name / id into firmographics (size, revenue, industry, technologies). Drop-in for legacy `GET /v2/company`. |
| `contact search` | `POST /v3/contacts/prospecting` | ICP prospecting: filter by job title, seniority, department, location, company size, revenue, industry, technologies, intent, signals → net-new contact IDs (non-PII preview). Generates leads the assistant did not already have. |
| `company search` | `POST /v3/companies/prospecting` | Same filter model, company-level. |
| `account usage` | `GET /v3/account/usage` | Credits used/remaining/total, plan, rate limits. Agent-natural pre-flight check before a credit-heavy prospecting sweep — and the provider-side **verifier** endpoint (see §4). |

**Deliberately deferred (v1):** Signals, Lookalikes, and the Subscriptions
(webhook) endpoints under the `/api/` prefix. Webhooks require a Helio-hosted
callback sink and a subscription lifecycle that an AnyCLI passthrough tool has
no place to deliver to; Signals/Lookalikes are narrower and can be added as
verbs later without reshaping the tool. Bulk (up to 100 identifiers per call) is
supported by the enrich endpoints and is exposed via repeatable identifier
flags rather than a separate verb.

**Billing note surfaced to the AI (docs, not code):** `search-and-enrich`
charges twice per result (one `api_search` + one reveal); the enrich verbs are
credit-consuming. The `account usage` verb exists so the assistant can reason
about spend. This goes in the AI-facing sub-doc, not the definition.

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
lusha contact enrich   --email | --linkedin-url | --name+--company[-domain] | --id   [--reveal emails,phones]
lusha contact search   --title --seniority --location --industry --company-size … [--page N --size 50]
lusha company enrich   --domain | --name | --id
lusha company search   --industry --location --size --revenue --technology …       [--page N --size 50]
lusha account usage
```

**JSON output shape.** Every verb accepts `--json` and emits a stable envelope
so the assistant parses deterministically regardless of Lusha's nesting:

- Enrich verbs → `{"data": {<revealed contact|company object>}, "meta":
  {"credits_charged": <n>, "request_id": "…"}}`. Lusha's search-then-reveal
  response carries `has` / `canReveal` preview flags; the service flattens the
  revealed record into `data` and drops preview-only scaffolding.
- Search verbs → `{"data": [<preview objects>], "meta": {"page": N,
  "total": M, "has_more": bool}}`. Preview objects are non-PII (IDs + `has`
  availability flags) — the assistant then calls an enrich verb on chosen IDs.
- `account usage` → `{"data": {"credits": {"used", "remaining", "total"},
  "plan": "…"}}` (fields passed through as returned).
- Errors (all verbs) → exit 1 with `{"error": {"code": "…", "message": "…"}}`;
  a `401` from Lusha maps to an auth-class error message pointing the assistant
  at reconnecting.

---

## 4. Helio provider bundle plan (`integrations/providers/lusha/provider.yaml`)

Hidden-first (`presentation.visible: false`). Manual-token (api_key) shape,
Figma/`manual_api_token` precedent — **not** the mongodb no-verify path, because
Lusha *does* offer a clean HTTPS verify signal.

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
  runtime_strategy: manual_api_token

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

**Verify + identity — the one real design decision.** The `manual_api_token`
strategy auto-composes *verify-before-write*: a provider-declared header + an
HTTPS identity endpoint hit at connect time, so a bad key is rejected before it
reaches Vault. Lusha's natural verify endpoint is `GET /v3/account/usage`
(`200` on a valid key, `401` on an invalid/missing one). **But** that endpoint
returns credit/plan data only — it exposes **no stable per-account identifier or
human label** (no email, no account id, no org name). This is the same gap the
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

**`standard_oauth` vs adapter:** neither — this is a `manual_api_token` bundle,
zero provider-specific Go on the Helio side (the whole point of the manual-token
golden path). No `service/adapter_*.go`.

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
| **L1** anycli unit | `go test ./...` in anycli. `httptest.Server` fakes for `contacts/search-and-enrich`, `companies/search-and-enrich`, `contacts/prospecting`, `companies/prospecting`, `account/usage`. Assert: `api_key` header injected; request body shapes (identifier flags → JSON body; reveal flags); `--json` envelope for success + `401`/`429` error rendering; exit-code contract (0/1/2). Never hits the real API. | No |
| **L2** dev harness, real API | `make build-harness`; `ANYCLI_CRED_API_KEY=<real key> anycli lusha -- account usage` (cheapest, no credit burn, proves header+auth), then one `contact enrich --email <known>` against a real known contact to prove the enrich body shape + reveal path. Mandatory before pin bump. | **Yes** — a real Lusha Premium/Scale API key (account pool, lane 2). Burns a credit on the enrich call. |
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
