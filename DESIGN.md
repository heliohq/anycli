# Tool design: Iterable (`iterable`)

Scratch design for the `heliox tool iterable` provider. Batch lead strips this
file at batch end. Catalog row 268 (Wave 3, Marketing & Notifications).

## 0. Naming axes (master plan §3)

| Axis | Value |
|---|---|
| ① CLI command word | `iterable` (flat, ungrouped) |
| ② anycli tool id | `iterable` (`definitions/tools/iterable.json`) |
| ③ provider catalog key | `iterable` (`integrations/providers/iterable/`) |

All three axes are **identical** — no `toolToProvider` divergence entry is
needed in `helio-cli/internal/toolcred/resolver.go` (identity holds, same as
`slack`/`notion`/`bitly`). Go package name: `internal/tools/iterable/` (no
dash, no leading digit → no normalization). Not a grouped family (open
question 2 does not apply).

## 1. Auth lane — verified against official docs (api_key CONFIRMED)

Catalog + audit both place Iterable in the **`api_key`** lane; the 2026-07-21
OAuth audit row 270 verdict is "no viable multi-tenant path → stays api_key".
Independently verified against Iterable's official docs and this holds:

- **Authentication is a project-scoped API key sent in a custom request
  header** — `Api-Key: <key>` (the underscore spelling `Api_Key` is also
  accepted). It is **not** an OAuth Bearer token and **not** `Authorization`.
  Iterable's recent security change requires the key in the HTTP header, never
  in the query string or request body. Sources:
  https://support.iterable.com/hc/en-us/articles/360043464871-API-Keys and the
  interactive spec at `https://api.iterable.com/api/docs` (auth type `apikey`,
  key `Api-Key`, `in: header`).
- There is **no user-level OAuth authorize flow** and **no multi-tenant app**:
  a key is minted by a project admin in the Iterable UI (Settings → API Keys),
  scoped to a single project, with a chosen permission set (server-side vs
  JWT-enabled; we use a standard **server-side** key). This is exactly the
  api_key rubric — the test account yields the key directly, no app
  registration, no lane-1 review clock.
- **No divergence from the catalog/audit** — recorded here for completeness:
  none. Lane confirmed `api_key`.

### The data-center split (the one real design decision)

Iterable runs two isolated data centers and **a key is bound to exactly one**:

- US (USDC) projects → base URL `https://api.iterable.com`
- EU (EDC) projects → base URL `https://api.eu.iterable.com`

A USDC key returns auth errors against the EDC host and vice-versa; there is
no cross-DC routing. So the region is **part of the credential**, not a global
constant. We do **not** auto-probe both hosts — silent DC-fallback is forbidden
by the repo's fail-fast hard rule and would leak the key to the wrong DC.

**The region must be captured, but the storage contract permits exactly one
stored secret** (see §3 — `validateCredentialInputSchema` in
`go-services/integration-service/model/runtime_contract.go` rejects any
`credential_input` that is not a single required field, for both `api_key` and
`credentials` auth). Two typed fields (`api_key` + `region`) is therefore
**not shippable** — it fails `provider-gen` and there is no runtime path for a
second typed field to reach the deriver anyway (the manual write path collapses
the connect payload to one secret string). So region is **folded into the
single secret**: the user supplies `<region>:<api_key>` (e.g.
`us:1234abcd…`), the same "one field carries two facts, split downstream"
shape MongoDB uses (a DSN carries host + credentials in one pasted secret and
the Helio-side deriver extracts the host as the account key). This is the only
manual-credential precedent that actually exists on `main`; the region-prefix
split is a small, reviewed extension of it (§3).

## 2. anycli definition & service (SKILL.md stage 1–2)

### Tool form: `service` type

No official Iterable CLI exists → **`service` type** (the default; 21/23
current definitions are service). HTTP logic under
`internal/tools/iterable/`, registered `RegisterService("iterable",
&iterable.Service{})` in `internal/tools/register.go`. Copy the
`internal/tools/notion/` shape: `BaseURL`/`HC`/`Out`/`Err` struct for
httptest injection, cobra tree grouped by resource, typed `apiError`, exit
codes 0 (success) / 1 (runtime/API failure) / 2 (usage/parse), `--json`
structured error envelope.

