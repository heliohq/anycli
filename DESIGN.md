# Crisp — per-tool design (`heliox tool crisp`)

**Status:** scratch design on branch `tool/crisp` (batch lead strips this file at
batch-end). **Catalog row:** #77 Crisp · anycli id `crisp` · provider key
`crisp` · auth lane `api_key` · Wave 2 · Support.

## 0. Audit reconciliation (official docs vs. catalog)

The 2026-07-21 OAuth audit verdict for row 79/Crisp is **"no viable multi-tenant
path — stays `api_key`."** Verified against Crisp's official docs and it holds —
**no divergence to record**:

- Crisp's only authentication is a **token keypair** (`identifier` + `key`) sent
  as HTTP Basic auth (`Authorization: Basic base64(identifier:key)`) plus an
  `X-Crisp-Tier` header (`plugin` or `website`). There is **no
  authorization-code redirect flow** a shared Helio app could have arbitrary
  customer accounts authorize.
  Sources: <https://docs.crisp.chat/guides/rest-api/authentication/plugin-token/>,
  <https://docs.crisp.chat/guides/rest-api/authentication/website-token/>.
- Crisp's "Connect"/Marketplace model is **plugin-subscription**, not OAuth: a
  workspace owner *installs* a marketplace plugin, which subscribes that
  plugin's single shared token to their website. That is a per-install grant to
  one shared token, not a per-user authorization-code grant that mints an
  isolated user token — so it fails the audit rubric's "multi-tenant
  authorization-code OAuth" test. `api_key` is correct.

The credential the user supplies is their own Crisp token keypair; it flows
through Helio's vault + token gateway exactly like MongoDB's connection string
(the `manual_credentials` precedent).

## 1. What an AI teammate does with Crisp → API surface

Crisp is a customer-messaging/support platform (live-chat widget, shared inbox,
CRM "People"). A Helio teammate sitting in a support workspace does inbox and
contact work, so the tool wraps the **Website Conversations**, **Messages**, and
**People** resources of the Crisp REST API v1 (base URL
`https://api.crisp.chat/v1/`). Everything is scoped to a `website_id` (the
workspace). Confirmed endpoint templates (docs REST reference):

| Teammate intent | Method + path |
|---|---|
| Triage the inbox | `GET /v1/website/{website_id}/conversations/{page_number}` |
| Read one thread | `GET /v1/website/{website_id}/conversation/{session_id}` |
| Read a thread's messages | `GET /v1/website/{website_id}/conversation/{session_id}/messages` |
| Reply to a customer | `POST /v1/website/{website_id}/conversation/{session_id}/message` |
| Resolve / change state | `PATCH /v1/website/{website_id}/conversation/{session_id}/state` |
| Assign / route | `PATCH /v1/website/{website_id}/conversation/{session_id}/routing` |
| Look up a contact | `GET /v1/website/{website_id}/people/profile/{people_id}` |
| List/search contacts | `GET /v1/website/{website_id}/people/profiles/{page_number}` |
| Add a contact | `POST /v1/website/{website_id}/people/profile` |
| Discover accessible workspace(s) | `GET /v1/plugin/connect/websites/all` |

The People-profile and Connect-websites path exactly (page suffix, query
params for search/filter such as `search_text`, `filter_*`) are re-verified at
L2 against the live API before the anycli pin is cut — the reference-doc render
truncates their parameter tables. Conversations/messages/state (rows 1–5) are
literally confirmed.

**Out of scope for v1** (deliberately subtracted): Visitors, Campaigns,
Helpdesk, Analytics, Website Settings, Media/Bucket. They are not what a support
teammate reaches for first; add verbs later if demand appears. Webhooks are
irrelevant (heliox is request/response, not a subscriber).

## 2. anycli definition

**Type: `service`** (stage-1 rubric). Crisp ships no official agent-friendly
CLI binary; the integration is the HTTP API, so this is a `service`-type tool in
`internal/tools/crisp/` — the same shape as `notion`/`slack`, not a `cli`
wrapper. Go package `crisp` (id has no dashes → package == id).

### Auth shape (the load-bearing detail)

Crisp needs three things per request: the token keypair, the tier, and the
website in the path. Only the keypair is a secret; tier is fixed and website is
a routing parameter.

- **Credential field (resolver-supplied):** `access_token`, carrying the
  colon-joined keypair string **`identifier:key`** — this is exactly Crisp's own
  documented `curl --user "{identifier}:{key}"` shape. Crisp identifiers/keys
  are colon-free (UUID/hex), so the service splits on the first `:`
  unambiguously. Injected as env `CRISP_TOKEN`.
- The service builds `Authorization: Basic base64(identifier:key)` itself (do
  not pre-base64 in storage — store the human-pasteable `identifier:key`).
