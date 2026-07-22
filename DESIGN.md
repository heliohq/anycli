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
- `Braintree-Version: 2025-06-01` (a `YYYY-MM-DD` API version; pinned by the service as a package constant, not user-supplied — see the version-pin policy below)
- `Content-Type: application/json`

**Braintree-Version pin policy (verified against the changelog).** The header is a *date*, and the API returns the schema **as of that date** — an old pin hides later schema. The official guidance is "use the date on which you begin integrating." An old placeholder like `2019-01-01` is wrong for this tool: the Braintree changelog adds the `Dispute` type on **2019-10-03** and `DisputeSearchInput` / `search.disputes` and subscription-node fields later still (earliest changelog entry is 2019-03-18; latest is 2026-07-14). A pre-dispute pin would make `braintree dispute search` and parts of `subscription get` dead on arrival with unknown-field / unavailable-type errors in `errors[]` even on valid credentials. So the pin is a **current** date (integration date, `2025-06-01` here) chosen so **all** in-scope operations — `ping`, transaction search/get/refund/reverse, customer, dispute search, subscription — resolve under one schema. The version is a single package constant with the bump policy recorded next to it; L1 pins a query per verb against the fake and L2 asserts each in-scope verb's fields resolve under the pinned version against the live sandbox (§5).

Body is the standard `{"query": "...", "variables": {...}}` envelope. GraphQL errors return **HTTP 200 with a top-level `errors[]` array** (and an `extensions.errorClass`), so success detection is body-shaped, not status-shaped — the service must inspect `errors[]`, not just the HTTP code.

### What an AI teammate actually does with Braintree (drives endpoint selection)

A teammate works a merchant's payment operations — read-heavy, with a few money-movement writes:

| Teammate intent | GraphQL operation (verified to exist) |
|---|---|
| "Is the connection alive / are the keys valid?" | `query { ping }` → `"pong"` |
| "Find the transaction for order X / this customer / this date range" | `query { search { transactions(input: TransactionSearchInput) { edges { node { … } } } } }` |
| "Show me transaction `<id>`" | `query { node(id: $id) { ... on Transaction { … } } }` |
| "Refund transaction `<id>` (fully or partial `$amount`)" | `mutation { refundTransaction(input: RefundTransactionInput) { … } }` |
| "Void transaction `<id>` (unsettled only — **errors** if already settled)" | `mutation { voidTransaction(input: VoidTransactionInput) { … } }` |
| "Reverse transaction `<id>` (void if unsettled, **full refund** if settled)" | `mutation { reverseTransaction(input: ReverseTransactionInput) { … } }` |
| "Look up customer `<id>` / search customers" | `query { node(id:) { ... on Customer } }`, `search { customers(input: CustomerSearchInput) }` |
| "What disputes are open?" | `query { search { disputes(input: DisputeSearchInput) } }`, dispute `node` |
| "Subscription status for `<id>`" | `query { node(id:) { ... on Subscription } }` |

