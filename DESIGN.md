# Paperform — per-tool integration design

Scratch design for the `tool/paperform` branch (both repos). Batch lead strips
this at batch-end. Catalog row 215: `paperform` / `paperform` / **api_key** /
Wave 3 / Forms & Surveys.

## 0. Ground-truth verification (done before writing)

- **Official docs**: `https://paperform.readme.io` (reference + `llms.txt`
  OpenAPI/Markdown index). Base URL `https://api.paperform.co/v1`. Auth is
  **Bearer**: `Authorization: Bearer <API_KEY>`. API keys are generated on the
  account page; access requires a **Standard or Business** Paperform plan (some
  write endpoints are Business-API-only). Rate limited → `429` with
  `X-RateLimit-*` / `Retry-After` headers. Enterprise secondary regions use
  `https://api.{region}.paperform.co/` (out of scope — single-region default).
- **Auth-lane check vs catalog**: catalog + oauth-audit row 217 both say
  **api_key, "no viable multi-tenant path"**. Confirmed against official docs:
  Paperform exposes **only** a per-account API key (account-page token); there
  is **no** OAuth2 authorization-code app, no client registration, no scopes.
  The audit verdict holds — **stays api_key**. No divergence to record.
- **Identity endpoint check (the one thing that bites)**: the `llms.txt` index
  lists forms, fields, submissions, partial-submissions, products, coupons,
  webhooks, spaces, translations, files, and Papersign — and **no**
  `/account`, `/me`, or `/whoami` endpoint. There is no per-account identifier
  anywhere in the response bodies. This drives the identity decision in §4.

## 1. Tool form decision (skill stage 1) — `service`

No official Paperform CLI exists. Default rubric applies → **`service`** type,
implemented in anycli `internal/tools/paperform/` against the v1 HTTP API. 21 of
23 shipped definitions are service type; this follows `notion` as the shape
reference (cobra tree grouped by resource + `BaseURL`/`HC`/`Out`/`Err` struct,
`--json` error envelope, exit codes 0/1/2).

## 2. API surface wrapped — and why

Driven by what an AI teammate actually does with Paperform: **triage and
summarize form responses**, inspect a form's structure, and do light commerce
housekeeping. Read-heavy. Endpoints wrapped (all under
`https://api.paperform.co/v1`, `Authorization: Bearer`):

| anycli subcommand | Method + path | Why (teammate use) |
|---|---|---|
| `form list` | `GET /forms` | Enumerate the account's forms |
| `form get --form <slug_or_id>` | `GET /forms/{id}` | Inspect one form |
| `field list --form <id>` | `GET /forms/{id}/fields` | Understand a form's questions before reading responses |
| `field get --form <id> --key <k>` | `GET /forms/{id}/fields/{key}` | Resolve a single field |
| `submission list --form <id> [--limit --skip --sort --after --before]` | `GET /forms/{id}/submissions` | The core action: read responses to summarize/triage |
| `submission get --id <id> [--form <id>]` | `GET /forms/{id}/submissions/{id}` (or `GET /submissions/{id}`) | Pull one response in full |
| `partial-submission list --form <id>` | `GET /forms/{id}/partial-submissions` | Follow-up on abandoned responses |
| `space list` / `space get --id` / `space forms --id` | `GET /spaces`, `/spaces/{id}`, `/spaces/{id}/forms` | Navigate the workspace tree |
| `product list --form <id>` | `GET /forms/{id}/products` | Read order/product config |
| `coupon list --form <id>` / `coupon get --form <id> --code <c>` | `GET /forms/{id}/coupons`, `/coupons/{code}` | Read discount config |

**Deliberately scoped out of v1** (kept minimal, subtract-before-add):
destructive deletes (`DELETE /submissions/{id}`, partial deletes), Business-API
writes (form/field update, product create/update, webhook CRUD), coupon
create/update, Papersign (`/papersign/*` is a distinct product surface + plan).
These are additive later behind the same bundle; v1 is a safe read surface plus
`coupon`/`product`/`space` reads. `POST /forms/{id}/submit` (programmatic
submission) is a **verify-at-L2** candidate — the `llms.txt` index did not list
it explicitly; include only if L2 confirms it against the live API. Pagination
(`limit`/`skip`/`after`/`before`/`sort`) is confirmed at L2 and passed through as
flags, not synthesized.