### Credential binding (`definitions/tools/iterable.json`)

**One** binding — the single stored secret, which carries `<region>:<api_key>`:

```json
{
  "name": "iterable",
  "type": "service",
  "description": "Iterable cross-channel marketing: users, events, lists, campaigns, transactional email",
  "auth": {
    "credentials": [
      { "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "ITERABLE_API_KEY"} }
    ]
  }
}
```

The service reads `ITERABLE_API_KEY`, **splits on the first colon**: the prefix
is the region (`us` → `api.iterable.com`, `eu` → `api.eu.iterable.com`; any
other prefix, or a missing colon, is a fail-fast exit 2 — no silent default),
and the remainder is the raw project key sent as header `Api-Key: <key>` on
every request. Iterable keys are hex-shaped and never contain a colon, so
first-colon-split is unambiguous (same rationale as a MongoDB DSN carrying
everything in one pasted string). Single-secret manual credentials are the
established precedent — MongoDB is the one live `manual_credentials` provider
on `main`, and it likewise packs multiple facts into one pasted secret and
derives the readable account key from it. There is **no** multi-typed-field
manual-credential precedent (the storage face is single-secret; §3).

### Subcommands (driven by what a Marketing/Notifications teammate does)

An AI teammate on Iterable reads a contact's profile and event history,
updates fields/preferences, subscribes/unsubscribes list membership, checks
how a campaign performed, and fires a transactional email. Verbs map to
official endpoints (all confirmed against the interactive spec / support
docs):

| Group | Verb | Method + path |
|---|---|---|
| `user` | `get` | `GET /api/users/{email}` or `GET /api/users/byUserId/{userId}` |
| `user` | `update` | `POST /api/users/update` |
| `user` | `delete` | `DELETE /api/users/{email}` |
| `user` | `fields` | `GET /api/users/getFields` |
| `event` | `track` | `POST /api/events/track` |
| `event` | `list` | `GET /api/events/{email}` |
| `list` | `list` | `GET /api/lists` |
| `list` | `subscribe` | `POST /api/lists/subscribe` |
| `list` | `unsubscribe` | `POST /api/lists/unsubscribe` |
| `list` | `users` | `GET /api/lists/getUsers?listId=…` |
| `campaign` | `list` | `GET /api/campaigns` |
| `campaign` | `metrics` | `GET /api/campaigns/metrics?campaignId=…` |
| `template` | `list` | `GET /api/templates` |
| `email` | `send` | `POST /api/email/target` (send existing campaign/template to a user) |
| `catalog` | `list` | `GET /api/catalogs` |

Scope note: the export bulk-download family (`POST /api/export/data.json`,
`/api/export/{jobId}/files`) is deferred to a follow-up — it returns
JSONL/CSV bulk dumps, not the per-record JSON an agent reasons over, so it
does not fit the first-cut agent surface. `GET /api/campaigns` doubles as the
connect-time verifier probe (§4).

### JSON output shape

Follow notion's contract: default text rendering for humans, `--json` emits
the provider's response JSON verbatim under a stable wrapper on success and
the `apiError` envelope on failure (`{"error":{"code","message","status"}}`).
Iterable returns `{"code":"Success","msg":…,"params":…}` on writes and
resource arrays on reads; the service surfaces `code`/`msg` faithfully and
maps any non-`Success` Iterable `code` (200-body error dialect exists on some
endpoints) to exit 1 — check for this in the service like notion does, rather
than trusting HTTP status alone.

## 3. Helio provider bundle (`integrations/providers/iterable/provider.yaml`)

Hidden-first. Manual-credential (api_key) bundle, MongoDB-shaped —
`auth.type: credentials`, `runtime_strategy: manual_credentials`, no OAuth
block, no `required_config_fields` (nothing to register in integration-service
config → renders `configured: true` while hidden and needs no lane-1 landing).

