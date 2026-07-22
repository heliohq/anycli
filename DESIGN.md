# Omnisend — per-tool design (`tool/omnisend`)

Scratch design file for the `helio-tool-provider` pipeline. Batch lead strips
this at batch-end. Catalog row 274: product **Omnisend**, anycli id
`omnisend`, provider key `omnisend`, auth lane **oauth_review**, wave **3**,
category **Marketing & Notifications**.

All three naming axes collapse to the same token — `omnisend` — so there is
**no** `toolToProvider` divergence entry (axis ② == axis ③, no grouped
family). This is the gmail/notion/slack "identity" case.

---

## 0. Audit re-verification (independent check against official docs)

The 2026-07-21 OAuth audit put Omnisend at `oauth_review` / high confidence:
"Standard authorization-code flow (RFC 6749) that any customer account can
authorize, but client registration is not self-serve: developers submit a form
and Omnisend issues OAuth credentials manually in 1-3 business days."

Verified against the official OAuth reference
(`https://api-docs.omnisend.com/reference/oauth`) — **the verdict holds**:

- Authorization-code grant (`response_type=code`, `state` for CSRF), user
  signs in + approves a consent page, callback carries `code`, token exchange
  POST swaps it. This is a genuine multi-tenant authorize flow any Omnisend
  account can grant. → an OAuth lane is correct, not `api_key`.
- Client registration is **a Google Form submission**; Omnisend replies "with
  oAuth credentials in 1-3 business days." A human-issuance gate before any
  external account can authorize → **`oauth_review`, not `oauth_light`**.
  Lane confirmed, no §6 amendment needed.

### The one real divergence to record: OAuth authorizes the *new* API line, not v3

Omnisend ships **two API generations**, and this materially drives the anycli
service target:

| | Stable/legacy line | New dated line |
|---|---|---|
| Base URL | `https://api.omnisend.com/v3` | `https://api.omnisend.com/api` |
| Auth | `X-API-KEY: <key>` (per-store key) | **OAuth Bearer** + `Omnisend-Version` header |
| Version pin | path `/v3` | header `Omnisend-Version: 2026-03-15` |
| Registration | self-serve key in store settings | form-gated OAuth client |

The audit's OAuth evidence is real, but OAuth tokens authorize against the
**new dated line** (`/api`, version header), *not* the widely-indexed v3
`X-API-KEY` surface. Omnisend's own docs say a store is "only possible to
connect … with [the] oAuth flow," so the dated line is the sanctioned
multi-tenant path — exactly what a shared Helio client needs. **Decision: the
anycli service targets the OAuth-authenticated dated line** (`/api` base,
`Authorization: Bearer`, `Omnisend-Version: 2026-03-15`). The v3 `X-API-KEY`
surface is deliberately **not** wrapped — mixing it in would fork auth shapes
and strand the token gateway. This is the divergence recorded per the "follow
official docs" instruction; the lane itself is unchanged.

---

## 1. API surface this tool wraps, and why

Driven by what an AI teammate actually does inside a marketing/CRM automation
tool: read and mutate the audience, trigger lifecycle events, run and inspect
campaigns, and segment. Endpoints (all on the dated line, scope-gated):

