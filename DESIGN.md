# Gorgias — per-tool design (`heliox tool gorgias`)

Scratch design for the Gorgias external tool provider, per the
`helio-tool-provider` pipeline and the 298-integrations master plan
(catalog row 74). This file is committed on branch `tool/gorgias` and is
stripped by the batch lead at batch-end.

- **anycli id (axis ②):** `gorgias`
- **provider catalog key (axis ③):** `gorgias`
- **CLI command word (axis ①):** `gorgias` (flat, ungrouped)
- **auth lane:** `oauth_review` (catalog row 74; OAuth audit row 76, high confidence)
- **wave / batch:** Wave 2, Support category
- **tool type:** `service` (anycli built-in HTTP service; no official CLI)

All three axes are identical (`gorgias`), so **no `toolToProvider`
divergence entry** is needed in `helio-cli/internal/toolcred/resolver.go`.

Gorgias is a helpdesk / customer-support platform (tickets, customer
conversations, macros, satisfaction surveys). It is the near-twin of the
already-designed **Zendesk** tool (`tool/zendesk` branch): same product
category, same `oauth_review` lane, and — the load-bearing similarity — a
**per-subdomain OAuth and API surface**. This design deliberately mirrors the
Zendesk bundle shape and depends on the same shared **instance-scoped OAuth
capability (Option A)**.

---

## 1. Official API surface this tool wraps, and why

### 1.1 What an AI teammate does with Gorgias

Gorgias is where a support team lives. An AI teammate connected to a
customer's Gorgias helpdesk is asked to do the things a support agent or ops
lead does from the ticket queue:

- **Triage and read the queue** — "What tickets are open / unassigned right
  now?", "Show me ticket 12345 and its whole conversation."
- **Answer / update a ticket** — draft or post a reply message, change
  status (open/closed), assign, add tags.
- **Look up a customer** — "Find the customer with email x@y.com and their
  recent tickets" (support context before replying).
- **Report** — "How many tickets did we close this week?", satisfaction
  survey results.

That maps to a small, high-value slice of the REST API. We wrap the
resources that back those verbs and skip the admin-plane resources (Rules,
Widgets, Voice-Call provisioning, Jobs, Integrations config) that an AI
teammate has no natural reason to drive.

### 1.2 Endpoints wrapped (official REST API)

Base URL is **per-account**: `https://{subdomain}.gorgias.com/api/`. Every
Gorgias account is a helpdesk instance keyed by its subdomain
(`acme.gorgias.com`), and *all* REST + OAuth traffic goes to that host —
this is the single most important fact about the integration (see §3).
The API is REST/JSON, cursor-paginated (`?cursor=`, `?limit=`), and accepts
an OAuth2 Bearer access token in the `Authorization` header for public apps.

Verified against the official reference (developers.gorgias.com/reference):

| Resource | Endpoint(s) wrapped | Verb it backs |
|---|---|---|
| Tickets | `GET /api/tickets`, `GET /api/tickets/{id}`, `POST /api/tickets`, `PUT /api/tickets/{id}` | list / retrieve / create / update (status, assignee, tags) |
| Ticket Messages | `GET /api/tickets/{id}/messages`, `POST /api/tickets/{id}/messages` | read the conversation / post a reply |
| Customers | `GET /api/customers`, `GET /api/customers/{id}`, `POST /api/customers/search` (or `GET /api/customers?email=`) | look up a customer |
| Users (agents) | `GET /api/users`, `GET /api/users/{id}` | resolve/assign agents |
| Tags | `GET /api/tags` | tag lookup for triage |
| Views | `GET /api/views`, `GET /api/views/{id}/items` | the saved queues agents work from |
| Satisfaction Surveys | `GET /api/satisfaction-surveys` | CSAT reporting |
| Account | `GET /api/account` | identity / health-check |

**Write contract (verified — `POST /tickets`, `POST /tickets/{id}/messages`).**
Per the official create docs (developers.gorgias.com/docs/create-a-ticket-using-api,
/docs/create-a-new-message-in-ticket-via-api), a ticket message **always**
requires `channel` **and** `via`; a ticket create additionally repeats
`channel` + `via` + `from_agent` at the ticket level. Valid `channel` values are
`api | email | phone | sms | internal-note`; valid `via` values are
`api | email | internal-note`. The `email`/`phone`/`sms` channels also require a
`source` object routing addresses (`source.from.address` + `source.to[].address`);
for email the from-address must be an email integration already connected to
Gorgias. The tool therefore **defaults `--channel api`** (delivers into the
ticket with no routing setup), always emits `via` (derived from the channel or
set with `--via`), and exposes `--source-from` / `--source-to` for the routed
channels. Earlier drafts defaulted `--channel email` and omitted `via`, which the
live API rejects — corrected during code review.

