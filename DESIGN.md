# Kustomer — per-tool design (`heliox tool kustomer`)

Scratch design for the Kustomer provider, batch-lead strips at batch end.

- **anycli id (axis ②):** `kustomer`
- **provider catalog key (axis ③):** `kustomer`
- **CLI command word (axis ①):** `kustomer` (flat; no family group)
- **auth lane:** `oauth_review` (Wave 2, Support) — verified against official docs, see §4
- **anycli tool type:** `service`
- **Branches:** anycli `tool/kustomer`, Helio `tool/kustomer`

Axes ②=③=① all equal `kustomer`, so **no `toolToProvider` divergence entry** and
**no grouped-command wiring** are needed. This is the identity case
(`ProviderFor("kustomer") == "kustomer"`).

---

## 1. What an AI teammate does with Kustomer, and the API surface it needs

Kustomer is an omnichannel customer-support CRM (customers, their conversations,
the messages inside them, agent notes, and search). An AI support teammate in a
Helio channel is asked to: *"what's open for acme.com?"*, *"summarize this
ticket"*, *"reply to the customer"*, *"leave an internal note"*, *"find the
customer for this email"*. That maps to a small, high-leverage slice of the REST
v1 surface — read + light write over five resources, plus search. We deliberately
do **not** wrap the admin/config surface (Klasses, KObjects custom-object schema,
Queues/Routing, Workflows, Access Management, Brands) — those are org-admin
operations, not teammate actions, and each is a large sub-API.

