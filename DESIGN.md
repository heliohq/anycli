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
constant. We capture it as an explicit, non-secret field (`region`, enum
`us|eu`, default `us`). We do **not** auto-probe both hosts — silent
DC-fallback is forbidden by the repo's fail-fast hard rule and would leak the
key to the wrong DC. This mirrors the ServiceNow/Braze precedent where an
instance/region is a first-class captured field alongside the secret.

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

### Credential bindings (`definitions/tools/iterable.json`)

Two bindings — the secret key and the non-secret region:

```json
{
  "name": "iterable",
  "type": "service",
  "description": "Iterable cross-channel marketing: users, events, lists, campaigns, transactional email",
  "auth": {
    "credentials": [
      { "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "ITERABLE_API_KEY"} },
      { "source": {"field": "region"},
        "inject": {"type": "env", "env_var": "ITERABLE_REGION"} }
    ]
  }
}
```

The service reads `ITERABLE_REGION` (`us`→`api.iterable.com`,
`eu`→`api.eu.iterable.com`; empty defaults to `us`; any other value is a
fail-fast exit 2), then sends every request with header `Api-Key:
$ITERABLE_API_KEY`. Multi-binding manual credentials are the established
precedent (mixpanel / braze / snov / servicenow inject a secret + a
non-secret host/region together).

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

Hidden-first. Manual-credential (api_key) bundle, ServiceNow/mongodb-shaped —
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

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: iterable_api_key
        secret: true
        required: true
        placeholder: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
      - name: region
        label_key: iterable_region
        secret: false
        required: true
        # closed enum us|eu; default us — the data center the key belongs to
    setup_url: https://support.iterable.com/hc/en-us/articles/360043464871-API-Keys

identity:
  source: strategy      # no project-name endpoint; label derived from region

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
    api_key: token.access_token       # secret via existing UpsertUserToken path
    region: connection.account_key     # non-secret, also the label seed
    account_key: connection.account_key

tool:
  name: iterable
  kind: api-key
```

Axis notes: `key:` == directory name == `iterable` (generator enforces
equality). `tool.name` == `iterable` == anycli id. No `tool.command` (flat,
ungrouped). No `experiment:` (GA, not preview-gated).

### Identity / account label

The API key is **project-scoped with no user or project-name endpoint**, so
there is nothing to `GET` for a display name — same situation as mongodb
(`identity.source: strategy`). Label is region-derived: "Iterable — US" /
"Iterable — EU". The `region` value is the human-readable `account_key`, so a
teammate connecting a US and an EU project produces two distinct connections
under one provider (`mode: isolated` supports this).

### Possible integration-service capability growth (Option A, one narrow touch)

Connect-time **verification** is desirable (mongodb ships no-verify only
because a DSN has no cheap probe; Iterable has `GET /api/campaigns`, a 200-on-
valid-key probe). If the existing manual-credential verifier already supports
an arbitrary **custom header name** with the raw key as its value, reuse it
with `Api-Key` + verify URL `https://api.iterable.com/api/campaigns` (region-
selected). If today's verifier only knows Bearer/Basic schemes (tally=Bearer,
fullstory/servicenow=Basic precedents), add one reviewed enum value — a
"custom-header" verifier that injects `{header_name}: {key}` and treats 2xx as
valid — exactly the Option-A capability-growth pattern prior api_key tools
used (lemlist basic_password, tally Bearer verifier). If that lands, add
`identity.verify` metadata to the bundle; otherwise ship no-verify like
mongodb (a bad key then surfaces at first use via anycli `CredentialRejected`).
Decide at stage-2 implementation; either way it is a *narrow* touch, no
`service/adapter_*.go` — the runtime path stays generic manual_credentials.

## 4. Test plan — five layers (SKILL.md stage 9)

| Layer | What runs | Needs external creds? |
|---|---|---|
| L1 | `go test ./...` in anycli: httptest fake for `api.iterable.com`; assert `Api-Key` header injected, region→base-URL mapping (us/eu/empty/invalid), request shapes for each verb, and both text + `--json` error rendering. No real API. | No |
| L2 | `ANYCLI_CRED_API_KEY=<key> ANYCLI_CRED_REGION=us anycli iterable -- list list` (and a `user get`, `event track`, `campaign list`) against the **real** Iterable API. Mandatory before pin bump. | **Yes** — one server-side API key from a real Iterable project (test-account pool). |
| L3 | `provider-gen` + `provider-gen --check` (five projections) locally on-branch; `helio-cli` build via uncommitted `go.mod replace` → anycli branch; `go test ./...` both repos incl. `helio-cli/.../cmds/tool` and any integration-service verifier test. | No |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `iterable`, seed `access_token`=key, `account_key`=`us`) → `heliox tool iterable -- list list` reaches the live API through the real token gateway. api_key is seedable (not a minted provider). | **Yes** — same real key as L2. |
| L5 | Full connect path once before the visible flip: `heliox tool iterable auth` → connect link → **paste key + pick region** in the real connect UI (`POST /connections/credentials`) → connection shows connected/configured (`GET /connections`) → one **unseeded** live `heliox tool iterable -- list list`. This is the **api_key key-entry L5 path** (master plan §2), not the OAuth consent path. | **Yes** — real key; agent-drivable via agent-browser, human fallback on UI breakage. |

L2/L4/L5 all consume the **same single server-side API key** from the account
pool — no OAuth app, no lane-1 dev-app creation, no review clearance gates
anything. The only gate on the visible flip is L5 + docs published + icon
registered + the anycli pin shipping the `iterable` definition.

## 5. Remaining stage checklist (per SKILL.md)

- Stage 3 pin bump: batch-end only (one anycli tag / pin per batch); on-branch
  uses the local `replace`, never committed.
- Stage 5 generate: five projections committed together at batch end (this
  branch is **expected** to fail `provider-gen --check` in CI until then — do
  not commit a local regen to green the branch).
- Stage 6 service: manual_credentials needs **zero** OAuth service code and no
  `config/`+`deploy/` secret landing (no `required_config_fields`); the only
  possible touch is the §3 verifier enum (Option A).
- Stage 7 icon: `ui/helio-app/src/integrations/icons/iterable.svg` +
  `providerIcons.ts` (manual, batch-end).
- Stage 8 docs: provider sub-doc under
  `agents/plugins/heliox/skills/tool/`, plugin version bump (batch-end
  publish).
- Stage 10 rollout: deploy hidden → L5 → flip `visible: true` + regenerate as
  the single go-live change.
