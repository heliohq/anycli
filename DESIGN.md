# Chargebee — per-tool design (`heliox tool chargebee`)

Scratch design for the Chargebee external tool provider, produced per the
`helio-tool-provider` pipeline skill and the 298-integrations master plan
(`docs/design/008-300-integrations-rollout-plan.md`, row 150). This file lives
on branch `tool/chargebee` and is stripped by the batch lead at batch-end.

- **Catalog row:** 150 · Product **Chargebee** · anycli id **`chargebee`** ·
  provider key **`chargebee`** · auth lane **`api_key`** · Wave **2** ·
  category **Finance**.
- **Audit verdict** (`oauth-audit.md`, row 152): `api_key`, "no viable
  multi-tenant path". **Confirmed against official docs** — Chargebee exposes
  no multi-tenant authorization-code OAuth for third-party apps; the REST API
  authenticates with a per-site API key over HTTP Basic auth. Lane stays
  `api_key`. No divergence to record.

## 1. What an AI teammate does with Chargebee

Chargebee is a subscription-billing / revenue platform. A Helio teammate wired
to a customer's Chargebee site acts as a billing operator: look up a customer
and their subscriptions, read invoices and credit notes, check payment/collection
status, create or change a subscription, issue account credits, browse the
product catalog (items / item prices / plans), record metered usage, and pull
the billing event stream. This is a read-heavy management surface with a few
high-value writes — exactly the AnyCLI-passthrough shape, not inference.

## 2. Official API surface this tool wraps

Verified against the official docs (https://apidocs.chargebee.com/docs/api and
`/docs/api/auth`):

- **Product / version:** Chargebee Billing REST API **v2** (current, stable;
  official client libraries for Node/Python/PHP/Java/Go/Ruby/.NET all target it).
- **Base URL (per-site):** `https://{site}.chargebee.com/api/v2` — the `{site}`
  subdomain is the customer's Chargebee site name (e.g. `acme-test`). The API
  is single-tenant per site; there is no global host that resolves a key to a
  site, so **the site is mandatory input alongside the key**. (The docs also
  show an alternate unified host `https://api.chargebee.com/api/v2/{site}/`; the
  host-based form above is what the official client libraries and the `curl`
  quickstart use, so this tool uses it.)
- **Auth:** HTTP **Basic**. **API key is the username; password is empty** —
  `Authorization: Basic base64("<api_key>:")` (the trailing colon is required
  before base64). Keys are environment-specific (test-site vs live-site) and
  come from *Settings → Configure Chargebee → API & Webhooks → API Keys*.
- **Response format:** JSON only. List responses are
  `{ "list": [ { "<resource>": {…} }, … ], "next_offset": "<opaque>" }`;
  single-object responses are `{ "<resource>": {…} }`.
- **Pagination:** `limit` (max 100) + opaque `offset` cursor echoed as
  `next_offset`. Filters use bracketed operators (`status[is]=active`).
- **No dedicated identity/verify endpoint.** Connectivity/validity is checked
  with a cheap authenticated read; this tool uses `GET /customers?limit=1`
  (Customers exists on every site) — 2xx ⇒ key+site valid, 401/403 ⇒ rejected.

**Endpoints wrapped (driven by §1):**

| Resource | Endpoints | Why |
|---|---|---|
| Customers | `GET /customers`, `GET /customers/{id}`, `POST /customers`, `POST /customers/{id}` | Who is being billed; the entry point for most lookups |
| Subscriptions | `GET /subscriptions`, `GET /subscriptions/{id}`, `POST /customers/{id}/subscription_for_items`, `POST /subscriptions/{id}/…`, cancel/reactivate | The core billing object; create/change/cancel |
| Invoices | `GET /invoices`, `GET /invoices/{id}`, `GET /invoices/{id}/pdf` | Billing history + document links |
| Credit notes | `GET /credit_notes`, `GET /credit_notes/{id}` | Refund/adjustment records |
| Product catalog | `GET /items`, `GET /items/{id}`, `GET /item_prices`, `GET /item_prices/{id}`, `GET /plans`, `GET /plans/{id}` | What can be sold (PC 2.0 items/item_prices; PC 1.0 plans) |
| Payments | `GET /payment_sources`, `GET /transactions`, `GET /transactions/{id}` | Payment instruments + collection status |
| Usage | `GET /usages`, `POST /usages` (create) | Metered/usage-based billing |
| Events | `GET /events`, `GET /events/{id}` | Billing activity stream for monitoring |
| Escape hatch | `GET` any path via `chargebee -- get --path <p>` | Cover the long tail (quotes, estimates, orders, exports) without a verb per resource |

