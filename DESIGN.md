# Zoho CRM — per-tool design (batch scratch file)

**Tool:** Zoho CRM · anycli id `zoho-crm` · provider key `zoho_crm` · catalog row 61 (Wave 2, CRM) · auth lane `oauth_light`.
**Branches:** anycli `tool/zoho-crm` (this worktree) + Helio `tool/zoho-crm`.
**Status:** design. This file is stripped by the batch lead at batch end; durable facts land in the provider bundle, the anycli definition/tests, and the heliox plugin sub-doc.

All provider facts below were verified against Zoho's official docs on 2026-07-21:

- OAuth overview / register client / auth request / token / refresh / revoke:
  `zoho.com/crm/developer/docs/api/v8/{oauth-overview,register-client,auth-request,access-refresh,refresh,revoke-tokens}.html`
- Scopes: `zoho.com/crm/developer/docs/api/v8/scopes.html`
- Records / search / insert: `zoho.com/crm/developer/docs/api/v8/{get-records,search-records,insert-records}.html`
- Header schemes: the CRM data plane (`zohoapis.com/crm/v8`) requires `Authorization: Zoho-oauthtoken <t>` (CRM V8 docs); the accounts server's userinfo endpoint (`accounts.zoho.com/oauth/user/info`) additionally accepts `Authorization: Bearer <t>` per the Zoho Accounts OAuth protocol doc (`zoho.com/accounts/protocol/oauth/use-access-token.html`, whose Bearer example is shown against that accounts endpoint). These are different hosts — Bearer is not assumed to work on the CRM host.

## 0. Audit-verdict note (divergence log)

The 2026-07-21 OAuth audit (`docs/design/008-300-integrations-rollout-plan/oauth-audit.md`) has **no row for Zoho CRM** — its scope was only the 250 tools that sat in the `api_key` lane pre-audit, and Zoho CRM was `oauth_light` from the seed catalog. The lane was therefore re-verified from official docs for this design:

- **`oauth_light` is confirmed.** Client registration at `api-console.zoho.com` is fully self-serve (server-based client type; credentials issued immediately; no review/approval gate documented — only input validation errors). One registered client is authorized by arbitrary Zoho accounts via a standard OAuth2 authorization-code flow.
- **One real divergence found (not lane-affecting): multi-DC.** See §3.4 — Zoho is multi-datacenter, and the token endpoint is DC-specific. V1 is scoped to the US DC; this is recorded here rather than inherited from any catalog assumption.

## 1. What an AI teammate does with Zoho CRM → API surface

The tool wraps the **Zoho CRM REST API v8** (current major version) at `https://www.zohoapis.com/crm/v8`. Driving use cases, in priority order:

1. **Look up a person/company/deal before or after a conversation** — "what do we know about jane@acme.com?" → Search Records (`GET /crm/v8/{module}/search?email=…|criteria=…|word=…`).
2. **Capture new leads/contacts from conversations** — Insert Records (`POST /crm/v8/{module}`, body `{"data":[{…}]}`).
3. **Keep deals current** — Update Records (`PUT /crm/v8/{module}/{id}`), e.g. stage/amount/close-date changes.
4. **Log context on records** — Notes subresource (`GET/POST /crm/v8/{module}/{id}/Notes`).
5. **Answer pipeline questions** — List Records (`GET /crm/v8/{module}?fields=…`) and COQL (`POST /crm/v8/coql`, `{"select_query":"…"}`) for precise filtered/aggregated reads (also the workaround for search-index lag: Zoho documents that fresh writes may 204 in search; COQL reads them immediately).
6. **Discover field API names before create/update** — Settings metadata (`GET /crm/v8/settings/modules`, `GET /crm/v8/settings/fields?module=…`). Zoho create/update bodies are keyed by field *API names* (`Last_Name`, not "Last Name"), so metadata discovery is a hard prerequisite for reliable writes by an agent, not a nice-to-have.
7. **Ownership/assignment context** — Users (`GET /crm/v8/users`, incl. `?type=CurrentUser`) and Org (`GET /crm/v8/org`).

