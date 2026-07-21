# Tool design: Freshservice

**Catalog row:** #79 · Product: Freshservice · anycli id `freshservice` · provider key `freshservice` · auth lane `api_key` · Wave 2 · Support.
**Branches:** anycli `tool/freshservice` (this worktree), Helio `tool/freshservice` (`2helio/.claude/worktrees/tool-freshservice`).
**Status:** design. Scratch file — the batch lead strips it at batch end.

## 0. Verification summary (independent, against official docs + repo code)

Sources read directly: the official Freshservice API v2 reference (`api.freshservice.com`)
— base-URL/auth/pagination/rate-limit, create-ticket (mandatory-field + status/priority/
source/type enum codes) and filter-tickets (30/page × 10-page = 300-result cap, no
`per_page`) pages — plus repo code
(`integration-service/model/catalog.go`, `model/runtime_contract.go`,
`service/manual_credentials_identity.go`, `service/manual_token_verifier.go`,
`service/provider_registry.go`) and the shipped `mongodb` bundle as the
instance-URL precedent.

- **Auth lane `api_key` HOLDS.** The self-serve, no-review path is HTTP Basic
  auth with a per-profile API key (`-u apikey:X`, key as username, any dummy
  password). Verified facts:
  - Base URL is **per-account**: `https://<domain>.freshservice.com/api/v2/…`
    (HTTPS only, no custom CNAME). There is no global API host — every request
    needs the account domain.
  - Basic username/password auth was deprecated 2023-05-31; **API-key Basic
    auth is the current documented scheme**. The key lives in Profile Settings.
  - **DIVERGENCE from the audit note, recorded per the independent-judgment
    instruction.** The audit row 81 says "no viable multi-tenant path"; the API
    docs state Freshservice *does* expose **OAuth 2.0 since April 2024**. On
    inspection this OAuth is bound to the **Freshworks Developer / Marketplace
    app platform** (per-endpoint scopes, app published/reviewed on the
    Freshworks Neo platform), not a self-serve distributable authorization-code
    client an arbitrary customer can consent to for a shared external service.
    Under the OAuth-audit rubric (multi-tenant, self-serve or review-gated
    authorization-code) it is at best a heavy partner-review lane with an
    api_key fallback — so **api_key remains the correct v1 lane** and the
    catalog lane is unchanged. Flag for stage-1 re-confirmation only; do not
    re-lane.
  - **No `GET /agents/me` / current-agent endpoint exists** (confirmed in the
    docs: "your ability to access data depends on your profile's permissions",
    with no whoami route). This matters for connect-time verification (§3): there
    is no clean HTTPS identity endpoint returning a stable agent id + label.

- **The single-secret manual-credential plane cannot natively express
  Freshservice's two required inputs (domain + key) — but the existing
  `manual_credentials` (DSN-class) strategy solves it with ZERO platform
  growth.** Three constraints verified in code:
  1. `model/runtime_contract.go` `validateCredentialInputSchema`: a declared
     `auth.credential_input` schema **must be exactly one required field**
     (design 317 D5 P3 — the storage face is a single secret in the user-token
     payload; "Relax this together with the D8 multi-field vault face"). A
     naive `[domain, api_key]` two-field form is rejected at startup.
  2. `model/catalog.go` `APIKeyPolicy` is `{Header, SetupURL}` only — no auth
     *scheme*, and the `declarativeManualTokenVerifier` sets the header to the
     **raw** token (`req.Header.Set(APIKey.Header, token)`). It cannot emit
     `Authorization: Basic base64(apikey:X)`, and its `Identity.URL` is a
     single **static** HTTPS string with no per-domain templating. So the
     `manual_api_token` verify path is unusable for Freshservice.
  3. But `service/provider_registry.go` composes `manual_credentials` with the
     **`dsnHostIdentityDeriver`**, and that deriver
     (`service/manual_credentials_identity.go`) is **generic**: it does
     `url.Parse(secret)` and returns the host as the account key/label, doing
     **no** provider-side HTTP call and never leaking the secret into the
     identity map. A **URL-shaped single credential** therefore fits the
     existing plane exactly, the same way a MongoDB DSN does.