| anycli group | Endpoints (dated `/api`) | OAuth scope | Why an AI teammate needs it |
|---|---|---|---|
| `contact`  | `GET/POST /contacts`, `GET/PATCH /contacts/{id}`, update-by-email, batch tag add/remove | `contacts.read` / `contacts.write` | Look up a subscriber, add/update contacts, tag for segmentation — the core "who" of the audience. |
| `event`    | `POST /events` (send customer event) | `events.write` | Fire a custom event ("trial started", "renewed") that triggers an automation workflow — the human-natural "nudge the funnel" action. |
| `campaign` | `GET/POST /campaigns`, `GET/PATCH/DELETE /campaigns/{id}`, send, cancel, copy, A/B resume/stop/winner, UTM get/update | `campaigns.read` / `campaigns.write` | Draft/list/send a campaign and read its state — the primary teammate ask ("send the July newsletter", "did the promo go out?"). |
| `segment`  | `GET/POST /segments`, `GET/PATCH/DELETE /segments/{id}`, statistics | `segments.read` / `segments.write` | Build/inspect an audience slice to target a send. |
| `product`  | `GET/POST /products`, `GET/PUT/DELETE /products/{id}`; product-categories parallel | `products.read` / `products.write` | Sync catalog data that campaigns/automations reference (product picker, browse-abandon). |
| `report`   | `POST /reports` (+ statistics) | `analytics.read` | Pull campaign/automation performance for a "how did it do?" answer. |
| `batch`    | `GET/POST /batches`, get-info, get-items | `products` / `contacts` / `events` (read/write) | Bulk contact/product/order/event upserts without N API calls — efficiency lens. |
| `brand`    | `GET /brands` (brand info) | `brands.read` | Identity/account confirmation (see §4 identity). |

Kept **out of v1** to hold the tool small and agent-legible: email-template /
email-content / universal-layout CRUD, image upload, automation-block replace.
These are builder-surface operations a human does in the Omnisend UI, not
teammate asks; add later only if demand shows up (Code Health: subtract before
adding). `contact`/`event`/`campaign`/`segment` cover the 80% teammate loop.

---

## 2. anycli definition (stage-1 rubric)

**Type: `service`** (default). The `cli`-type rubric requires an official,
non-interactive, `--json`-capable binary provisionable into the runtime image;
Omnisend ships no CLI. So HTTP-against-the-API service, following
`internal/tools/notion/` as the reference shape.

- **Definition:** `definitions/tools/omnisend.json`, `name: "omnisend"`,
  `type: "service"`, one `CredentialBinding`: source `field: access_token` →
  inject `env` `OMNISEND_ACCESS_TOKEN` (Bearer). The `Omnisend-Version:
  2026-03-15` header is a **constant emitted by the service code**, not a
  credential — it never varies per account, so it does not belong in the
  definition's auth block.
- **Go package:** `internal/tools/omnisend/` (id has no dashes → package
  `omnisend`), registered `RegisterService("omnisend", &omnisend.Service{})`
  in `internal/tools/register.go` (the one shared-registry file; batch-end
  merge).
- **Shape:** copy notion's `BaseURL`/`HC`/`Out`/`Err` struct so httptest can
  point `BaseURL` at a fake and capture stdout/stderr. Cobra tree grouped by
  resource (`contact`, `event`, `campaign`, `segment`, `product`, `report`,
  `batch`, `brand`). Exit-code contract: 0 success, 1 runtime/API failure via
  typed `apiError`, 2 usage/parse.

### Subcommands / verbs (v1)

```
omnisend contact list [--email --limit --paginate]
omnisend contact get --id <id>
omnisend contact create --email <e> [--phone --first-name --tags ...]
omnisend contact update --id <id> [--tags-add --tags-remove ...]
omnisend event send --email <e> --event-id <id> [--fields '<json>']
omnisend campaign list [--status --limit]
omnisend campaign get --id <id>
omnisend campaign send --id <id>
omnisend segment list
omnisend segment get --id <id>
omnisend product list [--limit]
omnisend report generate --type <t> [--from --to]
omnisend batch get --id <id>
```

### JSON output shape

`--json` on by default for agent consumption (AGENTS.md). Success:
`{"data": <provider payload, minimally reshaped>, "paging": {"next": "<cursor|null>"}}`
where the provider returns a `paging.next` cursor. Error envelope (typed
`apiError`, exit 1):
`{"error": {"code": "<omnisend_code|http_status>", "message": "<msg>", "status": <n>}}`.
Match notion's plain-text-vs-`--json` dual rendering exactly — both are unit
tested.

---

## 3. Credential fields & exact auth flow (oauth_review)

**Flow (verified):**

