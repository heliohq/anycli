# Tool provider design — Loops

- **anycli id (axis ②):** `loops`
- **provider catalog key (axis ③):** `loops`
- **CLI command word (axis ①):** `loops` (flat command, no group)
- **auth lane:** `api_key` (catalog row 272; OAuth audit row 274 — "no viable multi-tenant path", stays `api_key`)
- **wave / category:** Wave 3 · Marketing & Notifications
- **go package (stage 2):** `internal/tools/loops`
- **RegisterService string:** `loops`

All three naming axes are identical (`loops`/`loops`/`loops`), so **no
`toolToProvider` divergence entry** is added in `resolver.go`, no `tool.command`
override, and no `tool.group`. This is the simplest naming shape in the program.

---

## 1. Verification of the catalog against official docs

The catalog and the OAuth audit both place Loops in the `api_key` lane. I
independently checked the official Loops API surface and confirm this is
correct — with **no divergence to record**:

- **Auth model.** The official OpenAPI spec (`https://app.loops.so/openapi.json`,
  version 1.20.0) declares a single security scheme:
  `securitySchemes.apiKey = { "type": "http", "scheme": "bearer" }`. Every call
  authenticates with `Authorization: Bearer <API_KEY>`. There is **no OAuth
  authorization-code flow, no `/authorize` or `/token` endpoint, no client
  registration** — an account's key is minted in-app (Settings → API →
  *Generate key*) and revoked from the same screen. This is a per-account bearer
  API key with no multi-tenant authorization path, matching the audit rubric for
  `api_key` exactly.
- **Registration / token semantics.** Keys are long-lived, team-scoped bearer
  tokens (no expiry, no refresh cycle). A team may hold several named keys; each
  is scoped to its team with full-team access (no per-key scope selection on the
  wire). Rate limit: 10 req/s per team (`x-ratelimit-*` headers, `429` on
  breach).

**Divergence on the auth lane: none.** Official docs agree with both the catalog
auth lane (`api_key`) and the audit verdict. (One unrelated **request-shape**
spec/doc divergence — the `ContactDeleteRequest` `required` array — is recorded
in §2; it does not affect the auth lane.)

---

## 2. Which API surface this tool wraps, and why

**Base URL:** `https://app.loops.so/api` (endpoint paths carry the `/v1/…`
prefix; e.g. `POST https://app.loops.so/api/v1/contacts/create`).

Loops is a transactional-email + audience/marketing platform. Its full v1 API is
large (contacts, events, transactional email, mailing lists, plus a builder
surface: campaigns, workflows, themes, components, audience segments, email
messages, uploads, event patterns, campaign/transactional groups, dedicated
sending IPs). An AI teammate's real jobs with Loops are **CRM + messaging**:
add/update/look-up a contact, trigger a workflow via an event, and send a
templated transactional email. The heavy builder surface is authoring-tool
territory that a human configures in the Loops UI — not what a teammate drives
from chat.

So the tool wraps the **CRM + messaging core**, mirroring the shaped subset the
sibling `sendgrid` / `customer-io` / `brevo` tools expose. Endpoints, all
verified present in the OpenAPI spec:

| Command | Method + path | Purpose |
|---|---|---|
| `loops whoami` | `GET /v1/api-key` | Verify key; returns `{success, teamName}` (also the identity endpoint, §4) |
| `loops contact create` | `POST /v1/contacts/create` | Create a contact (`email` required) |
| `loops contact update` | `PUT /v1/contacts/update` | Update/upsert (`email` **or** `userId` required) |
| `loops contact find` | `GET /v1/contacts/find?email=…|userId=…` | Find a contact → array |
| `loops contact delete` | `POST /v1/contacts/delete` | Delete by `email` or `userId` |
| `loops contact suppression get` | `GET /v1/contacts/suppression?email=…|userId=…` | Suppression status |
| `loops contact suppression remove` | `DELETE /v1/contacts/suppression?email=…|userId=…` | Un-suppress |
| `loops contact-property list` | `GET /v1/contacts/properties` | List custom contact properties |
| `loops contact-property create` | `POST /v1/contacts/properties` | Create a custom property |
| `loops event send` | `POST /v1/events/send` | Fire an event → triggers workflows |
| `loops email send` | `POST /v1/transactional` | Send a templated transactional email |
| `loops email list` | `GET /v1/transactional` | List transactional templates (+ their data variables) |
| `loops list ls` | `GET /v1/lists` | List mailing lists (ids for subscription control) |

**Deliberately out of scope for v1** (documented, not wrapped): campaigns,
workflows, themes, components, audience-segments, email-messages, uploads,
event-patterns, campaign/transactional-groups, dedicated-sending-ips. These are
low-frequency authoring operations. If a concrete teammate need appears, the
follow-up is a **single generic passthrough** subcommand
(`loops api <METHOD> <path> [--query k=v] [--body <json>]`) rather than typing
out the whole builder tree — but v1 ships without it to keep the surface tight
and every command intention-revealing.

