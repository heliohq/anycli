# Tool design: Tally

Scratch design for the `tally` external tool provider (master plan
`008-300-integrations-rollout-plan.md` row 50, Wave 1, Forms & Surveys).
Committed on branch `tool/tally`; the batch lead strips it at batch end.

Naming axes (master plan §3), all identical for Tally — no divergence:

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `tally` | bundle `tool.command` (flat, not grouped) |
| ② anycli tool id | `tally` | `definitions/tools/tally.json` |
| ③ provider catalog key | `tally` | `integrations/providers/tally/` |

Because ② == ③, **no `toolToProvider` entry** is added in
`helio-cli/internal/toolcred/resolver.go`. `ProviderFor("tally")` falls through
to identity.

---

## 1. What an AI teammate does with Tally, and the API surface it needs

Tally (tally.so) is a form builder. What a teammate actually does: **read the
responses people submitted to a form**, understand a form's structure, pull
per-form analytics, and (less often) create/update forms and wire webhooks.
"Read submissions" is the load-bearing use case — the AI triages intake,
summarizes survey results, or syncs responses elsewhere.

### Official API (verified against docs, 2026-07-22)

- Base URL: `https://api.tally.so` (HTTPS only; plain HTTP rejected).
- REST + JSON. Bearer-token auth (see §3).
- Rate limit: **100 requests/minute**; docs recommend webhooks over polling.
- OpenAPI: `https://developers.tally.so/api-reference/openapi.json`
  (`servers[0].url = https://api.tally.so`, `securitySchemes.bearerAuth =
  {type: http, scheme: bearer}`).

Endpoints the tool wraps, driven by the use cases above (paths confirmed from
the OpenAPI spec):

| Purpose | Method + path |
|---|---|
| Current user (identity) | `GET /users/me` |
| List forms | `GET /forms` (`page`, `limit`, `workspaceIds[]`) |
| Get a form | `GET /forms/{formId}` |
| List a form's questions | `GET /forms/{formId}/questions` |
| **List submissions** | `GET /forms/{formId}/submissions` (`page`, `limit` default 50, `filter=all\|completed\|partial`, `startDate`, `endDate`, `afterId`) |
| Get one submission | `GET /forms/{formId}/submissions/{submissionId}` |
| Form analytics | `GET /forms/{formId}/analytics/{metrics\|visits\|submissions\|drop-off\|dimensions}` |
| List workspaces | `GET /workspaces` |
| Create form | `POST /forms` |
| Update form | `PATCH /forms/{formId}` |
| Delete form | `DELETE /forms/{formId}` |
| Webhooks CRUD | `GET/POST /webhooks`, `PATCH/DELETE /webhooks/{webhookId}` |

**Why this subset.** Reads (`forms`, `questions`, `submissions`, `analytics`,
`users/me`, `workspaces`) are the teammate's bread and butter and ship in the
first cut. Writes (`POST/PATCH/DELETE /forms`, webhooks) are secondary but cheap
to add on the same client and give the AI form-authoring + automation-wiring
ability. Deliberately **out of first scope** (low teammate value, add later if
asked): form `blocks` PATCH (raw block-tree editing is brittle for an AI),
org invites/users management, folders CRUD, webhook-event retry. These stay
unimplemented rather than half-supported.

---

## 2. anycli definition

### Type: `service` (per skill stage-1 rubric)

Tally has **no official CLI binary** to wrap, so the `cli`-type criteria fail
outright. Implement a `service`-type tool in `internal/tools/tally/` against the
REST API — the default and the case for 21 of 23 existing definitions.

### `definitions/tools/tally.json`

```json
{
  "name": "tally",
  "type": "service",
  "description": "Tally forms and submissions (personal API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "TALLY_API_KEY"}
      }
    ]
  }
}
```

The resolver-supplied `access_token` field carries the **raw** Tally key
(`tly-…`); the service adds the `Bearer ` scheme when building the request
header (`Authorization: Bearer <TALLY_API_KEY>`). Storing the raw key (not
`Bearer tly-…`) keeps the stored secret provider-neutral and matches the
outbound projection (`slack.json`/`bitly.json` single-`access_token` shape).

### Service implementation (`internal/tools/tally/`)

Go package name `tally` (id has no dashes/leading digit, so no normalization).
Registered in `internal/tools/register.go` `init()` as
`RegisterService("tally", &tally.Service{})`. Copy the shape of
`internal/tools/notion/`: a `Service` struct exposing `BaseURL` / `HC`
(`*http.Client`) / `Out` / `Err` writers so unit tests point `BaseURL` at an
`httptest.Server` and capture stdout/stderr; a cobra tree grouped by resource;
the documented exit-code contract **0** success, **1** runtime/API failure
(typed `apiError`), **2** usage/parse error; and a `--json` structured error
envelope on every command.

Cobra tree (verbs are provider-neutral; each maps to one endpoint above):