1. Authorize: `GET https://app.omnisend.com/oauth2/authorize?response_type=code&client_id=…&redirect_uri=…&scope=<space-separated>&state=…`
2. Callback: `redirect_uri?code=…&state=…`
3. Token: `POST https://app.omnisend.com/oauth2/token`, `Content-Type:
   application/x-www-form-urlencoded`, body `grant_type=authorization_code`,
   `code`, `client_id`, `client_secret`, `redirect_uri`. → returns
   `access_token`, `refresh_token`, `scope`, `token_type: Bearer`,
   `expires_in`.
4. API calls: `Authorization: Bearer <access_token>` + `Omnisend-Version:
   2026-03-15` against `https://api.omnisend.com/api`.

**Client auth style → `token_exchange_style: form_secret`** — client_id +
client_secret in the form body, urlencoded. Matches the existing enum
(LinkedIn precedent). **PKCE: none** (confidential client, docs describe no
PKCE).

**Token semantics — the deliberate call:** the response includes a
`refresh_token` and an `expires_in`, but Omnisend states the access token
"will never expire (unless the user … revoke[s] it)." So treat it as a
**non-expiring bearer token**: `refresh_lease: none`, seed `access_token`
only (§5 L4). This mirrors notion/slack (non-expiring) rather than the
short-expiry refresh path — no refresh-cycle capability is exercised because
the provider has none in practice. (If L2/L5 shows tokens actually expiring,
revisit to `refresh_lease: refresh` + seed a refresh_token — an allowed-set
value that already exists, so still zero capability growth.)

**Credential fields the bundle declares:** `auth.required_config_fields:
[oauth.client_id, oauth.client_secret]` — the form-issued client credentials,
supplied per-environment via integration-service config (§4), never in the
bundle. The user's access token enters through the standard OAuth callback and
is stored in Vault; the bundle's `credential.fields` maps
`access_token: token.access_token`, `account_key: connection.account_key`.

---

## 4. Helio provider bundle plan (`integrations/providers/omnisend/`)

Golden path — `connection.runtime_strategy: standard_oauth`, **hidden-first**.
Expected to need **zero** integration-service Go (standard_oauth composes the
exchanger/identity/revoker declaratively).

```yaml
schema: helio.provider/v1
key: omnisend
go_name: Omnisend

presentation:
  name: Omnisend
  description_key: omnisend
  consent_domain: omnisend.com
  visible: false            # hidden-first; flip is the single go-live change

auth:
  type: oauth
  owner: assistant          # a teammate connects one store on the org's behalf
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.omnisend.com/oauth2/authorize
    token_url: https://app.omnisend.com/oauth2/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [contacts.read, contacts.write, campaigns.read, campaigns.write,
             events.write, segments.read, segments.write, products.read,
             analytics.read]
    single_active_token: false
    refresh_lease: none

identity:
  source: token_response    # PRIMARY — see decision below
  stable_key: /account_id   # confirm exact pointer at L2 against a real token
  label_candidates: [/store_name, /account_id]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: standard_oauth

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: omnisend
  kind: oauth
```

**Axis mapping:** ① command `omnisend` · ② anycli id `omnisend` · ③ key
`omnisend` — all identical. No `toolToProvider` entry, no `toolGroups` entry
(not a grouped family).

### Identity — the one thing to nail at stage-1 L2

`standard_oauth`'s `declarativeIdentityResolver` extracts a stable key by RFC
6901 JSON Pointer from either the token response or a `userinfo` GET.

