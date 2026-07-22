# Sprout Social — per-tool integration design

Scratch design for the `tool/sprout-social` branch (anycli + Helio). Stripped at
batch end by the batch lead. English-only per anycli AGENTS.md.

- **anycli id (axis ②):** `sprout-social`  → Go package `internal/tools/sproutsocial/`
- **provider catalog key (axis ③):** `sprout_social`  → `integrations/providers/sprout_social/`
- **CLI command word (axis ①):** unset → defaults to `sprout-social` (flat, no `tool.group`); invoked `heliox tool sprout-social -- …`
- **auth lane:** `api_key` · **wave:** 3-hold · **category:** Social & Media · catalog row 209
- **tool form:** `service` (HTTP API; no official CLI binary)

## 0. Audit / catalog reconciliation (verified against official docs)

The master-plan catalog (row 209) and the OAuth audit (row 211, "no viable
multi-tenant path → api_key") both classify Sprout Social as **api_key**. The
official docs confirm and refine this:

- Sprout's public API supports **two** token types on the same `Authorization:
  Bearer` header: an **Account-scoped API Access Token** (created in-app) and an
  OAuth 2.0 JWT. The OAuth provider is **not** a self-serve, multi-tenant
  authorization-code app any customer can authorize against one registered
  Helio client — it sits behind the same rep-provisioned partner gate as API
  access. So the audit verdict holds: **api_key is the correct lane**; we use the
  account-scoped API Access Token, a single long-lived bearer secret the user
  pastes. No divergence to record.
- **3-hold driver confirmed (account procurement):** API access requires (a) an
  appropriate paid plan (Advanced/Premium tier) **and** (b) per-account API
  authorization granted by a Sprout account representative, **and** (c) accepting
  the Analytics API Terms of Service in Settings. There is no self-serve free
  tier. This is exactly the §6 "no self-serve or paid-only API tier" risk that
  re-laned the tool to 3-hold; it gates L2 and L5 (test-account pool), not the
  code. The pre-verify gate for this tool is **account-pool feasibility**, not
  API feasibility (the API itself is well-documented and integrable).

Sources: https://api.sproutsocial.com/docs/ · https://sproutsocial.com/insights/sprout-social-api/

## 1. API surface wrapped, and why

Base URL `https://api.sproutsocial.com`. Every path is
`/v1/<customer_id>/<resource>` **except** the discovery endpoint
`/v1/metadata/client`, which is the only call that does not carry a customer id
(it *returns* the customer ids the token can see). Rate limit 60 req/min,
250k/month. All responses are a JSON envelope `{ "data": …, "paging"?: …,
"error"?: … }`.

An AI teammate embedded in a social team does three jobs with Sprout: **read
performance analytics**, **triage the shared inbox**, and **draft scheduled
posts**. The wrapped surface is scoped to exactly those, plus the metadata reads
needed to resolve the ids those calls require:

| Job | Endpoints wrapped | Method |
|---|---|---|
| Discovery (bootstrap) | `/v1/metadata/client` | GET |
| Account metadata | `/v1/{cid}/metadata/customer` (profiles), `…/customer/tags`, `…/groups`, `…/users`, `…/topics`, `…/teams`, `…/queues` | GET |
| Analytics | `/v1/{cid}/analytics/profiles`, `/v1/{cid}/analytics/posts` | POST |
| Inbox | `/v1/{cid}/messages` | POST |
| Publishing | `/v1/{cid}/publishing/posts` (create draft), `/v1/{cid}/publishing/posts/{id}` (get) | POST / GET |
| Cases | `/v1/{cid}/cases/filter` | POST |

Analytics/messages/cases are POST-with-a-filter-body endpoints (Sprout's filter
DSL, e.g. `created_time.in(2026-01-01…2026-02-01)`, plus `metrics`, `fields`,
`page`). We do **not** model the DSL in flags — that would be a lossy
re-implementation the AI would fight. Instead each POST verb takes thin
ergonomic flags (`--filter` repeatable, `--metric` repeatable, `--fields`,
`--page`) **and** a `--body <json>` raw-passthrough escape hatch, so the AI can
send any documented Sprout query body verbatim. Media upload, listening topic
metrics, and paid/ad data are **out of scope for v1** (media upload is a
multi-part binary flow with little teammate value; ads data is explicitly
unavailable via this API).

## 2. anycli definition (`definitions/tools/sprout-social.json` + `internal/tools/sproutsocial/`)

**Type `service`** — no official Sprout CLI exists; the stage-1 `cli`-type
rubric fails, so we implement HTTP against the REST API in
`internal/tools/sproutsocial/`, registered `RegisterService("sprout-social",
&sproutsocial.Service{})` in `internal/tools/register.go`. Package name
`sproutsocial` (dashes dropped, per master-plan §3 Go-package rule). Copy the
`internal/tools/notion/` shape: cobra tree grouped by resource, a
`BaseURL`/`HC`/`Out`/`Err` struct so unit tests point at `httptest`, `--json`
raw-envelope output, exit-code contract 0 success / 1 API-or-runtime failure
(typed `apiError` carrying Sprout's `error` body + `X-Sprout-Request-ID`) / 2
usage/parse error.

### Credential injection (definition `auth`)

Two credential bindings, both `type: env`:

- `access_token` → env `SPROUT_SOCIAL_TOKEN`. The service builds
  `Authorization: Bearer $SPROUT_SOCIAL_TOKEN` on every request.
- `customer_id` → env `SPROUT_SOCIAL_CUSTOMER_ID`. Injected default customer id
  (see §4 — sourced from the connection account key). The service uses it to
  fill the `{cid}` path segment.

`customer_id` being pre-injected means the AI's common path (`heliox tool
sprout-social -- analytics posts …`) needs **no discovery round-trip** — the id
is already in the environment. A global `--customer-id` flag **overrides** the
injected default for tokens that can see multiple customers, and `metadata
client` lists them.

### Command tree (resource-grouped verbs)

```
sprout-social
  metadata client                 GET  /v1/metadata/client            (no cid; discovery)
  metadata profiles               GET  /v1/{cid}/metadata/customer
  metadata tags|groups|users|topics|teams|queues
                                  GET  /v1/{cid}/metadata/customer/<r>
  analytics profiles  [--filter…] [--metric…] [--fields] [--page] [--body json]
                                  POST /v1/{cid}/analytics/profiles
  analytics posts     [same flags]
                                  POST /v1/{cid}/analytics/posts
  messages list       [same flags]
                                  POST /v1/{cid}/messages
  cases filter        [same flags]
                                  POST /v1/{cid}/cases/filter
  publishing create   --profile-id --text [--scheduled-at] [--body json]
                                  POST /v1/{cid}/publishing/posts   (draft only, per API)
  publishing get <publishing_post_id>
                                  GET  /v1/{cid}/publishing/posts/{id}
```

Global flags: `--customer-id` (override), `--json` (raw envelope), `--page`.

### JSON output shape

Default: a compact human summary per resource (id + name/network + key metric).
`--json`: the Sprout envelope passed through unmodified (`{data, paging}`), so
the AI can page (`paging.next_cursor`) and read every field. Errors in `--json`
mode render the typed error envelope `{ "error": { "message", "request_id",
"status" } }` on stderr, exit 1.

## 3. Credential fields & auth flow (verified against official docs)

- **One secret:** the Sprout **API Access Token** — a long-lived,
  **non-expiring** account-scoped bearer token. No refresh cycle, no client
  id/secret, no OAuth redirect. Created by the user at **Settings → Reporting →
  API Tokens** after accepting the Analytics API Terms of Service (and the X
  Content EULA if X data is needed), on an API-provisioned Advanced/Premium
  account.
- **Header:** `Authorization: Bearer <token>`.
- **Token semantics:** single opaque string; the user pastes it into Helio's
  connect drawer; it is stored in Vault via the write-only
  `POST /connections/credentials` API and never enters the bundle.
- **Identity / account resolution:** the token alone is opaque — the account
  identity (customer id + name) is obtained by calling `GET /v1/metadata/client`
  at connect time. Response shape:
  `{"data":[{"customer_id":687751,"name":"My Business"}, …]}`. We derive
  `account_key = customer_id` (as a string) and `label = name` from `data[0]`.
  A token can list multiple customers; connect captures the first as the default
  account (the AI reaches the others via `--customer-id` / `metadata client`).

## 4. Helio provider bundle plan (`integrations/providers/sprout_social/provider.yaml`)

Hidden-first (`presentation.visible: false`). Manual-token (`api_key`) bundle —
no OAuth block, no client secret, `required_config_fields: []` (so the provider
renders `configured: true` with zero integration-service config; no
`config/`+`deploy/` secret append, unlike the oauth lanes).

```yaml
schema: helio.provider/v1
key: sprout_social
go_name: SproutSocial
presentation:
  name: Sprout Social
  description_key: sprout_social
  consent_domain: sproutsocial.com
  visible: false
auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    setup_url: https://app.sproutsocial.com/settings/global-features/api
identity:
  source: userinfo
  url: https://api.sproutsocial.com/v1/metadata/client
  stable_key: /data/0/customer_id      # numeric → string (see capability note)
  label_candidates: [/data/0/name]
connection:
  mode: isolated
  disconnect_mode: local_only          # Sprout has no token-revoke endpoint
  runtime_strategy: manual_credentials
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    access_token: token.access_token
    customer_id: connection.account_key   # projects the connect-time customer id
tool:
  name: sprout-social
  kind: api-key
```

### Axis ②↔③ divergence → resolver entry

`sprout-social` (id) vs `sprout_social` (key) is a mechanical dash↔underscore
pair. Add to `helio-cli/internal/toolcred/resolver.go` `toolToProvider`:
`"sprout-social": "sprout_social"` (+ resolver test), **unless** master-plan
open-question 1 (mechanical id→key normalization in `ProviderFor`) has landed
before this batch — in which case no entry is needed and adding one is dead
code. Confirm the OQ1 decision at implementation time.

### Two integration-service capability dependencies (connect-time verify only)

The connect-time verifier must (a) send `Authorization: **Bearer** <token>` and
(b) read a **numeric** `customer_id` as the stable key. On `main` today the
declarative `manualTokenVerifier` sets the header value to the **raw** token
(no scheme) and `jsonPointerString` returns non-string JSON as "no string value"
— so neither is satisfied out of the box.

**Recommended: a small compiled `sproutClientVerifier`** (precedent:
`courierBrandsVerifier`, moz/semrush/fullstory/lemlist verifiers) registered for
`sprout_social` in `provider_registry.go`. It GETs `/v1/metadata/client` with
`Authorization: Bearer <token>`, and from `data[0]` formats `customer_id`
(int → canonical string) as `accountKey` and `name` as `label`. This handles the
Bearer scheme **and** the numeric coercion in one reviewed, self-contained unit
with no dependency on other batches' in-flight declarative growths.

**Alternative (only if both already merged):** reuse the declarative verifier if
(i) the `api_key.scheme: bearer` reviewed enum (tally/loops "Bearer-scheme
verifier" precedent) and (ii) numeric stable-key coercion in `jsonPointerString`
(hubspot precedent) are both on `main` at implementation time. Verify before
choosing; do not assume sibling-branch capabilities have landed.

Note the anycli **data-plane** call path needs neither capability — the anycli
service owns its own `Authorization: Bearer` header and treats `customer_id` as
an opaque injected string. The capabilities are purely about the one
integration-service connect-time identity/verify request.

### UI + docs (batch-end shared surfaces)

- Icon: `ui/helio-app/src/integrations/icons/sprout_social.svg` + register in
  `providerIcons.ts` (manual, never generated).
- i18n: `tools.desc.sprout_social` across all locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` (customer-id
  model, filter-body passthrough, draft-only publishing), one plugin version
  bump per batch.

## 5. Test plan — five layers

| Layer | Coverage for this tool | Needs external creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes for each verb: assert path (incl. `/v1/{cid}/…` and the cid-less `metadata/client`), `Authorization: Bearer` injection, `--customer-id` override, POST body assembly from `--filter/--metric/--body`, `--json` raw envelope vs summary, exit codes 0/1/2, typed `apiError` from an `error` body. TDD-first. | No |
| **L2** dev harness vs real API | `ANYCLI_CRED_ACCESS_TOKEN=… ANYCLI_CRED_CUSTOMER_ID=… anycli sprout-social -- metadata client` then `-- metadata profiles` / `-- analytics posts --body '…'`. Proves field names + Bearer injection + path shape against live Sprout. | **Yes** — API-provisioned Advanced/Premium account (3-hold pre-verify gate) |
| **L3** `provider-gen --check` + both repos' unit suites | bundle strict-decode, directory-key equality, HTTPS identity/setup URLs, resolver `toolToProvider` test, compiled-verifier unit test (int-cid → string account_key, name → label, non-2xx → 4xx). | No |
| **L4** singleton + seeded credential | `POST /internal/test-only/connections/seed` with `provider:"sprout_social"`, `account_key:"687751"` (the customer id), `access_token:<real>` (non-expiring → seed access_token only, omit refresh/expiry). Then `heliox tool sprout-social -- analytics posts …` reaches live Sprout via the token gateway. | **Yes** — same real token as L2 |
| **L5** full connect flow (api_key key-entry path) | Once, hidden, before flip: open connect link → paste API token → `sproutClientVerifier` hits `/v1/metadata/client`, connection shows connected/configured (`GET /connections`) with the customer name as label → one **unseeded** live `heliox tool sprout-social -- …` succeeds. Agent-drivable (agent-browser) with human fallback per master-plan §2 lane 3. | **Yes** — real token + connect UI |

L2/L4/L5 all block on the **same single credential**: one API-provisioned Sprout
account's access token. Because that account is paid-plan + rep-authorized, its
procurement is the tool's critical path (the 3-hold account-pool pre-verify),
not any code capability. L1/L3 are fully offline and can be completed
immediately.

## 6. Rollout

Land hidden (bundle `visible: false`) in the 3-hold batch; anycli tool ships in
the batch tag + pin bump; L1–L4 green while hidden; L5 key-entry sweep after the
batch-end merge; then `visible: true` + `provider-gen` regenerate as the single
go-live change. No review clock (api_key), so the only gate to the flip is L5,
which is gated on the procured API account.
