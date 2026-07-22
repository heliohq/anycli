# Postmark — per-tool design (`heliox tool postmark`)

Batch scratch design for the Postmark provider, per the
`helio-tool-provider` pipeline and the 298-integrations master plan
(`docs/design/008-300-integrations-rollout-plan.md`). Catalog row 53:
`api_key` · Wave 1 · Email & Messaging. Axes ② `postmark` / ③ `postmark`.

This file is committed on branch `tool/postmark` as a working artifact; the
batch lead strips it at batch-end.

---

## 0. Audit verdict — verified against official docs (independent judgment)

The master-plan catalog and the OAuth audit both put Postmark in the
**`api_key`** lane ("no viable multi-tenant path per rubric",
`oauth-audit.md` row 53). I verified this directly against Postmark's official
API reference rather than inheriting it:

- Postmark's API overview (https://postmarkapp.com/developer/api/overview)
  states authentication is **token-only**: every request carries an HTTP
  header holding an API token; missing/invalid returns `401`. **There is no
  OAuth authorization-code flow of any kind** — no authorize endpoint, no
  client registration, no consent screen. A missing multi-tenant
  authorization-code flow is exactly the rubric's condition for staying
  `api_key`.
- Two token types exist, both plain secrets pasted by the account holder:
  **Server API Token** (`X-Postmark-Server-Token`, scoped to one server) and
  **Account API Token** (`X-Postmark-Account-Token`, account-wide). Neither is
  minted through an OAuth exchange.

**Verdict: `api_key` confirmed. No divergence from the audit.** Recorded here
per the "check, don't inherit" constraint; no DESIGN divergence note is owed
to the master plan.

---

## 1. What the tool wraps and why (driven by AI-teammate use)

Postmark is a **transactional + broadcast email delivery** provider. What an
AI teammate actually does with it, in priority order:

1. **Send email** on behalf of the user's confirmed sender signature —
   transactional ("here's your report", "your task is done") and templated.
