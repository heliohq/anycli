# Freshdesk — per-tool design (`heliox tool freshdesk`)

**Status:** proposed (Wave 1, Support category, row 20).
**Auth lane:** `api_key` — confirmed against the master catalog (§4 row 20) and the
2026-07-21 OAuth audit (row 20: "no viable multi-tenant path → api_key").
**Naming (three axes, all aligned — no divergence):**

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `freshdesk` | bundle `tool.command` (defaults to `tool.name`; flat, not grouped) |
| ② anycli tool id | `freshdesk` | `definitions/tools/freshdesk.json` + `RegisterService("freshdesk", …)` |
| ③ provider catalog key | `freshdesk` | `integrations/providers/freshdesk/` |

Because ② == ③, **no `toolToProvider` entry** is added to
`helio-cli/internal/toolcred/resolver.go` (the mechanical identity case). Go
package name is `freshdesk` (no leading digit, no dashes).

---

## 1. Official API surface & why these endpoints

Source of truth: the official Freshdesk API v2 docs (`https://developers.freshdesk.com/api/`),
verified 2026-07-22. Ground facts that drive the whole design:

- **Base URL is per-account:** `https://<domain>.freshdesk.com/api/v2/<resource>`
  (e.g. `https://acme.freshdesk.com/api/v2/tickets`). "Works only via Freshdesk
  domains and not via custom CNAMEs", HTTPS only. **The `<domain>` subdomain is
  not derivable from the API key** — it is a required second input (see §3, §4).
- **Auth is HTTP Basic:** the API key as username with any dummy password —
  documented literally as `-u apikey:X`, i.e.
  `Authorization: Basic base64("<api_key>:X")`. No OAuth. No bearer token.
- **Identity endpoint:** `GET /api/v2/agents/me` ("Currently Authenticated
  Agent") — the natural credential-verification + account-label call.
- **Rate limits:** account-wide per-minute; `429` carries `Retry-After`
  (seconds). Response headers `X-RateLimit-Remaining` / `X-RateLimit-Total`.
- **Pagination:** `page` (1-based) + `per_page` (default 30, max 100); a `link`
  response header holds the next-page URL. Avoid page > 500 (deep pagination).

### What an AI teammate actually does in Freshdesk

Freshdesk is a customer-support helpdesk. A teammate's real jobs are: **triage
and answer tickets**, **look up who a requester/company is**, and **report on
queue state**. That maps to a focused, resource-grouped surface — not the full
CRUD-for-everything catalog. Wrapped endpoints and the reason each is included:

| Group | Endpoint(s) | Why an AI teammate needs it |
|---|---|---|
| Tickets | `GET /tickets`, `GET /tickets/{id}` (`?include=conversations,requester`), `POST /tickets`, `PUT /tickets/{id}` | Read the queue, open a ticket in full, file a new ticket, change status/priority/assignment/tags. Core loop. |
| Ticket search | `GET /search/tickets?query="…"` | Find tickets by requester, status, priority, tag, updated-at — the "does an open ticket already exist for X?" question. |
| Conversations | `POST /tickets/{id}/reply`, `POST /tickets/{id}/notes`, `GET /tickets/{id}/conversations` | Reply to the customer, add a private/public internal note, read the full thread. This is the teammate's primary write action. |
| Contacts | `GET /contacts`, `GET /contacts/{id}`, `POST /contacts`, `PUT /contacts/{id}`, `GET /search/contacts?query="…"` | Identify/resolve the requester, create or correct a contact record. |
| Companies | `GET /companies`, `GET /companies/{id}`, `GET /search/companies?query="…"` | Account context for B2B support ("what else is open for this company?"). |
| Agents | `GET /agents`, `GET /agents/{id}`, `GET /agents/me` | Route/assign tickets to the right agent; `/agents/me` is also the identity check. |

Deliberately **out of scope for v1** (keep the surface small, add later only if
demand appears): admin/config resources (groups, business hours, SLA policies,
canned responses, ticket fields/forms), the surveys/satisfaction API, time
entries, and bulk/async endpoints. None are part of the day-to-day teammate
loop; they are administrator territory.

---

## 2. anycli definition & service

### 2.1 Tool form — `service` type

`cli` type is rejected: there is no official, non-interactive, `--json`-capable
Freshdesk binary to provision into the runtime image. Freshdesk is a plain REST
API → **`service` type** implemented in `internal/tools/freshdesk/`, following
the `internal/tools/notion` / `internal/tools/bitly` shape (a `Service` struct
with `BaseURL`/`HC`/`Out`/`Err` seams so tests point at an `httptest` server).

### 2.2 Command tree (cobra, resource-grouped)

```
freshdesk
  ticket   list            GET  /tickets              (--updated-since --status --priority --requester-id --company-id --page --per-page --order-by --order-type)
           get    --id     GET  /tickets/{id}         (--include conversations,requester,company,stats)
           create          POST /tickets              (--subject --description --email|--requester-id --priority --status --group-id --responder-id --tags --cc --custom-fields <json>)
           update --id     PUT  /tickets/{id}         (--subject --description --priority --status --group-id --responder-id --tags --add-tags --custom-fields <json>)
           search --query  GET  /search/tickets?query (Freshdesk query language, quoted)
           reply  --id     POST /tickets/{id}/reply   (--body --cc --bcc)
           note   --id     POST /tickets/{id}/notes   (--body --private/--public --notify <emails>)
           conversations   GET  /tickets/{id}/conversations   --id (--page --per-page)
  contact  list            GET  /contacts             (--email --company-id --updated-since --page --per-page)
           get    --id     GET  /contacts/{id}
           create          POST /contacts             (--name --email --phone --mobile --company-id --custom-fields <json>)
           update --id     PUT  /contacts/{id}
           search --query  GET  /search/contacts?query
  company  list            GET  /companies            (--page --per-page)
           get    --id     GET  /companies/{id}
           search --query  GET  /search/companies?query
  agent    list            GET  /agents               (--email --page --per-page)
           get    --id     GET  /agents/{id}
           me              GET  /agents/me             (identity / connectivity check)
```

Conventions (inherited from the built-in service contract, design 003 §3, and
the notion/bitly precedents):

- **JSON passthrough output.** Every command writes the provider's JSON response
  verbatim to stdout (+ newline). No client-side reshaping — the AI consumes
  Freshdesk's native shape. `--json` is a persistent no-op flag accepted for
  uniformity (output is always JSON).
- **Non-interactive only.** All input via flags/env; structured bodies
  (`--custom-fields`, `--cc`) accept raw-JSON flag values validated before send.
- **Exit-code contract:** `0` success; `1` runtime/API failure via a typed
  `apiError` (Freshdesk error body `{"description":…, "errors":[…]}` surfaced);
  `2` usage/parse error. `401`/`403` (bad or revoked API key) map to
  `execution.RejectCredential(err)` so the host marks the credential rejected.
  `429` surfaces Freshdesk's `Retry-After` in the error message (no client-side
  retry — the teammate/host decides).
