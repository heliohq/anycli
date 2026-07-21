# Tool design: Segment

Scratch design for the `segment` external tool provider behind `heliox tool`.
Batch-lead strips this file at batch end. Follows
`.claude/skills/helio-tool-provider/SKILL.md` and master plan
`docs/design/008-300-integrations-rollout-plan.md` (row 120, Wave 2, Analytics).

## 0. Summary & the three naming axes

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `segment` | bundle `tool.command` (defaults to `tool.name`) |
| ② anycli tool id | `segment` | `definitions/tools/segment.json` |
| ③ provider catalog key | `segment` | `integrations/providers/segment/` |

All three are identical, so **no `toolToProvider` divergence entry** is needed
(verified: `helio-cli/internal/toolcred/resolver.go` has no `segment` entry and
needs none — `ProviderFor("segment")` already returns `"segment"`). Go package
name is `segment` (id has no dashes/leading digit).

Auth lane: **`api_key`** — confirmed, no divergence from the catalog or the
2026-07-21 OAuth audit (see §7).

## 1. Which official API surface, and why

Segment (Twilio Segment) exposes two distinct HTTP APIs with two different
credentials:

- **Tracking API** (`https://api.segment.io/v1`, `track`/`identify`/`page`/…):
  ingests customer events. Auth = a per-**source** *write key* over HTTP Basic
  (`base64(writeKey:)`). This is a data-plane firehose, one credential per
  source.