## 3. anycli definition & tool surface

### 3.1 Tool form — `service` type

`service` type per the stage-1 rubric. There is **no** official, non-interactive,
`--json`-capable Chargebee CLI binary to wrap (Chargebee ships language client
libraries, not a CLI), so `cli` type does not apply. Implement in
`internal/tools/chargebee/` against the v2 REST API, following the `notion`
service as the shape reference (cobra tree grouped by resource, injectable
`BaseURL`/`HC`/`Out`/`Err`, exit codes 0 success / 1 runtime/API failure via
typed `apiError` / 2 usage/parse, `--json` structured error envelope).

Go package name: `chargebee` (id has no dashes; `internal/tools/chargebee/`).
Register as `RegisterService("chargebee", &chargebee.Service{})` in
`internal/tools/register.go` (rides the batch-end registry merge).

### 3.2 Definition JSON (`definitions/tools/chargebee.json`)

Two credential bindings — the site is not a secret but is required to build the
host, so it is injected alongside the key:

```json
{
  "name": "chargebee",
  "type": "service",
  "description": "Chargebee subscription billing (customers, subscriptions, invoices, catalog) via the v2 REST API",
  "auth": {
    "credentials": [
      { "source": {"field": "api_key"}, "inject": {"type": "env", "env_var": "CHARGEBEE_API_KEY"} },
      { "source": {"field": "site"},    "inject": {"type": "env", "env_var": "CHARGEBEE_SITE"} }
    ]
  }
}
```

The service reads `CHARGEBEE_SITE` to build `https://{site}.chargebee.com/api/v2`
and sets `Authorization: Basic base64(CHARGEBEE_API_KEY + ":")` on every request.
Multiple `CredentialBinding` entries are already supported by the AnyCLI schema
(`AuthConfig.credentials` is a list), so **no AnyCLI capability gap** — this is a
plain two-field service definition.

### 3.3 Subcommands / verbs

Grouped by resource, read-first:

```
chargebee customer      list|get|create|update
chargebee subscription  list|get|create|change|cancel|reactivate
chargebee invoice       list|get|pdf
chargebee credit-note   list|get
chargebee item          list|get
chargebee item-price    list|get
chargebee plan          list|get
chargebee payment-source list
chargebee transaction   list|get
chargebee usage         list|create
chargebee event         list|get
chargebee get           --path <p> [--query k=v ...]   # read-only GET escape hatch
```

Common flags: `--limit` (≤100), `--offset <cursor>`, repeated `--filter
status[is]=active` (mapped verbatim to Chargebee bracket-operator query params),
and per-verb id/body flags. Writes take explicit typed flags (or `--param k=v`)
rather than free-form JSON bodies, matching the built-in-service conventions.

### 3.4 JSON output shape

Emit Chargebee's native JSON on stdout (the `{ "<resource>": {…} }` /
`{ "list": […], "next_offset": … }` envelope) so the AI sees the provider's own
field names; list verbs surface `next_offset` for follow-up paging. Errors: exit
1 with a `--json` structured error envelope carrying Chargebee's `api_error_code`
/ `message` (typed `apiError`), exit 2 for usage/parse. Never mocked/short-circuited
output — success means a real v2 call returned.

## 4. Credential fields & auth flow

- **Fields (two):** `site` (subdomain, non-secret, e.g. `acme-test`) and
  `api_key` (secret). Basic auth, key-as-username, empty password.
