# Tool design — Missive

Scratch design for the `missive` external tool provider, batch-lead strips at batch-end.

- **anycli id (axis ②):** `missive`
- **provider catalog key (axis ③):** `missive`
- **CLI command word (axis ①):** `missive`
- **auth lane (catalog):** `api_key`
- **wave:** 2 · **category:** Support
- **Branches:** anycli `tool/missive`, Helio `tool/missive`

All three naming axes are identical (`missive` = `missive` = `missive`): no `toolToProvider`
divergence entry, no grouped-family `tool.group`. Go package name `missive` (no dashes, no
leading digit — no normalization needed).

---

## 1. What Missive is, and what an AI teammate does with it

Missive is a collaborative **shared-inbox** client: a team works email, SMS, WhatsApp, and social
channels out of shared accounts, with internal comments/mentions threaded next to customer
messages and co-written drafts. It sits in the plan's **Support** category alongside Front,
Help Scout, Gorgias, and Kustomer — the shared-inbox lane.

An AI teammate embedded in a Missive workspace does the same things a human teammate does in a
shared inbox, which drives the endpoint selection in §2:

1. **Triage the shared inbox** — list conversations in a mailbox (inbox / assigned / closed /
   a team inbox / a shared label), read the latest thread state.
2. **Read a thread** — pull the messages, internal comments, and posts on one conversation to
   understand context before acting.
3. **Leave an internal note** — inject a **post** (a comment/annotation) into a conversation,
   the API's headline write ("inject data in any Missive conversation"), e.g. drop a summary,
   flag a lead, or notify the human team.
4. **Reply to the customer** — create a **draft** and optionally send it (email or SMS) as a
   reply on an existing conversation or a new one.
5. **Change conversation state** — close, (un)assign, add/remove shared labels via a
   conversation PATCH.
6. **Manage contacts/CRM sync** — list/get/create/update contacts within a contact book (the
   documented "sync contacts between Missive and your CRM" use case).

Reporting/analytics (`POST /v1/analytics/reports`) is out of scope for v1 — it is a
create-report-then-poll async flow (reports expire 60s after completion) that adds a stateful
retry loop with little AI-teammate value versus the inbox verbs above. Revisit if demand appears.

## 2. Official API surface wrapped, and why

**Base URL:** `https://public.missiveapp.com/v1` · **Format:** JSON in/out (POST requires
`Content-Type: application/json`) · success = HTTP 200/201 (201 may have an empty body).
Docs: <https://missiveapp.com/docs/developers/rest-api> and
`.../rest-api/endpoints`, `.../rest-api/rate-limits`.

Endpoints wrapped, mapped to the §1 jobs (all verified against the official endpoints page):

| Job | anycli verb (proposed) | Method + path | Key params |
|---|---|---|---|
| Triage inbox | `conversations list` | `GET /v1/conversations` | **one mailbox filter required** (`inbox`/`all`/`assigned`/`closed`/`flagged`/`shared_label`/`team_inbox`/…); `limit`≤50, `until` (cursor = `last_activity_at`); `email`/`domain`/`contact_organization` filters (mutually exclusive → 400 if combined) |
| Read conversation | `conversations get` | `GET /v1/conversations/:id` | — |
| Read thread | `conversations messages` / `comments` / `posts` | `GET /v1/conversations/:id/{messages,comments,posts}` | `limit`≤10, `until` (cursor by `delivered_at`/`created_at`) |
| Leave internal note | `posts create` | `POST /v1/posts` | body `posts{ conversation \| references, text/markdown, notification, … }` |
| Reply / send | `drafts create` | `POST /v1/drafts` | body `drafts{ ..., conversation \| references, send:true }` |
| Change state | `conversations update` | `PATCH /v1/conversations/:id` (comma-sep ids for bulk) | close/assign/label mutations |
| List contact books | `contact-books list` | `GET /v1/contact_books` | `limit`≤200, `offset` |
| List/search contacts | `contacts list` | `GET /v1/contacts` | **`contact_book` required**; `limit`≤200, `offset`, `search`, `modified_since`, `order`, `include_deleted` |
| Get contact | `contacts get` | `GET /v1/contacts/:id` | — |
| Create/update contact | `contacts create` / `contacts update` | `POST /v1/contacts` · `PATCH /v1/contacts/:id` | — |
| (optional v1.1) List teams | `teams list` | `GET /v1/teams` | `organization`, `limit`, `offset` |
| (optional v1.1) List canned responses | `responses list` | `GET /v1/responses` | — |

