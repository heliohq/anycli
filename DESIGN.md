# ActiveCampaign — `heliox tool activecampaign` design

Per-tool design for the 298-integrations rollout (master plan
`docs/design/008-300-integrations-rollout-plan.md`, catalog row 126). Scratch
file on branch `tool/activecampaign`; the batch lead strips it at batch-end.

- **Product:** ActiveCampaign (marketing automation + CRM / email)
- **anycli id (axis ②):** `activecampaign`
- **provider catalog key (axis ③):** `activecampaign`
- **CLI command word (axis ①):** `activecampaign`
- **auth lane:** `api_key` (catalog row 126; OAuth audit row 128 verdict below)
- **wave / category:** Wave 2 · Marketing

All three naming axes are the identical single token `activecampaign` (no dash,
no corporate family, no collision) → **no `toolToProvider` divergence entry**
(`ProviderFor`/`ToolFor` identity holds). Go package name is `activecampaign`
(no dashes, no leading digit → used verbatim per master-plan §3 stage-2 rule).

---

## 1. Auth model — verified against official docs (drives everything)

**Verified against the official API docs, not the catalog:**

- Authentication is a single HTTP header **`Api-Token: <key>`** on every
  request. Source: developers.activecampaign.com/reference/authentication
  ("All requests to the API are authenticated by providing your API key … as an
  HTTP header named `Api-Token`", example `Api-Token: 123abc-def-ghi`).
- The base URL is **per-account**:
  `https://<account>.api-us1.com/api/3/` — the account subdomain **and its
  data-center segment** (`api-us1`, `api-us2`, …) are shown together on
  Settings → Developer. Source: reference/authentication example
  `https://123456demo.api-us1.com/api/3/...`.
- **No OAuth.** The authentication page documents only `Api-Token`; there is no
  authorize/token endpoint, no client registration, no scopes. Each user in an
  account has their own unique API key; the key does not expire and is not
  refreshed (revoked only by the account admin regenerating it).
- Rate limit: 5 requests/second per account (reference/rate-limits). The anycli
  service does no client-side throttling — it surfaces the provider's 429 as a
  runtime (exit-1) API error like every other non-2xx.

### 1.1 Audit reconciliation (independent judgment)

OAuth-audit row 128 verdict is **`api_key` — "no viable multi-tenant path"**,
and catalog row 126 lanes it `api_key`. The official docs **confirm** this: no
OAuth surface of any kind exists. No divergence to record — the catalog and the
official docs agree, so the tool ships in the `api_key` lane as written.

### 1.2 The credential shape: this is "endpoint + secret", not a bare key

The `Api-Token` alone is **insufficient** — a request cannot be formed without
the per-account base URL, and the data-center segment (`api-usN`) is **not
derivable** from the account name (it depends on which server the account is
provisioned on). So this is a **two-input** credential, exactly the
"instance base URL + token" class the **Freshdesk** bundle established
(design 317 OQ3, `origin/tool/freshdesk`):

| Input | Secret? | Example | Storage projection |
|---|---|---|---|
| `account_url` | no | `https://youraccount.api-us1.com` | `connection.account_key` (never Vault) |
| `api_key` | yes | `123abc-def-ghi…` | `token.access_token` (single vault secret) |

This is the ServiceNow/Freshdesk endpoint+secret precedent, **not** the single
-field mongodb/`manual_api_token` shape. It rides the already-built generic
two-field `manual_credentials` capability (see §4.2) — **zero new
integration-service code** for ActiveCampaign.

---

## 2. API surface wrapped — and why (driven by AI-teammate work)

Base path for every call: `{account_url}/api/3/`. An AI teammate on
ActiveCampaign does contact ops, list/tag segmentation, CRM deal reads/writes,
and campaign/automation reporting. The tool wraps the v3 REST resources that
serve those, verb-first, with `--json` passthrough of the provider body.

| Subcommand group | Endpoints (relative to `/api/3/`) | Why an AI teammate needs it |
|---|---|---|
| `contact list` | `GET contacts` (`?email=`, `?search=`, `?filters[...]`, `limit`/`offset`) | Find/segment contacts, the core object |
| `contact get` | `GET contacts/{id}` | Read one contact + linked fields |
| `contact create` | `POST contacts` (sync-friendly) | Add a lead/subscriber |
| `contact update` | `PUT contacts/{id}` | Maintain contact data |
| `contact delete` | `DELETE contacts/{id}` | Remove a contact |
| `list list` | `GET lists`, `GET lists/{id}` | Discover audiences to target |
| `contact subscribe` | `POST contactLists` `{contactList:{list,contact,status}}` | Add/remove a contact to a list (status 1/2) |
| `tag list` / `tag create` | `GET tags`, `POST tags` | Discover / create segmentation tags |
| `contact tag` / `contact untag` | `POST contactTags` `{contactTag:{contact,tag}}` / `DELETE contactTags/{id}` | Apply/remove a tag (segmentation trigger) |
| `deal list` / `deal get` | `GET deals` (`?filters[...]`), `GET deals/{id}` | CRM pipeline read for reporting |
| `deal create` / `deal update` | `POST deals`, `PUT deals/{id}` | Move CRM work forward |
| `pipeline list` / `stage list` | `GET dealGroups`, `GET dealStages` | Resolve pipeline/stage ids for deal ops |
| `campaign list` / `campaign get` | `GET campaigns`, `GET campaigns/{id}` | Campaign performance reporting |
| `automation list` | `GET automations` | Discover automations |
| `contact automate` | `POST contactAutomations` `{contactAutomation:{contact,automation}}` | Enroll a contact into an automation |
| `field list` | `GET fields`, `GET accounts` | Resolve custom-field ids / B2B accounts |

Sources: reference/overview (resource inventory: contacts, lists, deals,
campaigns, tags, automations, custom fields), plus the per-resource reference
pages (create/list/get contact, update-list-status-for-contact →
`contactLists`, create-contact-tag → `contactTags`, list-all-deals,
retrieve-all-lists, list-all-automations, create-a-contact-custom-field).

**Scope boundary (subtract before adding):** ship the read + high-value write
verbs above. Exclude the long tail (webhooks, ecommerce `ecomOrders`, forms,
message/template CRUD, account custom-field variants) from v1 — they are
addable later as new subcommands without touching auth or the bundle. Bulk
`POST contact/sync` is deferred; `contact create` covers the single-contact
path an assistant actually issues turn-by-turn.

### 2.1 Pagination & output shape

- Pagination is **`limit` (default 20, max 100) + `offset`** query params, with
  a `meta.total` count in the body (reference/pagination). List subcommands
  expose `--limit` / `--offset` and pass them through; the service does not
  auto-paginate (an agent asks for the next page explicitly, bounded output).
- Every subcommand emits the provider's JSON body verbatim on stdout (contacts,
  deals, etc. are already provider-neutral JSON — no re-shaping). `--json` is
  the default and only output mode, matching the AnyCLI agent-consumption rule
  (AGENTS.md: "target `--json` output and non-interactive flags").