- **Registration model:** none. The customer generates a site API key in
  Chargebee settings; no Helio-side app registration, no client id/secret, no
  redirect URI, no review. This is why the lane is `api_key` and why lane 1 (OAuth
  app registration) has **nothing to do** for Chargebee.
- **Token semantics:** the API key is a long-lived, non-expiring, non-refreshing
  secret scoped to one site+environment. No refresh cycle to exercise.
- **Identity / account key:** the **site subdomain** is the natural stable,
  human-readable account key and label (one Chargebee account per site). It comes
  from user input, not from the verify response.

## 5. Helio provider bundle plan (`integrations/providers/chargebee/provider.yaml`)

**Three-axis naming (no divergence):** axis ① command `chargebee`, axis ② anycli
id `chargebee`, axis ③ provider key `chargebee` — all identical. **No
`toolToProvider` entry, no `tool.group`.** Hidden-first (`presentation.visible:
false`).

Bundle is a **manual credential** provider (`auth.type: api_key`) that stores a
single secret (the API key) via `POST /connections/credentials`; the non-secret
`site` becomes the connection `account_key`. Credential projection for the token
gateway → AnyCLI data map:

```yaml
credential:
  fields:
    api_key: token.access_token       # the stored secret
    site:    connection.account_key   # the site subdomain
    account_key: connection.account_key
```

This reuses existing credential sources (`token.access_token`,
`connection.account_key`) exactly as `mongodb` maps `connection_string` +
`account_key` — **no token-gateway/projection capability gap**.

### 5.1 Connect-time capability growth (the one gap on this base)

The worktree base's manual paths do **not** fit Chargebee as-is:

- `manual_credentials` (`dsnHostIdentityDeriver`) — single secret, no
  verification, identity parsed *from* the secret. Chargebee's identity (site) is
  a *separate* input, not inside the key.
- `manual_api_token` (`declarativeManualTokenVerifier`) — verifies against a
  **fixed** `Identity.URL` by setting **one header to the raw token**. Chargebee's
  verify URL is **site-templated** and auth is **Basic (key-as-username)**, not a
  raw header; and the connect form needs **two fields**.

