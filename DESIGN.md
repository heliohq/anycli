# Brevo — `heliox tool brevo` design

Scratch design for the Brevo external tool provider, per the
`helio-tool-provider` pipeline. Batch-lead strips this file at batch-end.

- **anycli id (axis ②):** `brevo`
- **provider catalog key (axis ③):** `brevo`
- **CLI command word (axis ①):** `brevo` (flat command; no group)
- **Auth lane:** `api_key` (Wave 2, Marketing category — catalog row 127)
- **Master plan row:** `| 127 | Brevo | brevo | brevo | api_key | 2 | Marketing |`
- **OAuth audit verdict:** `api_key` — "no viable multi-tenant path" (audit row 129)

Because axis ② == axis ③ == `brevo` (no dashes, no corporate prefix), there is
**no** `toolToProvider` divergence entry and **no** grouped-family membership.
Go package name (stage 2) is `brevo` — the id has no dashes and no leading
digit, so no normalization applies.

---

## 1. What an AI teammate does with Brevo → API surface

Brevo (formerly Sendinblue) is an all-in-one marketing/CRM platform: email
marketing campaigns, transactional email/SMS, contact lists, and a light CRM.
For a Helio AI teammate the load-bearing jobs are **sending mail**, **managing
the contact database**, and **reading campaign/list state** to report on
outreach. The tool wraps the **Brevo REST API v3** (base
`https://api.brevo.com/v3`, verified from the official cURL example in the
[getting-started guide](https://developers.brevo.com/docs/getting-started)).

Endpoint selection is driven by those jobs, not by wrapping the whole 200+
endpoint surface. The v1 tool wraps this subset (all verified against the
official reference):

| Job (what the teammate is asked to do) | Endpoint(s) | Verb |
|---|---|---|
| Send a one-off/event email ("email this customer the summary") | `POST /v3/smtp/email` | write |
| Add / upsert a contact ("add jane@acme.com to Brevo") | `POST /v3/contacts` (`updateEnabled:true` to upsert) | write |
| Update a contact's attributes | `PUT /v3/contacts/{identifier}` | write |
| Look up a contact | `GET /v3/contacts/{identifier}` | read |
| List / search contacts | `GET /v3/contacts` | read |
| Delete a contact (GDPR/cleanup) | `DELETE /v3/contacts/{identifier}` | write |
| Manage list membership ("add these to the Newsletter list") | `POST /v3/contacts/lists/{listId}/contacts/add` | write |
| Discover list IDs / read a list | `GET /v3/contacts/lists`, `GET /v3/contacts/lists/{listId}` | read |
| Create a list | `POST /v3/contacts/lists` | write |
| Report on campaigns | `GET /v3/emailCampaigns`, `GET /v3/emailCampaigns/{campaignId}` | read |
| Create an email campaign | `POST /v3/emailCampaigns` | write |
| Resolve a verified sender before sending | `GET /v3/senders` | read |
| Account identity / plan+credits check | `GET /v3/account` | read |

Transactional (`/v3/smtp/email`) and marketing (`/v3/emailCampaigns`) are kept
as **distinct verbs** — they are distinct Brevo surfaces (one-off event mail vs
scheduled bulk campaign), and conflating them is a known agent footgun.

Confirmations verified against official docs:
- `POST /v3/contacts` force-merges on shared identifier unless `updateEnabled`
  is set; list IDs are **integers**, not strings.
- Brevo **blocks all email-send API calls from unverified senders** (even for
  testing) — `GET /v3/senders` is therefore a first-class discovery verb so
  the agent can pick a verified sender rather than 400 on send.

Out of scope for v1 (documented so a later agent can extend, not re-derive):
SMS (`/v3/transactionalSMS`), WhatsApp, CRM deals/companies/tasks
(`/v3/crm/*`), custom objects, and webhooks. These are additive verbs on the
same `api-key` credential; none change the auth or bundle shape.

---

## 2. anycli definition

**Type: `service`** (stage-1 rubric). Brevo ships no official
non-interactive, `--json`-capable, env-injectable CLI that can be provisioned
into the runtime image. (Brevo *does* now publish `@getbrevo/cli`, but it is an
OAuth-app **scaffolding/dev** tool — `brevo app create`, local test server —
not a credential-injected data-plane binary; it fails every `cli`-type gate.)
So the HTTP logic is a hand-written service under
`internal/tools/brevo/`, registered `RegisterService("brevo", &brevo.Service{})`
in `internal/tools/register.go`. This matches 21/23 shipped definitions and the
`internal/tools/notion/` reference shape.

**Definition file** `definitions/tools/brevo.json`:

```json
{
  "name": "brevo",
  "type": "service",
  "description": "Brevo (email marketing, transactional email, contacts, CRM) as a tool",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "BREVO_API_KEY"}
      }
    ]
  }
}
```

The credential field is `access_token` (not `api_key`): the Helio token gateway
always projects the stored secret through `credential.fields.access_token`
(§4), and the anycli harness maps `ANYCLI_CRED_ACCESS_TOKEN` → `access_token`.
Injected as env `BREVO_API_KEY`; the service reads it and sends it as the
`api-key` request header (Brevo's scheme — verified from the official cURL
`-H "api-key: YOUR_API_KEY"`).

### Command tree (cobra, grouped by resource — notion pattern)

```
brevo email send            # POST /v3/smtp/email   (transactional)
brevo contact create        # POST /v3/contacts
brevo contact update        # PUT  /v3/contacts/{id}
brevo contact get           # GET  /v3/contacts/{id}
brevo contact list          # GET  /v3/contacts
brevo contact delete        # DELETE /v3/contacts/{id}
brevo list ls               # GET  /v3/contacts/lists
brevo list get              # GET  /v3/contacts/lists/{id}
brevo list create           # POST /v3/contacts/lists
brevo list add-contacts     # POST /v3/contacts/lists/{id}/contacts/add
brevo campaign list         # GET  /v3/emailCampaigns
brevo campaign get          # GET  /v3/emailCampaigns/{id}
brevo campaign create       # POST /v3/emailCampaigns
brevo sender ls             # GET  /v3/senders
brevo account get           # GET  /v3/account
```

### Service struct + JSON output shape

Mirror `notion.Service`: a struct carrying `BaseURL` / `HC *http.Client` /
`Out io.Writer` / `Err io.Writer` so unit tests point at an `httptest.Server`
and capture output. Exit-code contract: `0` success, `1` runtime/API failure
(typed `apiError` rendering Brevo's `{"code","message"}` error body), `2`
usage/parse errors.

Default output is the provider JSON passed through (agent-friendly). Every
command supports `--json` for a structured envelope; on error, `--json` emits
`{"error":{"code","message","status"}}` carrying Brevo's own `code`/`message`
and the HTTP status. Brevo error bodies are `{ "code": "...", "message": "..." }`
(e.g. `unauthorized`, `invalid_parameter`), which the service surfaces verbatim
in the envelope so the agent can self-correct.

---

## 3. Credential fields & auth flow (api_key lane — verified)

### Registration & token semantics (official docs)

- **Auth scheme:** API key in the HTTP header **`api-key`** (verified:
  getting-started cURL and `GET /v3/account` reference both require `api-key`).
- **Header name matches** the generator's `headerNamePattern`
  `^[A-Za-z][A-Za-z0-9-]*$` — `api-key` is valid.
- **Registration model:** fully **self-serve, no review**. A Brevo user mints a
  v3 API key from the dashboard (Settings → **SMTP & API** → **API Keys**). No
  app registration, no client id/secret, no Helio-side config.
- **Scopes:** v3 API keys are **account-scoped, all-or-nothing** — there is no
  per-key scope selection on the api_key path (scopes exist only on the OAuth
  path, §3.1). The key grants the account's full API surface.
- **Token lifetime:** non-expiring until the user revokes/rotates it in the
  dashboard. No refresh cycle.

### Helio-side flow

`api_key` / `manual_api_token` bundle. The user pastes their key into the
connect drawer; it is stored in Vault through the write-only
`POST /connections/credentials` path (no OAuth redirect). At tool-run time the
token gateway serves it and heliox injects it into anycli's credential map as
`access_token`. Because the key never expires, seed/serve is direct — no
refresh-and-write-back path to exercise (§5, L4).

**Identity / connection labelling** (verified against `GET /v3/account`, which
returns `email` = "Login Email", `companyName`, `firstName`, `lastName`):

- `identity.source: userinfo`, `identity.url: https://api.brevo.com/v3/account`
- `stable_key: /email` (login email — stable per account)
- `label_candidates: [/companyName, /email]` (company name first, email
  fallback)

This endpoint doubles as the api_key **verification** endpoint: a bad key
returns `401 {"code":"unauthorized"}`, so the connect flow rejects invalid keys
at entry rather than at first tool use.

### 3.1 Divergence from the OAuth audit — recorded

The audit (row 129) lanes Brevo `api_key` with note "no viable multi-tenant
path" and **no** evidence URL. Independent verification against official docs
**refines the evidence but confirms the verdict**:

Brevo **does** now offer an OAuth 2.0 authorization-code flow (Keycloak-based,
added 2024–2025 via the public Brevo CLI):
- authorize `https://oauth.brevo.com/realms/partner/oauth/authorize`
- token `https://oauth.brevo.com/realms/partner/oauth/token`
- access token 1h, refresh token 30d, scopes like `contacts:read`,
  `contacts:write`, `crm:read`, `crm:write`.

However, per Brevo's own docs, **OAuth is currently only available for private
integrations inside an organization — non-public / non-distributable apps, not
intended for public distribution or marketplace listing.** Under the audit
rubric ("OAuth moves lanes only when one registered app can be authorized by
*arbitrary customer accounts*"), a per-org private integration is **not** a
viable multi-tenant shared-client flow. **The `api_key` verdict therefore
stands**, but with corrected evidence: OAuth exists but is
private-integration-only, not "absent." If Brevo later opens OAuth to public
distributable apps, Brevo becomes an `oauth_light`/`oauth_review` re-lane
candidate (§6 catalog amendment) — the endpoints above are recorded here so
that future agent does not re-derive them. Until then, api_key is both correct
and strictly simpler (no lane-1 app registration, agent-drivable L5).

---

## 4. Helio provider bundle plan (`integrations/providers/brevo/provider.yaml`)

Hidden-first. Shape follows the `api_key` / `manual_api_token` contract in
`provider-gen/manifest.go` + `validate.go` (verified against the merged
`mongodb` credentials bundle and the api_key validation path):

```yaml
schema: helio.provider/v1
key: brevo
go_name: Brevo

presentation:
  name: Brevo
  description_key: brevo
  consent_domain: brevo.com
  visible: false            # hidden until anycli pin ships the brevo tool + L5 passes

auth:
  type: api_key
  owner: individual
  api_key:
    header: api-key
    setup_url: https://app.brevo.com/settings/keys/api

identity:
  source: userinfo
  url: https://api.brevo.com/v3/account
  stable_key: /email
  label_candidates: [/companyName, /email]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: brevo
  kind: api-key
```

Notes:
- `runtime_strategy: manual_api_token` requires **zero** server config fields
  (`required_config_fields` omitted) — validate.go rejects config fields for
  this strategy. No `config/` or `deploy/` secret changes, no lane-1 work.
- No `auth.credential_input` block: for api_key bundles it is optional and its
  absence selects the client's implicit single-token default schema (one
  secret field) — correct for Brevo's single account-scoped key.
- `presentation.order` intentionally omitted while hidden (validate.go only
  requires a positive `order` for *visible* providers); it's assigned at the
  visible flip.
- **Three axes**: `tool.name: brevo` (②) == directory `brevo` (③); `tool.command`
  omitted → defaults to `brevo` (①). No group, no resolver entry.

Icon (not in bundle): `ui/helio-app/src/integrations/icons/brevo.svg` +
manual registration in `providerIcons.ts`. i18n: `tools.desc.brevo` +
`description_key: brevo` label across locales.

AI-facing doc: provider sub-doc under
`agents/plugins/heliox/skills/tool/` describing the verb tree and the
verified-sender-before-send gotcha; plugin version bump + marketplace publish
at batch-end.

---

## 5. Test plan → five layers

| Layer | Scope for Brevo | External creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: `internal/tools/brevo/` unit tests against an `httptest.Server` fake — assert request path/verb, `api-key` header injection, request body shape (send email, contact upsert `updateEnabled`, list add), and both plaintext + `--json` error rendering of Brevo's `{code,message}` body. No network. | No |
| **L2** | Dev harness against the **real** Brevo API: `ANYCLI_CRED_ACCESS_TOKEN=<key> anycli brevo -- account get`, then `sender ls`, `contact create --json` (upsert), `list ls`, `email send` from a verified sender. Proves field names + injection match live API. **Mandatory before pin bump.** | **Yes** — a real Brevo API key (free-tier account) + one verified sender for the send test |
| **L3** | `provider-gen` + `provider-gen --check` (five projections consistent); `helio-cli` build with local `go.mod replace` → anycli branch; `go test ./cmd/heliox/cmds/tool/`; integration-service unit suite. api_key/`manual_api_token` needs **no** integration-service capability growth — the strategy already exists (mongodb/other api_key bundles). | No |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with `provider:"brevo"`, `access_token:<real key>` (api_key providers **are** seedable — only minted is rejected). Seed `access_token` **only** (non-expiring key, no `refresh_token`/`expires_at` — nothing to refresh). Then `heliox tool brevo -- account get` / `contact list` returns real data through the real token gateway. | **Yes** — real key + a real seeded assistant/org/user identity in local Mongo |
| **L5** | One full connect round-trip, hidden, before the visible flip — the **api_key key-entry path** (master plan §2): open connect link → paste key in the real connect UI → stored via `POST /connections/credentials`, verified against `GET /v3/account` → connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool brevo -- account get` succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. | **Yes** — real key + running singleton connect UI |

Go-live: only after L5 passes, flip `presentation.visible: true` (+ assign
`presentation.order`) and regenerate as the single go-live change.

### External-credential summary
L1 and L3 need **no** external credentials. L2, L4, L5 all require a **real
Brevo API key** (free-tier self-serve — no app registration, no review), and
L2/L5's email-send step additionally needs **one verified sender** on that
account (Brevo blocks sends from unverified senders). No OAuth app, no lane-1
registration, no client id/secret anywhere in the flow.

---

## Sources

- [Getting started (auth header, base URL)](https://developers.brevo.com/docs/getting-started)
- [Authentication schemes](https://developers.brevo.com/docs/authentication-schemes)
- [GET /v3/account (identity fields)](https://developers.brevo.com/reference/get-account)
- [Send a transactional email](https://developers.brevo.com/reference/send-transac-email)
- [Manage contacts / lists](https://developers.brevo.com/docs/synchronise-contact-lists)
- [Get all lists](https://developers.brevo.com/reference/get-lists) · [Get senders](https://developers.brevo.com/reference/get-senders) · [Get email campaigns](https://developers.brevo.com/reference/get-email-campaigns)
- [OAuth 2.0 integration guide](https://developers.brevo.com/docs/oauth-integration-guide) · [OAuth 2.0 (private-integration limitation)](https://developers.brevo.com/docs/integrating-oauth-20-to-your-solution) · [Public Brevo CLI release](https://www.brevo.com/releases/oauth-integrations/)