- **Pagination is explicit, not auto-followed.** `--page`/`--per-page` flags
  passthrough; the tool does not silently walk all pages (bounded output for an
  AI context window). Per-page default 30, documented max 100.

### 2.3 Definition JSON (`definitions/tools/freshdesk.json`)

Two credential bindings — Freshdesk needs the **key** (Basic-auth secret) **and**
the **domain** (to build the base URL). Both come from the resolver-supplied
data map (Helio bundle projection in §3.3; `ANYCLI_CRED_*` for the L2 harness):

```json
{
  "name": "freshdesk",
  "type": "service",
  "description": "Freshdesk support desk as a tool (tickets, contacts, agents) via the v2 REST API",
  "auth": {
    "credentials": [
      { "source": {"field": "api_key"}, "inject": {"type": "env", "env_var": "FRESHDESK_API_KEY"} },
      { "source": {"field": "domain"},  "inject": {"type": "env", "env_var": "FRESHDESK_DOMAIN"} }
    ]
  }
}
```

The `Service.Execute` reads both env vars; missing either → exit 1 with a clear
message. It **normalizes the domain** (accepts `acme`, `acme.freshdesk.com`, or
`https://acme.freshdesk.com` and reduces to the `<domain>.freshdesk.com` host),
then builds `baseURL = https://<host>/api/v2` and the Basic header
`Authorization: Basic base64("<api_key>:X")`. anycli stays credential-agnostic:
it only knows two field names and how to deliver them — exactly the mongodb
two-value-in-one-secret precedent generalized to two clean fields.

---

## 3. Helio provider bundle plan (`integrations/providers/freshdesk/provider.yaml`)

### 3.1 Auth shape — `api_key` / `manual_credentials`, hidden-first

