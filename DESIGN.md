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
  stored today. (This is the one Amplitude-specific wrinkle vs. mongodb, whose
  DSN carries its own host.)
- **Region is a first-class correctness axis, not cosmetic.** An Amplitude
  project lives in exactly one data-residency silo (US or EU); its keys are
  unknown to the other silo's host. So a valid EU-project key called against the
  US-default host returns `401 invalid API key` — indistinguishable at the
  transport layer from a genuinely dead key. Because the credential and the
  stored Connection metadata carry **no region signal**, neither the assistant
  nor the token gateway can tell "wrong region" from "stale credential." The
  auth build and error contract MUST NOT treat a US-default 401 as proof the
  credential is dead (see §3.4) — otherwise the entire EU cohort is trapped in a
  reconnect loop that cannot fix the real problem (`--region eu`).
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

Exit codes: `0` success · `1` runtime/API failure (typed `apiError`) · `2`
usage/parse (bad flags, malformed `AMPLITUDE_API_CREDENTIALS`). `--json` renders
the structured error envelope. Amplitude returns non-2xx with a JSON/text body;
capture `message`/`error` where present.

**Credential rejection is a flag, not an exit code.** The real signal the
token-gateway stale-credential feedback loop and L4/L5 key on is the
`execution.Result.CredentialRejected` boolean, set by wrapping the error with
`execution.RejectCredential(...)` — exactly the notion/sheets/calendar/drive
`classifyCredentialError(status, body, err)` precedent (`internal/tools/drive/
auth_error.go`). Exit 1 is just the generic runtime-failure code shared with
every transport/API/rate-limit error and does **not** by itself invalidate a
credential. So the contract is: `classifyCredentialError` sets
`CredentialRejected` **only** when a 401 is unambiguously the credential's
fault, and exit stays 1 either way.

**Region-aware 401 classification (the EU trap, §3.2).** A 401 is only
unambiguous evidence of a dead credential when the region that produced it was
**explicitly chosen** by the caller (`--region us` or `--region eu` passed
in) — then `classifyCredentialError` sets `CredentialRejected = true`. When the
request used the **default** region (US, region not asserted by the caller), a
401 is region-ambiguous: it may be a live EU-project key hitting the wrong silo.
In that case:

- do **NOT** set `CredentialRejected` (leave it false so the gateway does not
  invalidate a possibly-live EU credential), and
- the error string/`--json` hint MUST explicitly say: *"401 from the US host —
  if this is an EU data-residency project, retry with `--region eu` before
  reconnecting."*

This gives the assistant a concrete self-correction path (retry EU) instead of a
false "credential is stale, reconnect" verdict it cannot act on. Only after an
**explicit** `--region eu` retry also 401s is the credential classified rejected.
Malformed `AMPLITUDE_API_CREDENTIALS` is exit 2 and never touches the flag.

### 3.5 anycli tests (L1)

TDD, `httptest.Server` fakes only (never the live API in unit tests). Assert:
(a) `Authorization: Basic <base64(apiKey:secretKey)>` header on every request;
(b) region flag selects EU host; (c) each subcommand's path + query-param
mapping; (d) `e=`/`s=` URL-encoding; (e) `chart` CSV → JSON envelope; (f)
`export` → file receipt (fake zip bytes), no binary on stdout; (g) **credential
classification keys on the flag, not the exit code** — assert
`result.CredentialRejected == true` (via `execution.IsCredentialRejected`) when
a 401 comes from an **explicitly chosen** region, and
`result.CredentialRejected == false` when a 401 comes from the **default** (US)
region, with the returned error string containing the `--region eu` retry hint;
exit code is 1 in both 401 cases and 2 for malformed creds; both plain and
`--json` modes (mirrors the notion/sheets/drive `CredentialRejected` test
tables); (h) secret never appears in any error string.

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

**Region is the gap this flow leaves open.** Because region is neither stored at
connect nor derivable from the credential, an EU-project user's key is accepted
fine at connect (no verify) but every default-`--region us` tool call 401s. §3.4
keeps that from being mis-reported as a dead credential (no `CredentialRejected`
on the US-default 401 + an explicit `--region eu` hint), so an AI teammate can
self-correct. That is the v1 mitigation. The **cleaner** fix is to capture
region at connect so the assistant never has to guess:

- **Within D5 (one required field):** fold region into the single field via an
  optional region suffix the deriver parses, e.g. `apiKey:secretKey@eu`
  (default US when absent). The deriver strips and records region into
  non-secret Connection metadata; heliox then defaults `--region` from it. Keeps
  the one-required-field cap.
- **Post-D8 (Option B, §5.4):** carry region as a **second non-secret stored
  field** alongside the two-secret face, projected to the runtime so the
  assistant never guesses.

Either capture path is preferred over the guess-and-retry mitigation once its
enabling capability lands; the §3.4 hint is the floor, not the ceiling.

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
for every `manual_credentials` provider (`service/provider_registry.go:96`, the
single `RuntimeStrategyManualCredentials` branch). That deriver `url.Parse`s the
secret and demands a host — Amplitude's `apiKey:secretKey` has no host, so
Connect would fail for every user. Amplitude needs its own first-colon-split
deriver, plus the branch must stop hardwiring one deriver.

**The deriver-selection mechanism is a once-per-batch SHARED change, not a
per-tool append.** This is the crux to get right at coordination time. Amplitude
is not riding landed precedent here: as of this batch there is no crisp bundle
and no non-DSN deriver merged in integration-service — crisp (keypair identity),
servicenow/freshdesk (endpoint + secret) are **concurrent Wave-2 siblings** being
built in parallel, and all of them must break the **same** hardwired branch at
`provider_registry.go:96`. If each tool independently drops its own
`map[model.Provider]deriver` or `switch` into that one branch, they collide at
the batch-end merge — precisely the shared-surface hazard master-plan §2 is
structured to avoid (the same class as the D8 multi-field face flagged in §5.4).

Coordination contract:

1. **Add an Amplitude identity deriver** (`service/manual_credentials_identity.go`
   sibling): split the stored secret on the first colon; **account_key = label =
   the API-Key half** (readable, stable per project — OQ2 human-readable
   constraint), never exposing the Secret-Key half into `Connection` metadata.
   **No provider-side request** (mongodb OQ1 no-verify): a live check would need
   a region it does not have (§3.2/§4), so a wrong key or wrong region surfaces
   at first tool use via AnyCLI's `CredentialRejected` flag under the §3.4
   region-aware rules. This deriver is Amplitude-owned and collision-free.
2. **The selection mechanism itself is owned by whichever sibling
   (crisp / servicenow / freshdesk / amplitude) lands it first.** Amplitude does
   **not** fork a parallel selector; it registers its first-colon-split deriver
   into the already-landed mechanism (or, if amplitude is first, lands the
   mechanism and the siblings register into it). Prefer a **bounded,
   self-describing selector** — a bundle-declared deriver *kind* resolved through
   the service's existing closed-capability convention (the same way
   `runtime_strategy` and `tool.kind` are closed enums), so the set of derivers
   is enumerable and validated at load — over a raw `map[model.Provider]deriver`
   that every tool edits by hand (which reintroduces the per-provider-edit
   collision the closed-enum convention exists to prevent). Default remains
   `dsnHostIdentityDeriver` for bundles that declare no kind. Unit-test that each
   declared kind resolves to its deriver and that Amplitude's never leaks the
   secret half.

Step 1 is Amplitude-orthogonal and lands with this tool regardless of A/B below.
Step 2 is the shared surface: coordinate it once with the co-built siblings
rather than treating any of them as pre-existing precedent.

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
- **Option B (post-D8):** native labeled fields — two secret (`api_key`,
  `secret_key`) plus an optional **non-secret `region` field** (§1 EU trap
  fix) — via the design-317 **D8 multi-secret vault face**. Better UX and it
  closes the region gap by storing region at connect. D8 is not on main. If D8
  lands (tracked via servicenow/freshdesk/crisp) before Amplitude's batch
  merges, adopt B: the two secrets are joined server-side into the same Basic
  pre-image (deriver unchanged), and region is projected to the runtime so
  heliox defaults `--region` from it instead of guessing. **Reconcile at batch
  time** with whatever multi-field face those siblings shipped — do not fork a
  parallel one. This is the same shared-surface discipline as the §5.1 deriver
  selector: one face for the batch, not one per tool.

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
| **L1** anycli unit | §3.5 httptest fakes: Basic header, region host, per-endpoint paths, `e=`/`s=` encoding, CSV→envelope, export→file receipt, **explicit-region 401 → `result.CredentialRejected==true`** and **default-US 401 → `CredentialRejected==false` + `--region eu` hint**, malformed→exit2, secret-never-leaked | No |
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
