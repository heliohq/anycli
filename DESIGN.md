# Crisp — per-tool design (`heliox tool crisp`)

**Status:** scratch design on branch `tool/crisp` (batch lead strips this file at
batch-end). **Catalog row:** #79 Crisp · anycli id `crisp` · provider key
`crisp` · auth lane `api_key` · Wave 2 · Support.

## 0. Audit reconciliation (official docs vs. catalog)

The 2026-07-21 OAuth audit verdict for row 79/Crisp is **"no viable multi-tenant
path — stays `api_key`."** Verified against Crisp's official docs and it holds —
**no divergence to record**:

- Crisp's only authentication is a **token keypair** (`identifier` + `key`) sent
  as HTTP Basic auth (`Authorization: Basic base64(identifier:key)`) plus an
  `X-Crisp-Tier` header (`website` or `plugin`). There is **no
  authorization-code redirect flow** a shared Helio app could have arbitrary
  customer accounts authorize.
  Sources: <https://docs.crisp.chat/guides/rest-api/authentication/website-token/>,
  <https://docs.crisp.chat/guides/rest-api/authentication/plugin-token/>.
- Crisp's "Connect"/Marketplace model is **plugin-subscription**, not OAuth: a
  workspace owner *installs* a marketplace plugin, which subscribes that
  plugin's single shared token to their website. That is a per-install grant to
  one shared token, not a per-user authorization-code grant that mints an
  isolated user token — so it fails the audit rubric's "multi-tenant
  authorization-code OAuth" test. `api_key` is correct.

The credential the user supplies is their own Crisp token keypair; it flows
through Helio's vault + token gateway like MongoDB's connection string (the
`manual_credentials` precedent) — with one narrow capability difference called
out in §3 (a keypair account-key deriver, because a Crisp keypair has no DSN
host to derive an account label from).

### Tier decision (corrects the prior draft)

The prior draft defaulted to the **plugin _development_ token**. That is the
wrong credential for an AI support teammate, and the reasons it cited do not
survive the docs:

- The plugin development token requires a **separate Marketplace account** (the
  docs: "This account is different from your main Crisp account"), creating a
  **Private plugin**, then generating a token — and it "has a **low daily
  request quota**, which you can still reset on your Marketplace dashboard
  on-demand." A hand-reset low daily quota is unfit for a teammate that bursts
  `inbox triage → read thread → read messages → reply` on every interaction.
- The draft's sole justification for plugin tier was that `--website`
  auto-resolves via `plugin/connect/websites/all`. But genuine multi-website
  reach only exists on a **production / marketplace-published** plugin token;
  the self-serve development token is bound to the developer's own
  workspace(s), so auto-resolve buys nothing a single-workspace teammate needs.

**v1 defaults to the website tier** (`X-Crisp-Tier: website`). Per the official
website-token doc it is: self-serve by the workspace **owner** (Settings →
Workspace Settings → Advanced configuration → API Token → **Generate Token**),
**"No Marketplace account required,"** a **fixed 10,000 requests/day** quota, and
scoped to exactly the one workspace an AI teammate operates in. Same keypair
shape (`identifier` + `key`, HTTP Basic), same single stored secret; the only
per-request difference from plugin tier is the constant header value
`X-Crisp-Tier: website`. Plugin tier (and its `connect/*` website enumeration)
is a documented **future** option, not v1 — see §3.

## 1. What an AI teammate does with Crisp → API surface

Crisp is a customer-messaging/support platform (live-chat widget, shared inbox,
CRM "People"). A Helio teammate sitting in a support workspace does inbox and
contact work, so the tool wraps the **Website Conversations**, **Messages**,
**People**, and **Operators** resources of the Crisp REST API v1 (base URL
`https://api.crisp.chat/v1/`). Everything is scoped to a `website_id` (the
workspace) — all v1 routes live under `/v1/website/{website_id}/…`, which a
website-tier token can reach. Confirmed endpoint templates (docs REST
reference):

| Teammate intent | Method + path |
|---|---|
| Triage the inbox | `GET /v1/website/{website_id}/conversations/{page_number}` |
| Read one thread | `GET /v1/website/{website_id}/conversation/{session_id}` |
| Read a thread's messages | `GET /v1/website/{website_id}/conversation/{session_id}/messages` |
| Reply to a customer | `POST /v1/website/{website_id}/conversation/{session_id}/message` |
| Resolve / change state | `PATCH /v1/website/{website_id}/conversation/{session_id}/state` |
| Assign / route | `PATCH /v1/website/{website_id}/conversation/{session_id}/routing` |
| Resolve operator email → id | `GET /v1/website/{website_id}/operators/list` |
| Look up a contact | `GET /v1/website/{website_id}/people/profile/{people_id}` |
| List/search contacts | `GET /v1/website/{website_id}/people/profiles/{page_number}` |
| Add a contact | `POST /v1/website/{website_id}/people/profile` |

