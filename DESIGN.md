# Apollo.io — `heliox tool` provider design

Scratch design for the `tool/apollo` batch branch (both repos). Batch lead
strips this file at batch-end. Catalog row 65: `apollo` / `apollo` /
`oauth_review` / Wave 2 / Sales Engagement. OAuth audit row 67 verdict:
`oauth_review`, confidence high.

All facts below were verified against Apollo's official developer docs
(`docs.apollo.io`) on 2026-07-22, not inherited from the catalog. Where the
official docs add material nuance beyond the catalog/audit, it is called out
under **Divergences** (§7).

---

## 1. What an AI sales teammate does with Apollo, and the API surface that serves it

Apollo.io is a sales-intelligence + engagement platform. Its objects, per the
official API reference, are **People/Contacts, Organizations/Accounts,
Sequences (emailer campaigns), Email Accounts, Tasks, and Opportunities
(deals)**. An AI teammate on Helio uses Apollo to: find net-new prospects,
enrich a known person/company with verified email/phone, save prospects as
contacts, enroll contacts into outbound sequences, create follow-up tasks, and
read/advance deal state. That intent drives the wrapped surface.

Base URL (all REST): `https://api.apollo.io/api/v1`. Every call is JSON
in / JSON out.

Endpoints the tool wraps, grouped by the object an agent reasons about:

| Group | Endpoint (method + path) | Why an agent needs it |
|---|---|---|
| `people search` | `POST /mixed_people/api_search` | Net-new prospecting by title/seniority/location/industry filters. Returns people (no contact details). ⚠ master-key-gated — see §7. |
| `people enrich` | `POST /people/match` ; `POST /people/bulk_match` | Resolve one / up to 10 people to verified email + phone (the credit-consuming enrichment step). |
| `org search` | `POST /mixed_companies/api_search` | Find target accounts by size/industry/location. |
| `org enrich` | `GET /organizations/enrich` ; `POST /organizations/bulk_enrich` | Company firmographics from a domain. |
| `contacts` | `POST /contacts` ; `POST /contacts/bulk_create` ; `PUT /contacts/{id}` ; `POST /contacts/search` ; `GET /contact_stages` | Persist a prospect into the team's DB (prerequisite for sequencing), update stage, list existing contacts. |
| `accounts` | `POST /accounts` ; `PUT /accounts/{id}` ; `POST /accounts/search` | Account (saved company) CRUD + search. |
| `sequences` | `GET /emailer_campaigns/search` ; `POST /emailer_campaigns/{id}/add_contact_ids` ; `POST /emailer_campaigns/remove_or_stop_contact_ids` | List sequences, enroll contacts, stop/remove. ⚠ add/remove are master-key-gated (§7); enrollment also needs a sending `email_account_id`. |
| `tasks` | `POST /tasks/bulk_create` ; `POST /tasks/search` | Create follow-up tasks against contacts; list tasks. |
| `deals` | `POST /opportunities` ; `GET /opportunities/search` ; `PATCH /opportunities/{id}` | Pipeline read + write (`opportunity_write` scope). |
| `users` | `GET /users/search` ; `GET /email_accounts` | Resolve team member ids and sending mailboxes (needed to enroll into sequences). |

Scope decision: **read + enrich + light write is the core.** People/Org
search and enrichment plus contacts/sequences are the high-value surface;
opportunities and tasks round out CRM parity. We ship all groups as
subcommands but treat the master-key-gated ones (§7) as L2-conditional — a
group that OAuth provably cannot reach is dropped from the shipped tool rather
than shipped broken (no silent-fallback rule).

Endpoints deliberately **not** wrapped: bulk *export* jobs, webhook
management, and the deprecated `/v1/…` non-`api_search` people endpoints —
out of an agent's turn-scoped workflow.

## 2. anycli definition

**Type: `service`.** Apollo has no official first-party CLI; it is a pure REST
API. Per the stage-1 rubric (`cli` only when an official, non-interactive,
`--json`-capable, image-provisionable binary exists), Apollo is unambiguously
`service` type. Implementation lives in `internal/tools/apollo/`, registered
`RegisterService("apollo", &apollo.Service{})` in `internal/tools/register.go`;
definition at `definitions/tools/apollo.json`.

Package name = anycli id = `apollo` (no dashes, no leading digit → no
normalization; matches §3 Go-package rule).

Cobra tree (grouped by resource, following the `internal/tools/notion/`
reference shape — `BaseURL`/`HC`/`Out`/`Err` struct, httptest-driven tests,
exit-code contract 0/1/2, `--json` structured error envelope):

