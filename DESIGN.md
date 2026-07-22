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

`GET /server` doubles as the identity probe (§4), and its response is a
**secret-bearing body**: the `ApiTokens` array echoes back the server's live
Server API Token value(s) (confirmed against the official `GET /server`
reference). Two distinct sinks must therefore keep `ApiTokens` out of what they
persist/print — and both do so **by construction**, neither needs a new shared
capability:

1. **anycli CLI output** (`postmark server get`) — print only `ID`, `Name`,
   `ServerLink`, and safe metadata (opens/links tracking, delivery type,
   inbound address); never the raw body, never `ApiTokens`. Provider-local to
   the anycli service package.
2. **integration-service identity persistence** (the connect path, §3/§4) — the
   default `declarativeManualTokenVerifier` returns the **entire** parsed
   `/server` body as the `identity` map, and `ManualCredentialService.Connect`
   writes it verbatim to `metadata["identity"]` in the `connections` collection,
   which is **plaintext at rest** (only the credential referenced by
   `credential_id` is KMS-vaulted). For Postmark that map would carry the live
   Server API Token a **second time, in cleartext** — duplicating a KMS-vaulted
   secret into plaintext Mongo, which defeats the point of vaulting it. This
   **at-rest duplication is the load-bearing, verified harm.** It is *not*
   additionally an AI / read-API exposure: `dto.Connection` — the shape returned
   by `GET /connections` and the internal `GET /internal/connections/by-identity`
   — has **no `Metadata` field** (verified in `dto/connection.go`), so
   `metadata["identity"]` is never marshalled to any client. This sink is closed
   in §3 by a **provider-local named verifier that builds the identity map from
   only the safe fields** (`ApiTokens` never enters it), with zero change to the
   shared declarative identity contract.

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

**Postmark error dialect (important).** The `{"ErrorCode":<int>,"Message":"…"}`
envelope is present on **all error responses** (any endpoint) and on **send
responses** (a successful send carries `ErrorCode 0`). Successful **reads**
(`GET /server`, `/templates`, `/messages/outbound`, `/bounces`, …) return their
data object/list directly, with **no `ErrorCode` field at all** — so the
success key below is "HTTP 2xx AND (`ErrorCode` absent OR `== 0`)", which Go's
zero-value default satisfies for the absent case. The single-send vs. batch
contracts differ, and the design keys on the union of both:

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

The service therefore keys success on **HTTP 2xx AND (`ErrorCode` absent OR
`== 0`)** rather than solely on HTTP status — the `absent` arm covers the
successful reads (whose body carries no `ErrorCode`, so Go decodes the field to
its `0` zero value), the `== 0` arm covers a successful send, and the rule stays
correct when the batch per-message array (where a `200` entry may carry a
non-zero `ErrorCode`) arrives in v1.1. Any non-zero `ErrorCode` maps to exit `1`
with the provider `Message` surfaced; `401` (bad token) and `422`/other `4xx`
map to exit `1`; malformed local flags → exit `2`.
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

This is a **manual API-token** bundle with a real HTTPS verification endpoint
(`GET /server`), so `runtime_strategy: manual_api_token` — distinct from
mongodb's `manual_credentials` no-verify DSN path. The header/stable-key/label
mechanics reuse the existing `manual_api_token` shape (GET `identity.url` with
the bundle-declared `auth.api_key.header`
`req.Header.Set(definition.APIKey.Header, token)`, then extract a string stable
key + label via JSON pointer) — the same Loops/Tally-class fixed-header
verifier shape.

**The one integration-service change is a provider-local named verifier — NOT a
shared-contract change.** Postmark is the first `manual_api_token` provider
whose identity endpoint returns a **secret-bearing body**: `GET /server` echoes
the caller's own Server API Token in `ApiTokens` (§1). The default
`declarativeManualTokenVerifier.Verify` returns the **whole** parsed body as the
`identity` map, and `Connect` persists it verbatim to plaintext
`metadata["identity"]` — so for Postmark that map would duplicate the live token
in cleartext. On the postmark branch base the `RuntimeStrategyManualAPIToken`
case hardcodes `declarativeManualTokenVerifier{}` (verified in
`provider_registry.go`), so Postmark does need *some* code to close this — but
the minimal, idiomatic fix is a **provider-local named verifier**, not a new
field on the shared declarative Identity contract:

- Add `postmarkServerVerifier` (implements the existing `manualTokenVerifier`
  interface, `service/manual_token_verifier.go` neighborhood). It GETs the
  bundle-declared `identity.url` (`/server`) with the bundle-declared
  `auth.api_key.header`, treats **2xx as valid** (`401/403` →
  `invalid_provider_credential`), extracts the stable key (`/ServerLink`) and
  label (`/Name`) via the declared JSON pointers, and returns a **freshly built
  identity map containing only those safe values** — `{ServerLink, Name}`. The
  raw body (and therefore `ApiTokens`) is never copied into the map.
- Select it for Postmark in `composeProviderRegistration` — a small
  provider-keyed branch in the `RuntimeStrategyManualAPIToken` case that
  **defaults to `declarativeManualTokenVerifier{}`** for every other
  `manual_api_token` provider. This mirrors exactly how the
  `RuntimeStrategyManualCredentials` case already selects `dsnHostIdentityDeriver{}`
  (mongodb), whose `Verify` likewise returns only a derived safe field
  (`{"host": host}`) and never lets the secret reach `metadata["identity"]`.

**Why the named verifier and not an `identity.redact` field.** An earlier draft
closed the leak by adding a general `identity.redact` denylist to the **shared**
declarative Identity contract (`model/catalog.go` + `cmd/provider-gen`
projection + strict-decode + the five generated projections). That is
unnecessary capability growth for one provider's need, and it is a **denylist** —
the DESIGN itself conceded a denylist is the less-safe model (a future
secret-echoing identity field would leak until someone enumerates it in
`redact`). The named verifier is **allowlist-by-construction**: it emits only
the two fields the framework actually reads, so no secret-bearing field can leak
regardless of what `/server` returns later. It also needs **zero** change to the
shared Identity contract or any of the five generated projections, and it
matches the program's established idiom — the many named `*Verifier`/`*Deriver`
capabilities (`dsnHostIdentityDeriver`, and the semrush/fullstory-class named
verifiers), the integration-service rule *"prefer a compiled generic capability
or a narrow named strategy, never an unbounded YAML expression"*, and the repo's
*"subtract before adding / minimal orthogonal"* discipline. If a general
redaction/allowlist capability is later judged worthwhile program-wide, it
should be escalated as its **own** shared-capability decision (allowlist
preferred — persist only the referenced pointers), not smuggled in via one
tool's bundle.

**Why not a secret-free identity endpoint.** `GET /server` is the **only**
Server-token endpoint that returns a stable per-server identity (`ID` / `Name`
/ `ServerLink`). The other Server-token reads (`/deliverystats`,
`/messages/outbound`, `/templates`, `/bounces`) return activity/summary lists
with **no stable server-identity key** to anchor `account_key` on. Keeping
`/server` as the probe and having `postmarkServerVerifier` build a safe identity
map from it is the minimal orthogonal fix.

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
  owner: individual           # the connecting human owns the pasted secret —
                              # the manual_api_token contract pins owner to
                              # individual (mongodb precedent); per-connection
                              # isolation comes from connection.mode: isolated,
                              # not from owner (see §3).
  api_key:
    header: X-Postmark-Server-Token
    setup_url: https://postmarkapp.com/support/article/1008-what-are-the-account-and-server-api-tokens

identity:
  source: userinfo            # GET /server verifies the token & names the server;
                              # postmarkServerVerifier (§3) reads it and emits a
                              # safe identity map — ServerLink + Name only, never
                              # the raw body. No new bundle field.
  url: https://api.postmarkapp.com/server
  stable_key: /ServerLink     # string, stable (embeds the immutable server id),
                              # unique per server — see the stable-key note (§3).
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

