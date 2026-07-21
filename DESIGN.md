# Tool design: HubSpot (`hubspot`)

**Batch scratch doc** — lives on branch `tool/hubspot`; the batch lead strips it at batch-end.
**Catalog row:** #16 · Product HubSpot · anycli id `hubspot` · provider key `hubspot` · auth lane `oauth_review` · Wave 1 · CRM.
**Date:** 2026-07-21.

## 0. Naming (master plan §3)

| Axis | Value | Notes |
|---|---|---|
| ① CLI command word | `hubspot` | flat command, no `tool.group`; `heliox tool hubspot -- …` |
| ② anycli tool id | `hubspot` | `definitions/tools/hubspot.json`, `RegisterService("hubspot", …)` |
| ③ provider catalog key | `hubspot` | `integrations/providers/hubspot/` |

② == ③ (identical strings, no dash/underscore divergence) → **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go`. Go package: `internal/tools/hubspot/`.

## 1. Auth-lane verification (independent, against official docs)

The catalog says `oauth_review`. HubSpot is absent from the oauth-audit file (the audit only
covered the 250 pre-audit `api_key` rows; HubSpot was laned `oauth_review` from the seed).
Verified independently:

- HubSpot public apps authenticate exclusively via OAuth 2.0 authorization-code; there is no
  PKCE for general APIs (community/staff confirmation, 2026-03; PKCE exists only for the
  separate MCP-server OAuth 2.1 path, which is out of scope here).
  Sources: https://developers.hubspot.com/docs/api-reference/latest/authentication/manage-oauth-tokens ,
  https://community.hubspot.com/t5/APIs-Integrations/OAuth2-Proof-Key-for-Code-Exchange-PKCE-Support/m-p/1255824
- **Review gate is real, but it gates scale, not function.** On Developer Platform 2025.2
  (mandatory for new apps — legacy public-app creation was sunset 2026-06-23), a
  marketplace-distribution public app is **capped at 25 installs until it passes App
  Marketplace listing review**; approval removes the cap. Unlisted installs also show a
  "not reviewed by HubSpot" trust banner (users must type "I accept the risk" unless the
  developer domain is DNS-verified).
  Sources: https://developers.hubspot.com/changelog/new-marketplace-distribution-app-install-limits ,
  https://developers.hubspot.com/docs/apps/legacy-apps/public-apps/overview
- **Verdict: `oauth_review` confirmed** — dev-mode/test-portal installs (and the whole L1–L5
  pass) work pre-review under the 25-install cap, exactly the "dev unaffected, visible flip
  gated on review clearance" model. No divergence from the catalog to record. Lane-1 note:
  submit the Marketplace listing early; also do the optional DNS domain verification to
  drop the consent-screen risk banner during the review window.

## 2. What an AI teammate does with HubSpot → API surface

HubSpot is the CRM system of record. The high-value teammate loops are:

1. **Look up context before acting** — "who is jane@acme.com, what company, which open
   deals, what happened last?" → contacts/companies/deals/tickets read + search +
   associations.
2. **Keep the CRM current** — create/update contacts, companies, deals (stage moves),
   tickets after a call/email/meeting.
3. **Log work and create follow-ups** — notes on records, tasks with due dates/owners.
4. **Route and report** — resolve owners for assignment, read pipelines/stages to interpret
   and set deal state, list properties to discover the portal's schema (portals are heavily
   customized; hardcoding property names fails).

Wrapped surface — **CRM v3 objects API family** (stable, fully supported; HubSpot's new
date-versioned `/crm/objects/2026-03/...` endpoints exist but v3 remains the supported
mainline and matches all reference material):

| Area | Endpoints |
|---|---|
| Objects CRUD (contacts, companies, deals, tickets) | `GET/POST /crm/v3/objects/{type}`, `GET/PATCH/DELETE /crm/v3/objects/{type}/{id}` (+ `idProperty=email` lookup for contacts) |
| Search | `POST /crm/v3/objects/{type}/search` (filterGroups, query, sorts, properties, limit/after paging) |
| Engagements: notes, tasks | `POST/GET/PATCH/DELETE /crm/v3/objects/notes|tasks[/{id}]`, `POST …/search`; `hs_timestamp` required on create; associations inline via `associations[]` (HUBSPOT_DEFINED typeIds) |
| Associations | v4: `PUT /crm/v4/objects/{fromType}/{fromId}/associations/default/{toType}/{toId}`, `GET /crm/v4/objects/{fromType}/{fromId}/associations/{toType}`, `DELETE` same shape |
| Owners | `GET /crm/v3/owners`, `GET /crm/v3/owners/{id}` |
| Pipelines | `GET /crm/v3/pipelines/{objectType}` (deals, tickets) |
| Properties | `GET /crm/v3/properties/{objectType}`, `GET /crm/v3/properties/{objectType}/{name}` |
| Account | `GET /account-info/v3/details` (portal id — whoami/smoke) |

Deliberately **out of v1 scope**: calls/meetings/emails engagement logging (needs
`sales-email-read` redaction rules and heavier association semantics), lists membership
management, custom objects, files, marketing/CMS APIs. All are additive later without
breaking the command shape.

Rate limits (design note only; no client-side throttling in v1, matching all precedents):
OAuth-app requests are limited per portal (~100–110 req/10s tier-dependent; search
endpoints ~5 req/s; daily portal cap). anycli surfaces HubSpot's 429 JSON as a normal
`--json` error envelope; the brain retries.

## 3. anycli definition + service implementation

**Stage-1 rubric: `service` type.** The official CLI (`@hubspot/cli`) is a
developer/CMS-build tool (projects, themes, serverless functions), not a CRM data CLI; it
does not cover the surface above. No agent-friendly official binary → service type, like
21 of 23 existing definitions.

`definitions/tools/hubspot.json`:

```json
{
  "name": "hubspot",
  "type": "service",
  "description": "HubSpot CRM as a tool (OAuth access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "HUBSPOT_ACCESS_TOKEN"}
      }
    ]
  }
}
```

`internal/tools/hubspot/` — copy the notion/bitly shape: `Service{BaseURL, HC, Out, Err}`
(BaseURL default `https://api.hubapi.com`, overridable for httptest), duck-typed
`Execute(ctx, args, env)`, registered in `internal/tools/register.go` init
(`RegisterService("hubspot", &hubspot.Service{})` — registry merge rides the batch-end
merge; the package + definition JSON merge freely mid-batch).