- **Tier is fixed to `plugin`** — sent as a constant `X-Crisp-Tier: plugin`
  header, not a credential. Rationale in §3. (No env/field for it.)
- **`website_id` is a routing parameter, not a credential** — a global
  `--website <id>` flag, mirroring `mongodb`'s per-invocation `--db/--collection`.
  When omitted, the service auto-resolves via `GET /v1/plugin/connect/websites/all`:
  if the token can reach exactly one website, use it; if several, exit 2 with the
  candidate list so the AI re-runs with `--website`. `crisp website list` exposes
  the same discovery explicitly.

`auth` block:

```json
"auth": {
  "credentials": [
    { "source": { "field": "access_token" },
      "inject": { "type": "env", "env_var": "CRISP_TOKEN" } }
  ]
}
```

### Command tree (cobra, grouped by resource)

Follow the `notion` reference shape: a `BaseURL`/`HC`/`Out`/`Err` struct so
tests point at an `httptest` server; `--json` on every leaf; exit codes 0 / 1
(API failure via typed `apiError`) / 2 (usage). Global persistent `--website`.

```
crisp conversation list   [--website] [--page N] [--filter-status ...] --json
crisp conversation get     --session <id> [--website] --json
crisp conversation messages --session <id> [--website] [--page] --json
crisp conversation reply   --session <id> --text <msg> [--from operator] [--website] --json
crisp conversation state   --session <id> --state resolved|pending|unresolved [--website] --json
crisp conversation route   --session <id> --operator <email|id> [--website] --json
crisp people list          [--website] [--page N] [--search <q>] --json
crisp people get           --people <id> [--website] --json
crisp people create        --email <e> [--nickname <n>] [--website] --json
crisp website list         --json     # GET /plugin/connect/websites/all
```

**JSON output shape** — provider-neutral, agent-first (built-in service
convention 003 §3): every leaf prints a single JSON object
`{"data": <crisp payload>, "meta": {"website_id": "...", "page": N}}` on
success; errors print `{"error": {"code": "...", "message": "...",
"status": <http>}}` on stderr with the matching non-zero exit. List verbs pass
through Crisp's `data` array plus `meta.page`; the raw Crisp envelope
(`{error, reason, data}`) is unwrapped so the AI never parses Crisp's transport
error dialect.

### Tests (L1, httptest fakes, TDD first)

- Auth header assembly: `CRISP_TOKEN=abc:def` → request carries
  `Authorization: Basic <base64("abc:def")>` **and** `X-Crisp-Tier: plugin`.
- `--website` omitted → service calls `connect/websites/all`; single-website
  fake auto-selects; multi-website fake exits 2 with candidate list.
- Each verb: request method/path/body shape + injected headers.
- Error rendering: Crisp `{error:true, reason:"...", data:...}` and a raw HTTP
  4xx/5xx both render the `{"error":...}` envelope in plain and `--json` modes;
  `invalid_session` (bad/mis-encoded token) maps to a clear "reconnect" message.
- Never hit the real API from a unit test.

## 3. Credential model, auth flow & token semantics (verified vs. official docs)

**Token registration model.** Crisp issues token keypairs (`identifier` + `key`)
in two tiers:

- **Plugin token** (recommended, tier `plugin`): created in the Crisp
  Marketplace. A **development** plugin token is Crisp's own recommended
  getting-started path — "the best way to get started… easily generate a token
  key/identifier pair," usable on **all plugin-tier routes with no scope
  restriction**, bound to the developer's **own trusted website**, lower quota.
  A **production** plugin token (for publishing) requires declaring route
  scopes and can span multiple subscribed websites. Plugin-tier tokens are
  **exempt from per-minute rate limiting** (daily quota instead), which suits an
  agent that bursts.
- **Website token** (tier `website`): generated by a workspace **owner** in
  settings; single-workspace, 10,000 req/day, owner-managed.

**v1 fixes tier = `plugin`** and connects with a **plugin development token**,
because: (a) it is Crisp's recommended self-serve path, (b) it is not
per-minute-rate-limited, (c) tier `plugin` is the one that can call
`plugin/connect/websites/all`, enabling the zero-config `--website`
auto-resolve. Website-tier support is a documented future option — it would
require carrying the tier alongside the secret, which the single-secret storage
face (below) does not allow today, so it is intentionally deferred, not smuggled
in.

**Token semantics.** Keypairs are **long-lived, non-expiring, non-refreshing**
(there is no refresh cycle) — the token-gateway serves the stored secret
directly, like a Slack bot token. Revocation is user-side (regenerate/revoke in
Crisp); Helio disconnect is `local_only`.