`resolveManualSecret` today hard-enforces exactly one `credential_input` field,
so even a no-verify variant cannot carry the `site`. Growth is therefore
unavoidable; since it is unavoidable, do the **verified** variant (catches a
wrong site or typo'd key at connect time rather than at first tool use). The
minimal orthogonal capability (follow the already-precedented
servicenow "endpoint+secret", freshdesk "domain+key", braze "endpoint+key",
mailjet/lemlist "Basic-scheme verifier" shapes — reconcile with whatever has
already merged to main at implement time):

1. **Two-field manual input:** allow `credential_input.fields` to declare exactly
   one `secret: true` field (`api_key`) plus zero-or-more non-secret context
   fields (`site`). Generalize `resolveManualSecret` → return `(secret,
   contextValues)`; secret → Vault, context → verifier + account key. Relax the
   `provider-gen` "single field" check to "single secret field".
2. **Site-templated Basic-auth verifier:** a `siteScopedAPIKeyVerifier` that
   reads `site` from context, builds `https://{site}.chargebee.com/api/v2/customers?limit=1`
   (bundle-declared host/path template with a `{site}` placeholder), sends Basic
   auth with `api_key` as username / empty password, and on 2xx returns identity
   `{site}` with `account_key = label = site`; 401/403 → `invalid_provider_credential`.
   This adds a `scheme: basic_username` option to the api-key policy (vs the
   existing raw-header) and a `{site}`-templated identity URL, mirroring the
   servicenow instance-templated precedent.

No adapter, no OAuth, no config secrets. Because the bundle needs **no** Helio
client id/secret, integration-service `config/`+`deploy/` get **no appends** and
lane 1 is not involved; the provider is `configured: true` on the strength of its
compiled verification contract alone.

### 5.2 Bundle skeleton

```yaml
schema: helio.provider/v1
key: chargebee
go_name: Chargebee
presentation:
  name: Chargebee
  description_key: chargebee
  consent_domain: chargebee.com
  visible: false
auth:
  type: api_key
  owner: individual
  credential_input:
    fields:
      - name: site
        label_key: chargebee_site
        secret: false
        placeholder: "acme-test"
        required: true
      - name: api_key
        label_key: chargebee_api_key
        secret: true
        required: true
    setup_url: https://apidocs.chargebee.com/docs/api/auth
identity:
  source: strategy      # site-scoped verifier; account key = site
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials   # (verified variant per §5.1)
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    api_key: token.access_token
    site: connection.account_key
    account_key: connection.account_key
tool:
  name: chargebee
  kind: api-key
```

(Exact `runtime_strategy` / policy enum names follow whichever the merged
two-field-verified capability lands under — the intent above is authoritative.)

### 5.3 Other Helio-side artifacts

- **UI icon:** `ui/helio-app/src/integrations/icons/chargebee.svg` + manual
  register in `providerIcons.ts` (never generated).
- **i18n:** `chargebee` description + `chargebee_site` / `chargebee_api_key`
  field labels across all locales.
- **AI-facing docs:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/`, published with the batch plugin bump.
- **Generation:** `provider-gen` + `--check` from `go-services/integration-service`;
  five projections committed together at batch end (never on the tool branch).

## 6. Test plan — five layers

| Layer | What it proves for Chargebee | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fake: base URL built from `CHARGEBEE_SITE`; `Authorization: Basic base64(key:)` header; each verb's method/path/query (`limit`, `offset`, `filter` brackets); native-JSON passthrough; `--json` error envelope + exit codes 0/1/2 | **No** |
| **L2** harness real API | `ANYCLI_CRED_SITE=<site> ANYCLI_CRED_API_KEY=<key> anycli chargebee -- customer list --limit 1` returns real data; confirms field names, Basic-auth injection, and request shape against the live v2 API | **Yes** — real Chargebee test-site + key |
| **L3** `provider-gen --check` + both repos' suites | bundle validates (directory=key, single secret field, https URLs); integration-service `siteScopedAPIKeyVerifier` unit test (httptest 2xx→identity{site}, 401→invalid); helio-cli `cmds/tool` build/test | **No** |
| **L4** singleton + seed + heliox | seed `POST /internal/test-only/connections/seed` with `provider: chargebee`, `account_key: <site>`, `access_token: <api_key>`; then `heliox tool chargebee -- customer list` reaches the live API through the token gateway (api_key providers are seedable; site rides `account_key`, key rides `access_token`) | **Yes** — real test-site + key |
| **L5** full connect flow | api_key L5 path (agent-drivable, human fallback): open the connect link → enter **site + api_key** in the real two-field connect form → verifier accepts (site-templated Basic auth) → connection shows connected/`configured` in `GET /connections` → one **unseeded** live `heliox tool chargebee` command succeeds. Validates the two-field form + verification path the L4 seed bypasses | **Yes** — real test-site + key |

**Externally-supplied credentials** are needed for **L2, L4, L5** (one real
Chargebee **test-site** API key from the account pool — free test sites are
self-serve, so no paid tier or partner review; not a 3-hold risk). **L1 and L3**
are fully self-contained.

## 7. Rollout

Ship hidden. After L1–L4 pass on-branch (local `provider-gen` + `--check`, and
`helio-cli/go.mod` pointed at this anycli branch via a **local, uncommitted**
`replace`), the batch lead lands the shared surfaces at batch-end (registry,
pin bump, one `provider-gen` run, `providerIcons.ts`, plugin publish). After the
per-batch L5 sweep passes, flip `presentation.visible: true` + regenerate as the
single go-live change. No `oauth_review` gate — visible flip is gated on L5 only.

## Sources

- Chargebee API overview & getting started — https://apidocs.chargebee.com/docs/api
- Chargebee authentication (Basic auth, key-as-username, empty password) — https://apidocs.chargebee.com/docs/api/auth