**Stable-key choice.** `/ServerLink` is the correct stable key on its own
merits: it is a **string**, it is **stable** (it embeds the immutable numeric
server id — the documented response shape is
`https://postmarkapp.com/servers/<id>/streams`, so a server rename does not
change it), and it is **unique per server**. `/Name` is the human-readable
label. `GET /server` returns `ID` as a JSON number, but `/ServerLink` is
preferred over `/ID` because it is already a stable *string* — this holds
independent of whether numeric-stable-key coercion (in-flight in sibling work)
has landed, so the choice introduces no stable-key-coercion dependency. The only
integration-service change this tool needs is the provider-local
`postmarkServerVerifier` (above).

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
  3. integration-service runs the provider-local `postmarkServerVerifier` (§3):
     `GET https://api.postmarkapp.com/server` with `X-Postmark-Server-Token:
     <token>` + `Accept: application/json`. `401/403` →
     `invalid_provider_credential` ("the provider rejected this token"); `2xx` →
     extract `account_key = /ServerLink`, `label = /Name`, and return an
     identity map built from **only** those safe fields, so the persisted
     `metadata["identity"]` never carries the live Server API Token (`ApiTokens`
     is never copied into the map — §3).
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
| **L3** `provider-gen --check` + both repos' unit suites | Regenerate the five projections locally (uncommitted); confirm `postmark` bundle strict-decodes, `manual_api_token` needs no config fields, `identity.source: userinfo` + `identity.url` + `stable_key`/`label_candidates` validate (**no new bundle field**), `tool.kind: api-key`. **New integration-service unit tests**: (a) `postmarkServerVerifier.Verify` against a `/server` fake whose body includes `ApiTokens` returns an identity map containing **only** `ServerLink` + `Name` (`ApiTokens` absent **by construction**), proving the persisted `metadata["identity"]` is secret-free; (b) a registry test that Postmark composes `postmarkServerVerifier` while every other `manual_api_token` provider still gets the default `declarativeManualTokenVerifier`. helio-cli builds with a local `replace` → anycli branch. | No |
| **L4** singleton + seed endpoint | `POST /internal/test-only/connections/seed` with `provider: postmark`, `access_token: <real server token>`, a real seeded assistant/org identity; then `heliox tool postmark -- server get` reaches the live API through the token gateway. Seed **access_token only**. | **Yes** — a real Server API Token (same as L2) |
| **L5** full connect flow (once, pre-flip) | **api_key key-entry path** (master plan §2, not OAuth): open connect link → paste the Server API Token in the real UI → connection shows connected/configured (`GET /connections`) → one **unseeded** live `postmark server get` succeeds. Agent-drivable (agent-browser), human fallback on UI breakage. | **Yes** — a real Server API Token + connect UI |

**Externally-supplied-credential layers: L2, L4, L5** (all need one real
Postmark Server API Token from a test-pool account — Postmark offers a free
developer tier with a sandbox server, so procurement is self-serve, no review).
L1 and L3 are fully hermetic.

---

## 6. Stage checklist status & shared-surface impact

- Axis ②↔③ identity → **no `toolcred/resolver.go` entry**.
- **One narrow, provider-local integration-service Go change** (§3): add
  `postmarkServerVerifier` (a named `manualTokenVerifier` alongside
  `declarativeManualTokenVerifier` / `dsnHostIdentityDeriver`) that builds the
  identity map from only the safe `/ServerLink` + `/Name` fields, and select it
  for Postmark via a provider-keyed branch in the `RuntimeStrategyManualAPIToken`
  case of `composeProviderRegistration` (defaulting to
  `declarativeManualTokenVerifier{}` for all other providers). This is
  allowlist-by-construction and touches **no** shared declarative-identity
  contract, **no** `cmd/provider-gen` projection, and **none** of the five
  generated projections. It ships with unit tests (§5 L3). An earlier draft
  proposed a general `identity.redact` field on the shared `Identity` contract;
  that is rejected in favor of this narrower named verifier (§3).
- **No `config/` or `deploy/` change** — manual token, no server-side secrets.
- Shared surfaces this tool touches at batch-end: the **integration-service
  named-verifier capability** (`service/manual_token_verifier.go` new
  `postmarkServerVerifier` + `service/provider_registry.go` provider-keyed
  selection + unit tests) — **no** `model/catalog.go` / `cmd/provider-gen`
  change; anycli `register.go` (`RegisterService("postmark", …)`), the anycli
  pin bump, `provider-gen` regen (five projections, unchanged shape),
  `providerIcons.ts` append, and the plugin version bump + docs publish
  (`agents/plugins/heliox/skills/tool/`). It does **not** touch the resolver map
  or the OAuth config surface.
- Ships **hidden**; visible flip is the single go-live change after L5.