**Pagination limit.** The official pagination reference documents only a
**default of 30** for `?limit=` and specifies **no maximum**; the tool sends
`limit` only when the caller sets it and its help text asserts no unverified
upper bound.

**Why these and not the whole API:** the master plan's built-in-service
conventions favor a provider-neutral, agent-shaped slice over 1:1 API
mirroring. Tickets + Ticket Messages + Customers + Users cover the read/reply/
triage loop that is 90% of what a support AI does; Tags/Views/Satisfaction
round out triage and reporting; Account is the identity anchor. Admin/config
resources are intentionally out of scope for v1 and can be added later
behind the same bundle without a new provider.

---

## 2. anycli definition

### 2.1 Type decision (stage-1 rubric)

**`service` type.** The rubric picks `cli` only when an official,
non-interactive, `--json`-capable binary exists that can be provisioned into
the runtime image. Gorgias ships no official CLI — only the REST API — so
this is a service-type tool implemented in
`internal/tools/gorgias/` against the HTTP API, exactly like the 21 existing
service definitions (`notion` is the reference shape; `zendesk` on
`tool/zendesk` is the direct subdomain-scoped sibling).

### 2.2 Go package + registration

- Definition file: `definitions/tools/gorgias.json` (name `gorgias`).
- Service package: `internal/tools/gorgias/` (package `gorgias` — id has no
  dashes, so no normalization needed).
- Registered in `internal/tools/register.go`:
  `RegisterService("gorgias", &gorgias.Service{})`.

### 2.3 Command tree (cobra, resource-grouped like notion)

```
heliox tool gorgias -- ticket list      [--status open|closed] [--assignee <id>] [--view <id>] [--cursor <c>] [--limit N]
heliox tool gorgias -- ticket get       <ticket-id>
heliox tool gorgias -- ticket create    --customer-email <e> --subject <s> --body <b> [--channel api] [--via ...] [--source-from <a>] [--source-to <a>]
heliox tool gorgias -- ticket update    <ticket-id> [--status ...] [--assignee <id>] [--add-tag <t>]
heliox tool gorgias -- message list     <ticket-id> [--cursor <c>]
heliox tool gorgias -- message create   <ticket-id> --body <b> [--channel api] [--via ...] [--from-agent] [--sender-email <e>] [--source-from <a>] [--source-to <a>]
heliox tool gorgias -- customer list    [--email <e>] [--cursor <c>]
heliox tool gorgias -- customer get     <customer-id>
heliox tool gorgias -- customer search  --query <q>
heliox tool gorgias -- user list        [--cursor <c>]
heliox tool gorgias -- tag list
heliox tool gorgias -- view list
heliox tool gorgias -- view items       <view-id> [--cursor <c>]
heliox tool gorgias -- satisfaction list [--cursor <c>]
heliox tool gorgias -- account get
```

### 2.4 JSON output shape

Follow the notion/zendesk service contract exactly:

- Default: human-readable text; `--json` on every subcommand emits a
  structured envelope.
- Success `--json`: the provider-neutral projection of the resource (list
  responses carry `{ "data": [...], "cursor": "<next>", "has_next": bool }`
  reflecting Gorgias cursor pagination; single-resource returns the object).
- **Exit codes:** `0` success, `1` runtime/API failure (typed `apiError`
  wrapping the Gorgias error body + HTTP status), `2` usage/parse error —
  the documented notion contract. `--json` errors render the structured
  error envelope, never a bare stack.
- Struct carries `BaseURL`/`HC`/`Out`/`Err` so unit tests point at an
  `httptest.Server` and capture stdout/stderr (L1).

### 2.5 Credential injection (the definition's `auth` block)

Gorgias is subdomain-scoped, so the service needs **two** injected values —
the OAuth access token *and* the account subdomain (to build the base URL).
This is the identical shape to the Zendesk definition:

```json
{
  "name": "gorgias",
  "type": "service",
  "description": "Gorgias helpdesk as a tool (OAuth token, per-account subdomain)",
  "auth": {
    "credentials": [
      { "source": { "field": "access_token" },
        "inject": { "type": "env", "env_var": "GORGIAS_ACCESS_TOKEN" } },
      { "source": { "field": "subdomain" },
        "inject": { "type": "env", "env_var": "GORGIAS_SUBDOMAIN" } }
    ]
  }
}
```

The service builds `https://$GORGIAS_SUBDOMAIN.gorgias.com/api/...` and sets
`Authorization: Bearer $GORGIAS_ACCESS_TOKEN`. anycli knows nothing about
OAuth or subdomains — it only injects the two resolver-supplied fields, and
the Helio side (§3) is what captures the subdomain at connect time and
projects it into the `subdomain` credential field.

---

## 3. Credential fields & the exact auth flow (oauth_review lane)

### 3.1 Verifying the lane against official docs

The catalog and audit both classify Gorgias `oauth_review`. Confirmed
against
`developers.gorgias.com/docs/oauth2-authentication-for-creating-apps-with-gorgias`
and `.../reference/authentication`:

- Gorgias has two auth models: **private app → API key** (Basic auth,
  email + API key, single-account) and **public app → OAuth2**, and
  *"Using OAuth2 is mandatory for public apps."* A multi-tenant Helio
  integration is a public app, so OAuth2 is the only correct path.
- The **review gate**: a public app is created on the Gorgias partner
  account, then submitted for review; approved apps surface in each
  helpdesk's in-product App Store with a "connect app" button. Drafts are
  testable only against a sandbox subdomain before approval. That in-product
  App Store approval **is** the `oauth_review` gate — it decouples cleanly
  from dev via hidden-first (review clearance gates only the visible flip,
  never dev/L4/merge, per master plan §2 lane 1).
- **Divergence check:** none. Official docs agree with the audit verdict
  (row 76). No change to the catalog's auth lane is warranted.

### 3.2 The OAuth2 flow (verified, per-subdomain)

- **Authorize:** `GET https://{subdomain}.gorgias.com/oauth/authorize`
  with `response_type=code`, `client_id`, `redirect_uri`, `scope`, `state`
  (CSRF), optional `nonce`.
- **Token exchange:** `POST https://{subdomain}.gorgias.com/oauth/token`,
  `Content-Type: application/x-www-form-urlencoded`, **HTTP Basic client
  auth** (`client_id:client_secret`), body `grant_type=authorization_code`,
  `code`, `redirect_uri`. → `standard_oauth` `token_exchange_style:
  form_basic`.
- **No PKCE** documented (client-secret + Basic auth is the client
  authentication). Bundle sets `pkce: none`.
- **Token semantics:** `token_type: Bearer`; access token **expires in 24h**
  (`expires_in: 86400`); response also returns `refresh_token`, `id_token`,
  `scope`. The **refresh token does not expire and is non-rotating** — refresh
  is `POST /oauth/token` with `grant_type=refresh_token` + `refresh_token`.
  (This is the one material difference from Zendesk, whose post-2026-04
  clients issue 30-min *rotating* refresh tokens. Gorgias needs no rotation
  write-back beyond storing the new access token + expiry.)
- If the app is deactivated on the helpdesk, the whole authorize flow must be
  repeated (normal disconnect/reconnect).

### 3.3 Scopes (verified against `/docs/oauth2-scopes`)

Scope names are `resource:action`, **not** `read:all`. We request the
minimal set for the read/reply/triage loop:

```
openid email profile offline
tickets:read tickets:write
customers:read
users:read
tags:read
satisfaction_survey:read
account:read
```

- `offline` is **required** to receive a refresh token.
- `openid email profile` back the identity/label extraction (§3.4).
- Avoid the deprecated `write:all` (docs mark it temporary / slated for
  removal). If ticket-tagging needs write on tags, add `tags:write` at
  implementation time rather than reaching for `write:all`.

### 3.4 Identity / stable account key

Gorgias user ids are per-instance, so — exactly like Zendesk — the stable
account key is composed as `{subdomain}:{user.id}` by the instance-scoped
identity resolver. Identity source is `GET /api/account` (or the OIDC
`id_token`); `label_candidates` fall back through account/user name → email.

### 3.5 Credential fields projected to anycli