The People-profile paths (page suffix, query params for search/filter such as
`search_text`, `filter_*`) and the `operators/list` response shape are
re-verified at L2 against the live API before the anycli pin is cut — the
reference-doc render truncates their parameter tables. Conversations / messages
/ state / routing (rows 1–6) are literally confirmed against the REST reference.

**Website enumeration is deliberately out of v1.** `GET
/v1/plugin/connect/websites/all/{page_number}` (note: singular `/plugin/`, with
a trailing `{page_number}` segment) is a **plugin-tier** route — a website-tier
token cannot call it. Because v1 is website-tier, there is no `crisp website
list` command and no `--website` auto-resolve; `website_id` is supplied by the
operator (see §2). If plugin-tier support lands later, `crisp website list` +
auto-resolve are added then, against that exact page-suffixed path.

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
website in the path. Only the keypair is a secret; tier is a fixed constant and
website is a routing parameter.

- **Credential field (resolver-supplied):** `access_token`, carrying the
  colon-joined keypair string **`identifier:key`** — this is exactly Crisp's own
  documented `curl --user "{identifier}:{key}"` shape. Crisp identifiers/keys
  are colon-free (UUID/hex), so the service splits on the first `:`
  unambiguously. Injected as env `CRISP_TOKEN`.
- The service builds `Authorization: Basic base64(identifier:key)` itself (do
  not pre-base64 in storage — store the human-pasteable `identifier:key`).
- **Tier is fixed to `website`** — sent as a constant `X-Crisp-Tier: website`
  header, not a credential. Rationale in §0/§3. (No env/field for it.)
- **`website_id` is a routing parameter, not a credential** — a global
  `--website <id>` flag, mirroring `mongodb`'s per-invocation `--db/--collection`.
  A website-tier token cannot enumerate websites, so there is **no auto-resolve
  fallback**: `--website` is **required** on every verb (the service exits 2 with
  a clear usage error when it is missing). This is fine for a single-workspace
  teammate — the `website_id` is stable and the operator already knows it (it is
  visible in the Crisp dashboard URL and settings); the AI-facing doc instructs
  the assistant to carry it across a session once given.

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
(API failure via typed `apiError`) / 2 (usage, incl. missing `--website`).
Global persistent required `--website`.

```
crisp conversation list    [--page N] [--filter-status ...] --website <id> --json
crisp conversation get      --session <id> --website <id> --json
crisp conversation messages --session <id> [--page N] --website <id> --json
crisp conversation reply    --session <id> --text <msg> [--from operator] --website <id> --json
crisp conversation state    --session <id> --state resolved|pending|unresolved --website <id> --json
crisp conversation route    --session <id> --operator <id|email> --website <id> --json
crisp people list           [--page N] [--search <q>] --website <id> --json
crisp people get            --people <id> --website <id> --json
crisp people create         --email <e> [--nickname <n>] --website <id> --json
```

**`conversation route` operator resolution.** Crisp's routing PATCH body is
`{"assigned": {"user_id": "<operator UUID>"}}` (confirmed against the REST
reference — `assigned.user_id` is the required "Operator user identifier"). So
`--operator` accepts **either** a raw operator `user_id` **or** an email: when
the value contains `@`, the service first calls `GET
/v1/website/{website_id}/operators/list`, matches the email to its `user_id`,
and fails with a clear "no operator with email X in this website" error (exit 1)
if unmatched. This one extra lookup is the only multi-call verb; documented here
so it is not smuggled in silently.

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
  `Authorization: Basic <base64("abc:def")>` **and** `X-Crisp-Tier: website`.
- `--website` omitted → exit 2 usage error (no network call attempted).
- `conversation route --operator user@example.com` → service calls
  `operators/list`, resolves to the matching `user_id`, PATCHes
  `{"assigned":{"user_id":...}}`; unknown email → exit 1 with a clear message;
  `--operator <uuid>` skips the lookup and PATCHes directly.
- Each verb: request method/path/body shape + injected headers.
- Error rendering: Crisp `{error:true, reason:"...", data:...}` and a raw HTTP
  4xx/5xx both render the `{"error":...}` envelope in plain and `--json` modes;
  `invalid_session` / 401 (bad/mis-encoded token) maps to a clear "reconnect"
  message.
