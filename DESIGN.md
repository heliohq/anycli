# Courier — per-tool design (`heliox tool courier`)

Scratch design for the Courier tool provider. Batch-lead strips this at batch end.

- **Catalog row:** 269 — Product *Courier*, anycli id `courier`, provider key `courier`, auth `api_key`, wave 3, category *Marketing & Notifications*.
- **OAuth audit verdict (row 271 in the audit file):** `api_key`, "no viable multi-tenant path".
- **Naming axes:** ① CLI word `courier`, ② anycli id `courier`, ③ provider key `courier` — all identical. No `toolToProvider` divergence entry, no group. (Verified against master-plan §3 and `helio-cli/internal/toolcred/resolver.go`.)

## 0. Independent verification of the auth lane (official docs vs catalog)

Checked against the official Courier API reference (`courier.com/docs/reference`, `/docs/api-reference/send/send-a-message`, `/docs/llms.txt`):

- **Auth:** `Authorization: Bearer <API_KEY>`. A missing/invalid key returns `401`. Keys are workspace-scoped (per Courier's Test/Production environments) and minted self-serve in the Courier dashboard — no app registration, no consent screen.
- **Multi-tenant OAuth:** none. Courier publishes no authorization-code flow by which arbitrary customer workspaces authorize a single shared Helio app. Third-party access is exclusively the workspace's own Bearer key.
- **Conclusion:** the audit verdict and the catalog agree with the official docs — **`api_key` lane is correct**. No divergence to record in this file.

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

## 3. Credential fields & auth flow (api_key lane)

- **What the user supplies:** one field — the Courier API key (Bearer token). Connect-form schema `auth.credential_input.fields: [{ name: api_key, label_key: api_key, secret: true, required: true }]`, `setup_url: https://app.courier.com/settings/api-keys`. The key never touches the bundle; it enters via the write-only `POST /connections/credentials` and is stored in Vault.
- **Outbound projection:** bundle `credential.fields: { api_key: token.access_token }`. The token gateway stores the key as `token.access_token` and projects it back under the neutral name `api_key`, which the anycli definition's `source.field: api_key` consumes → `COURIER_API_KEY` env. (Harness parity: `ANYCLI_CRED_API_KEY` → `api_key`.)
- **Bundle auth block:** `auth.type: api_key`, `auth.api_key: { header: Authorization, setup_url: https://app.courier.com/settings/api-keys }`, `connection: { mode: isolated, disconnect_mode: local_only, runtime_strategy: manual_api_token }`.

### The one real design decision — verify-first identity for a whoami-less Bearer provider

Courier has **no account/whoami endpoint** (confirmed in the official reference — only resource endpoints exist) and its key is an **opaque Bearer token** with no parseable structure. Neither stock verifier fits as shipped:

- `declarativeManualTokenVerifier` (the `manual_api_token` default) `GET`s `identity.url` and (a) sets the header to the **raw token** (no scheme) and (b) extracts a `stable_key` JSON pointer from the response. Courier needs `Authorization: Bearer <token>` and returns **no per-account stable field** anywhere.
- `dsnHostIdentityDeriver` (`manual_credentials`) derives identity from a connection-string host — Courier's key is not a DSN.

**Decision (recommended, Option A):** ship `manual_api_token` with two narrow integration-service capabilities, reusing whatever a sibling api_key-without-whoami provider already landed on `main` at batch time:

1. **Bearer scheme on the verifier** — an optional `auth.api_key.scheme: bearer` (default raw) so the verifier sends `Authorization: Bearer <token>`. This is the exact capability the **tally** (task "Bearer-scheme verifier capability (reuse or add)"), **moz**, **semrush**, and **fullstory** api_key providers added; **reuse it, do not fork**.
2. **Verify against a benign authenticated endpoint + synthetic identity** — point the verifier at a cheap read (`GET /brands` — `200` with a live key, `401` without) purely as a liveness check, and, since no Courier response carries a stable account id, derive the account key as a short **non-reversible fingerprint of the token** (label `"Courier"`). If a sibling whoami-less provider already added a `token_fingerprint`/constant deriver, reuse it; otherwise add one narrow reviewed deriver (identity.source `strategy`, mirroring `dsnHostIdentityDeriver`'s no-endpoint pattern). A fingerprint keeps `(org, provider, account_key)` distinct per distinct key so two Courier connections don't collide, while never storing the raw token in the account key.

**Option B (only if stage-1 real-API exploration finds a stable identifier):** if any Courier `GET` is confirmed to return a stable per-workspace id, use plain `manual_api_token` + the Bearer scheme + that endpoint as `identity.url` with the stable field as `stable_key` — the simpler, endpoint-backed path. Stage-1 research settles A vs B before the dev branch; default is **A**.

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
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    scheme: bearer           # capability (reuse tally/moz/semrush/fullstory)
    setup_url: https://app.courier.com/settings/api-keys
  credential_input:
    fields:
      - { name: api_key, label_key: api_key, secret: true, required: true }
identity:
  source: strategy           # token-fingerprint deriver (Option A); or userinfo (Option B)
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    api_key: token.access_token
tool:
  name: courier
  kind: api-key
```

Field names/enums are validated against `cmd/provider-gen/manifest.go` + `validate.go` on this branch; `scheme` is the reused capability field. Then, per SKILL stages 5–10:

- **Generate** (batch-end, batch lead): `provider-gen` + `provider-gen --check` → the five projections committed together. On-branch: run both locally for L3, **do not commit** the regen (would red-CI the batch).
- **Service code:** none beyond the two reused/narrow capabilities above — `manual_api_token` is otherwise config-free.
- **Config:** api_key providers need **no** Helio client id/secret; nothing to add to `config/`/`deploy/`. (Config Sync rule is a no-op here.)
- **UI icon:** `ui/helio-app/src/integrations/icons/courier.svg` + register in `providerIcons.ts` (manual, never generated). i18n: `tools.credentialField.api_key` already exists from sibling api_key tools; add `description_key` string `courier` if absent.
- **AI-facing docs:** provider sub-doc under `agents/plugins/heliox/skills/tool/` (`send`/track/target examples), plugin version bump + marketplace publish once per batch.

## 5. Test plan — five layers

| Layer | What it proves for Courier | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | Definition + `internal/tools/courier` unit tests against an `httptest` fake: `send` body assembly (`{message:{to,content,data,routing}}`), recipient mutual-exclusion (exit 2), `Authorization: Bearer` header injection, `messages`/`brands`/`audiences` GET shapes, `--json` error envelope + exit-code contract (0/1/2), 401→`apiError{status:401}`. Never hits the real API. | No |
| **L2** `anycli courier -- <args>` harness vs real `api.courier.com` | `ANYCLI_CRED_API_KEY=<real key> anycli courier -- send --user-id ... --title hi --body test`, then `anycli courier -- message get <requestId>` and `message list`. Proves field names/injection/request shapes match the live API — mandatory before pin bump. | **Yes — a real Courier API key** (from the test-account pool) |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle strict-decodes; `courier`/`courier` unique; directory-key equality; HTTPS setup URL; the `scheme:bearer` capability + `strategy` identity validate; `helio-cli` builds against the anycli branch via a local `replace`; `go test ./cmd/heliox/cmds/tool/`. | No |
| **L4** singleton + seeded credential | `POST /internal/test-only/connections/seed` with `provider:"courier"`, `access_token:<real key>` (api_key providers are seedable; seed `access_token` only, no refresh). Then `heliox tool courier -- send ...` resolves the token through `GET /connections/token` and hits the live API. Bypasses the connect UI. | **Yes — the same real key** reaches the live API; needs a real seeded assistant/org identity in local Mongo |
| **L5** full connect flow (once, pre-visible-flip) | **api_key key-entry path** (not the OAuth checklist): `heliox tool courier auth` → open connect link → paste the key through the real connect UI (`POST /connections/credentials`, verified against `GET /brands` with the Bearer scheme) → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool courier -- send ...` succeeds through the token gateway. Agent-drivable (agent-browser), human fallback on UI breakage. | **Yes — real key**; validates the verify-first + fingerprint-identity path end to end |

**Credential-gated layers:** L2, L4, L5 all need one real Courier API key from the account pool; L1/L3 need none. Courier offers a self-serve free tier, so the key is procurable without partner review — no registration lane, which is exactly why it sits in `api_key`.

## Open items to settle at stage 1 (real-API exploration)

1. **Identity Option A vs B** — confirm whether any Courier `GET` returns a stable per-workspace id (→ B, endpoint-backed `userinfo`); else A (token-fingerprint `strategy`). Default A.
2. **Reuse check** — confirm the Bearer-scheme verifier capability and the whoami-less identity deriver are on `main` (tally/moz/semrush/fullstory precedents) before adding anything; only add the narrow deriver if truly absent.
3. **Pagination field split** — verify at L2 that `messages`/`profiles` use `results`/`paging` and `lists`/`audiences` use `items`; wire the cursor flags to whichever the live API returns.
