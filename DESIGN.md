# Braintree — per-tool DESIGN (tool/braintree)

**Catalog row:** #169 · anycli id `braintree` · provider key `braintree` · auth lane `api_key` · wave 3 · category Payments & Commerce.
**Scratch design file** on branch `tool/braintree`, both repos. Batch lead strips it at batch-end.

This doc is the stage-1 design for the `braintree` external tool provider behind `heliox tool braintree`. It follows the `helio-tool-provider` pipeline (10 stages) and the master rollout plan (008-300) §2/§3. Everything below was verified against Braintree's **official** GraphQL API docs and the actual repo code, not assumed.

---

## 0. Audit-verdict reconciliation (independent check)

The 2026-07-21 OAuth audit put Braintree at **`api_key` — "no viable multi-tenant path"**. I re-verified against official docs and **confirm** the lane:

- Braintree's supported, self-serve server integration path is **API keys**: `merchant_id`, `public_key`, `private_key`, obtained from **Control Panel → Settings → API → API Keys**. The GraphQL API authenticates with HTTP Basic `base64(public_key:private_key)`.
- Braintree once shipped **Braintree OAuth / "Braintree Auth"** (a merchant-authorizes-platform flow), but it is a **partner/PayPal-gated** program with no self-serve app registration for arbitrary integrators — exactly the "per-partner, no practical path" case the audit rubric keeps in `api_key`. There is no self-serve multi-tenant authorization-code client we can register.

**Verdict: `api_key` stands. No divergence to record in DESIGN.** (Recorded here per the independent-judgment mandate; no change to the catalog/audit needed.)

Sources: Braintree GraphQL "Making API Calls" (auth = Basic public/private key + `Braintree-Version`), keys sourced from Control Panel Settings→API.

---

## 1. Official API surface wrapped, and why

**Chosen surface: the Braintree GraphQL API** (the modern, JSON-native, publicly documented API), **not** the legacy XML "server SDK" gateway (`api.braintreegateway.com/merchants/<merchant_id>/...`).

Why GraphQL:
- **Agent-friendly / JSON-native.** anycli's built-in service convention targets `--json` structured output for agents (AGENTS.md). The GraphQL API speaks JSON in and JSON out; the legacy API returns XML and is designed to be driven by the per-language SDKs, not called directly — a poor fit for a thin HTTP service.
- **Single endpoint, key-pair auth, versioned.** One URL per environment, HTTP Basic with the API key pair, and a stable `Braintree-Version` date header. No path-templating on `merchant_id`, no XML (de)serialization.
- **Covers the teammate use cases** below in one schema (transactions, customers, disputes, subscriptions, refunds/voids).

**Endpoints (verified):**
- Sandbox: `https://payments.sandbox.braintree-api.com/graphql`
- Production: `https://payments.braintree-api.com/graphql`

**Auth (verified):** every request carries three headers:
- `Authorization: Basic base64(public_key + ":" + private_key)`
- `Braintree-Version: 2019-01-01` (a `YYYY-MM-DD` API version; pinned by the service, not user-supplied)
- `Content-Type: application/json`

Body is the standard `{"query": "...", "variables": {...}}` envelope. GraphQL errors return **HTTP 200 with a top-level `errors[]` array** (and an `extensions.errorClass`), so success detection is body-shaped, not status-shaped — the service must inspect `errors[]`, not just the HTTP code.

### What an AI teammate actually does with Braintree (drives endpoint selection)

A teammate works a merchant's payment operations — read-heavy, with a few money-movement writes:

| Teammate intent | GraphQL operation (verified to exist) |
|---|---|
| "Is the connection alive / are the keys valid?" | `query { ping }` → `"pong"` |
| "Find the transaction for order X / this customer / this date range" | `query { search { transactions(input: TransactionSearchInput) { edges { node { … } } } } }` |
| "Show me transaction `<id>`" | `query { node(id: $id) { ... on Transaction { … } } }` |
| "Refund transaction `<id>` (fully or partial `$amount`)" | `mutation { refundTransaction(input: RefundTransactionInput) { … } }` |
| "Void / reverse transaction `<id>`" | `mutation { reverseTransaction(input: ReverseTransactionInput) { … } }` |
| "Look up customer `<id>` / search customers" | `query { node(id:) { ... on Customer } }`, `search { customers(input: CustomerSearchInput) }` |
| "What disputes are open?" | `query { search { disputes(input: DisputeSearchInput) } }`, dispute `node` |
| "Subscription status for `<id>`" | `query { node(id:) { ... on Subscription } }` |