- **Public API** (US: `https://api.segmentapis.com`): the modern unified
  **management / observability** plane — CRUD over Sources, Destinations,
  Warehouses, Tracking Plans, Functions, Spaces, IAM, plus usage/delivery
  metrics. Auth = one workspace-scoped **Bearer token**.
  (Refs: <https://docs.segmentapis.com/>, Public API overview
  <https://segment.com/docs/api/public-api/>.)

**Data residency — US-scoped in v1 (documented limitation).** Verified against
the official docs (<https://docs.segmentapis.com/tag/Getting-Started/>): the
Public API is served from **region-specific hosts by workspace data residency**
— **US** workspaces use `api.segmentapis.com`, **EU** workspaces use
`eu1.api.segmentapis.com`. The SAME workspace-scoped token only works against
its own region's root; a token from an EU workspace 401s/404s against the US
root at BOTH connect-time identity verification (§3/§4) and every runtime call.
v1 **scopes to US-resident workspaces** and hardcodes the US host. This strands
the (GDPR-driven) EU cohort with a bare 401/404 and no in-band cause, so it is
called out as an **owed follow-up capability**, not a silent gap:
**region-aware connections** — a per-connection base URL selected at connect
time (a `region: us|eu` connect-form field → dual identity URL + a
region-stamped `SEGMENT_BASE_URL` injected alongside the token). Named here so
the batch tracks it; not smuggled in as "no capability growth owed." Until it
lands, the L2/L4/L5 test workspace **must be US-resident** (§6).

**This tool wraps the Public API.** Rationale driven by what an AI teammate
actually does with a CDP:

1. **Inventory / wiring** — "what sources feed which destinations?" (list
   sources, list a source's connected destinations, list warehouses).
2. **Observability** — "is data flowing, and what's failing?" (a source's
   events volume; delivery metrics / delivery overview). This is the single
   highest-value agent use case for a CDP.
3. **Governance** — "what's in the tracking plan?" (tracking plans + rules).
4. **Admin visibility** — list IAM users/groups, functions, Unify spaces &
   audiences.

The Tracking API is deliberately **out of scope**: emitting production
analytics events is not a teammate task, and its per-source write-key model is a
different credential class that does not fit the one-token `api_key` lane. Mixing
both planes into one tool would force two credential kinds — a smell. One tool,
one credential (the Public API workspace token).

The Public API is **Team/Business-tier only** — recorded here because it gates
the L2/L5 test account (§6), not because it changes the design.

### Endpoints wrapped (all under the region host, US `https://api.segmentapis.com` in v1)

REST, cursor-paginated. List responses are `{"data": {...}, "pagination":
{"current","next","totalEntries"}}`; the query params are `pagination[count]`
(1–200) and `pagination[cursor]`. The tool passes provider JSON through
verbatim (see §2). Concretely wrapped:

- `GET /` — **Get Workspace** (also the identity/verify endpoint, §3).
  Confirmed against the official Go SDK: `getWorkspace` → `localVarPath =
  localBasePath + "/"`
  (`segmentio/public-api-sdk-go/api/api_workspaces.go`).
- `GET /sources`, `GET /sources/{id}`, `GET /sources/{id}/connected-destinations`
- `GET /destinations`, `GET /destinations/{id}`
- `GET /warehouses`, `GET /warehouses/{id}`
- `GET /tracking-plans`, `GET /tracking-plans/{id}`, `GET /tracking-plans/{id}/rules`
- `GET /functions`
- `GET /spaces`, `GET /spaces/{spaceId}/audiences`
- `GET /iam/users`, `GET /iam/groups`
- Delivery/usage (**PROVISIONAL — L2-gated**): `GET /sources/{id}/events-volume`
  and the delivery-metrics surface (`.../metrics` / delivery-overview). These
  two paths are **not confirmed Public API paths** — the exact path names and
  query shape are pinned at L2 against the live API. An implementer MUST NOT
  hardcode `events-volume`/`metrics` before L2 confirms them; if a path does not
  exist, drop the dedicated subcommand and leave the surface reachable via the
  raw `request` verb.
- **Raw escape hatch**: `segment request` (see §2). The Public API has 100+
  endpoints; hand-wiring all is neither in the 2–3h budget nor necessary. A
  generic passthrough (bearer-injected, JSON-through) keeps the whole surface
  reachable, mirroring `notion`'s top-level `fetch`.

Scope is **read-first**. Writes (create source, update tracking plan, …) are
reachable only through the raw `request` verb with an explicit non-GET
`--method`; no dedicated mutation subcommands ship in v1 (least-surprise for an
autonomous teammate against a production CDP).

## 2. anycli definition

**Type: `service`** (stage-1 rubric). No official Segment CLI exists that is
non-interactive, `--json`-capable, and image-provisionable; the `cli` type does
not apply. Implement against the Public API HTTP surface in
`internal/tools/segment/`, following `internal/tools/notion/` for shape
(cobra tree grouped by resource; `BaseURL`/`HC`/`Out`/`Err` struct so unit tests
point at an `httptest.Server`; documented exit codes 0 success / 1 runtime-API
failure via typed `apiError` / 2 usage-parse; `--json` structured error
envelope).

`definitions/tools/segment.json`:

```json
{
  "name": "segment",
  "type": "service",
  "description": "Twilio Segment Public API — manage and observe workspace sources, destinations, warehouses, tracking plans, and delivery",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SEGMENT_TOKEN"}
      }
    ]
  }
}
```

The service reads `SEGMENT_TOKEN` and sends `Authorization: Bearer
<SEGMENT_TOKEN>` on every request (the Bearer scheme is built by the anycli
service itself — independent of the integration-service verifier scheme in §3).
Base URL default `https://api.segmentapis.com` (US), overridable in tests. When
the region follow-up (§1) lands, this default is overridden per-connection by an
injected `SEGMENT_BASE_URL` (US `api.segmentapis.com` / EU
`eu1.api.segmentapis.com`); v1 ships the US default only.

### Subcommand tree (verbs)

```
segment workspace get
segment source list [--count N] [--cursor C]
segment source get --id <id>
segment source connected-destinations --id <id>
segment destination list [--count N] [--cursor C]
segment destination get --id <id>
segment warehouse list [--count N] [--cursor C]
segment warehouse get --id <id>
segment tracking-plan list [--count N] [--cursor C]
segment tracking-plan get --id <id>
segment tracking-plan rules --id <id>
segment function list [--count N] [--cursor C]
segment space list [--count N] [--cursor C]
segment space audiences --space-id <id> [--count N] [--cursor C]
segment iam user list [--count N] [--cursor C]
segment iam group list [--count N] [--cursor C]
# PROVISIONAL / L2-gated — path names unconfirmed; do not hardcode until L2 pins them:
segment delivery events-volume --source-id <id> [--start ... --end ...]
segment delivery metrics --source-id <id> --destination-config-id <id> [...]
segment request --method GET --path /sources [--query 'pagination[count]=100'] [--body @file]
```

`--count`/`--cursor` map to `pagination[count]`/`pagination[cursor]`.

### JSON output shape

Agent-facing, stable, provider-passthrough. Success prints the provider's
`data` object plus a normalized `pagination` block so an agent can page without
parsing Segment's envelope:

```json
{ "data": { "sources": [ ... ] },
  "pagination": { "next": "<cursor|null>", "totalEntries": 42 } }
```

Single-object GETs print `{ "data": { "source": { ... } } }` unwrapped one
level (the provider already nests by resource). Errors (`--json`) use the notion
envelope: `{ "error": { "code": "...", "message": "...", "status": <http> } }`,
exit 1; usage/parse errors exit 2.

## 3. Credential fields & the exact auth flow (api_key lane, verified)

**Registration model.** A **Public API token** is created by a *Workspace Owner*
in the Segment app: Settings → Workspace settings → Access Management → Tokens →
Create Token → **Public API token** (Owner or Member access). Tokens are
**scoped to exactly one workspace** and are long-lived (no expiry; revoked in
the same UI; auto-revoked if leaked to a public repo via GitHub secret
scanning). There is **no third-party authorization-code OAuth** a shared Helio
client could run to act on arbitrary customer workspaces — the user pastes their
own token. This is the textbook `api_key` lane.
(Ref: <https://docs.segmentapis.com/tag/Getting-Started/>,
<https://docs.segmentapis.com/tag/Authentication/>.)

**Wire auth.** `Authorization: Bearer <token>` over HTTPS (port 443 only).

**Connect-time verification (integration-service).** Declarative
`manual_api_token` path, **zero new service code**, reusing the api_key bundle
verifier (`service/manual_token_verifier.go`,
`declarativeManualTokenVerifier`): it GETs `identity.url` with the
bundle-declared header and derives the account key + label from the response via
JSON Pointer.

- **Header value needs the `Bearer` scheme prefix.** Verified in the current
  repo: the base verifier does `req.Header.Set(definition.APIKey.Header, token)`
  — the *raw* token (`service/manual_token_verifier.go:34`) — and `APIKeyPolicy`
  today has only `Header` + `SetupURL`, no scheme field
  (`model/catalog.go:167`). Segment's root endpoint 401s without `Bearer `. So
  Segment **depends on** an `auth.api_key.scheme` capability (a `scheme:
  bearer`-style field that makes the verifier send `"Bearer "+token`) that the
  Instantly api_key batch is expected to introduce. That capability is **not yet
  merged in this repo**, and its exact field name / shape
  (`APIKeyPolicy.Scheme`? `HeaderValue()`? enum values?) is **assumed, not
  settled** — **verify the actually-merged API at integration time** and adjust
  this bundle if Instantly names or implements it differently (dependency note
  in §5).
- **Identity endpoint.** `GET https://api.segmentapis.com/` returns
  `{"data":{"workspace":{"id","name","slug"}}}`. Stable account key =
  `/data/workspace/id` (immutable workspace id); label candidates =
  workspace name then slug then id. Two tokens for the same workspace upsert the
  same connection (identity is workspace-level, not token-level).

**Storage & runtime.** The pasted token is a single secret stored through the
existing `UpsertUserToken` write path (`credential.fields.access_token:
token.access_token`) — no new `CredentialSource`, no token-gateway change. At
runtime the token gateway serves it and heliox injects it via anycli's
credential map into `SEGMENT_TOKEN`. Non-expiring ⇒ **no refresh cycle**; L4
seeds `access_token` only.

## 4. Helio provider bundle plan (hidden-first)

`integrations/providers/segment/provider.yaml`:

```yaml
schema: helio.provider/v1
key: segment
go_name: Segment

presentation:
  name: Segment
  description_key: segment
  consent_domain: segment.com
  # Hidden-first (master plan §Hidden-first rollout). Flip to visible only after
  # ALL of: the anycli segment tool ships in the pinned AnyCLI + heliox rebuild
  # / runtime image, a reviewed brand icon lands in helio-app, tools.desc.segment
  # ships in all locales, and the L5 api_key key-entry connect flow is verified
  # end to end. Pick an unoccupied presentation.order when flipping.
  visible: false

# api_key auth: the user pastes a Segment Public API token (workspace-scoped
# bearer, created by a Workspace Owner in Access Management → Tokens). No
# third-party OAuth exists for arbitrary customer workspaces, so this is an
# api_key / manual_api_token provider. Segment requires "Authorization: Bearer
# <token>", so scheme: bearer makes the connect-time verifier send the Bearer
# prefix (the raw default would 401). NOTE: the api_key.scheme field depends on
# the (not-yet-merged) Instantly capability — confirm its real name/shape before
# authoring this bundle (§3/§5). The token is verified against GET / (Get
# Workspace) before any Vault write.
auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    scheme: bearer
    setup_url: https://docs.segmentapis.com/tag/Getting-Started/

# Get Workspace resolves the workspace from the token itself and returns a
# stable id + human-readable name/slug. url is the US root; v1 is US-scoped
# (§1 data residency). When the region follow-up lands, this becomes a
# region-selected pair (US https://api.segmentapis.com/ | EU
# https://eu1.api.segmentapis.com/) — an EU token verified against the US root
# 401s/404s, so an EU cohort is unsupported until then.
identity:
  source: userinfo
  url: https://api.segmentapis.com/
  stable_key: /data/workspace/id
  label_candidates: [/data/workspace/name, /data/workspace/slug, /data/workspace/id]

connection:
  mode: isolated
  # Segment exposes no self-revoke-my-own-token API worth calling; users revoke
  # tokens in Access Management. Disconnect just deletes the stored credential.
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: segment
  kind: api-key   # wire-compat value; clients route the connect drawer by auth_type
```

Axis alignment: `tool.command` omitted ⇒ defaults to `tool.name: segment`
(flat command, not a grouped family). Directory name = `key` = `segment` (the
generator enforces equality).

Companion artifacts (batch-end merge, per master plan §2):
- **Generation**: one `provider-gen` + `--check` run updates the five
  projections; committed together with the bundle at batch end.
- **UI icon**: `ui/helio-app/src/integrations/icons/segment.svg` + hand-register
  in `providerIcons.ts` (never generated).
- **i18n**: `tools.desc.segment` (all 9 locales) + the api_key connect-drawer
  strings; gates the visible flip.
- **AI-facing docs**: `segment` sub-doc under
  `agents/plugins/heliox/skills/tool/`; plugin version bump + marketplace
  publish (one per batch).
- **No `experiment` gate** (GA on flip; leave field empty).

## 5. Integration-service capability dependency (v1: none owed; region deferred)

This tool writes **zero** integration-service Go code. Its only non-default need
— rendering `Authorization: Bearer <token>` in the connect-time verifier — is
served by an **`auth.api_key.scheme` bearer capability** that the Instantly
api_key batch is expected to introduce. As verified in the current repo, that
capability is **not yet merged**: `APIKeyPolicy` has only `Header` + `SetupURL`
(`model/catalog.go:167`) and `declarativeManualTokenVerifier` sends the raw
token (`service/manual_token_verifier.go:34`). Segment therefore **depends on**
that cross-batch capability (a one-field, orthogonal growth, not a per-provider
adapter) — flagged at stage 1 rather than mid-wave. **Verify the actually-merged
field name and shape at integration time**; if the Instantly batch names or
implements the scheme field differently than assumed (`APIKeyPolicy.Scheme` /
`HeaderValue` / enum `raw|bearer`), this bundle must change to match. No compiled
`service/adapter_*.go` and no new `runtime_strategy` are justified: the response
shape (JSON, bearer header, single userinfo GET) sits fully inside the closed
`manual_api_token` capability set.

## 6. Test plan — five layers

| Layer | What it proves for Segment | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/segment/` cobra tree + `definitions/tools/segment.json` against `httptest` fakes: request path/method, `Authorization: Bearer` header injection, `pagination[...]` query encoding, `data`/`pagination` passthrough, `--json` error envelope, exit codes 0/1/2. Never hits the real API. | No |
| **L2** `anycli segment -- <args>` harness | `ANYCLI_CRED_ACCESS_TOKEN=<real token> anycli segment -- workspace get` and `-- source list` return real workspace/sources from a live Team/Business workspace. Pins the exact identity path (`/`), delivery-metrics path/query, and pagination shape against the live API. | **Yes** — one Team/Business-tier Public API token (account pool) |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle strict-decodes; five projections regenerate clean; `helio-cli` builds (local `replace` → anycli branch) and `go test ./cmd/heliox/cmds/tool/` passes; the visible-tool CLI test skips the still-hidden `segment`. | No |
| **L4** singleton + seed + `heliox tool segment -- …` | `POST /internal/test-only/connections/seed` with `provider":"segment","access_token":"<real token>"` (seedable — api_key user-token provider; no refresh, so `access_token` only). Then `heliox tool segment -- workspace get` reaches the live API through the real token gateway + anycli. Uses a real seeded org/assistant/owner identity. | **Yes** — same L2 token |
| **L5** full connect flow (once, pre-flip) | api_key **key-entry** path (master plan §2, not the OAuth path): open connect link → paste token in the real connect UI → stored via `POST /connections/credentials`, verified against `GET /` → connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool segment -- workspace get` through the real token gateway succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. | **Yes** — same token + connect UI |

Externally-supplied credentials are needed for **L2, L4, L5** (one Public API
token from a Team/Business-tier test workspace). L1/L3 need nothing.

**Test workspace must be US-resident.** Because v1 hardcodes the US host (§1),
the L2/L4/L5 workspace has to be a **US** data-residency workspace — a token
from an EU workspace verifies against `eu1.api.segmentapis.com`, not the US root
this build targets, so it **fails identity verification at connect (§3/§4) and
every runtime call with a bare 401/404 and no in-band cause**. A tester handed
an EU token would be left debugging that opaque failure; the region follow-up
(§1) is what unblocks the EU cohort. Record this in the L2/L5 runbook so the
account-pool token is provisioned from a US workspace.

## 7. Audit reconciliation (no divergence)

The 2026-07-21 OAuth audit lists Segment (row 122) as **`api_key` — "no viable
multi-tenant path"**. Verified against official docs: Public API tokens are
workspace-scoped bearer tokens minted by a Workspace Owner; Segment's only OAuth
("Enable with Segment", <https://segment.com/docs/partners/enable-with-segment/>)
is a **partner** program for destination/source builders enabling *their*
integration inside a customer's workspace — not a general authorization-code
flow a shared Helio client can run to act on arbitrary customer workspaces. The
audit verdict stands; **no catalog amendment**. Lane, id, key, and wave are
unchanged from the master plan.