Freshdesk is a **manually-supplied credential** provider: the user pastes their
API key (from Freshdesk → Profile Settings → *Your API Key*) and gives their
Freshdesk domain. There is no OAuth, so **no `oauth.*` config, no client
id/secret, no `required_config_fields`** — the bundle is fully self-contained and
renders `configured: true` with zero integration-service config (unlike the
OAuth lanes, this adds nothing to human lane 1's config-append queue). Ships
`presentation.visible: false` first (hidden-first: registers as a cobra-hidden,
runnable heliox command so L4/L5 run against it as-is; flip is the single
go-live change after L5).

### 3.2 The one real design decision — carrying the per-account **domain**

Freshdesk is the first catalog exemplar of the **"instance base URL + token"**
credential class the master plan flags in open question 3 (Jenkins, Metabase,
Rocket.Chat, Mattermost are the same shape, later waves). The integration-service
storage face today is **a single vault secret** plus a non-secret,
human-readable `account_key` on the `Connection` (design 317 D5;
`model.validateCredentialInputSchema` hard-caps a declared connect form to
**exactly one required secret field**). Freshdesk needs the API key **and** the
domain. Two options were considered:

**Option A (recommended) — domain is the `account_key`; small capability growth.**
The Freshdesk domain is exactly what `account_key` is for: a stable,
human-readable per-account identity and dedup key (the mongodb bundle already
uses the DSN host as `account_key`). Storage stays single-secret: the **API key**
is the one vault secret (`token.access_token`); the **domain** is the
`account_key` (a `Connection` field, never in vault). Projection to anycli:

```yaml
credential:
  fields:
    api_key: token.access_token        # the single vault secret
    domain:  connection.account_key    # the Freshdesk subdomain, reused as account_key
```

Both sources already exist in the closed `CredentialSource` allowlist
(`model.CredentialSourceTokenAccessToken`, `CredentialSourceConnectionAccountKey`)
and `projectCredential` already materializes both — **zero token-gateway
change**. The growth is confined to the **connect write path**: the connect form
must accept the domain alongside the secret, and set `account_key = <domain>`.
Concretely:

  1. Relax `validateCredentialInputSchema` to allow, in addition to the one
     required `secret: true` field, one **non-secret** `required` field flagged
     as the account/instance input — its value lands in `account_key`, never in
     the vault payload, so the single-secret storage invariant is preserved
     (this is the minimal, orthogonal extension the D5 comment itself defers to
     "the D8 multi-field vault face"; it does **not** open multi-secret storage).
  2. Add an **instance-scoped manual verifier** (sibling of
     `declarativeManualTokenVerifier` / `manualCredentialsIdentity`): build
     `https://<domain>.freshdesk.com/api/v2/agents/me`, send
     `Authorization: Basic base64("<api_key>:X")`, and on 2xx set
     `account_key = <domain>` and label from the agent's `contact.name`/`email`
     in the response. `401/403` → `invalid_provider_credential` (verify-first,
     before any Vault write). This gives Freshdesk **real** connect-time
     verification rather than the mongodb no-verify (OQ1) fallback.

  Bundle shape for Option A:

```yaml
auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - {name: domain,  label_key: freshdesk_domain,  secret: false, required: true,
         placeholder: "acme.freshdesk.com"}
      - {name: api_key, label_key: freshdesk_api_key, secret: true,  required: true,
         placeholder: "your Freshdesk API key"}
    setup_url: https://support.freshdesk.com/support/solutions/articles/215517
identity:
  source: strategy            # instance-scoped verifier derives account_key = domain
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials
tool: {name: freshdesk, kind: api-key}
```

**Option B (rejected) — combined single secret `"<domain>|<api_key>"`.** Keeps
the connect form to one field and a Freshdesk `identity.source: strategy` parser
splits the domain out (pure mongodb-DSN analogue, no integration-service schema
change). Rejected because it couples two orthogonal concerns into one opaque
pasted string (poor, error-prone UX; anti-orthogonal per repo Code Health), and
still needs a Freshdesk-specific strategy parser — so it is not actually less
integration-service code, only worse ergonomics and no connect-time verify.

Option A is the recommended target: it is orthogonal (secret vs instance are
separate inputs), verifiable, reuses existing credential sources, and **seeds
the OQ3 URL+token capability** that ≥4 later tools reuse — landing it once for
Freshdesk pays down that class. It is comparable in size to the per-account
instance-URL growth sibling Wave-1 tools already landed (Salesforce
`instance_url` capture, Zendesk instance-scoped OAuth). Flag at stage 1 for the
batch lead: this bundle is **not** a zero-service-code `standard_oauth` drop-in;
it carries the one bounded capability-growth item above.

### 3.3 Icon, docs, resolver

- **UI icon:** `ui/helio-app/src/integrations/icons/freshdesk.svg` + register in
  `ui/helio-app/src/integrations/providerIcons.ts` (manual, never generated).
- **AI-facing docs:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` (the ticket-reply loop is the headline;
  document the domain-at-connect requirement and the search query language),
  then plugin version bump + marketplace publish (batch-end).
- **Resolver:** none — ② == ③, so no `toolToProvider` entry.
- **Generation:** `provider-gen` + `provider-gen --check` from
  `go-services/integration-service`; commit all five projections with the bundle
  in one change (batch-end).

---

## 4. Credential fields & exact auth flow (api_key lane verified)

- **Registration model:** none. Any Freshdesk agent/admin reads their API key
  from *Profile Settings → Your API Key* (or resets it there). No app
  registration, no OAuth client, no review — the `api_key` lane is correct and
  needs no human-lane-1 app creation. (Divergence check vs. catalog/audit:
  official docs confirm no multi-tenant authorization-code OAuth exists → the
  audit's api_key verdict holds; **no divergence to record**.)
- **Token semantics:** the API key is a **long-lived, non-expiring** per-agent
  secret with the same scope/permissions as that agent in the web portal. No
  refresh cycle, no scopes parameter. Revocation = the user resets the key in
  Freshdesk (surfaces as `401` → `RejectCredential`, connection needs
  reconnect).
- **Wire auth:** `Authorization: Basic base64("<api_key>:X")` against
  `https://<domain>.freshdesk.com/api/v2/…`. The dummy password is literally
  `X` (any characters accepted). This is built inside the anycli service from
  the two injected env vars; the Helio token gateway only projects the two
  fields — it never constructs Basic auth.
- **Two credential inputs, one secret:** `api_key` (secret, → vault via
  `token.access_token`) and `domain` (non-secret, → `account_key`). See §3.2.

---

## 5. Test plan — five layers

| Layer | Scope for Freshdesk | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in anycli. `httptest` fake Freshdesk server asserts: base-URL built from `FRESHDESK_DOMAIN` (incl. domain normalization of `acme` / `acme.freshdesk.com` / full URL), `Authorization: Basic base64(apikey:X)` header, each subcommand's method/path/query/body, JSON passthrough, and error rendering — `401/403`→`RejectCredential`, `429`→`Retry-After` surfaced, `2` on usage errors. Never hits the real API. | No |
| **L2** harness real-API | `make build-harness` then `ANYCLI_CRED_API_KEY=<key> ANYCLI_CRED_DOMAIN=<domain> anycli freshdesk -- agent me` and a `ticket list` / `ticket search --query "status:2"`. Proves field names, env injection, Basic-auth construction, and request shapes match the live API. Mandatory before the pin bump. | **Yes** — a real Freshdesk account API key + its domain (account pool, human lane 2). A free trial account suffices. |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` green with the bundle; `go test ./...` in integration-service (incl. the Option-A capability-growth unit tests: instance-scoped verifier build/verify/label + the relaxed `validateCredentialInputSchema`) and `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with a local `replace` to the anycli branch. | No |
| **L4** singleton + seed | Start singleton (`env: dev`). Seed via `POST /internal/test-only/connections/seed` with `provider: freshdesk`, `access_token: <api_key>`, `account_key: <domain>` (the seed writes the same single-secret + account_key shape the connect path produces; api_key is a user-token provider so it is seedable — non-expiring, seed `access_token` only, no `refresh_token`). Then `heliox tool freshdesk -- agent me` and `ticket list` resolve the token through the real gateway and hit live Freshdesk. | **Yes** — same real key + domain as L2. |
| **L5** full connect flow | Hidden-tool pre-flip check via the **api_key key-entry path** (master plan §2, not the OAuth checklist): `heliox tool freshdesk auth` → open connect link → enter **domain + API key** in the real connect UI → connection shows connected/`configured` in `GET /connections` → one **unseeded** live command (`ticket list`) through the real token gateway succeeds. This is the step that exercises Option A's two-field connect form + instance verifier end-to-end. Agent-drivable (agent-browser; key pasted from the pool) with human fallback. | **Yes** — real key + domain; run once before the visible flip. |

**Rollout:** land hidden → bump anycli pin (freshdesk tool shipped) → L1–L4 green
→ L5 key-entry sweep → flip `presentation.visible: true` + regenerate as the
single go-live change.

---

## 6. Open items for the batch lead

1. **Option-A capability growth is on the critical path** for this tool — it is
   not a `standard_oauth`/zero-service-code bundle. It's a bounded, reviewed
   extension (one non-secret account field + one instance-scoped verifier),
   reused by the OQ3 URL+token class. Confirm it lands with the bundle, not after.
2. **First "instance URL + token" exemplar.** Whatever shape Option A takes
   should be written to generalize (a non-secret "instance" input feeding
   `account_key` + an instance verifier that templates the identity URL), so
   Jenkins/Metabase/Rocket.Chat/Mattermost reuse it rather than each re-growing.
3. **Domain normalization lives in anycli** (accept bare/host/URL). The bundle
   stores whatever the user typed as `account_key`; the service normalizes at
   request time. Keep one normalization site — the anycli service — so seeded
   and connect-entered domains behave identically.