| anycli field | source | anycli env var |
|---|---|---|
| `access_token` | `token.access_token` | `GORGIAS_ACCESS_TOKEN` |
| `subdomain` | `connection.metadata.subdomain` | `GORGIAS_SUBDOMAIN` |
| `account_key` | `connection.account_key` | (not injected; gateway identity) |

---

## 4. Helio provider bundle plan

Single bundle at `integrations/providers/gorgias/provider.yaml`, hidden-first
(`presentation.visible: false`). Mirrors the Zendesk bundle. Intended final
shape:

```yaml
schema: helio.provider/v1
key: gorgias
go_name: Gorgias

presentation:
  name: Gorgias
  description_key: gorgias
  consent_domain: gorgias.com
  visible: false            # oauth_review: flip gated on App Store approval + config landing

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    # {subdomain} substituted by the instance-scoped OAuth capability (§4.3);
    # .gorgias.com host-suffix allowlist is the SSRF guard.
    authorize_url: https://{subdomain}.gorgias.com/oauth/authorize
    token_url: https://{subdomain}.gorgias.com/oauth/token
    token_exchange_style: form_basic
    pkce: none
    scopes: [openid, email, profile, offline,
             "tickets:read", "tickets:write", "customers:read",
             "users:read", "tags:read", "satisfaction_survey:read",
             "account:read"]
    single_active_token: false
    refresh_lease: credential   # serialize refresh per credential; write back new access token + expiry

identity:
  source: userinfo
  url: https://{subdomain}.gorgias.com/api/account
  stable_key: /id                 # composed {subdomain}:{id} by the instance resolver
  label_candidates: [/name, /account, /id]

connection:
  mode: isolated
  disconnect_mode: local_only     # no standard RFC 7009 revoke; ship local_only first
  runtime_strategy: standard_oauth

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
    subdomain: connection.metadata.subdomain

tool:
  name: gorgias
  kind: oauth
```

### 4.1 Three-axis naming (master plan §3)

- Axis ① CLI word `gorgias`, axis ② anycli id `gorgias`, axis ③ key
  `gorgias` — all identical. **No `toolToProvider` entry, no grouped
  family.** Flat command `heliox tool gorgias`.

### 4.2 `standard_oauth` vs adapter

`standard_oauth` with `token_exchange_style: form_basic`, `pkce: none`,
declarative userinfo identity — **no compiled `service/adapter_*.go`**. The
only thing outside today's golden path is the per-subdomain URL templating,
which is the shared capability below (not a Gorgias-specific adapter).

### 4.3 Shared dependency — instance-scoped OAuth capability (Option A)

Gorgias' authorize/token/identity URLs are per-subdomain. As of this design
that generic capability is **not yet on `main`** (grep of
`go-services/integration-service` finds no `instance`/`subdomain` handling).
It is the *same* capability the Zendesk bundle documents as BLOCKED-ON and
that Salesforce's per-org `instance_url` needs — built **once, generically,
and shared**. It comprises:

1. A reviewed `auth.oauth.instance` block: subdomain input field + a
   host-suffix allowlist (`.gorgias.com`) as the SSRF guard.
2. Connect-time capture of the subdomain onto the connection
   (`connection.metadata.subdomain`).
3. `{subdomain}` templating in the `standardOAuthExchanger` and the
   `declarativeIdentityResolver`.
4. A new reviewed `connection.metadata.subdomain` credential source.
5. Composed stable key `{subdomain}:{id}`.

Until it lands, `provider-gen --check` will **not** pass for this bundle —
which is the expected on-branch state (bundles ride the batch-end merge, and
CI red-until-batch-end is normal per master plan §2). The Gorgias bundle
consumes this capability; it does not fork its own. If Zendesk's batch lands
the capability first, Gorgias needs zero integration-service code.

**SSRF allowlist is NOT pinned by the committed bundle (visible-flip gate).**
The `.gorgias.com` host-suffix allowlist currently exists only as a *comment* in
`provider.yaml` — there is no machine-readable `auth.oauth.instance` block yet,
because the decoder that would strict-parse it is part of the unlanded shared
capability (adding the block now would break `provider-gen` strict decode ahead
of the capability). So nothing in the committed bundle actually enforces the
allowlist: enforcement depends entirely on item 1 above once it lands. This is a
hard precondition for going visible. At batch integration, confirm the shared
capability derives/enforces `.gorgias.com` (from `consent_domain` or an explicit
instance block) and that the subdomain input + URL templating are validated
against it — do **not** flip `presentation.visible: true` until that is verified
end-to-end (the L5 connect run below). The bundle ships `visible: false` until
then.

