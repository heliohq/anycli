# Tool design: Knock (`knock`)

Scratch design for the `helio-tool-provider` pipeline. Batch lead strips this at
batch-end. Catalog row 270 (§4 of the 298-integrations rollout plan): product
**Knock**, anycli id `knock`, provider key `knock`, auth lane **api_key**, wave
**3**, category **Marketing & Notifications**.

## 0. Audit verification (independent, against official docs)

The 2026-07-21 OAuth audit (row 272 in `oauth-audit.md`) keeps Knock in the
**api_key** lane: "no viable multi-tenant path". The **lane** is verified
against Knock's own docs and confirmed. On the key format the earlier
catalog/design assumption is **partly** corrected. Knock's official docs **do**
show an `sk_test_` prefix for the Development environment — the API reference
overview authenticates with `Authorization: Bearer sk_test_12345` and the Elixir
SDK docs use `api_key: "sk_test_12345"`. So it is **wrong** to call `sk_test_` a
"Stripe placeholder, not Knock's scheme"; Knock's development secret keys really
do carry an `sk_test_` prefix. What is **not** safe is deriving a meaningful
environment *label* from the prefix — the reasons are below, and they still
support the static-label decision in §4, just for the correct reasons.

- Knock's data API (`https://api.knock.app/v1`) authenticates with a single
  **environment-scoped secret key** — public keys prefix `pk_`, secret keys
  prefix `sk_` (development keys observed as `sk_test_…`). Source:
  docs.knock.app/developer-tools/api-keys ("they start with `pk_` for a public
  key, vs `sk_` for a secret key"; "each environment has its own unique set of
  API keys") and the API reference ("You must pass your API key to Knock as a
  Bearer token using the `Authorization` header", example `Bearer sk_test_12345`).
- Environments are **open-ended**: an account starts with Development and
  Production but can add arbitrary custom environments (e.g. Staging), inserted
  below Production in the promotion chain (docs.knock.app/concepts/environments).
  A **custom** environment cannot be represented from any prefix (there is no
  documented `sk_staging_`-style scheme), and production keys are not documented
  to carry a readable env segment. So a prefix-derived environment label would be
  correct only for the Development case and wrong/empty for the rest — which is
  why §4 uses a **static** label plus a fingerprint suffix rather than a
  prefix-parsed one. The static-label conclusion stands; the "no sk_test_ exists"
  premise does not.
- There is **no** multi-tenant authorization-code OAuth flow: no
  `authorize`/`token` endpoints, no "one registered app that arbitrary Knock
  customers authorize". The only credential a customer can produce for
  server-side calls is their own secret key. This is exactly the audit rubric's
  "no viable multi-tenant path" → api_key. **api_key lane confirmed.**
- Public keys (`pk_*`) exist but only identify the account for client-side SDKs
  (in-app feed, guides) and cannot perform server API calls — irrelevant here.

## 1. Official API surface wrapped, and why

Knock is **notification infrastructure**: you model recipients, then trigger
workflows that fan a single event out across channels (email, SMS, push,
in-app, Slack, …). An AI teammate on Helio uses Knock the way a human on-call
or growth engineer would — *send a notification, make sure the right person
gets it, and check whether it landed*. That intent drives the wrapped surface.

Base URL `https://api.knock.app/v1`. Auth `Authorization: Bearer <secret key>`
on every request. All endpoints below are verified from the official API
reference (docs.knock.app/api-reference and /reference).

| AI-teammate intent | Knock resource | Endpoints wrapped |
|---|---|---|
| **Send a notification** (the #1 job) | Workflows | `POST /workflows/{key}/trigger` (body: `recipients[]`, `data`, `actor`, `tenant`, `cancellation_key`, `settings.sandbox_mode/skip_delay`; returns `{workflow_run_id}`; accepts `Idempotency-Key`), `POST /workflows/{key}/cancel` |
| **Know who to notify** | Users (recipients) | `PUT /users/{id}` (identify/upsert), `GET /users/{id}`, `GET /users`, `DELETE /users/{id}`, `POST /users/{id}/merge` |
| **Route to the right channel + respect opt-outs** | Channel data & preferences | `GET/PUT/DELETE /users/{id}/channel_data/{channel_id}`, `GET /users/{id}/preferences`, `GET/PUT/DELETE /users/{id}/preferences/{preference_set_id}` |
| **Did it land? was it seen/read?** | Messages | `GET /messages`, `GET /messages/{id}`, `GET /messages/{id}/content`, `GET /messages/{id}/events`, `GET /messages/{id}/delivery_logs`, `GET /messages/{id}/activities`, `PUT/DELETE /messages/{id}/{seen,read,archived}`, `PUT /messages/{id}/interacted` |
| **Non-user recipients** (a project, doc, account) + who follows them | Objects & subscriptions | `PUT/GET/DELETE /objects/{collection}/{id}`, `GET /objects/{collection}`, `GET /objects/{collection}/{id}/{messages,schedules,subscriptions,preferences}` |
| **Scope by customer/workspace** | Tenants | `PUT/GET/DELETE /tenants/{id}`, `GET /tenants` |
| **Send later / recurring** | Schedules | `POST /schedules`, `GET /schedules`, `PUT /schedules`, `DELETE /schedules` |

**Explicitly deferred (first ship):** the `*/bulk/*` batch endpoints
(`/users/bulk/identify`, `/objects/bulk/set`, `/tenants/bulk/set`,
`/schedules/bulk/create`, `/channels/{id}/messages/bulk/{action}`, …). They are
throughput tools for data migrations, not the interactive "notify a person /
check delivery" loop an assistant runs turn-by-turn; add them in a follow-up if
demand shows. **Out of scope entirely:** the Knock **Management API**
(`control.knock.app`, service-token auth) that edits workflow *definitions*,
layouts, and commits — that is authoring infrastructure a human owns in the
dashboard, not a runtime action a teammate takes.

## 2. anycli definition

**Tool form — `service` type** (stage-1 rubric). No official Knock CLI binary
exists to wrap; the surface is a plain Bearer-auth JSON REST API. Default
`service` applies (21 of 23 shipped definitions are service type). No cli-type
justification holds (no binary to provision into the runtime image).

- **Definition file:** `definitions/tools/knock.json`.
- **Go package:** `internal/tools/knock/` (id has no dash → package name
  `knock`, per §3 "Go package name" rule). Registered as
  `RegisterService("knock", &knock.Service{})` in `internal/tools/register.go`
  (batch-end shared-surface edit).
- **Auth injection** (definition `auth.credentials`): source field `api_key`
  → inject `env` `KNOCK_API_KEY`. The service reads `KNOCK_API_KEY` and sends
  `Authorization: Bearer <KNOCK_API_KEY>` on every call (built in the service,
  not the definition — mirrors how notion turns `NOTION_TOKEN` into its header).

```json
{
  "name": "knock",
  "type": "service",
  "description": "Knock as a tool (notification infrastructure; secret API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "KNOCK_API_KEY"}
      }
    ]
  }
}
```

**Service package (cobra tree, mirrors the notion service shape):**
`Service` struct with `BaseURL` (default `https://api.knock.app/v1`, overridable
for httptest), `HC *http.Client`, `Out`/`Err` writers. `Execute` reads
`KNOCK_API_KEY` (fail fast → exit 1 with a structured error envelope when
empty), builds the cobra root, dispatches subcommands.

Subcommands / verbs (group → verb), grouped by the resource nouns above:

- `workflow trigger` (`--key`, `--recipients` repeatable / JSON, `--data` JSON,
  `--actor`, `--tenant`, `--cancellation-key`, `--sandbox`, `--skip-delay`,
  `--idempotency-key`) · `workflow cancel` (`--key`, `--cancellation-key`,
  `--recipients`)
- `user identify` (`--id`, `--data` JSON) · `user get` · `user list`
  (`--page-size`, `--after`) · `user delete` · `user merge`
  (`--id`, `--from-id`) · `user get-preferences` / `user set-preferences`
  (`--set` id, `--data` JSON) · `user get-channel-data` /
  `user set-channel-data` / `user delete-channel-data` (`--channel-id`)
- `message list` (filters: `--recipient`, `--channel-id`, `--status`,
  `--tenant`, `--workflow`, `--page-size`, `--after`) · `message get` ·
  `message content` · `message events` · `message activities` ·
  `message delivery-logs` · `message mark` (`--state seen|read|interacted|archived`,
  `--undo`)
- `object set` / `object get` / `object delete` / `object list`
  (`--collection`, `--id`) · `object subscriptions`
- `tenant set` / `tenant get` / `tenant list` / `tenant delete` (`--id`)
- `schedule create` / `schedule list` / `schedule update` / `schedule delete`

**JSON output shape.** Pass Knock's JSON response body through verbatim to
stdout (one JSON document per invocation), exactly like the notion service's
`emitJSON` — Knock already returns clean, envelope-consistent JSON
(`{entries, page_info}` for lists, resource objects for gets,
`{workflow_run_id}` for trigger). No re-shaping; the agent consumes the
provider's own schema. Errors: exit 0 success; exit 2 usage/param errors
(bad flags, invalid JSON, missing required id); exit 1 API/transport errors
(Knock non-2xx surfaced with status + body). Under `--json`, errors render as
the structured error envelope on stderr; stdout stays pure JSON. `--json` is
the default output for an agent tool.

**AnyCLI credential-rejected classification.** A revoked/invalid `sk_` key
makes Knock return `401`; the service surfaces it as an API error whose body
the token gateway/AnyCLI classifies as `CredentialRejected` — this is the
signal the Helio side relies on given the no-network-verify identity choice
(§4).

## 3. Credential fields and the exact auth flow

**Credential:** one field — the Knock **secret key** (opaque, `sk_`-prefixed;
the environment it targets is chosen by *which* environment's key the user
pastes, not by the prefix). The user creates/reads it in the Knock dashboard
under Developers → API keys (per environment). It is long-lived, non-expiring,
and carries no refresh cycle (there is nothing to refresh — a static secret).

- Registration model: **none** (no app registration, no OAuth client, no
  Helio-side client id/secret). The user's own environment key *is* the
  credential. This is why lane-1's OAuth-app-registration queue does **not**
  touch Knock, and why the integration-service needs **zero** provider config
  fields (`auth.required_config_fields: []`).
- Scopes/token semantics: the secret key can "perform any API request" —
  Knock has no per-key scoping on the data API; access is all-or-nothing within
  the key's environment. Display-only capability slugs (§4 bundle) disclose
  what the key grants at connect time.
- Injection path at runtime: connect UI → write-only
  `POST /connections/credentials` → Vault; at tool time the token gateway
  projects the stored secret into AnyCLI's credential map field `api_key` →
  definition injects `KNOCK_API_KEY` → service sends the Bearer header.

## 4. Helio provider bundle plan

Directory `integrations/providers/knock/provider.yaml`, `presentation.visible:
false` (hidden-first). **Three naming axes all identical → no
`toolToProvider` divergence entry, no `resolver.go` edit:**

| Axis | Value |
|---|---|
| ① CLI command word (`tool.command`/`tool.name`) | `knock` |
| ② anycli tool id (`definitions/tools/knock.json`) | `knock` |
| ③ provider catalog key (bundle dir / `key:`) | `knock` |

**Runtime strategy — `manual_credentials` with a credential-derived identity
(no network verify).** This is the crux and the one **capability-growth flag
(stage 1, per rollout §6 "flag adapter/credential-kind candidates at stage 1").**

**The one unavoidable new capability is a secret-fingerprint identity source.**
Knock's data API has **no whoami / account-identity endpoint** — every list
endpoint returns `{entries, page_info}` with no account-identifying string, and
`GET /messages?page_size=1` has no account field and returns empty `entries`
when the env has no messages. So **no Knock response yields a value for
`identity.stable_key` to point at.** The account key can therefore only be
**derived from the credential itself** (a deterministic fingerprint of the
secret), exactly the design-317 model `dsnHostIdentityDeriver` already uses for
mongodb/braze (no provider-side request; a bad secret surfaces at first tool
use via AnyCLI `CredentialRejected`). Knock differs only in *what* is derived,
since a secret key has no readable host — it has no host **and** no whoami, so
the derived value is a fingerprint, not a parsed field.

This reframes the choice below. It is **not** "Option A needs a new capability,
Option B avoids it." **Both** options need the *same* one new thing — a
fingerprint-producing identity source — because neither the `dsnHostIdentityDeriver`
(manual_credentials) nor the `declarativeManualTokenVerifier` (manual_api_token)
can produce a stable_key from a hostless, whoami-less Knock key on main. The real
question is only **which strategy hosts that one fingerprint capability**: the
`manual_credentials` deriver path (Option A) or the `manual_api_token` verifier
path (Option B).

> **Hard prerequisite — NOT on main today.** On `main`,
> `composeProviderRegistration` (integration-service
> `service/provider_registry.go`) binds **every** `manual_credentials` provider
> unconditionally to `dsnHostIdentityDeriver`, which `url.Parse`es the secret and
> requires a DSN host. A Knock `sk_...` key has no host, so on main it returns
> `manualCredentialFormatError` and **rejects the connect**. There is currently
> **no deriver-selection mechanism** on main and **no**
> `bearerKeyFingerprintIdentityDeriver`; both live only on the unmerged sibling
> branch `tool/amplitude` (the `#deriver-selection` work). Knock's bundle is
> therefore **not landable on its own** — it is batch-serialized behind that
> capability. Both pieces (a) the deriver-selection wiring in
> `composeProviderRegistration` and (b) the new fingerprint deriver must be built
> and merged before Knock's connect can compose. This is tracked as blocking
> **Open Decision 1**, not a confirmation.

- **Recommended (Option A): a reusable `bearerKeyFingerprintIdentityDeriver`**
  selected via the (to-be-merged) deriver-selection mechanism. It does **no**
  network call and derives:
  - **stable_key (dedup)** = a deterministic truncated SHA-256 fingerprint of
    the full secret — the actually load-bearing value. Two distinct keys never
    collide on `(org, assistant, provider, account_key)` and re-pasting the same
    key is idempotent. The secret never enters connection metadata (only its
    fingerprint), matching the dsnHost "secret-free metadata" rule.
  - **label** = a **static `"Knock"`** (optionally suffixed with the
    fingerprint's short form, e.g. `Knock · a1b2c3`, to disambiguate two
    connected keys). A prefix-derived environment label is dropped **not**
    because `sk_test_` is fake — it is real for Development (§0) — but because it
    would be right only for the Development case: custom environments have no
    documented prefix scheme and production keys are not documented to carry a
    readable env segment, so a prefix parse yields correct-for-dev / wrong-or-empty
    for everything else (§0). The environment is server-side state keyed by the
    opaque secret with no documented way to read a name back from the credential,
    so any environment-in-the-label scheme is **out of scope**. A static label
    plus fingerprint suffix disambiguates two connected keys without ever falling
    back to the raw hash that dsnHost OQ2 forbids.

  This deriver is **not Knock-specific**: rows 268–272 (Iterable, Courier,
  Knock, Novu, Loops) plus Mailjet are the same shape — one opaque Bearer
  secret, no whoami. Build it once as the cluster's reusable capability, not a
  per-tool fork (rollout "prefer growing one reviewed capability").

- **Alternative (Option B — adopt only if reviewers want a positive connect-time
  signal):** host the same fingerprint capability on the `manual_api_token`
  strategy instead. This is **not capability-free**, and the earlier framing that
  it "does not depend on the unbuilt deriver-selection path" was **wrong**. The
  shipped `declarativeManualTokenVerifier` (manual_token_verifier.go:53-60)
  **requires** `identity.stable_key` to resolve to a non-empty string **inside the
  identity-endpoint response body** and hard-fails the connect otherwise — it has
  **no secret-fingerprint fallback**. Since no Knock response yields a stable_key
  (above), the stock verifier **cannot** connect Knock; "still fingerprinting for
  the account key" is not something that code path does or can do. So Option B
  needs a **new compiled `manual_api_token` verifier variant** that fingerprints
  the secret as the stable_key — the *same* fingerprint capability as Option A,
  just wired to the verifier strategy. Its only genuine delta over Option A is the
  optional `GET /messages?page_size=1` (`Authorization: Bearer <key>`) `200` check
  before the Vault write, for a positive connect-time signal. Trade-off: it adds
  request surface for marginal value (AnyCLI already classifies the `401` at first
  use), needs a workflow-free read path (message list is safe and always exists),
  and — being a verifier variant — does not de-risk the fingerprint work, since
  that work is unavoidable in either option.

Bundle sketch — the shape follows the shipped `manual_credentials` precedent
`integrations/providers/mongodb/provider.yaml` exactly (auth.type `credentials`,
`identity.source: strategy`, `credential.fields` projecting `token.access_token`
plus `account_key: connection.account_key`, `tool.kind: api-key` with a dash —
the wire-compat value, distinct from the catalog's `api_key` **lane label**,
which is not the bundle's `auth.type` enum). Copy the exact field values from
mongodb/braze so the generator's closed contract passes:

```yaml
schema: helio.provider/v1
key: knock
go_name: Knock
presentation:
  name: Knock
  description_key: knock
  consent_domain: knock.app
  visible: false          # hidden-first; flip + regen is the single go-live change
  order: <next free>
auth:
  type: credentials       # design-317 manual-credentials shape (NOT the catalog "api_key" lane label)
  owner: individual
  credential_input:
    fields:
      - {name: api_key, label_key: api_key, secret: true, required: true, placeholder: "sk_test_..."}
    setup_url: https://dashboard.knock.app     # Developers → API keys
identity:
  source: strategy                              # deriver; no stable_key/url required
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials
resources: {selection: none, discovery: none, enforcement: none}
# Single secret stored via the existing UpsertUserToken write path (design 317 D5):
# project the token itself + the deriver-produced account key, mirroring mongodb.
credential:
  fields:
    api_key: token.access_token
    account_key: connection.account_key
tool:
  name: knock
  kind: api-key           # wire-compat value (317 D2), dash — clients route the drawer by auth_type
```

Note the credential field carries the secret through `token.access_token` (the
Bearer scheme is built in the anycli service, §2 — the bundle does not declare a
header sub-block; the invented `auth.api_key.header` block in an earlier draft is
not part of the manual_credentials contract and is dropped).

- **required_config_fields:** empty — a `manual_credentials` provider "does not
  use server config fields" (validate.go), so **no `config/` + `deploy/` secret
  append** and no lane-1 landing for Knock. All-fields-absent renders
  `configured: false`? No — manual-token providers are configured by their
  compiled verification/deriver contract, not by env config, so Knock shows as
  configured (Connect enabled) as soon as the bundle + deriver ship.
- **UI icon:** `ui/helio-app/src/integrations/icons/knock.svg` + hand-register
  in `providerIcons.ts` (never generated). i18n: `tools.credentialField.api_key`
  (reuse) + `tools.description.knock` + any display-scope slugs.
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` (knock: how to trigger a workflow, the
  recipient-must-exist rule, sandbox_mode for safe test sends, reading message
  status). Plugin version bump + marketplace publish ride the batch-end merge.

## 5. Test plan — five layers

| Layer | What it proves for knock | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | Definition unit test (field→env binding) + service cobra tests against httptest fakes: trigger body assembly (recipients/data/actor/tenant/idempotency), list pagination passthrough, message mark states, error/exit-code mapping (401→exit 1 API error, bad `--data` JSON→exit 2), verbatim JSON emit. | No |
| **L2** `anycli knock -- …` dev harness vs REAL api.knock.app | `ANYCLI_CRED_api_key=sk_…` (a Development-environment key); run read-safe `message list` and `user list` first, then `user identify` + `workflow trigger --sandbox` (needs a workflow `key` to exist in the test env — otherwise assert the 404 path and keep the live send to a configured workflow). | **Yes** — a real Knock **test-environment secret key**, and (for trigger) one workflow configured in that env. |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle strict-decode + closed-contract validation; `manual_credentials` + `identity.source: strategy` accepted; resolver test unchanged (no divergence entry). Run `provider-gen` locally on-branch, do **not** commit projections. **Note:** the deriver-selection wiring and the fingerprint deriver are prerequisites that do **not** exist on main (see §4 hard-prerequisite box); this layer proves the bundle validates, not that connect composes — connect/composition only passes once the Open-Decision-1 capability (Option A) or the Option-B verifier lands and merges. | No |
| **L4** singleton + seed + `heliox tool knock -- …` | `POST /internal/test-only/connections/seed` with `provider:"knock"`, `access_token:"sk_…"` (api_key providers are seedable; seed `access_token` only — no refresh cycle). Then `heliox tool knock -- message list` reaches live Knock through the token gateway → real JSON. | **Yes** — a real Knock test key to seed (L4 success = seeded key reaches the live API). |
| **L5** full connect flow, once, before visible flip | **api_key key-entry path** (agent-drivable, master-plan §2): open `heliox tool knock auth` connect link → paste the `sk_…` key through the real connect UI (`POST /connections/credentials`) → connection shows connected/configured in `GET /connections` (immediate — no network verify) → one **unseeded** live command (`heliox tool knock -- message list`, or `workflow trigger --sandbox` against a configured workflow) succeeds through the token gateway. That unseeded run is the completion signal. | **Yes** — a real Knock test key, pasted through the UI; agent-driven with human fallback on UI breakage. |

External-credential summary: **L2, L4, L5 each need a real Knock
test-environment secret key** (one key covers all three); L2/L5 additionally
need one workflow configured in that env *only if* exercising a live send
rather than a read. **L1 and L3 need no external credentials.**

## 6. Rollout

Ship hidden (`visible: false`). The anycli service + definition land
independently, but the Helio bundle's connect path is **gated on a
fingerprint-producing identity source landing on main** (Open Decision 1 / §4
hard-prerequisite box) — this is required in **both** options, so there is no
"skip the capability" path. Until it merges, the bundle validates (L3) but
connect does not compose. Sequence: land the one fingerprint capability, hosted
on whichever strategy OD1 picks — the `manual_credentials` deriver (Option A) or
a new `manual_api_token` verifier variant (Option B) — then the bundle, pass
L1–L4 on branch, then the batch-end merge (one pin bump, one `provider-gen`, one
plugin publish). Run the L5 key-entry sweep after batch-end while still hidden;
only then flip `presentation.visible: true` + regenerate as the single go-live
change. Wave 3, no review clock (api_key) — the gates on "done" are the
capability prerequisite and L5.

## 7. Open decisions for the implementer

1. **Fingerprint identity capability (BLOCKING prerequisite, stage-1 flag).**
   No path on `main` today produces a stable_key from a hostless, whoami-less
   Knock key: `composeProviderRegistration` binds all `manual_credentials`
   providers to `dsnHostIdentityDeriver`, which rejects a hostless `sk_` key, and
   the `manual_api_token` `declarativeManualTokenVerifier` hard-fails when the
   response has no stable_key with no fingerprint fallback (§4). A
   fingerprint-producing identity source is therefore required **regardless of
   option** — the decision is only **which strategy hosts it**, not whether it is
   built:
   - **Option A (recommended):** land the deriver-selection wiring in
     `composeProviderRegistration` **and** add a shared
     `bearerKeyFingerprintIdentityDeriver` (no network call; **label = static
     `"Knock"`**, optionally `+ short fingerprint`; **stable_key = truncated
     SHA-256** of the secret). **Do not** derive the label from the key prefix:
     `sk_test_` is real for Development but a readable environment name is not
     obtainable for custom/production environments (§0), so it is out of scope.
     Build it as the reusable capability serving the rows-268–272 cluster, not a
     Knock-only path.
   - **Option B (only for a positive connect-time signal — NOT capability-free):**
     add a **new** `manual_api_token` verifier variant that fingerprints the
     secret as stable_key (the stock `declarativeManualTokenVerifier` cannot, §4),
     optionally gated on a `GET /messages?page_size=1` `200` check (tally-style
     Bearer scheme). This hosts the *same* fingerprint capability on the verifier
     strategy; it does **not** avoid building the fingerprint work and does not
     de-risk against the deriver-selection wiring slipping. Pick it only if a
     pre-Vault `200` signal is wanted.
2. **Trigger safety.** Default `workflow trigger` to require an explicit
   `--recipients`; surface `--sandbox`/`--skip-delay` prominently in the
   AI-facing doc so a teammate can dry-run a send before a real fan-out.
3. **Bulk endpoints** deferred to a follow-up unless demand appears.
