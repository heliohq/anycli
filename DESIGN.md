# Tool design: FullStory

Scratch design for the FullStory `heliox tool` provider. Batch-lead strips
this file at batch end. English only.

- **anycli id (axis ②):** `fullstory`
- **provider catalog key (axis ③):** `fullstory`
- **CLI command word (axis ①):** `fullstory` (flat, ungrouped)
- **auth lane:** `api_key`
- **catalog row:** 122 · Wave 2 · category Analytics
- **tool form:** `service` type (no official, non-interactive, `--json`-capable
  FullStory CLI binary exists; wrap the Server API over HTTP)

## 1. Audit verification vs official docs

Master-plan catalog and the 2026-07-21 OAuth audit (oauth-audit.md row 124)
both classify FullStory as **`api_key`** — "no viable multi-tenant path". I
verified this against FullStory's official developer docs and **confirm it**:

- Authentication is a single account-scoped API key generated in-app
  (Settings → Integrations → API Keys), passed in the `Authorization` header.
  There is **no OAuth2 authorization-code flow** — no `authorize`/`token`
  endpoints, no per-customer app registration, no consent screen. A shared
  Helio-owned client cannot mint per-account tokens. `api_key` is correct.
  Source: https://developer.fullstory.com/server/authentication/ ,
  https://help.fullstory.com/hc/en-us/articles/360052021773-Managing-API-Keys

**Divergence recorded (auth scheme).** The audit/catalog only fix the *lane*,
not the header shape. The official docs specify a **non-standard use of the
`Basic` scheme**: the header value is literally

```
Authorization: Basic {YOUR_API_KEY}
```

where `{YOUR_API_KEY}` is the **raw key token used verbatim** — it is *not*
base64(`user:password`) as RFC 7617 Basic auth normally requires, and it is
*not* a `Bearer` token. FullStory's key already embeds its data-center routing
prefix (format `<datacenter>.<token>`, e.g. `na1.xxxxx`), and all requests hit
`https://api.fullstory.com` regardless of region. This "`Basic` keyword +
raw key, no base64" shape is the one thing both the anycli service and the
Helio manual-token verifier must implement precisely; it differs from the
sibling analytics precedents (dataforseo = `Basic` + base64(login:password),
freshdesk = `Basic` + base64(key:X)). See §4 and §5 capability note.

## 2. API surface wrapped and why

FullStory is a session-replay + product-analytics / digital-experience tool.
An AI teammate's high-value jobs are **investigative reads** ("pull up this
user's recent sessions after their bug report", "what did this user do in
session X", "does this user exist / what are their properties") plus a small
set of **writes** (record a server-side custom event, upsert user
properties). The v2 Server API (`https://api.fullstory.com/v2`) plus the two
still-current v1 read endpoints cover exactly this. Endpoints wrapped:

| Group | Verb | HTTP | Path | Why an AI teammate needs it |
|---|---|---|---|---|
| session | `list` | GET | `/sessions/v2?uid=|email=&limit=` | The killer flow: given a user's uid/email, return their recent session replay URLs (`results[].{id, app_url, created_time}`, default 20). Entry point for every "investigate this user" task. |
| session | `events` | GET | `/v2/sessions/{session_id}/events` | Full captured event stream for one session (`session_id` is the `deviceId:sessionId` value from `session list`). Lets the teammate reason over what happened without opening the replay UI. |
| user | `get` | GET | `/v2/users?uid=` (or `/v2/users/{id}`) | Resolve a user by app uid → FullStory `id`, `display_name`, `email`, custom `properties`. Confirms identity before any session/deletion action. |
| user | `list` | GET | `/v2/users` (filter criteria) | Find users matching filter criteria. |
| user | `upsert` | POST | `/v2/users` | Create/update a single user's properties server-side (product-analytics enrichment). |
| event | `create` | POST | `/v2/events` | Record a server-side custom event against a user (funnel/behavior instrumentation). |
| me | `whoami` | GET | `/me` (v1) | Officially documented "test your key" endpoint; returns the key's `role` (USER/ARCHITECT/ADMIN). Doubles as anycli connectivity check and the Helio bundle's identity/verification endpoint (§5). |