Explicitly **out of scope for V1** (thin value per line of code, or separate review surfaces): Bulk Read/Write jobs, file/attachment upload, blueprints/workflows administration, email sending, Cadences, timeline/audit APIs, Territories, webhooks/notifications (channel subscription needs a callback surface anycli doesn't have).

V8 contract details that shape the CLI (verified):

- List records: `fields` param is **mandatory** (max 50 field API names); `per_page` default/max 200; `page` covers only the first 2,000 records, then `page_token`/`next_page_token` takes over (token valid 24 h, cannot combine with `page`); `sort_by` ∈ {`id`,`Created_Time`,`Modified_Time`}.
- Search: exactly one of `criteria` / `email` / `phone` / `word` (priority order if several); optional `fields`; hard cap 2,000 results (`LIMIT_REACHED`); needs the extra scope `ZohoSearch.securesearch.READ` on top of the module scope (missing it → `OAUTH_SCOPE_MISMATCH` 401).
- Insert: `{"data":[…]}`, max 100 records/call, per-record status objects, HTTP 207 on mixed outcomes; optional `trigger` array (pass `[]` to suppress workflows — exposed as a flag, default = provider default, i.e. automations run).
- Auth header: the service sends `Authorization: Zoho-oauthtoken <access_token>` — the scheme the CRM V8 docs require on every `zohoapis.com/crm/v8` call. The Helio-side declarative identity probe does **not** hit the CRM host; it hits the accounts-server userinfo endpoint, which accepts `Bearer` (§3.3). Do not conflate the two hosts.

## 2. anycli definition & service

### 2.1 Stage-1 form decision: `service`

`cli` type fails the rubric: Zoho's official CLI is for Catalyst/serverless development, not CRM data operations; there is no official, agent-friendly, `--json` CRM binary to wrap. → `service` type in `internal/tools/zohocrm/` (dashes dropped per the §3 naming rule; matches the `microsoftcalendar` precedent).

### 2.2 Definition `definitions/tools/zoho-crm.json`

```json
{
  "name": "zoho-crm",
  "type": "service",
  "description": "Zoho CRM as a tool (OAuth user access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "ZOHO_CRM_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single binding, `<TOOL>_ACCESS_TOKEN` naming per the `calendar`/`microsoft-outlook`/`bitly` precedents. No `api_domain` binding in V1 (US-DC scoping, §3.4); the service's `BaseURL` zero-value defaults to `https://www.zohoapis.com` and stays injectable for tests and for the future multi-DC field (missing credential fields resolve to empty and are skipped at injection — `internal/credential/resolve.go` + `ApplyBindings` — so adding an optional `api_domain` binding later is non-breaking).

Registration: `RegisterService("zoho-crm", &zohocrm.Service{})` in `internal/tools/register.go` — **batch-end merge only** (shared surface); the definition JSON and `internal/tools/zohocrm/` merge freely mid-batch.

### 2.3 Cobra tree (resource-grouped, notion-style)

```
zoho-crm record list    --module <api_name> --fields a,b,c [--page N|--page-token T] [--per-page N] [--sort-by id|Created_Time|Modified_Time] [--sort-order asc|desc]
zoho-crm record get     --module M --id ID [--fields ...]
zoho-crm record create  --module M --data '<json object or array>' [--no-triggers]
zoho-crm record update  --module M --id ID --data '<json object>' [--no-triggers]
zoho-crm record delete  --module M --id ID [--ids id1,id2,...]
zoho-crm record search  --module M (--criteria '(F:op:v)…' | --email E | --phone P | --word W) [--fields ...] [--page N] [--per-page N]
zoho-crm query          --coql 'select Last_Name from Leads where ...'
zoho-crm note list      --module M --id ID
zoho-crm note add       --module M --id ID --title T --content C
zoho-crm module list
zoho-crm field list     --module M
zoho-crm user list      [--type AllUsers|ActiveUsers|CurrentUser|...]
zoho-crm user me        (sugar for user list --type CurrentUser)
zoho-crm org get
```

Design points:

- `--module` takes the module **API name** (`Leads`, `Contacts`, `Accounts`, `Deals`, `Tasks`, `Events`, `Calls`, custom `*__c`-style names) and is passed through verbatim — no client-side allowlist, so custom modules work day one.
- `record list` enforces `--fields` as required (the API mandates it); the error message tells the agent to run `zoho-crm field list --module M` first. `--page` and `--page-token` are mutually exclusive (API contract).
- `record search` enforces exactly one of the four search selectors, mirroring the API's priority rule as a hard CLI error instead of silent precedence.
- `--data` accepts a JSON object (single record) or array (bulk ≤100); the service wraps it into `{"data":[…]}`. `--no-triggers` maps to `"trigger": []`.
- Output/exit contract per design 003 §3 and the notion precedent: success prints the provider's JSON response verbatim to stdout; failure is exit 1 with a one-line stderr error carrying Zoho's `code`/`message` (typed `apiError`), usage errors exit 2; `--json` gives the structured error envelope. HTTP 207 (per-record mixed status on bulk writes) is surfaced as success output (the per-record statuses are in the body) — agents inspect per-record `code`.
- Struct: `Service{ BaseURL string; HC *http.Client; Out, Err io.Writer }`, zero values → production endpoint, per the built-in service conventions.

### 2.4 anycli tests (TDD, L1)

Write tests first per anycli AGENTS.md. `httptest` fakes assert, per subcommand: URL path/method/query (incl. `fields` propagation, `page` vs `page_token` exclusivity), `Authorization: Zoho-oauthtoken` header injection from `ZOHO_CRM_ACCESS_TOKEN`, `{"data":[…]}` body wrapping and `trigger:[]` mapping, COQL body shape, and both plain + `--json` error rendering of a Zoho error body (`{"data":[{"code":"INVALID_DATA",…}]}` and top-level `{"code":"OAUTH_SCOPE_MISMATCH",…}` shapes — Zoho uses both). Plus: missing credential → exit 1 explicit message; usage errors → exit 2.

## 3. Credentials & OAuth (verified against official docs)

### 3.1 Flow

Standard OAuth2 authorization-code, server-based client, self-serve at `api-console.zoho.com`:

- Authorize: `https://accounts.zoho.com/oauth/v2/auth` with `response_type=code`, `access_type=offline` (**required** to get a refresh token), scopes comma-separated. `prompt=consent` forces re-consent / fresh refresh token on reconnect (Zoho supports it though the v8 auth-request page doesn't list it; keep it — precedent: google_calendar bundle).
- Grant code: single-use, ~1–2 min validity; callback carries `code`, `location`, `accounts-server`.
- Token: `POST {accounts_URL}/oauth/v2/token`, **form body** with `grant_type=authorization_code`, `client_id`, `client_secret`, `redirect_uri`, `code` → matches `token_exchange_style: form_secret` exactly.
- Response: `{access_token, refresh_token, api_domain, token_type:"Bearer", expires_in:3600}`.
- Refresh: `POST {accounts_URL}/oauth/v2/token` with `grant_type=refresh_token` — **no rotation**: refresh response returns a new access token only, the refresh token is permanent until revoked (same shape as Google; the standard_oauth refresh path already handles keep-old-refresh-token).
- Revoke: `POST {accounts_URL}/oauth/v2/token/revoke?token={refresh_token}` — revokes the refresh token (org-specific).
- Token semantics: access token 1 h; tokens are organization- and environment-specific (production vs sandbox vs developer). A user with multiple CRM orgs picks one at consent time — the connection is to one org, which is fine under `connection.mode: isolated`.

### 3.2 Scopes

```
ZohoCRM.modules.ALL          # record CRUD on all modules incl. Notes subresource
ZohoSearch.securesearch.READ # required by GET /{module}/search (401 OAUTH_SCOPE_MISMATCH without it)
ZohoCRM.coql.READ            # POST /coql
ZohoCRM.settings.modules.READ
ZohoCRM.settings.fields.READ
ZohoCRM.users.READ           # user list + assignment context (`user list` / `user me`)
ZohoCRM.org.READ             # org get
AaaServer.profile.READ       # identity probe: accounts.zoho.com/oauth/user/info
```

`ZohoCRM.modules.ALL` is deliberate (vs per-module scopes): the tool is module-generic including custom modules, and per-module scopes would break `--module <custom>`. Operation types are `ALL/CREATE/READ/UPDATE/DELETE` (no `WRITE`).

### 3.3 Identity

`identity.source: userinfo` against `https://accounts.zoho.com/oauth/user/info` (accounts server, query-free):

- Why the accounts endpoint, not the CRM one: the declarative resolver fetches userinfo with `Authorization: Bearer <token>` (`oauth_exchange.go` `fetchUserInfo`). Zoho's CRM V8 docs require the custom `Zoho-oauthtoken` prefix on every `zohoapis.com/crm/v8` call, so the CRM `/crm/v8/users?type=CurrentUser` endpoint is **not** usable by the Bearer-sending declarative resolver — and it would need a query on the userinfo URL. The accounts-server `/oauth/user/info` endpoint, by contrast, officially accepts `Bearer` (Zoho Accounts "Using Access token" doc, whose Bearer example runs against this exact endpoint) and takes no query. Requires the `AaaServer.profile.READ` scope.
- Response: `{"First_Name":…,"Last_Name":…,"Email":"<string>","Display_Name":"<string>","ZUID":<number>}` → `stable_key: /Email`, `label_candidates: [/Display_Name, /Email]`. `ZUID` is the natural person id but it is a JSON **number**, and `jsonPointerString`/the resolver require a **string** value; `Email` is the only string identity field, so it is the stable key. (Switching the stable key to `ZUID` would need generic numeric-stable-key support in the resolver — out of scope here, tracked separately with the HubSpot row.)
- No adapter and no service-side capability growth: identity stays fully declarative, and the earlier query-permitting userinfo validator is dropped (zoho_crm was its only consumer).
- Note: `Email`/`ZUID` are account-level (one Zoho user), not org-scoped, so the same human connecting two orgs yields **one** account key. That is fine under `connection.mode: isolated` (tokens remain org-specific). Trade-off: a Zoho email change would mint a new account key on reconnect — an accepted limitation for the hidden-first beta.

### 3.4 Multi-DC — the one real divergence (V1 scoping decision)

Zoho runs isolated DCs (`.com`, `.eu`, `.in`, `.com.au`, `.jp`, `.com.cn`, `zohocloud.ca`, `.sa`). Verified contract: the authorize endpoint auto-redirects the user's login to their home DC; the callback's `accounts-server` param names the DC-specific accounts host, and **grant/access/refresh tokens must all be generated against that host** — exchanging an EU code at `accounts.zoho.com` fails with `invalid_client` (domain mismatch). With the console's Multi-DC option enabled, each DC additionally gets its **own client secret**.

`standard_oauth` posts to one fixed `token_url` and serves one `oauth.client_secret`, so full multi-DC is out of its closed capability set. Decision:

- **V1 ships US-DC-pinned**: `authorize_url`/`token_url`/identity URL/service `BaseURL` all on `.com` hosts. US-DC accounts (the test-account pool and the primary beta cohort) work end-to-end; a non-US Zoho account fails at token exchange with an explicit provider error (no silent fallback, per the Hard Rules).
- **Follow-up (flag to batch lead, shared with Zoho Books row 183):** proper multi-DC needs (a) callback-`accounts-server`-directed token/refresh/revoke endpoints, (b) per-DC client secrets in config, and (c) delivering token-response `api_domain` to the tool as a credential field — `connection.metadata.*` credential sources are a closed reviewed enum (`provider-gen/validate.go: knownCredentialSources`) that today only knows `person_urn`. Per the provider-yaml guidance this should grow as a **generic reviewed capability** (it is a Zoho-family trait, not a one-off), not a per-provider adapter. Until then, US-only is the honest scope and must be stated in the AI-facing provider sub-doc.

### 3.5 Config

`auth.required_config_fields: [oauth.client_id, oauth.client_secret]` — supplied by lane 1 in `config/` + the `deploy/` Helm Secret together (Config Sync rule); dev client id/secret arrive as uncommitted local `config/cloud.yaml` entries for the on-branch L4 run. Zoho app registration inputs for lane 1: server-based client; redirect URI = the standard integration-service callback; homepage = helio.im. No review gate; registration is minutes, not weeks.

## 4. Helio provider bundle plan

`integrations/providers/zoho_crm/provider.yaml` (held to batch-end merge; sketch):

```yaml
schema: helio.provider/v1
key: zoho_crm
go_name: ZohoCRM

presentation:
  name: Zoho CRM
  description_key: zoho_crm
  consent_domain: accounts.zoho.com
  visible: false            # hidden-first; flip is the separate go-live change
  order: <batch-lead assigns at batch end>

auth:
  type: oauth
  owner: individual         # provider sees a person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://accounts.zoho.com/oauth/v2/auth
    token_url: https://accounts.zoho.com/oauth/v2/token
    token_exchange_style: form_secret
    pkce: none
    authorize_params:
      access_type: offline   # required: yields the refresh token
      prompt: consent        # forces fresh refresh token on reconnect
    scopes: [ZohoCRM.modules.ALL, ZohoSearch.securesearch.READ, ZohoCRM.coql.READ,
             ZohoCRM.settings.modules.READ, ZohoCRM.settings.fields.READ,
             ZohoCRM.users.READ, ZohoCRM.org.READ, AaaServer.profile.READ]
    display_scopes: [modules.ALL, securesearch.READ, coql.READ,
                     settings.READ, users.READ, org.READ, profile.READ]
    single_active_token: false
    refresh_lease: none
    revoke:
      url: https://accounts.zoho.com/oauth/v2/token/revoke
      client_auth: none
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://accounts.zoho.com/oauth/user/info   # accounts server; accepts Bearer, query-free
  stable_key: /Email                                # ZUID is a JSON number; resolver is string-only
  label_candidates: [/Display_Name, /Email]

connection:
  mode: isolated
  disconnect_mode: provider_revoke
  runtime_strategy: standard_oauth   # zero service-side Go; no adapter

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: zoho-crm
  kind: oauth
```

Open items to confirm at implementation time (against generator validation, not docs): whether the `revoke` block's query-vs-form token placement matches Zoho's `?token=` expectation (Google precedent posts form `token=` and Zoho accepts the token param in the body of a POST as well — verify in L5), and the `display_scopes` shortening convention.

**Naming axes:** ① CLI word = `zoho-crm` (**flat command, no `tool.group`** — grouping `heliox tool zoho crm` is deferred per master-plan open question 2; only one other Zoho tool exists in the catalog (Zoho Books, Wave 3, a *different* Zoho OAuth app family), and a later regrouping would be a breaking command rename, so the group decision is explicitly punted to the zoho-books batch with this note as input). ② anycli id = `zoho-crm`. ③ provider key = `zoho_crm`. The ②↔③ pair is mechanical dash↔underscore: **one `toolToProvider` entry** (`"zoho-crm": "zoho_crm"`) in `helio-cli/internal/toolcred/resolver.go` at batch end — unless master-plan open question 1 (mechanical normalization in `ProviderFor`/`ToolFor`) has landed by then, in which case no entry.

**Hidden-first:** bundle lands `visible: false`; the visible flip + regen is the single go-live change after L5 (and never earlier — hidden tools are fully runnable, cobra-hidden).

**Other batch-end riders:** icon `ui/helio-app/src/integrations/icons/zoho_crm.svg` + `providerIcons.ts` append; provider sub-doc under `agents/plugins/heliox/skills/tool/` (must state the US-DC limitation and the `field list`-before-write workflow); plugin version bump + marketplace publish; anycli pin bump. **Not committed from this branch:** provider-gen regenerated projections (run locally for validation only), `helio-cli/go.mod` `replace` pointing at this anycli worktree, and dev client id/secret in local `config/cloud.yaml` — all per master plan §2.

## 5. Test plan (five layers)

| Layer | What runs for zoho-crm | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...` — `internal/tools/zohocrm/` httptest suites of §2.4 | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real US-DC token> anycli zoho-crm -- module list`, `record search --module Contacts --email …`, `record create/update/delete` round-trip on a scratch Lead, `query --coql …`, `user me` — against live `www.zohoapis.com` | **yes** — a real access token from the pool's US-DC Zoho CRM test org. Before lane 1's app exists, a Self Client grant token from the API console (self-serve, same org) mints an equivalent access token; note Self Client grant codes live ≤10 min, mint and use immediately |
| L3 | local `provider-gen` + `provider-gen --check` against the branch bundle (not committed); helio-cli build/tests with local uncommitted `go.mod` `replace` → this worktree; integration-service unit suite | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` with real `access_token` **and** `refresh_token`, deliberately short `expires_at` so the first `heliox tool zoho-crm -- user me` is forced through the token-gateway refresh path (Zoho refresh returns no new refresh token — verify write-back keeps the old one); then live-API success | **yes** — real token pair from lane 1's registered dev app (client id/secret in local uncommitted `config/cloud.yaml`); real seeded org/user/assistant identities |
| L5 | full `heliox tool zoho-crm auth` → connect link → Zoho consent (US-DC test account; multi-org account picks the test org) → `oauth_connected` event → one unseeded live run. Human-in-the-loop (lane 3), gates the visible flip. Also the point to verify the declarative revoke against Zoho's `/token/revoke` on disconnect | **yes** — lane 1 app config landed in `config/` + `deploy/`; pool US-DC account with a human driving consent |

L1 and L3 are agent-only; L2/L4/L5 require externally supplied credentials (test-account pool + lane 1's dev app) as marked.

Definition of done tracks the master plan: L1–L5 green, docs published, icon registered, then the visible flip as its own change.