---

## 3. anycli definition (stage 1 + 2)

### 3.1 Tool form: `service` type

`cli` type is rejected — no official ActiveCampaign binary exists. Implement
**`service`** type against the v3 HTTP API under
`internal/tools/activecampaign/` (21 of 23 shipped definitions are service
type; this is the default). Registered in `internal/tools/register.go`
`init()` as `RegisterService("activecampaign", &activecampaign.Service{})`.

### 3.2 `definitions/tools/activecampaign.json`

Two credential bindings — this is the shape that distinguishes ActiveCampaign
from single-token bundles (bitly). Both injected as env vars:

```json
{
  "name": "activecampaign",
  "type": "service",
  "description": "ActiveCampaign marketing automation & CRM (API v3, Api-Token header)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "ACTIVECAMPAIGN_API_TOKEN"}
      },
      {
        "source": {"field": "account_url"},
        "inject": {"type": "env", "env_var": "ACTIVECAMPAIGN_API_URL"}
      }
    ]
  }
}
```

- `field: api_key` matches the bundle projection key for the secret; injected as
  `ACTIVECAMPAIGN_API_TOKEN`, sent as the `Api-Token` header.
- `field: account_url` matches the bundle projection key for the non-secret
  account input (= `connection.account_key`); injected as
  `ACTIVECAMPAIGN_API_URL`, used as the request base.

### 3.3 Service implementation (`internal/tools/activecampaign/`)

Copy the shape of `internal/tools/notion/` (the reference service impl):

- `Service` struct with `BaseURL` override (empty ⇒ read from
  `ACTIVECAMPAIGN_API_URL`), `HC *http.Client`, `Out`/`Err` writers so tests
  point at an `httptest.Server` and capture stdout/stderr.
