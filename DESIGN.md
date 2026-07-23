# NetSuite — per-tool design (`tool/netsuite`)

Scratch design for the 3-hold pre-verify + build of the NetSuite `heliox tool`
provider. Committed on branch `tool/netsuite`; the batch lead strips it at
batch-end. Catalog row 147/149: `NetSuite | netsuite | netsuite | api_key |
3-hold | Finance` (master plan §4). This tool is one of the three "non-standard
auth shape" flags (§6) and must clear the 3-hold **pre-verify** gate before its
dev branch is allowed to merge.

Every claim below was checked against Oracle's official SuiteTalk REST
documentation and the actual repo code, per the batch-lead instruction that
nothing in the prompt/catalog is exempt. Divergences from the catalog are
recorded in §8.

---

## 0. Pre-verify verdict (3-hold gate)

**Verdict: BUILD, api_key lane confirmed, `service` type. The one genuinely new
anycli concern is OAuth 1.0a request signing inside the service package (§2). On
the Helio side the credential-form shape is a **build-time capability decision**,
not a hard constraint: the PRIMARY plan is a five-field `manual_credentials`
connect form (account_id + the four TBA secrets), which is an already-established
program pattern (paypal / zoominfo / plaid grew exactly this on sibling
branches); a single JSON-blob credential is kept only as an explicitly-labeled
interim FALLBACK for the case where that multi-field capability has genuinely not
merged to the netsuite build base by build time. Either shape stores one opaque
secret payload that anycli decodes and signs — the signing path is identical, so
this decision changes only the first-run UX, not the runtime. One small
Helio-side growth stays to confirm at stage 2: an account-key identity deriver
(§4 / minor finding).**

Verified against the 3-hold base itself (`go-services/integration-service`), but
read as a **snapshot of the frozen `tool/netsuite` base**, not a permanent
platform limit:
- `manual_credentials` storage on *this frozen base* is single-secret: design
  317 D5 (`model/runtime_contract.go:328`, `validateCredentialInputSchema`)
  requires `auth.credential_input.fields` to be **exactly one required field**,
  and the runtime write path (`service/manual_credential.go:179`,
  `resolveManualSecret`) fails fast if `len(Fields) != 1`. So a five-field
  connect form is not expressible **on this frozen base as-is** — that is the
  only sense in which it is constrained.
- BUT multi-field `manual_credentials` is an **established program pattern**,
  not a hypothetical: paypal, zoominfo, and plaid each relaxed D5 to a
  multi-field connect form (integration-service packs the N labeled fields into
  the single token payload) on their sibling `tool/*` branches. Those bundles
  are simply **absent from this frozen worktree base**, sitting on unmerged
  siblings — and this 3-hold batch runs as the **last** batch of Wave 3 (master
  plan §5), so that capability will almost certainly be merged to main by
  netsuite build time. The master plan even **budgeted** growth for NetSuite
  ("distinct vault credential kind and/or signing adapter", §6), so "zero
  growth" was never a mandate.
- The shipped single-blob precedent (mongodb, one `connection_string` field →
  `token.access_token`) remains a valid **fallback** shape if multi-field has
  not landed. It is not the recommended first-run.
- `knownCredentialSources` on the netsuite base is
  `{token.access_token, connection.account_key, connection.metadata.person_urn,
  credential.app_id, credential.brand, binding.user_access_token}`
  (`cmd/provider-gen/validate.go`) — there is **no** `token.<field>` per-field
  source under either shape, so both plans project the packed payload through the
  existing `token.access_token`; a per-field `credential.fields` map
  (`token.account_id`, …) is neither needed nor allowed.

**Stage-2 action: re-read the netsuite build base** before coding the bundle. If
multi-field `manual_credentials` (paypal-style) is present, adopt it (five
labeled fields — the recommended shape). If — and only if — it genuinely has not
merged, ship the single-blob interim fallback and record the divergence. Either
way the anycli service is unchanged (§2).

The master plan flagged three risks for NetSuite; each is resolved here:

1. **Non-standard auth shape (§6).** Confirmed real. NetSuite Token-Based
   Authentication (TBA) is OAuth **1.0a** per-request signing with HMAC-SHA256
   over **four** secrets plus the account id — not a bearer key. This is
   handled entirely inside the anycli `netsuite` service package (stdlib
   `crypto/hmac`+`crypto/sha256` signing); it needs **no** signing logic on the
   Helio side. The five TBA values reach anycli as **one opaque secret payload**
   through the existing `token.access_token` source, which the service decodes at
   startup (§2/§4). How that payload is *collected* is the build-time decision
   above: PRIMARY is a five-field connect form (integration-service packs the
   labeled fields into the token payload — the paypal/zoominfo/plaid pattern);
   FALLBACK is a single JSON blob the user pastes (mongodb pattern). Both land
   the same JSON payload in the vault and hand it back verbatim, so the anycli
   decode + sign path is identical either way. The plan's "distinct vault
   credential kind and/or signing adapter" hedge resolves to: **no** new vault
   kind (multi-field `manual_credentials` already packs into the single token
   payload), and the "signing adapter" lives in anycli, not in a Helio
   `service/adapter_*.go`. The one Helio-side growth to confirm at stage 2 is the
   account-key identity deriver (§4, minor finding): the only shipped
   `manual_credentials` deriver (`dsnHostIdentityDeriver`) parses a DSN **host
   out of the secret**, not a JSON `account_id`, so a small sibling deriver is
   the single candidate integration-service growth beyond the (likely
   already-merged) multi-field form — flagged, to confirm/build at stage 2, not
   asserted away.

