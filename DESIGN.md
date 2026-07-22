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

**Verdict: BUILD, api_key lane confirmed, `service` type, NO new
integration-service capability expected (reuse multi-field
`manual_credentials`), one new anycli concern (OAuth 1.0a request signing
inside the service package).**

The master plan flagged three risks for NetSuite; each is resolved here:

1. **Non-standard auth shape (§6).** Confirmed real. NetSuite Token-Based
   Authentication (TBA) is OAuth **1.0a** per-request signing with HMAC-SHA256
   over **four** secrets plus the account id — not a bearer key. This is
   handled entirely inside the anycli `netsuite` service package (stdlib
   `crypto/hmac`+`crypto/sha256` signing); it needs **no** new anycli
   credential-injection primitive (5 plain env-var bindings) and **no** signing
   logic on the Helio side. The credential *shape* (5 fields, one non-secret)
   is served by the existing multi-field `manual_credentials` capability
   (grown on main by zoominfo / paypal / braintree / mixpanel before 3-hold
   runs — see §4). The plan's "distinct vault credential kind and/or signing
   adapter" hedge resolves to **neither**: the vault stores an opaque
   multi-field payload (already supported), and the "signing adapter" lives in
   anycli, not in a Helio `service/adapter_*.go`.

2. **Deprecation clock (§6).** Confirmed and re-confirmed: from release
   **2027.1** NetSuite will not allow **new** TBA integrations to be created
   (Oracle: *"as of 2027.1, no new integrations using TBA can be created for
   SOAP web services, REST web services, and RESTlets"*); **existing** TBA
   integrations keep working. The OAuth 2.0 alternative stays unattractive for
   a shared server-to-server client (short-lived access tokens + refresh, and
   authorization-code now requires PKCE). Because the deprecation blocks only
   *new integration records* and our TBA integration/consumer app is created
   **once** (well before 2027.1) and reused across all customer accounts, TBA
   is the right lane for the shipping horizon. §8 records the OAuth-2.0 future
   path so this is a deliberate, revisitable choice.

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
`1234567`). Verified: the URL subdomain uses the **hyphen** form while the
OAuth **realm** uses the **underscore** form of the same account id — this
asymmetry is a classic TBA foot-gun and the service package owns the transform.

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
- Governance: 10 concurrent requests/account; HTTP **429** with `Retry-After`.
  The service maps 429 to the runtime-error exit code and echoes `Retry-After`
  in the `--json` error envelope so the agent can back off (it does **not**
  auto-retry/block — fail-fast per the repo hard rule).

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

`definitions/tools/netsuite.json` (5 credential bindings, all `env`):

```json
{
  "name": "netsuite",
  "type": "service",
  "description": "NetSuite ERP as a tool — SuiteTalk REST records + SuiteQL (Token-Based Auth)",
  "auth": {
    "credentials": [
      { "source": {"field": "account_id"},      "inject": {"type": "env", "env_var": "NETSUITE_ACCOUNT_ID"} },
      { "source": {"field": "consumer_key"},     "inject": {"type": "env", "env_var": "NETSUITE_CONSUMER_KEY"} },
      { "source": {"field": "consumer_secret"},  "inject": {"type": "env", "env_var": "NETSUITE_CONSUMER_SECRET"} },
      { "source": {"field": "token_id"},         "inject": {"type": "env", "env_var": "NETSUITE_TOKEN_ID"} },
      { "source": {"field": "token_secret"},     "inject": {"type": "env", "env_var": "NETSUITE_TOKEN_SECRET"} }
    ]
  }
}
```

Go package: `internal/tools/netsuite/` (id has no dashes/leading digit, so
package name == id). Registered as `RegisterService("netsuite",
&netsuite.Service{})` in `internal/tools/register.go` (the one shared-registry
merge held to batch-end per master plan §2).

### Service structure (copy `internal/tools/notion/` shape)

`Service{ BaseURL string; HC *http.Client; Out, Err io.Writer }` so unit tests
point `BaseURL` at an `httptest.Server` and capture output. `Execute(ctx,
args, env)` reads the 5 fields from the `env` map (services read the injected
`env map`, not `os.Getenv` — verified against notion/slack), fails fast with a
clear message if any required field is empty, then dispatches a cobra tree.

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
- Emit header: `Authorization: OAuth realm="<ACCOUNT_ID underscore form>",
  oauth_consumer_key="…", oauth_token="…", oauth_signature_method="HMAC-SHA256",
  oauth_timestamp="…", oauth_nonce="…", oauth_version="1.0",
  oauth_signature="<rfc3986(sig)>"`.