2. **Look up what was sent / received** — search the outbound activity stream
   (did the message deliver? to whom? what's the status?), inspect a specific
   message's details, and read inbound messages routed to a Postmark inbound
   address.
3. **Diagnose deliverability** — read bounces and delivery stats when a send
   didn't land, and reactivate a bounced/deactivated recipient.
4. **Discover templates** the account already has, so a send can reference an
   existing `TemplateAlias`/`TemplateId` rather than hand-rolling HTML.

Everything above is **Server-level** and is served by the **Server API Token**
(`X-Postmark-Server-Token`). Account-level operations (creating servers,
managing domains/sender-signatures, rotating tokens) use the **Account API
Token** and are **out of scope** for v1: they are administrative setup, not
teammate work, and mixing a second token kind into one connection would break
the single-secret manual-credential contract (design 317). If sender-signature
/ domain management is wanted later it is a **separate `postmark-account`
provider** with its own token — not an axis on this one.

Base URL for all endpoints: `https://api.postmarkapp.com`. All requests set
`Accept: application/json`; write requests set `Content-Type: application/json`
(Postmark returns HTTP `415` Unsupported Media Type — surfaced with application
`ErrorCode` `409` "JSON required" — if these are not `application/json`). The
service always sets both headers, so this is non-load-bearing.

### Endpoint surface (all require `X-Postmark-Server-Token`)

| Group | Verb → endpoint | Purpose |
|---|---|---|
| send | `POST /email` | Send a single email |
| send | `POST /email/withTemplate` | Send using a template (`TemplateId`/`TemplateAlias` + `TemplateModel`) |
| send | `POST /email/batch` | Batch send (≤500 msgs / ≤50 MB) — v1.1 |
| send | `POST /email/batchWithTemplates` | Batch templated send — v1.1 |
| messages | `GET /messages/outbound` | Search sent messages (`count`,`offset`,`recipient`,`fromemail`,`tag`,`subject`,`status`,`messagestream`) |
| messages | `GET /messages/outbound/{id}/details` | One outbound message's detail + events |
| messages | `GET /messages/inbound` | Search inbound messages (`count`,`offset`,`recipient`,`fromemail`,`subject`,`status`) |
| messages | `GET /messages/inbound/{id}/details` | One inbound message's detail |
| templates | `GET /templates` | List templates (`count`,`offset`) |
| templates | `GET /templates/{idOrAlias}` | Get one template |
| bounces | `GET /deliverystats` | Delivery / bounce summary |
| bounces | `GET /bounces` | Search bounces (`count`,`offset`,`type`,`inactive`,`emailFilter`,`tag`,`messageID`) |
| bounces | `GET /bounces/{id}` | One bounce |
| bounces | `PUT /bounces/{id}/activate` | Reactivate a deactivated recipient |
| server | `GET /server` | Current server metadata — **also the connect verification/identity endpoint** (§4) |

`GET /server` doubles as the identity probe (§4). It echoes back the server's
`ApiTokens` array, so the service **must not** print the raw `GET /server`
response by default — surface only `ID`, `Name`, `ServerLink`, and safe
metadata (opens/links tracking, delivery type, inbound address).

---

## 2. anycli definition & service (axis ②: `postmark`)

**Type: `service`** (stage-1 rubric). Postmark ships **no official CLI**; the
integration is HTTP against `api.postmarkapp.com`. This is the default and
matches 21/23 existing definitions. `cli` type is not applicable (no binary to
provision).

`definitions/tools/postmark.json`:

```json
{
  "name": "postmark",
  "type": "service",
  "description": "Postmark transactional email: send, message activity, templates, bounces (Server API token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "server_token"},
        "inject": {"type": "env", "env_var": "POSTMARK_SERVER_TOKEN"}
      }
    ]
  }
}
```

- **Field name `server_token`** (not `access_token`) — the anycli field is the
  provider's real credential name, and Postmark's token is literally the
  "Server API Token". The Helio bundle's `credential.fields` maps
  `server_token: token.access_token`, exactly the mongodb precedent
  (`connection_string: token.access_token`). anycli is Helio-agnostic; the
  field name is local to the definition.
- **Injected as env** `POSTMARK_SERVER_TOKEN`; the service reads it and sets
  the `X-Postmark-Server-Token` request header. (anycli has no "custom header"
  inject type — env → header inside the service is the established shape.)

### Service package `internal/tools/postmark/` (Go pkg `postmark`)

Package name is the anycli id with dashes dropped; `postmark` is already a
legal Go identifier — no normalization needed. Copy the **notion** service
shape (the reference impl): a cobra tree grouped by resource, a
`BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest.Server`, and
the documented exit-code contract (`0` success, `1` runtime/API failure via a
typed `apiError`, `2` usage/parse) with a `--json` structured error envelope.

Command tree (verbs are the teammate's mental model, `--json` on every leaf):

```
postmark email send        --from --to [--cc --bcc --subject --html --text \
                                --reply-to --tag --stream --track-opens \
                                --track-links --metadata --header --attachment]
postmark email send-template --from --to (--template-id | --template-alias) \
                                --model <json> [--stream ...]
postmark message list-outbound [--count --offset --recipient --from-email \
                                --tag --subject --status --stream]
postmark message get-outbound  <message-id>
postmark message list-inbound  [--count --offset --recipient --from-email \
                                --subject --status]
postmark message get-inbound   <message-id>
postmark template list         [--count --offset]
postmark template get          <id-or-alias>
postmark bounce list           [--count --offset --type --inactive \
                                --email --tag --message-id]
postmark bounce get            <bounce-id>
postmark bounce activate       <bounce-id>
postmark stats delivery
postmark server get
```

`email batch` / `email send-template` batch variants are a v1.1 add — keep the
v1 surface tight around single-send + activity + diagnostics.

**JSON output shape.** Each leaf prints the provider JSON unwrapped on success
(list endpoints already return `{TotalCount, Messages|Bounces|Templates:[…]}`;
`server get` prints a **redacted** subset, never `ApiTokens`). `--json` on
error emits the standard anycli envelope `{"error":{"code","message",…}}`.

**Postmark error dialect (important).** Postmark returns a JSON body
`{"ErrorCode":<int>,"Message":"…"}` on every response. The single-send vs.
batch contracts differ, and the design keys on the union of both:

- **Single send (`POST /email`, `POST /email/withTemplate` — the v1 surface).**
  Success is HTTP **200** with `ErrorCode 0`. Validation failures (`300`
  invalid email request, `406` inactive recipient, unconfirmed sender
  signature, etc.) come back as HTTP **422** with a non-zero `ErrorCode`; a bad
  token is HTTP **401** (`ErrorCode 10`). The single-send API never returns
  `200` with a non-zero `ErrorCode` — validation errors are HTTP 4xx. (Verified
  against postmarkapp.com/developer/api/overview.)
- **Batch (`POST /email/batch`, `POST /email/batchWithTemplates` — deferred to
  v1.1).** These return HTTP **200** with a **per-message array** where
  individual entries may carry a non-zero `ErrorCode` while the transport status
  stays `200`. This is the only surface where `200` + non-zero `ErrorCode`
  occurs, and it becomes load-bearing once batch lands.

The service therefore keys success on **`ErrorCode == 0`** rather than solely on
HTTP status — a rule that is correct for single send today (200 ⇔ ErrorCode 0)
and stays correct when the batch per-message array arrives in v1.1. Any non-zero
`ErrorCode` maps to exit `1` with the provider `Message` surfaced; `401` (bad
token) and `422`/other `4xx` map to exit `1`; malformed local flags → exit `2`.
Unit tests assert this `ErrorCode`-and-status classification and the injected
`X-Postmark-Server-Token` header against an `httptest` fake.

The test-token value `POSTMARK_API_TEST` (Postmark's documented no-delivery
sandbox token for `POST /email`) is used in L2 send tests to avoid real
delivery while still exercising the live API contract.

---

## 3. Helio provider bundle (axes ①/③ + hidden-first)

Directory `integrations/providers/postmark/provider.yaml`. Axis ①
`postmark`, axis ③ `postmark`, axis ② `postmark` — **all three identical, so
NO `toolToProvider` entry** is added (the resolver map holds only ②↔③
divergences). No grouped family; flat command.

This is a **manual API-token** bundle that DOES have a real HTTPS verification
endpoint (`GET /server`), so it uses `runtime_strategy: manual_api_token` +
`declarativeManualTokenVerifier` — distinct from mongodb's
`manual_credentials` no-verify DSN path. **Zero integration-service capability
growth** is required: the `manual_api_token` strategy on `main` already GETs
`identity.url` with the bundle-declared `auth.api_key.header`, then extracts a
string stable key + label via JSON pointer
(`service/manual_token_verifier.go` → `declarativeManualTokenVerifier`,
selected in `provider_registry.go` for `RuntimeStrategyManualAPIToken`). This
is precisely the Loops/Tally-class Bearer-verifier shape, except the header is
a custom fixed name — which the capability already parameterizes
(`req.Header.Set(definition.APIKey.Header, token)`).

```yaml
schema: helio.provider/v1
key: postmark
go_name: Postmark

presentation:
  name: Postmark
  description_key: postmark
  consent_domain: postmarkapp.com
  # Hidden-first (stage 4/10). Do NOT flip until: the anycli postmark tool ships
  # in the pinned AnyCLI + heliox rebuild, a reviewed icon lands in helio-app,
  # tools.desc.postmark ships in all locales, and L5 (§5) passes. Pick an
  # unoccupied presentation.order at flip time.
  visible: false

auth:
  type: api_key
  owner: assistant            # the AI teammate's own connection (isolated), like notion
  api_key:
    header: X-Postmark-Server-Token
    setup_url: https://postmarkapp.com/support/article/1008-what-are-the-account-and-server-api-tokens

identity:
  source: userinfo            # GET /server verifies the token & names the server
  url: https://api.postmarkapp.com/server
  stable_key: /ServerLink     # STRING & stable (embeds the immutable server id);
                              # /ID is a JSON number and jsonPointerString is
                              # string-only on main, so ServerLink is the
                              # zero-growth stable key (see §3 note).
  label_candidates: [/Name, /ServerLink]

connection:
  mode: isolated
  disconnect_mode: local_only   # Postmark has no token-revoke API; disconnect is local
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    server_token: token.access_token   # single secret via the existing UpsertUserToken path
    account_key: connection.account_key

tool:
  name: postmark
  kind: api-key
```

**Stable-key choice (verified against the code, not assumed).** `GET /server`
returns `ID` as a JSON **number**; `manual_token_verifier.go` extracts the
stable key via `jsonPointerString`, which returns only `string` values
(`declarative_identity.go`). A numeric `/ID` would fail verification on `main`.
`/ServerLink` is a **string**, is **stable** (it embeds the immutable numeric
server id, e.g. `https://account.postmarkapp.com/servers/<id>/streams`), and is
unique per server — so it is the correct zero-growth stable key. `/Name` is the
human-readable label (server rename keeps `/ServerLink` stable). Deliberately
**not** growing numeric-stable-key coercion: a string key is available, and the
master plan's "subtract before adding" + one-shared-surface-per-batch ethos
favors touching zero integration-service files for a Wave-1 api_key tool.

**Config.** `manual_api_token` uses **no** `required_config_fields` and no
integration-service secrets (validator: `manual_api_token … does not use
server config fields`). The user pastes their own Server API Token through the
write-only `POST /connections/credentials` path into Vault. So **no `config/`
or `deploy/` change** — Postmark has nothing in the seventh shared surface
(OAuth config), unlike the oauth lanes.

**Icon (manual, never generated):**
`ui/helio-app/src/integrations/icons/postmark.svg` + hand-register in
`providerIcons.ts`. **i18n:** `tools.desc.postmark` (description_key
`postmark`) across all locale bundles.

---

## 4. Credential fields & the exact auth/connect flow

- **Credential kind:** one secret, the **Postmark Server API Token** — an
  opaque UUID-style string found under a Postmark server's **API Tokens** tab.
  Account owner / admin / server admin can read it. No scopes, no expiry, no
  refresh: a server token is a static bearer of that server's privileges until
  the user rotates it in the Postmark UI.
- **Connect (L5 key-entry path, no OAuth):**
  1. `heliox tool postmark auth` mints a connect intent → connect link.
  2. User pastes the Server API Token into the connect form (single secret
     field, `secret: true`).
  3. integration-service runs `declarativeManualTokenVerifier`: `GET
     https://api.postmarkapp.com/server` with `X-Postmark-Server-Token: <token>`
     + `Accept: application/json`. `401/403` → `invalid_provider_credential`
     ("the provider rejected this token"); `2xx` → extract `account_key =
     /ServerLink`, `label = /Name`.
  4. Token stored in Vault via `writeUserTokenCredential`; connection row
     upserts by `(org, assistant, provider=postmark, account_key)` and shows
     **connected** in the integrations UI.
  5. One unseeded live command (e.g. `postmark server get`) through the real
     token gateway succeeds → loop closed.
- **Token semantics for L4 seeding:** non-expiring bearer → seed
  `access_token` **only**, omit `refresh_token`/`expires_at` (the
  "non-expiring bot/app token" class in `references/integration-testing.md`).
  The token gateway serves it directly; there is no refresh path to exercise.
- **Multi-server note:** the connection is per **server** (its `ServerLink`
  account key). A user who wants two Postmark servers connects each token
  separately; the isolated-mode connection model handles this natively — no
  special-casing.

---

## 5. Test plan → the five layers

| Layer | Postmark specifics | Needs external creds? |
|---|---|---|
| **L1** anycli unit (`go test ./...`) | `httptest` fake for `api.postmarkapp.com`: assert `X-Postmark-Server-Token` header injection, `Accept`/`Content-Type` set on writes, request shape for `POST /email` + `send-template` + each GET; assert the single-send error contract — a `422` + non-zero `ErrorCode` (e.g. `406` inactive recipient) → exit 1 with `Message` surfaced, `200` + `ErrorCode 0` → exit 0 — plus the `ErrorCode == 0` success key (which also covers the v1.1 batch `200` + per-message non-zero `ErrorCode` case once added) and the `--json` error envelope; assert `server get` redacts `ApiTokens`. | No — fakes only |
| **L2** dev-harness real API (`anycli postmark -- …` + `ANYCLI_CRED_SERVER_TOKEN`) | Run against a **real Postmark server**: `server get`, `message list-outbound`, `template list`, `bounce list`, and a `email send` using the `POSTMARK_API_TEST` sandbox token (no real delivery) + one real send to a controlled inbox. Proves field names + header injection match the live contract. | **Yes** — a real Server API Token (test account pool, lane 2) |
| **L3** `provider-gen --check` + both repos' unit suites | Regenerate the five projections locally (uncommitted); confirm `postmark` bundle strict-decodes, `manual_api_token` needs no config fields, `identity.source: userinfo` + `identity.url` validate, `tool.kind: api-key`. helio-cli builds with a local `replace` → anycli branch. | No |
| **L4** singleton + seed endpoint | `POST /internal/test-only/connections/seed` with `provider: postmark`, `access_token: <real server token>`, a real seeded assistant/org identity; then `heliox tool postmark -- server get` reaches the live API through the token gateway. Seed **access_token only**. | **Yes** — a real Server API Token (same as L2) |
| **L5** full connect flow (once, pre-flip) | **api_key key-entry path** (master plan §2, not OAuth): open connect link → paste the Server API Token in the real UI → connection shows connected/configured (`GET /connections`) → one **unseeded** live `postmark server get` succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. | **Yes** — a real Server API Token + connect UI |

**Externally-supplied-credential layers: L2, L4, L5** (all need one real
Postmark Server API Token from a test-pool account — Postmark offers a free
developer tier with a sandbox server, so procurement is self-serve, no review).
L1 and L3 are fully hermetic.

---

## 6. Stage checklist status & shared-surface impact

- Axis ②↔③ identity → **no `toolcred/resolver.go` entry**.
- **No integration-service Go change** — `manual_api_token` +
  `declarativeManualTokenVerifier` cover it as-is (verified in
  `provider_registry.go` / `manual_token_verifier.go`).
- **No `config/` or `deploy/` change** — manual token, no server-side secrets.
- Shared surfaces this tool touches at batch-end: anycli `register.go`
  (`RegisterService("postmark", …)`), the anycli pin bump, `provider-gen`
  regen (five projections), `providerIcons.ts` append, and the plugin
  version bump + docs publish (`agents/plugins/heliox/skills/tool/`). It does
  **not** touch the resolver map or the OAuth config surface — the two
  lightest shared surfaces for a batch.
- Ships **hidden**; visible flip is the single go-live change after L5.