Deliberately **out of scope for v1**: `chargePaymentMethod` / `authorizePaymentMethod` (creating charges). Creating a charge needs a client-side-collected `paymentMethodId` (client token → hosted fields), which is not something a server-side teammate holds; wiring it would imply a payment-collection UX we don't own. v1 is **operational** (search/inspect/refund/void/reverse/dispute), which is the honest teammate surface. A **read-only** `query` passthrough (below) leaves arbitrary *read* shapes reachable for power use without our curating every field; it deliberately does **not** re-open charge/refund/void write paths (see the mutation-rejection rule in §2).

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
braintree transaction refund <id> [--amount X --order-id O]   # refundTransaction (settled only)
braintree transaction void <id>        # voidTransaction — void-only; ERRORS if already settled
braintree transaction reverse <id>     # reverseTransaction — void if unsettled, FULL REFUND if settled
braintree customer get <id>            # node(id) as Customer
braintree customer search [flags]      # search.customers (--email --id ...)
braintree dispute search [flags]       # search.disputes (--status --received-after/before)
braintree dispute get <id>             # node(id) as Dispute
braintree subscription get <id>        # node(id) as Subscription
braintree query <graphql> [--var k=v]  # escape hatch: raw GraphQL — READ-ONLY (mutations rejected, exit 2)
```

`transaction refund` / `transaction void` / `transaction reverse` are the money-movement verbs; they are ordinary subcommands here — the **approval gate** (design 318, `inspect` action facts, already merged on this worktree base) governs whether the runtime prompts before executing, so anycli does not add its own confirmation.

**Why `void` and `reverse` are two separate verbs (not one).** Braintree's official Transactions guide documents these as **distinct mutations with distinct semantics**: `voidTransaction` cancels a transaction *only* while it is unsettled (authorized / submitted-for-settlement) and **errors** ("Transaction cannot be voided in its current state") once it has settled — it never moves money that already left. `reverseTransaction` is the *universal reversal*: it voids an unsettled transaction but issues a **full refund** on an already-**settled** one. Mapping the verb literally named `void` to `reverseTransaction` would mean invoking `braintree transaction void <settled-id>` **silently issues a full refund** — a real, surprising money-movement the teammate did not ask for, and exactly the kind of unexpected charge-path the "no charge-creation" scope decision exists to avoid. So `void` maps to `voidTransaction` (void-only, honest failure on settled), and the universal-reverse convenience is exposed under its own explicit name `reverse` → `reverseTransaction`, whose help text and `inspect` action facts state plainly that it issues a full refund on settled transactions. The approval gate then reasons over two truthful verbs instead of one mislabeled one.

**Why the `query` passthrough is read-only in v1.** The whole point of curating money-movement into named verbs (`refund` / `void` / `reverse`) is that the design-318 approval gate reasons about each via structured `inspect` action facts before any funds move. A raw passthrough that accepted arbitrary GraphQL would re-admit `chargePaymentMethod` / `refundTransaction` / `reverseTransaction` under the benign-looking `query` command — a money-moving mutation that would **not** surface the same structured approval-gate facts the curated verbs do, i.e. a confirmation-bypass relative to the very surface we curated. So for v1 the passthrough is **read-only**: the service parses the supplied GraphQL and **rejects any operation whose type is `mutation`** (or an anonymous selection that resolves to the mutation root) with a usage error (exit 2, no network call). Reads (`query { … }`) pass through unmodified. This keeps `query` as a genuine field-coverage escape hatch for inspection without letting it become an un-gated write channel; if a future version wants curated passthrough writes, they must route through named verbs (or an explicitly gate-classified mutation path), not this hatch.

### JSON output shape

Provider-neutral, agent-first:
- List verbs (`*search*`): `{ "items": [ … normalized nodes … ], "page_info": { "has_next_page": bool, "end_cursor": "…" } }` — flatten GraphQL `edges[].node` into `items`, surface `pageInfo` for cursoring (`--after`).
- Single-object verbs (`get`, `refund`, `void`, `reverse`, `ping`): the normalized object directly (`ping` → `{ "result": "pong" }`).
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
- **Identity derivation (no extra round-trip):** `account_key = "<environment>/<merchant_id>"` (environment-qualified — see below), human-readable `account_label = "<merchant_id> (<environment>)"`. Braintree GraphQL has no cheap "whoami" that returns the merchant id, so identity is derived from the **input** (the mongodb `dsnHostIdentityDeriver` philosophy: readable, deterministic, secret-free), while `ping` supplies the validity check the DSN case couldn't.
- **Why the key is environment-qualified:** the `(org, provider, account_key)` index is **non-unique** and manual connections dedup by `account_key` (see integration-service AGENTS "AccountKey uniqueness"). A user can connect the *same* `merchant_id` string in sandbox and production (Braintree test/prod ids are normally distinct, but reuse is possible). If `account_key` were bare `merchant_id`, the second connect would upsert over the first and silently rebind one environment's credential onto the other's row. Qualifying the key with `environment` keeps the two genuinely-different accounts as two rows; the label already carries the environment, so key and label stay consistent.

**This is NOT a drop-in of an existing verifier — it is a capability growth.** On this worktree base, `composeProviderRegistration` hardwires **every** `runtime_strategy: manual_credentials` provider to `dsnHostIdentityDeriver{}` (`provider_registry.go:88-98`), which is by design 317 D5/OQ1 the **NO-VERIFY** path: it does `url.Parse` on the stored secret and derives an account key from the DSN host, issuing **no** provider request. There is no per-provider verifier selection for `manual_credentials`. The two existing manual verifiers both fail Braintree:
- `dsnHostIdentityDeriver` — no-verify; `url.Parse(<JSON values blob>)` yields `host == ""` and returns `manualCredentialFormatError` ("requires a connection string with a host"), so a Braintree connect would be **rejected outright and no ping would ever run**.
- `declarativeManualTokenVerifier` — a **GET** to a bundle-declared identity URL with a **single** bundle-declared header; Braintree needs a **GraphQL POST** with `Basic(public:private)`, a JSON body, and body-shaped (`errors[]`) error detection. It does not fit.

So Braintree needs a **bespoke `braintreePingVerifier`** (GraphQL `ping` POST + Basic auth + `errors[]`-shaped failure + input-derived environment-qualified identity) **plus** a registry-dispatch change so `manual_credentials` can select a per-provider verifier instead of the unconditional `dsnHostIdentityDeriver`. Both the verifier and the dispatch change are load-bearing capability growth for this tool — counted as integration-service growth #2 in §4.1, not an incidental addition. The bundle therefore **cannot** simply set `identity.source: strategy` and expect a ping: absent the dispatch change, `strategy` still resolves to the no-verify DSN deriver. `braintreePingVerifier` parses the multi-field values, validates `environment`, POSTs `ping`, and returns `(identity{merchant_id, environment}, accountLabel="<merchant_id> (<environment>)", accountKey="<environment>/<merchant_id>")`.

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
  source: strategy          # resolves to braintreePingVerifier ONLY after the §4.1 registry-dispatch
                            # change; on the base, manual_credentials → dsnHostIdentityDeriver (no-verify).
                            # Verifier derives account_key = "<environment>/<merchant_id>" (env-qualified)

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

### 4.1 Two integration-service capability dependencies

Braintree is **not** a pure bundle addition. It needs **two** growths in integration-service, and the second is substantial — do not scope this as "multi-field is the only dependency."

**Growth #1 — multi-field manual credential storage/projection (Option A).** On **this worktree base**, `service/manual_credential.go`'s `resolveManualSecret` is **single-secret only** — it asserts `len(CredentialInput.Fields) == 1` and hard-fails otherwise ("credential schema is not single-secret"), and the projection has exactly one source (`token.access_token`, as `mongodb` uses). Braintree needs **four** fields projected as four credential-map entries.

- **Stage 0 (verify on branch base):** check whether the multi-field `manual_credentials` capability (values-map storage + N-field projection like `token.values.<name>`) has already landed on the dev branch base for this batch (`zoominfo`/`mixpanel`/`snov` grew pieces of it earlier). If present, reuse it — Growth #1 collapses to zero new storage code.
- **If not yet present, grow it (Option A, minimal orthogonal):** extend `resolveManualSecret` to accept the schema's N required fields (drop the `==1` assertion; require every declared field non-empty, reject unknown keys — the existing fail-fast semantics generalize cleanly), store the field set as a JSON `values` blob through the existing single `writeUserTokenCredential` access-token payload (no new vault credential kind — same "single opaque secret" storage, the secret is now a small JSON object), and teach the token-gateway projection to source `token.values.<field>` by decoding that blob. This is additive: existing single-field bundles (`mongodb`) keep working because a one-field schema is just N=1.

**Growth #2 — a verifying manual path for Braintree (the load-bearing one).** This is the crux the reviewer flagged, and it is easy to under-scope. On this base, `composeProviderRegistration` binds **every** `runtime_strategy: manual_credentials` provider unconditionally to `dsnHostIdentityDeriver{}` (`provider_registry.go:88-98`). By design 317 D5/OQ1 `manual_credentials` **means no-verify**: a DSN has no HTTPS identity endpoint, so the base contract stores the secret blindly and derives the account key from the DSN host, issuing no provider request. There is **no per-provider verifier selection** for `manual_credentials`. Consequences the bundle alone cannot escape:

- If we ship the bundle as `runtime_strategy: manual_credentials` + `identity.source: strategy` with **no** registry change, `strategy` still resolves to `dsnHostIdentityDeriver`. At connect, `resolveManualSecret` returns the JSON `values` blob, the deriver runs `url.Parse(blob)` → `host == ""` → `manualCredentialFormatError` ("requires a connection string with a host"), and **every** Braintree connect is rejected — no ping ever runs. This is the exact failure the audit describes.
- Neither existing manual verifier fits Braintree: `dsnHostIdentityDeriver` is no-verify DSN; `declarativeManualTokenVerifier` is a **GET** to one declared URL with **one** declared header, whereas Braintree needs a **GraphQL POST** with `Basic(public:private)`, a JSON body, and body-shaped (`errors[]`-in-200) success detection.

- **Stage 0 (verify on branch base — same reuse discipline as Growth #1, do NOT skip):** the two load-bearing pieces of Growth #2 are (i) the **dispatch mechanism** that lets `manual_credentials` (or a sibling strategy) pick a **per-provider** verifier/deriver instead of the hardwired `dsnHostIdentityDeriver`, and (ii) the **Braintree-specific verifier** itself. Piece (i) is the reusable one, and this batch has been **repeatedly** adding exactly this shape of per-provider verifier/deriver selection for the manual path — e.g. amplitude's `deriver-selection`, crisp's keypair identity deriver, iterable's `region_prefix` deriver + deriver-selection, mailerlite's identity deriver, and zoominfo's multi-field `manual_credentials` capability. So **before** building any dispatch, grep the accumulated branch base (`provider_registry.go` `composeProviderRegistration` + `manual_credentials`/`api_key` composition, `manual_token_verifier.go`, `manual_credentials_identity.go`, any deriver-selection map) to check whether a per-provider verifier/deriver **selection mechanism** for the manual credential path has already landed. **If it has, reuse it** — Growth #2 collapses to *only* piece (ii): write `braintreePingVerifier` and register it as the `braintree` entry in the existing selection map; do **not** add a second, parallel dispatch path (that would be the "adding a discriminator to an overloaded model" smell the code-health rules call out). **Only if no such selection exists on the base** do you add the dispatch itself (one of the two decompositions below).

So — **absent an existing selection mechanism** — Growth #2 is a **bespoke `braintreePingVerifier`** (GraphQL `ping` POST + Basic auth + `errors[]`-shaped failure + input-derived environment-qualified identity) **plus** a registry-dispatch change so `manual_credentials` can select a per-provider verifier instead of the hardwired `dsnHostIdentityDeriver`. Two orthogonal decompositions of that dispatch are acceptable; pick one and record it:

1. **Per-provider verifier dispatch in `composeProviderRegistration`** for the `manual_credentials` case (e.g. a small provider→verifier map, default `dsnHostIdentityDeriver` for the DSN providers, `braintreePingVerifier` for `braintree`), keeping the single `manual_credentials` strategy; **or**
2. **A new verifying multi-field manual runtime strategy** (e.g. `manual_credentials_verified`) whose registration selects the per-provider verifier, leaving the existing `manual_credentials` = no-verify contract untouched.

Either way, state plainly in the PR that `manual_credentials`' **base** contract is no-verify (317 D5/OQ1) and that the ping is only reachable through this new dispatch — the bundle's `identity.source: strategy` is necessary but **not sufficient** on its own. `braintreePingVerifier` parses the multi-field values, validates `environment` (`sandbox`/`production`, else `manualCredentialFormatError`), POSTs `ping`, and returns `(identity{merchant_id, environment}, accountLabel="<merchant_id> (<environment>)", accountKey="<environment>/<merchant_id>")`.

**Environment field & the manifest:** `credentialFieldManifest` has no enum/select type — only `name/label/secret/placeholder/required`. So `environment` is a **validated free-text field**: `braintreePingVerifier` rejects anything other than `sandbox`/`production` with a `manualCredentialFormatError` (surfaced as `invalid_provider_credential`). *Open question (small):* whether to add an `enum`/`options` field type to the manifest for closed-set inputs like this; deferred — free-text + validation is sufficient and matches the "reviewed closed capability set, no unbounded YAML" rule.

### Generation + config

- **Generate:** from `go-services/integration-service`, `go run ./cmd/provider-gen` then `--check`; commit all five projections together at batch end (never mid-batch — CI `--check` gate).
- **Config:** none. Manual/`api_key` providers need no integration-service client id/secret, so there are **no `config/` or `deploy/` appends** and no lane-1 landing dependency. A Braintree connection renders `configured: true` with zero server config.
- **UI icon:** `ui/helio-app/src/integrations/icons/braintree.svg` + register in `providerIcons.ts` (manual, never generated). Add i18n `tools.desc.braintree` + the four `tools.credentialField.braintree_*` label keys across all locales.
- **AI-facing docs:** add a `braintree` provider sub-doc under `agents/plugins/heliox/skills/tool/`; bump plugin version + publish at batch end.

---

## 5. Test plan → the five layers

| Layer | What it proves for Braintree | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/braintree` unit tests against an `httptest` GraphQL fake: asserts (a) `Authorization: Basic base64(pub:priv)` header, (b) `Braintree-Version` (the pinned constant) + `Content-Type` headers, (c) **host selected by `BRAINTREE_ENVIRONMENT`** (sandbox vs production; reject unknown), (d) each verb's query/variables shape (one query pinned per verb, so a schema-field rename or a version regression surfaces here) — **including that `transaction void` emits `voidTransaction` and `transaction reverse` emits `reverseTransaction` (never the reverse mapping)**, (e) `errors[]`-in-200 mapped to typed `apiError` exit 1, (f) `edges[].node`→`items`+`page_info` flattening, (g) secrets never printed, (h) **`query` rejects a `mutation` operation locally with exit 2 and issues no HTTP request, while a `query { … }` read passes through.** Definition JSON loads via `LoadBundled`. | No — all faked |
| **L2** `anycli braintree -- ping` / `-- transaction search …` harness | Real **sandbox** GraphQL API with `ANYCLI_CRED_MERCHANT_ID` / `_PUBLIC_KEY` / `_PRIVATE_KEY` / `_ENVIRONMENT=sandbox`. Proves field names, injection, host, and query shapes match the live API — **including an explicit assertion that every in-scope verb (`ping`, transaction search/get/refund/void/reverse, customer get/search, dispute search/get, subscription get) resolves with no `errors[]` unknown-field/unavailable-type entry under the pinned `Braintree-Version` — confirming both `voidTransaction` and `reverseTransaction` are distinct, present mutations.** If any verb's type/field is absent at the pin, bump the pin to a date that exposes all of them and re-run. Mandatory before the pin bump. | **Yes** — a Braintree **sandbox** account's merchant_id + public/private key |
| **L3** `provider-gen --check` + both repos' suites | Bundle strict-decodes; five projections regenerate clean; **both integration-service growths tested** — (a) `braintreePingVerifier` unit tests (valid `ping`→ok; 401/403→`invalid_provider_credential`; `errors[]`-in-200→reject; bad `environment`→`manualCredentialFormatError`; identity `accountKey="<environment>/<merchant_id>"` + label `"<merchant_id> (<environment>)"`, and a **same-`merchant_id`-different-`environment`** case proving two distinct account keys), (b) the registry-dispatch change (`manual_credentials`/new-strategy selects `braintreePingVerifier` for `braintree` and still selects `dsnHostIdentityDeriver` for the DSN providers), (c) multi-field `resolveManualSecret` tests (§4.1 Growth #1, if not already on base); helio-cli `go build ./... && go test ./cmd/heliox/cmds/tool/` with a local `replace` to this anycli branch. | No |
| **L4** singleton + seed + `heliox tool braintree -- …` | `POST /internal/test-only/connections/seed` a Braintree connection (multi-field `values`, `api_key`/`credentials` type is seedable — it's a user-token provider, not minted), then `heliox tool braintree -- ping` and `-- transaction search` through the real token gateway → live sandbox API. Proves the projected credential map reaches anycli and authenticates. | **Yes** — sandbox creds seeded (no refresh cycle → seed the static values, no `expires_at`) |
| **L5** connect UI (api_key key-entry path) + one unseeded live run | Master-plan §2 api_key L5: open connect link → paste the four fields in the real CredentialForm → `POST /connections/credentials` verifies via `braintreePingVerifier` → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool braintree -- ping` (or `transaction get`) through the real token gateway succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. Runs once, still hidden, before the visible flip. | **Yes** — sandbox creds entered through the UI |

**External-credential summary:** L2, L4, L5 all require a **Braintree sandbox account** (free, self-serve at `sandbox.braintree.gateway` signup) yielding `merchant_id` + `public_key` + `private_key`. This is the account-pool lane's only Braintree dependency (no production account needed for the hidden→L5 path; production is only exercised by a real user post-flip).

---

## 6. Rollout

Hidden-first (`visible: false`), through L1–L5 while hidden, then the single go-live change = `presentation.visible: true` + `provider-gen` regenerate, gated on L5 passing (no review clearance needed — `api_key` has no review lane). Wave 3, api_key sub-batch (majority agent throughput).

**Stage-1 flags for the batch lead:**
1. **TWO integration-service capability growths (§4.1), not one — each gated on a stage-0 reuse check.** (a) Multi-field manual credential storage/projection (Option A) — verify on the branch base at stage 0; grow if absent. (b) **A verifying manual path for Braintree** — a bespoke `braintreePingVerifier` PLUS a per-provider verifier-**selection** dispatch, because on this base `manual_credentials` is hardwired to the no-verify `dsnHostIdentityDeriver` (`provider_registry.go:88-98`; 317 D5/OQ1). Do **not** assume `identity.source: strategy` alone yields a ping — without the dispatch the bundle routes to the DSN deriver and rejects every connect. **Apply the same stage-0 discipline as (a):** this batch has repeatedly added per-provider verifier/deriver selection for the manual path (amplitude deriver-selection, crisp keypair deriver, iterable region deriver-selection, mailerlite deriver, zoominfo multi-field), so first check whether a selection mechanism already landed on the accumulated base and **reuse it** (register `braintreePingVerifier` as the `braintree` entry) rather than building a second parallel dispatch; only add the dispatch itself if none exists. The verifier (piece ii) is always new; the dispatch (piece i) is load-bearing only if absent.
2. **No lane-1 / config work** — api_key, no OAuth app, no `config/`+`deploy/` appends.
3. **v1 scope is operational** (search/inspect/refund/void/reverse/dispute + a **read-only** `query` passthrough that rejects mutations); charge-creation flows are intentionally excluded (client-token/paymentMethodId dependency). `void` = `voidTransaction` (errors on settled); `reverse` = `reverseTransaction` (full refund on settled) — two separate verbs so `void` never silently moves settled funds.
4. **`Braintree-Version` pin is a current date, not `2019-01-01`.** The header selects the schema as of that date; the `Dispute` type postdates 2019-01-01 (added 2019-10-03). L2 must assert every in-scope verb resolves under the pin before the pin bump; record the version-bump policy next to the package constant.
5. **`account_key` is environment-qualified** (`"<environment>/<merchant_id>"`), so the same `merchant_id` in sandbox vs production does not dedup two distinct connections onto one row.

---

## Sources (official / verified)

- Braintree GraphQL — Making API Calls (auth = HTTP Basic `base64(public_key:private_key)`, `Braintree-Version: YYYY-MM-DD`, `Content-Type: application/json`; endpoints `payments[.sandbox].braintree-api.com/graphql`; keys from Control Panel Settings→API): https://developer.paypal.com/braintree/graphql/guides/making_api_calls/
- Braintree GraphQL — Transactions guide (`search`, `refundTransaction`, `voidTransaction` — void-only, errors once settled — vs `reverseTransaction` — voids if unsettled, **full refund** if settled, `chargePaymentMethod`, `TransactionSearchInput`): https://developer.paypal.com/braintree/graphql/guides/transactions/
- Braintree GraphQL — Reference/Explorer (`ping`, `node`, search inputs): https://developer.paypal.com/braintree/graphql/reference/
- Braintree GraphQL — CHANGELOG (version-date semantics; `Dispute` type added 2019-10-03, earliest entry 2019-03-18, latest 2026-07-14 — establishes why the `Braintree-Version` pin must be a current date, not `2019-01-01`): https://github.com/braintree/graphql-api/blob/master/CHANGELOG.md
- Repo precedents: `internal/tools/mongodb/` + `definitions/tools/mongodb.json` (manual multi-secret env injection), `internal/tools/notion/` (service shape / exit codes / `--json`), `integrations/providers/mongodb/provider.yaml` (manual_credentials bundle), `go-services/integration-service/service/manual_credential.go` + `manual_credentials_identity.go` (single-secret base + deriver pattern), `helio-cli/internal/toolcred/resolver.go` (no divergence needed).