- Cobra tree grouped by resource (`contact`, `list`, `tag`, `deal`, `pipeline`,
  `stage`, `campaign`, `automation`, `field`) with the verbs in §2.
- **Domain normalization is the single site here** (per design 317: normalize
  at request time in AnyCLI, store verbatim in the bundle). Accept the pasted
  `account_url` in any of: full URL (`https://acct.api-us1.com`), with trailing
  slash, with a stray `/api/3`, or bare host (`acct.api-us1.com`) → normalize to
  `https://acct.api-us1.com/api/3`. One normalization function, unit-tested.
- Every request sets `Api-Token: <ACTIVECAMPAIGN_API_TOKEN>` and
  `Content-Type: application/json`.
- Exit-code contract (notion precedent): 0 success; **1** runtime/API failure
  (non-2xx from ActiveCampaign, including 429 rate limit and 401/403 credential
  rejection classified as `CredentialRejected`, transport failure); **2**
  usage/parse errors (bad flags, invalid JSON, unknown subcommand, missing
  required env). Errors render as a `--json` structured envelope on stderr.
- Pre-parse guard: if `ACTIVECAMPAIGN_API_TOKEN` or `ACTIVECAMPAIGN_API_URL` is
  empty, emit the structured error and exit 1 (mirror notion's token check).

TDD (AGENTS.md: tests first): `activecampaign_test.go` builds an
`httptest.Server` fake asserting (a) the `Api-Token` header is injected, (b) the
base-URL normalization variants all resolve to `…/api/3/<path>`, (c) request
bodies for `contact create` / `contact tag` / `contact subscribe` /
`contact automate` match the v3 wrapper shapes (`{contact:…}`,
`{contactTag:{contact,tag}}`, `{contactList:{list,contact,status}}`,
`{contactAutomation:{contact,automation}}`), (d) `--limit`/`--offset`
passthrough, (e) both plain and `--json` error rendering for a 401 and a 422.
Never hits the real API.

---

## 4. Helio provider bundle plan

### 4.1 `integrations/providers/activecampaign/provider.yaml` (hidden-first)

Modeled verbatim on `origin/tool/freshdesk`'s two-field
`manual_credentials` bundle (the established endpoint+secret exemplar):

```yaml
schema: helio.provider/v1
key: activecampaign
go_name: ActiveCampaign

presentation:
  name: ActiveCampaign
  description_key: activecampaign
  consent_domain: activecampaign.com
  visible: false   # hidden-first; flip gate in §4.4

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: account_url        # non-secret account/instance input → AccountKey
        label_key: activecampaign_account_url
        secret: false
        placeholder: "https://youraccount.api-us1.com"
        required: true
      - name: api_key            # the single vault secret
        label_key: activecampaign_api_key
        secret: true
        placeholder: "your ActiveCampaign API key"
        required: true
    setup_url: https://help.activecampaign.com/hc/en-us/articles/207317590-Getting-started-with-the-API

identity:
  source: strategy               # no HTTPS identity endpoint; account_url is the key/label

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
    api_key: token.access_token       # single secret
    account_url: connection.account_key  # non-secret instance value

tool:
  name: activecampaign
  kind: api-key
```

Both `credential.fields` sources (`token.access_token`,
`connection.account_key`) already exist in the closed `CredentialSource`
allowlist (`model/catalog.go`) — **zero token-gateway change**. `setup_url`
points at ActiveCampaign's official "Getting started with the API" help article
(where Settings → Developer shows the URL + Key together).

### 4.2 integration-service capability: reuse Freshdesk's, add nothing

The generic **two-field `manual_credentials`** capability is already built on
`origin/tool/freshdesk`:

- `validateCredentialInputSchema` (model/runtime_contract.go) accepts a schema
  of **exactly one secret field + optionally one non-secret field** (both
  required) — ActiveCampaign's two fields (1 secret + 1 non-secret) validate as-is.
- `accountInputIdentityDeriver` (service/manual_credentials_identity.go) is
  **provider-agnostic**: it takes the non-secret account input verbatim as the
  stable, human-readable account key **and** label, does **no** provider-side
  verify (correct — ActiveCampaign has no cheap identity endpoint we need, and
  `identity.source: strategy` forbids `identity.url`), and never lets the secret
  enter the identity map. `AccountKeyInputField` routes `account_url` away from
  Vault to `Connection.AccountKey`.

ActiveCampaign adds **no** integration-service Go. The only cross-tool
dependency: the Freshdesk two-field capability must land on main **before**
ActiveCampaign's batch-end merge (both are same-program; if Freshdesk's
capability is not yet merged at ActiveCampaign's batch, ActiveCampaign's
`provider-gen --check` and startup validation fail until it is — track in the
wave board). A no-verify bad key surfaces at first tool use via AnyCLI's
`CredentialRejected` classification (401/403 → exit 1), the accepted trade-off
for the whole `manual_credentials` class.