- Never hit the real API from a unit test.

## 3. Credential model, auth flow & token semantics (verified vs. official docs)

**Token registration model.** Crisp issues token keypairs (`identifier` +
`key`) in two tiers:

- **Website token** (v1 default, tier `website`): generated **self-serve by a
  workspace owner** in Settings → Workspace Settings → Advanced configuration →
  API Token → **Generate Token**. **No Marketplace account required.** Fixed
  **10,000 requests/day**, scoped to that single workspace. Credentials are
  shown once at generation. Owner-only management (only owners can
  generate/regenerate/revoke).
- **Plugin token** (deferred, tier `plugin`): created in the Crisp
  **Marketplace** (a separate account from the main Crisp account) by creating a
  **Private plugin**. A **development** token has a **low daily quota, manually
  resettable** on the Marketplace dashboard; a **production** (published) token
  is what actually spans multiple subscribed websites and can call the
  `plugin/connect/*` enumeration routes.

**v1 fixes tier = `website`** and connects with an **owner-generated website
token**, because it is (a) self-serve with **no Marketplace / private-plugin /
Add-Trusted-Workspace friction**, (b) a **fixed 10,000/day** quota rather than a
low, hand-reset development quota — ample for a support teammate's triage/read/
reply bursts, and (c) scoped to exactly the one workspace an AI teammate
operates in. Plugin-tier support (and its `connect/*` multi-website enumeration
+ `crisp website list`) is a documented future option; it would only carry the
different constant `X-Crisp-Tier: plugin` header and add the enumeration verb —
still one stored secret — so nothing about the storage face changes, and it is
intentionally deferred, not smuggled in.

**Account label / identity derivation (explicit — required capability growth).**
A Crisp keypair is `identifier:key` with **no host** and an **opaque UUID**
`identifier`, so mongodb's `dsnHostIdentityDeriver` (which `url.Parse`s the
secret and demands a host) would **reject every Crisp Connect** with "requires a
connection string with a host." The prior draft's "zero capability growth, same
as mongodb" claim was therefore wrong. Crisp needs a **narrow, reviewed keypair
identity deriver** for the `manual_credentials` strategy:

- Split the pasted secret on the first `:` into `identifier` + `key`; reject
  (local `manualCredentialFormatError`, a 4xx user-input error, never echoing
  the secret) if either half is empty.
- Return **`account_key = identifier`** (the non-secret UUID half — stable and
  dedup-safe) and **account label = `Crisp · <identifier>`**. This is an opaque
  UUID rather than a human-meaningful host like mongodb's — accepted for v1; it
  is stable and readable, and is **never** a hash of the secret. The `key`
  half and the full secret never enter the returned identity map, so Connection
  metadata stays secret-free.

Selecting this deriver is one added branch/enum in the `manual_credentials`
registration (§4) — the same shape as the existing `dsnHostIdentityDeriver`
choice, not a new runtime strategy.

**Connect-time verification: out of scope (OQ1 no-verify).** Crisp *does* expose
`HEAD /v1/plugin/connect/session/valid` and `GET /v1/plugin/connect/account`,
but both are **plugin-tier** routes (`/v1/plugin/…`) — a website-tier token
cannot call them, and there is no website-tier tokenless self-identification
endpoint (every website route needs a `website_id` in the path, which is not
known at connect time). So, exactly like mongodb, `manual_credentials` performs
**no provider-side verification at Connect**: a bad keypair is stored as-is and
surfaces at first use as AnyCLI `CredentialRejected` (401 → "reconnect").

**Token semantics.** Keypairs are **long-lived, non-expiring, non-refreshing**
(there is no refresh cycle) — the token-gateway serves the stored secret
directly, like a Slack bot token. Revocation is user-side (regenerate/revoke in
Crisp settings, owner-only); Helio disconnect is `local_only`.

**Scopes.** A website token is unscoped within its single workspace (full
website-tier route access). Route-scope declarations only exist for production
plugin tokens — irrelevant to the v1 `api_key` connect path.