- **PRIMARY (zero capability growth):** `identity.source: token_response` with
  a pointer into the token payload (like notion's `/workspace_id`). **Verify
  at L2** whether Omnisend's token response carries a store/account id — if it
  does, this is the clean path; fix `stable_key`/`label_candidates` to the
  real pointers observed.
- **FALLBACK if the token response has no id:** `identity.source: userinfo`,
  `url: https://api.omnisend.com/api/brands`, pointer at the brand id. **Risk
  to flag now:** that GET needs the `Omnisend-Version: 2026-03-15` header, and
  the generic `declarativeIdentityResolver` may not send a per-provider
  constant header. If the fallback is required, that header-injection gap is a
  **narrow capability-growth item** (a reviewed constant-header field on the
  userinfo resolver) — the calcom `/v2/me` header-gate decision is the
  precedent. Do not build the adapter speculatively; decide at L2.

No `service/adapter_*.go` is anticipated — Omnisend's token exchange, error
dialect (standard JSON), and disconnect are all standard-shaped.

### Config (human lane 1, batch-end / pre-L5)

`oauth.client_id` + `oauth.client_secret` land together (partial config fails
startup) in integration-service config: `config/` locally **and** the Helm
Secret under `deploy/` (Config Sync hard rule), as per-provider appends. Absent
config → `configured:false` (Connect disabled), safe to ship hidden.

### UI + docs (batch-end)

- Icon `ui/helio-app/src/integrations/icons/omnisend.svg` + hand-append to
  `providerIcons.ts`; i18n `tools.desc.omnisend` label.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/`, plugin version
  bump + marketplace publish (one publish per batch).

---

## 5. Test plan → five layers

| Layer | What runs | External creds needed |
|---|---|---|
| **L1** | anycli `go test ./...`: definition load + service unit tests against an `httptest` fake — assert request path (`/api/...`), `Authorization: Bearer` injection, the constant `Omnisend-Version` header, pagination cursor handling, and plain-vs-`--json` error rendering. Never hits the real API. | none |
| **L2** | dev harness `ANYCLI_CRED_ACCESS_TOKEN=<tok> anycli omnisend -- contact list` against the **real** dated `/api`. Proves field names, Bearer + version header, and request shape match live. **Also the stage-1 identity probe:** capture a real token response + `GET /brands` to fix `identity` pointers (token_response vs userinfo decision). | **Yes** — a real Omnisend account + an access token minted from the dev OAuth app (human lane 1 + account pool). |
| **L3** | `provider-gen` + `provider-gen --check` (5 projections regen locally on-branch, uncommitted); anycli + helio-cli + integration-service unit suites; helio-cli built with a local `go.mod replace` at the anycli branch so heliox carries the tool. | none |
| **L4** | singleton + `POST /internal/test-only/connections/seed` (provider `omnisend`, seed `access_token` only — non-expiring, omit refresh_token/expires_at) using a **real** seeded assistant/org identity, then `heliox tool omnisend -- contact list` returns real data through the token gateway. | **Yes** — dev OAuth app must exist to mint the seed token (lane 1 gates L4); real local seeded identity. |
| **L5** | once, hidden, pre-flip: `heliox tool omnisend auth` → consent on Omnisend's dev app → `oauth_connected` event → one unseeded live run. Human-in-the-loop (oauth L5). | **Yes** — human consent session on a real Omnisend account + the registered dev app + landed config. |

**Gates:** dev-app creation (form, 1-3 business days) gates L4/L5; **review
clearance gates only the visible flip** — code lands complete-but-hidden in
wave 3 regardless of review state. Definition-of-done = five layers green +
docs published + icon registered + visible flip.

## 6. Capability-growth summary

- **Expected: none.** `form_secret` + `pkce:none` + `refresh_lease:none` +
  `identity.source:token_response` is the standard_oauth golden path (notion /
  linkedin precedents).
- **Contingent (decide at L2, do not pre-build):** if identity must come from
  a `userinfo` `GET /brands` that requires the constant `Omnisend-Version`
  header, add a reviewed constant-header field to the userinfo resolver
  (narrow, calcom-precedented) — otherwise skip.

## Sources

- OAuth flow: https://api-docs.omnisend.com/reference/oauth
- API overview / v3 vs dated line + scopes: https://support.omnisend.com/en/articles/1061798-omnisend-api-overview-and-developer-resources , https://api-docs.omnisend.com/v3/reference/getting-started , https://api-docs.omnisend.com/v3/reference/contacts , https://api-docs.omnisend.com/v3/reference/batches
- Master plan row 274 + oauth-audit row 276 (this repo).