Cobra tree (resource-grouped, non-interactive, flags only):

```
hubspot account                                  # GET /account-info/v3/details (whoami / L2 smoke)
hubspot contact  get <id> [--by-email] | list | create | update <id> | delete <id> | search
hubspot company  get <id> | list | create | update <id> | delete <id> | search
hubspot deal     get <id> | list | create | update <id> | delete <id> | search
hubspot ticket   get <id> | list | create | update <id> | delete <id> | search
hubspot note     create | get <id> | list | update <id> | delete <id>
hubspot task     create | get <id> | list | update <id> | complete <id> | delete <id>
hubspot assoc    create <fromType> <fromId> <toType> <toId> | list <fromType> <fromId> <toType> | delete <fromType> <fromId> <toType> <toId>
hubspot owner    list [--email] | get <id>
hubspot pipeline list <objectType>               # deals | tickets, includes stages
hubspot property list <objectType> | get <objectType> <name>
```

Shared flag conventions:
- `--prop key=value` (repeatable) for create/update property maps; `--properties a,b,c`
  for read projections (HubSpot returns only defaults otherwise).
- `search`: `--query <text>`, `--filter property:operator:value` (repeatable; one
  filterGroup AND semantics in v1), `--sort prop[:desc]`, `--limit`, `--after`.
- `note create` / `task create`: `--body/--subject/--due/--owner`, and
  `--contact/--company/--deal/--ticket <id>` (repeatable) mapping to inline
  `associations[]` with the documented HUBSPOT_DEFINED type ids (e.g. task→contact 204,
  note→contact 202) held in one package-level table.