### Request-shape notes (from the spec, load-bearing for the impl)

- **`contact create` / `contact update`** — the `ContactFields` schema defines
  these **first-class** (non-additional) properties: `email`, `firstName`,
  `lastName`, `source`, `subscribed`, `userGroup`, `userId`, `mailingLists`.
  `create` requires `email` (`ContactRequest = ContactFields + required:[email]`);
  `update` requires **exactly one** of `email`/`userId` (`anyOf`). The CLI
  exposes each first-class field as a named convenience flag — `--email`,
  `--first-name`, `--last-name`, `--source`, `--subscribed`, `--user-group`,
  `--user-id`, `--mailing-list id=bool` (repeatable) — so the most common
  contact attributes (firstName/lastName especially) are intention-revealing,
  not buried in a generic escape hatch. On top of that,
  `ContactFields.additionalProperties` (`string|number|boolean`) permits
  arbitrary custom properties, exposed via a repeatable `--property key=value`
  (typed-coerced) plus an escape hatch `--json '<raw object>'` merged into the
  body — so the AI can also set any custom field without a definition change.
  (Custom properties must already exist in Loops before use, per the spec.)
- **`contact delete`** accepts **exactly one** of `email` **or** `userId` — the
  same "exactly-one" shape as `find`/`suppression`. **Spec/doc divergence
  (recorded):** the OpenAPI `ContactDeleteRequest` schema lists
  `required: [email, userId]` (both), but the official endpoint docs say
  *"Include only one of `email` or `userId`,"* and the live API returns **`400`**
  ("email and userId are both provided") when both are sent. The OpenAPI
  `required` array is a known spec bug; the docs + live behavior are
  authoritative here. The CLI therefore **requires exactly one identifier and
  rejects the both-provided case client-side** (usage error, exit 2) — it does
  **not** forward both. (Building the wrong validation here would 400 on every
  invocation and an L1 fake would lock in the wrong contract — exactly the
  provider quirk L2 exists to catch.)
- **`event send`** body: `eventName` required, plus `email` **or** `userId`,
  optional `eventProperties` (object), optional `mailingLists`. Honors an
  optional `--idempotency-key` (spec `Idempotency-Key` header; `409` on replay).
- **`email send`** body: `email` + `transactionalId` required; optional
  `dataVariables` (object of template vars), `addToAudience` (bool),
  `attachments` (support-gated). Also honors `--idempotency-key`.

---

## 3. anycli definition

**Type: `service`** (per stage-1 rubric). No official Loops CLI exists; the tool
is a thin non-interactive cobra tree over the Loops REST API — identical shape to
`bitly` / `notion` / `slack`. 21 of 23 shipped definitions are `service` type;
this is one more.

### `definitions/tools/loops.json`

```json
{
  "name": "loops",
  "type": "service",
  "description": "Loops as a tool (transactional email, contacts, events; API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "LOOPS_API_KEY"}
      }
    ]
  }
}
```

The credential field name is `api_key` (matches the Helio bundle's
`credential.fields` key, §5). anycli injects it as `LOOPS_API_KEY`; the service
builds `Authorization: Bearer <LOOPS_API_KEY>` itself on every request.

### `internal/tools/loops/` (service impl)

Copy the `bitly` package shape (it is the in-tree `service`-type + Bearer-auth
precedent):

- `loops.go` — `Service{ BaseURL, HC, Out, Err }`, `Execute(ctx, args, env)`
  reads `LOOPS_API_KEY` (empty → stderr + exit 1), builds the cobra root, wires
  subcommands. `DefaultBaseURL = "https://app.loops.so/api"`.
- `client.go` — one `call(ctx, method, path, query, payload)` helper:
  `Authorization: Bearer <token>`, `Accept: application/json`, JSON body when
  present. Non-2xx → typed `apiError`; **401 → `execution.RejectCredential`** so
  the token gateway learns the key is dead. Loops error bodies carry
  `{message}` (and a deprecated `error`); extract `message` for the failure text.
- Resource files: `contact.go`, `contact_property.go`, `event.go`,
  `transactional.go` (`email …`), `list.go`, `whoami.go`.
- `harness_test.go` + per-file `_test.go` — `httptest.Server` fakes asserting
  method, path, query, `Authorization: Bearer …` header, request body, and both
  plain-text and `--json` error rendering. **Never hits the real API** (that is
  L2). TDD: tests first, per anycli AGENTS.md.

**JSON output shape.** Every command emits the provider's JSON response verbatim
to stdout (+ trailing newline), passthrough style — exactly like `bitly`. A
persistent `--json` flag is accepted for uniformity but output is always JSON
(the API only speaks JSON). **Exit codes:** `0` success · `1` runtime/API failure
(typed `apiError`; `401` also rejects the credential) · `2` usage/parse errors
(cobra).

