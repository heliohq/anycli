# Tool design: Amplitude (`heliox tool amplitude`)

Scratch design for the Amplitude external tool provider, produced per the
`helio-tool-provider` pipeline (Helio `.claude/skills/helio-tool-provider/SKILL.md`)
and the master rollout plan
(`anycli/docs/design/008-300-integrations-rollout-plan.md`, catalog row 118,
Wave 2, Analytics). Batch lead strips this file at batch-end.

## 0. Naming axes (master plan §3)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `amplitude` | bundle `tool.command` (flat, none needed — equals `tool.name`) |
| ② anycli tool id | `amplitude` | `definitions/tools/amplitude.json` |
| ③ provider catalog key | `amplitude` | `integrations/providers/amplitude/` |

All three axes are identical (no corporate-family prefix, no dashed brand), so
**no `toolToProvider` divergence entry** is added in
`helio-cli/internal/toolcred/resolver.go`. Go package name (stage 2) is
`amplitude` (no digits/dashes to normalize).

## 1. Auth lane verification (independent check vs. catalog & audit)

Catalog row 118 and the 2026-07-21 OAuth audit (row 120 in the audit table)
both assign **`api_key`** with note "no viable multi-tenant path". Verified
against Amplitude's official docs: Amplitude's REST data/query APIs authenticate
with **HTTP Basic auth**, username = project **API Key**, password = project
**Secret Key**, both found in Amplitude project settings. There is **no
multi-tenant authorization-code OAuth** flow for these APIs (no registered app
that arbitrary Amplitude accounts authorize). Amplitude does offer OAuth
elsewhere for its newer platform surfaces, but the Dashboard/Export/Cohorts
data APIs an AI teammate needs are project-key + secret-key only. **The audit
verdict stands: `api_key`.** No divergence to record in DESIGN.

Sources (official):
- Export API — Basic `-u '{api_key}:{secret_key}'`, `GET https://amplitude.com/api/2/export`, EU base `https://analytics.eu.amplitude.com`.
- Dashboard REST API — Basic auth, endpoints under `/api/2/*` and `/api/3/*`.
- Behavioral Cohorts API — Basic auth, `/api/3/cohorts`, `/api/5/cohorts/*`.

## 2. Which API surface this tool wraps, and why

Driven by what an AI teammate actually does with Amplitude: **read and analyze
product-analytics results** ("what's DAU this week", "pull the signup funnel
conversion", "why did retention drop", "find this user's event stream",
"download the numbers behind chart X"). That is a **read/query** surface.

Deliberately **out of scope**: the HTTP V2 / Batch **event-ingestion** APIs
(sending events). Instrumentation is an SDK/build-time concern, not a teammate
action, and ingesting fabricated events from an assistant is a data-integrity
hazard. This tool is analysis-only.

Wrapped endpoints (all Basic auth; base = `https://amplitude.com` or, for EU
data-residency projects, `https://analytics.eu.amplitude.com`):

| Verb (subcommand) | Method + path | Purpose |
|---|---|---|
| `segmentation` | `GET /api/2/events/segmentation` | Event segmentation — counts/uniques/props over time, the core metric query |
| `funnels` | `GET /api/2/funnels` | Funnel conversion / drop-off across an ordered event list |
| `retention` | `GET /api/2/retention` | N-day / bracket retention |
| `events list` | `GET /api/2/events/list` | Catalog of tracked event types (needed to build valid `segmentation`/`funnels` queries) |
| `user-search` | `GET /api/2/usersearch` | Resolve a user/device/email → Amplitude ID |
| `user-activity` | `GET /api/2/useractivity` | A user's raw event stream by Amplitude ID |
| `chart` | `GET /api/3/chart/:id/csv` | Results behind an existing saved chart (analyst already built it in the UI) |
| `cohorts list` | `GET /api/3/cohorts` | Discoverable behavioral cohorts |
| `export` | `GET /api/2/export` | Raw event export (zipped JSONL) for a `start`/`end` hour range |

Rationale for the cut: `segmentation`/`funnels`/`retention` are the three chart
types an analyst asks for by name; `events list` is their prerequisite (invalid
event types return "Unable to create definition from arguments"); `user-search`
→ `user-activity` is the two-step "look at this user" flow (search returns the
Amplitude ID that activity requires); `chart` lets the assistant read a result
an analyst already curated without re-specifying the query; `cohorts list`
surfaces saved audiences. `export` is included but flagged secondary (§4) — it
returns bytes up to 4GB, so it is handled as a file receipt, not JSON
passthrough. The async cohort **download** chain (`/api/5/cohorts/request/*`,
poll + file) is **omitted from v1**: it is a 3-call async job with a 500/month
cap and low teammate value versus its complexity; add later if demanded.