- `hs_timestamp` is defaulted to now on `note create` when not given (HubSpot requires it).

Output contract = notion precedent: provider JSON passthrough on stdout + newline; exit
codes 0 success / 1 runtime-API failure (typed apiError from HubSpot's
`{"status":"error","message",...,"correlationId"}` body) / 2 usage errors; `--json` yields
the structured error envelope. 401/`EXPIRED_AUTHENTICATION` renders the standard
auth-error hint (credential invalid → reconnect), matching `auth_error.go` precedents.

## 4. Credential fields and OAuth flow (verified against official docs)

**Registration model:** one Helio-owned **public app** in a HubSpot developer account
(Developer Platform 2025.2, marketplace distribution). Lane 1 creates it, sets redirect
URI to integration-service's callback, configures the scope set below, and distributes
client id/secret as uncommitted local `config/cloud.yaml` entries for on-branch L4;
committed config lands per Config Sync (`config/` + Helm Secret in `deploy/` together).

**Flow (authorization code, no PKCE):**
- Authorize: `https://app.hubspot.com/oauth/authorize?client_id&redirect_uri&scope&state`.
  The `scope` param must exactly cover the app's required scopes; the installing user must
  have access to every required scope or install fails (Super Admin fallback).
- Token: `POST https://api.hubspot.com/oauth/2026-03/token`, form-encoded body with
  `grant_type=authorization_code|refresh_token`, `client_id`, `client_secret` — i.e.
  **`token_exchange_style: form_secret`**, RFC 6749-conformant responses. (The legacy
  `https://api.hubapi.com/oauth/v1/token` is deprecated-but-operational; new integrations
  are told to use the versioned endpoint. The 2026-03 response also returns `hub_id` and
  `scopes`, which we use for identity.)
- **Token semantics:** access token expires in 1800 s (`expires_in`); refresh token is
  long-lived, **non-rotating**, and dies only on app uninstall / scope change / manual
  revoke → seedable refresh cycle, `single_active_token: false`, `refresh_lease: none`.
