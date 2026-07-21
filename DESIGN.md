# Novu — per-tool design (`heliox tool novu`)

Scratch design for the Novu tool provider, produced per the `helio-tool-provider`
pipeline and the 298-integrations master plan (row 271). Batch lead strips this
file at batch-end.

- **Catalog row 271**: Product Novu · anycli id `novu` · provider key `novu` ·
  auth lane `api_key` · wave 3 · category Marketing & Notifications.
- **Three naming axes** (all identical — no divergence, no `toolToProvider`
  entry needed): ① CLI word `novu` · ② anycli id `novu` · ③ provider key `novu`.
- **Go package**: `internal/tools/novu/` (id has no dash/leading digit).

## 1. Auth lane verification against official docs

**Verdict: `api_key` is correct.** Confirmed against the official Novu docs.

- OAuth audit row 273 assigns `api_key` ("no viable multi-tenant path"). Novu's
  own REST API auth is a **static per-environment secret key** presented on
  every request. There is no customer-facing authorization-code OAuth server for
  third parties to register one app that arbitrary Novu accounts authorize — the
  "OAuth" surfaces in Novu are (a) its own dashboard login and (b) *outbound*
  channel-provider credentials it stores for you, neither of which is a
  Helio-connectable authorize flow. So the audit verdict holds under the rubric.
- **Header/scheme (verified):** `Authorization: ApiKey <NOVU_SECRET_KEY>`. The
  docs explicitly warn it is the literal `ApiKey` prefix, **not** `Bearer`.
  (https://docs.novu.co/api-reference/authentication)
- **Key source:** Novu Dashboard → Developer → API Keys, per environment.
- **Scope:** the secret key is **environment-scoped** (Development / Production /
  custom). Each environment has its own secret key + application identifier;
  requests only touch resources in that environment.
- **Registration model:** none. The user pastes an existing secret key; there is
  no app to register, no client id/secret, no redirect URI. This is the same
  manual-single-secret shape as the shipped `mongodb` bundle and the
  api_key precedents (mixpanel / braze / lemlist / segment / fullstory).
- **Regions (matters):** base URL is region-specific — US `https://api.novu.co`
  (default) and EU `https://eu.api.novu.co`. A key is valid against exactly one
  region. The tool must let the user select region (see §3/§4); there is no way
  to derive region from the key itself.

## 2. Official API surface this tool wraps, and why

Novu is notification infrastructure: one `trigger` call fans a payload out to a
subscriber across the channels a workflow defines (in-app, email, SMS, push,
chat). An AI teammate's real jobs with Novu are: **send/notify** (trigger a
workflow to a person or an audience), **manage recipients** (subscribers +
their channel identifiers and preferences), **manage audiences** (topics),
**inspect delivery** (messages / activity feed), and **read config**
(workflows, integrations). The tool wraps the mature **v1 REST API** (base
`/v1`); v2 exists but v1 is the complete, stable surface for these jobs.

Command tree (resource-grouped, mirroring `internal/tools/notion/`):

| Group | Verbs → endpoint | Why (teammate job) |
|---|---|---|
| `event` | `trigger` → `POST /v1/events/trigger`; `bulk` → `POST /v1/events/trigger/bulk`; `broadcast` → `POST /v1/events/trigger/broadcast`; `cancel` → `DELETE /v1/events/trigger/{transactionId}` | The core action: send a notification by workflow id to a subscriber/topic/list. Output must surface the delivery **outcome**, not just acceptance — see the `status` semantics below. |
| `subscriber` | `list` (search) → `GET /v1/subscribers`; `get` → `GET /v1/subscribers/{id}`; `create` → `POST /v1/subscribers`; `update` → `PUT /v1/subscribers/{id}`; `delete` → `DELETE /v1/subscribers/{id}`; `preferences` → `GET /v1/subscribers/{id}/preferences`; `set-preferences` → `PATCH /v1/subscribers/{id}/preferences` | Manage recipients + channel identifiers (email/phone/deviceTokens) and opt-in state. |
| `topic` | `list` → `GET /v1/topics`; `create` → `POST /v1/topics`; `get` → `GET /v1/topics/{key}`; `add-subscribers` → `POST /v1/topics/{key}/subscribers`; `remove-subscribers` → `POST /v1/topics/{key}/subscribers/removal` | Audience grouping for broadcast-to-segment sends. |
| `workflow` | `list` → `GET /v1/workflows`; `get` → `GET /v1/workflows/{id}` | Read-only: discover the trigger identifiers `event trigger` needs. |
| `message` | `list` → `GET /v1/messages`; `delete` → `DELETE /v1/messages/{id}` | Delivery inspection (filter by channel / subscriber / transactionId). |
| `activity` | `list` → `GET /v1/notifications`; `get` → `GET /v1/notifications/{id}` | Activity feed / debugging a triggered run. |
| `integration` | `list` → `GET /v1/integrations`; `active` → `GET /v1/integrations/active` | Read-only: which channel providers are configured. |