**Why these and not more:** the six jobs above are the complete shared-inbox teammate loop
(read → note → reply → state → contacts). `messages` `POST /v1/messages` is deliberately
excluded — it only creates messages in a **custom channel** (a channel-integration feature, not
general messaging) and does not fit the teammate model. `teams`/`responses` are cheap read-only
adds pencilled for v1.1 once the core loop is validated.

**Pagination is not uniform** — the service layer must encode two shapes: contacts/contact-books
use `limit`+`offset`; conversations and their sub-resources use `limit`+`until` (a timestamp
cursor, and a page may return *more* than `limit`, so the tool must page on the returned cursor,
not on count). Verbs expose `--limit`/`--offset`/`--until` explicitly; no hidden auto-paging in
v1 (agent decides when to fetch more).

**Rate limits (must be handled):** 5 req/s and 5 concurrent, ~900 req / 15 min; `429` returns
`Retry-After` + `X-RateLimit-*`. The service honors `Retry-After` on `429` with a single bounded
retry (mirroring existing service-type tools) and surfaces the limit in the error otherwise —
never a silent spin.

## 3. anycli definition

**Tool form = `service` type** (stage-1 rubric): there is no official Missive CLI binary, so the
`cli` arm (like `github`→`gh`) does not apply. Implement `internal/tools/missive/` against the
REST API, matching the 21-of-23 service-type precedent (notion/bitly/slack/…).

`definitions/tools/missive.json` (mirrors the notion/bitly shape — single bearer secret injected
as an env var; the service builds the `Authorization: Bearer` header itself):

```json
{
  "name": "missive",
  "type": "service",
  "description": "Missive collaborative shared inbox as a tool (personal API token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "MISSIVE_TOKEN"}
      }
    ]
  }
}
```

**Service implementation (`internal/tools/missive/`), key points:**
- Reads `MISSIVE_TOKEN` from env, sends `Authorization: Bearer <token>` + `Accept:
  application/json` on every request; `Content-Type: application/json` on POST/PATCH. The
  `Bearer ` scheme is composed **inside the service** — the injected env var is the bare token
  (this is why the Helio-side header handling in §4 matters only for connect-time verification,
  not for the runtime data path).
- Base URL constant `https://public.missiveapp.com/v1`, overridable via an unexported test seam
  (httptest) for L1.
- Subcommand tree grouped by resource: `conversations {list,get,messages,comments,posts,update}`,
  `posts create`, `drafts create`, `contacts {list,get,create,update}`, `contact-books list`.
- Non-interactive, flag-driven (AGENTS.md: no prompts). Bodies for create/update accept a
  `--json` inline payload or `--file -` stdin, following the existing service tools' write verbs.
- **JSON output shape:** provider-neutral pass-through of Missive's JSON. List verbs emit
  `{ "items": [...], "next_until"|"next_offset": <cursor-or-null> }` so the agent has an explicit
  paging handle; single-object verbs emit the resource object verbatim; write verbs (posts/drafts
  create) emit the created resource, or `{ "ok": true }` when Missive returns 201-no-body.
  Errors emit `{ "error": { "status": <code>, "message": ... , "retry_after": <sec?> } }` on
  non-2xx and exit non-zero.

## 4. Credential model & auth flow (api_key lane)