## 3. anycli definition (stage-1 rubric → `service` type)

`cli` type is rejected: there is no official, non-interactive, `--json`-capable
Amplitude binary to provision into the runtime image. So **`service` type**,
HTTP against the REST API — the notion/bitly precedent (21 of 23 shipped
definitions are service type).

### 3.1 Definition JSON (`definitions/tools/amplitude.json`)

```json
{
  "name": "amplitude",
  "type": "service",
  "description": "Amplitude product analytics (read/query: segmentation, funnels, retention, cohorts, user activity, chart export)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_credentials"},
        "inject": {"type": "env", "env_var": "AMPLITUDE_API_CREDENTIALS"}
      }
    ]
  }
}
```

**One credential field.** Amplitude Basic auth needs two secrets, but Helio's
token gateway hands AnyCLI exactly one stored secret (the vault token payload,
design 317 D5 — see §5). The single injected value is the Basic-auth pre-image
`apiKey:secretKey` (the literal `user:password` string). This is the direct
analog of the **mongodb** precedent, whose one stored secret is a DSN that
embeds `user:pass@host`.

### 3.2 Service implementation (`internal/tools/amplitude/`)

Copy the `internal/tools/notion/` shape: a cobra tree grouped by resource, a
`BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest` server and
capture output, a typed `apiError`, and the documented exit-code contract.

- **Auth build.** Read `AMPLITUDE_API_CREDENTIALS`. Validate it splits into
  exactly two non-empty halves on the **first** colon (Amplitude secret keys do
  not contain colons; split on first colon so an errant one fails loudly rather
  than silently). On malformed input exit **2** with static guidance that never
  echoes the secret. Set `Authorization: Basic base64(rawValue)` — the raw value
  is already `user:pass`, so base64 it directly.
- **Region.** Root persistent flag `--region us|eu` (default `us`) selects the
  base host (`https://amplitude.com` vs `https://analytics.eu.amplitude.com`);
  `--base-url` overrides for tests. Region is **not derivable from the
  credential** and is **not** a secret, so it is a per-invocation AI arg, never
  stored. (This is the one Amplitude-specific wrinkle vs. mongodb, whose DSN
  carries its own host.)
- **Query params.** Each subcommand maps flags to the documented query params
  (`e=`/`s=` JSON for segmentation/funnels, `start`/`end`, `m=` metric,
  `i=` interval, `amplitude_id`, `user`, etc.). The service URL-encodes the JSON
  `e=`/`s=` values (docs call this out explicitly). Complex `e`/`s` are passed
  through as raw JSON strings from the AI (`--events '<json>'`) rather than
  re-modeled — Amplitude's segment grammar is large; do not reinvent it.

### 3.3 JSON output shape

- Dashboard JSON endpoints (`segmentation`, `funnels`, `retention`,
  `events list`, `user-search`, `user-activity`, `cohorts list`): **pass through
  Amplitude's JSON body verbatim on stdout** (+ newline) — provider-neutral,
  the notion/bitly convention.
- `chart` (`/csv`): the response is CSV; wrap in a JSON envelope
  `{"format":"csv","chart_id":"...","data":"<csv text>"}` so stdout stays JSON.
- `export` (bytes, up to 4GB): mirror bitly's `qr image` non-JSON handling —
  stream the zip to a file under the workspace/temp dir and emit a JSON
  **receipt** `{"saved":"<path>","bytes":N,"start":"...","end":"..."}`; never
  splat binary to stdout. Guard size and surface Amplitude's 400 (4GB /
  365-day limits) as a typed error.

### 3.4 Exit-code + error contract (notion contract)

`0` success · `1` runtime/API failure (typed `apiError`; **401 → credential
rejected**, so the token-gateway "stale credential" feedback loop and L4/L5 can
classify it) · `2` usage/parse (bad flags, malformed `AMPLITUDE_API_CREDENTIALS`).
`--json` renders the structured error envelope. Amplitude returns non-2xx with a
JSON/text body; capture `message`/`error` where present.

### 3.5 anycli tests (L1)