### 4.4 Config (lane 1, Config Sync hard rule)

`oauth.client_id` / `oauth.client_secret` land together as per-provider
appends in **both** `config/` (local) and the `deploy/` Helm Secret —
partial config fails integration-service startup; all-absent renders
`configured: false` and is safe to ship hidden. Landing must precede this
provider's L5 run.

### 4.5 UI icon + docs

- `ui/helio-app/src/integrations/icons/gorgias.svg` + hand-registered in
  `providerIcons.ts` (never generated).
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/gorgias/`, plus
  the batch-end plugin version bump + marketplace publish.

---

## 5. Test plan — five layers

| Layer | What it proves for Gorgias | External creds needed |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/gorgias/` request shape (base URL built from `GORGIAS_SUBDOMAIN`, `Authorization: Bearer` header), cursor pagination parsing, `--json` success + typed `apiError`/exit-code rendering — all against an `httptest.Server` fake. Definition JSON injects both env vars. | **No** — httptest only |
| **L2** dev harness vs real API | `ANYCLI_CRED_ACCESS_TOKEN=… ANYCLI_CRED_SUBDOMAIN=acme anycli gorgias -- ticket list --json` returns real tickets from a live Gorgias account. Proves field names, injection, and request shape match the live API. Mandatory before pin bump. | **Yes** — a real Gorgias account subdomain + a valid token (OAuth access token, or an API key issued from a test helpdesk for L2 shape-checking) |
| **L3** `provider-gen --check` + both suites | Bundle strict-decodes; five projections regenerate; helio-cli + integration-service unit suites green. **Expected to fail `--check` on-branch** until the instance-scoped capability (§4.3) + batch-end regen land — validated locally via `provider-gen` against the branch bundle, not committed. | No |
| **L4** singleton + seeded credential | `POST /internal/test-only/connections/seed` a `gorgias` connection (`access_token` + short `expires_at` + `metadata.subdomain`), then `heliox tool gorgias -- account get` reaches the live API through the real token gateway; short expiry forces the refresh-and-write-back path. Requires the §4.3 capability so the seeded subdomain templates the base URL. | **Yes** — real access + refresh token from a test account (seed exercises refresh); real seeded org/assistant identity in local Mongo |
| **L5** full connect flow (once, pre-flip) | `heliox tool gorgias auth` → connect link → real OAuth consent on the Gorgias sandbox/dev app (subdomain entered/captured) → `oauth_connected` event → unseeded live run. Human-in-the-loop (oauth L5). | **Yes** — Gorgias dev/sandbox app (client id/secret in config) + a real account to consent on |

**Externally-supplied-credential layers:** L2, L4, L5 (and L4/L5 additionally
depend on lane-1 dev-app creation and the §4.3 shared capability). L1 and L3
are fully self-contained.

### 5.1 Rollout ordering (recap)

Hidden-first: land anycli `gorgias` (merges freely mid-batch) → batch-end pin
bump + one `provider-gen` run carrying this bundle + the shared instance
capability → L4 while hidden → L5 human consent → flip
`presentation.visible: true` + regenerate as the single go-live change, gated
additionally on Gorgias App Store review clearance (oauth_review).

---

## 6. Open items / flags for the batch lead

- **Shared capability sequencing.** Gorgias, Zendesk, Salesforce, Freshdesk,
  Freshservice, ServiceNow all need per-instance/subdomain handling. Confirm
  which branch lands the Option-A instance-scoped OAuth capability so
  Gorgias doesn't duplicate it. Gorgias adds **zero** new integration-service
  code beyond consuming it.
- **`account:read` vs `/api/account` identity.** If `GET /api/account`
  requires a scope not in the minimal set, identity extraction fails at
  connect — `account:read` is included for exactly this. Verify at L2.
- **Refresh semantics.** Non-rotating, non-expiring refresh token (unlike
  Zendesk) — `refresh_lease: credential` is still safe (serialization is
  harmless) but rotation write-back is a no-op; keep the standard
  expiry-driven refresh.
- **Scope minimization at review.** `oauth_review` scrutinizes scopes; we
  request read-heavy scopes + `tickets:write` only. Add `tags:write` /
  `customers:write` only if a concrete verb needs them, never `write:all`.