**Verified against official docs (divergence from a naive read, recorded here):**
- Missive auth is a **personal API token** (Bearer), *not* OAuth. Generate under Preferences →
  API → "Create a new token"; the org must be on the **Productive plan**. Tokens are prefixed
  `missive_pat-`. **This confirms the catalog `api_key` lane and the OAuth-audit verdict for
  row 78 ("no viable multi-tenant path → api_key")** — there is no authorization-code app, no
  client id/secret, no review gate. No divergence from the audit.
- **Tokens are strictly personal** — "There is no organization-level or shared-account-specific
  token." A personal token can *reach* every shared account the user can access; scoping to a
  shared account is done per-request via `account`/`mailbox` params, **not** by a different token.
  Consequence: the Helio connection is `owner: individual`, and there is **no whoami/`/me`
  endpoint** — the closest identity surface, `GET /v1/users`, returns a *list* of org users with
  no marker for the token owner. This is the load-bearing constraint for §4's bundle shape.
- Credential fields: exactly one secret, the PAT. No secret pair, no instance URL, no refresh.

**Helio provider bundle (`integrations/providers/missive/provider.yaml`) — recommended shape.**

The api_key lane maps to two possible bundle strategies in integration-service, and Missive's
missing whoami decides between them:

- `auth.type: api_key` → `runtime_strategy: manual_api_token`: the `declarativeManualTokenVerifier`
  GETs a declared `identity.url` with header `APIKey.Header: <token>` and **requires** a non-empty
  `stable_key` string extracted from a **single JSON object**. Two blockers for Missive:
  (a) the verifier sets the header to the **raw token** with no scheme — Missive needs
  `Authorization: Bearer <token>`, and `APIKeyPolicy` has only `Header`, no scheme/prefix field;
  (b) there is no single-object identity endpoint to extract a `stable_key` from (`GET /v1/users`
  is a list, order-unstable, and never identifies the token owner). So `manual_api_token` does
  **not** fit as the capability stands.

- `auth.type: credentials` → `runtime_strategy: manual_credentials` (the mongodb / design-317
  pattern): **no** provider-side verification at connect, a schema-driven credential form, and a
  strategy-derived account key/label. This sidesteps *both* blockers — no Bearer-scheme header is
  sent at connect, no `stable_key` extraction is attempted. A bad token surfaces at first use via
  AnyCLI's `CredentialRejected` → stale-feedback loop, exactly as for mongodb.

**Recommendation: Option A — `manual_credentials` (no-verify), mirroring mongodb.** It reuses an
already-shipped strategy wholesale and is the honest fit for "single opaque secret, no whoami."
Proposed bundle:

```yaml
schema: helio.provider/v1
key: missive
go_name: Missive

presentation:
  name: Missive
  description_key: missive
  consent_domain: missiveapp.com
  visible: false            # hidden-first; flip is the single go-live change

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_token
        label_key: missive_api_token
        secret: true
        placeholder: "missive_pat-..."
        required: true
    setup_url: https://missiveapp.com/help/api-documentation/getting-started

identity:
  source: strategy          # no whoami; account key/label from a strategy, never a hash

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token   # PAT stored via the existing user-token write path
    account_key: connection.account_key

tool:
  name: missive
  kind: api-key             # wire-compat kind; drawer routed by auth_type
```

**Capability gap to flag for the batch lead (the one real decision):** `manual_credentials`
today binds `dsnHostIdentityDeriver`, which parses a **connection-string host** out of the
secret. Missive's PAT is an opaque token with no parseable human-readable component, so that
deriver produces nothing usable. Two clean resolutions, in preference order:
  1. **Add a generic single-secret identity strategy** (small, reviewed capability): account
     key/label comes from a user-supplied non-secret `account_label` field in `credential_input`
     (e.g. "Support inbox"), not from the secret — human-readable, never a hash, and it never
     leaks the token. This is the minimal orthogonal add and generalizes to every future opaque
     api_key provider with no whoami.
  2. **Option B (verify-first) — grow `manual_api_token`** with (i) an optional `scheme: Bearer`
     on `APIKeyPolicy` (used by the verifier header build *and* nothing else — the runtime data
     path is anycli-side) and (ii) a "verify-without-stable-identity" mode that accepts a 200
     from a probe endpoint (e.g. `GET /v1/contact_books`) as validity proof while falling the
     account key/label back to a strategy. This buys connect-time rejection of bad tokens at the
     cost of **two** capability edits and still needs the same strategy for identity — strictly
     more surface than Option A for marginal UX. Recommend only if connect-time verification is
     deemed required.