**Single-secret storage constraint (design 317 D5).** integration-service
stores a manual credential as **exactly one secret in the user-token payload**
(`validateCredentialInputSchema`: the connect form must be exactly one required
field; `knownCredentialSources` = `token.access_token`,
`connection.account_key`, `connection.metadata.person_urn`). Crisp fits the
**single-secret** face: the whole secret is the one `identifier:key` string;
tier is a constant and `website_id` is a routing arg, so neither needs storage.
This is the same storage envelope as `mongodb` — no new CredentialSource, no
token-gateway change. The **one** difference from mongodb is the keypair account
deriver above (mongodb reuses the DSN host deriver; Crisp cannot).

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
    setup_url: https://docs.crisp.chat/guides/rest-api/authentication/website-token/

identity:
  source: strategy                   # keypair deriver (§3), not the DSN host deriver

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

**Narrow integration-service capability growth (the one Go change).** The
`manual_credentials` strategy currently hardwires `dsnHostIdentityDeriver` in
`composeProviderRegistration` (`provider_registry.go`). That deriver rejects a
hostless secret, so Crisp needs a **keypair identity deriver** selected for
Crisp (§3): split `identifier:key`, return `account_key = identifier` and label
`Crisp · <identifier>`, no provider-side HTTP call (OQ1 no-verify preserved).
This is a small, reviewed addition analogous to the existing DSN deriver — not a
new runtime strategy, no new CredentialSource, no token-gateway change, and no
`required_config_fields` (that strategy uses no server config fields). **No
`config/` or `deploy/` secret** — the user's keypair is the only credential and
it enters via `POST /connections/credentials` into Vault; there is no Helio-side
client id/secret to land (unlike OAuth providers), so lane-1 config is a no-op
for Crisp.

**Generation:** from `go-services/integration-service`, `go run
./cmd/provider-gen` then `--check`; commit all five projections together with the
bundle at batch-end.

**UI icon (manual, never generated):** `ui/helio-app/src/integrations/icons/crisp.svg`
+ register in `providerIcons.ts`.

**AI-facing docs:** add a `crisp` sub-doc under
`agents/plugins/heliox/skills/tool/` covering the verbs, the **required
`--website`** flag (how to find the `website_id` in the Crisp dashboard, and to
carry it across the session), the `--operator` email→id resolution behavior, and
that the credential is the **website token** `identifier:key` keypair generated
by the workspace owner under Settings → Workspace Settings → Advanced; bump +
publish the plugin at batch-end.

## 5. Test plan → five layers

| Layer | Crisp specifics | External creds? |
|---|---|---|
| **L1** anycli unit | httptest fakes per §2: Basic+`X-Crisp-Tier: website` header assembly, missing-`--website` exit-2, `--operator` email→id resolution (and uuid passthrough), per-verb request shape, Crisp/HTTP error → `{"error"}` envelope, 401/`invalid_session` handling. `go test ./...` green. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN="identifier:key" anycli crisp -- conversation list --website <id>`, then `conversation get/messages/reply`, `conversation route --operator <email>`, `people get` against a **real Crisp workspace + owner-generated website token**. Confirms field name/injection and the still-unverified People-path/`operators/list` shapes. | **Yes** — real Crisp workspace + website token (account pool) |
| **L3** generate + suites | `provider-gen --check` clean (incl. the keypair-deriver capability); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service suite incl. a keypair-deriver unit test (`identifier:key` → `account_key=identifier`; empty half → 4xx format error). (On-branch: local `replace` → anycli branch + local regen, uncommitted.) | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"crisp"`, `access_token:"identifier:key"` (seedable — user-token/api_key provider), then `heliox tool crisp -- conversation list --website <id>` reaches the live API through the real token gateway. **Seed `access_token` only** — no refresh cycle to exercise. | **Yes** — same real keypair as L2 |
| **L5** connect flow (pre-flip) | api_key key-entry path (master plan §2): open the connect link → paste the `identifier:key` keypair via the real connect UI (`POST /connections/credentials`) → connection shows connected/`configured` with account label `Crisp · <identifier>` in `GET /connections` → one **unseeded** live `heliox tool crisp -- conversation list --website <id>` succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. Run once, still hidden, before flipping `visible: true`. | **Yes** — real keypair through the connect UI |

L1/L3 need no external creds (L3 is local build/test only); **L2, L4, L5 all
consume the same externally supplied Crisp _website_ token keypair + a real
workspace** (human lane 2 account pool). No OAuth app registration (lane 1) is
required — Crisp is `api_key`.

## 6. Rollout

Ship hidden (`visible: false`) with the batch; land anycli tool + pin, the
keypair-deriver capability + bundle + five projections, resolver (no entry
needed), icon, docs at batch-end; run L1–L4 on-branch and the L5 key-entry sweep
post-merge; then flip `visible: true` + regenerate as the single go-live change.