**Deliberately out of scope for v1 of the tool:**
- Batch import endpoints (`/v2/users/batch`, `/v2/events/batch`) — bulk ETL,
  not interactive teammate work; add later if demand appears.
- AI Session Summary / Session Context (`/v2/sessions/{id}/summary`,
  `/context`) — part of the separate Anywhere: Activation product and require
  a `config_profile` id the account may not have provisioned; revisit once a
  test account confirms availability.
- Segment Export / privacy / extraction-rule admin — Architect/Enterprise-only
  surfaces; not general teammate flows.

Rationale for read-heavy shape: the catalog places FullStory in Analytics and
the account pool for L2/L5 is a standard (non-Enterprise) tenant, so the design
targets endpoints reachable with a **Standard**-level key (send data, list
sessions) plus **Get/List User** which most plans expose. Export/delete
(Architect, Enterprise-only) is intentionally excluded so the tool is usable on
the test tier.

## 3. anycli definition

**Type:** `service` (stage-1 rubric: no official non-interactive `--json` CLI
binary; implement against the HTTP API). Go package `internal/tools/fullstory/`
(id has no dashes → package name `fullstory`), registered
`RegisterService("fullstory", &fullstory.Service{})` in
`internal/tools/register.go`.

**Definition `definitions/tools/fullstory.json`:**

```json
{
  "name": "fullstory",
  "type": "service",
  "description": "FullStory digital-experience analytics: user sessions, events, and profiles via the Server API",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "FULLSTORY_API_KEY"}
      }
    ]
  }
}
```

- Single credential field `api_key` (the raw FullStory key), injected as env
  `FULLSTORY_API_KEY`. The service constructs the header value
  `"Basic " + os.Getenv("FULLSTORY_API_KEY")` — **no base64, no `Bearer`**
  (the §1 divergence). This mirrors the notion/bitly service shape (fixed
  base URL `https://api.fullstory.com`, injected-token header) rather than the
  connection-string shape (mongodb).

**Command tree** (cobra, resource-grouped, copying the notion reference shape —
`BaseURL`/`HC`/`Out`/`Err` struct so httptest fakes can drive it):

```
fullstory session list   --uid <id> | --email <e> [--limit N]
fullstory session events --session <deviceId:sessionId>
fullstory user get       --uid <id> | --id <fsid> [--include-schema]
fullstory user list      [--uid <id>] [--email <e>] [filters]
fullstory user upsert    --uid <id> [--display-name ..] [--email ..] [--prop k=v ...]
fullstory event create   --uid <id> --name <event> [--prop k=v ...]
fullstory me             # GET /me — role/permission check
```

**JSON output shape.** All commands emit provider-neutral JSON to stdout;
list-style commands wrap in `{"results":[...]}` echoing FullStory's own
envelope, single-object gets emit the object directly. Exit-code contract from
notion: `0` success, `1` runtime/API failure via typed `apiError` (with
`--json` structured error envelope), `2` usage/parse error. FullStory `429`
(monthly server-event quota exceeded) maps to exit `1` with the quota reason
surfaced verbatim so the teammate can explain the failure. `--json` is the
default machine format; every subcommand is non-interactive (flags only), per
anycli AGENTS.md.

## 4. Credential fields & auth flow

**Lane:** `api_key` (manual token). Exact flow:

1. **Registration model — none.** No developer app, no OAuth client, no
   redirect URI, no review. The account admin generates a key in the FullStory
   UI (Settings → Integrations → API Keys → Create key), choosing a permission
   level. The **key value is shown exactly once** at creation — the user copies
   it and pastes it into Helio's connect UI. This is why L5 is agent-drivable
   (key-entry path, master plan §2) rather than human-in-the-loop OAuth.
