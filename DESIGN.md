# Courier — per-tool design (`heliox tool courier`)

Scratch design for the Courier tool provider. Batch-lead strips this at batch end.

- **Catalog row:** 269 — Product *Courier*, anycli id `courier`, provider key `courier`, auth `api_key`, wave 3, category *Marketing & Notifications*.
- **OAuth audit verdict (row 271 in the audit file):** `api_key`, "no viable multi-tenant path".
- **Naming axes:** ① CLI word `courier`, ② anycli id `courier`, ③ provider key `courier` — all identical. No `toolToProvider` divergence entry, no group. (Verified against master-plan §3 and `helio-cli/internal/toolcred/resolver.go`.)

## 0. Independent verification of the auth lane (official docs vs catalog)

Checked against the official Courier API reference (`courier.com/docs/reference/authorization`, `/docs/reference/api-overview`, `/docs/llms.txt`):

- **Auth:** `Authorization: Bearer <API_KEY>`. A missing/invalid key returns `401`. Keys are workspace-scoped (per Courier's Test/Production environments) and minted self-serve in the Courier dashboard — no app registration, no consent screen.
- **Multi-tenant OAuth:** none. Courier publishes no authorization-code flow by which arbitrary customer workspaces authorize a single shared Helio app. Third-party access is exclusively the workspace's own Bearer key.
- **No account/whoami identifier.** Confirmed against `/docs/llms.txt`: there is **no** `/me`, `/account`, or workspace-info endpoint. `GET /tenants` and `GET /brands` return user-defined scope objects (tenant ids are caller-supplied; brand objects carry only per-brand `brand_id`), never a system-generated stable workspace/account id. So there is nothing to extract as a JSON-pointer identity `stable_key`.
- **Conclusion:** the audit verdict and the catalog agree with the official docs — **`api_key` lane is correct**. Two divergences from a naive reading of "api_key", both forced by the repo and recorded here:
  - **The bundle realizes the api_key lane as `auth.type: credentials`, not `auth.type: api_key`.** The `api_key` auth type is coupled (via `runtime_strategy: manual_api_token` → `declarativeManualTokenVerifier`) to a GET that extracts a non-empty JSON-pointer `stable_key` from the response body — which Courier has none of. The precedent for a *verify-first, whoami-less* api-key provider is **moz** (a shipped api-key-lane tool), which ships `auth.type: credentials` + `runtime_strategy: manual_credentials` + a compiled verifier, and marks the client-routing lane with `tool.kind: api-key`. Courier follows that shape. mongodb is the same `credentials`/`manual_credentials`/`kind: api-key` shape for its (no-verify) case.
  - **The liveness endpoint is a compiled Go constant, not a bundle `identity.url`.** `cmd/provider-gen/validate.go` (`validateIdentity`) rejects `identity.url` for any `identity.source` other than `userinfo`, and `userinfo` in turn requires a JSON-pointer `stable_key`. A whoami-less provider therefore **cannot** carry its probe URL in the bundle; moz's `mozQuotaVerifier` hardcodes `mozJSONRPCURL` and keeps `identity.source: strategy` with no `url`. Courier does the same.

## 1. API surface wrapped, and why

Base URL `https://api.courier.com`. Courier is *notification infrastructure*: an AI teammate uses it to **send a notification** to a person/list/audience across the workspace's configured channels (email, SMS, push, Slack, etc.), then **track and manage** what it sent. The tool wraps the endpoints that serve that job and skips the design-time surface (template authoring, brand design, tenant provisioning at scale) that a human configures in the Courier UI.

| Verb (CLI) | Method + path | Why an AI teammate needs it |
|---|---|---|
| `send` | `POST /send` | The core action: dispatch a notification to a recipient (`user_id`/`email`/`phone_number`/`list_id`/`audience_id`) using a `template` id or inline `content` (`title`+`body`), with `data`, `routing`, `brand_id`. Returns `202` + `requestId`. |
| `message get` | `GET /messages/{id}` | Check delivery status/outcome of a `requestId` (send returns `202` "accepted", not "delivered"). |
| `message list` | `GET /messages` | List recent messages (cursor `paging`), filter by recipient/status/notification/tag. |
| `message history` | `GET /messages/{id}/history` | Per-message delivery timeline (enqueued → sent → delivered / error). |
| `message cancel` | `POST /messages/{id}/cancel` | Cancel an enqueued/delayed message. |
| `list list` / `list get` | `GET /lists`, `GET /lists/{id}` | Discover mailing lists to target with `list_id`. |
| `list subscribe` / `list unsubscribe` | `PUT`/`DELETE /lists/{id}/subscriptions/{user}` | Manage list membership. |
| `audience list` / `audience get` | `GET /audiences`, `GET /audiences/{id}` | Discover audiences to target with `audience_id`. |
| `profile get` | `GET /profiles/{id}` | Read a recipient profile (channels on file). |
| `profile subscriptions` | `GET /profiles/{id}/lists` | Lists a user is subscribed to. |
| `brand list` / `brand get` | `GET /brands`, `GET /brands/{id}` | Resolve a `brand_id` for a branded send. |
| `automation invoke` | `POST /automations/invoke` | Trigger an ad-hoc automation run. |

Cut from scope (design-time / low AI value): template CRUD (`POST/PUT /notifications/...`), brand CRUD, tenant provisioning writes, bulk jobs. They can be added later as verbs without reshaping the tree.

`send` is the load-bearing verb; everything else is read/track/target discovery around it. Pagination follows Courier's own split: `messages`/`profiles` return a `results` array with a `paging` cursor; `lists`/`audiences` return `items`. The tool surfaces `--cursor`/`--limit` and passes the provider's cursor through unmodified.

## 2. anycli definition

**Type: `service`** (per SKILL.md stage-1 rubric). No official Courier CLI binary exists to wrap, so the `cli` path is out; implement a `service` tool against the REST API — matching 21/23 shipped definitions and the `notion` reference shape.

- **Definition:** `definitions/tools/courier.json` — `name: courier`, `type: service`, one credential binding: `source.field: api_key` → `inject: {type: env, env_var: COURIER_API_KEY}`.
- **Package:** `internal/tools/courier/` (id has no dashes/leading digit, so the Go package name is `courier`). Registered `RegisterService("courier", &courier.Service{})` in `internal/tools/register.go` (the one shared file that lands at batch end).
- **Service shape (copy `internal/tools/notion/`):** `Service{ BaseURL, HC, Out, Err }` so unit tests point `BaseURL` at an `httptest.Server`. `DefaultBaseURL = "https://api.courier.com"`. The service reads `COURIER_API_KEY` from the injected env and sets `Authorization: Bearer <key>` + `Accept: application/json` on every request.
- **cobra tree:** top-level `send`; resource groups `message` (`get`/`list`/`history`/`cancel`), `list` (`get`/`list`/`subscribe`/`unsubscribe`), `audience` (`get`/`list`), `profile` (`get`/`subscriptions`), `brand` (`get`/`list`), `automation` (`invoke`). Runnable group commands (`newGroupCmd`) so an unknown subcommand fails rather than exiting 0.
- **`send` flags:** recipient (mutually exclusive `--user-id` / `--email` / `--phone` / `--list-id` / `--audience-id`), content (`--template` XOR `--title`+`--body`), plus `--data` (JSON), `--routing` (JSON), `--brand-id`. Builds the `{ "message": { ... } }` body; usage errors (missing recipient, both template and title) exit 2 before any HTTP call.
- **JSON output shape:** provider JSON is passed through on stdout (send → `{"requestId":"..."}`; list → `{"results":[...],"paging":{...}}`). Global `--json` flag forces the structured **error** envelope `{"error":{"message":...,"kind":"usage|api","status":<HTTP>}}`; without it, errors render as plain text to stderr. **Exit codes:** 0 success, 1 runtime/API failure (Courier non-2xx via a typed `apiError` carrying `status`), 2 usage/parse. A `401` maps to `apiError{status:401}` so the token gateway/credential-rejection loop can classify it.

## 3. Credential fields & auth flow (api_key lane, realized as `credentials` + `manual_credentials`)

- **What the user supplies:** one field — the Courier API key (Bearer token). Connect-form schema `auth.credential_input.fields: [{ name: api_key, label_key: api_key, secret: true, required: true }]` with `auth.credential_input.setup_url: https://app.courier.com/settings/api-keys` (`validateCredentialInput` requires a setup_url when `auth.type: credentials`). The key never touches the bundle; it enters via the write-only `POST /connections/credentials` and is stored in Vault.
- **Outbound projection:** bundle `credential.fields: { api_key: token.access_token, account_key: connection.account_key }`. The single pasted secret is stored on the user-token write path as `token.access_token` and projected back under the neutral field name `api_key`, which the anycli definition's `source.field: api_key` consumes → `COURIER_API_KEY` env. The derived per-key `account_key` (see below) is projected under `connection.account_key`, exactly like bitly/mongodb/moz, so the identity reaches the connection record/runtime consistently with siblings. (Harness parity: `ANYCLI_CRED_API_KEY` → `api_key`.)
- **Bundle auth block:** `auth.type: credentials`, `identity.source: strategy` (no `url`, no `stable_key`), `connection: { mode: isolated, disconnect_mode: local_only, runtime_strategy: manual_credentials }`, `tool.kind: api-key` (wire-compat client routing).

### The one real design decision — verify-first identity for a whoami-less Bearer provider

Courier has **no account/whoami endpoint** (confirmed in §0 against the official reference — only user-defined scope endpoints exist) and its key is an **opaque Bearer token** with no parseable structure and no per-account stable field in any response. This is the exact situation **moz** already shipped and had reviewed. The two stock verifiers do **not** compose into what Courier needs, and neither should be described as a reuse here:

- `declarativeManualTokenVerifier` (the `manual_api_token` default) `GET`s `identity.url` and extracts a non-empty `stable_key` JSON pointer from the response, erroring if it is absent. Courier has no stable field, so this verifier cannot produce an account key at all. The tally/semrush/fullstory `auth.api_key.scheme: bearer` field is a modifier **on this same declarative verifier** — it still requires a `stable_key`, so it does not apply to a whoami-less provider. (tally can use it only because tally *does* expose `GET /users/me` with `/id`.)
- `dsnHostIdentityDeriver` (`manual_credentials`) does **no** HTTP call at all (verified in `service/manual_credentials_identity.go`) and derives identity by parsing a **DSN host** — on an opaque non-DSN token it returns a `manualCredentialFormatError`, failing every Connect. So it is neither a liveness check nor a usable no-verify path for Courier.

**Decision — a single new compiled capability, following the moz precedent exactly:** add one narrow, reviewed **`courierBrandsVerifier`** to integration-service, selected via the per-provider `manualCredentialsVerifier(provider)` switch that moz introduced under `case RuntimeStrategyManualCredentials` in `composeProviderRegistration` (that switch replaces the courier-branch-point default of an unconditional `dsnHostIdentityDeriver{}`; see §4a for the wiring/merge note). The verifier, mirroring `mozQuotaVerifier`:

1. **Liveness via a hardcoded Bearer GET.** It `GET`s a compiled Go constant `courierBrandsURL = "https://api.courier.com/brands"` with `Authorization: Bearer <token>` + `Accept: application/json`. The endpoint is a **constant, not a bundle `identity.url`** — `provider-gen` (`validateIdentity`) forbids `identity.url` for any `identity.source` except `userinfo`, and `userinfo` requires a `stable_key` Courier lacks; moz solves this the same way with `mozJSONRPCURL`. `GET /brands` is a cheap, generously rate-limited (200/min) authenticated read that returns `200` (even `results:[]` for an empty workspace) with a live key and `401` with a bad one — a real provider-side check, not a silent no-verify downgrade.
2. **Non-2xx ⇒ rejection.** Any non-2xx (notably `401`) maps to `manualCredentialFormatError` — a user-input problem surfaced before any Vault write, never a server fault; the static message never echoes the secret.
3. **Fingerprint fallback for the account key.** Because no Courier response carries a stable id, it derives `account_key` from a **non-reversible fingerprint of the token** (truncated SHA-256), with a human-readable masked label `"Courier API token ····<last4>"` (moz's readable-spirit fallback; moz uses a bare 4-char tail — a truncated hash is used here for stronger cross-key dedup so two distinct Courier keys never collide on `(org, provider, account_key)`). The raw token never enters the identity map, keeping Connection metadata secret-free.

This is a **new hybrid** (Bearer GET liveness + fingerprint identity), not a reuse of the no-endpoint DSN deriver nor of the declarative verifier — named as one capability so review scope is one file (`service/manual_courier_verifier.go` + its `_test.go`, mirroring `manual_moz_verifier.go`).

Disconnect is `local_only` (Courier has no key-revocation API; deleting the Vault credential is the whole story). No refresh cycle — a Courier key is a non-expiring PAT the gateway serves directly.

## 4. Helio provider bundle plan (hidden-first)

`integrations/providers/courier/provider.yaml`, `key: courier`, `go_name: Courier`, `presentation.visible: false` (hidden-first; runs as a cobra-hidden command through L4/L5).

```yaml
schema: helio.provider/v1
key: courier
go_name: Courier
presentation:
  name: Courier
  description_key: courier
  consent_domain: courier.com
  visible: false            # flip true only as the single go-live change after L5
  order: <batch-assigned>
auth:
  type: credentials          # api_key lane realized as credentials (whoami-less, no stable_key) — moz/mongodb shape
  owner: individual
  credential_input:
    setup_url: https://app.courier.com/settings/api-keys   # required for auth.type: credentials
    fields:
      - { name: api_key, label_key: api_key, secret: true, required: true }
identity:
  source: strategy           # compiled courierBrandsVerifier; endpoint is a Go constant, so NO url / NO stable_key
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    api_key: token.access_token
    account_key: connection.account_key   # carries the derived per-key fingerprint identity (bitly/mongodb/moz parity)
tool:
  name: courier
  kind: api-key              # wire-compat client routing value; auth.type stays credentials
```

Field names/enums are validated against `cmd/provider-gen/manifest.go` + `validate.go` on this branch. There is **no** `auth.api_key` block and **no** `scheme` field — the Bearer header is compiled into `courierBrandsVerifier` (like moz's `x-moz-token`), not declared in the bundle. Then, per SKILL stages 5–10:

- **Generate** (batch-end, batch lead): `provider-gen` + `provider-gen --check` → the five projections committed together. On-branch: run both locally for L3, **do not commit** the regen (would red-CI the batch).
- **Config:** api_key-lane providers need **no** Helio client id/secret; `manual_credentials` declares no `required_config_fields` (validated by `validateConfigContract`), so there is nothing to add to `config/`/`deploy/`. (Config Sync rule is a no-op here.)

### 4a. integration-service capability (the one code change)

- **`courierBrandsVerifier`** in `service/manual_courier_verifier.go` (+ `_test.go`), implementing `manualTokenVerifier` — the Bearer-GET-liveness + fingerprint-identity hybrid from §3. Structure and error vocabulary mirror `manual_moz_verifier.go` (`manualCredentialFormatError` on reject; identity map excludes the secret).
- **Registry wiring.** moz replaced the courier-branch-point `case RuntimeStrategyManualCredentials: → dsnHostIdentityDeriver{}` with a per-provider `manualCredentialsVerifier(provider)` switch (mongodb → DSN no-verify, moz → quota verify). Courier adds one arm: `courier → courierBrandsVerifier{}`. If moz has already merged to `main` when this batch lands, extend that switch; if not, Courier introduces the switch (generalizing mongodb+moz+courier) in the same change. Confirm at batch time via `grep manualCredentialsVerifier service/provider_registry.go` — see Open item 2.
- No other service code: `manual_credentials` is otherwise config-free, uses the existing UpsertUserToken write path, and projects through the existing non-expiring-PAT branch (zero new `CredentialSource`, zero token-gateway change).
- **UI icon:** `ui/helio-app/src/integrations/icons/courier.svg` + register in `providerIcons.ts` (manual, never generated). i18n: `tools.credentialField.api_key` already exists from sibling api_key tools; add `description_key` string `courier` if absent.
- **AI-facing docs:** provider sub-doc under `agents/plugins/heliox/skills/tool/` (`send`/track/target examples), plugin version bump + marketplace publish once per batch.

## 5. Test plan — five layers

| Layer | What it proves for Courier | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | Definition + `internal/tools/courier` unit tests against an `httptest` fake: `send` body assembly (`{message:{to,content,data,routing}}`), recipient mutual-exclusion (exit 2), `Authorization: Bearer` header injection, `messages`/`brands`/`audiences` GET shapes, `--json` error envelope + exit-code contract (0/1/2), 401→`apiError{status:401}`. Never hits the real API. | No |
| **L2** `anycli courier -- <args>` harness vs real `api.courier.com` | `ANYCLI_CRED_API_KEY=<real key> anycli courier -- send --user-id ... --title hi --body test`, then `anycli courier -- message get <requestId>` and `message list`. Proves field names/injection/request shapes match the live API — mandatory before pin bump. | **Yes — a real Courier API key** (from the test-account pool) |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle strict-decodes; `courier`/`courier` unique; directory-key equality; HTTPS `credential_input.setup_url`; `identity.source: strategy` with **no** `url`/`stable_key` validates (a `url` here would fail `validateIdentity`); `credential.fields` carries both `api_key`/`account_key`; `courierBrandsVerifier` unit tests (200 live, 401 → `manualCredentialFormatError`, fingerprint account_key stable per key); `helio-cli` builds against the anycli branch via a local `replace`; `go test ./cmd/heliox/cmds/tool/`. | No |
| **L4** singleton + seeded credential | `POST /internal/test-only/connections/seed` with `provider:"courier"`, `access_token:<real key>` (api_key providers are seedable; seed `access_token` only, no refresh). Then `heliox tool courier -- send ...` resolves the token through `GET /connections/token` and hits the live API. Bypasses the connect UI. | **Yes — the same real key** reaches the live API; needs a real seeded assistant/org identity in local Mongo |
| **L5** full connect flow (once, pre-visible-flip) | **key-entry path** (not the OAuth checklist): `heliox tool courier auth` → open connect link → paste the key through the real connect UI (`POST /connections/credentials`, which runs `courierBrandsVerifier`: Bearer `GET /brands` liveness + fingerprint account_key) → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool courier -- send ...` succeeds through the token gateway. Agent-drivable (agent-browser), human fallback on UI breakage. | **Yes — real key**; validates the verify-first + fingerprint-identity path end to end |

**Credential-gated layers:** L2, L4, L5 all need one real Courier API key from the account pool; L1/L3 need none. Courier offers a self-serve free tier, so the key is procurable without partner review — no registration lane, which is exactly why it sits in `api_key`.

## Open items to settle at stage 1 (real-API exploration)

1. **Liveness probe holds** — confirm at L2 that `GET /brands` returns `200` for a live key on an **empty** workspace (`results:[]`, not `404`) so the verifier's status-only liveness check is correct; if not, substitute the cheapest always-200 authenticated read (candidate: `GET /messages?limit=1`) as the `courierBrandsVerifier` constant. (Official docs settle the identity question: there is **no** stable account id, so the fingerprint path is fixed — there is no `userinfo` alternative to weigh.)
2. **Registry-switch merge state** — `grep manualCredentialsVerifier go-services/integration-service/service/provider_registry.go` at batch time. Present (moz merged) → add the `courier` arm. Absent → introduce the per-provider switch (mongodb+moz+courier) in this change, replacing the unconditional `dsnHostIdentityDeriver{}`.
3. **Pagination field split** — verify at L2 that `messages`/`profiles` use `results`/`paging` and `lists`/`audiences` use `items`; wire the cursor flags to whichever the live API returns.