Deliberately **out of scope for v1**: `chargePaymentMethod` / `authorizePaymentMethod` (creating charges). Creating a charge needs a client-side-collected `paymentMethodId` (client token → hosted fields), which is not something a server-side teammate holds; wiring it would imply a payment-collection UX we don't own. v1 is **operational** (search/inspect/refund/void/dispute), which is the honest teammate surface. A `query` passthrough (below) leaves charge flows reachable for power use without our curating them.

---

## 2. anycli definition (stage-1 rubric)

### Tool form: **`service` type** (not `cli`)

The `cli` rubric requires an official, non-interactive, `--json`-capable binary provisionable into the image. Braintree ships **SDK libraries** (Ruby/Node/Python/…), not an official CLI. So `service` type, implemented in `internal/tools/braintree/` against the GraphQL HTTP API — matching 21 of 23 existing definitions and the `notion`/`x` precedents.

### Definition JSON — `definitions/tools/braintree.json`

Credential fields arrive in the resolver data map and are injected as env vars (mirrors `mongodb.json`'s `connection_string → env`). The service reads four fields:

```json
{
  "name": "braintree",
  "type": "service",
  "description": "Braintree payments operations via the GraphQL API (transactions, refunds, customers, disputes)",
  "auth": {
    "credentials": [
      { "source": {"field": "merchant_id"},  "inject": {"type": "env", "env_var": "BRAINTREE_MERCHANT_ID"} },
      { "source": {"field": "public_key"},    "inject": {"type": "env", "env_var": "BRAINTREE_PUBLIC_KEY"} },
      { "source": {"field": "private_key"},   "inject": {"type": "env", "env_var": "BRAINTREE_PRIVATE_KEY"} },
      { "source": {"field": "environment"},   "inject": {"type": "env", "env_var": "BRAINTREE_ENVIRONMENT"} }
    ]
  }
}
```

Note: `merchant_id` is **not** required by GraphQL auth (the key pair is merchant-scoped), but the service injects it because (a) it is the human-readable account identity Helio derives the account key from, and (b) it future-proofs any legacy-endpoint or reporting call. `environment` (`sandbox`|`production`) selects the host at runtime — the same key pair is only valid against one environment, so it is legitimately part of the credential.

### Go package: `internal/tools/braintree/` (id has no dash → package `braintree`)

Follow the `internal/tools/notion/` shape (the reference implementation): a `Service` struct with `BaseURL`/`HC`/`Out`/`Err` seams so unit tests point at an `httptest` server and capture output; a cobra tree grouped by resource; the documented exit-code contract (0 success, 1 runtime/API failure via a typed `apiError`, 2 usage/parse). Register in `internal/tools/register.go`: `RegisterService("braintree", &braintree.Service{})`.

The service builds one GraphQL client: host from `BRAINTREE_ENVIRONMENT` (default reject if not `sandbox`/`production`), `Authorization: Basic base64(BRAINTREE_PUBLIC_KEY:BRAINTREE_PRIVATE_KEY)`, `Braintree-Version` a package constant. All subcommands POST a `{query, variables}` body; a shared `do(query, vars)` decodes the response, and if `errors[]` is non-empty raises the typed `apiError` (exit 1) with `errors[0].message` + `extensions.errorClass`.

### Subcommands / verbs (cobra tree)

```
braintree ping                         # query { ping }  — health/credential check
braintree transaction search [flags]   # search.transactions (--status --amount-min/max --created-after/before --customer-id --order-id --first N)
braintree transaction get <id>         # node(id) as Transaction
braintree transaction refund <id> [--amount X --order-id O]   # refundTransaction
braintree transaction void <id>        # reverseTransaction
braintree customer get <id>            # node(id) as Customer
braintree customer search [flags]      # search.customers (--email --id ...)
braintree dispute search [flags]       # search.disputes (--status --received-after/before)
braintree dispute get <id>             # node(id) as Dispute
braintree subscription get <id>        # node(id) as Subscription
braintree query <graphql> [--var k=v]  # escape hatch: raw GraphQL query/mutation with typed variables
```

`transaction refund` / `transaction void` are the money-movement verbs; they are ordinary subcommands here — the **approval gate** (design 318, `inspect` action facts, already merged on this worktree base) governs whether the runtime prompts before executing, so anycli does not add its own confirmation.

### JSON output shape

Provider-neutral, agent-first:
- List verbs (`*search*`): `{ "items": [ … normalized nodes … ], "page_info": { "has_next_page": bool, "end_cursor": "…" } }` — flatten GraphQL `edges[].node` into `items`, surface `pageInfo` for cursoring (`--after`).
- Single-object verbs (`get`, `refund`, `void`, `ping`): the normalized object directly (`ping` → `{ "result": "pong" }`).
- Default human-readable table; `--json` emits the structured form. Errors in `--json` mode use notion's structured error envelope (`{ "error": { "message": …, "class": … } }`) at exit 1.
- **Never** echo `private_key`/`public_key` in output or errors.

---

## 3. Credential fields and the exact auth flow

**Lane:** `api_key` (manual, multi-field). No OAuth, no app registration, so **no lane-1 config work** (no `oauth.client_id/secret`, no `config/` + `deploy/` Secret appends). This is the cheapest lane — pure agent throughput.

**Fields the user pastes at connect time** (all four sourced from Control Panel → Settings → API):

| field | secret | notes |
|---|---|---|
| `merchant_id` | false | account identity; also the derived account label root |
| `public_key` | false | Braintree literally names it "public key"; it is the Basic **username** and not sensitive alone |
| `private_key` | **true** | the Basic **password**; the only true secret |
| `environment` | false | `sandbox` or `production`; selects the API host |

**Registration model / token semantics (verified):** API keys are long-lived, non-expiring, revocable only in the Control Panel. There is no refresh cycle and no token exchange — this is a static credential set, which is why the `api_key`/`manual_credentials` path (not `standard_oauth`) is correct. Scopes are **role-based on the merchant side** (the key inherits the API user's role); there is no wire-level scope parameter to request.

**Verification at connect (recommended: verify, don't no-verify).** Unlike `mongodb` (which has no HTTPS identity endpoint, so it stores blindly), Braintree GraphQL exposes a trivially cheap authenticated `query { ping }`. So connect **should** validate the key pair before the Vault write, against the host selected by the `environment` field:

- Build Basic auth from `public_key:private_key`, POST `{"query":"query { ping }"}` to the environment's `/graphql`.
- `200` + `data.ping == "pong"` → valid. HTTP `401`/`403` → reject with `invalid_provider_credential` ("the provider rejected this credential"). Malformed `environment` (not `sandbox`/`production`) → local `manualCredentialFormatError` (never hits the network).
- **Identity derivation (no extra round-trip):** `account_key = merchant_id`, human-readable `account_label = "<merchant_id> (<environment>)"`. Braintree GraphQL has no cheap "whoami" that returns the merchant id, so identity is derived from the **input** (the mongodb `dsnHostIdentityDeriver` philosophy: readable, deterministic, secret-free), while `ping` supplies the validity check the DSN case couldn't.

This is a **narrow, per-provider verifier** in integration-service (`service/provider_registry.go` registry), analogous to existing ones (`sproutClientVerifier`, `postmarkServerVerifier`, `courierBrandsVerifier`). Name it `braintreePingVerifier`. It: parses the multi-field values, validates `environment`, does the `ping`, and returns `(identity{merchant_id, environment}, accountLabel, accountKey=merchant_id)`.

---

## 4. Helio provider bundle plan (axes + integration-service)

### Three-axis naming (no divergence)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `braintree` | bundle `tool.command` (defaults to `tool.name`; ungrouped, so omit) |
| ② anycli tool id | `braintree` | `definitions/tools/braintree.json` |
| ③ provider catalog key | `braintree` | bundle dir `integrations/providers/braintree/` |

②==③, so **no `toolToProvider` entry** and **no `toolGroups` entry** — Braintree is a flat, ungrouped command. (Confirms master-plan §3: only the 24 dashed ids get resolver entries; `braintree` is not one.)

### `integrations/providers/braintree/provider.yaml` (hidden-first)

Shape follows the `mongodb` manual-credentials bundle, extended to multi-field. `auth.type: credentials` (design 317 secret-bearing manual path), `runtime_strategy: manual_credentials`, `identity.source: strategy` (the `braintreePingVerifier` derives the key/label), `disconnect_mode: local_only` (no provider-side revoke — keys are revoked in the Braintree Control Panel).

```yaml
schema: helio.provider/v1
key: braintree
go_name: Braintree

presentation:
  name: Braintree
  description_key: braintree
  consent_domain: braintreepayments.com
  visible: false            # hidden-first; flip is the single go-live change (stage 10)

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - { name: merchant_id, label_key: braintree_merchant_id, secret: false, placeholder: "abc123merchantid", required: true }
      - { name: public_key,  label_key: braintree_public_key,  secret: false, placeholder: "your public key",  required: true }
      - { name: private_key, label_key: braintree_private_key, secret: true,  placeholder: "your private key", required: true }
      - { name: environment, label_key: braintree_environment, secret: false, placeholder: "sandbox or production", required: true }
    setup_url: https://articles.braintreepayments.com/control-panel/important-gateway-credentials

identity:
  source: strategy          # braintreePingVerifier derives account_key = merchant_id

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    merchant_id: token.values.merchant_id     # multi-field projection (see §4.1)
    public_key:  token.values.public_key
    private_key: token.values.private_key
    environment: token.values.environment
    account_key: connection.account_key

tool:
  name: braintree
  kind: api-key             # wire-compat value (317 D2); client routes the drawer by auth_type
```

### 4.1 The one real capability dependency: **multi-field manual credentials**

This is the crux and the only non-mechanical part. On **this worktree base**, `service/manual_credential.go`'s `resolveManualSecret` is **single-secret only** — it asserts `len(CredentialInput.Fields) == 1` and hard-fails otherwise ("credential schema is not single-secret"), and the projection has exactly one source (`token.access_token`, as `mongodb` uses). Braintree needs **four** fields projected as four credential-map entries.

Two of the multi-field manual-credential building blocks were already built on sibling branches earlier in this program (catalog task history): `zoominfo` ("integration-service multi-field manual_credentials capability") and `mixpanel`/`snov`/`zoominfo` multi-field storage. So the plan is:

- **Stage 0 (verify on branch base):** check whether the multi-field `manual_credentials` capability (values-map storage + N-field projection like `token.values.<name>`) has already landed on the dev branch base for this batch. If present, **reuse it** — Braintree becomes a pure bundle + verifier addition with zero new storage code.
- **If not yet present, grow it (Option A, minimal orthogonal):** extend `resolveManualSecret` to accept the schema's N required fields (drop the `==1` assertion; require every declared field non-empty, reject unknown keys — the existing fail-fast semantics generalize cleanly), store the field set as a JSON `values` blob through the existing single `writeUserTokenCredential` access-token payload (no new vault credential kind — same "single opaque secret" storage, the secret is now a small JSON object), and teach the token-gateway projection to source `token.values.<field>` by decoding that blob. This is additive: existing single-field bundles (`mongodb`) keep working because a one-field schema is just N=1.

The bundle above is written against the multi-field projection (`token.values.<name>`); if Stage 0 finds only single-field support, §4.1 Option A is in-scope for this tool's PR (mirroring how `zoominfo`/`servicenow`/`crisp` grew a narrow integration-service capability alongside their bundle).

**Environment field & the manifest:** `credentialFieldManifest` has no enum/select type — only `name/label/secret/placeholder/required`. So `environment` is a **validated free-text field**: the `braintreePingVerifier` rejects anything other than `sandbox`/`production` with a `manualCredentialFormatError` (surfaced as `invalid_provider_credential`). *Open question (small):* whether to add an `enum`/`options` field type to the manifest for closed-set inputs like this; deferred — free-text + validation is sufficient and matches the "reviewed closed capability set, no unbounded YAML" rule.

### Generation + config

- **Generate:** from `go-services/integration-service`, `go run ./cmd/provider-gen` then `--check`; commit all five projections together at batch end (never mid-batch — CI `--check` gate).
- **Config:** none. Manual/`api_key` providers need no integration-service client id/secret, so there are **no `config/` or `deploy/` appends** and no lane-1 landing dependency. A Braintree connection renders `configured: true` with zero server config.
- **UI icon:** `ui/helio-app/src/integrations/icons/braintree.svg` + register in `providerIcons.ts` (manual, never generated). Add i18n `tools.desc.braintree` + the four `tools.credentialField.braintree_*` label keys across all locales.
- **AI-facing docs:** add a `braintree` provider sub-doc under `agents/plugins/heliox/skills/tool/`; bump plugin version + publish at batch end.

---

## 5. Test plan → the five layers

| Layer | What it proves for Braintree | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/braintree` unit tests against an `httptest` GraphQL fake: asserts (a) `Authorization: Basic base64(pub:priv)` header, (b) `Braintree-Version` + `Content-Type` headers, (c) **host selected by `BRAINTREE_ENVIRONMENT`** (sandbox vs production; reject unknown), (d) each verb's query/variables shape, (e) `errors[]`-in-200 mapped to typed `apiError` exit 1, (f) `edges[].node`→`items`+`page_info` flattening, (g) secrets never printed. Definition JSON loads via `LoadBundled`. | No — all faked |
| **L2** `anycli braintree -- ping` / `-- transaction search …` harness | Real **sandbox** GraphQL API with `ANYCLI_CRED_MERCHANT_ID` / `_PUBLIC_KEY` / `_PRIVATE_KEY` / `_ENVIRONMENT=sandbox`. Proves field names, injection, host, and query shapes match the live API. Mandatory before the pin bump. | **Yes** — a Braintree **sandbox** account's merchant_id + public/private key |
| **L3** `provider-gen --check` + both repos' suites | Bundle strict-decodes; five projections regenerate clean; `braintreePingVerifier` unit tests (valid `ping`→ok, 401→reject, bad `environment`→format error, identity = merchant_id/label); multi-field `resolveManualSecret` tests if §4.1 Option A is exercised; helio-cli `go build ./... && go test ./cmd/heliox/cmds/tool/` with a local `replace` to this anycli branch. | No |
| **L4** singleton + seed + `heliox tool braintree -- …` | `POST /internal/test-only/connections/seed` a Braintree connection (multi-field `values`, `api_key`/`credentials` type is seedable — it's a user-token provider, not minted), then `heliox tool braintree -- ping` and `-- transaction search` through the real token gateway → live sandbox API. Proves the projected credential map reaches anycli and authenticates. | **Yes** — sandbox creds seeded (no refresh cycle → seed the static values, no `expires_at`) |
| **L5** connect UI (api_key key-entry path) + one unseeded live run | Master-plan §2 api_key L5: open connect link → paste the four fields in the real CredentialForm → `POST /connections/credentials` verifies via `braintreePingVerifier` → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool braintree -- ping` (or `transaction get`) through the real token gateway succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. Runs once, still hidden, before the visible flip. | **Yes** — sandbox creds entered through the UI |

**External-credential summary:** L2, L4, L5 all require a **Braintree sandbox account** (free, self-serve at `sandbox.braintree.gateway` signup) yielding `merchant_id` + `public_key` + `private_key`. This is the account-pool lane's only Braintree dependency (no production account needed for the hidden→L5 path; production is only exercised by a real user post-flip).

---

## 6. Rollout

Hidden-first (`visible: false`), through L1–L5 while hidden, then the single go-live change = `presentation.visible: true` + `provider-gen` regenerate, gated on L5 passing (no review clearance needed — `api_key` has no review lane). Wave 3, api_key sub-batch (majority agent throughput).

**Stage-1 flags for the batch lead:**
1. **Multi-field manual credentials** is the only capability dependency (§4.1) — verify it's on the branch base at stage 0; if not, this tool's PR grows the narrow capability (Option A). This is the same "verify capability, grow if absent" pattern zoominfo/mixpanel/servicenow used.
2. **No lane-1 / config work** — api_key, no OAuth app, no `config/`+`deploy/` appends.
3. **v1 scope is operational** (search/inspect/refund/void/dispute + `query` passthrough); charge-creation flows are intentionally excluded (client-token/paymentMethodId dependency).

---

## Sources (official / verified)

- Braintree GraphQL — Making API Calls (auth = HTTP Basic `base64(public_key:private_key)`, `Braintree-Version: YYYY-MM-DD`, `Content-Type: application/json`; endpoints `payments[.sandbox].braintree-api.com/graphql`; keys from Control Panel Settings→API): https://developer.paypal.com/braintree/graphql/guides/making_api_calls/
- Braintree GraphQL — Transactions guide (`search`, `refundTransaction`, `reverseTransaction`, `chargePaymentMethod`, `TransactionSearchInput`): https://developer.paypal.com/braintree/graphql/guides/transactions/
- Braintree GraphQL — Reference/Explorer (`ping`, `node`, search inputs): https://developer.paypal.com/braintree/graphql/reference/
- Repo precedents: `internal/tools/mongodb/` + `definitions/tools/mongodb.json` (manual multi-secret env injection), `internal/tools/notion/` (service shape / exit codes / `--json`), `integrations/providers/mongodb/provider.yaml` (manual_credentials bundle), `go-services/integration-service/service/manual_credential.go` + `manual_credentials_identity.go` (single-secret base + deriver pattern), `helio-cli/internal/toolcred/resolver.go` (no divergence needed).