2. **Permission levels (hierarchy Standard → Architect → Admin).** Standard
   suffices for send-data + `list sessions`; **Get/List User** and session
   events generally require the key's read access; Architect (Enterprise-only)
   is needed for export/delete — which this tool does not wrap, so a Standard
   or read-capable key is the documented minimum. The tool does not enforce a
   level; a `403`/insufficient-permission from the API surfaces as a normal
   exit-`1` error naming the missing permission.
3. **Token semantics.** The key is **long-lived and non-expiring** (revoked
   only by deletion in-app). No refresh cycle. → L4 seeds `access_token`/
   `api_key` only, omits `refresh_token`/`expires_at`; the token gateway serves
   it directly (the "non-expiring token" class in
   `references/integration-testing.md`).
4. **Injection.** Credential field name `api_key`; anycli injects env
   `FULLSTORY_API_KEY`; the service sends `Authorization: Basic <key>`.

No secret ever lives in the bundle or repo; the user's key enters via the
write-only `POST /connections/credentials` API and is stored in Vault
(`references/provider-yaml.md`).

## 5. Helio provider bundle plan

Directory `integrations/providers/fullstory/provider.yaml`, `key: fullstory`,
`go_name: FullStory`. **`presentation.visible: false`** initially (hidden-first;
flip is the single go-live change after L5). Axes all coincide
(`fullstory`/`fullstory`/`fullstory`) → **no `toolToProvider` entry needed**,
no grouped-family `tool.command`.

Shape (manual-token / api_key bundle, following the reviewed metadata pattern —
fixed verification header, HTTPS identity endpoint, setup URL; user token never
in the bundle):

```yaml
schema: helio.provider/v1
key: fullstory
go_name: FullStory
presentation:
  name: FullStory
  description_key: fullstory
  consent_domain: fullstory.com
  visible: false
  order: <batch-assigned>
auth:
  type: api_key
  owner: individual
  required_config_fields: []          # user-supplied key only; no Helio-side client id/secret
  manual_credentials:
    fields: [api_key]                  # single reviewed field, entered in connect UI
    setup_url: https://help.fullstory.com/hc/en-us/articles/360052021773-Managing-API-Keys
identity:
  source: userinfo
  url: https://api.fullstory.com/me    # v1 "test your key" endpoint
  # verification: presence of role proves the key is valid + its permission level
  verify_header: { name: Authorization, scheme: basic_raw }  # "Basic " + raw key, NO base64
  stable_key: <finalize at L2 from the real /me body>        # candidate /uid or /email
  label_candidates: [/email, /uid]
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: standard_api_key   # or the repo's api_key equivalent; no OAuth exchanger
credential:
  fields:
    api_key: credential.api_key
    account_key: connection.account_key
tool:
  name: fullstory
  kind: api_key
```

(Field names above are indicative — reconcile against the current
`provider_catalog` model and a sibling api_key bundle merged on main, e.g.
semrush/dataforseo/freshdesk, at implementation time; the master plan's
batch-end merge owns the five projections.)

**Capability decision — the one thing to verify before coding.** FullStory's
`Authorization: Basic <raw-key>` header is the divergence from every existing
verifier. The manual-token identity verifier must send the key **verbatim**
after `Basic ` with no base64 and no synthesized `login:password`. Before
implementation, confirm the current integration-service manual-token/api_key
verifier can express this:
- If the verifier already supports a per-provider header name + scheme keyword
  with the token passed through unencoded, set `scheme: Bearer`→`Basic` and
  done (no service code).
