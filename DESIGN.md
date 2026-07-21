# Tool design — Hotjar (`heliox tool hotjar`)

Scratch design for the `hotjar` external tool provider. Batch-lead strips this
file at batch end. Branch: `tool/hotjar` (both repos). Status: **3-hold
holdback batch pre-verify design** (Wave 3 final batch, §5 of the master plan).

## 0. Naming axes (master plan §3) and lane

| Axis | Value |
|---|---|
| ① CLI command word (`tool.command`) | `hotjar` (flat; no family group) |
| ② anycli tool id (`definitions/tools/<id>.json`) | `hotjar` |
| ③ provider catalog key (bundle dir / `key:`) | `hotjar` |

②≡③ (identity). Mechanical, no divergence → **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go`. Catalog row 121, category Analytics,
auth lane **`api_key`**, re-laned Wave 2 → **3-hold** on API-feasibility risk.

## 1. Pre-verify verdict (the gate this tool must clear before dev starts)

Hotjar is in the 3-hold batch precisely because its public API was flagged as
possibly unintegrable ("essentially user-lookup/GDPR endpoints with no general
data API" — master plan §6). **I verified against Hotjar's own documentation and
that characterization is partially outdated.** Divergences recorded here (per the
prompt's "record divergence" instruction):

- **Divergence 1 — the API is not GDPR-only.** Hotjar ships a REST API whose
  three documented feature areas are **Survey responses export**, **User
  lookup**, and **Deletion request** (GDPR). The survey-responses export is a
  real, teammate-useful data surface — not just a lookup/delete shim. So the
  §6 "no general data API" note is too pessimistic: there is a bounded but
  genuine read surface (survey/feedback responses). **The tool is integrable.**
  Recommendation: **passes the 3-hold pre-verify gate**, scoped read-first.
- **Divergence 2 — auth lane confirmed `api_key`, but for a better reason.**
  The OAuth audit put Hotjar in `api_key` as "no viable multi-tenant path". The
  underlying mechanism is that each customer, from their own Hotjar org, mints a
  **client_id + client_secret credential pair** (Settings → API, Admin-only,
  auto-expires after 1 year) and exchanges it for a bearer via an OAuth
  **client-credentials** grant. There is **no authorization-code/consent flow** a
  single Helio app could register for arbitrary tenants. Under the audit rubric
  (multi-tenant authorization-code required to leave `api_key`), the `api_key`
  verdict is **correct and re-confirmed** — the credential is a user-supplied
  secret pair, exactly the api_key lane's shape.
- **Divergence 3 — host/version ambiguity to resolve at L2.** Official docs
  sample requests use host **`api.hotjar.io`** with a `/v1/...` path
  (`GET /v1/sites/{site_id}/surveys`, `Authorization: Bearer <token>`).
  Third-party guides cite `https://api.hotjar.com/v1`; one source calls the
  export surface "v2". Hotjar is now part of Contentsquare and docs are
  migrating. **Do not hardcode the host/version from this doc** — pin the exact
  base URL, token-exchange path, and version at **stage-1 research → L2 harness**
  against a live Ask-Scale account, then freeze it in the definition.

**Residual risk that keeps it in 3-hold: account procurement, not feasibility.**
The survey-export surface is gated to **Ask Scale** plan; user-lookup/deletion to
**Observe Scale**. L2 (real-API harness) and L5 (live key-entry) both need a paid
Ask-Scale test account in the pool. If procurement fails, swap per risk #2
(catalog-amendment, hold 298 total). API keys also hard-expire at 1 year — a
long-lived connection will silently 401 and need re-entry (surface as
`CredentialRejected`, document in the AI-facing doc).

## 2. What an AI teammate does with Hotjar → which API surface we wrap

Driven by the teammate use case (an analytics colleague pulling voice-of-customer
data), the tool wraps the **Survey Responses API** as the primary surface, with
**User lookup** as a secondary GDPR/ops surface. Endpoints below are the *shape*
to implement; exact paths/host are frozen at L2 (Divergence 3).