Sibling api_key tools in this program that needed capability growth for non-standard auth shapes
(`semrush`/`dataforseo`/`serpapi` "verifier"/"in:query" capabilities) are the precedent that this
kind of small reviewed capability add is the sanctioned path — not a per-tool adapter.

**No integration-service Go adapter is needed** either way — the runtime data path is pure
AnyCLI passthrough (token → env → `Authorization: Bearer`), and the connect path uses an existing
(mongodb) or minimally-extended generic strategy. No client id/secret means **no `config/` +
`deploy/` secret append** (human lane 1's landing step is a no-op for Missive) — it renders
`configured: true` with zero server config, safe to ship hidden immediately.

## 5. Test plan — five layers

| Layer | Scope for Missive | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `internal/tools/missive/` table tests with `httptest` fakes: header build (`Authorization: Bearer`, `Content-Type`/`Accept`), both pagination shapes (`offset` for contacts, `until` cursor for conversations incl. the "page may exceed limit" case), the mandatory-filter errors (conversations without a mailbox filter → surfaced 400; contacts without `contact_book`), `429`+`Retry-After` single-retry, 201-empty-body → `{ok:true}`, and JSON output envelopes. TDD: write these first. | **No** — fakes only |
| **L2** harness real-API | `anycli missive -- conversations list --inbox --limit 5`, `contact-books list`, `posts create` (into a scratch conversation), against the **real** API with `ANYCLI_CRED_missive_access_token=missive_pat-…`. Confirms base URL, Bearer scheme, real pagination cursors, and rate-limit headers. | **Yes** — a real `missive_pat-` token from a Productive-plan org (test-account pool, human lane 2) |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` green with the bundle; `cd go-services/integration-service && go test ./...` and helio-cli `go test ./cmd/heliox/cmds/tool/` green. If Option A's identity-strategy capability is added, its unit test lands here. Run locally on-branch; **do not** commit the regen (batch-lead owns the one canonical regen). | No |
| **L4** singleton + seeded creds | `make run-singleton`; `POST /internal/test-only/connections/seed` a Missive connection with the real PAT; `heliox tool missive -- conversations list --inbox` (built via `helio-cli/go.mod` local `replace` → this anycli branch). Success = the seeded token reaches `public.missiveapp.com` and returns real data through the token gateway. | **Yes** — same real token as L2 |
| **L5** full connect flow | **api_key key-entry path** (agent-drivable per master plan §2, human fallback): open the connect link → paste the `missive_pat-` token through the real connect UI → stored via write-only `POST /connections/credentials` → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool missive -- conversations list` through the real token gateway succeeds. This is the completion signal; runs once before the visible flip. Follow the key-entry L5 checklist (master plan §2 upstreamed into `references/integration-testing.md`). | **Yes** — real token + the real connect UI |

**Externally-supplied-credential layers: L2, L4, L5** (all need one real `missive_pat-` token from
a Productive-plan Missive org — the single test-account procurement for this tool). L1 and L3 are
fully hermetic.

## 6. Rollout notes

- Ship hidden (`visible: false`); code-complete = L1–L4 green while hidden. Flip `visible: true` +
  regenerate as the single go-live change after L5 passes (no review gate — api_key lane).
- UI icon `ui/helio-app/src/integrations/icons/missive.svg` + `providerIcons.ts` registration
  (manual, not generated); AI-facing sub-doc under `agents/plugins/heliox/skills/tool/`.
- Open decision for the batch lead before Helio-side dev: **Option A identity strategy vs Option B
  verify-first** (§4). Recommendation: Option A (add the generic single-secret label strategy).
