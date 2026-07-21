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
| `event` | `trigger` → `POST /v1/events/trigger`; `bulk` → `POST /v1/events/trigger/bulk`; `broadcast` → `POST /v1/events/trigger/broadcast`; `cancel` → `DELETE /v1/events/trigger/{transactionId}` | The core action: send a notification by workflow id to a subscriber/topic/list. |
| `subscriber` | `list` (search) → `GET /v1/subscribers`; `get` → `GET /v1/subscribers/{id}`; `create` → `POST /v1/subscribers`; `update` → `PUT /v1/subscribers/{id}`; `delete` → `DELETE /v1/subscribers/{id}`; `preferences` → `GET /v1/subscribers/{id}/preferences`; `set-preferences` → `PATCH /v1/subscribers/{id}/preferences` | Manage recipients + channel identifiers (email/phone/deviceTokens) and opt-in state. |
| `topic` | `list` → `GET /v1/topics`; `create` → `POST /v1/topics`; `get` → `GET /v1/topics/{key}`; `add-subscribers` → `POST /v1/topics/{key}/subscribers`; `remove-subscribers` → `POST /v1/topics/{key}/subscribers/removal` | Audience grouping for broadcast-to-segment sends. |
| `workflow` | `list` → `GET /v1/workflows`; `get` → `GET /v1/workflows/{id}` | Read-only: discover the trigger identifiers `event trigger` needs. |
| `message` | `list` → `GET /v1/messages`; `delete` → `DELETE /v1/messages/{id}` | Delivery inspection (filter by channel / subscriber / transactionId). |
| `activity` | `list` → `GET /v1/notifications`; `get` → `GET /v1/notifications/{id}` | Activity feed / debugging a triggered run. |
| `integration` | `list` → `GET /v1/integrations`; `active` → `GET /v1/integrations/active` | Read-only: which channel providers are configured. |

Verified endpoints: `POST /v1/events/trigger` (request `name`/`workflowId`,
`to`, `payload`, `overrides`, `transactionId`, `actor`, `tenant`; response
`acknowledged`, `status`, `transactionId`), `GET /v1/subscribers`, and
`GET /v1/environments` (§3). The remaining paths follow Novu's documented v1
resource conventions and are **confirmed at stage-1 dev against the live API +
`https://api.novu.co/openapi.json`** before the branch leaves anycli (L2 is the
gate). Scope of v1 wrapped is read + notification-send; workflow authoring
(create/sync) is intentionally excluded — it is a build-time / SDK concern, not
a teammate runtime action.

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
  provider JSON (envelope-unwrapped where Novu wraps in `{"data": …}`). Exit-code
  contract identical to notion: `0` success, `1` runtime/API failure via typed
  `apiError` (with a `--json` structured error envelope), `2` usage/parse errors.
  List verbs pass through Novu's `page`/`limit` pagination flags.
- **No interactive prompts** (anycli AGENTS.md): every input is a flag —
  `--workflow`, `--to`, `--payload` (JSON string), `--subscriber-id`, `--email`,
  `--topic-key`, `--transaction-id`, etc.

TDD per anycli AGENTS.md: write `*_test.go` httptest fakes first (one per
resource group), then implement. Never hit the real API from a unit test; L2
harness (`ANYCLI_CRED_API_KEY=… ANYCLI_CRED_API_BASE=… anycli novu -- …`)
validates against the live API before the pin bump.

## 4. Helio provider bundle plan (`integrations/providers/novu/provider.yaml`)

Hidden-first (`presentation.visible: false`). Manual-single-secret shape with a
region selector and provider-side verification (Novu, unlike mongodb, exposes an
HTTPS identity endpoint, so we verify rather than store-blind).

- **Naming (all identical):** ① `tool.command`/CLI word `novu` · ② `tool.name`
  `novu` · ③ key/dir `novu`. **No `toolToProvider` entry** — axes ② and ③ do not
  diverge.
- **`auth.type: credentials`** (wire `api-key`), `owner: individual`,
  `runtime_strategy: manual_credentials`. `credential_input.fields`:
  - `api_key` — secret, required, label "Novu secret key", placeholder shows the
    Developer → API Keys origin; `setup_url: https://dashboard.novu.co` (Developer
    → API Keys).
  - `region` — required enum `us` (default) / `eu`, non-secret. The bundle maps
    the choice to the base URL projected into the credential map as `api_base`
    (`us` → `https://api.novu.co`, `eu` → `https://eu.api.novu.co`). This is the
    braze/mixpanel "region field feeds a derived base" precedent.