Verified against `https://api.novu.co/openapi.json` (fetched 2026-07): `POST
/v1/events/trigger` (request `name`/`workflowId`, `to`, `payload`, `overrides`,
`transactionId`, `actor`, `tenant`; **201 response wrapped as `{"data":
TriggerEventResponseDto}`** with fields `acknowledged`, `status`, `error` (array
of strings), `transactionId`, `activityFeedLink`, `jobData`), `GET
/v1/environments` (**200 wrapped as `{"data": [EnvironmentResponseDto…]}`**), and
the environment identity endpoint (§4). The remaining paths follow Novu's
documented v1 resource conventions and are **confirmed at stage-1 dev against the
live API + `openapi.json`** before the branch leaves anycli (L2 is the gate).
Scope of v1 wrapped is read + notification-send; workflow authoring (create/sync)
is intentionally excluded — it is a build-time / SDK concern, not a teammate
runtime action.

**Trigger outcome semantics (matters for an AI teammate).** `acknowledged: true`
(and the `201`) mean only that Novu **accepted** the trigger — not that anything
was delivered. The load-bearing field is the `status` enum, whose verified values
(from `TriggerEventResponseDto` in `openapi.json`) are exactly:
`processed` (the success state) · `error` · `trigger_not_active` ·
`no_workflow_active_steps_defined` · `no_workflow_steps_defined` ·
`no_tenant_found` · `invalid_recipients`. A teammate that reads `acknowledged`/HTTP
`201` as "delivered" silently misses inactive-workflow, no-steps, invalid-recipient
and missing-tenant cases. `event trigger` output therefore surfaces `status`
together with `error[]` and `activityFeedLink` (from the unwrapped `data`), and the
L1 fake asserts the enum passes through — checking only `transactionId` would let a
`trigger_not_active` result read as success.

## 3. anycli definition (stage-1 rubric)

**Type: `service`.** No official Novu CLI exists that is non-interactive,
`--json`-capable, and image-provisionable — so the default `service` type
against the HTTP API applies (matches 21/23 shipped definitions). Implementation
lives in `internal/tools/novu/`.

`definitions/tools/novu.json`:

```json
{
  "name": "novu",
  "type": "service",
  "description": "Novu notification infrastructure as a tool (environment secret key)",
  "auth": {
    "credentials": [
      { "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "NOVU_SECRET_KEY"} },
      { "source": {"field": "api_base"},
        "inject": {"type": "env", "env_var": "NOVU_API_BASE"} }
    ]
  }
}
```

- **Credential fields (two):** `api_key` (the environment secret key, injected as
  `NOVU_SECRET_KEY`) and `api_base` (region base URL, injected as
  `NOVU_API_BASE`, defaulting to `https://api.novu.co` when empty). Region is a
  credential-map value, not a per-invocation flag, so the resolved connection
  carries it — this mirrors the braze/segment "region/DSN travels with the
  credential" precedent rather than the mongodb single-secret shape.
- **Auth header:** the service sets `Authorization: ApiKey <NOVU_SECRET_KEY>` on
  every request. Note the literal `ApiKey ` scheme prefix (not `Bearer`).
- **Service struct** follows `notion`: `BaseURL` (from `NOVU_API_BASE`), `HC`,
  `Out`, `Err`, cobra tree grouped by resource. Tests point `BaseURL` at an
  `httptest.Server` and assert request path, the `Authorization: ApiKey …`
  header, query/body shape, and both plain + `--json` error rendering.