```
tally form list            [--workspace <id>] [--page N] [--limit N]
tally form get      --form <id>
tally form questions --form <id>
tally form create   --file <json>|--stdin        # POST /forms body
tally form update   --form <id> --file <json>|--stdin
tally form delete   --form <id>
tally submission list --form <id> [--filter all|completed|partial]
                      [--page N] [--limit N] [--after-id <id>]
                      [--start-date <iso>] [--end-date <iso>]
tally submission get  --form <id> --submission <id>
tally analytics <metrics|visits|submissions|drop-off|dimensions> --form <id>
tally webhook list
tally webhook create  --file <json>|--stdin
tally webhook update  --webhook <id> --file <json>|--stdin
tally webhook delete  --webhook <id>
tally workspace list
tally me                                          # GET /users/me
```

**JSON output shape.** Each read command prints the Tally response body
verbatim to stdout as a single JSON document (Tally already returns clean JSON:
`{ "items": [...], "page", "limit", "total", "hasMore" }` for list endpoints,
an object for single-resource GETs). No re-wrapping — the AI parses provider
JSON directly, and pagination fields (`page`/`limit`/`hasMore`) pass through so
the AI drives its own paging via `--page`. Errors render as
`{"error":{"tool":"tally","code":"...","message":"...","status":<http>}}`
(mirrors notion's envelope). `list` commands never auto-follow pagination —
one page per invocation, bounded output.

---

## 3. Credentials and auth flow (`api_key` lane — verified)

**Audit verdict confirmed.** oauth-audit.md row 50: *"no viable multi-tenant
path → api_key"*. Independent check of the official docs agrees: Tally's
developer surface for third parties is a **personal API key** only. OAuth
exists for a few partner connect-platforms (e.g. Pipedream) but the developer
docs expose **no self-serve OAuth app registration** for arbitrary developers —
there is no authorize/token/client-registration flow a shared Helio client
could ride. So `api_key` is correct; **record no divergence from the catalog.**

### Key semantics (from `developers.tally.so/api-reference/api-keys`)

- Created self-serve at **Settings → API keys** (`tally.so/settings/api-keys`)
  → "Create API key"; shown once ("You won't be able to see it again").
- Prefix `tly-…`; sent as `Authorization: Bearer tly-…`.
- **User-scoped**, not workspace-scoped: a key is tied to the creating user and
  inherits that user's permissions across their workspaces. No expiry
  documented, but a key **stops working if the user leaves/is removed** from the
  org. (No fine-grained scopes today; noted as future.)

### Helio connect path (manual token, verify-first)

Auth type `api_key`, owner `individual`. The user pastes their `tly-…` key
through the write-only `POST /connections/credentials` connect UI; it is stored
in Vault. Because Tally exposes a clean identity endpoint, use **verify-first**
(`runtime_strategy: manual_api_token`) rather than the no-verify DSN path
(`manual_credentials`, mongodb's shape):

- `identity.source: userinfo`, `identity.url: https://api.tally.so/users/me`.
- `identity.stable_key` and `label_candidates` are RFC 6901 JSON pointers into
  the `/users/me` body. The OpenAPI 200 schema is loosely typed (only
  `subscriptionPlan` is declared), so **the exact pointers are confirmed at L2**
  against the live body; expected: `stable_key: /id`, `label_candidates:
  [/email, /fullName, /id]`. The account label is then the user's email — a
  human-readable connection label, far better UX than mongodb's DSN-host.

### The one capability question: Bearer scheme on the verifier

`service/manual_token_verifier.go` `declarativeManualTokenVerifier` sets
`req.Header.Set(definition.APIKey.Header, token)` — the **raw** token as the
whole header value. `model.APIKeyPolicy` today carries only `{Header, SetupURL}`
(no scheme). With `Header: Authorization` that yields `Authorization: tly-…`,
which Tally **rejects** — it requires the `Bearer ` scheme. So plain
`declarativeManualTokenVerifier` cannot verify a Bearer-scheme key as-is.

**Recommended (Option A): a small reviewed capability** — add an optional
`scheme` enum (`none` default | `bearer`) to `apiKeyManifest` /
`model.APIKeyPolicy`, and have `declarativeManualTokenVerifier` send
`Authorization: Bearer <token>` when `scheme: bearer`. Orthogonal ("which
header" vs "which scheme"), a closed enum (no free expression, satisfies the
integration-service "no arbitrary provider scripting" rule), and reused by every
future Bearer-header `api_key` tool. The stored secret and outbound projection
stay the raw `tly-…` — only the inbound verify request gains the prefix.

> **Shared-surface note (master plan §2).** This is the exact "verifier
> capability" growth several sibling Bearer `api_key` tools already needed
> (fullstory/moz/semrush/instantly per the batch task log). On this branch
> (`APIKeyPolicy` = `{Header, SetupURL}` only), the scheme field is **not**
> present. The Tally implementer MUST first check `main` for an already-landed
> `scheme`/`bearer` capability and **reuse it** rather than re-adding a second
> one — capability growth is a shared surface owned at the batch-end merge.

**Fallback (Option B): no-verify.** Ship `runtime_strategy: manual_credentials`
+ `identity.source: strategy` (mongodb's path): store the key without a
provider round-trip; a bad key surfaces at first `heliox tool tally` call via
anycli's `CredentialRejected`. This needs zero integration-service change but
loses connect-time validation and the email account label (account key would be
a synthetic/strategy value). Only take this if Option A's capability is
contested at review — `/users/me` makes verify-first clearly better.

Outbound (both options): `credential.fields` project `access_token:
token.access_token` and `account_key: connection.account_key` — the existing
non-expiring-PAT token-gateway branch, zero new `CredentialSource`.

---

## 4. Helio provider bundle plan (`integrations/providers/tally/provider.yaml`)

Hidden-first (`presentation.visible: false`). Sketch (Option A shape):

```yaml
schema: helio.provider/v1
key: tally
go_name: Tally

presentation:
  name: Tally
  description_key: tally
  consent_domain: tally.so
  visible: false            # flip only after L5; anycli pin must ship `tally`

auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    scheme: bearer          # Option A capability (see §3); omit if using Option B
    setup_url: https://tally.so/settings/api-keys

identity:
  source: userinfo
  url: https://api.tally.so/users/me
  stable_key: /id           # confirm exact pointers at L2 against live body
  label_candidates: [/email, /fullName, /id]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: tally
  kind: api-key
```

Other batch-end shared-surface touches for Tally:

- **Generation** (`go-services/integration-service`): `go run ./cmd/provider-gen`
  + `--check`; the five projections commit together at batch end (never on the
  tool branch — expected red `--check` on-branch until then, per master plan §2).
- **Service side**: `api_key`/`manual_api_token` needs **no** Helio client
  id/secret and **no** `config/`+`deploy/` append (the whole point of the
  key-entry lane) — the only possible service change is the §3 Option-A scheme
  capability, if not already on main.
- **UI icon**: `ui/helio-app/src/integrations/icons/tally.svg` + register in
  `providerIcons.ts` (manual, never generated).
- **i18n**: `tools.desc.tally` (+ any `label_key`) across locales before flip.
- **AI-facing docs**: provider sub-doc under
  `agents/plugins/heliox/skills/tool/`, plugin version bump + marketplace
  publish (one per batch).

---

## 5. Test plan → the five layers

| Layer | Tally specifics | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: `httptest.Server` fakes for `/forms`, `/forms/{id}/submissions` (assert `filter`/`page`/`limit`/`after-id` query params + `Authorization: Bearer <key>` header), `/users/me`, error rendering (plain + `--json`), exit codes 0/1/2. Never hit the live API. | No (fakes) |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=tly-… anycli tally -- form list` and `… submission list --form <id>` against real Tally; **confirm `/users/me` body shape here** to lock `stable_key`/`label_candidates`. Mandatory before pin bump. | **Yes** — a real Tally key + a form with submissions (account pool) |
| **L3** generate + suites | `provider-gen --check` green with the bundle (+ scheme capability if Option A); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; `make test-integration-service` (covers the manual-token verify path + any new scheme unit test). | No |
| **L4** singleton + seed | `manual_api_token` is a user-token provider → seedable. `POST /internal/test-only/connections/seed` with `provider":"tally"`, real `access_token` from the pool (non-expiring key → seed `access_token` only, omit refresh/expiry), real seeded org/assistant/owner ids; then `heliox tool tally -- form list` returns live data. Requires the local `go.mod` `replace` → this anycli branch so heliox embeds `tally`. | **Yes** — real key (L4 success = seeded token reaches live API) |
| **L5** connect flow | **api_key key-entry path** (master plan §2, not the OAuth checklist): open the connect link → paste the `tly-…` key through the real connect UI (`POST /connections/credentials`, verified against `/users/me`) → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool tally` run through the token gateway succeeds. Agent-drivable (agent-browser) with human fallback. Run once, hidden, before the visible flip. | **Yes** — real key + connect UI |

Credential-dependent layers: **L2, L4, L5** need a real Tally API key from the
test-account pool (free tier is sufficient — the API is free to all Tally
users). L1 and L3 are hermetic. No OAuth app registration is needed at any
layer (api_key lane), so there is **no lane-1 dev-app dependency** — only an
account-pool key.

### Rollout

Land hidden → anycli tag + `helio-cli` pin bump → batch-end `provider-gen` →
L1–L4 green → L5 key-entry sweep → flip `presentation.visible: true` +
regenerate as the single go-live change (skill stage 10).

---

## 6. Divergences from prompt / catalog recorded

- **None on the auth lane.** Catalog row 50 and oauth-audit row 50 both say
  `api_key`; official docs confirm (no self-serve OAuth). Kept `api_key`.
- **One flagged capability dependency**, not a catalog divergence: verify-first
  against `/users/me` requires a Bearer scheme on the manual-token verifier
  (§3 Option A). Reuse the sibling-added capability on main if present; else add
  it as a reviewed closed-enum field. Fallback to no-verify (Option B) exists.
