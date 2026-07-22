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

**Verdict: BUILD, api_key lane confirmed, `service` type, ZERO new
integration-service credential *source* growth (single-blob credential through
the existing `token.access_token`, mongodb precedent), ONE small candidate
integration-service growth to confirm/build at stage 2 (an account-key identity
deriver — §4 / minor finding), and one genuinely new anycli concern (OAuth 1.0a
request signing inside the service package).**

Verified against the 3-hold base itself (`go-services/integration-service`),
not assumed:
- `knownCredentialSources` on the netsuite base is
  `{token.access_token, connection.account_key, connection.metadata.person_urn,
  credential.app_id, credential.brand, binding.user_access_token}`
  (`cmd/provider-gen/validate.go`) — there is **no** `token.<field>` per-field
  source. A per-field `credential.fields` map (`token.account_id`,
  `token.consumer_key`, …) would be rejected by `provider-gen --check` as an
  unknown credential source.
- `manual_credentials` storage is **single-secret**: design 317 D5
  (`model/runtime_contract.go:validateCredentialInputSchema`) hard-requires
  `auth.credential_input.fields` to be **exactly one required field**, and the
  runtime write path (`service/manual_credential.go:resolveManualSecret`)
  fails fast if `len(Fields) != 1`. A 5-field connect form is therefore
  **impossible** on this base without integration-service Go growth.
- The only shipped `manual_credentials` precedent is **mongodb** (one
  `connection_string` field → `token.access_token`). The zoominfo / paypal /
  braintree / mixpanel bundles cited in an earlier draft **do not exist** in
  either worktree base; that claim was fabricated and is removed. The design
  now anchors on the single real precedent (mongodb, single-blob).

The master plan flagged three risks for NetSuite; each is resolved here:

1. **Non-standard auth shape (§6).** Confirmed real. NetSuite Token-Based
   Authentication (TBA) is OAuth **1.0a** per-request signing with HMAC-SHA256
   over **four** secrets plus the account id — not a bearer key. This is
   handled entirely inside the anycli `netsuite` service package (stdlib
   `crypto/hmac`+`crypto/sha256` signing); it needs **no** signing logic on the
   Helio side. Because Helio storage is single-secret (D5, above), the five
   TBA values are carried as **one JSON credential blob** through the existing
   `token.access_token` source — the mongodb single-field pattern extended so
   the blob is a JSON object the anycli service decodes at startup (§2/§4), not
   five per-field vault columns. This means **zero** new credential sources and
   **zero** composite-decode Go on the Helio side. The plan's "distinct vault
   credential kind and/or signing adapter" hedge resolves to **neither**: the
   vault stores an opaque single-secret payload (already supported), and the
   "signing adapter" lives in anycli, not in a Helio `service/adapter_*.go`.
   The one thing that is **not** free is the account-key identity deriver (§4,
   minor finding): the only shipped `manual_credentials` deriver
   (`dsnHostIdentityDeriver`) parses a DSN **host out of the secret**, not a
   JSON `account_id`, so a small sibling deriver is the single candidate
   integration-service growth — flagged, to confirm/build at stage 2, not
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

`definitions/tools/netsuite.json` (**one** credential binding — the single-blob
pattern, mirroring mongodb's one `connection_string` → one env var; §0 explains
why five separate bindings are impossible on this base):

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
`token.access_token` (§4); the single opaque secret Helio stores **is** this
JSON string, so no new Helio credential source or composite-decode Go is
required. The five-way split lives entirely in anycli.

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
consent redirect; the user pastes credentials they generate inside NetSuite.
Because Helio storage is single-secret (§0, design 317 D5), the connect form is
**one** `credentials` field into which the user pastes a **JSON object**
carrying the five TBA values below (the mongodb single-field precedent, where
mongodb's one field is a DSN and NetSuite's is a small JSON blob). The
`setup_url` documents both where each value comes from and the exact JSON shape
to paste. Verified against Oracle's TBA setup docs, the five sub-fields are
exactly:

| JSON sub-field | Secret | Purpose | Where the user gets it |
|---|---|---|---|
| `account_id` | no | Realm (canonical uppercase/underscore form) + URL host (lowercase/hyphen form), both **derived** from this one value (§2) | Setup → Company → Company Information (Account ID); or Company URLs → SuiteTalk |
| `consumer_key` | yes | Integration record consumer key | Setup → Integration → Manage Integrations → New (enable TBA); shown once |
| `consumer_secret` | yes | Integration record consumer secret | same integration record; shown once |
| `token_id` | yes | Access token id | Setup → Users/Roles → Access Tokens → New (or "Manage Access Tokens") |
| `token_secret` | yes | Access token secret | same access-token page; shown once |

These are **sub-keys of the one stored secret**, not five separate vault
columns or five Helio credential-form fields. The whole JSON string is what
Helio persists (as `token.access_token`) and hands back verbatim; anycli owns
the split (§2). The UX trade-off (one JSON paste vs. a five-input form) is the
price of zero integration-service growth — see §8 for the multi-field
alternative that was rejected for this reason.

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

Bundle shape follows the **`mongodb` single-field `manual_credentials`
precedent** — the only extant manual-credentials bundle and the only shape the
netsuite base supports (single-secret D5, §0). One `credentials` field, sourced
through the existing `token.access_token`; the five TBA values ride inside it as
JSON (§2/§3). **No new credential sources, no per-field `token.<field>` map, no
composite-decode Go.**

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
    fields:
      # Exactly ONE required field (design 317 D5 single-secret storage). The
      # user pastes a JSON object of the five TBA values; anycli decodes it.
      - { name: credentials, label_key: netsuite_credentials, secret: true, required: true, placeholder: '{"account_id":"1234567_SB1","consumer_key":"…","consumer_secret":"…","token_id":"…","token_secret":"…"}' }
    setup_url: https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_4381113277.html

identity:
  source: strategy          # no HTTPS userinfo endpoint
  # account_key/label should be the readable account_id. The ONLY shipped
  # manual_credentials deriver (dsnHostIdentityDeriver) parses a DSN *host out
  # of the secret* — it does not JSON-decode a blob to pull out account_id. So
  # a small sibling deriver (blob → account_id → account_key/label) is the ONE
  # candidate integration-service growth for this tool (§0, §8). Confirm at
  # stage 2 by reading the 3-hold base: if such a deriver already exists, cite
  # it; otherwise add it (small, ~mongodb-sized) before coding the bundle.

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
    # Single-secret projection: the JSON blob IS the access token. mongodb
    # precedent — connection_string: token.access_token.
    credentials: token.access_token
    account_key: connection.account_key

tool:
  name: netsuite
  kind: api-key           # wire-compat value (design 317 D2)