- **JSON output shape:** provider-neutral, `--json`-first. Success prints the
  provider JSON, and unwrapping is **per-endpoint aware — never a blanket
  "unwrap `.data`"** (verified against `openapi.json`, which is genuinely mixed):
  - Single-resource and action responses wrap the payload in a top-level `data`
    object: `POST /v1/events/trigger` → `{"data": {…}}`, the environment
    identity endpoint (§4) → `{"data": {…}}`.
  - Collection endpoints wrap in `data` too but as an array with pagination
    siblings: `GET /v1/environments` → `{"data": […]}`; paginated list resources
    (`GET /v1/messages`, `GET /v1/notifications`) return a `{"data": […], "page",
    "pageSize", "totalCount", "hasMore"}` envelope.
  - A few endpoints (`GET /v1/integrations`) return a **bare** array with no
    `data` wrapper.

  So the service unwraps `data` where present and passes the bare shape through
  otherwise; each resource-group L1 fake asserts its own actual shape rather than
  a single global assumption. Exit-code contract identical to notion: `0`
  success, `1` runtime/API failure via typed `apiError` (with a `--json`
  structured error envelope), `2` usage/parse errors. List verbs pass through
  Novu's `page`/`limit` pagination flags.
- **No interactive prompts** (anycli AGENTS.md): every input is a flag —
  `--workflow`, `--to`, `--payload` (JSON string), `--subscriber-id`, `--email`,
  `--topic-key`, `--transaction-id`, etc.

TDD per anycli AGENTS.md: write `*_test.go` httptest fakes first (one per
resource group), then implement. Never hit the real API from a unit test; L2
harness (`ANYCLI_CRED_API_KEY=… ANYCLI_CRED_API_BASE=… anycli novu -- …`)
validates against the live API before the pin bump.

## 4. Helio provider bundle plan (`integrations/providers/novu/provider.yaml`)

Hidden-first (`presentation.visible: false`). Manual-secret shape with a region
selector and provider-side verification. Novu **does** expose an HTTPS identity
endpoint scoped to the presented key, so we verify rather than store-blind — but
verification is only available under the `manual_api_token` strategy on `main`
(`manual_credentials` hardwires the DSN-host deriver and does **no** HTTPS check —
see the strategy note below).

- **Naming (all identical):** ① `tool.command`/CLI word `novu` · ② `tool.name`
  `novu` · ③ key/dir `novu`. **No `toolToProvider` entry** — axes ② and ③ do not
  diverge.
- **`auth.type: credentials`** (wire `api-key`), `owner: individual`,
  `runtime_strategy: manual_api_token`. Rationale: on `main`,
  `manual_api_token` (`declarativeManualTokenVerifier`) is the only manual-secret
  strategy that runs a declarative verifier against `Identity.URL` + resolves
  identity via an RFC 6901 `StableKey` pointer before the Vault write;
  `manual_credentials` verifies nothing. `credential_input.fields`:
  - `api_key` — secret, required, label "Novu secret key", placeholder shows the
    Developer → API Keys origin; `setup_url: https://dashboard.novu.co` (Developer
    → API Keys).
  - `region` — enum `us` (default) / `eu`, non-secret. The choice selects the base
    URL (`us` → `https://api.novu.co`, `eu` → `https://eu.api.novu.co`) for both
    `Identity.URL` and the runtime `NOVU_API_BASE`. **Stage-1 flag (open):**
    `manual_api_token`'s P3 storage face is a single required secret, so a second
    input field + a region-derived base is the one bundle capability to resolve at
    stage 1 — either (a) `region` as a second non-secret input field feeding a
    region→base projection, or (b) two fixed-base regional variants (`novu` /
    `novu-eu`) if multi-field-plus-verifier is not jointly expressible on `main`.
    Verify against `main` before writing; do not assume the braze/segment
    multi-field precedent carries a verifier (it rides `manual_credentials`, which
    does not verify).