```yaml
schema: helio.provider/v1
key: iterable
go_name: Iterable

presentation:
  name: Iterable
  description_key: iterable
  consent_domain: iterable.com
  visible: false        # flip only after L5 + docs + icon + pin (stage 10)

# auth.type credentials (design 317 D5): ONE stored secret. The user pastes
# "<region>:<api_key>" (e.g. us:1234abcd…). There is no HTTPS identity endpoint
# scoped to the key, so connect stores it without provider-side verification
# (OQ1 no-verify): a bad key surfaces at first use via AnyCLI's
# CredentialRejected. The account key/label is the region prefix (OQ2 —
# human-readable), derived Helio-side by the region_prefix deriver (see below).
auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: iterable_api_key
        secret: true
        required: true
        placeholder: "us:xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
        # ONE required field only — the storage face is single-secret
        # (validateCredentialInputSchema). Region is the "<region>:" prefix
        # of this value, not a second field.
    setup_url: https://support.iterable.com/hc/en-us/articles/360043464871-API-Keys

identity:
  source: strategy      # no project-name endpoint; label derived from region
  deriver: region_prefix  # NEW reviewed manual_credentials deriver (see below)

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

# Single secret through the existing UpsertUserToken write path (design 317 D5:
# token.access_token). The stored value is the full "<region>:<api_key>" string;
# anycli splits it. account_key is the region, projected for label/dedup only —
# anycli does not need it separately (it splits the secret itself).
credential:
  fields:
    api_key: token.access_token
    account_key: connection.account_key

tool:
  name: iterable
  kind: api-key
```

Axis notes: `key:` == directory name == `iterable` (generator enforces
equality). `tool.name` == `iterable` == anycli id. No `tool.command` (flat,
ungrouped). No `experiment:` (GA, not preview-gated).

### Identity / account label — and the capability growth this OWNS

The API key is **project-scoped with no user or project-name endpoint**, so
there is nothing to `GET` for a display name — same situation as MongoDB
(`identity.source: strategy`). The account key/label is region-derived:
"Iterable — US" / "Iterable — EU". Because `region` is the human-readable
account key, a teammate connecting a US and an EU project produces two distinct
connections under one provider (`mode: isolated` supports this) — with **one**
stored field each, not two.

This design **owns** the following non-narrow integration-service work — it is
NOT zero-touch, and must be scoped and reviewed on its own:

1. **A new `manual_credentials` identity deriver, `regionPrefixIdentityDeriver`**
   (in `service/manual_credentials_identity.go`, alongside the existing
   `dsnHostIdentityDeriver`). Its `Verify` performs no provider request
   (OQ1 no-verify), first-colon-splits the stored secret, validates the prefix
   against the closed set `{us, eu}`, and returns `accountKey = region`,
   `label = "Iterable — US"|"Iterable — EU"`, `identity = {"region": region}`.
   The secret (including the raw key) never enters the returned identity map,
   keeping Connection metadata secret-free — same invariant `dsnHostIdentityDeriver`
   holds. A missing/invalid prefix returns a `manualCredentialFormatError`
   (surfaced as a 4xx "paste `<region>:<key>`" message), never a 5xx.
2. **Per-provider deriver selection.** Today
   `composeProviderRegistration` (`service/provider_registry.go`) hardwires
   `manual: dsnHostIdentityDeriver{}` for *every* `manual_credentials` provider.
   That single deriver `url.Parse`s the secret and requires a host, so it would
   **reject** an Iterable key outright — this path cannot be reused as-is even
   for a US-only bare key. Add a reviewed `identity.deriver` enum
   (`dsn_host` (default) | `region_prefix`) to the bundle contract and switch
   on it in the `manual_credentials` arm. This stays inside the closed-capability
   discipline (a named, compiled deriver — no arbitrary YAML expression).

This is the D5-single-secret path (unchanged storage face) — it does **not**
require the deferred D8 multi-field vault face. It is strictly a new *deriver +
selection* capability, distinct from the multi-field storage relaxation the
runtime_contract comment defers.

### Connect-time verification — not for the first cut

