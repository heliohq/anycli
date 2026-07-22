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
- **Divergence 3 — host/version RESOLVED at stage-1 research (2026-07-22).**
  Verified against Hotjar's own API docs (help.hotjar.com API Reference +
  Contentsquare Responses API reference) and cross-checked against a public
  reference implementation (`yasin749/hotjar-mcp-server`). Frozen contract:
  - Base host **`https://api.hotjar.io`**, single global version **`/v1`** (no
    "v2"; the third-party `api.hotjar.com` cite is wrong).
  - Token exchange **`POST /v1/oauth/token`**, `application/x-www-form-urlencoded`
    body `grant_type=client_credentials&client_id=…&client_secret=…` →
    `{"access_token","token_type":"Bearer","expires_in":3600}`. (Note the path is
    `/v1/oauth/token`, NOT snov's `/v1/oauth/access_token`.)
  - Data calls carry `Authorization: Bearer <token>`. Cursor pagination: send
    `limit` + `cursor`; response carries `next_cursor` (null when exhausted).
    Rate limit 3000/min → **429**. Missing header → **401**; bad/insufficient
    token → **403**.
  - Endpoints (see §2 table): `GET /v1/sites/{site_id}/surveys`,
    `GET /v1/sites/{site_id}/surveys/{survey_id}`,
    `GET /v1/sites/{site_id}/surveys/{survey_id}/responses` (newest-first),
    `POST /v1/organizations/{organization_id}/user-lookup`.
- **Divergence 4 — user-lookup and deletion are the SAME endpoint (new, safety-
  critical).** Contrary to §2's original assumption of two separate endpoints,
  there is ONE endpoint `POST /v1/organizations/{organization_id}/user-lookup`
  whose JSON body carries a **`delete_all_hits`** boolean: `false` looks up the
  data subject's captured data, `true` **silently purges it**. The deletion
  "verb" DESIGN excluded is not a separate path we can simply omit — it is a flag
  on the lookup path. Consequence for the implementation: the `user lookup`
  subcommand **hardcodes `delete_all_hits: false`** and exposes **no flag** that
  can flip it; a unit test asserts the outgoing body always carries
  `delete_all_hits=false`. This makes the destructive capability structurally
  unreachable from the toolset while still delivering the read-only lookup, and
  is a stronger guarantee than "we just didn't add a delete command."

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

| Verb (subcommand) | Method + path (frozen at stage-1) | Purpose |
|---|---|---|
| `survey list` | `GET /v1/sites/{site_id}/surveys` | Enumerate surveys for a site (site picker for `responses`). `--cursor`/`--limit`. |
| `survey get` | `GET /v1/sites/{site_id}/surveys/{survey_id}` | One survey's detail; `--with-questions` includes question metadata. |
| `survey responses` | `GET /v1/sites/{site_id}/surveys/{survey_id}/responses` | Export responses, sorted newest-first, cursor-paginated. Primary read. `--cursor`/`--limit`. |
| `user lookup` | `POST /v1/organizations/{organization_id}/user-lookup` (JSON body `data_subject_email`, `delete_all_hits:false` hardcoded) | Find a data subject's captured data (GDPR/ops). Read-only by construction (Divergence 4). |

**Explicitly excluded from v1 of the tool: any deletion capability.** Deletion is
not a separate endpoint but the `delete_all_hits: true` mode of the user-lookup
endpoint (Divergence 4), which immediately and silently purges captured data. The
`user lookup` subcommand therefore hardcodes `delete_all_hits: false` with no
override flag, so a teammate exporting survey feedback is not one malformed
argument away from deleting a customer's Hotjar data. If a deletion verb is ever wanted, it is a separate, human-gated
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
  **client-credentials token exchange** itself (`POST /v1/oauth/token`,
  form-urlencoded `grant_type=client_credentials`
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
      { "source": {"field": "client_id"},  "inject": {"type": "env", "env_var": "HOTJAR_CLIENT_ID"} },
      { "source": {"field": "api_secret"}, "inject": {"type": "env", "env_var": "HOTJAR_CLIENT_SECRET"} }
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
- `auth.credential_input.fields`: `client_id` (secret) and `api_secret`
  (secret), both `required: true`; `setup_url` → Hotjar Settings → API page.
  **Field is `api_secret`, NOT `client_secret`** — see Divergence 5.
- `connection.runtime_strategy: manual_credentials`, `mode: isolated`,
  `disconnect_mode: local_only`.
- `identity.source: strategy` — no cheap HTTPS userinfo endpoint; the generic
  `multiFieldClientIdentityDeriver` (design 317 D8) derives the human-readable
  `account_key` from the first connect-form field (`client_id`), never a hash.
- `credential.fields`: `client_id: token.client_id`, `api_secret:
  token.api_secret`, `account_key: connection.account_key` — the two secrets are
  stored as a JSON composite in the token payload and projected to AnyCLI as two
  named fields (design 317 D8 multi-field face).

- **Divergence 5 — multi-field manual_credentials is NOT free on this branch's
  main; it is real capability growth (the DESIGN's cost estimate was wrong).**
  The DESIGN assumed `mixpanel`/`snov` multi-field `manual_credentials` was
  already on main, so only a deriver was needed. On this branch's actual main
  base, `model.validateCredentialInputSchema` still enforces the design 317 D5
  **single-secret** rule (exactly one required field), and the credential-source
  denylist forbids any field named `client_secret`. Two-secret Hotjar therefore
  requires the design 317 D8 multi-field capability, which the sibling
  multi-field tools (`snov`/`mixpanel`/`lusha`) each grow identically on their
  branches: (1) relax `validateCredentialInputSchema` to allow N≥1 required
  fields for `AuthCredentials` (JSON-composite storage); (2) add credential
  sources `token.client_id` + `token.api_secret` (`api_secret`, not
  `client_secret`, to keep the denylist intact); (3) the generic
  `multiFieldClientIdentityDeriver` (first-field account key). This tool applies
  that **identical** shared-capability diff so it merges as a no-op with the
  siblings at batch end — it is not a new hotjar-specific capability, and Hotjar
  ships **zero** provider-specific service code.
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