- **Revoke:** no RFC 7009 endpoint (v1 has `DELETE /oauth/v1/refresh-tokens/{token}`,
  path-embedded — outside the declarative revoker's POST shape) →
  `disconnect_mode: local_only`; users additionally uninstall from HubSpot's Connected
  Apps page (which invalidates the refresh token provider-side).

**Scopes** (granular model; pinned at app registration, re-verified at lane-1 time against
https://developers.hubspot.com/scopes):

- Required: `oauth` (always present), `crm.objects.contacts.read/.write`,
  `crm.objects.companies.read/.write`, `crm.objects.deals.read/.write`,
  `crm.objects.owners.read`, `crm.schemas.contacts.read`, `crm.schemas.companies.read`,
  `crm.schemas.deals.read`, `tickets` (the documented tickets scope; engagement
  notes/tasks endpoints are covered by the contacts scopes per the official tasks guide).
- No sensitive/highly_sensitive scopes (Enterprise-gated, raise review bar, not needed).

**Credential field contract:** single `access_token` (bundle
`credential.fields: access_token: token.access_token`, plus `account_key`), delivered to
anycli as env `HUBSPOT_ACCESS_TOKEN`. Refresh is entirely token-gateway-side.

## 5. Helio provider bundle plan

`integrations/providers/hubspot/provider.yaml` (hidden-first; five projections regenerate
at batch end only — local `provider-gen` runs for validation are not committed):

```yaml
schema: helio.provider/v1
key: hubspot
go_name: HubSpot

presentation:
  name: HubSpot
  description_key: hubspot
  consent_domain: hubspot.com
  visible: false            # flip = single go-live change after L5 + marketplace review
  order: <batch-assigned>

auth:
  type: oauth
  owner: assistant          # portal-level install (Notion/Slack workspace precedent)
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.hubspot.com/oauth/authorize
    token_url: https://api.hubspot.com/oauth/2026-03/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [oauth, crm.objects.contacts.read, crm.objects.contacts.write,
             crm.objects.companies.read, crm.objects.companies.write,
             crm.objects.deals.read, crm.objects.deals.write,
             crm.objects.owners.read, crm.schemas.contacts.read,
             crm.schemas.companies.read, crm.schemas.deals.read, tickets]
    display_scopes: [contacts, companies, deals, tickets, owners, schemas]
    single_active_token: false
    refresh_lease: none

identity:
  source: token_response    # 2026-03 token response carries hub_id
  stable_key: /hub_id
  label_candidates: [/hub_id]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: standard_oauth

resources: {selection: none, discovery: none, enforcement: none}

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: hubspot
  kind: oauth
```

**Known blocker to resolve during dev (recorded divergence from "zero service code"):**
HubSpot's stable identity key (`hub_id` in the token response; `portalId` in
`GET /account-info/v3/details`) is a **JSON number**, and
`integration-service/service/declarative_identity.go` `jsonPointerString` accepts only
string values — `/hub_id` fails identity resolution today. Per the skill's guidance
("grow the generic capability set before reaching for an adapter"), the plan is a small
generic integration-service change: canonical stringification of JSON numbers in
stable-key/label pointer extraction (integral float64 → decimal string), with unit tests.
This benefits every future numeric-id provider. Fallback if that change is rejected in
review: a narrow `service/adapter_hubspot.go` using
`POST /oauth/2026-03/token/introspect` (which would also upgrade the connection label to
`hub_domain` / user email — noted as an optional follow-up either way; with the generic
path the label is the bare portal id).

Other Helio-side items: icon `ui/helio-app/src/integrations/icons/hubspot.svg` +
`providerIcons.ts` entry (manual, batch-end); AI-facing sub-doc
`agents/plugins/heliox/skills/tool/hubspot.md` (batch plugin bump/publish); no
`experiment` flag (Wave-1 GA path, hidden-first only); no `toolToProvider` entry (§0).

## 6. Test plan (five layers)

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes for every subcommand — request path/method/body shape, `Authorization: Bearer` injection from `HUBSPOT_ACCESS_TOKEN`, property-map and `--filter` parsing, inline `associations[]` construction (type-id table), pagination `after` passthrough, HubSpot error body → exit 1 + `--json` envelope, 401 auth hint, usage → exit 2. TDD: tests first per anycli AGENTS.md. | none |
| L2 | Dev harness against the **real** API: `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli hubspot -- account`, then contact search/create/update, deal + pipeline read, note/task create with associations on the test portal. Token minted from the lane-1 dev app install (or a private-app token from the test portal for early smoke — same header semantics). | **yes** — test-pool HubSpot portal + lane-1 dev app (or private-app token) |
| L3 | Helio side, on-branch: local `provider-gen` + `--check` against the bundle (not committed); `helio-cli` built with local uncommitted `go.mod` `replace` → anycli worktree; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service unit suite incl. the numeric-pointer identity change + a HubSpot-shaped token-response identity test. | none |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with **real** `access_token` + `refresh_token` from the dev-app install and a deliberately short `expires_at`, forcing the token-gateway refresh path on first `heliox tool hubspot -- account` / `contact search`. Requires lane-1 dev client id/secret as uncommitted local `config/cloud.yaml` entries. | **yes** — real portal tokens + dev app client id/secret |
| L5 | Once, pre-flip, human-in-the-loop (oauth lane): `heliox tool hubspot auth` → connect link → HubSpot account-picker + consent (expect the unreviewed-app banner / "I accept the risk" until domain verification or listing) → `oauth_connected` event on the channel → unseeded live command. Visible flip additionally gated on **Marketplace listing review clearance** (§1). | **yes** — human with test-portal Super Admin login |

Definition of done follows the master plan: L1–L4 green on-branch, batch-end merge
(registry, pin bump, one regen, icon, docs publish), L5 sweep, then
`visible: true` + regenerate as the single go-live change after HubSpot review clears.