| Verb (subcommand) | Method + path (verify at L2) | Purpose |
|---|---|---|
| `survey list` | `GET /v1/sites/{site_id}/surveys` | Enumerate surveys for a site (site picker for `responses`). |
| `survey responses` | `GET /v1/sites/{site_id}/surveys/{survey_id}/responses` | Export responses, sorted newest-first, cursor-paginated. Primary read. |
| `user lookup` | `POST /v1/.../lookup` (body `data_subject_email`) | Find a data subject's captured data (GDPR/ops). |

**Explicitly excluded from v1 of the tool: the deletion request endpoint.** It is
destructive (`delete_all_hits: true` immediately purges captured data) and has no
place in an unattended AI-teammate toolset by default. A teammate exporting
survey feedback must not be one malformed argument away from deleting a customer's
Hotjar data. If a deletion verb is ever wanted, it is a separate, human-gated
follow-up — noted here so the decision is explicit, not accidental. The
client-side **Identify API** / JS SDK (`hj('identify', …)`) and Events API are
browser instrumentation, out of scope for a server-side passthrough tool.

## 3. anycli definition & implementation

- **Type: `service`** (stage-1 rubric). No official non-interactive `--json` CLI
  exists → HTTP service in `internal/tools/hotjar/` against the REST API. Mirror
  `internal/tools/notion/` shape: cobra tree grouped by resource (`survey`,
  `user`), `BaseURL`/`HC`/`Out`/`Err` struct for httptest fakes, exit-code
  contract (0 ok / 1 API failure via typed `apiError` / 2 usage), `--json`
  structured output + error envelope.
- **Credential shape — two manual fields, exchange done in-service.** The
  service receives `client_id` + `client_secret` (see §4), performs the
  **client-credentials token exchange** itself (POST `grant_type=client_credentials`
  → `{access_token, token_type:"Bearer"}`), caches the bearer for the process
  lifetime, and calls the data endpoints with `Authorization: Bearer <token>`.
  This keeps the whole OAuth-ish dance inside anycli (which "knows nothing about
  OAuth" at the *host* layer — it just runs the definition), so Helio stores a
  static secret pair and needs **zero token-gateway/OAuth machinery**. This is
  the same in-service client-credentials pattern already used by the `snov`
  service (client_id/secret → token → call), which is the precedent to copy.
- **Definition `auth.credentials`** — two `field`→`env` bindings:

```json
{
  "name": "hotjar",
  "type": "service",
  "description": "Hotjar as a tool (survey responses export + user lookup)",
  "auth": {
    "credentials": [
      { "source": {"field": "client_id"},     "inject": {"type": "env", "env_var": "HOTJAR_CLIENT_ID"} },
      { "source": {"field": "client_secret"}, "inject": {"type": "env", "env_var": "HOTJAR_CLIENT_SECRET"} }
    ]
  }
}
```

- **JSON output shape**: list verbs emit `{ "results": [ … ], "next": "<cursor>" }`
  mirroring Hotjar's own top-level-object + `results[]` convention (and its
  cursor pagination), so pagination is a first-class `--cursor` flag rather than
  hidden. Single-object verbs emit the object directly. Errors → `apiError`
  envelope; map upstream 401/403 (auth/permission/plan-tier) and 429
  (rate-limit, 3000/min) to distinct human messages.
- **TDD (anycli AGENTS.md)**: `httptest.Server` fakes assert (a) the
  client-credentials POST body + that the returned bearer is injected on the
  subsequent data call, (b) request shape/paths, (c) plain + `--json` rendering
  of 401/403/429. Never hit the real API from unit tests. Register in
  `internal/tools/register.go` `init()` (`RegisterService("hotjar", &hotjar.Service{})`)
  — batch-end shared-surface merge.

## 4. Helio provider bundle plan (hidden-first)

