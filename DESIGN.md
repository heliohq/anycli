# PhantomBuster — per-tool design (`heliox tool phantombuster`)

**Catalog row:** #72 · anycli id `phantombuster` · provider key `phantombuster` · auth lane
`api_key` · Wave 2 · category Sales Engagement (master plan §4).

**Audit verdict (row 74, oauth-audit.md):** `no viable multi-tenant path` → **stays
`api_key`**. Independently confirmed against the official docs: PhantomBuster has **no
customer-facing authorization-code OAuth flow at all**. Its only credential is a
per-workspace **API key** minted in Workspace settings; there is no `authorize`/`token`
endpoint, no client registration, no consent screen. The rubric (a shared multi-tenant
authorization-code app) is unmeetable, so `api_key` is correct with **high confidence** —
no divergence from the catalog to record.

## 1. Official API surface & why these endpoints

**Provider:** PhantomBuster — cloud automation platform. A "Phantom" is a saved automation
(LinkedIn/social/web scraping, lead extraction, enrichment, outreach). Runs are
asynchronous: launching a Phantom queues a **container** (one execution) and returns a
container id, not a result; you then poll incremental output and fetch the structured
result separately.

**What an AI teammate actually does with PhantomBuster** (drives the endpoint choice):
1. Discover which Phantoms exist in the workspace ("what automations can I run?").
2. Launch a Phantom, optionally overriding its argument JSON (e.g. a list of profile
   URLs to scrape).
3. Watch the run — poll status + incremental console output until it reaches a terminal
   state.
4. Fetch the structured result of a run (the extracted rows).
5. Check remaining quota/execution-time **before** launching (a launch over quota fails
   `429` mid-run with no recoverable partial result — a real footgun the wrapper should
   let the AI avoid).

**Base URL:** `https://api.phantombuster.com/api/v2/` — HTTPS, JSON.

**Auth:** API key in the **`X-Phantombuster-Key`** HTTP request header (this is the **v2**
header; the deprecated v1 header `X-Phantombuster-Key-1` is NOT used — v2 only). The key
grants **full workspace/org access** (no scoping); this must be surfaced in the connect
drawer copy and the AI-facing doc so users understand the blast radius before pasting.

**Timestamp caveat (worth a wrapper normalization note):** v2 date/time fields are
Unix ms; v1 were seconds. We wrap v2 only, so ms throughout.

**Endpoints wrapped** (chosen to cover the five behaviors above, nothing speculative):