- **Identity / verification (revised — key-scoped `/environments/me`, not the
  org-wide list).** Verifier + identity source = the **current-environment**
  endpoint: `GET {api_base}/v1/environments/me` with `Authorization: ApiKey
  <key>`. This endpoint is key-scoped: it returns the **single** environment the
  presented secret belongs to, wrapped as `{"data": {…}}` with `_id`,
  `identifier`, `name`, `_organizationId` directly on the object (verified: it is
  what every official Novu SDK's `getCurrentEnvironment()` calls — e.g. the
  official PHP SDK does `GET environments/me` then reads `['data']`).
  - **Verifier:** a plain `200`-check on `/environments/me` — **no org-wide list,
    no `apiKeys[]` array-membership scan.** A `401`/`404` before any Vault write
    means a bad key (the existing `declarativeManualTokenVerifier` reject-before-
    persist path).
  - **`account_key` (stable):** the single returned environment's globally-unique
    `_id`, via one RFC 6901 pointer `StableKey: /data/_id` — expressible by the
    **existing** `declarativeIdentityResolver` with **zero** new deriver. Distinct
    envs of one org (dev vs prod) carry distinct `_id`s, so `_id` alone satisfies
    the "two keys of the same org must not collide" intent that previously forced
    the `<_organizationId>:<identifier>` composite. Label candidates
    `/data/name`, fallback `/data/identifier`.
    - *Correction to the review's mechanism:* the review suggested composing
      `<_organizationId>:<identifier>` via "the existing colon-split/JSON-pointer
      deriver (amplitude precedent)." That is **not** expressible on `main`:
      `StableKey` is a single pointer resolving to one value, and the amplitude
      deriver is a colon-**split** (decompose one value) — the opposite of the
      colon-**join** a composite would need. The single-pointer `_id` reaches the
      same outcome (no custom deriver) more directly, so the goal — deleting
      `novuEnvironmentIdentityDeriver` — stands; only the route differs.
  - **Verifier scheme:** the `Authorization: ApiKey <secret>` literal-prefix
    scheme. On `main` `APIKeyPolicy` carries only a `Header` name (the verifier
    injects the raw secret as that header's value), so the `ApiKey ` prefix is
    not yet expressible; this adds one reviewed scheme member (literal-prefix
    `ApiKey`, sibling to the `Bearer` tally added) — a narrow, orthogonal growth,
    not an adapter. Reuse it if a prior api_key tool already landed it on `main`.
- **`credential.fields` projection:** `api_key: token.access_token` (single secret
  through the existing `UpsertUserToken` write path — zero new `CredentialSource`),
  `api_base: <region-derived>`, `account_key: connection.account_key`. The
  region-derived base projection is the bundle capability flagged above; if the
  closed `CredentialSource` set cannot carry it, resolve via the two-variant
  option (b) rather than widening the source vocabulary.
- **`connection`**: `mode: isolated`, `disconnect_mode: local_only` (no
  provider-side token revoke — the user rotates the key in Novu).
- **No service adapter.** `manual_api_token` + the `ApiKey`-scheme verifier +
  declarative single-pointer identity covers it; no `service/adapter_novu.go`, no
  custom deriver.
- **Config Sync:** api_key lane needs **no** integration-service client
  id/secret, so there is nothing to append to `config/` + `deploy/` (Novu has no
  Helio-held OAuth app). `required_config_fields` is empty ⇒ `configured: true`
  with no env supply.

**Capability-growth summary (integration-service) — two items (was three; the
custom deriver is deleted):** (1) `ApiKey`-literal verifier scheme (the
`Authorization: ApiKey <secret>` prefix) if not already present on `main`;
(2) the region-derived base for `Identity.URL` + `NOVU_API_BASE` — resolved
either as a second non-secret input field with a region→base projection, or as
two fixed-base regional variants if multi-field-plus-verifier is not jointly
expressible on `main`. Identity itself needs **no** growth: `/environments/me`
returns a single env, so `StableKey: /data/_id` reuses the existing
`declarativeIdentityResolver` — the previously-planned
`novuEnvironmentIdentityDeriver` is gone. Both remaining items are reviewed
enum/projection additions behind the generator's closed contract — verify each
against `main` before writing, reuse if a prior api_key tool already added an
equivalent.

## 5. Test plan → five layers

| Layer | Concretely for Novu | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — httptest fakes per resource group (event/subscriber/topic/workflow/message/activity/integration); assert path, `Authorization: ApiKey …` header, `--to`/`--payload` body shape, `page`/`limit` passthrough, plain + `--json` error envelopes, exit codes 0/1/2. Two shape-specific asserts: (a) `event trigger` unwraps `{"data":…}` and **passes the `status` enum through** (e.g. a `trigger_not_active` fake must not render as success — checking only `transactionId` would miss it), plus `error[]`/`activityFeedLink`; (b) per-endpoint envelope handling — `data`-wrapped (environments/me, trigger, environments list), pagination-envelope (messages/notifications), and bare-array (integrations) fakes each assert their own shape, never a blanket unwrap. | No — fakes only. |
| **L2** harness real API | `ANYCLI_CRED_API_KEY=<dev-env secret> ANYCLI_CRED_API_BASE=https://api.novu.co anycli novu -- event trigger --workflow <id> --to <subscriberId> --payload '{}'`, plus `subscriber list`, `topic list`. **Gate the identity endpoint here:** confirm `GET /v1/environments/me` is live and returns `{"data":{…}}` with the key's single env (it is absent from `openapi.json` — legacy-but-live per the official SDKs; if L2 shows it gone, fall back to the org-list `/v1/environments` + `apiKeys[]` scan and reinstate the deriver). Also confirm the trigger `status` enum on an intentionally inactive workflow. Confirms unverified endpoint paths against the live v1 API + `openapi.json`. | **Yes** — a real Novu account + Development-environment secret key (free tier; self-serve). |
| **L3** generation + suites | From `go-services/integration-service`: `go run ./cmd/provider-gen` then `--check`; run helio-cli + integration-service unit suites (incl. the new deriver/verifier-scheme tests). On-branch only: local `replace` of anycli + local regen, **not committed** (batch lead owns the canonical regen). | No. |
| **L4** singleton + seed | `make run-singleton`; `POST /internal/test-only/connections/seed` with `provider":"novu"`, a real Novu secret key as `access_token`, against a real seeded assistant/org; then `heliox tool novu -- subscriber list`. Novu is a user-token (api_key) provider ⇒ seedable. Non-expiring key ⇒ seed `access_token` only, no `refresh_token`/`expires_at`. | **Yes** — same real secret key as L2. |
| **L5** full connect flow | Hidden-tool connect sweep (api_key key-entry path, per master plan §2): open connect link → paste secret key + pick region → stored via `POST /connections/credentials`, verified against `GET /v1/environments/me` → connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool novu -- event trigger …` succeeds through the real token gateway. Agent-drivable (api_key L5), human fallback on UI breakage. Run once before the visible flip. | **Yes** — real secret key + Novu account, entered through the real connect UI. |

**Externally-supplied credentials** gate L2, L4, L5 (all the same artifact: one
real Novu secret key from a self-serve free account's Development environment;
one EU-region key optional to prove region routing). L1 and L3 need none.

## 6. Rollout

Land hidden (all five projections regenerated together at batch-end by the batch
lead; on-branch regen/`replace`/local config are **uncommitted** and CI
`provider-gen --check` red on-branch is expected). Bump the anycli pin once the
Novu tag ships. Register UI icon (`ui/helio-app/src/integrations/icons/novu.svg`
+ `providerIcons.ts`) and the AI-facing sub-doc under
`agents/plugins/heliox/skills/tool/` at batch-end. Flip `visible: true` +
regenerate as the single go-live change only after L5 passes.

## Divergences from the prompt / catalog / review (recorded per instructions)

The catalog's `api_key` lane and the audit's "no viable multi-tenant path"
verdict for Novu are confirmed by the official docs. Catalog-uncaptured nuances
(handled in the bundle, not lane changes): (a) the **environment-scoped** (not
account-scoped) key, and (b) the **US/EU region** split needing a region selector
and region-derived base URL.

Corrections found while verifying the review findings against
`https://api.novu.co/openapi.json` (fetched 2026-07) and the official Novu SDKs —
recorded because they diverge from the review's stated facts:

1. **`/v1/environments/me` is adopted, but it is absent from the current
   `openapi.json`.** The review called it "verified against the official docs";
   the machine-readable spec does **not** list it (only `/v1/environments`
   list). It is nonetheless live — every official SDK's `getCurrentEnvironment()`
   calls `GET environments/me` (verified in the official PHP SDK:
   `$this->get("environments/me")['data']`). We adopt it as a legacy-but-live
   key-scoped endpoint and make **L2 the gate** that proves it before the branch
   ships (with the org-list + scan as the documented contingency, not a silent
   runtime fallback).

2. **Envelope shapes are the reverse of the review's claim.** The review said
   `/v1/environments` is a "bare top-level array … confirmed," with most other
   resources wrapping in `{"data":…}`. Per `openapi.json` **and** the PHP SDK,
   `/v1/environments` (list), `/v1/environments/me`, and `POST /v1/events/trigger`
   **all wrap in `{"data":…}`**; the genuinely bare/paginated endpoints are the
   list resources (`/v1/messages`, `/v1/notifications` → pagination envelope) and
   `/v1/integrations` (bare array). The correct, verified principle (per-endpoint
   unwrap, not blanket) survives; only the direction is fixed. §3 and the L1 plan
   now encode the real per-endpoint map.

3. **The account_key mechanism the review proposed is not expressible.** It
   suggested composing `<_organizationId>:<identifier>` with "the existing
   colon-split/JSON-pointer deriver (amplitude precedent)." On `main`,
   `declarativeIdentityResolver.StableKey` is a **single** RFC 6901 pointer, and
   the amplitude deriver is a colon-**split** (the inverse of a colon-**join**).
   Because `/environments/me` returns exactly one env, its globally-unique `_id`
   (`StableKey: /data/_id`) is a single-pointer stable key — reaching the review's
   goal (delete `novuEnvironmentIdentityDeriver`) more directly than the proposed
   route.

4. **The trigger `status` enum has seven values, not the review's four.**
   Verified from `TriggerEventResponseDto`: `processed` (success), `error`,
   `trigger_not_active`, `no_workflow_active_steps_defined`,
   `no_workflow_steps_defined`, `no_tenant_found`, `invalid_recipients`. §2 and
   the L1 assertion carry the full set, including the omitted
   `no_workflow_steps_defined`.

5. **Strategy correction.** The prior draft put the verifier on
   `runtime_strategy: manual_credentials`; on `main` that strategy runs **no**
   HTTPS verification (hardwired DSN-host deriver). Verification + declarative
   identity is a `manual_api_token` capability, so the bundle moves to
   `manual_api_token`; the region second-field interaction with its single-secret
   storage face is the one item flagged for stage-1 resolution.

6. **Subscribers / topics / workflows are now `/v2`, not `/v1` (implementation
   divergence, verified at stage 1 against `api.novu.co/openapi.json`
   2026-07-22).** The design said "the tool wraps the mature v1 REST API (base
   `/v1`)." The current machine-readable spec shows Novu has **moved** the CRUD
   surfaces for these three resources to `/v2` (`/v2/subscribers`, `/v2/topics`,
   `/v2/workflows`); the v1 forms are absent from the spec. Events, messages,
   notifications, integrations, and environments stay `/v1`. The tool therefore
   wraps a **mixed** surface, so the injected `NOVU_API_BASE` is the region
   **host only** (no `/v1`), and each command builds its own versioned path
   (`/v1/events/trigger`, `/v2/subscribers`, …). This is a code-shape change from
   the notion precedent (whose `BaseURL` bakes in `/v1`), not a lane or auth
   change. L2 confirms the v1 forms are truly retired (not merely undocumented)
   before the pin bump.

7. **Verbatim passthrough instead of per-endpoint unwrap (simplification).** §3
   proposed a per-endpoint envelope map (unwrap `data` where present, pass bare
   shapes through). Implementation instead passes **every** response through
   verbatim — matching the notion/bitly precedent exactly — because a blanket
   "unwrap top-level `data`" is provably wrong for the pagination endpoints
   (`/v1/messages`, `/v1/notifications` carry a `data` array **plus** `page`/
   `hasMore` siblings that unwrapping would drop), and verbatim loses no
   information for any endpoint. The trigger outcome requirement is still met:
   the `{"data":{status,error,activityFeedLink,…}}` envelope is emitted whole, so
   `status` is visible; the L1 test `TestEventTriggerSurfacesNonProcessedStatus`
   asserts a `trigger_not_active` result is not masked. This deletes the
   envelope-map maintenance burden and one class of unwrap bugs — a subtract, not
   an add.

8. **Trigger `to` envelope wrap confirmed.** The rendered docs page claimed the
   trigger `201` is **not** wrapped; `openapi.json` (authoritative) shows it **is**
   `{"data": TriggerEventResponseDto}`. §2's wrapped-response statement stands; the
   docs-page reading was wrong.