TDD, `httptest.Server` fakes only (never the live API in unit tests). Assert:
(a) `Authorization: Basic <base64(apiKey:secretKey)>` header on every request;
(b) region flag selects EU host; (c) each subcommand's path + query-param
mapping; (d) `e=`/`s=` URL-encoding; (e) `chart` CSV → JSON envelope; (f)
`export` → file receipt (fake zip bytes), no binary on stdout; (g) 401 → exit 1
credential-rejected, malformed creds → exit 2, both in plain and `--json` modes;
(h) secret never appears in any error string.

## 4. Credential fields & exact auth flow

**Registration model (verified):** the user creates/opens an Amplitude project;
project settings expose an **API Key** and a **Secret Key**. No app
registration, no OAuth, no review — a test/free project yields both keys
immediately (this is why the tool is agent-drivable through L1–L4 and api_key
L5). Both keys are **project-scoped** and **long-lived** (no expiry, no refresh
cycle) → seed `access_token` only at L4, no `refresh_token`/`expires_at`.

**Token semantics / sensitivity:** the **Secret Key** is the sensitive half;
the **API Key** is the lower-sensitivity project identifier also embedded in
client SDKs. Basic auth requires **both**, so **both must be stored**. Because
the token gateway projects a single opaque token payload, both are stored as one
colon-joined secret `apiKey:secretKey` (the Basic pre-image), and only the
non-secret API-Key half is ever surfaced as the human-readable account
key/label (§5).

**End-to-end flow:** connect (`POST /connections/credentials`, write-only) stores
the pasted `apiKey:secretKey` in Vault via the design-317 single-token path →
token gateway serves it as `credential.api_credentials` → heliox injects
`AMPLITUDE_API_CREDENTIALS` → AnyCLI sets `Authorization: Basic …` → live REST
call. No provider-side verification at connect (see §5 no-verify).

## 5. Helio provider bundle plan (`integrations/providers/amplitude/provider.yaml`)

Amplitude needs **two secrets**, both sensitive — a stronger shape than the
sibling `endpoint + api_key` tools (freshdesk/servicenow: one secret + one
non-secret host). The single-field storage face is therefore mandatory today:
design-317 **D5** allows exactly one required connect-form field and the vault
face is one secret; the multi-secret vault face (**D8**) is explicitly deferred
in `model/runtime_contract.go`. So v1 folds both keys into one field, exactly
like mongodb's DSN.

**Chosen strategy:** `auth.type: credentials` + `runtime_strategy:
manual_credentials` + `identity.source: strategy` — the mongodb bundle shape.

```yaml
schema: helio.provider/v1
key: amplitude
go_name: Amplitude

presentation:
  name: Amplitude
  description_key: amplitude
  consent_domain: amplitude.com
  visible: false           # hidden-first; flip is the single go-live change

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_credentials
        label_key: amplitude_api_credentials
        secret: true
        placeholder: "apiKey:secretKey"
        required: true       # exactly one required field (D5)
    setup_url: https://amplitude.com/docs/apis/authentication   # "where to get the keys" (project settings)

identity:
  source: strategy           # no HTTPS userinfo; region unknown at connect

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_credentials: token.access_token   # single stored secret → the one AnyCLI field
    account_key: connection.account_key

tool:
  name: amplitude
  kind: api-key              # 317 D2 wire-compat; client routes the drawer by auth_type
```

### 5.1 The one narrow integration-service capability growth