### 4.3 Non-generated Helio artifacts

- **Resolver:** none — axis ② == axis ③ == `activecampaign`, identity mapping,
  no `toolToProvider` entry.
- **UI icon:** `ui/helio-app/src/integrations/icons/activecampaign.svg` +
  register in `ui/helio-app/src/integrations/providerIcons.ts` (manual, never
  generated).
- **i18n:** `activecampaign`, `activecampaign_account_url`,
  `activecampaign_api_key` label keys in all 9 locales (visible-flip gate).
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` documenting the `account_url` + key
  connect step and the subcommand surface; plugin version bump + marketplace
  publish ride the batch-end merge.

### 4.4 Rollout (hidden-first)

Ship `visible: false`. Flip gate (all required): anycli `activecampaign` tool
ships in the pinned AnyCLI + heliox rebuild / runtime image; reviewed brand icon
in helio-app; the three i18n keys in all locales; five-layer test pass green
through L5 (key-entry path). Flip = `visible: true` + `provider-gen` regenerate
as the single go-live change; pick an unoccupied `presentation.order`.

---

## 5. Test plan — five layers

| Layer | What it proves for ActiveCampaign | External creds? |
|---|---|---|
| **L1** | `go test ./...` in anycli: httptest fake asserts `Api-Token` injection, URL normalization variants, v3 body wrappers (`contactTag`/`contactList`/`contactAutomation`), limit/offset passthrough, `--json` + plain error rendering. | No |
| **L2** | `ANYCLI_CRED_API_KEY=… ANYCLI_CRED_ACCOUNT_URL=https://acct.api-us1.com anycli activecampaign -- contact list --limit 5` (and a `contact create`+`contact tag` round-trip) against the **real** account — proves field names, header injection, and request shapes match live v3. Mandatory before the pin bump. | **Yes** — real ActiveCampaign account (test-account pool) |
| **L3** | `provider-gen` + `provider-gen --check` (bundle validates on the Freshdesk two-field capability); anycli + integration-service + helio-cli unit suites green; helio-cli built with a local `replace` to this anycli branch. | No |
| **L4** | singleton + `POST /internal/test-only/connections/seed` `{provider:"activecampaign", account_key:"https://acct.api-us1.com", access_token:"<real key>"}` → `heliox tool activecampaign -- contact list`. Non-expiring key: seed `access_token` only (no refresh cycle). `account_key` carries the account URL back to anycli via the projection. Reaches live v3. | **Yes** — real key seeded (seed bypasses the connect UI) |
| **L5** | Key-entry connect path (api_key sweep, master-plan §2): open connect link → enter `account_url` + `api_key` through the real connect UI (`POST /connections/credentials`) → connection shows connected/configured (`GET /connections`) → one **unseeded** live `heliox tool activecampaign -- contact list` succeeds. Run once, hidden, before the visible flip. Agent-drivable (agent-browser), human fallback on UI breakage. | **Yes** — real account, key pasted through the UI |

L1 and L3 need no credentials. L2, L4, L5 all consume one real ActiveCampaign
account from the test-account pool (lane 2). There is **no OAuth L5** (no
consent screen) — the api_key key-entry checklist applies.

---

## 6. Summary of decisions

- **api_key lane confirmed** against official docs (no OAuth exists) — agrees
  with catalog row 126 and audit row 128; no divergence recorded.
- **Two-field endpoint+secret** credential (account URL + `Api-Token`), riding
  the generic Freshdesk `manual_credentials` capability — **zero new
  integration-service code**, zero token-gateway change, zero resolver entry.
- **service-type** anycli tool over v3 REST; verb-first cobra tree scoped to the
  contact/list/tag/deal/campaign/automation work an AI teammate does; single
  URL-normalization site in the service.
- **hidden-first**, flip gated on the pinned anycli + icon + i18n + full L5.