Verification is desirable in principle (a bad key otherwise surfaces only at
first use). The existing `manual_api_token` strategy already supports a
declarative custom-header probe (`declarativeManualTokenVerifier` sets
`definition.APIKey.Header` to the token and extracts identity via an RFC 6901
pointer over the JSON response) — so a "custom-header verifier" is **not** new
capability. But it does not fit Iterable: (a) Iterable's cheap probe
`GET /api/campaigns` returns a campaign array with **no per-account stable
identity field** to pin `account_key` to, and (b) that strategy cannot route
between the US/EU hosts from a region prefix. So Iterable ships **no-verify**
like MongoDB (`manual_credentials`), and the region deriver above is the whole
Helio-side lifecycle. Revisit verification only if Iterable later exposes a
key-scoped identity endpoint; it is **not** a prerequisite for the visible flip.

## 4. Test plan — five layers (SKILL.md stage 9)

| Layer | What runs | Needs external creds? |
|---|---|---|
| L1 | `go test ./...` in anycli: httptest fake for `api.iterable.com`; assert `Api-Key` header injected, `<region>:<key>` split (us/eu → base URL; missing/invalid prefix → exit 2), request shapes for each verb, and both text + `--json` error rendering. No real API. | No |
| L2 | `ANYCLI_CRED_API_KEY="us:<key>" anycli iterable -- list list` (and a `user get`, `event track`, `campaign list`) against the **real** Iterable API. Mandatory before pin bump. | **Yes** — one server-side API key from a real Iterable project (test-account pool). |
| L3 | `provider-gen` + `provider-gen --check` (five projections) locally on-branch — **now passes** the single-required-field check (one field); `helio-cli` build via uncommitted `go.mod replace` → anycli branch; `go test ./...` both repos incl. `helio-cli/.../cmds/tool`, the new `regionPrefixIdentityDeriver` unit test, and the per-provider deriver-selection test in integration-service. | No |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `iterable`, seed `access_token`=`us:<key>`, `account_key`=`us`) → `heliox tool iterable -- list list` reaches the live API through the real token gateway. api_key is seedable (not a minted provider). | **Yes** — same real key as L2. |
| L5 | Full connect path once before the visible flip: `heliox tool iterable auth` → connect link → **paste `<region>:<key>` as the single secret** in the real connect UI (`POST /connections/credentials`) → connection shows connected/configured with label "Iterable — US" (`GET /connections`) → one **unseeded** live `heliox tool iterable -- list list`. This is the **api_key key-entry L5 path** (master plan §2), not the OAuth consent path. | **Yes** — real key; agent-drivable via agent-browser, human fallback on UI breakage. |

L2/L4/L5 all consume the **same single server-side API key** from the account
pool — no OAuth app, no lane-1 dev-app creation, no review clearance gates
anything. The only gate on the visible flip is L5 + docs published + icon
registered + the anycli pin shipping the `iterable` definition + the
integration-service deriver capability (§3) merged.

## 5. Remaining stage checklist (per SKILL.md)

- Stage 3 pin bump: batch-end only (one anycli tag / pin per batch); on-branch
  uses the local `replace`, never committed.
- Stage 5 generate: five projections committed together at batch end (this
  branch is **expected** to fail `provider-gen --check` in CI until then — do
  not commit a local regen to green the branch). With the single-field bundle,
  the local on-branch `provider-gen --check` itself passes the schema gate.
- Stage 6 service + capability growth: manual_credentials needs **no OAuth
  service code** and no `config/`+`deploy/` secret landing (no
  `required_config_fields`), **but it is NOT zero-touch**: this design owns the
  two integration-service items in §3 — (1) the new `regionPrefixIdentityDeriver`
  with its unit test, and (2) the reviewed `identity.deriver` bundle enum +
  per-provider deriver selection in `composeProviderRegistration`, with its test.
  Run `make test-integration-service` for both. No `service/adapter_*.go`, no
  token-gateway change — the runtime path stays generic manual_credentials with
  the single-secret storage face.
- Stage 7 icon: `ui/helio-app/src/integrations/icons/iterable.svg` +
  `providerIcons.ts` (manual, batch-end).
- Stage 8 docs: provider sub-doc under
  `agents/plugins/heliox/skills/tool/`, plugin version bump (batch-end
  publish). Document the `<region>:<key>` paste format for the connect step.
- Stage 10 rollout: deploy hidden → L5 → flip `visible: true` + regenerate as
  the single go-live change.