`composeProviderRegistration` currently **hardwires** `dsnHostIdentityDeriver`
for every `manual_credentials` provider (`service/provider_registry.go`). That
deriver `url.Parse`s the secret and demands a host — Amplitude's
`apiKey:secretKey` has no host, so Connect would fail for every user. Amplitude
needs a small, precedented growth (the same class as crisp's "keypair identity
deriver", servicenow/freshdesk endpoint work):

1. Add an **Amplitude identity deriver** (`service/manual_credentials_identity.go`
   sibling): split the stored secret on the first colon; **account_key = label =
   the API-Key half** (readable, stable per project — OQ2 human-readable
   constraint), never exposing the Secret-Key half into `Connection` metadata.
   **No provider-side request** (mongodb OQ1 no-verify): region is unknown at
   connect, so a live check would be ambiguous; a wrong key or wrong region
   surfaces at first tool use via AnyCLI `CredentialRejected` (401 → exit 1).
2. Make the `manual_credentials` branch **select the deriver per provider**
   (e.g. a small map keyed by `model.Provider`, defaulting to
   `dsnHostIdentityDeriver`) instead of hardwiring the DSN one. This is additive
   and shared with any future multi-part-secret manual provider. Unit-test both
   derivers stay selected correctly and Amplitude's never leaks the secret half.

This growth is required regardless of A/B below; it is the axis-orthogonal
"identity from a non-DSN structured secret" capability.

### 5.2 Axis ①/②/③ naming in the bundle

Flat provider (no `tool.group`): `tool.name: amplitude` == command word ==
provider key. No `toolToProvider` entry, no `toolGroups` entry.

### 5.3 Config, experiment, icon

- **No integration-service config** (`config/` / `deploy/`): `manual_credentials`
  declares no `required_config_fields` — no Helio client id/secret exists. The
  Config-Sync hard rule has nothing to sync for this provider.
- **No experiment gate** (GA lane once flipped); leave `experiment` empty.
- **UI icon** (manual, never generated): `ui/helio-app/src/integrations/icons/amplitude.svg`
  + register in `ui/helio-app/src/integrations/providerIcons.ts`.
- **i18n:** `tools.desc.amplitude` and the `amplitude_api_credentials`
  credential-field label key across all locales before the visible flip.

### 5.4 Decision: single colon-joined field (A) vs. native two-field (B)

- **Option A (chosen for v1):** one `api_credentials` field, `apiKey:secretKey`.
  Ships on current capability (D5), zero D8 dependency, mongodb-precedented, and
  the stored value is literally the Basic pre-image. UX papercut: the user
  hand-joins two console values with a colon.
- **Option B (post-D8):** two native labeled fields (`api_key`, `secret_key`)
  via the design-317 **D8 multi-secret vault face**. Better UX, but D8 is not on
  main. If D8 lands (tracked via servicenow/freshdesk/crisp) before Amplitude's
  batch merges, adopt B: two-field `credential_input`, joined server-side into
  the same Basic pre-image, deriver unchanged. **Reconcile at batch time** with
  whatever multi-field face those siblings shipped — do not fork a parallel one.

## 6. AI-facing docs (stage 8)

Add `agents/plugins/heliox/skills/tool/amplitude.md` (sub-doc): the read-only
scope, the `apiKey:secretKey` connect format, the mandatory `--region` for EU
projects, the `e=`/`s=` raw-JSON segment grammar with one worked
`segmentation` example, and the query-cost/rate-limit note (concurrent 1000
cost / 5 min, 108,000 cost/hour) so the assistant paces heavy funnel/retention
pulls. Bump plugin version + publish at batch end (one publish per batch).

## 7. Test plan → the five layers

| Layer | Amplitude specifics | Needs external creds? |
|---|---|---|
| **L1** anycli unit | §3.5 httptest fakes: Basic header, region host, per-endpoint paths, `e=`/`s=` encoding, CSV→envelope, export→file receipt, 401→exit1 / malformed→exit2, secret-never-leaked | No |
| **L2** harness real API | `ANYCLI_CRED_API_CREDENTIALS='apiKey:secretKey' anycli amplitude -- events list` and one `segmentation` against a real Amplitude **test project**; confirm real data, then EU host if an EU test project exists | **Yes** — real project API+secret keys (free/test project) |
| **L3** provider-gen + suites | `provider-gen` + `--check` (bundle strict-decode, D5 single-field check, `credential.fields` projection honesty); integration-service unit suite incl. the new Amplitude identity deriver + per-provider selection tests; helio-cli `go test ./cmd/heliox/cmds/tool/` with a local `replace` to this anycli branch | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `access_token='<apiKey>:<secretKey>'` (no refresh — long-lived), real seeded assistant/org identity; then `heliox tool amplitude -- events list` reaches the live API through the token gateway | **Yes** — same real keys, seeded |
| **L5** api_key connect sweep | Agent-drivable (master plan §2 api_key L5): open connect link → paste `apiKey:secretKey` via `POST /connections/credentials` real UI → connection shows connected/configured (`GET /connections`) → one **unseeded** live `amplitude` command succeeds. Human fallback on UI breakage. Run once, hidden, before the visible flip | **Yes** — real keys |

L2/L4/L5 all require the same externally-supplied Amplitude **test-project API
Key + Secret Key** (account-pool lane, master plan §2 lane 2) — no app
registration, so procurement is just a free Amplitude project. An EU test
project is optional (only to exercise `--region eu` at L2).

## 8. Rollout

Land hidden (`visible: false`) with the anycli pin bumped, the identity-deriver
growth, bundle, icon, i18n, and docs. Run L1–L4 while hidden, then the api_key
L5 sweep; flip `presentation.visible: true` + pick an unoccupied
`presentation.order` + regenerate as the single go-live change.