- **Conclusion / recommended v1 (mongodb-clone, ships hidden today):** model
  the one secret as a URL that carries both pieces —
  `https://<api_key>@<domain>.freshservice.com`. `url.Parse` yields
  `Host = <domain>.freshservice.com` (the human-readable, stable account
  key/label) and keeps the api_key in userinfo (never entering the identity
  map). The anycli service re-parses the same blob to build the base URL and the
  Basic-auth header. **No new CredentialSource, no verifier change, no
  multi-field vault work.** This is identical in shape to the shipped `mongodb`
  provider and to the concurrent ServiceNow tool (`<instance>.service-now.com`,
  task #202) — the two should adopt one agreed instance-URL convention.
  A labelled two-field form (`domain` + `api_key`) is the cleaner **eventual**
  UX but is gated on the design-317 **D8 multi-field vault face**; it is a
  follow-up, not a v1 blocker (§4.3).

## 1. What an AI teammate does with Freshservice → API surface

Freshservice is IT Service Management (ITSM). An AI teammate on an IT/service desk:
triages incoming tickets (incidents + service requests), replies to the requester or
adds a private note, reassigns/retags/changes status/priority/group, looks up the
requester (the employee who raised it) and the agent/group to route to, filters the
queue the way a human agent does, and — the ITSM differentiator vs Freshdesk — looks
up **assets** (CMDB) referenced by a ticket. That drives the v1 surface, all under
`https://<domain>.freshservice.com/api/v2`:

| Group | Endpoints wrapped | Why |
|---|---|---|
| ticket | `GET /tickets` (paginated), `GET /tickets/{id}`, `POST /tickets`, `PUT /tickets/{id}`, `GET /tickets/filter?query="…"` | Core ITSM object: read/create/update status·priority·group·agent; filter is how agents work a queue |
| ticket conversation | `GET /tickets/{id}/conversations`, `POST /tickets/{id}/reply`, `POST /tickets/{id}/notes` | Replying (public) and noting (private) IS the job |
| requester | `GET /requesters`, `GET /requesters/{id}`, `GET /requesters?email=` | Who raised the ticket; employee lookup |
| agent | `GET /agents`, `GET /agents/{id}` | Assignment targets + operator self-context (no `/me`, so `?email=` lookup) |
| group | `GET /groups` | Assignment groups |
| asset | `GET /assets`, `GET /assets/{display_id}`, `GET /assets?filter="…"` | CMDB lookup — the ITSM differentiator |

Deliberately **out of v1** (all additive later): Changes / Problems / Releases (heavier
ITIL modules), Service Catalog ordering + Service Requests item flow, Solutions/KB
writes, SLA/business-hours/automation admin, ticket delete/forget, time entries. The
v1 cut mirrors the Zendesk/Freshdesk "triage + reply + route + look up" teammate loop.

Pagination (standard list endpoints, e.g. `GET /tickets`): `page` (1-based) + `per_page`
(default 30, max 100); next-page URL in the `link` response header — surface
`next_page`/`link` in JSON output. **`GET /tickets/filter` is the exception** and does
**not** share this contract: it returns **30/page fixed (`per_page` is ignored),
`page` must be 1–10 (hard cap 300 results), and only tickets created in the last 30 days
are returned by default**. Because the paging semantics genuinely differ, filtered
listing is a **separate `ticket search` subcommand** (§2) rather than a `--filter` flag on
`ticket list`, so a paging flag never silently changes meaning. Rate limits: on
`429`, surface a typed API error including `Retry-After` and the `X-RateLimit-*`
headers; **no silent retry loop** (fail fast per Helio hard rules — invalid requests
still consume quota, so a retry storm is actively harmful).

## 2. anycli definition

**Stage-1 rubric → `service` type.** There is no official agent-friendly Freshservice
CLI (the Freshworks CLI `fdk` scaffolds Marketplace apps; it is not a ticket/asset
CLI). So all `cli`-type conditions fail at the first test → implement `service` type
against the HTTP API, like 21/23 existing definitions. Follow the `internal/tools/notion/`
shape (`Service{BaseURL, HC, Out, Err}`, httptest-overridable base).

**Axes (all three identical → no `toolToProvider` divergence entry):**
① CLI command word `freshservice` (flat, no group) · ② anycli id `freshservice` ·
③ provider key `freshservice`. Go package `internal/tools/freshservice/` (no dashes →
no normalization), registered in `internal/tools/register.go` `init()` as
`RegisterService("freshservice", &freshservice.Service{})` (registration rides the
batch-end merge; the package + `definitions/tools/freshservice.json` merge freely
mid-batch).

`definitions/tools/freshservice.json` (single injected credential — the URL blob):

```json
{
  "name": "freshservice",
  "type": "service",
  "description": "Freshservice ITSM as a tool (per-account domain + API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "freshservice_url"},
        "inject": {"type": "env", "env_var": "FRESHSERVICE_URL"}
      }
    ]
  }
}
```

- **`source.field` MUST match the bundle's `credential.fields` key, not
  `access_token`.** helio-cli `internal/toolcred/resolver.go` `credentialData()`
  returns the bundle-projected `resp.Credential` map **verbatim** whenever it is
  non-nil (always true for a bundle that declares `credential.fields`); the
  legacy top-level `access_token` is dropped. Since §4.1 projects the secret as
  `credential.fields: freshservice_url` (not `access_token`), the runtime
  credential map is `{freshservice_url, account_key}` with **no** `access_token`
  key. Selecting `source: {field: access_token}` would resolve to empty, inject a
  blank `FRESHSERVICE_URL`, and fail every command (malformed-blob → exit 2).
  The shipped `mongodb` precedent proves the contract: its anycli definition uses
  `source: {field: connection_string}` to match its bundle key
  `connection_string` (both verified in repo code). This cross-repo field-name
  join is invisible to L1–L3 (the anycli definition + harness are self-consistent
  on whatever field name they pick; provider-gen validates only the bundle) and
  first surfaces at L4 through the real token gateway — see §5 L4.
- **One credential env var, `FRESHSERVICE_URL`**, carrying the whole
  `https://<api_key>@<domain>.freshservice.com` blob. The service parses it once at
  startup: `url.Parse` → `Host` builds `BaseURL = https://<host>/api/v2`; the userinfo
  username is the API key → `Authorization: Basic base64(<api_key>:X)`. The `:X` dummy
  password is fixed (Freshservice ignores it). Base64 the pair itself; do not put the
  key in the URL sent to Freshservice (strip userinfo before building request URLs).
- **Why one blob, not two env vars:** Helio's manual plane persists exactly one secret
  per connection (§0). The domain is not separately sourced; it rides inside the blob
  and the service extracts it. This is the mongodb precedent, not a Freshservice
  special case.
- Malformed blob (no host, or key missing) → exit 2 (usage) with static guidance that
  never echoes the secret.

**Command tree (axis-① word `freshservice`, flat):**

```
freshservice ticket list    [--updated-since T] [--per-page N] [--page N]          # GET /tickets     — per_page ≤100, walks full set
freshservice ticket search  --query "status:2 AND priority:1" [--page N]           # GET /tickets/filter — 30/page fixed, page 1–10, 30-day window
freshservice ticket get     <id> [--conversations]
freshservice ticket create  --subject S --description D --email E [--status ST] [--priority P] [--group-id G] [--agent-id A] [--type T]
freshservice ticket update  <id> [--status ST] [--priority P] [--group-id G] [--agent-id A] [--tags a,b]
freshservice ticket reply   <id> --body BODY
freshservice ticket note    <id> --body BODY [--private]
freshservice requester list [--email E] [--per-page N] [--page N]
freshservice requester get  <id>
freshservice agent list     [--email E] [--per-page N] [--page N]
freshservice agent get      <id>
freshservice group list     [--per-page N] [--page N]
freshservice asset list     [--filter "…"] [--per-page N] [--page N]
freshservice asset get      <display-id>
```

**`ticket create` required-field reality — always the agent-on-behalf context.** Helio
*always* authenticates with a stored API key, and Freshservice treats every API-key create
as an **agent creating on behalf of a requester** — in that context `status` and
`priority` are **server-side mandatory** (the Service Portal / Freshdesk default them; the
agent API does not). Leaving them optional and unset would 400 every `ticket create` —
breaking the primary write path. So the CLI **supplies defaults when the flags are omitted:
`--status` → `2` (Open), `--priority` → `2` (Medium)**, both overridable, and the applied
default is documented in help + the AI doc. Caveats the create docs do **not** enumerate,
worth a one-line note because there is no create-time escape hatch (**`bypass_mandatory`
exists only on `PUT`/update, not on `POST`**): an account **priority matrix** can override
the sent `priority` (Admin → Priority Matrix); account-configured **mandatory custom
fields** (and sometimes `department_id`) can still 400 a create with fields this synopsis
doesn't list. Surface the raw `errors[]` body verbatim so the agent sees exactly which
field the account requires. (Also: pass no `responder_id` to leave a ticket unassigned —
`null` is rejected; omit the flag.)

**`ticket list` vs `ticket search` — split so a paging flag means one thing per command.**
`ticket list` → `GET /tickets`: `--per-page` adjustable up to **100**, `--page` walks the
full dataset via the `link` header. `ticket search` → `GET /tickets/filter`: the endpoint
**fixes 30 results/page and ignores `per_page`**, so `ticket search` deliberately exposes
**no `--per-page`**; `--page` is validated to **1–10** (hard cap 300 results) and the CLI
errors on out-of-range rather than silently returning nothing, and the default 30-day
created-window is documented. This split (not one `list --filter`) is the fail-fast /
no-silent-behavior-divergence resolution required by the Helio hard rules — a flag never
silently no-ops depending on another flag.

**JSON output shape (provider-neutral, agent-tuned — the 003 §3 convention):** every
command prints a single JSON object to stdout. List commands:
`{"items": [...], "page": N, "per_page": N, "next_page": N|null}` (`next_page` derived
from the `link` header, so the agent paginates without parsing Freshservice's raw
envelope). Get/create/update: the bare resource object `{"ticket": {...}}` unwrapped to
the resource fields. Errors: `{"error": {"status": <http>, "message": "...",
"provider_code": "..."}}` on stderr, carrying Freshservice's `description`/`errors[]`
body and, on 429, `retry_after`. Exit codes: `0` success · `1` runtime/API failure
(typed `apiError` with HTTP status + body) · `2` usage/missing-credential.

Unit tests (TDD, httptest fakes): base-URL + Basic-header construction from a blob;
each command's request shaping; pagination `link`→`next_page`; `ticket create` injects the
`status=2`/`priority=2` defaults when the flags are omitted (and honours overrides);
`ticket search` sends no `per_page` and rejects `--page` outside 1–10; 401→typed auth
error; 429→`retry_after`; malformed-blob→exit 2. No real network in L1.

## 3. Credential fields and the exact auth flow

- **Provider-side credential:** one Freshservice **API key** (Profile Settings →
  below the change-password box). It is account-scoped and inherits the profile's
  permissions — so the connected key should belong to an agent with the ticket/asset
  scope the teammate needs.
- **What the user supplies at connect (v1, single field):** a URL-shaped string
  `https://<api_key>@<domain>.freshservice.com`. The bundle marks it `secret: true`
  (the whole blob contains the key). Placeholder + AI-doc show the exact shape and note
  URL-encoding the key if it contains reserved characters (Freshservice keys are
  normally alphanumeric, so this is a rare caveat).
- **Storage:** stored as the single secret in the user-token payload
  (`token.access_token`), same write path as mongodb — zero new CredentialSource.
- **Identity / account key:** `runtime_strategy: manual_credentials` →
  `dsnHostIdentityDeriver` `url.Parse`s the blob and returns
  `Host = <domain>.freshservice.com` as **both** the stable dedup account key and the
  human-readable displayed account. The api_key in userinfo never enters the identity
  map (verified in code). **No connect-time provider call is made** (design 317 OQ1
  no-verify): a bad key/domain surfaces at first use via AnyCLI's `CredentialRejected`
  classification (a 401 from Freshservice). Accepted trade-off, identical to mongodb.
- **Runtime auth (every request):** `Authorization: Basic base64(<api_key>:X)` against
  `https://<domain>.freshservice.com/api/v2/…`. Keys are long-lived (no expiry, no
  refresh) → `refresh_lease: none`, `disconnect_mode: local_only` (Freshservice has no
  key-revoke API; disconnect just drops the stored secret).

## 4. Helio provider bundle plan

### 4.1 `integrations/providers/freshservice/provider.yaml` (recommended v1, mongodb-clone)

```yaml
schema: helio.provider/v1
key: freshservice
go_name: Freshservice

presentation:
  name: Freshservice
  description_key: freshservice
  consent_domain: freshservice.com
  visible: false            # hidden-first; flip is the single go-live change (stage 10)

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: freshservice_url
        label_key: freshservice_url
        secret: true
        placeholder: "https://<api_key>@<your-domain>.freshservice.com"
        required: true       # exactly one required field (design 317 D5)
    setup_url: https://support.freshservice.com/support/solutions/articles/50000000306-where-do-i-find-my-api-key-

identity:
  source: strategy           # no HTTPS whoami; host derived from the blob

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
    freshservice_url: token.access_token
    account_key: connection.account_key

tool:
  name: freshservice
  kind: api-key            # wire-compat value (317 D2); client routes the drawer by auth_type
```

- **Axes:** directory/`key` = `freshservice` (③), `tool.name` = `freshservice` (②),
  command word `freshservice` (①) — all identical, so **no `toolToProvider` entry** and
  no `toolGroups` change. Flat command, not a family group.
- `auth.required_config_fields` is **absent** — a manual-credential provider needs no
  integration-service client id/secret, so nothing lands in `config/` or the `deploy/`
  Helm Secret for this tool (unlike the OAuth siblings). Renders `configured: true`
  with no config, safe to ship hidden immediately.
- **Zero service code:** `manual_credentials` + `dsnHostIdentityDeriver` +
  `noopRevoker` are already composed in `provider_registry.go`. No `adapter_*.go`.

### 4.2 Other required Helio-side artifacts (ride the batch-end merge)

- **UI icon:** `ui/helio-app/src/integrations/icons/freshservice.svg` + hand-register in
  `providerIcons.ts` (manual, never generated).
- **Generation:** from `go-services/integration-service`, `go run ./cmd/provider-gen`
  then `--check`; commit all five projections together with the bundle.
- **AI-facing doc:** provider sub-doc under `agents/plugins/heliox/skills/tool/`
  documenting the command tree, the URL-blob credential format, filter-query syntax, the
  split `ticket list` vs `ticket search` pagination contracts, and — because the
  create/update APIs accept **integer codes, not labels** — the **enum code tables the
  agent must send** (without these, `--status`/`--priority`/`--source` are effectively
  unusable):
  - **status:** `2` Open · `3` Pending · `4` Resolved · `5` Closed (accounts may add
    custom statuses with codes ≥ 6)
  - **priority:** `1` Low · `2` Medium · `3` High · `4` Urgent
  - **source:** `1` Email · `2` Portal · `3` Phone · `4` Chat · `5` Feedback widget ·
    `9` Walkup · `10` Slack · `15` MS Teams · … (publish the full list in the doc)
  - **type:** **DIVERGENCE from the review finding, verified against the official
    create-ticket schema** — `type` is a **string**, not an integer enum: e.g.
    `"Incident"` (default) or `"Service Request"`. Document it as a free-string field, not
    a numeric code, so the agent doesn't send an integer that Freshservice rejects.

  Plugin version bump + marketplace publish (one per batch).

### 4.3 Capability decision to surface to the batch lead (stage 1)

The v1 above needs **no** platform growth. The one open call: whether to ship the
URL-blob UX now, or wait for the **design-317 D8 multi-field vault face** to give a
labelled two-field form (`domain` + `api_key`) — the genuinely clean UX. Recommendation:
**ship the URL-blob now, hidden** (Freshservice is Wave 2; blocking Wave 2 api_key tools
on D8 defeats the "pure agent throughput" lane), and adopt the two-field form as a
later, mechanical bundle swap once D8 lands. This is a **shared** decision with the
`mongodb` line and the concurrent **ServiceNow** api_key tool (task #202) — all three
are instance-URL + key providers; converge on one convention (blob format, field label
key, and the eventual D8 two-field schema) rather than three divergent ones.

## 5. Test plan → the skill's five layers

| Layer | What runs for Freshservice | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in anycli: httptest fakes cover blob→BaseURL+Basic-header, every subcommand's request/response shaping, `link`→`next_page` pagination, 401 auth error, 429 `retry_after`, malformed-blob→exit 2. TDD, no network. | No |
| **L2** harness real-API | `anycli freshservice -- ticket list --per-page 1` etc. with `ANYCLI_CRED_*` set to a real blob; hit a real Freshservice trial and confirm real tickets/requesters/agents/assets return, pagination advances, and a bad key yields a clean typed 401. No Helio services. | **Yes** — a real Freshservice account + API key (test-account pool lane 2). Free 14-day trial suffices. |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (five projections regenerate clean; strict decode passes with only reviewed fields); anycli unit suite + `helio-cli`/integration-service suites green. Validate on-branch with local regen + `go.mod` `replace` → anycli branch (regen and replace **not committed**; batch lead produces the canonical regen). | No |
| **L4** singleton + seed | `make run-singleton` + `POST /internal/test-only/connections/seed` seeding the URL blob, then `heliox tool freshservice -- ticket list` through the real token gateway (hidden tool runs as a cobra-hidden command — no visible flip needed). Success = the seeded blob reaches the live Freshservice API and returns real data. **This is the layer that verifies the cross-repo field-name join** (§2): the bundle projects `credential.fields: freshservice_url`, the resolver returns that map verbatim, and the anycli `source: {field: freshservice_url}` selects it → a non-empty `FRESHSERVICE_URL` reaches the service. A field-name mismatch (e.g. `access_token`) passes L1–L3 self-consistently and only breaks here, as an empty inject → exit 2. | **Yes** — same real blob as L2 (seed bypasses the connect UI). |
| **L5** full connect flow | Once before the visible flip: open the connect link → paste the URL blob through the real connect UI (stored via write-only `POST /connections/credentials`) → connection shows connected/configured in `GET /connections` (account label = `<domain>.freshservice.com`) → one **unseeded** live `heliox tool freshservice -- ticket list` through the real token gateway succeeds. This is the **api_key key-entry L5 path** (master plan §2), agent-drivable via agent-browser with human fallback; **not** the OAuth-consent path. | **Yes** — real account; agent pastes the key from the account pool. |

**Layers needing externally supplied credentials:** L2, L4, L5 (one real Freshservice
account + API key from the test-account pool). L1 and L3 are hermetic. There is **no**
OAuth app-registration dependency (lane 1) — Freshservice is api_key, so the only
external dependency is the test account itself.

**Definition of done / flip gate:** all five layers green · AI-facing doc
published · icon registered in `providerIcons.ts` · **the i18n locale strings the
bundle references ship in all 9 supported locales** (`de-DE`, `en-US`, `es-ES`,
`fr-FR`, `ja-JP`, `ko-KR`, `pt-BR`, `zh-CN`, `zh-TW`) — the
`presentation.description_key` `freshservice` (rendered as `tools.desc.freshservice`)
and the `credential_input.label_key` `freshservice_url` — mirroring the mongodb
bundle's flip-gate note that gates `visible: true` on `tools.desc.mongodb` shipping
in all 9 locales · then `presentation.visible: true` + regenerate as the single
go-live change.