## 3. anycli definition

- `definitions/tools/paperform.json`, `name: "paperform"`, `type: "service"`,
  one-line `description`.
- `auth`: single `CredentialBinding` — source `field: access_token`, inject
  `type: env`, `env_var: PAPERFORM_API_KEY`. The service reads
  `PAPERFORM_API_KEY` and sets `Authorization: Bearer <token>` (Paperform's
  Bearer scheme; the service prepends `Bearer `, the raw token is the field).
- Go package `internal/tools/paperform/` (id has no dashes → package name ==
  id). Registered `RegisterService("paperform", &paperform.Service{})` in
  `internal/tools/register.go` (batch-end shared-file merge).
- **JSON output shape** (notion precedent): each command prints the provider's
  JSON payload (list endpoints → the `results`/array payload; get endpoints →
  the object) to stdout; no reshaping beyond passthrough. Errors render as a
  `--json` structured envelope `{ "error": { "message", "status" } }` on stderr.
  Exit codes: `0` success, `1` runtime/API failure (typed `apiError`, carries
  Paperform's `4xx/5xx` + body), `2` usage/parse error. `429` maps to exit `1`
  with the `Retry-After` surfaced in the message.

## 4. Credential + auth flow (api_key lane)

- **Credential**: one secret, the account-page API key. Entered by the user
  through the connect UI (write-only `POST /connections/credentials`), stored in
  Vault, never in the bundle. Token-gateway projects it to
  `credential.fields.access_token: token.access_token`; anycli injects it as
  `PAPERFORM_API_KEY`. No refresh cycle — the key is long-lived until the user
  rotates it on the account page.
- **No Helio-side config**: `manual_api_token` needs **no** OAuth client
  id/secret. `auth.required_config_fields: []` → integration-service needs **no**
  `config/` + `deploy/` Secret append for Paperform (unlike the oauth lanes).
  This removes lane-1's per-provider config landing from Paperform's critical
  path; the provider is `configured: true` as soon as the bundle ships.
- **Verification + identity — the design decision.** Paperform has **no
  account/identity endpoint** (§0), so the standard `declarativeManualTokenVerifier`
  (which GETs a bundle-declared identity URL and reads an RFC-6901
  `stable_key` from the object) cannot apply — `GET /forms` returns a list with
  no account id. Decision:
  - **Verify liveness** with a bundle-declared header + a stable listing
    endpoint: `GET https://api.paperform.co/v1/forms?limit=1` with
    `Authorization: Bearer <token>`; reject non-2xx before any Vault write.
  - **Derive identity from the credential**, not the network: a deterministic
    **credential-fingerprint** `account_key`/`stable_key` (short SHA-256 prefix
    of the token, never the secret itself), with a static i18n label
    (`tools.account.paperform`). This is the same "Bearer-scheme verifier +
    fingerprint identity" pattern the sibling Forms/notification batch tools
    (tally, loops, knock #328) established for keyless single-token providers.
  - **Capability**: `identity.source: strategy` selecting a
    credential-fingerprint identity deriver. **Reuse** the shared fingerprint
    deriver **if** a sibling tool has already landed it on the batch merge base;
    otherwise this is the one narrow capability to add in integration-service
    (a `credentialFingerprintDeriver` implementing `manualTokenVerifier`:
    liveness GET on a bundle URL + `Authorization: Bearer` header, then
    `account_key = sha256(token)[:12]`). Verify presence on the merge base at
    stage 5; do not fork a second deriver if one exists.

## 5. Naming (three axes) — all identical, zero divergence

| Axis | Value |
|---|---|
| ① CLI command word (`tool.command`) | `paperform` (flat, no `tool.group`) |
| ② anycli tool id (`definitions/tools/<id>.json`) | `paperform` |
| ③ provider catalog key (bundle dir + `key:`) | `paperform` |

②==③ → **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`
(identity holds). No `toolGroups` entry. This is a clean single-token provider.

## 6. Helio provider bundle plan (`integrations/providers/paperform/provider.yaml`)

Hidden-first. Shape (manual_api_token / api_key):

```yaml
schema: helio.provider/v1
key: paperform
go_name: Paperform
presentation:
  name: Paperform
  description_key: paperform
  consent_domain: paperform.co
  visible: false            # hidden-first; flip is the single go-live change
  order: <batch-assigned>
auth:
  type: api_key
  owner: assistant
  required_config_fields: []   # no OAuth client — no config/ + deploy/ append
  api_key:
    header: Authorization      # Bearer scheme; service prepends "Bearer "
    setup_url: https://paperform.co/  # → account API-key page (confirm exact URL at stage)
identity:
  source: strategy             # credential-fingerprint deriver (§4)
  label_candidates: [/label]   # static label from the deriver
connection:
  mode: isolated
  disconnect_mode: local_only  # api key has no server-side revoke endpoint
  runtime_strategy: manual_api_token
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: paperform
  kind: api_key
```

- **Axis ①/②/③ in the bundle**: `tool.name: paperform` (②), no `tool.command`
  override (① defaults to ②), directory `paperform` == `key` (③).
- **Hidden-first rollout**: land bundle `visible: false`; bump the anycli pin
  (batch-end) so the pinned anycli ships the `paperform` definition; run L1–L5
  while hidden; flip `visible: true` + regenerate as the single go-live change.
  No `oauth_review` gate (api_key), so the visible flip gates only on L5 + docs.
- **Setup URL**: verify the exact account API-key page path at stage 4 (docs say
  "generate on your account page"); the bundle `setup_url` is safe presentation
  metadata, no secret.

## 7. Test plan (five layers) — external-credential map

| Layer | What it proves for Paperform | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: httptest fake asserts request path/method, `Authorization: Bearer` injection, list/get JSON passthrough, `--json` error envelope, exit codes 0/1/2, `429` handling | **No** |
| **L2** | dev harness vs **real** `api.paperform.co`: `ANYCLI_CRED_ACCESS_TOKEN=<key> anycli paperform -- form list` returns real forms; confirms field names, pagination flags, and the `/submit` question | **Yes** — real Paperform **Standard/Business** API key |
| **L3** | `provider-gen --check` (bundle strict-decode + invariants) + both repos' unit suites; **+ the fingerprint-deriver unit test** if §4 capability is added | **No** |
| **L4** | singleton + `POST /internal/test-only/connections/seed` (api_key providers are seedable) with `access_token`, then `heliox tool paperform -- form list` reaches the live API through the token gateway | **Yes** — same real key |
| **L5** | api_key key-entry path (master plan §2): connect link → paste key → `GET /connections` shows connected/configured → one **unseeded** `heliox tool paperform -- form list` succeeds. No OAuth consent (none exists) | **Yes** — real key + Paperform account |

- Layers needing externally supplied credentials: **L2, L4, L5** (one real
  Paperform API key from a Standard/Business-plan account; account pool lane).
  L1 and L3 are fully agent-runnable with no credentials.
- **L5 is agent-drivable** (api_key key-entry path via agent-browser, human
  lane-3 fallback on UI breakage) — no human-in-the-loop consent, since
  Paperform has no OAuth flow.
- On-branch L3/L4: run `provider-gen` + `--check` locally against the branch
  bundle and point `helio-cli/go.mod` at this anycli branch with a local
  `replace`; do **not** commit the regen or the replace (batch lead owns the
  canonical regen + pin bump).

## 8. Remaining checklist (stages 7–10)

- UI icon `ui/helio-app/src/integrations/icons/paperform.svg` + register in
  `providerIcons.ts` (manual, batch-end).
- i18n: `tools.descriptions.paperform`, `tools.account.paperform` (static
  identity label), any credential-field label key.
- AI-facing doc under `agents/plugins/heliox/skills/tool/` (read surface +
  "responses live under `submission list --form <id>`"); plugin version bump +
  marketplace publish ride the batch-end merge.
- Rollout: deploy hidden → L1–L5 green → flip `visible: true` + regenerate.