### `internal/tools/register.go`

Add the import and `RegisterService("loops", &loops.Service{})`. This is a
**batch-serialized shared surface** (master plan §2) — merged by the batch lead
at batch end, not mid-batch. The per-tool `definitions/tools/loops.json` and the
`internal/tools/loops/` package are generation-inert and merge freely before then.

---

## 4. Credential fields and the exact auth flow

**Credential:** one field — the Loops team API key (`api_key`), a long-lived
bearer token. No secret pair, no refresh, no expiry.

**Connect-time flow (Helio side, `manual_api_token` strategy):**

1. User pastes the API key into the connect form (single secret; stored via the
   write-only `POST /connections/credentials` path into Vault — never in the
   bundle).
2. integration-service **verifies before storing** via the declarative manual
   token verifier (`service/manual_token_verifier.go`): `GET` the
   bundle-declared identity endpoint `https://app.loops.so/api/v1/api-key` with
   the bundle-declared header.
   - **Success (`200`)** → body `{ "success": true, "teamName": "<Team>" }`.
     Identity `stable_key` = `/teamName`; `label_candidates` = `[/teamName]`.
     The account key/label is the human-readable team name (Loops exposes no
     numeric team id on this endpoint — same "human-readable, never a hash"
     stance as the mongodb DSN-host precedent).
   - **`401`** → body `{ "success": false, "message": "Invalid API key" }`;
     verifier maps `401/403` to `invalid_provider_credential` and the connect
     fails cleanly before anything reaches Vault.
3. Runtime: the token gateway serves the stored key; heliox injects it into
   anycli's credential map (`api_key`), anycli sets `LOOPS_API_KEY`, and the
   service sends `Authorization: Bearer <key>` to the live API.

### Capability gap: Bearer scheme on the manual-token verifier (reuse-or-add)

Loops requires `Authorization: **Bearer** <key>`. On the branch's main base,
`declarativeManualTokenVerifier.Verify` does
`req.Header.Set(definition.APIKey.Header, token)` — it sends the **raw token**
with no scheme prefix, and `apiKeyManifest`/`model.APIKeyPolicy` carry only
`Header` + `SetupURL`, with **no** way to express a `Bearer ` prefix. As-is, the
verifier would send `Authorization: <key>` and Loops would reject it `401` at
connect time, even for a valid key.