| Verb + path (under `/api/v2`) | Purpose | Maps to subcommand |
|---|---|---|
| `GET /orgs/fetch` | Current org identity (id/name) | connect-time identity verify (Helio side) + `org get` |
| `GET /orgs/fetch-resources` | Org resources + **usage/quota** | `org resources` |
| `GET /agents/fetch-all` | List all Phantoms in the org (filterable by `inputTypes`/`outputTypes`/`agentIds`) | `agent list` |
| `GET /agents/fetch` | One Phantom by `id` (returns `orgS3Folder` + `s3Folder` needed to build result URLs) | `agent get` |
| `POST /agents/launch` | Queue a run; body `{ id, argument?, saveArgument? }`; returns `containerId` | `agent launch` |
| `GET /agents/fetch-output` | Most-recent container's incremental console + `resultObject`, offset via `fromOutputPos` | `agent output` |
| `POST /agents/abort` | Abort a Phantom's running container(s) | `agent abort` |
| `GET /containers/fetch-all` | All runs for a Phantom (query is `agentId`, per the docs' noted exception) | `container list` |
| `GET /containers/fetch` | One run by container `id` (status, timing, exit code) | `container get` |
| `GET /containers/fetch-output` | Incremental output for a specific container, offset via `fromOutputPos` | `container output` |
| `GET /containers/fetch-result-object` | Structured result object of a specific run | `container result` |
| `GET /users/fetch-me` | Current user info | `me` |

**Deliberately out of scope for v1 of the wrapper** (documented, not built): agent
create/update/delete (`/agents/save` — mutating a Phantom's script/config is a
power-user authoring task, not teammate work), CRM contact saving (`/orgs/save-crm-*`,
requires an active HubSpot integration), agent-group management, and **workflows** (the
provider explicitly does not allow launching workflows via API — the AI chains individual
Phantoms instead). Keeping the first cut read + launch + observe keeps the JSON contract
small and the L2 surface testable with one paid account.

> **Result files (`.csv`/`.json`):** `fetch-output`/`fetch-result-object` return the
> structured `resultObject` inline, which is what the AI consumes. The separate
> S3-file path (`GET /agents/fetch` → `orgS3Folder`+`s3Folder` → construct
> `https://phantombuster.s3.amazonaws.com/<orgS3Folder>/<s3Folder>/result.(csv|json)`)
> is exposed as fields on `agent get` output so the AI can hand a human a download link,
> but the wrapper does not itself download the S3 blob (it is an unauthenticated public
> URL, outside the credential path).

## 2. anycli definition & service

### 2.1 Tool form: `service` type (stage-1 rubric)

`service` type — there is no official PhantomBuster CLI binary to wrap; the integration is
the HTTP API. (This matches 21/23 shipped definitions.) No binary provisioning, no
`source`/`binary` fields.

- **anycli id (axis ②):** `phantombuster` — `definitions/tools/phantombuster.json`.
- **Go package (stage-2 rule):** `internal/tools/phantombuster/` — id has no dashes and no
  leading digit, so the package name equals the id. Registered
  `RegisterService("phantombuster", &phantombuster.Service{})` in
  `internal/tools/register.go` (batch-end shared-file merge).
- **provider key (axis ③):** `phantombuster`. **No ②↔③ divergence → no
  `toolToProvider` entry** and no `toolGroups` entry (flat command).
- **CLI command word (axis ①):** `phantombuster` (flat; not a grouped family).

### 2.2 Command tree (resource-grouped, notion precedent)

```
phantombuster agent list        [--input-types <t,...>] [--output-types <t,...>] [--ids <id,...>] [--with-argument]
phantombuster agent get         --id <agentId>
phantombuster agent launch      --id <agentId> [--argument '<json>'] [--save-argument]
phantombuster agent output      --id <agentId> [--from-pos <n>]
phantombuster agent abort       --id <agentId>
phantombuster container list    --agent-id <agentId>
phantombuster container get     --id <containerId>
phantombuster container output  --id <containerId> [--from-pos <n>]
phantombuster container result  --id <containerId>
phantombuster org get
phantombuster org resources
phantombuster me
```

Copy `internal/tools/notion/`'s shape: a cobra tree grouped by resource; a
`BaseURL`/`HC`/`Out`/`Err` struct so unit tests point `BaseURL` at an `httptest.Server`
and capture stdout/stderr; a typed `apiError` carrying HTTP status; the documented
exit-code contract **0** success / **1** runtime/API failure / **2** usage/parse error;
and a `--json` structured-error envelope on every command.

### 2.3 JSON output shape

`--json` (default-on for AI callers, per house convention) emits a provider-neutral
envelope, not the raw PhantomBuster body:

```jsonc
// success (agent launch)
{ "ok": true, "data": { "container_id": "1234567890123456", "agent_id": "987654321" } }
// success (agent list) — arrays under data.items with a stable subset of fields
{ "ok": true, "data": { "items": [ { "id": "...", "name": "...", "script": "...",
    "last_end_status": "...", "org_s3_folder": "...", "s3_folder": "..." } ] } }
// error
{ "ok": false, "error": { "code": "api_error", "status": 429,
    "message": "org execution-time quota exceeded" } }
```

- Terminal-state detection for `container get`/`agent output` normalizes the provider's
  status into an explicit `data.status` + `data.is_running` boolean so the AI's poll loop
  has a stable field, not a provider-versioned string.
- `--from-pos` echoes back the next `data.output_pos` cursor so incremental polling is a
  documented loop, not guesswork.
- Timestamps passed through as provider ms plus an ISO-8601 mirror
  (`*_at_iso`) for readability.

### 2.4 Credentials & exact auth flow

- **Definition `auth`:** one `CredentialBinding` — `source.field: api_key` injected as
  `inject.type: env, env_var: PHANTOMBUSTER_API_KEY`. The service reads that env var and
  sets the `X-Phantombuster-Key` header on every request. (Field name `api_key` matches
  the Helio bundle's `credential.fields` mapping — §3.)
- **No refresh cycle:** the API key is a long-lived, non-expiring workspace secret. There
  is no OAuth exchange, no refresh token, no token endpoint. anycli stays credential-shape
  agnostic — it receives one field from the resolver and injects it; it never learns the
  key is "an API key" vs "a bearer token".

```
ANYCLI_CRED_API_KEY=<workspace-key> anycli phantombuster -- org resources        # L2 harness
```

## 3. Helio provider bundle plan (`integrations/providers/phantombuster/provider.yaml`)

**`api_key` path — no new integration-service Go, but only because the identity value is
a JSON string (verified, not assumed).** PhantomBuster exposes an HTTPS identity endpoint
reachable with the same header, so it uses `runtime_strategy: manual_api_token` and the
built-in `declarativeManualTokenVerifier`
(`go-services/integration-service/service/manual_token_verifier.go`): at connect time the
service GETs `identity.url` with header `auth.api_key.header = X-Phantombuster-Key`, then
extracts the stable account key + label via JSON pointers. This is a strictly better fit
than the `mongodb` `manual_credentials`/`dsnHostIdentityDeriver` path (which does **no**
provider-side verification) — PhantomBuster CAN verify the key at connect time, so a bad
key is rejected immediately with `invalid_provider_credential`, not deferred to first use.

**Load-bearing caveat — the verifier extracts JSON *strings* only.** Both the connect-time
verifier and the OAuth-side `declarativeIdentityResolver` go through `jsonPointerString`
(`service/declarative_identity.go`), which does `value.(string)`: a JSON *number* at the
stable key yields `ok=false` and the connect verify fails with `identity has no string
value at stable key`. PhantomBuster uses very large numeric-looking ids (e.g.
`2008952215470815`), so the "is `id` a string or a number on the wire?" axis is as
decisive as the envelope axis — and, unlike envelope shape, it is **not** a bundle
one-liner: a JSON-number id would require the numeric-stable-key integration-service
capability (the exact work HubSpot needed), not a pointer edit. This tool avoids that only
because the official OpenAPI schema types `orgs/fetch.id` (and `users/fetch-me`'s
`user.id`) as `"string"` — verified below. **If** a live L2 `orgs/fetch` ever returns `id`
as a bare number (schema drift), fall back to a guaranteed string-valued field or gate the
flip on the numeric-stable-key capability being merged to `main`.

Hidden-first (`presentation.visible: false`) until the anycli pin ships the tool and
L1–L5 pass.

```yaml
schema: helio.provider/v1
key: phantombuster
go_name: PhantomBuster

presentation:
  name: PhantomBuster
  description_key: phantombuster
  consent_domain: phantombuster.com
  visible: false            # flip only as the single go-live change after L5
  order: <pick an unoccupied slot at flip time>

auth:
  type: api_key
  owner: individual
  api_key:
    header: X-Phantombuster-Key
    # Points at the current "Retrieve Your Results Via API" article, which both
    # (a) mints the key from Workspace settings → API Keys and (b) uses the v2
    # header X-Phantombuster-Key — matching the bundle header above. Do NOT link
    # article 4401916698130 ("Get Started with the PhantomBuster API"): its body
    # still instructs the deprecated v1 header X-Phantombuster-Key-1, which would
    # contradict this bundle and the L2 header check (see header note below).
    setup_url: https://support.phantombuster.com/hc/en-us/articles/23117755693458-Retrieve-Your-Results-Via-API
  credential_input:
    fields:
      - name: api_key
        label_key: phantombuster_api_key
        secret: true
        required: true
        placeholder: "Your PhantomBuster API key (Workspace settings → API key)"

identity:
  source: userinfo
  # Official OpenAPI schema (hub.phantombuster.com, get_orgs-fetch): body is a
  # RAW object (no {status,data} envelope) and `id` is typed "string" — so the
  # string-only declarative verifier extracts it directly. See the two-axis
  # verification note below (envelope AND string-vs-number both resolved).
  url: https://api.phantombuster.com/api/v2/orgs/fetch
  stable_key: /id                      # raw envelope, string-typed id (verified)
  label_candidates: [/name, /id]

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
    api_key: token.access_token        # pasted key stored in the user-token access_token slot
    account_key: connection.account_key

tool:
  name: phantombuster
  kind: api-key
```

### Naming axes (master plan §3)
- ① CLI word: `phantombuster` · ② anycli id: `phantombuster` · ③ provider key:
  `phantombuster` — all identical; **no resolver / no group** entries.

### Identity verification — two axes, both closed against the OpenAPI schema
The `hub.phantombuster.com/reference` HTML pages render response schemas client-side and
hide field names, so the `identity` pointers were resolved against the machine-readable
OpenAPI schema behind those pages (`get_orgs-fetch` / `get_users-fetch-me`, reachable as
the `.md` twins and indexed from `hub.phantombuster.com/llms.txt`). Two independent axes
had to close, **not one**:

1. **Envelope (raw vs `{status,data}`)** — decides `/id` vs `/data/id`. *Verified:*
   `orgs/fetch` returns a **raw** object, no envelope → `stable_key: /id`. This axis, if it
   had gone the other way, would indeed be a one-line pointer edit.
2. **Value type (JSON string vs number)** — decides whether the string-only verifier works
   at all (see the load-bearing caveat above). *Verified:* the schema types top-level `id`
   as `"string"` in `orgs/fetch` and `user.id` as `"string"` in `users/fetch-me`, even
   though the ids are large integers on screen. So `/id` extracts cleanly with **zero** new
   Go. This axis is **not** a pointer edit if it inverts — a number would require the
   numeric-stable-key capability.

Because L4 seed bypasses the verifier entirely, neither axis is exercised until L5. To
avoid a late, unbudgeted L5 failure, **L2 must run a live `orgs/fetch` and assert `id`
comes back JSON-string-typed** (not just "some id is present"), confirming the schema
matches production. Fallback identity endpoint if `orgs/fetch` proves unsuitable:
`users/fetch-me` (`/user/id`, label from `/user/firstName` or `/user/email`) — same
string-typing verified.

### Auth-header reconciliation (v2 vs v1)
The bundle header `X-Phantombuster-Key` is the **v2** header; the deprecated v1 header is
`X-Phantombuster-Key-1`. This is confirmed by the **current** "Retrieve Your Results Via
API" article (the `setup_url` above) and the live OpenAPI reference, **not** by the older
"Get Started with the PhantomBuster API" article (4401916698130), whose body still shows
`X-Phantombuster-Key-1`. `setup_url` deliberately links the former so the connect-drawer
doc and the L2 header assertion agree; if the linked article is ever swapped, keep it on a
page that uses `X-Phantombuster-Key` so the doc never contradicts the bundle.

### Config Sync
No integration-service **config** is needed: `api_key` providers carry **no**
`oauth.client_id`/`client_secret` (`required_config_fields` empty), so there is nothing to
land in `config/` or the `deploy/` Helm Secret, and the provider renders
`configured: true` with an empty config. (This is the seventh shared surface for
oauth tools only; PhantomBuster skips it entirely.)

### Icon & docs (batch-end shared surfaces)
- `ui/helio-app/src/integrations/icons/phantombuster.svg` + register in
  `providerIcons.ts` (manual).
- i18n: `tools.desc.phantombuster` + `phantombuster_api_key` label across locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` — emphasize the async
  launch→poll→result loop, the quota pre-check, and the **full-org-access** key scope.

## 4. Test plan → five layers

| Layer | What proves it here | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — `httptest.Server` fakes for each endpoint; assert request path, `X-Phantombuster-Key` header injection, `--argument` JSON body on launch, `fromOutputPos` cursor round-trip, terminal-status normalization, and both plaintext + `--json` error envelopes (401/429/400). Never hits the real API. | No |
| **L2** harness real-API | `ANYCLI_CRED_API_KEY=<key> anycli phantombuster -- org resources` / `agent list` / a real `agent launch` + `container get` poll against a live paid workspace. Confirms header name, field names, envelope shape, and the launch→poll→result contract match reality. | **Yes** — one **paid** PhantomBuster workspace API key (API is paid-plans-only) from the account pool |
| **L3** generation + suites | `provider-gen --check` (bundle strict-decode + closed-enum + directory-key equality) and both repos' unit suites (helio-cli `go test ./cmd/heliox/cmds/tool/`, integration-service). On-branch: local `provider-gen`, `go.mod` `replace` → anycli branch, both green. Not committed. | No |
| **L4** singleton + seed | Start singleton (`env: dev`), `POST /internal/test-only/connections/seed` with `provider: phantombuster`, a **real** workspace key as `access_token` (api_key providers are seedable; no `refresh_token`/`expires_at` — non-expiring key served directly by the token gateway), real seeded `org_id`/`owner_user_id`/`assistant_id`; then `heliox tool phantombuster -- org resources` returns live data through the real token gateway. | **Yes** — same paid workspace key |
| **L5** connect flow | Per master plan §2, this is the **api_key key-entry** path, not OAuth: open the connect link → paste the key in the real connect UI (`POST /connections/credentials`, which triggers the declarative verify against `orgs/fetch`) → connection shows connected/`configured` in `GET /connections` → one **unseeded** live `heliox tool phantombuster -- org resources` succeeds through the token gateway. Agent-drivable (agent-browser) with human fallback. Run once, hidden, before the visible flip. | **Yes** — same paid workspace key; the L4 seed bypass cannot substitute (it skips the connect UI + verify) |

**Externally-supplied-credential layers: L2, L4, L5** — all satisfied by a single paid
PhantomBuster workspace API key (procured one wave ahead, master plan lane 2). No app
registration, no review clock, no OAuth consent session — the whole `api_key` lane's
throughput advantage.

**Definition of done:** L1–L5 green · docs published · icon registered · then
`presentation.visible: true` + `provider-gen` regenerate as the single go-live change.