**Scopes.** A development plugin token is unscoped (all plugin routes).
Production tokens declare scopes covering the conversation/people/message routes
in §1 — relevant only at the eventual marketplace-publish step, not for the
`api_key` connect path.

**Single-secret storage constraint (design 317 D5).** integration-service
stores a manual credential as **exactly one secret in the user-token payload**
(`validateCredentialInputSchema`: the connect form must be exactly one required
field; `knownCredentialSources` = `token.access_token`,
`connection.account_key`, `connection.metadata.person_urn`). Crisp fits **with
zero capability growth** because the whole secret is the one `identifier:key`
string; tier is constant and website is a routing arg, so neither needs storage.
This is the same envelope as `mongodb` (one composite connection string) — no
new credential source, no token-gateway change, no service-side adapter.

## 4. Helio provider bundle plan (`integrations/providers/crisp/`)

Three naming axes all coincide → **no `toolToProvider` entry** (identity holds):

| Axis | Value |
|---|---|
| ① CLI command word | `crisp` |
| ② anycli tool id | `crisp` |
| ③ provider catalog key | `crisp` |

`provider.yaml` (hidden-first; modeled on `mongodb`'s `manual_credentials`
bundle):

```yaml
schema: helio.provider/v1
key: crisp
go_name: Crisp

presentation:
  name: Crisp
  description_key: crisp
  consent_domain: crisp.chat
  visible: false            # flip only after L5 + icon + locales + pin land

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: access_token          # the identifier:key keypair
        label_key: crisp_token
        secret: true
        placeholder: "identifier:key"
        required: true
    setup_url: https://docs.crisp.chat/guides/rest-api/authentication/plugin-token/

identity:
  source: strategy                   # no HTTPS userinfo for a keypair

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
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: crisp
  kind: api-key
```

**No integration-service Go.** `manual_credentials` needs zero server code and
no `required_config_fields` (validated: that strategy "does not use server
config fields"). No `adapter_*.go`. **No `config/` or `deploy/` secret** — the
user's keypair is the only credential and it enters via
`POST /connections/credentials` into Vault; there is no Helio-side client
id/secret to land (unlike OAuth providers), so lane-1 config is a no-op for
Crisp.

**Generation:** from `go-services/integration-service`, `go run
./cmd/provider-gen` then `--check`; commit all five projections together with the
bundle at batch-end.

**UI icon (manual, never generated):** `ui/helio-app/src/integrations/icons/crisp.svg`
+ register in `providerIcons.ts`.

**AI-facing docs:** add a `crisp` sub-doc under
`agents/plugins/heliox/skills/tool/` covering the verbs, the `--website`
auto-resolve/override behavior, and that the credential is the plugin
**development** token's `identifier:key` keypair; bump + publish the plugin at
batch-end.

## 5. Test plan → five layers

| Layer | Crisp specifics | External creds? |
|---|---|---|
| **L1** anycli unit | httptest fakes per §2: Basic+`X-Crisp-Tier` header assembly, `--website` auto-resolve vs. exit-2 ambiguity, per-verb request shape, Crisp/HTTP error → `{"error"}` envelope, `invalid_session` handling. `go test ./...` green. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN="identifier:key" anycli crisp -- website list`, then `conversation list`, `conversation reply`, `people get` against a **real Crisp workspace + development plugin token**. Confirms field name/injection and the still-unverified People/Connect path+param shapes. | **Yes** — real Crisp workspace + plugin dev token (account pool) |
| **L3** generate + suites | `provider-gen --check` clean; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service suite. (On-branch: local `replace` → anycli branch + local regen, uncommitted.) | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"crisp"`, `access_token:"identifier:key"` (seedable — it is a user-token/api_key provider), then `heliox tool crisp -- website list` / `conversation list` reaches the live API through the real token gateway. **Seed `access_token` only** — no refresh cycle to exercise. | **Yes** — same real keypair as L2 |
| **L5** connect flow (pre-flip) | api_key key-entry path (master plan §2): open the connect link → paste the `identifier:key` keypair via the real connect UI (`POST /connections/credentials`) → connection shows connected/`configured` in `GET /connections` → one **unseeded** live `heliox tool crisp -- conversation list` succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. Run once, still hidden, before flipping `visible: true`. | **Yes** — real keypair through the connect UI |

L1/L3 need no credentials; **L2, L4, L5 all consume the same externally supplied
Crisp development-plugin-token keypair + a real workspace** (human lane 2 account
pool). No OAuth app registration (lane 1) is required — Crisp is `api_key`.

## 6. Rollout

Ship hidden (`visible: false`) with the batch; land anycli tool + pin, bundle +
five projections, resolver (no entry needed), icon, docs at batch-end; run
L1–L4 on-branch and the L5 key-entry sweep post-merge; then flip
`visible: true` + regenerate as the single go-live change.