2. **Deprecation clock (§6).** Confirmed and re-confirmed: from release
   **2027.1** NetSuite will not allow **new** TBA integrations to be created
   (Oracle: *"as of 2027.1, no new integrations using TBA can be created for
   SOAP web services, REST web services, and RESTlets"*); **existing** TBA
   integrations keep working. This design's credential model is
   **customer-owned** (§3): each customer creates the integration record *and*
   the access token inside their **own** NetSuite account and pastes all five
   fields — there is **no** shared Helio-owned integration app. Oracle is
   explicit that a consumer key/secret "is always specific to one NetSuite
   account" and cannot be transplanted; the only cross-account distribution
   mechanism is a bundled SuiteApp with a shared Application ID, and even then
   each installing account mints its **own** consumer key/secret — a mechanism
   this design does not build. Consequently the 2027.1 cutoff **does** bite:
   from 2027.1 a *new* customer can no longer create a TBA integration record,
   so new-customer onboarding via this flow is blocked (already-connected
   customers keep working). TBA is the right lane for the near-term shipping
   horizon — there is no self-serve multi-tenant OAuth app to register (§3,
   oauth-audit row 149) — but the deprecation makes OAuth 2.0 the **required
   successor for onboarding sooner than a naive read implies**, not an optional
   far-future path. §8 records that OAuth-2.0 migration concretely, so this is a
   deliberate, time-boxed choice on the 2027.1 clock.

3. **Test-account procurement (§6).** NetSuite has no self-serve/free tier. L2
   and L5 require a real NetSuite account (or an Oracle-provided sandbox
   `*_SB1`) from the account-pool lane; this design marks those layers
   credential-gated (§7). If the account pool cannot procure one at the 3-hold
   pre-verify gate, NetSuite is swapped out via the catalog-amendment mechanism
   (master plan open question 5) — the code below is otherwise complete.

---

## 1. Official API surface wrapped (and why)

NetSuite exposes several programmatic surfaces (SOAP SuiteTalk, RESTlets,
SuiteScript, the REST Record API, and SuiteQL). An AI teammate doing finance
work in NetSuite needs to **read across records with real joins/aggregation**
(reporting, reconciliation, "what did we invoice ACME last quarter") and to
**read/create/update individual transaction and entity records** (look up a
customer, create a sales order, update an invoice). That maps cleanly onto the
modern **SuiteTalk REST Web Services** surface — we deliberately avoid SOAP
(retiring, WSDL-heavy, cannot use OAuth 2.0 in the future) and RESTlets
(bespoke per-account SuiteScript, not a general contract).

Base URL (account-specific host):

```
https://<account-host>.suitetalk.api.netsuite.com/services/rest/
```

where `<account-host>` is the account id lowercased with `_` → `-`
(e.g. account `9876543_SB1` → host `9876543-sb1`; production `1234567` →
`1234567`). Verified: the URL subdomain uses the **lowercase/hyphen** form
while the OAuth **realm** uses the **uppercase/underscore** canonical form of
the same account id (`9876543_SB1`) — this asymmetry is a classic TBA foot-gun,
and the service package owns **deriving both** from the single account-id field
(§2) rather than trusting the casing the user happened to paste.

Two REST sub-APIs are wrapped:

| Sub-API | Endpoint pattern | Method(s) | AI use |
|---|---|---|---|
| **Record** (CRUD) | `record/v1/{recordType}` and `record/v1/{recordType}/{id}` | GET (list/get), POST (create), PATCH (update), DELETE | Look up / create / update a specific customer, invoice, sales order, vendor bill, item, etc. |
| **SuiteQL** (query) | `query/v1/suiteql` | POST, body `{"q": "<SQL>"}`, `?limit=&offset=` | Joined/aggregate reads across records — the primary "answer a finance question" path |
| **Metadata catalog** | `record/v1/metadata-catalog` (+ `?select=`) | GET | Discover available record types/fields (schema help for the AI) |

Verified endpoint contracts from Oracle docs:

- **SuiteQL requires the header `Prefer: transient`.** Omitting it returns
  `INVALID_HEADER`. The service always sets it on `query`.
- SuiteQL pagination is `limit`/`offset` query params; the response carries
  `hasMore`, `count`, `offset`, `totalResults`, and `links[].rel=next`. The
  service surfaces `hasMore`/`next` and supports `--limit`/`--offset`
  passthrough (default page cap kept small and agent-friendly).
- Record writes: `Content-Type: application/json`; `record/v1` supports a
  `?replace=` and reference-by-`externalId` via the `externalId` path segment,
  but v1 scope keeps to internal-id CRUD + create to stay minimal (§8 defers
  external-id addressing).
- Governance: an **account-tier-dependent** concurrency limit (driven by the
  account's SuiteCloud Processors / service tier — *not* a universal "10"),
  surfaced as HTTP **429**. The service maps 429 to the runtime-error exit code
  and, **when NetSuite actually returns a `Retry-After` header, echoes it
  best-effort** as `retry_after` in the `--json` error envelope (NetSuite's
  concurrency 429 is not documented to reliably carry `Retry-After`, so the
  field is frequently empty); the agent can then back off. The service does
  **not** auto-retry/block — fail-fast per the repo hard rule.

Async SuiteQL (`Prefer: respond-async` + job polling) is **out of v1 scope**
(§8) — transient synchronous queries cover the teammate use cases and avoid a
long-poll loop inside a single CLI invocation.

Sources (official + primary technical writeups):
- Oracle: *Executing SuiteQL Queries Through REST Web Services* — https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_157909186990.html
- Oracle: *Using SuiteQL with SuiteTalk REST Web Services* — https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_158394344595.html
- Oracle: *Token-based Authentication and Web Services* (TBA + 2027.1 deprecation) — https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_4381113277.html
- Modern Treasury / LedgerSync TBA HMAC-SHA256 walkthroughs — https://www.moderntreasury.com/journal/how-to-authenticate-to-netsuites-suitetalk-rest-web-services-api · https://www.ledgersync.dev/posts/netsuite-authentication/

---

## 2. anycli definition (axis ②: `netsuite`)

**Type: `service`** (stage-1 rubric). No official non-interactive `--json` CLI
exists that takes injected TBA creds and is provisionable into the image, so
the `cli` type is excluded; NetSuite joins the 21/23 `service`-type majority.
The service does REST + OAuth 1.0a signing over HTTP — squarely the `service`
shape.

`definitions/tools/netsuite.json` (**one** credential binding regardless of the
Helio connect-form shape — anycli always receives one opaque secret, the TBA
JSON payload, through a single env var; the multi-field-vs-single-blob choice of
§0 is a Helio-side decision that does not reach this definition):

```json
{
  "name": "netsuite",
  "type": "service",
  "description": "NetSuite ERP as a tool — SuiteTalk REST records + SuiteQL (Token-Based Auth)",
  "auth": {
    "credentials": [
      { "source": {"field": "credentials"}, "inject": {"type": "env", "env_var": "NETSUITE_CREDENTIALS"} }
    ]
  }
}
```

`NETSUITE_CREDENTIALS` carries a **JSON object** with the five TBA sub-fields:

```json
{"account_id":"1234567_SB1","consumer_key":"…","consumer_secret":"…","token_id":"…","token_secret":"…"}
```

The Helio bundle sources the `credentials` field from the existing
`token.access_token` (§4). Whether the user filled a five-field form (primary)
or pasted a JSON blob (fallback), Helio stores and returns the **same single
opaque JSON payload**, so the anycli definition and decode are invariant to that
decision — the five-way split always lives in anycli, and no per-field Helio
credential source or composite-decode Go is required either way.

Go package: `internal/tools/netsuite/` (id has no dashes/leading digit, so
package name == id). Registered as `RegisterService("netsuite",
&netsuite.Service{})` in `internal/tools/register.go` (the one shared-registry
merge held to batch-end per master plan §2).

### Service structure (copy `internal/tools/notion/` shape)

`Service{ BaseURL string; HC *http.Client; Out, Err io.Writer }` so unit tests
point `BaseURL` at an `httptest.Server` and capture output. `Execute(ctx,
args, env)` reads `NETSUITE_CREDENTIALS` from the `env` map (services read the
injected `env map`, not `os.Getenv` — verified against notion/slack),
`json.Unmarshal`s it into a small `tbaCreds{AccountID, ConsumerKey,
ConsumerSecret, TokenID, TokenSecret}` struct, and fails fast with a clear
message if the env var is missing, is not valid JSON, or leaves any of the five
sub-fields empty — then dispatches a cobra tree. A malformed/empty blob is a
usage error (exit 2); a live credential rejection by NetSuite is a runtime
error (exit 1). The JSON-decode + per-field presence check is unit-tested with
an httptest fake (L1, §7).

**Signing (the one genuinely new concern).** A small internal signer builds the
OAuth 1.0a Authorization header per request:
- Collect oauth params: `oauth_consumer_key`, `oauth_token` (= token_id),
  `oauth_signature_method=HMAC-SHA256`, `oauth_timestamp`, `oauth_nonce`,
  `oauth_version=1.0`.
- Build the signature base string: `METHOD & rfc3986(url-without-query) &
  rfc3986(sorted-merged-params)` where params merge the oauth params **and**
  any query-string params (SuiteQL's `limit`/`offset` must be included in the
  base string — a common bug if forgotten).
- Signing key = `rfc3986(consumer_secret) & rfc3986(token_secret)`; signature =
  base64(HMAC-SHA256(key, base)).
- Emit header: `Authorization: OAuth realm="<canonical account id>",
  oauth_consumer_key="…", oauth_token="…", oauth_signature_method="HMAC-SHA256",
  oauth_timestamp="…", oauth_nonce="…", oauth_version="1.0",
  oauth_signature="<rfc3986(sig)>"`.
- Both the URL host and the realm are **derived** (never used verbatim) from the
  decoded `account_id` sub-field, and the two forms differ — the classic TBA
  foot-gun:
  - Host = `strings.ToLower(strings.ReplaceAll(accountID, "_", "-"))`
    → lowercase/hyphen, e.g. `9876543-sb1`.
  - Realm = `strings.ToUpper(strings.ReplaceAll(accountID, "-", "_"))`
    → uppercase/underscore **canonical** form, e.g. `9876543_SB1`.
  NetSuite's realm is case-sensitive and expects the canonical uppercase/
  underscore form (Oracle: the account id "must match exactly, including
  upper-case and lower-case letters"), so normalizing rather than trusting the
  pasted casing means a user who enters `1234567_sb1` (or the hyphen host form)
  still signs correctly. Production numeric ids are case-/separator-neutral;
  only sandbox suffixes (`_SB1`) actually exercise the transform, so the L1
  suite pins a lowercased-input vector (§7).

Implemented with stdlib only (`crypto/hmac`, `crypto/sha256`, `encoding/base64`,
`net/url`) — no third-party OAuth1 dependency. RFC-3986 percent-encoding
(unreserved `A-Za-z0-9-._~` only) is its own tested helper; the `precision_test`
pattern in `internal/tools/x/` is the precedent for signature-exactness tests.

### Subcommands / verbs (axis-① command word `netsuite`, flat — no group)

```
heliox tool netsuite -- query   --q "<suiteql>" [--limit N] [--offset N]
heliox tool netsuite -- record get    --type <recordType> --id <internalId>
heliox tool netsuite -- record list   --type <recordType> [--limit N] [--offset N]
heliox tool netsuite -- record create --type <recordType> --body '<json>'
heliox tool netsuite -- record update --type <recordType> --id <id> --body '<json>'
heliox tool netsuite -- record delete --type <recordType> --id <id>
heliox tool netsuite -- metadata [--type <recordType>]     # schema discovery
```

`query` is the workhorse (joined reads); `record` is the CRUD resource group,
mirroring notion's resource-grouped tree. `metadata` surfaces the catalog so the
AI can discover record types/fields before writing.

### JSON output shape

`--json` on every subcommand (default plain-text for humans, structured for
agents). Success passes the provider's JSON payload through largely verbatim
(SuiteQL `{items|totalResults|count|hasMore|links}`; record objects as
returned), with a thin envelope only where NetSuite is inconsistent (create
returns `Location` header → surface the new internal id as
`{"id": "...", "location": "..."}`). Exit-code contract identical to notion:
**0** success, **1** runtime/API failure (NetSuite non-2xx incl. 401 cred
rejection and 429, transport error), **2** usage/parse (bad flags, invalid
`--body` JSON, unknown record verb). Errors render as a `{"error": {...}}`
JSON envelope under `--json`, plain text otherwise; 429 carries a best-effort
`retry_after` (populated only when NetSuite actually returns a `Retry-After`
header — see §1 — and omitted/empty otherwise).

---

## 3. Credential fields and exact auth flow (TBA / OAuth 1.0a)

TBA is a **manual credential** flow (api_key lane) — there is no browser
consent redirect; the user enters credentials they generate inside NetSuite.
Five values are needed. The connect-form shape is the build-time decision of §0:

- **PRIMARY — five labeled fields (`manual_credentials`, paypal/zoominfo/plaid
  pattern).** Five required, labeled inputs; integration-service packs them into
  the single token payload. This is the good first-run for a finance admin who
  is copying five discrete secrets off five different NetSuite screens, and it
  validates each field at connect time rather than surfacing a malformed
  hand-assembled blob as a stale failure at first query.
- **FALLBACK — one JSON-blob field (mongodb single-secret pattern).** Used only
  if multi-field `manual_credentials` has genuinely not merged to the netsuite
  build base by build time (re-verify at stage 2). The user pastes a JSON object
  of the five values into one `credentials` field; the `setup_url` documents the
  exact shape. Trade-off: no per-field connect-time validation, and the user
  must JSON-escape secrets that contain special characters.

Either way the five TBA values are the same. Verified against Oracle's TBA setup
docs, they are exactly:

| Field | Secret | Purpose | Where the user gets it |
|---|---|---|---|
| `account_id` | no | Realm (canonical uppercase/underscore form) + URL host (lowercase/hyphen form), both **derived** from this one value (§2) | Setup → Company → Company Information (Account ID); or Company URLs → SuiteTalk |
| `consumer_key` | yes | Integration record consumer key | Setup → Integration → Manage Integrations → New (enable TBA); shown once |
| `consumer_secret` | yes | Integration record consumer secret | same integration record; shown once |
| `token_id` | yes | Access token id | Setup → Users/Roles → Access Tokens → New (or "Manage Access Tokens") |
| `token_secret` | yes | Access token secret | same access-token page; shown once |

Under the primary form these are five vault-form fields that integration-service
packs into the one stored token payload; under the fallback they are five
sub-keys of one pasted JSON secret. In **both** cases Helio persists a single
JSON payload (as `token.access_token`) and hands it back verbatim; anycli owns
the decode + split (§2).

Registration model (why this is api_key, not oauth): the NetSuite admin creates
**one integration record** (consumer key/secret) and **one access token** (token
id/secret) bound to a role with "REST Web Services" + "Log in using Access
Tokens" permissions, inside **their own** NetSuite account. There is no
multi-tenant authorize endpoint we register a Helio app against, and there is no
single shared Helio-owned integration record: consumer key/secret are always
account-specific (Oracle), so each customer mints their own four secrets. That
is precisely the rubric's "OAuth is per-instance / impractical for a shared
client → stays api_key" (oauth-audit row 149: *"no viable multi-tenant path →
api_key"*). Confirmed correct. (Corollary — because the record is customer-owned,
the 2027.1 TBA cutoff blocks *new-customer* onboarding; §0 risk 2 / §8.)

Token semantics: TBA tokens **do not expire** (valid until revoked or the
user/role changes) — so there is no refresh cycle; seed/serve the four secrets
directly (like Slack's bot token, per the L4 "non-expiring token" guidance).

Per-request auth flow at runtime:
1. Token gateway serves the stored single-secret payload (the JSON blob) via
   `token.access_token` → helio-cli resolver → anycli credential map → one
   `NETSUITE_CREDENTIALS` env var.
2. anycli `netsuite` service JSON-decodes the blob into the five sub-fields,
   derives host+realm from `account_id`, signs each HTTP request with
   HMAC-SHA256 over the four secrets, sets `Prefer: transient` for SuiteQL, and
   calls `*.suitetalk.api.netsuite.com`.

---

## 4. Helio provider bundle plan (axis ③: `netsuite`)

Directory `integrations/providers/netsuite/provider.yaml`, **hidden-first**
(`presentation.visible: false`). Axis ② == axis ③ == `netsuite` (mechanical
identity) → **no `toolToProvider` entry** needed in
`helio-cli/internal/toolcred/resolver.go` (verified: the resolver only lists
divergent ids). Three-axis summary:

- ① CLI command word: `netsuite` (flat; no `tool.group` — NetSuite is a
  single product, not a Google/Microsoft-style family).
- ② anycli tool id: `netsuite`.
- ③ provider catalog key / directory: `netsuite`.

Bundle shape follows the **multi-field `manual_credentials` pattern**
(paypal / zoominfo / plaid) as the PRIMARY plan: five labeled required fields,
which integration-service packs into the single token payload sourced through
the existing `token.access_token`. If multi-field has genuinely not merged to
the netsuite build base by build time (re-verify at stage 2 by reading the
base), fall back to the single JSON-blob field (mongodb precedent). Either way
the vault stores one opaque JSON payload and there is **no per-field
`token.<field>` source map and no composite-decode Go on the Helio side** — the
five-way split is anycli's.

```yaml
schema: helio.provider/v1
key: netsuite
go_name: NetSuite

presentation:
  name: NetSuite
  description_key: netsuite
  consent_domain: netsuite.com
  visible: false          # hidden-first; flip is the single go-live change

auth:
  type: credentials
  owner: individual
  credential_input:
    # PRIMARY: five labeled required fields (multi-field manual_credentials,
    # paypal/zoominfo/plaid pattern). integration-service packs them into the
    # single token payload. FALLBACK (only if multi-field is genuinely absent
    # from the netsuite build base — re-verify at stage 2): a single required
    # `credentials` field into which the user pastes the JSON blob.
    fields:
      - { name: account_id,      label_key: netsuite_account_id,      secret: false, required: true, placeholder: '1234567_SB1' }
      - { name: consumer_key,    label_key: netsuite_consumer_key,    secret: true,  required: true }
      - { name: consumer_secret, label_key: netsuite_consumer_secret, secret: true,  required: true }
      - { name: token_id,        label_key: netsuite_token_id,        secret: true,  required: true }
      - { name: token_secret,    label_key: netsuite_token_secret,    secret: true,  required: true }
    setup_url: https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_4381113277.html

identity:
  source: strategy          # no HTTPS userinfo endpoint
  # account_key/label should be the readable account_id. The stored secret is a
  # JSON payload under BOTH shapes (multi-field packs the fields into it; the
  # fallback stores the pasted blob), and the ONLY shipped manual_credentials
  # deriver (dsnHostIdentityDeriver) parses a DSN *host out of the secret* — it
  # does not JSON-decode the payload to pull out account_id. So a small sibling
  # deriver (JSON payload → account_id → account_key/label) is the ONE candidate
  # integration-service growth for this tool beyond the multi-field form (§0,
  # §8). Confirm at stage 2 by reading the base: if such a deriver already
  # exists, cite it; otherwise add it (small, ~mongodb-sized) before coding the
  # bundle.

connection:
  mode: isolated
  disconnect_mode: local_only     # nothing to revoke server-side (manual creds)
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    # Multi-field form (primary): integration-service packs the five labeled
    # fields into the single token payload, which projects here as the access
    # token — anycli decodes the same JSON either way. (Fallback single-blob:
    # the pasted JSON IS the access token; mongodb precedent.)
    credentials: token.access_token
    account_key: connection.account_key

tool:
  name: netsuite
  kind: api-key           # wire-compat value (design 317 D2)
```

Nothing secret lives in the bundle (per `references/provider-yaml.md`): the field
values enter through the write-only `POST /connections/credentials` API and are
stored in Vault as the single token payload (integration-service packs the
multi-field form; the fallback stores the pasted blob directly); the bundle only
declares the field names. `manual_credentials` takes **no**
`required_config_fields` (there is no Helio-side client id/secret), so no
`config/` + `deploy/` secret append is needed — NetSuite is safe to ship hidden
with zero environment config, and there is no `standard_oauth` adapter. Icon:
`ui/helio-app/src/integrations/icons/netsuite.svg` + manual `providerIcons.ts`
registration (never generated). i18n: `tools.desc.netsuite` + one
`tools.credentialField.netsuite_*` label key per field under the primary form
(each label naming the NetSuite screen it comes from and pointing at `setup_url`),
or the single `netsuite_credentials` blob label under the fallback, across all
locales.

Generation (batch-end, one run): `provider-gen` + `provider-gen --check`
projects the five generated **files** together (Go catalog, UI fallback, SDK
union, helio-data fixture, check twin); on-branch we run it locally for L3 only
and do **not** commit the projections (master plan §2). "Five files" is the
generator's fixed output set — distinct from the credential source, which under
both connect-form shapes is just the existing `token.access_token`.

---

## 5. Why no `service/adapter_*.go` on the Helio side

The plan hedged "signing adapter." Concretely it is **not** needed on the Helio
side: `manual_credentials` stores one opaque JSON payload (packed from the
multi-field form, or the pasted blob on the fallback) and sources it to the token
gateway with zero provider-specific Go; the HMAC-SHA256 signing happens in anycli
at request time, where the secrets actually are. Reaching for a
`service/adapter_*.go` (the Slack/Discord/X/LinkedIn precedents) is only
justified by non-standard **OAuth token-exchange or revoke** behavior — NetSuite
TBA has neither (no exchange, no revoke). So the adapter budget the master plan
reserved for NetSuite is spent in anycli, not integration-service.

---

## 6. Naming / axes (no divergence)

| Axis | Value | Registered where |
|---|---|---|
| ① CLI command word | `netsuite` | bundle `tool.name` (no `tool.command`/`group`) |
| ② anycli tool id | `netsuite` | `definitions/tools/netsuite.json` |
| ③ provider catalog key | `netsuite` | bundle dir name / `key:` |

②↔③ identical ⇒ zero `toolToProvider` entries, zero `toolGroups` entries.

---

## 7. Test plan — five layers

| Layer | What it proves for NetSuite | Needs external creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `NETSUITE_CREDENTIALS` JSON decode + per-field presence (missing var, malformed JSON, and any-empty-sub-field all → usage exit 2); signature-base-string + header exactness against known TBA test vectors; host/realm transform vectors (host → lowercase/hyphen, realm → **uppercase/underscore canonical**, incl. a lowercased-input `1234567_sb1` case asserting realm-uppercasing); SuiteQL sets `Prefer: transient` and folds `limit`/`offset` into the signature base; record CRUD request shape (method/path/body); 429 → error envelope with `retry_after` populated **only when** the fake returns `Retry-After` (and empty when it does not); exit-code 0/1/2 matrix — all against an `httptest.Server` fake. **No real API.** | No |
| **L2** dev harness vs real API | `ANYCLI_CRED_CREDENTIALS='{"account_id":…,"consumer_key":…,…}' anycli netsuite -- query --q "SELECT id, companyname FROM customer FETCH FIRST 5 ROWS ONLY"` returns real rows; a `record get --type customer --id <id>` returns a real record. Proves the blob decode, signing, and the host/realm transform match the **live** account. | **Yes** — a real NetSuite account/sandbox from the account pool |
| **L3** `provider-gen --check` + both suites | Bundle strict-decodes (the connect-form schema passes validation — five required fields under the multi-field primary, or one required `credentials` field on the fallback; `credentials: token.access_token` passes the known-credential-source allow-list); five generated files regenerate consistently; `helio-cli` builds against the anycli branch via a local `replace`; integration-service + helio-cli unit suites green. If the stage-2 account-key deriver (§4) is added, its unit test lands here too. **No real creds.** | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with the packed JSON payload as the `access_token` (single token payload; `test_seed:true`), real seeded org/assistant identities, then `heliox tool netsuite -- query …` reaches the live API through the real token gateway. Seed the payload directly (no `expires_at` — TBA tokens don't expire, like Slack). | **Yes** — same real creds as L2 |
| **L5** full connect flow (once, pre-flip) | The **api_key key-entry** L5 path (master plan §2, not the OAuth path): open the connect link → fill the five `manual_credentials` fields (or, on the fallback shape, paste the JSON blob) through the real connect UI (stored via `POST /connections/credentials`) → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool netsuite -- query …` succeeds. Agent-drivable (agent-browser) with human fallback. | **Yes** — real creds + the connect UI |

L2/L4/L5 are **credential-gated on the account pool** (§0 risk 3). L1/L3 run
with no external dependency and gate the branch on their own.

---

## 8. Divergences from the catalog / audit, and deferred scope

- **Catalog auth lane confirmed, not overridden.** Official docs agree with the
  audit (row 149) and master-plan §6: no multi-tenant authorization-code OAuth →
  **api_key**. TBA's OAuth-1.0a-ness does not change the lane (it is manual,
  per-instance credentials), it changes the anycli *implementation* (request
  signing). No `oauth_light`/`oauth_review` re-lane.
- **"Distinct vault credential kind and/or signing adapter" (§6) resolves to:**
  reuse `manual_credentials` — multi-field (primary) or single-blob (fallback),
  both packing into the single token payload, so **no new vault kind** — plus
  signing in anycli (**no Helio adapter**). Recorded here so the pre-verify hedge
  is closed.
- **Credential-form shape is a build-time decision (primary: multi-field).** Two
  shapes deliver the same five TBA values to anycli as one packed JSON token
  payload:
  - **PRIMARY — five-field `manual_credentials`** (paypal / zoominfo / plaid
    pattern): integration-service packs the labeled fields into the single token
    payload. This is an **established program pattern**, merged on sibling
    `tool/*` branches; it is simply **absent from the frozen `tool/netsuite`
    worktree base**. Since this 3-hold batch runs **last** in Wave 3 (master
    plan §5), the capability will almost certainly be on main by netsuite build
    time. Adopting it is not new NetSuite-specific Go — it is reusing a landed
    platform capability (relax D5 to multi-field + pack in `resolveManualSecret`,
    already done for the siblings), which projects through the existing
    `token.access_token` with **no** named per-field `credential.*` sources.
  - **FALLBACK — single JSON-blob field** (mongodb precedent): used only if the
    multi-field capability has genuinely not merged by build time. On the frozen
    base it is the only expressible shape, because design 317 D5
    (`runtime_contract.go:328` + `manual_credential.go:179`) requires exactly one
    required field. Re-verify at stage 2 which applies.
  Either shape stores one opaque JSON payload projected as `token.access_token`,
  with anycli decoding + signing (§2/§4). There is **no** five-source
  `token.<field>` map on the Helio side under either shape (`knownCredentialSources`
  in `cmd/provider-gen/validate.go` has no `token.<field>` entries). Correction
  to earlier framing: prior drafts called the paypal/zoominfo (and braze/mixpanel)
  bundles "fabricated"/nonexistent and treated single-blob as "the only real
  precedent." That is imprecise — those bundles exist on unmerged sibling
  branches, and multi-field `manual_credentials` is a real, established pattern;
  they are just absent from *this frozen base*. The shape is therefore a
  build-time decision to re-check at stage 2, not a settled constraint.
- **One candidate integration-service growth: the account-key deriver.** `identity.
  source: strategy` needs a deriver that yields a readable `account_key`/label from
  `account_id`. The stored secret is a JSON payload under both shapes (multi-field
  packs into it; the fallback stores the pasted blob), and the only shipped
  `manual_credentials` deriver
  (`service/manual_credentials_identity.go:dsnHostIdentityDeriver`) parses a DSN
  **host out of the secret**; it does not JSON-decode the payload to pull out
  `account_id`. So a small sibling deriver (JSON payload → `account_id`) is
  expected — confirm at stage 2 by reading the base (cite it if it already exists,
  add it if not). This is the one Helio-side growth §0 flags as "small, to
  confirm" beyond the (likely already-merged) multi-field form — stated plainly
  rather than asserted away.
- **OAuth 2.0 successor (deprecation-driven, deferred but not far-future).**
  From 2027.1 no new TBA integration *records* can be created in any account.
  Because this design is **customer-owned** (each customer creates their own
  integration record — §0 risk 2, §3), the cutoff blocks **new-customer
  onboarding** via TBA from 2027.1: already-connected customers keep working,
  but a fresh customer can no longer mint the consumer key/secret. There is no
  "single reusable Helio app created once" that sidesteps this — a consumer
  key/secret is always account-specific (Oracle), and even a bundled SuiteApp
  mints a per-account key on install, a distribution mechanism this design does
  not build. So OAuth 2.0 is the **required successor for onboarding, on the
  2027.1 clock**, not an optional far-future path: the successor bundle is an
  `oauth_review`-lane `standard_oauth` (authorization-code + **PKCE**,
  short-lived access token + refresh, account-specific token host). It is
  flagged and scoped here, not built in v1; the timeline is driven by the
  2027.1 deprecation, and this doc is the pointer for that work.
- **Deferred v1 scope:** async SuiteQL (`Prefer: respond-async` + job polling),
  external-id record addressing (`/record/v1/{type}/eid:{externalId}`),
  subrecord/sublist mutation, and `?replace=` upsert semantics. Sync transient
  SuiteQL + internal-id CRUD cover the teammate use cases; the deferred surface
  is additive later without breaking the definition.
- **Governance is fail-fast, not auto-retry.** 429/`Retry-After` is surfaced to
  the agent, not swallowed by an in-CLI backoff loop (repo hard rule: no silent
  fallback/blocking inside one invocation).