`integrations/providers/hotjar/provider.yaml`, `presentation.visible: false`.
Model on the **mongodb** manual-credential bundle, extended to two fields (the
`mixpanel` multi-field manual-credentials precedent). No OAuth block, no service
adapter, no integration-service config (client id/secret are the *user's*, pasted
through the connect UI, not env config).

- `auth.type: credentials`, `owner: individual`.
- `auth.credential_input.fields`: `client_id` (secret) and `client_secret`
  (secret), both `required: true`; `setup_url` → Hotjar Settings → API page.
- `connection.runtime_strategy: manual_credentials`, `mode: isolated`,
  `disconnect_mode: local_only`.
- `identity.source: strategy` — no cheap HTTPS userinfo endpoint; derive the
  human-readable `account_key` from `client_id` (the `snov` client_id-deriver
  precedent), never a hash. Verify a `client_id`-based deriver exists in
  integration-service; if not, that is the one small capability-growth item
  (a reviewed deriver enum value), analogous to `snov`/`crisp`.
- `credential.fields`: project both `client_id` and `client_secret` from the
  stored token payload to the runtime (multi-field manual credential — confirm
  `mixpanel`/`snov` multi-field `manual_credentials` projection is on main; it is
  the same shape, so expected **zero** new capability beyond the deriver).
- `tool.name: hotjar`, `tool.kind: api-key` (wire-compat value; drawer routes by
  auth_type). No `tool.group`, no `experiment` gate (GA once flipped).
- UI icon `ui/helio-app/src/integrations/icons/hotjar.svg` + `providerIcons.ts`
  append (manual, batch-end). AI-facing sub-doc under
  `agents/plugins/heliox/skills/tool/` — document: read-only surface, plan-tier
  requirement (Ask Scale for responses), 1-year key expiry re-entry, and the
  deliberate absence of a delete verb.

Generation: one `provider-gen` + `--check` run at batch end (five projections
committed together). On-branch validation only (local regen + `helio-cli` `go.mod`
`replace` → anycli branch); **do not commit** projections/replace/pin per §2.

## 5. Test plan → the five layers

| Layer | Hotjar-specific check | Needs external creds? |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes for the client-credentials exchange + bearer injection, survey list/responses shape, cursor paging, 401/403/429 rendering. | No |
| L2 | `anycli hotjar -- survey responses …` harness against the **real** API with `ANYCLI_CRED_CLIENT_ID` / `ANYCLI_CRED_CLIENT_SECRET`. **Freezes Divergence 3** (host `api.hotjar.io` vs `.com`, `/v1` path, token-exchange path, real `results[]`/cursor shape). | **Yes — Ask Scale account + minted key pair** |
| L3 | `provider-gen --check` + `helio-cli` + integration-service unit suites (resolver identity path, bundle strict-decode). | No |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `hotjar`, seed `client_id`+`client_secret` — an `api_key`/user-token provider, seedable) → `heliox tool hotjar -- survey list`. Bearer minted in-service from seeded pair reaching the live API is the success signal. | **Yes — same key pair** |
| L5 | **api_key key-entry path** (master plan §2, agent-drivable): open connect link → paste client_id/secret in real connect UI → `GET /connections` shows connected/configured → one **unseeded** live `heliox tool hotjar -- survey list` succeeds. No OAuth consent (there is none). | **Yes — same key pair** |

L2/L4/L5 all block on **one procured Ask-Scale test account**; that single
dependency is the tool's real critical path and the reason it sits in 3-hold.

## 6. Open decisions for the batch lead

1. **Confirm host/version at L2** before freezing the definition (Divergence 3).
2. **Deletion verb excluded** by default (§2) — confirm this stance; if a
   human-gated delete is wanted it is a separate follow-up, not this tool.
3. **`client_id` identity deriver**: reuse the `snov` deriver if merged; else the
   sole integration-service capability-growth item (one reviewed enum value).
4. **Account procurement is the go/no-go** for the 3-hold pre-verify — no
   Ask-Scale account ⇒ swap per risk #2 (hold 298 total).