This is a **generic** gap (the value-prefix scheme), not Loops-specific — every
Bearer-scheme `api_key` provider in this wave (`semrush`, `moz`, `fullstory`,
`tally`, `courier`, `novu`, `knock`, `iterable`, …) hits it. Per the skill's
`provider-yaml.md` guidance ("first check whether the gap is really
provider-specific or whether the generic capability set should grow one more
reviewed enum value"), the right fix is a **shared, reviewed capability growth**,
not a per-provider adapter:

- Add optional `scheme` to `apiKeyManifest` (yaml `auth.api_key.scheme`) and
  `model.APIKeyPolicy`, validated against a closed set (`{"", "bearer"}`; empty =
  today's raw-token behavior, back-compatible with mongodb-class bundles).
- In `declarativeManualTokenVerifier.Verify`, when `scheme == "bearer"`, set the
  header value to `"Bearer " + token`; otherwise unchanged.
- Unit-test both branches (raw and bearer) with an `httptest` identity server.

**Reuse-first:** this capability is a shared batch surface. If a sibling
Bearer-scheme `api_key` tool in the same batch has already landed `scheme` on
main, Loops **reuses** it and only sets `scheme: bearer` in its bundle. If not,
Loops' branch adds it (one reviewed enum value) and the batch lead lands it once.
Either way the runtime path is unaffected — the anycli service always builds its
own `Bearer` header, so this touches **connect-time verification only**.

---

## 5. Helio provider bundle plan (`integrations/providers/loops/provider.yaml`)

Hidden-first (`presentation.visible: false`). `manual_api_token` runtime strategy
(the api_key manual path), `standard`-shaped — no custom Go adapter.

```yaml
schema: helio.provider/v1
key: loops
go_name: Loops

presentation:
  name: Loops
  description_key: loops
  consent_domain: loops.so
  visible: false          # flip only after L5 (master plan §2 hidden-first)
  # order: <pick an unoccupied slot at flip time>

auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    scheme: bearer         # capability from §4 (reuse-or-add) — verify sends "Bearer <key>"
    setup_url: https://app.loops.so/settings/api

identity:
  source: userinfo
  url: https://app.loops.so/api/v1/api-key
  stable_key: /teamName
  label_candidates: [/teamName]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

# Single secret stored through the existing UpsertUserToken write path
# (token.access_token), exactly like mongodb — zero new CredentialSource.
credential:
  fields:
    api_key: token.access_token
    account_key: connection.account_key

tool:
  name: loops
  kind: api-key
```

**Axis mapping.** ① CLI word `loops` · ② anycli id `loops` · ③ key `loops` — all
identical → no `tool.command`, no `tool.group`, **no `toolToProvider` entry**.

**Generation.** From `go-services/integration-service`: `go run ./cmd/provider-gen`
then `--check`. Five projections regenerate together and are committed by the
**batch lead at batch end** — per the master plan §2 and the hard constraint,
this agent runs provider-gen **locally for validation only** and does **not**
commit the regenerated projections. The tool branch is expected to fail
`provider-gen --check` in CI until the batch-end merge.

**Config.** `manual_api_token` needs **no** `required_config_fields` and no
integration-service OAuth config (no client id/secret) — so there is **no
`config/` + `deploy/` Secret append** for Loops (the user's own key is the only
credential, entered at connect and stored in Vault). This sidesteps human lane 1
entirely.

**UI + docs (non-generated, per-tool):**
- Icon: `ui/helio-app/src/integrations/icons/loops.svg` + register in
  `providerIcons.ts` (manual append; batch-serialized shared surface).
- i18n: `tools.desc.loops` (and any `tools.scopes.*` if displayed) across locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` documenting the
  command tree, the `email send` template-id + `dataVariables` contract, and the
  `event send` → workflow-trigger model; plugin version bump + marketplace
  publish ride the batch-end merge.

---

## 6. Test plan — the five layers

| Layer | What it proves for Loops | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: `loops` service + definition unit tests against `httptest` fakes — asserts method/path/query, `Authorization: Bearer <key>` header, request bodies (first-class contact flags firstName/lastName/… as top-level body keys, custom props, event, transactional `dataVariables`), `contact delete` **rejects the both-identifiers case client-side (exit 2) and forwards exactly one**, 401→`RejectCredential`, `--json` error envelope, exit codes 0/1/2. | No (fakes) |
| **L2** | Dev harness against the **real** Loops API: `ANYCLI_CRED_API_KEY=<key> anycli loops -- whoami` (expect `{success, teamName}`), then `contact create/find/delete`, `event send`, `email list`, `list ls` against a **real Loops test team**. Proves field names + Bearer injection + request shapes match the live API. | **Yes** — a real Loops account API key (test-account pool, master plan lane 2) |
| **L3** | `provider-gen --check` (run locally on-branch; expected red in CI until batch end) + both repos' unit suites: `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`, and integration-service tests including the new/ reused Bearer-scheme verifier test. Point `helio-cli/go.mod` at the anycli branch via a **local, uncommitted** `replace`. | No |
| **L4** | Singleton + seed a real key through the token gateway: `POST /internal/test-only/connections/seed` with `provider: loops`, `access_token: <real key>`, a **real** seeded org/assistant identity → `201`; then `heliox tool loops -- whoami` and one write (`contact create`) reach the live API. Seeds `access_token` only (no refresh/expiry — non-expiring bot-class token). | **Yes** — real key (same as L2) |
| **L5** | One full connect flow, still hidden, via the **api_key key-entry path** (master plan §2, human lane 3, agent-drivable): open the connect link → paste the key through the real connect UI → integration-service verifies it against `GET /v1/api-key` (`teamName` becomes the account label) → connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool loops -- …` call succeeds through the real token gateway. | **Yes** — real key |

**Externally-supplied-credential layers: L2, L4, L5** — all need one real Loops
team API key from the test-account pool. L1 and L3 are hermetic. There is **no
OAuth app registration** (human lane 1 does not apply), so the only external
dependency is the account/key itself.

**Rollout (master plan §2 / skill stage 10):** land hidden with L1–L4 green +
the Bearer-scheme capability; run the L5 key-entry sweep at batch end; then flip
`presentation.visible: true` + regenerate as the single go-live change.

---

## 7. Open decisions

1. **Identity stability.** `teamName` is the only identity Loops' key-test
   endpoint returns; it is user-editable and could collide across teams a single
   assistant connects. Accepted for v1 (human-readable label per repo precedent);
   if collisions bite, the fallback is a synthetic `account_key` while keeping
   `teamName` as the label. No numeric team id is available on `GET /v1/api-key`.
2. **Builder surface.** Campaigns/workflows/themes/etc. are out of v1 scope.
   Revisit with a single generic `loops api <method> <path>` passthrough only if
   a concrete teammate need appears — do not pre-build the tree.
3. **`scheme` capability ownership.** The Bearer-scheme verifier growth (§4) is a
   shared batch surface; coordinate reuse-vs-add with the batch lead so exactly
   one branch lands the `apiKeyManifest.scheme` field.