- If the verifier hard-codes `Bearer`, or only offers a base64
  `basic_userpass` variant (dataforseo/freshdesk precedent), grow the reviewed
  scheme enum by **one** value — `basic_raw` (emit `Basic ` + token, no
  encoding) — rather than writing a compiled `service/adapter_fullstory.go`.
  This is a small closed-enum growth in the generic verifier, not a
  provider-specific adapter (per `provider-yaml.md`'s guidance to prefer
  growing the reviewed capability set over an adapter).

**Identity `/me` caveat (honest gap).** FullStory's public docs confirm `/me`
returns a `role` field (USER/ARCHITECT/ADMIN) and describe it as the key-test
endpoint, but do not fully publish the response body. The exact JSON-pointer
`stable_key` (`/uid` vs `/email` vs an org identifier) must be **captured from
the real `/me` response during L2** and finalized then; the bundle above lists
candidates. If `/me` yields no stable account-scoped identifier, fall back to a
constant account key (single connection per assistant), matching how other
account-level api_key providers key their connection.

Icon: `ui/helio-app/src/integrations/icons/fullstory.svg` + manual
`providerIcons.ts` registration (never generated). AI-facing sub-doc under
`agents/plugins/heliox/skills/tool/` documenting the session-investigation
flow. No integration-service config/secret appends (user-supplied key, so
`required_config_fields: []` → provider renders `configured: true` with no
Helio-side credentials to land).

## 6. Test plan — five layers

| Layer | What it proves for FullStory | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | `internal/tools/fullstory/` unit tests against an httptest fake: asserts `Authorization: Basic <key>` header is emitted **raw (no base64)**, correct paths (`/sessions/v2`, `/v2/sessions/{id}/events`, `/v2/users`, `/v2/events`, `/me`), query-param encoding (`uid`/`email`/`limit`), `{"results":[...]}` vs object output, and both plain + `--json` error rendering incl. `429` quota mapping. | No — fakes only |
| **L2** dev harness vs REAL api.fullstory.com | `ANYCLI_CRED_API_KEY=<real key> anycli fullstory -- me` then `session list --email <known>` / `session events` / `user get`. **Mandatory** — proves the raw-`Basic` header, field name, and paths match the live API, and **captures the real `/me` body to finalize the bundle `stable_key`** (§5). | **Yes** — a real FullStory API key from the account pool (test account with at least one captured user/session) |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle strict-decodes; five projections regenerate; helio-cli builds against the anycli branch via local `replace`; `toolToProvider` unchanged (no divergence). Branch is *expected* to fail `provider-gen --check` in CI until the batch-end merge — validate locally only. | No |
| **L4** singleton + seed endpoint | `POST /internal/test-only/connections/seed` with `provider: fullstory`, `access_token`/`api_key` = real key (no `refresh_token`/`expires_at` — non-expiring class); then `heliox tool fullstory -- session list --email <e>` returns real data through the token gateway. Runs against the hidden provider. | **Yes** — same real key as L2 (seeded); real seeded org/assistant identities in local Mongo |
| **L5** full connect flow (pre-flip) | **api_key key-entry path** (master plan §2, not the OAuth checklist): open connect link → paste key in real connect UI (stored via `POST /connections/credentials`, verified against `/me` with the `basic_raw` header) → connection shows connected/`configured` in `GET /connections` → one **unseeded** `heliox tool fullstory -- me` / `session list` succeeds through the token gateway. Agent-drivable (agent-browser), human fallback on UI breakage. Gates the visible flip. | **Yes** — real key + the real connect UI |

External-credential layers: **L2, L4, L5** need a real FullStory API key from
the account-pool test tenant (a Standard or read-capable key on an account with
≥1 captured user + session). L1 and L3 need no external credentials.

## 7. Rollout

Hidden-first: land bundle `visible: false`, anycli `fullstory` def+service, pin
bump (batch-end), L1–L4 green while hidden, run the L5 key-entry sweep, then
flip `presentation.visible: true` + regenerate as the single go-live change
(SKILL.md stage 10). No review clock (api_key), so the flip trails L5 only, not
any provider-review gate.
