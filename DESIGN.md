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
single secret**, the same "one field carries two facts, split downstream" shape
MongoDB uses (a DSN carries host + credentials in one pasted secret and the
Helio-side deriver extracts the host as the account key). This is the only
manual-credential precedent that actually exists on `main`; the region-prefix
split is a small, reviewed extension of it (§3).

### Region is NOT a per-connection-unique key (the account-key limitation)

Region routes the request host, but it is **not** a unique per-connection
identity. Iterable keys are **project-scoped**, and marketing/notifications
teams routinely run several projects in **one** data center (staging +
production, or one per brand) — all USDC keys, all region `us`. There is no
cheap project id to key on without a verify call, and `GET /api/campaigns`
(the only cheap probe) carries **no per-account identity field** (§4), so
region genuinely cannot serve as a unique per-connection key.

This matters because the manual-credential connect path resolves the
connection by `FindByAccountKey(org, assistant, provider, accountKey)` and, on
a hit, calls `UpdateCredential` — i.e. it treats the second connect as a
**reconnect and overwrites the stored credential**
(`go-services/integration-service/service/manual_credential.go:92-135`). If
`accountKey` were the bare region, two same-DC project keys would both derive
`accountKey="us"`, and connecting the second would **silently overwrite** the
first project's key onto the same connection — data loss with no error, cutting
against the repo's fail-fast / no-silent-fallback hard rule.

So the single secret carries an **optional project alias** as the disambiguator:
the user pastes `<region>:<alias>:<api_key>` (e.g. `us:staging:1234abcd…`) to
run multiple same-DC projects as distinct connections, or `<region>:<api_key>`
(e.g. `us:1234abcd…`) when a single connection per data center is enough.
Region stays the **host-routing** axis; alias is the **account-identity** axis
— two orthogonal facts, one stored secret. `account_key` becomes `region` (no
alias) or `region:alias` (aliased), so a US and an EU key never collide, an
aliased `us:staging` and `us:prod` are distinct, and two **un-aliased** US keys
consciously collapse onto one per-DC connection (a re-key, documented — not a
surprise, since the alias is the escape hatch). See §3 for the full
account/label model and the honest multi-account boundary.

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

**One** binding — the single stored secret, which carries
`<region>[:<alias>]:<api_key>`:

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

