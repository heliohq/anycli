# Tool design: Copper (CRM)

Scratch design for the `copper` external tool provider. Batch-lead strips this
file at batch end. Written per the `helio-tool-provider` pipeline skill and the
298-integrations master plan (§2 execution model, §3 naming). All facts below
were verified against Copper's official Developer API docs
(`developer.copper.com`), not inherited from the catalog.

## 0. Catalog row & audit verdict (verified)

| Axis | Value |
|---|---|
| ① CLI command word | `copper` (flat; not a family group) |
| ② anycli tool id | `copper` |
| ③ provider catalog key | `copper` |
| auth lane | `oauth_review` |
| wave / batch | Wave 2, CRM |

Axes ②==③ are identical, so **no `toolToProvider` divergence entry** is added
in `helio-cli/internal/toolcred/resolver.go`. `ProviderFor("copper")` returns
`"copper"` by the identity default.

**Divergence check against official docs.** The master-plan lane (`oauth_review`)
and the audit verdict (row 63: "OAuth supported yes; oauth_review; high;
client registration not self-serve — email partners@copper.com for
client_id/client_secret") are **confirmed** by
`https://developer.copper.com/introduction/oauth/`: Copper offers a standard
authorization-code OAuth2 flow that any customer's users can authorize once the
app is registered, but registration is gated behind a manual
`partners@copper.com` request — a human/partner gate, so `oauth_review` is
correct. No divergence to record. One nuance the catalog does not capture and
this design pins down: **Copper access tokens do not expire and there is no
refresh token** (official flow doc: "access tokens do not expire and do not
need to be refreshed"). This drives `refresh_lease: none` in the bundle — the
shipped **bitly** bundle is the exact precedent (non-expiring token, no refresh,
`form_secret` exchange, `userinfo` identity), not the usual short-expiry refresh
cycle.

## 1. Which official API surface & endpoints, and why

**API:** Copper Developer API v1. Base URL `https://api.copper.com/developer_api/v1`
(the former `api.prosperworks.com` host is retired). OAuth host is a different
origin: `https://app.copper.com` (authorize + token).

**Auth on API calls (verified).** With an OAuth token, Copper requires **only**
`Authorization: Bearer {access_token}` plus `Content-Type: application/json`.
The `X-PW-AccessToken` / `X-PW-Application: developer_api` / `X-PW-UserEmail`
header trio is the **API-key** path only and is NOT used with OAuth — verified
against the official OAuth quickstart curl example
(`curl -H "Authorization: Bearer {access_token}" .../v1/account`). This matters
twice: the anycli service sends a bare Bearer header, and the Helio-side
identity resolver's `userinfo` GET works with the generic Bearer-only
`declarativeIdentityResolver` (no custom header injection needed).

**What an AI teammate actually does with a CRM** drives the endpoint selection:
find/read/create/update the core CRM records (people, companies, leads,
opportunities), manage follow-up tasks, log activities, and resolve the
lookups those need (pipelines, stages, activity types). The wrapped endpoints:

| Group | Endpoints | Why |
|---|---|---|
| account/identity | `GET /account`, `GET /users/me`, `GET /users`, `GET /users/{id}` | whoami + assignee resolution; `/users/me` is the OAuth identity source |
| people (contacts) | `POST /people/search`, `GET /people/{id}`, `POST /people/fetch_by_email`, `POST /people`, `PUT /people/{id}`, `DELETE /people/{id}` | primary contact CRUD/lookup |
| companies | `POST /companies/search`, `GET/POST/PUT/DELETE /companies[/{id}]` | account records |
| leads | `POST /leads/search`, `GET/POST/PUT/DELETE /leads[/{id}]` | top-of-funnel |
| opportunities | `POST /opportunities/search`, `GET/POST/PUT/DELETE /opportunities[/{id}]` | deals/pipeline |
| tasks | `POST /tasks/search`, `GET/POST/PUT/DELETE /tasks[/{id}]` | follow-ups |
| activities | `POST /activities/search`, `GET /activities/{id}`, `POST /activities`, `DELETE /activities/{id}` | logging notes/calls/emails |
| lookups | `GET /pipelines`, `GET /pipeline_stages`, `GET /customer_sources`, `GET /loss_reasons`, `GET /activity_types`, `GET /contact_types` | id→name resolution for create/update |

Copper list/read is a **`POST /{resource}/search`** convention (JSON body with
filters + `page_number`/`page_size`), not GET-with-query — the service models
this explicitly. Rate limit is 600 req/min per token (HTTP 429 on exceed);
surfaced as a runtime error, not retried inside the tool.

Out of scope for v1 (kept lean, add later if a teammate need emerges): projects,
webhooks/subscriptions, bulk-create endpoints, custom-field-definition
management. Bulk and custom-field *values* on records are still expressible via
the `--json-body` escape hatch below.

## 2. anycli definition

**Type: `service`** (per stage-1 rubric). No official Copper CLI exists; the
surface is a REST API. This is the default and correct choice — like `notion`,
`hubspot`, `pipedrive`, `zoho-crm` precedents. Go package
`internal/tools/copper/` (id has no dashes/leading digit, so package name ==
id).

`definitions/tools/copper.json` (minimal, mirrors `notion.json`):

```json
{
  "name": "copper",
  "type": "service",
  "description": "Copper CRM as a tool (OAuth access token)",
  "auth": {
    "credentials": [
      { "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "COPPER_ACCESS_TOKEN"} }
    ]
  }
}
```

Registered in `internal/tools/register.go`: `RegisterService("copper", &copper.Service{})`.

**Service shape** (copy `internal/tools/notion/` structure): a cobra tree with a
`Service{ BaseURL, HC, Out, Err }` struct so unit tests point at an
`httptest.Server`. Subcommand tree grouped by resource, each a
verb-per-resource:

```
copper account get
copper user me | list | get --id N
copper person   list [--filter…] | get --id N | find-email --email x | create --json-body … | update --id N … | delete --id N
copper company  list | get | create | update | delete
copper lead     list | get | create | update | delete
copper opportunity list | get | create | update | delete
copper task     list | get | create | update | delete
copper activity list | get | create | delete
copper lookup   pipelines | pipeline-stages | customer-sources | loss-reasons | activity-types | contact-types
```

Verb→endpoint mapping: `list`→`POST /{res}/search`, `get`→`GET /{res}/{id}`,
`create`→`POST /{res}`, `update`→`PUT /{res}/{id}`, `delete`→`DELETE /{res}/{id}`,
`find-email`→`POST /people/fetch_by_email`.

**Flags.** Common typed convenience flags on `list` (`--name`, `--email`,
`--assignee-id`, `--page`, `--page-size`) that the service assembles into the
search JSON body, plus a universal **`--json-body '<json>'`** escape hatch on
`create`/`update`/`list` so agents can pass any Copper field (custom fields,
address blocks, bulk filters) without the definition enumerating Copper's
entire schema. `create`/`update` require `--json-body` (or typed flags) for the
record payload.

**JSON output shape.** Pass Copper's JSON response through verbatim on stdout
(agents consume the native record shape). A global `--json` flag switches the
error channel to the structured envelope; success output is always the raw
provider JSON. Exit-code contract identical to notion: `0` success, `1`
runtime/API failure (typed `apiError` from a Copper non-2xx, carrying
status+code+message; credential-rejection 401 classified for the token
gateway), `2` usage/parse errors (bad flags, invalid `--json-body`, unknown
subcommand). Errors render plain-text by default, JSON envelope under `--json`.

## 3. Credential fields & exact auth flow

**Credential the runtime injects:** a single non-expiring OAuth bearer
`access_token`, delivered to anycli as env `COPPER_ACCESS_TOKEN`.

**OAuth flow (verified, `oauth_review` lane):**

1. **Registration (human lane 1, not self-serve):** email `partners@copper.com`
   with app name, purpose, and HTTPS callback URL → Copper issues `client_id`
   + `client_secret`. This is the review gate that makes the lane
   `oauth_review`; it gates only the **visible flip**, not dev/L4 (hidden-first).
2. **Authorize:** `GET https://app.copper.com/oauth/authorize` with
   `response_type=code`, `client_id`, `redirect_uri`, `scope=developer/v1/all`,
   optional `state`.
3. **Token exchange:** `POST https://app.copper.com/oauth/token`,
   **form-encoded** body: `grant_type=authorization_code`, `code`,
   `redirect_uri`, `client_id`, `client_secret`. Response JSON:
   `{ "access_token": "…", "token_type": "Bearer", "scope": "developer/v1/all" }`.
4. **No refresh:** tokens never expire; there is no refresh token or refresh
   endpoint. Disconnect is local (Copper users revoke under Settings →
   Integrations → Active Integrations; no documented programmatic revoke
   endpoint for the token).

**Scopes:** exactly one — `developer/v1/all` (read + modify all user records).
Copper documents finer scopes as "planned" but none exist today. Copper's
authorize endpoint **requires** a wire-level `scope=developer/v1/all` param
(verified against the official flow doc — `scope` is a required authorize
parameter with that fixed value), so this value is the wire scope. In the
bundle that means it goes in **`oauth.scopes:`** (the `scope=` sent to the
authorize URL), and is mirrored in **`oauth.display_scopes:`** (the UI-only
consent slug). This is the **gmail** split (a real wire `scopes:` list plus a
short `display_scopes:` list), NOT the bitly/notion shape (those providers have
NO wire scope param, so they carry `display_scopes:` only).

**PKCE:** not documented/supported → `pkce: none`.

**Token exchange style:** form-encoded body with `client_id`+`client_secret`
**in the body** (not HTTP Basic) → `token_exchange_style: form_secret`.

## 4. Helio provider bundle plan (`integrations/providers/copper/provider.yaml`)

Hidden-first (`presentation.visible: false`). This is a **pure `standard_oauth`
bundle — zero service-side Go adapter** (the flow is textbook auth-code +
Bearer; nothing falls outside the closed `standard_oauth` capability set).
Shape follows the shipped **bitly** bundle (non-expiring token, `refresh_lease:
none`, `form_secret` exchange, flat `userinfo` identity), since Copper's token
response carries no account/user id — plus the **gmail** wire-`scopes:` /
`display_scopes:` split for the required `developer/v1/all` authorize scope.

```yaml
schema: helio.provider/v1
key: copper
go_name: Copper

presentation:
  name: Copper
  description_key: copper
  consent_domain: copper.com
  visible: false          # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual   # the provider sees a person (GET /users/me → a Copper user); per-user token
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.copper.com/oauth/authorize
    token_url: https://app.copper.com/oauth/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [developer/v1/all]        # wire scope=; Copper authorize requires it
    display_scopes: [developer/v1/all] # UI consent slug (same value)
    single_active_token: false
    refresh_lease: none               # tokens never expire

identity:
  source: userinfo                    # token response has no id; GET /users/me
  url: https://api.copper.com/developer_api/v1/users/me
  stable_key: /email                  # unique per Copper user, string-valued (see rationale); /id is numeric
  label_candidates: [/name, /email]

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
  name: copper
  kind: oauth
```

**Naming axes in the bundle:** dir name / `key: copper` = axis ③; `tool.name:
copper` = axis ②; no `tool.command`/`tool.group` (flat command, axis ① =
`copper`). No grouped family, no `experiment` gate (GA once visible).

**Owner rationale (`individual`).** Copper's OAuth token is per-user: the
connecting person authorizes their own Copper login and `GET /users/me` resolves
to a **person** (`id`/`name`/`email`), not a workspace/bot install. That is
exactly the shipped **bitly** / **gmail** / all `google_*` / `microsoft_*` /
linkedin / x shape — `owner: individual`. `owner: assistant` is reserved for
bot/workspace-install semantics (slack/lark self-built-app, discord bot-install,
notion workspace bot) and changes real behavior on `main`: it gates the connect
flow on the connecting user being an org admin
(`oauth_start.go:46,100` — `def.Owner == OwnerAssistant && !isOrgAdmin`) and
routes the credential through the SA/app-bot path
(`oauth_credentials.go` `writeAssistantCredential`, design 227 §5.1) instead of
the user-owned credential + trust-delegation path (`writeIndividualCredential`).
Both are wrong for a personal Copper login, so `individual` is required.

**Stable-key rationale (`/email`, a string).** `GET /users/me` returns the
per-user identity; `/account` (org-level id) would collide across users in the
same Copper account, so `/users/me` is the right `userinfo` source. **But
Copper's `id` is a numeric integer** (official example: `"id": 159258`,
unquoted), and the generic declarative resolver's `jsonPointerString`
(`service/declarative_identity.go`) extracts the stable key via a
`value.(string)` type assertion — **it does not coerce numbers to strings on
`main`** (verified against current `main`: a numeric `/id` resolves to
"identity has no string value at stable key"). So `/id` is *not* usable without
a numeric stable-key coercion capability growth, which is **not shipped on
`main`** — this corrects the assumption that hubspot/typefully verified such
coercion there. To keep this bundle inside the already-shipped capability set
(the design's zero-capability-growth thesis), the stable key is Copper's
**`/email`** — string-valued, present, and unique per Copper user (it is the
login identity). The precedent is therefore the **bitly `/login`** /
**gmail `/sub`** *string-from-userinfo* shape (both are string keys — not
numeric; the earlier "flat field, bitly/gmail shape" note is retained only for
the userinfo-fetch shape, not the key type). Tradeoff: `/email` is a login
identity a user can in principle change, so a reconnect after an email change
would mint a new `AccountKey`; this is acceptable for an isolated per-assistant
connection and is the only unique string Copper exposes (there is no stable
string subject id). If numeric stable-key coercion later lands on `main`
(the hubspot/typefully direction), the durable numeric `/id` becomes the
preferred key and this bundle can switch — a strict identity improvement, no
migration for existing string-keyed rows required beyond the usual reconnect.
Both `/users/me` and the (unused) `/account` work with Bearer-only auth
(verified), so the generic resolver needs no header capability growth.

**Config sync (human lane 1 landing):** `client_id`/`client_secret` land
together (partial config fails integration-service startup) in **both**
`config/` and the `deploy/` Helm Secret per the Config Sync hard rule — as the
per-provider append, landing before this provider's L5. Never in the bundle.

**Service code:** none. `standard_oauth` with `form_secret` + `pkce: none` +
flat `userinfo` identity + `refresh_lease: none` is fully within the existing
declarative capability set — every one of these enum values is already shipped
on `main` by the **bitly** bundle (`form_secret` + `userinfo` + `refresh_lease:
none`) and the **gmail** bundle (`form_secret` + wire `scopes:` + `userinfo`).
If, at
implementation, `GET /users/me` is found to need any non-Bearer header (it does
not, per docs), that would be the only trigger to reconsider — flagged here at
stage 1 per the master plan, expected negative.

## 5. Other Helio-side artifacts

- **UI icon:** `ui/helio-app/src/integrations/icons/copper.svg` + manual
  register in `providerIcons.ts` (Copper wordmark/logo; not generated).
- **i18n:** `description_key: copper` string in the integrations locale file.
- **AI-facing docs:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` describing the `copper` verbs, the
  `POST /search` list convention, and the `--json-body` escape hatch; plugin
  version bump + marketplace publish ride the batch-end merge.
- **Generation:** one `provider-gen` run at batch end updates all five
  projections together; run locally with `--check` on-branch for L3 but **do
  not commit** the regenerated projections (batch lead owns the canonical
  regen).

## 6. Test plan — five layers

| Layer | Copper-specific plan | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: `httptest.Server` fake asserts (a) `Authorization: Bearer` + `Content-Type: application/json` sent, no `X-PW-*`; (b) `list`→`POST /{res}/search` with the assembled JSON body + pagination; (c) `get/create/update/delete` verb→method/path mapping; (d) `find-email`→`POST /people/fetch_by_email`; (e) `--json-body` merged into payload; (f) non-2xx → `apiError` with exit 1, and `--json` error envelope; (g) 401 credential-rejection classification. Never hits real API. | No (fakes) |
| **L2** harness real-API | `ANYCLI_CRED_ACCESS_TOKEN=<real> anycli copper -- account get` and `-- person list --page-size 1`, `-- user me`. Proves field names, Bearer injection, and the real `POST /search` body shape against live Copper. Mandatory before pin bump. | **Yes** — real Copper OAuth token from the test-account pool (lane 2) |
| **L3** generation + suites | Local `provider-gen` + `provider-gen --check` against the branch bundle; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with a local `replace` to the anycli branch; integration-service unit suite. No resolver test needed (②==③, no divergence entry). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider":"copper"` and a **real** `access_token` (no `refresh_token`/`expires_at` — non-expiring bot-token class, seed `access_token` only), using a real seeded org/assistant/owner identity; then `heliox tool copper -- account get` reaches live Copper through the token gateway. Bypasses the connect UI. | **Yes** — real token seeded (dev-mode app from lane 1 gates this) |
| **L5** full connect flow | Once, hidden, pre-flip: `heliox tool copper auth` → Copper consent on the registered dev app → confirm `oauth_connected` system event → unseeded `heliox tool copper -- user me` through the new connection. Human-in-the-loop (oauth consent). Gated additionally on review clearance before the visible flip. | **Yes** — registered dev app (lane 1) + real Copper account consent (lane 3) |

**Credential-dependent layers:** L2, L4, L5 (all need a real Copper OAuth
token/account and, for L5, the partner-registered dev app). L1 and L3 are fully
agent-runnable with no external credentials.

**Definition of done:** all five layers green, docs published, icon registered,
then `presentation.visible: true` + regenerate as the single go-live change —
the visible flip additionally gated on `partners@copper.com` review clearance
(the `oauth_review` tail), which does not block code-complete-hidden.

## Sources

- Copper OAuth overview — https://developer.copper.com/introduction/oauth/index.html
- Copper OAuth flow (endpoints, non-expiring token) — https://developer.copper.com/introduction/oauth/flow.html
- Copper OAuth quickstart (Bearer-only API calls) — https://developer.copper.com/introduction/oauth/quickstart.html
- Copper authentication overview (API-key vs OAuth) — https://developer.copper.com/introduction/authentication.html
- Copper Fetch API User (`GET /users/me`) — https://developer.copper.com/account-and-users/fetch-api-user.html