**Official API (verified):** REST, JSON:API-shaped, versioned at `/v1`.
Base URL is the **org-subdomain form** `https://{orgname}.api.kustomerapp.com/v1`
— this is what the official Getting Started doc presents as THE base URL. It
explicitly warns that omitting `orgname` and using the generic host produces a
pod-routing error (*"Auth token associated with pod prod2 but request is being
handled by prod1"*), because Kustomer runs multiple prod pods and only the
org-subdomain routes the request to the pod that minted the token. The generic
`https://api.kustomerapp.com/v1` host works **only** for orgs that happen to live
on the default pod, so we treat it as a documented-but-unreliable fallback, never
the default (see §4 capability growth and the §7 pod-routing note). Auth is
`Authorization: Bearer {token}` (a space after `Bearer` is required). Rate limit
1000 req/min/org (HTTP 429 + `Retry-After`); conversation creation additionally
capped at 120/min/customer.

Sources verified:
- Authentication & keys — https://developer.kustomer.com/kustomer-api-docs/reference/authentication (Bearer scheme, admin-created keys via Settings > Security > API Keys)
- Getting started (base URL, pod routing) — https://developer.kustomer.com/kustomer-api-docs/reference/getting-started-with-kustomer-api
- Get Current User — https://developer.kustomer.com/kustomer-api-docs/reference/getcurrentuser
- Conversation/message endpoints — https://developer.kustomer.com/kustomer-api-docs/reference/getconversationsbycustomer , .../createamessagefromconversation

### Endpoints wrapped (all under `https://{orgname}.api.kustomerapp.com/v1`)

| Verb group | Method + path | Why the teammate needs it |
|---|---|---|
| customer get | `GET /customers/{id}` | resolve a customer record |
| customer get-by-email | `GET /customers/email={email}` | "who is bob@acme.com?" (value embedded in the path segment, URL-encoded; the `externalId={value}` / `phone={value}` lookup variants share this exact form) |
| customer conversations | `GET /customers/{id}/conversations` | "what's open for this customer?" |
| customer create | `POST /customers` | create a contact when none exists |
| conversation get | `GET /conversations/{id}` | load one ticket |
| conversation list | `GET /conversations` | recent/open tickets (paginated) |
| conversation create | `POST /conversations` / `POST /customers/{id}/conversations` | open a ticket |
| conversation update | `PATCH /conversations/{id}` | change status/priority/assignee/tags |
| message list | `GET /conversations/{id}/messages` | read the thread |
| message create | `POST /conversations/{id}/messages` | reply to the customer |
| note list | `GET /conversations/{id}/notes` | read internal notes |
| note create | `POST /conversations/{id}/notes` | leave an internal note |
| search | `POST /customers/search` (customer search) | free-form "find customers where…" |

Output: Kustomer returns JSON:API envelopes (`{"data": {...}, "meta": {...},
"links": {...}}`). We pass the provider JSON through **verbatim** on stdout (same
policy as the Notion service's `emitJSON`), and surface `links.next` /
`meta.pagination` for the agent to page — no bespoke reshaping. `--json` is the
default machine shape; errors go to stderr.

---

## 2. anycli definition (`service` type)

Per the stage-1 rubric, this is `service` type: there is no official
non-interactive `--json` Kustomer CLI to wrap, credentials are a bearer token
(env-injectable), and the surface is plain REST — the default lane (21 of 23
shipped definitions are service type).

`definitions/tools/kustomer.json`:

```json
{
  "name": "kustomer",
  "type": "service",
  "description": "Kustomer as a tool (support CRM: customers, conversations, messages, notes)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "KUSTOMER_API_TOKEN"}
      },
      {
        "source": {"field": "account_key"},
        "inject": {"type": "env", "env_var": "KUSTOMER_ORG_NAME"}
      }
    ]
  }
}
```

The `access_token` field is credential-source-agnostic: the same field name
carries an OAuth access token (the primary lane, §4) or, if Helio ever pivots the
bundle, an admin-created API key — both are used identically as
`Authorization: Bearer`. This is the key architectural fact that makes the lane
choice a **Helio-bundle** decision, not an anycli one.

The `account_key` field carries the captured `orgname` (the Salesforce
`instance_url` precedent applied to Kustomer's pod routing — see §4). The tool
builds its base as `https://{KUSTOMER_ORG_NAME}.api.kustomerapp.com/v1` so every
request lands on the pod that minted the token. When `KUSTOMER_ORG_NAME` is absent
(e.g. an L2 harness run before metadata capture exists), the tool falls back to
the generic `https://api.kustomerapp.com/v1` host — usable only for
default-pod orgs, and flagged as unreliable, never the shipped default.

`internal/tools/kustomer/` (copy the Notion service shape — `notion.go` root +
`client.go`): a `Service{BaseURL, HC, Out, Err}` struct so unit tests point
`BaseURL` at an `httptest.Server`; the base URL is resolved from
`KUSTOMER_ORG_NAME` (`https://{orgname}.api.kustomerapp.com/v1`), with
`DefaultBaseURL = "https://api.kustomerapp.com/v1"` only as the org-absent
fallback; one `call(ctx, token, method, path, payload)` helper setting the Bearer header;
the documented exit-code contract (0 success, 1 runtime/API failure via a typed
`apiError` carrying HTTP status, 2 usage/parse); a `--json` structured error
envelope (`{"error":{"message":…,"kind":"usage|api","status":…}}`).

### Cobra tree (grouped by resource, like Notion)

```
kustomer
  customer   get | get-by-email | conversations | create
  conversation  get | list | create | update
  message    list | create
  note       list | create
  search     customers      # POST /customers/search
```

Runnable group commands (`newGroupCmd` pattern) so an unknown subcommand fails
(exit 2) instead of a false-success help print. Global `--json` persistent flag;
pagination flags (`--page`, `--page-size`, or the JSON:API cursor) local to the
list/search commands.

**Check (L1):** `go test ./...` green — httptest fakes assert Bearer header
injection, org-subdomain base-URL construction from `KUSTOMER_ORG_NAME` (plus the
generic-host fallback when it is unset), request path/body shape, JSON:API
passthrough on stdout, and both plain + `--json` error rendering. Registered in `internal/tools/register.go`
`init()` as `RegisterService("kustomer", &kustomer.Service{})`; Go package
`kustomer` (id has no dashes, no normalization needed).

---

## 3. Credential fields & the exact auth flow

**Credential field the tool consumes:** `access_token` (→ `KUSTOMER_API_TOKEN`).
That is the entire anycli-visible credential contract.

**How the token is acquired (OAuth 2.0, the primary lane):**

Kustomer runs a standard authorization-code OAuth provider on a **separate apps
host** (`api.apps.kustomerapp.com`), distinct from the data API host:

- Authorize: `GET https://api.apps.kustomerapp.com/oauth/authorize`
  (`response_type=code`, `client_id`, `redirect_uri`, `scope`, `state`)
- Token: `POST https://api.apps.kustomerapp.com/oauth/token`
  (`grant_type=authorization_code`, then `grant_type=refresh_token` to renew)
- **Scopes** are Kustomer role/permission strings passed in `scope` (space or
  comma joined), e.g. `org.permission.customer.read`,
  `org.permission.conversation.read`, `org.permission.conversation.create`,
  `org.permission.message.read`, `org.permission.message.create`,
  `org.permission.note.read`, `org.permission.note.create`, plus
  **`offline_access`** — required to receive a refresh token.
- **Token semantics (verified):** access token `expires_in = 86400` (24h);
  refresh token is long-lived but **expires after 2 weeks if unused**; with a
  refresh token the app can mint new access tokens indefinitely until revoked.
  Without `offline_access` the connection dies after 24h. The returned access
  token "works like a regular Kustomer API key" — same `Authorization: Bearer`
  against the org-subdomain data API `https://{orgname}.api.kustomerapp.com/v1`.

**Registration model (this is what makes it `oauth_review`):** app registration
is self-serve via API (`POST` an app definition with an admin API key holding
`org.admin.apps` / `org.permission.apps`), **but** a self-serve app is
`"visibility": "private"` — installable **only on the developer's own org**
(Kustomer namespaces the private app id with the dev's `orgId`). Distributing one
Helio-registered client so that **arbitrary customer Kustomer orgs** can authorize
requires a **public** app, which goes through the **Partner with Kustomer**
program / app-submission review. That partner/publish gate before external orgs
can authorize is exactly the `oauth_review` rubric trigger.

Verified: https://developer.kustomer.com/kustomer-apps-platform/docs/oauth-provider
(self-serve private app, own-org scoping, public/partner distinction, endpoints,
`offline_access`, 86400s / 2-week token semantics).

**Audit reconciliation:** the master-plan catalog row 75 and the 2026-07-21 OAuth
audit (row 77, `oauth_review`, confidence *medium*) both hold under official docs
— **no divergence to record on the lane itself.** The audit's medium confidence
is resolved to a firm `oauth_review` here: the partner/public-app review gate is
confirmed. Per the plan (§2 lane 1), *dev-mode app creation* (a private own-org
app — self-serve) gates L4 and unblocks code-complete-hidden immediately; only the
*public-app review clearance* gates the visible flip.

---

## 4. Helio provider bundle plan (`integrations/providers/kustomer/provider.yaml`)

`standard_oauth` runtime strategy — the flow is a textbook authorization-code +
refresh exchange with a userinfo identity GET, fully inside the generic
capability set. **No `service/adapter_*.go` is needed** (unlike Slack/Discord/X).
Shipped **hidden-first** (`visible: false`).

Three-axis naming: `key: kustomer` (= directory name = axis ③), `tool.name:
kustomer` (axis ②), no `tool.group`/`tool.command` (flat command, axis ①). All
equal → no resolver map entry.

```yaml
schema: helio.provider/v1
key: kustomer
go_name: Kustomer

presentation:
  name: Kustomer
  description_key: kustomer
  consent_domain: kustomerapp.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual         # provider consent is an agent/admin person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://api.apps.kustomerapp.com/oauth/authorize
    token_url: https://api.apps.kustomerapp.com/oauth/token
    token_exchange_style: form_secret     # confirm form vs json_basic at L2
    pkce: none
    authorize_params: {}
    scopes:
      - offline_access
      - org.permission.customer.read
      - org.permission.conversation.read
      - org.permission.conversation.create
      - org.permission.message.read
      - org.permission.message.create
      - org.permission.note.read
      - org.permission.note.create
    display_scopes: [customers, conversations, messages, notes]
    single_active_token: false
    refresh_lease: none      # server refreshes via refresh_token; token gateway A3 path

identity:
  source: userinfo
  url: https://{orgname}.api.kustomerapp.com/v1/users/current  # pod-routed; templated from captured orgname
  stable_key: /data/relationships/org/data/id     # org id — stable, org-scoped connection
  label_candidates: [/data/attributes/email, /data/attributes/name, /data/relationships/org/data/id]
  # orgname is captured at connect (§4 metadata capture) so the userinfo call
  # itself routes to the right pod; exact capture source verified at L2.

connection:
  mode: isolated
  disconnect_mode: local_only     # Kustomer exposes no documented token-revoke endpoint (confirm at L5)
  runtime_strategy: standard_oauth

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key    # captured orgname → KUSTOMER_ORG_NAME (pod routing)

tool:
  name: kustomer
  kind: oauth
```

**Config landing (Config Sync hard rule):** add a `kustomer: {client_id,
client_secret}` block under `providers:` in **both** `config/integration-service.yaml.example`
(local) and the Helm-templated Secret under `deploy/` — id and secret land
together (a partially-configured provider fails integration-service startup; a
fully-absent one renders `configured:false` / Connect-disabled, safe while
hidden). Real values injected via K8s Secret in prod; never commit a secret.

**Capability-growth check:** the **likely-required growth is the `instance_url`-style
metadata-capture capability** (the Salesforce precedent, task #168) — reused here
to capture `orgname` and route every request to the org's pod. Kustomer runs
multiple prod pods, so a hardcoded generic base fails for the real fraction of
customer orgs not on the default pod; org-subdomain capture is the **expected
path**, not a fallback. The capture stores `orgname` into `connection.account_key`
(→ `KUSTOMER_ORG_NAME`) and templates the identity/userinfo URL to
`https://{orgname}.api.kustomerapp.com/v1`. Everything else (form/json token
exchange, userinfo JSON-Pointer identity, refresh via `refresh_token`) is already
in the `standard_oauth` set. Three things to pin down at L2:
1. **Metadata-capture source (the growth item).** Confirm whether `orgname` arrives
   in the token *response* (Salesforce `instance_url` shape — the clean case, lets
   the very first userinfo call route correctly) or must be derived from a
   `/v1/users/current` org read. If it is response-borne, this reuses the task #168
   capability verbatim; if it needs an API read, the capture ordering (bootstrap the
   read on the generic host, then pin the pod) is the only delta. Either way this is
   the growth pending L2 — **not zero growth**. The generic host is kept solely as a
   documented-but-unreliable alternative.
2. `token_exchange_style` — whether Kustomer's token endpoint wants
   `form_secret` (client id/secret in the form body) or `form_basic`/`json_basic`.
   Pick the matching existing enum value; no new capability.
3. The refresh-token **2-week idle expiry** is unusual — the token gateway's
   refresh path handles a normal refresh, but a connection idle > 2 weeks will get
   a refresh rejection and must surface as reconnect-required (the standard 401
   passthrough in `resolver.go`). This is behavior to *verify*, not new code.

**Icon:** `ui/helio-app/src/integrations/icons/kustomer.svg` + register in
`ui/helio-app/src/integrations/providerIcons.ts` (manual, never generated).

**AI-facing docs:** add a `kustomer` sub-doc under
`agents/plugins/heliox/skills/tool/`, bump the plugin version + publish at batch
end.

---

## 5. Test plan (five layers)

| Layer | What it proves for Kustomer | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: httptest fakes assert Bearer injection, each verb's method/path/body, JSON:API stdout passthrough, `--json` + plain error envelopes, exit codes 0/1/2 | No |
| **L2** | `ANYCLI_CRED_ACCESS_TOKEN=<key> ANYCLI_CRED_ACCOUNT_KEY=<orgname> anycli kustomer -- customer get-by-email bob@acme.com` (and a conversation list + message create) against the **real** API. Confirms the org-subdomain base routes to the token's pod, the exact `/v1` paths (incl. the `customers/email={email}` value-in-path form), JSON:API shapes, and — critically — the exact `orgname` capture source for the bundle (token response vs `/v1/users/current` org read, §4). Also confirm `token_exchange_style`. | **Yes** — a real Kustomer org (its `orgname`) + an admin API key (api_key harness path; no OAuth app needed for L2 since the token is a plain Bearer) |
| **L3** | `provider-gen` + `provider-gen --check` (five projections regenerate clean on-branch, not committed); `helio-cli` build with a local `replace` → anycli branch; `go test ./cmd/heliox/cmds/tool/`; integration-service unit suite | No |
| **L4** | singleton + `POST /internal/test-only/connections/seed` with `provider:"kustomer"`, seeded `access_token` (+ short `expires_at` and `refresh_token` to force the gateway refresh-and-writeback path), then `heliox tool kustomer -- conversation list` reaches the live API | **Yes** — a real access token from the dev-mode (private, own-org) OAuth app; app creation is self-serve and gates L4 (plan §2 lane 1) |
| **L5** | Full `heliox tool kustomer auth` → consent on the dev/private app → `oauth_connected` event on the channel → unseeded live run. Runs once, hidden, before the visible flip. | **Yes** — dev OAuth app + a real Kustomer org consent (human-in-the-loop, oauth L5) |

**Credential-dependent layers:** L2, L4, L5 (test-account pool lane + lane-1 dev
app). L1/L3 are fully agent-runnable offline. The **visible flip** additionally
waits on public-app **review clearance** (oauth_review), which gates neither dev,
L4, nor the batch-end merge.

---

## 6. Definition of done (this tool)

L1–L5 green · docs published · icon registered · then `visible: true` +
regenerate as the single go-live change — with the public-app review clearance as
the extra gate on the flip only. Until then: **code-complete (hidden)**.

---

## 7. Divergences & risks recorded (per the "independent judgment" mandate)

1. **Lane confirmed, not diverged.** Official docs confirm `oauth_review`: a
   self-serve OAuth app is private/own-org; multi-tenant distribution requires the
   Partner with Kustomer public-app review. Catalog + audit stand.
2. **Dual credential model → lane is a bundle-only choice.** Kustomer also offers
   admin-created API keys (same Bearer). Because the anycli tool is
   credential-source-agnostic (`access_token` either way), if the public-app
   partner review stalls indefinitely, Helio can pivot the *bundle* to a manual
   `api_key` shape (`auth.type: manual`, write-only `POST /connections/credentials`,
   a `users/current` verifier) with **zero anycli change**. Recorded as the
   contingency; primary path stays `oauth_review` per the catalog.
3. **Pod routing is the designed base, not a fallback.** The official Getting
   Started doc presents `https://{orgname}.api.kustomerapp.com/v1` as THE base URL
   and warns that the generic host errors with *"Auth token associated with pod
   prod2 but request is being handled by prod1"* — Kustomer runs multiple prod
   pods, so a hardcoded generic base fails for the real fraction of customer orgs
   not on the default pod. This design therefore **captures `orgname`/pod at
   connect and builds the org-subdomain base as the expected path**, reusing the
   Salesforce `instance_url` metadata-capture capability (task #168). The generic
   `api.kustomerapp.com` host is retained only as a documented-but-unreliable
   alternative (org-absent fallback), never the default. L2 verifies the exact
   capture source (token response vs `/v1/users/current` org read) and confirms the
   subdomain routes correctly.
4. **Refresh-token 2-week idle expiry.** Longer-lived than a typical rotating
   refresh token but with an idle-death clock: a connection unused > 2 weeks needs
   reconnect. Handled by the standard 401→reconnect passthrough; no new code, but
   call it out for the token gateway's reconnect UX.
5. **`disconnect_mode: local_only`** pending L5 — no documented OAuth token-revoke
   endpoint was found; if one exists, switch to `provider_revoke`.