- Host = `strings.ToLower(strings.ReplaceAll(accountID, "_", "-"))`; realm =
  the account id **as entered** (underscore/upper form). Both derived from the
  single `NETSUITE_ACCOUNT_ID` field.

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
JSON envelope under `--json`, plain text otherwise; 429 carries `retry_after`.

---

## 3. Credential fields and exact auth flow (TBA / OAuth 1.0a)

TBA is a **manual credential** flow (api_key lane) — there is no browser
consent redirect; the user pastes credentials they generate inside NetSuite.
Verified against Oracle's TBA setup docs, the required inputs are exactly:

| Field | Secret | Purpose | Where the user gets it |
|---|---|---|---|
| `account_id` | no | Realm (underscore form) + URL host (hyphen form) | Setup → Company → Company Information (Account ID); or Company URLs → SuiteTalk |
| `consumer_key` | yes | Integration record consumer key | Setup → Integration → Manage Integrations → New (enable TBA); shown once |
| `consumer_secret` | yes | Integration record consumer secret | same integration record; shown once |
| `token_id` | yes | Access token id | Setup → Users/Roles → Access Tokens → New (or "Manage Access Tokens") |
| `token_secret` | yes | Access token secret | same access-token page; shown once |

Registration model (why this is api_key, not oauth): the NetSuite admin creates
**one integration record** (consumer key/secret) and **one access token** (token
id/secret) bound to a role with "REST Web Services" + "Log in using Access
Tokens" permissions, inside **their own** NetSuite account. There is no
multi-tenant authorize endpoint we register a Helio app against — each customer
mints their own four secrets. That is precisely the rubric's "OAuth is
per-instance / impractical for a shared client → stays api_key" (oauth-audit row
149: *"no viable multi-tenant path → api_key"*). Confirmed correct.