The service reads `ITERABLE_API_KEY` and **splits on `:` into 2 or 3 parts**:
`parts[0]` is the region (`us` → `api.iterable.com`, `eu` →
`api.eu.iterable.com`; any other region, a part count outside {2,3}, or an
empty region/key is a fail-fast exit 2 — no silent default); the **last** part
is the raw project key sent as header `Api-Key: <key>` on every request; a
present middle part is the account alias, which anycli **ignores** (it is a
Helio-side account-identity fact only — anycli needs region + key). Iterable
keys are alphanumeric and never contain a colon, and the alias is colon-free by
construction (a colon in the alias would push the part count past 3 → exit 2),
so the split is unambiguous (same colon-free-key rationale a MongoDB DSN and
Amplitude's `region:key` split already rely on). Single-secret manual
credentials are the established precedent — MongoDB is the one live
`manual_credentials` provider on `main`, and it likewise packs multiple facts
into one pasted secret and derives the readable account key from it. There is
**no** multi-typed-field manual-credential precedent (the storage face is
single-secret; §3).

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
# "<region>:<alias>:<api_key>" (e.g. us:staging:1234abcd…) to run multiple
# same-DC projects as distinct connections, or "<region>:<api_key>" (e.g.
# us:1234abcd…) for one connection per data center. There is no HTTPS identity
# endpoint scoped to the key, so connect stores it without provider-side
# verification (OQ1 no-verify): a bad key surfaces at first use via AnyCLI's
# CredentialRejected. The account key/label is region (no alias) or
# region:alias (OQ2 — human-readable), derived Helio-side by the region_prefix
# deriver (see below). Region is NOT a per-connection-unique key on its own
# (§1) — the alias is the same-DC disambiguator.
auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: iterable_api_key
        secret: true
        required: true
        placeholder: "us:staging:xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
        # ONE required field only — the storage face is single-secret
        # (validateCredentialInputSchema). Region is the leading segment and the
        # optional alias the middle segment of THIS value, not extra fields.
    setup_url: https://support.iterable.com/hc/en-us/articles/360043464871-API-Keys

identity:
  source: strategy      # no project-name endpoint; label derived from region[:alias]
  deriver: region_prefix  # generic reviewed manual_credentials deriver (see below)

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

# Single secret through the existing UpsertUserToken write path (design 317 D5:
# token.access_token). The stored value is the full "<region>[:<alias>]:<api_key>"
# string; anycli splits it. account_key is region or region:alias, projected for
# label/dedup only — anycli does not need it separately (it splits the secret
# itself and ignores the alias segment).
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

### Identity / account label — the multi-account boundary (honest), and the capability growth this OWNS

The API key is **project-scoped with no user or project-name endpoint**, so
there is nothing to `GET` for a display name — same situation as MongoDB
(`identity.source: strategy`). Region alone is derivable without a verify call,
but **region is not a per-connection-unique key** (§1): Iterable keys are
project-scoped and one data center hosts many projects, so region-only identity
would silently overwrite (`FindByAccountKey` → `UpdateCredential`,
`manual_credential.go:92-135`) whenever a teammate connects a second same-DC
project. This is **not** presented as a clean isolated-multi-account win.

The honest boundary, resolved via the optional alias folded into the single
secret (§1):

- **Un-aliased** `<region>:<key>` → `accountKey = region` (`us`/`eu`),
  label `Iterable — US`|`Iterable — EU`. This is **one connection per data
  center** per assistant: a US and an EU key are two connections, but a second
  **un-aliased** US key is a conscious **re-key** of the US connection
  (standard manual-credential reconnect semantics), not a second isolated
  project. Documented in the bundle + connect docs so it is a stated model, not
  a surprise.
- **Aliased** `<region>:<alias>:<key>` → `accountKey = region:alias`
  (`us:staging`), label `Iterable — US (staging)`. Distinct alias → distinct
  connection, so `mode: isolated` genuinely delivers multiple same-DC projects
  (staging + production, per-brand) with **one** stored field each. `us:staging`
  and `eu:staging` never collide (region is part of the key).

This design **owns** the following non-narrow integration-service work — it is
NOT zero-touch, and must be scoped and reviewed on its own:

1. **A generic, bundle-parameterized `regionPrefixIdentityDeriver`** (in
   `service/manual_credentials_identity.go`, alongside the existing
   `dsnHostIdentityDeriver`) — deliberately **not** Iterable-specific. Its
   `Verify` performs no provider request (OQ1 no-verify), splits the stored
   secret into 2–3 colon segments, validates the region segment against a small
   **shared** region-label map (`{us: "US", eu: "EU"}`, package-level, reused by
   every US/EU-residency provider), and returns:
   - `accountKey` = `region` (no alias) or `region:alias` (aliased),
   - `label` = `"<brand> — <REGION>"` or `"<brand> — <REGION> (<alias>)"`, where
     **`<brand>` comes from `presentation.name`** (not a baked-in string), and
     `<REGION>` from the shared map,
   - `identity` = `{"region": region}` (+ `"alias"` when present).
   The secret (including the raw key) never enters the returned identity map,
   keeping Connection metadata secret-free — same invariant
   `dsnHostIdentityDeriver` holds. A missing/invalid region, out-of-range part
   count, or empty alias/key returns a `manualCredentialFormatError` (4xx
   "paste `<region>[:<alias>]:<key>`" guidance), never a 5xx. The label prefix
   and the `{us, eu}` set are thus **inputs**, not compiled-in Iterable
   concerns — the deriver is a generic region/prefix split.
2. **Per-provider deriver selection.** Today
   `composeProviderRegistration` (`service/provider_registry.go:88-98`)
   hardwires `manual: dsnHostIdentityDeriver{}` for *every* `manual_credentials`
   provider. That single deriver `url.Parse`s the secret and requires a host, so
   it would **reject** an Iterable key outright — this path cannot be reused
   as-is even for a US-only bare key. Add a reviewed `identity.deriver` enum
   (`dsn_host` (default) | `region_prefix`) to the bundle contract and switch on
   it in the `manual_credentials` arm. This stays inside the closed-capability
   discipline (a named, compiled deriver — no arbitrary YAML expression).

**Batch-lead reconciliation note (finding follow-up).** The Amplitude tool
(same Wave-2/3 batch, also US/EU residency) already added a first-colon-split
region identity deriver on its own branch, and Braze/Mixpanel are the other
residency-shaped siblings. Because bundles + generation are batch-end-serialized
(master plan §2), two near-identical region derivers and two competing
`identity.deriver` enum additions would collide at the batch-end merge. The
batch lead MUST reconcile these into **one** parameterized `regionPrefixIdentityDeriver`
+ **one** `region_prefix` enum value shared by Iterable and Amplitude (and any
other US/EU provider), rather than shipping per-provider copies. The same
reconciliation should evaluate whether the residency siblings need the optional
**alias** escape hatch: any sibling that keys the connection on region **alone**
(Amplitude, as shipped) carries the identical same-DC silent-overwrite exposure
described in §1; siblings whose credential already carries a project/account id
(e.g. Mixpanel's project id) derive a unique key and do not.

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
| L1 | `go test ./...` in anycli: httptest fake for `api.iterable.com`; assert `Api-Key` header injected, `<region>[:<alias>]:<key>` split (2/3 parts; `us`/`eu` → base URL; last part → key; middle alias ignored; invalid region, part count ∉ {2,3}, or empty region/key → exit 2), request shapes for each verb, and both text + `--json` error rendering. No real API. | No |
| L2 | `ANYCLI_CRED_API_KEY="us:<key>" anycli iterable -- list list` (and a `user get`, `event track`, `campaign list`) against the **real** Iterable API; plus one aliased `ANYCLI_CRED_API_KEY="us:staging:<key>"` run to prove the alias segment is ignored by anycli. Mandatory before pin bump. | **Yes** — one server-side API key from a real Iterable project (test-account pool). |
| L3 | `provider-gen` + `provider-gen --check` (five projections) locally on-branch — **now passes** the single-required-field check (one field); `helio-cli` build via uncommitted `go.mod replace` → anycli branch; `go test ./...` both repos incl. `helio-cli/.../cmds/tool`, the generic `regionPrefixIdentityDeriver` unit test (un-aliased → `accountKey=region`; aliased → `accountKey=region:alias`; brand from `presentation.name`; **two un-aliased same-DC keys derive the same `accountKey` → assert the re-key/overwrite path, not a second connection**), and the per-provider deriver-selection test in integration-service. | No |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `iterable`, seed `access_token`=`us:staging:<key>`, `account_key`=`us:staging`) → `heliox tool iterable -- list list` reaches the live API through the real token gateway. api_key is seedable (not a minted provider). | **Yes** — same real key as L2. |
| L5 | Full connect path once before the visible flip: `heliox tool iterable auth` → connect link → **paste `<region>:<alias>:<key>` (or `<region>:<key>`) as the single secret** in the real connect UI (`POST /connections/credentials`) → connection shows connected/configured with label "Iterable — US (staging)" (`GET /connections`) → one **unseeded** live `heliox tool iterable -- list list`. This is the **api_key key-entry L5 path** (master plan §2), not the OAuth consent path. | **Yes** — real key; agent-drivable via agent-browser, human fallback on UI breakage. |

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
  two integration-service items in §3 — (1) the generic, bundle-parameterized
  `regionPrefixIdentityDeriver` (brand from `presentation.name`, region from a
  shared map, optional alias) with its unit test, and (2) the reviewed
  `identity.deriver` bundle enum + per-provider deriver selection in
  `composeProviderRegistration`, with its test. Both are **shared** with the
  US/EU-residency siblings — see the §3 batch-lead reconciliation note so
  Iterable and Amplitude land one deriver + one enum value, not two.
  Run `make test-integration-service` for both. No `service/adapter_*.go`, no
  token-gateway change — the runtime path stays generic manual_credentials with
  the single-secret storage face.
- Stage 7 icon: `ui/helio-app/src/integrations/icons/iterable.svg` +
  `providerIcons.ts` (manual, batch-end).
- Stage 8 docs: provider sub-doc under
  `agents/plugins/heliox/skills/tool/`, plugin version bump (batch-end
  publish). Document the `<region>[:<alias>]:<key>` paste format for the connect
  step, including that omitting the alias means one connection per data center
  (a re-key on the next un-aliased same-DC connect) and that a distinct alias is
  how a teammate runs multiple same-DC projects as isolated connections.
- Stage 10 rollout: deploy hidden → L5 → flip `visible: true` + regenerate as
  the single go-live change.