- **Identity / verification.** Verifier + identity source = the environment
  endpoint: `GET {api_base}/v1/environments` with `Authorization: ApiKey <key>`;
  `200` (a non-empty array of `EnvironmentResponseDto`) ⇒ valid key. Each element
  carries `_id`, `identifier`, `name`, `_organizationId`, and `apiKeys[].key`.
  - **`account_key` (stable):** the environment whose `apiKeys[].key` equals the
    presented secret, keyed as `<_organizationId>:<identifier>` (org + env — two
    keys of the same org, dev vs prod, must not collide). Because the list is
    org-wide and the match is by array membership, an RFC-6901 JSON-pointer
    deriver cannot express it: this needs a small **`novuEnvironmentIdentityDeriver`**
    in integration-service (precedent: crisp keypair deriver, amplitude
    colon-split deriver, braze DSN-host deriver — a reviewed named deriver, not
    a new strategy). Label candidate = environment `name` (fallback `identifier`).
  - **Verifier scheme:** the `Authorization: ApiKey <secret>` custom scheme is a
    reviewed verifier-scheme enum value; if the current verifier capability only
    knows `Bearer`/`Basic` (tally added `Bearer`), this adds one enum member
    (`api_key_scheme` / literal-prefix `ApiKey`) — a narrow, orthogonal growth,
    not an adapter.
- **`credential.fields` projection:** `api_key: token.access_token` (single secret
  through the existing `UpsertUserToken` write path — zero new `CredentialSource`),
  `api_base: <region-derived>`, `account_key: connection.account_key`. If the
  generator's closed source set lacks a way to project the region-derived base as
  a credential value, that projection is the one reviewed capability the bundle
  needs — flag at stage 1.
- **`connection`**: `mode: isolated`, `disconnect_mode: local_only` (no
  provider-side token revoke — the user rotates the key in Novu).
- **No service adapter.** `manual_credentials` + verifier + named identity
  deriver covers it; no `service/adapter_novu.go`.
- **Config Sync:** api_key lane needs **no** integration-service client
  id/secret, so there is nothing to append to `config/` + `deploy/` (Novu has no
  Helio-held OAuth app). `required_config_fields` is empty ⇒ `configured: true`
  with no env supply.

**Capability-growth summary (integration-service):** (1) `ApiKey`-literal
verifier scheme if not already present; (2) `novuEnvironmentIdentityDeriver`;
(3) region→base credential projection if not expressible in the closed source
set. All three are reviewed enum/named-function additions, hidden behind the
generator's closed contract — verify each against `main` before writing, reuse
if a prior api_key tool already added an equivalent.

## 5. Test plan → five layers

| Layer | Concretely for Novu | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` — httptest fakes per resource group (event/subscriber/topic/workflow/message/activity/integration); assert path, `Authorization: ApiKey …` header, `--to`/`--payload` body shape, `page`/`limit` passthrough, plain + `--json` error envelopes, exit codes 0/1/2. | No — fakes only. |
| **L2** harness real API | `ANYCLI_CRED_API_KEY=<dev-env secret> ANYCLI_CRED_API_BASE=https://api.novu.co anycli novu -- event trigger --workflow <id> --to <subscriberId> --payload '{}'`, plus `subscriber list`, `topic list`, `environments`-backed verify. Confirms unverified endpoint paths against the live v1 API + `openapi.json`. | **Yes** — a real Novu account + Development-environment secret key (free tier; self-serve). |
| **L3** generation + suites | From `go-services/integration-service`: `go run ./cmd/provider-gen` then `--check`; run helio-cli + integration-service unit suites (incl. the new deriver/verifier-scheme tests). On-branch only: local `replace` of anycli + local regen, **not committed** (batch lead owns the canonical regen). | No. |
| **L4** singleton + seed | `make run-singleton`; `POST /internal/test-only/connections/seed` with `provider":"novu"`, a real Novu secret key as `access_token`, against a real seeded assistant/org; then `heliox tool novu -- subscriber list`. Novu is a user-token (api_key) provider ⇒ seedable. Non-expiring key ⇒ seed `access_token` only, no `refresh_token`/`expires_at`. | **Yes** — same real secret key as L2. |
| **L5** full connect flow | Hidden-tool connect sweep (api_key key-entry path, per master plan §2): open connect link → paste secret key + pick region → stored via `POST /connections/credentials`, verified against `GET /v1/environments` → connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool novu -- event trigger …` succeeds through the real token gateway. Agent-drivable (api_key L5), human fallback on UI breakage. Run once before the visible flip. | **Yes** — real secret key + Novu account, entered through the real connect UI. |

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

## Divergences from the prompt / catalog (recorded per instructions)

None. Official docs confirm the catalog's `api_key` lane and the audit's
"no viable multi-tenant path" verdict for Novu. The only nuances the catalog row
does not capture — both handled in the bundle, not lane changes — are (a) the
**environment-scoped** (not account-scoped) key and (b) the **US/EU region**
split requiring a region selector and region-derived base URL.