Token semantics: TBA tokens **do not expire** (valid until revoked or the
user/role changes) — so there is no refresh cycle; seed/serve the four secrets
directly (like Slack's bot token, per the L4 "non-expiring token" guidance).

Per-request auth flow at runtime:
1. Token gateway serves the stored multi-field payload → helio-cli resolver →
   anycli credential map → 5 env vars.
2. anycli `netsuite` service derives host+realm from `account_id`, signs each
   HTTP request with HMAC-SHA256 over the four secrets, sets `Prefer: transient`
   for SuiteQL, and calls `*.suitetalk.api.netsuite.com`.

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

Bundle shape follows the **`mongodb` manual-credentials precedent** extended to
five fields (multi-field `manual_credentials`, the capability grown on main by
zoominfo / paypal / braintree / mixpanel — so on the 3-hold base this is
already available; **no integration-service Go growth expected**, re-verify at
stage 2):

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
      - { name: account_id,      label_key: netsuite_account_id,      secret: false, required: true,  placeholder: "1234567 or 1234567_SB1" }
      - { name: consumer_key,     label_key: netsuite_consumer_key,     secret: true,  required: true }
      - { name: consumer_secret,  label_key: netsuite_consumer_secret,  secret: true,  required: true }
      - { name: token_id,         label_key: netsuite_token_id,         secret: true,  required: true }
      - { name: token_secret,     label_key: netsuite_token_secret,     secret: true,  required: true }
    setup_url: https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_4381113277.html

identity:
  source: strategy          # no HTTPS userinfo; account key derived from account_id
  # account_key/label = the account_id field (a named-field deriver). If the
  # strategy deriver on main cannot select a named input field yet, that is the
  # ONE small capability to confirm/grow at stage 2 (precedent: servicenow
  # endpoint-field capture, zoominfo multi-field). Re-verify before coding.

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
    account_id:      token.account_id
    consumer_key:    token.consumer_key
    consumer_secret: token.consumer_secret
    token_id:        token.token_id
    token_secret:    token.token_secret
    account_key:     connection.account_key

tool:
  name: netsuite
  kind: api-key           # wire-compat value (design 317 D2)
```

Nothing secret lives in the bundle (per `references/provider-yaml.md`): the four
secrets enter through the write-only `POST /connections/credentials` API and are
stored in Vault as a multi-field payload; the bundle only declares field names.
`manual_credentials` takes **no** `required_config_fields` (there is no
Helio-side client id/secret), so no `config/` + `deploy/` secret append is
needed — NetSuite is safe to ship hidden with zero environment config, and there
is no `standard_oauth` adapter. Icon: `ui/helio-app/src/integrations/icons/netsuite.svg`
+ manual `providerIcons.ts` registration (never generated). i18n:
`tools.desc.netsuite` + the five `tools.credentialField.netsuite_*` label keys
across all locales.

Generation (batch-end, one run): `provider-gen` + `provider-gen --check`
projects the five files together; on-branch we run it locally for L3 only and
do **not** commit the projections (master plan §2).

---

## 5. Why no `service/adapter_*.go` on the Helio side

The plan hedged "signing adapter." Concretely it is **not** needed on the Helio
side: `manual_credentials` stores an opaque multi-field secret payload and
sources each field to the token gateway with zero provider-specific Go; the
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
| **L1** anycli `go test ./...` | Signature-base-string + header exactness against known TBA test vectors; host/realm transform (`_`→`-`, lowercasing, realm preserves underscore/case); SuiteQL sets `Prefer: transient` and folds `limit`/`offset` into the signature base; record CRUD request shape (method/path/body); 429 → `retry_after` in `--json`; exit-code 0/1/2 matrix — all against an `httptest.Server` fake. **No real API.** | No |
| **L2** dev harness vs real API | `ANYCLI_CRED_ACCOUNT_ID=… ANYCLI_CRED_CONSUMER_KEY=… …(4 more) anycli netsuite -- query --q "SELECT id, companyname FROM customer FETCH FIRST 5 ROWS ONLY"` returns real rows; a `record get --type customer --id <id>` returns a real record. Proves field names, signing, and the host/realm transform match the **live** account. | **Yes** — a real NetSuite account/sandbox from the account pool |
| **L3** `provider-gen --check` + both suites | Bundle strict-decodes; five projections regenerate consistently; `helio-cli` builds against the anycli branch via a local `replace`; integration-service + helio-cli unit suites green. **No real creds.** | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with the five fields (`access_token`-less multi-field payload; `test_seed:true`), real seeded org/assistant identities, then `heliox tool netsuite -- query …` reaches the live API through the real token gateway. Seed the four secrets directly (no `expires_at` — TBA tokens don't expire, like Slack). | **Yes** — same real creds as L2 |
| **L5** full connect flow (once, pre-flip) | The **api_key key-entry** L5 path (master plan §2, not the OAuth path): open the connect link → paste account id + 4 secrets through the real connect UI (stored via `POST /connections/credentials`) → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool netsuite -- query …` succeeds. Agent-drivable (agent-browser) with human fallback. | **Yes** — real creds + the connect UI |

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
  reuse existing multi-field `manual_credentials` (no new vault kind) + signing
  in anycli (no Helio adapter). Recorded here so the pre-verify hedge is closed.
- **OAuth 2.0 future path (deprecation-driven, deferred).** From 2027.1 no new
  TBA integration *records* can be created. Our single reusable integration/
  consumer app is created once, pre-2027.1, so TBA remains valid for existing
  customers and for our shared client indefinitely; but if NetSuite ever forces
  migration, the successor is an `oauth_review`-lane `standard_oauth` bundle
  (authorization-code + **PKCE**, short-lived access token + refresh, account-
  specific token host). That is a future re-lane, not v1 work. Flagged, not
  built.
- **Deferred v1 scope:** async SuiteQL (`Prefer: respond-async` + job polling),
  external-id record addressing (`/record/v1/{type}/eid:{externalId}`),
  subrecord/sublist mutation, and `?replace=` upsert semantics. Sync transient
  SuiteQL + internal-id CRUD cover the teammate use cases; the deferred surface
  is additive later without breaking the definition.
- **Governance is fail-fast, not auto-retry.** 429/`Retry-After` is surfaced to
  the agent, not swallowed by an in-CLI backoff loop (repo hard rule: no silent
  fallback/blocking inside one invocation).