```
apollo people search   --title --seniority --location --org --page --per-page
apollo people enrich   --email | --name --org-domain   (single: /people/match)
apollo people bulk-enrich --file/-           (/people/bulk_match, ≤10)
apollo org search      --industry --location --employees-min --employees-max
apollo org enrich      --domain
apollo contacts create --email --first-name --last-name --title --org
apollo contacts bulk-create --file           (≤100, run_dedupe flag)
apollo contacts update <id> --stage-id …
apollo contacts search --q --page
apollo contacts stages                        (GET /contact_stages)
apollo accounts create|update|search
apollo sequences list  --q
apollo sequences add    <sequence_id> --contact-ids … --email-account-id …
apollo sequences stop   <sequence_id> --contact-ids …
apollo tasks create --contact-id --type --due-at --priority
apollo tasks search
apollo deals create|search|update
apollo users list
apollo email-accounts list
```

**JSON output shape.** Every leaf command emits a single JSON object on stdout
(`--json` is the default for agent consumption per anycli AGENTS.md): the
provider's response body passed through with a thin, stable envelope
`{ "ok": true, "data": <apollo response> }` on success, and the notion-style
`{ "ok": false, "error": { "code", "message", "status" } }` on failure. List
endpoints preserve Apollo's own `page`/`per_page`/`total_entries` pagination
fields inside `data` — we do not re-paginate. Exit codes: 0 success, 1
runtime/API failure (typed `apiError`), 2 usage/parse error.

## 3. Credentials & auth flow (oauth_review lane verified)

**Auth type: OAuth 2.0 authorization-code.** Verified against
`docs.apollo.io/docs/use-oauth-20-authorization-flow-…`:

- **Authorize:** `https://app.apollo.io/#/oauth/authorize` (SPA fragment form — §7).
- **Token (code exchange + refresh):** `POST https://app.apollo.io/api/v1/oauth/token`.
- **Grant:** authorization_code; **no PKCE** (relies on client secret).
- **Token type:** `Bearer`; sent as `Authorization: Bearer <access_token>` on
  every API call. `expires_in` = 2592000s (**30 days**). Response carries
  `access_token`, `refresh_token`, `scope`, `token_type`, `created_at`.
- **Refresh:** `grant_type=refresh_token` + `client_id` + `client_secret`;
  **refresh tokens rotate** — using one revokes the prior access+refresh pair,
  so the token gateway MUST persist the newly-returned `refresh_token` on every
  refresh (standard_oauth's refresh-and-write-back path handles this; L4
  exercises it via a short seeded `expires_at`).
- **Scopes:** `read_user_profile` + `app_scopes` auto-added; request the
  resource scopes the surface needs (`contacts_search`, `person_read`,
  `opportunity_write`, plus contacts/sequences/tasks write scopes). Scopes are
  locked at registration; editing requires re-authorizing.