```

Nothing secret lives in the bundle (per `references/provider-yaml.md`): the JSON
blob enters through the write-only `POST /connections/credentials` API and is
stored in Vault as the single-secret token payload; the bundle only declares the
one field name. `manual_credentials` takes **no** `required_config_fields`
(there is no Helio-side client id/secret), so no `config/` + `deploy/` secret
append is needed — NetSuite is safe to ship hidden with zero environment config,
and there is no `standard_oauth` adapter. Icon: `ui/helio-app/src/integrations/icons/netsuite.svg`
+ manual `providerIcons.ts` registration (never generated). i18n:
`tools.desc.netsuite` + the single `tools.credentialField.netsuite_credentials`
label key across all locales (the label should tell the user to paste the JSON
blob and point at the `setup_url` for its shape).

Generation (batch-end, one run): `provider-gen` + `provider-gen --check`
projects the five generated **files** together (Go catalog, UI fallback, SDK
union, helio-data fixture, check twin); on-branch we run it locally for L3 only
and do **not** commit the projections (master plan §2). "Five files" is the
generator's fixed output set — distinct from the (now single) credential source,
which is just the existing `token.access_token`.

---

## 5. Why no `service/adapter_*.go` on the Helio side

The plan hedged "signing adapter." Concretely it is **not** needed on the Helio
side: `manual_credentials` stores an opaque single-secret payload (the JSON
blob) and sources it to the token gateway with zero provider-specific Go; the
HMAC-SHA256 signing happens in anycli at request time, where the secrets
actually are. Reaching for a `service/adapter_*.go` (the Slack/Discord/X/
LinkedIn precedents) is only justified by non-standard **OAuth token-exchange or
revoke** behavior — NetSuite TBA has neither (no exchange, no revoke). So the
adapter budget the master plan reserved for NetSuite is spent in anycli, not
integration-service.

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
| **L3** `provider-gen --check` + both suites | Bundle strict-decodes (one required `credentials` field passes D5; `credentials: token.access_token` passes the known-credential-source allow-list); five generated files regenerate consistently; `helio-cli` builds against the anycli branch via a local `replace`; integration-service + helio-cli unit suites green. If the stage-2 account-key deriver (§4) is added, its unit test lands here too. **No real creds.** | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with the single JSON-blob secret as the `access_token` (single-secret payload; `test_seed:true`), real seeded org/assistant identities, then `heliox tool netsuite -- query …` reaches the live API through the real token gateway. Seed the blob directly (no `expires_at` — TBA tokens don't expire, like Slack). | **Yes** — same real creds as L2 |
| **L5** full connect flow (once, pre-flip) | The **api_key key-entry** L5 path (master plan §2, not the OAuth path): open the connect link → paste the JSON blob into the single `credentials` field through the real connect UI (stored via `POST /connections/credentials`) → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool netsuite -- query …` succeeds. Agent-drivable (agent-browser) with human fallback. | **Yes** — real creds + the connect UI |

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
  reuse existing single-secret `manual_credentials` (no new vault kind) + signing
  in anycli (no Helio adapter). Recorded here so the pre-verify hedge is closed.
- **Credential shape: single JSON blob, not five per-field sources.** An earlier
  draft projected five `token.<field>` sources and a five-field connect form.
  Verified against the netsuite base, both are unsupported: `knownCredentialSources`
  (`cmd/provider-gen/validate.go`) has no `token.<field>` entries (so
  `provider-gen --check` rejects them), and design 317 D5
  (`model/runtime_contract.go:validateCredentialInputSchema` +
  `service/manual_credential.go:resolveManualSecret`) hard-requires **exactly one**
  required `credential_input` field. The zero-growth path is therefore the mongodb
  single-blob pattern: one `credentials` field → the existing `token.access_token`,
  with anycli decoding the five sub-fields (§2/§4). The **rejected alternative** —
  keeping a five-input form — is genuine integration-service Go growth
  (paypal-style: relax D5 to multi-field, teach `resolveManualSecret` to pack
  the fields, add named `credential.*` sources + `render_symbols.go` symbols +
  `model.CredentialSource` constants + a composite decode path) and is **not** what
  this design ships. If product later insists on the five-input UX, that growth is
  the scope, and §0's zero-source-growth verdict no longer holds.
- **One candidate integration-service growth: the account-key deriver.** `identity.
  source: strategy` needs a deriver that yields a readable `account_key`/label from
  `account_id`. The only shipped `manual_credentials` deriver
  (`service/manual_credentials_identity.go:dsnHostIdentityDeriver`) parses a DSN
  **host out of the secret**; it does not JSON-decode the blob to pull out
  `account_id`. So a small sibling deriver (blob → `account_id`) is expected —
  confirm at stage 2 by reading the 3-hold base (cite it if it already exists, add
  it if not). This is the one place §0's verdict is "small growth, to confirm,"
  not "zero" — stated plainly rather than asserted away.
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