- **Registration = the oauth_review gate (confirmed):** apps are registered
  in-product (Settings > Integrations > API Keys > OAuth registration) and go
  live only after Apollo **approves** the registration ("Once registration is
  approved, you can use the Apollo playground…"), positioned for partners.
  This is exactly the `oauth_review` shape: dev/test app creation front-runs
  and gates L4; **approval gates only the visible flip**, never dev/L4/merge.

**Bundle credential fields:**
```
credential.fields:
  access_token: token.access_token
  account_key:  connection.account_key
```
Client id/secret never live in the bundle — they land in integration-service
config (`config/` + `deploy/` Helm Secret together, Config Sync rule) as
`oauth.client_id` / `oauth.client_secret` (lane-1 owned).

**Identity:** `source: userinfo`, separate GET
`https://app.apollo.io/api/v1/users/api_profile` with the bearer token
(the doc's designated "who owns this token" endpoint). `stable_key` and
`label_candidates` (`/email`, `/name`) are set from the `api_profile`
response — exact JSON pointers confirmed at L2 against a live response.

**anycli injection:** definition binds `access_token` → env
`APOLLO_ACCESS_TOKEN`; the service sends `Authorization: Bearer
$APOLLO_ACCESS_TOKEN`. anycli stays credential-agnostic (no OAuth knowledge).

## 4. Helio provider bundle plan

Three axes (no divergence → **no** `toolToProvider` entry; identity holds):

- **① CLI command word:** `apollo` (flat top-level `heliox tool apollo …`;
  not a grouped family).
- **② anycli tool id:** `apollo` (`definitions/tools/apollo.json`).
- **③ provider catalog key:** `apollo` (`integrations/providers/apollo/`).

`provider.yaml` (hidden-first, `standard_oauth`, modeled on `notion` +
`google_calendar` OAuth bundles):

```yaml
schema: helio.provider/v1
key: apollo
go_name: Apollo
presentation:
  name: Apollo
  description_key: apollo
  consent_domain: app.apollo.io
  visible: false            # hidden-first; flip is the single go-live change
  order: <next free>
auth:
  type: oauth
  owner: individual         # provider sees a person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.apollo.io/#/oauth/authorize   # §7 fragment form
    token_url: https://app.apollo.io/api/v1/oauth/token
    token_exchange_style: form_secret     # client_id/secret in the form body, no Basic
    pkce: none
    scopes: [read_user_profile, contacts_search, person_read, opportunity_write, …]
    display_scopes: [read_user_profile, contacts_search, person_read, opportunity_write]
    single_active_token: false
    refresh_lease: none
identity:
  source: userinfo
  url: https://app.apollo.io/api/v1/users/api_profile
  stable_key: /id            # confirm pointer at L2
  label_candidates: [/email, /name]
connection:
  mode: isolated
  disconnect_mode: local_only   # Apollo publishes no token-revocation endpoint; revoke is rotation-on-refresh
  runtime_strategy: standard_oauth
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: apollo
  kind: oauth
```

**Service-side Go: none expected.** Apollo is a standard authorization-code +
rotating-refresh provider with a separate-GET userinfo identity — squarely
inside the `standard_oauth` capability set (`standardOAuthExchanger` +
`declarativeIdentityResolver`). No `service/adapter_apollo.go`. The one thing
to re-confirm at L3 is that the generator's HTTPS-authorize-URL validation and
the exchanger accept the fragment-form authorize URL (§7); if not, that is a
narrow generator/validator fix, not a per-provider adapter.

Also required (batch-end shared surfaces): UI icon
`ui/helio-app/src/integrations/icons/apollo.svg` + `providerIcons.ts` append;
AI-facing sub-doc under `agents/plugins/heliox/skills/tool/`; the five
`provider-gen` projections regenerated in one commit.

## 5. Test plan — five layers

| Layer | What it proves for Apollo | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes for each command group: request path/method/body, `Authorization: Bearer` injection, pagination passthrough, both plain + `--json` error rendering. No real API. | No |
| **L2** `anycli apollo -- <args>` harness, `ANYCLI_CRED_ACCESS_TOKEN=…` | Real `api.apollo.io` calls prove field names, Bearer injection, and response shapes match live. **Critical Apollo-specific gate:** prove which groups an **OAuth** token can actually reach vs. master-key-only (§7). Groups OAuth cannot reach are dropped before the bundle ships. | **Yes** — an Apollo account + a real OAuth access token (from the dev app) or master API key for a first-pass shape check |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle strict-decodes; closed field/enum contract; unique names; directory==key; HTTPS authorize/token URLs (incl. fragment form); `helio-cli` build with local `replace` → anycli branch. | No |
| **L4** singleton + `POST /internal/test-only/connections/seed` + `heliox tool apollo -- …` | Seed `access_token`+`refresh_token` with a short `expires_at` so the next call forces the gateway's rotating-refresh write-back (A3), then a real command reaches Apollo. Runs against the hidden tool. | **Yes** — a real Apollo OAuth token pair from the dev app (lane 1); seeded onto a real local assistant/org identity |
| **L5** `tool auth` → connect link → real Apollo consent → unseeded run | Validates the actual connect UX: authorize (fragment URL), callback, `api_profile` identity extraction, `oauth_connected` event, one unseeded live call. Once, while hidden, before the visible flip. | **Yes** — a real Apollo account for human-in-the-loop consent (oauth L5 is lane-3 human; Apollo app must be **approved** far enough to run the consent flow, and review clearance gates the flip) |

## 6. Rollout

Land hidden (`visible: false`) in the Wave-2 batch; L1–L4 green while hidden;
dev-mode app registration front-runs (gates L4). Flip `visible: true` +
regenerate as the single go-live change **only after** (a) L5 round-trip and
(b) Apollo's OAuth-registration **approval** clears. Track approval/flip state
in the wave board.

## 7. Divergences from the prompt/catalog (verified against official docs)

1. **oauth_review lane — confirmed, no change.** Registration requires Apollo
   approval and is partner-positioned; matches the audit verdict exactly.
2. **OAuth scope coverage < master API key (material).** Apollo's own docs
   state OAuth "supports only the API endpoints listed in Apollo's public API
   documentation," and several high-value endpoints — People Search
   (`/mixed_people/api_search`) and sequence add/remove
   (`/emailer_campaigns/{id}/add_contact_ids`, `…/remove_or_stop_contact_ids`)
   — are documented as **master-API-key-only (403 otherwise)**. Whether an
   OAuth token with the right scopes reaches them is **not** guaranteed by the
   docs. This is the stage-1/L2 feasibility gate: if OAuth cannot reach a
   group, that group is dropped from the shipped surface (fail-fast, no silent
   API-key fallback). The enrich/contacts/tasks/deals/users core is expected
   OAuth-reachable and is the guaranteed-shippable floor.
3. **Authorize URL is a SPA fragment form** (`https://app.apollo.io/#/oauth/
   authorize`) rather than a plain query-string authorize endpoint. It is
   HTTPS (passes the generator gate) but the `standardOAuthExchanger`'s
   authorize-URL construction and the generator validator must be confirmed to
   handle the `#/…` fragment at L3/L5. Flag at stage 1; likely a small generic
   fix, not an adapter.
4. **Refresh-token rotation** — Apollo revokes the prior token pair on every
   refresh. Not a lane change, but a hard requirement on the gateway's
   write-back (verified live at L4). Recorded so it is not missed.
