# Adobe Acrobat Sign — per-tool design (`tool/adobe-sign`)

Scratch design for the Adobe Acrobat Sign integration under `heliox tool`, per
the `helio-tool-provider` pipeline and the 298-tool master rollout plan
(`docs/design/008-300-integrations-rollout-plan.md`). Batch-lead strips this
file at batch end.

- **Catalog row 220** — Product: Adobe Acrobat Sign · anycli id `adobe-sign` ·
  provider key `adobe_sign` · auth lane **oauth_review** · **Wave 3** (3-hold?
  no — plain Wave 3) · category Scheduling & eSign.
- **Three axes:** ① CLI command word `adobe-sign` · ② anycli id `adobe-sign` ·
  ③ provider key `adobe_sign`. **②↔③ diverge** (dash vs underscore) → a
  `toolToProvider` entry is **required** (verified below: `ProviderFor` does a
  plain map lookup with no mechanical dash→underscore normalization on this
  base — OQ1 has not landed). No `tool.group`, no `tool.command`.
- **Go package:** `internal/tools/adobesign/` (id dashes dropped, per master
  plan §3 stage-2 package rule; matches the `microsoft-calendar` →
  `microsoftcalendar` precedent). Only `definitions/tools/adobe-sign.json` and
  the `RegisterService("adobe-sign", …)` string carry the exact dashed id.

Every fact below was verified against Adobe's official Acrobat Sign developer
guide (OAuth, Managing OAuth Tokens, Getting Started/quickstart, REST API v6)
and the partner/multi-tenant registration docs, and cross-checked against the
actual integration-service code on this worktree's base. Where the official
docs refine or contradict the catalog/audit assumption, the divergence is
recorded inline. Adobe Sign was already `oauth_review` in the seed catalog, so
the 2026-07-21 OAuth audit (which only re-examined `api_key` rows) did not
cover it — its lane is re-verified from first principles in §4.3.

---

## 1. What an AI teammate does with Adobe Acrobat Sign → which API surface

Adobe Acrobat Sign's product is **electronic signature**. The Adobe unit of
work is an **agreement** (the DocuSign "envelope" analog): a document sent to
one or more recipients for signature, tracked to completion, then retrieved.
An AI teammate acting for a user overwhelmingly wants to:

- **Send a document for signature** — create + send an *agreement*, from either
  an uploaded file or a reusable *library document* (template).
- **Check status** — "has the contract been signed?" — agreement status and
  per-participant signing status.
- **List / search** recent agreements ("what's out for signature", "what
  completed this week").
- **Retrieve the signed document** (download the combined completed PDF).
- **Cancel** an agreement sent in error (Adobe's "cancel", sender-initiated —
  the analog of DocuSign void).
- **List library documents** so a send can reference a reusable template by id.

That maps to exactly one Adobe API: the **Acrobat Sign REST API v6**, whose
base path is `/api/rest/v6` **relative to the account's shard host**
(`api_access_point`, e.g. `https://api.na1.adobesign.com/`; see §4). The
wrapped resources:

| Resource | Endpoint (relative to `{api_access_point}api/rest/v6`) | Verb used for |
|---|---|---|
| TransientDocuments | `POST /transientDocuments` | upload a raw file → `transientDocumentId` (prerequisite of a file-based send) |
| Agreements | `POST /agreements` | create + send (body `state:"IN_PROCESS"`, source = transient or library doc) |
| Agreements | `GET /agreements` | list (paginated; group filter) |
| Agreements | `GET /agreements/{agreementId}` | one agreement's status |
| Agreements | `GET /agreements/{agreementId}/members` | per-participant signing status |
| Agreements | `PUT /agreements/{agreementId}/state` (`state:"CANCELLED"`) | cancel |
| Agreements | `GET /agreements/{agreementId}/combinedDocument` | download combined signed PDF |
| LibraryDocuments | `GET /libraryDocuments`, `GET /libraryDocuments/{id}` | list / inspect templates |

**Two-step send (Adobe-specific, verified):** unlike DocuSign, a file-based
send in v6 is **two calls** — first `POST /transientDocuments` (multipart
upload) returns a `transientDocumentId`, then `POST /agreements` references it
in `fileInfos[].transientDocumentId`. The anycli `agreement send --document
<pdf>` command performs both internally (upload-then-create); `agreement send
--library-id <id>` skips the transient step and references a library document
directly. A standalone `document upload` command is also exposed for callers
that want the transient id explicitly.

**Deliberately out of scope for v1** (heavier, lower-frequency, or
compliance-sensitive; add later if demand warrants): widgets (web forms),
workflows authoring beyond referencing a `workflowId`, bulk send / mega-sign,
interactive/embedded signing views (`POST /agreements/{id}/views`), webhooks,
the Users/Groups admin surface, and reminders. `POST /search` (advanced
agreement search by date/externalId) is a **fast-follow** to `GET /agreements`
if the simple list proves insufficient. The v1 surface is the "send / track /
retrieve / cancel" loop an assistant actually drives.

---

## 2. Tool form decision — `service` type

**Decision: `service` type** (HTTP against the Acrobat Sign REST API v6), per
the skill's stage-1 rubric. Adobe publishes SDKs and a Postman collection but
**no official, non-interactive, `--json`-capable CLI binary** that could be
provisioned into the runtime image — the `cli`-type criteria fail. Implement in
`internal/tools/adobesign/` against the HTTP API, copying the shape of the
`internal/tools/notion/` reference (cobra tree grouped by resource; a
`BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest` server;
exit-code contract 0 success / 1 runtime-or-API failure / 2 usage-or-parse; a
`--json` structured error envelope). Default path — 21 of 23 shipped
definitions are `service`.

---

## 3. anycli definition + command surface

`definitions/tools/adobe-sign.json` — `type: "service"`, `name: "adobe-sign"`.

**Credential bindings (all injected as env by the resolver-supplied map):**

| field | inject | why |
|---|---|---|
| `access_token` | env `ADOBE_SIGN_ACCESS_TOKEN` | Bearer for every v6 call |
| `base_uri` | env `ADOBE_SIGN_BASE_URI` | account's shard host (`api_access_point`), e.g. `https://api.na1.adobesign.com/` (§4) |

The service composes the base path as `{base_uri}api/rest/v6` and sends
`Authorization: Bearer {access_token}`. `base_uri` is supplied by the Helio
resolver (captured at connect time from `api_access_point` — §4); anycli never
performs OAuth, never calls `/baseUris`, and stays credential-agnostic, exactly
as the embeddable-core contract requires. **No `account_id` field** — unlike
DocuSign, Adobe's v6 path is not account-id-scoped (the shard host +
bearer identify the account), so only the shard host is needed.

**Command tree** (grouped by resource, mirroring notion / docusign):

```
adobe-sign agreement send                # POST /transientDocuments then POST /agreements
                                         #   (--document <pdf> | --library-id <id>, --recipient-email,
                                         #    --recipient-name, --name <agreement name>)
adobe-sign agreement list                # GET /agreements                (--group-id, --cursor, --page-size)
adobe-sign agreement get      <id>       # GET /agreements/{id}
adobe-sign agreement members  <id>       # GET /agreements/{id}/members
adobe-sign agreement cancel   <id>       # PUT /agreements/{id}/state  state=CANCELLED  (--comment)
adobe-sign agreement download <id>       # GET /agreements/{id}/combinedDocument  (--out <path>)
adobe-sign library list                  # GET /libraryDocuments
adobe-sign library get        <id>       # GET /libraryDocuments/{id}
adobe-sign document upload    <pdf>      # POST /transientDocuments  → transientDocumentId
```

**No runtime auth/discovery command in v1.** anycli receives an
already-shard-scoped `base_uri`; it makes **no** `/oauth/v2/userinfo` or
`/baseUris` call. `base_uri` is captured Helio-side at connect time (§4.2). A
future `whoami`/account-enumeration aid is deferred for the same reason as
DocuSign — it would reintroduce an auth-host dependency the sourcing map does
not express, and nothing in the v1 loop needs it.

**JSON output shape** — provider-neutral, `--json` flips every command to a
structured envelope, field names snake_case (not Adobe's raw camelCase):

- `agreement list --json` →
  `{"agreements":[{"id":"…","status":"OUT_FOR_SIGNATURE","name":"…","created":"…"}],"page_cursor":"…"}`
- `agreement get <id> --json` →
  `{"id":"…","status":"SIGNED","name":"…","created":"…"}`
- `agreement members <id> --json` →
  `{"participants":[{"email":"…","status":"WAITING_FOR_MY_SIGNATURE","order":1,"role":"SIGNER"}]}`
- `agreement send --json` → `{"agreement_id":"…","status":"IN_PROCESS"}`
- errors → `{"error":{"code":"api_error|usage","message":"…","status":404}}`
  with the matching non-zero exit code.

Adobe returns status enums like `OUT_FOR_SIGNATURE` / `SIGNED` / `CANCELLED`;
these pass through as `status` verbatim (they are already provider-neutral
enough and re-mapping them would lose fidelity).

---

## 4. Credential fields + the exact OAuth flow (oauth_review lane verified)

Adobe Acrobat Sign's OAuth is a standard **Authorization Code Grant** with
**three non-standard mechanics** that make it materially harder than the eSign
tools shipped so far (DocuSign / SignNow / BoldSign). All three are verified
against the official developer guide and cross-checked against
integration-service code on this base. They are the defining risk of this tool
and are called out explicitly.

### 4.1 Flow: Authorization Code Grant (confidential client)

Helio is a confidential server-side client (holds the client secret). Verified
endpoints (Adobe migrated `echosign.com` → `adobesign.com`; use the current
hosts):

- **authorize:** `https://secure.adobesign.com/public/oauth/v2` —
  **generic host, no shard.** The official quickstart is explicit: the
  authorize Base URI *"should NOT contain the 'shard' of an account (i.e.:
  na1, na2, eu1, jp1, etc.)"*. Query params: `response_type=code`, `client_id`,
  `redirect_uri`, `scope` (space-delimited), `state`. ✅ fits `authorize_url`.
- **token (code→token):** `POST /oauth/v2/token`, `Content-Type:
  application/x-www-form-urlencoded`, body `grant_type=authorization_code` +
  `code` + `redirect_uri` + `client_id` + `client_secret` — **client creds in
  the FORM BODY, not a Basic header** → bundle `token_exchange_style:
  form_secret` (NOT `form_basic`; this is the DocuSign divergence — DocuSign
  used Basic, Adobe uses form). **PKCE: none** (confidential client).
- **refresh:** `POST /oauth/v2/refresh` — **a DIFFERENT PATH from token**
  (`/refresh`, not `/token`), form body `grant_type=refresh_token` +
  `refresh_token` + `client_id` + `client_secret`.
- **revoke:** `POST /oauth/v2/revoke` (exists; shard-hosted — see §4.4).

**Scopes.** Adobe's scope model is `scope:modifier` where the modifier is
`self` (default — acts as the authorizing user, no admin needed), `group`
(needs group admin + Business/Enterprise), or `account` (needs account admin +
Business/Enterprise). For the send/track/retrieve/cancel loop as the individual
user, request the **`:self`** tier:
`agreement_read:self agreement_write:self agreement_send:self
library_read:self user_login:self`. `offline_access` is **not** required for a
refresh token on Adobe Sign (the standard token response includes
`refresh_token` directly) — verified against the official token guide; do not
add it.

**Token TTL.** Access token `expires_in: 3600` (1 hour) is present in the token
response, so **no `assumed_ttl_seconds` is needed** (contrast Salesforce, which
returns no expiry). `oauthManifest` has no `assumed_ttl` field on this base
anyway (verified, `cmd/provider-gen/manifest.go:81-91`) — and Adobe doesn't
need it.

**Refresh token is NON-rotating (verified).** Adobe reuses the *same* refresh
token across refreshes; each use resets its idle-expiry to 60 days from now.
The token expires only after **60 days of inactivity**. Consequences:

- **No `refresh_lease: credential` needed for a rotation hazard.** Unlike
  DocuSign/SignNow (which mint a new refresh token every refresh and therefore
  serialize per credential to avoid breaking the chain), Adobe's non-rotating
  token has no such hazard. Leave `refresh_lease` unset (default). This is a
  genuine divergence from the sibling eSign tools — do not copy their
  `refresh_lease: credential` line.
- **60-day idle expiry is a real operational risk**, not a code concern: a
  connection unused for 60 days dies and needs re-auth. This is inherent to
  Adobe and acceptable for v1 (an assistant that never touches Adobe for two
  months re-consents). Worth a one-line note in the AI-facing doc, not
  engineering.

### 4.2 Account base-host discovery — `api_access_point` (the first real design decision)

The v6 REST base host is **account-specific** (`na1`, `na2`, `eu1`, `au1`,
`jp1`, …). It arrives as **`api_access_point`** and is delivered in **two**
places (verified against the official quickstart):

1. As a **query parameter on the redirect** back to `redirect_uri`, alongside
   the `code`:
   `?code=…&api_access_point=https%3A%2F%2Fapi.na1.adobesign.com%2F&web_access_point=…&state=…`
2. **Also** in the **token JSON response** (`api_access_point`,
   `web_access_point`).

Because `api_access_point` is in the **token response**, capturing it is the
**Salesforce `instance_url` pattern**, not the DocuSign userinfo deriver — a
**static RFC-6901 JSON Pointer `/api_access_point` into the token response**,
no separate GET, no `is_default`-array selection. This makes Adobe strictly
**simpler on the capture axis** than DocuSign.

**But that capability is still net-new on this base (verified, not "shipped"):**

1. **`metadata_capture` does not exist on `oauthManifest` or
   `connectionManifest`.** `oauthManifest` (`cmd/provider-gen/manifest.go:81-91`)
   is exactly `{AuthorizeURL, TokenURL, TokenExchangeStyle, PKCE,
   AuthorizeParams, Scopes, DisplayScopes, SingleActiveToken, RefreshLease,
   Revoke}`; `connectionManifest` (`manifest.go:109-113`) is exactly `{Mode,
   DisconnectMode, RuntimeStrategy}`. The generator strict-decodes with
   `decoder.KnownFields(true)`, so any `metadata_capture:` key is a **hard
   strict-decode failure** until the field is added.
2. **The `connection.metadata.base_uri` credential source is absent from the
   allowlist.** `knownCredentialSources` (`cmd/provider-gen/validate.go:53-59`)
   is exactly `{token.access_token, connection.account_key,
   connection.metadata.person_urn, credential.app_id, credential.brand}`;
   `validateCredentials` rejects any other `credential.fields` source as unsafe.
3. **The projection that reads `/api_access_point` from the token response and
   writes it to connection metadata** must be wired (the Salesforce-`instance_url`
   machinery — which is itself *unmerged* on this base; verified: no `assumed_ttl`
   / `metadata_capture` on `oauthManifest`, so Salesforce's own growth has not
   landed here).

This is the same *class* of net-new work DocuSign needs, but **less** of it:
Adobe needs a **static token-response pointer**, whereas DocuSign needs a
userinfo GET + `is_default` array-selection deriver. If the Salesforce
`instance_url` metadata-capture growth lands first (a sibling review-lane
branch), Adobe reuses it wholesale by pointing `metadata_capture.base_uri` at
`/api_access_point` and adding the one `connection.metadata.base_uri`
credential source.

### 4.3 Per-account OAuth **host** for token exchange + refresh (the second, harder design decision)

This is where Adobe diverges from **every** OAuth tool shipped so far, and it
is the tool's defining risk. Two facts collide:

- **The refresh endpoint is a different path (`/oauth/v2/refresh`) on the
  account shard host.** integration-service refreshes via
  `token_refresh.go:153`: `endpoint := oauth2.Endpoint{TokenURL:
  def.OAuth.TokenURL}` — the refresh POST reuses the **single fixed
  `token_url`** through `golang.org/x/oauth2`, which appends
  `grant_type=refresh_token` to that same URL. Adobe's refresh needs (a) a
  **`/refresh` path** (not `/token`) and (b) potentially the **shard host** —
  neither expressible with one fixed `token_url`.
- **The code→token exchange host may also be shard-specific.** The official
  quickstart POSTs the exchange to a fixed host in its example, but the
  redirect hands back `api_access_point` *before* the exchange precisely so the
  app can target the account shard, and older Adobe walkthroughs fetch the base
  host via `GET /baseUris` before any REST call. Whether a **generic**
  `secure.adobesign.com` (or `api.adobesign.com`) host accepts the exchange +
  refresh for **all** shards, or whether the shard host is mandatory, is
  **genuinely ambiguous in the docs and MUST be resolved at stage-1 L2** with a
  real developer account.

**Stage-1 L2 decision gate (the single most important pre-dev check):**

- **If a generic OAuth host accepts both `/oauth/v2/token` and `/oauth/v2/refresh`
  for any shard** → **Option A (declarative), recommended:** `token_url` is the
  fixed generic host; add one net-new **`refresh_url`** field to `oauthManifest`
  and thread it into `token_refresh.go` (use `refresh_url` when set, else fall
  back to `token_url`) so the refresh POST hits `/oauth/v2/refresh`. Combined
  with §4.2's `metadata_capture` for the *API* base, the bundle stays
  `standard_oauth` with **two** narrow declarative growths (`refresh_url` +
  `metadata_capture`/`connection.metadata.base_uri`). No compiled adapter.
- **If the exchange and/or refresh must target the per-account shard host** →
  **Option B (narrow compiled adapter `service/adapter_adobesign.go`),
  required, not optional.** A fixed `refresh_url` cannot express a per-account
  host, so declarative growth is insufficient. The adapter — precedent
  `adapter_slack.go` / `adapter_linkedin.go` / `adapter_x.go` for
  "response shape or lifecycle outside the closed capability set" — would: read
  `api_access_point` from the callback/token response, perform the code→token
  exchange and the `/oauth/v2/refresh` refresh against `{api_access_point}`,
  and populate `base_uri` into connection metadata (projected through the
  still-net-new `connection.metadata.base_uri` credential source). This is the
  master plan's anticipated "a handful of the review lane need a narrow adapter"
  (§5, Bill.com-class) case.

**Recommendation:** treat Option A as the target and Option B as the
contingency; **the stage-1 L2 host probe decides**. Do not assume A silently —
the DocuSign design's lesson applies doubly here: "no adapter" and "reuses
shipped capabilities" are not both guaranteed true, and Adobe's separate-path
shard-hosted refresh is stronger evidence for an adapter than DocuSign's was.
Either way, `base_uri` reaches anycli as a credential field and the anycli
service is unchanged — all shard/OAuth complexity is Helio-side.

### 4.4 Disconnect / revoke

Adobe has `POST /oauth/v2/revoke`, but it is **shard-hosted** (same per-account
host problem as refresh) and integration-service's declarative revoker uses a
fixed `revoke.url`. So set **`disconnect_mode: local_only`** (the SignNow /
DocuSign precedent): Helio forgets the credential; the user revokes app access
from their Adobe account. A declarative shard-aware revoker is only worth
building if Option B's adapter already exists (it would own the shard host
anyway) — defer.

### 4.5 oauth_review lane — verified from first principles

Adobe distinguishes **CUSTOMER** (single-tenant, own account) from **PARTNER**
(multi-tenant, other organizations' accounts) API applications at registration
time. Helio's model — arbitrary customer accounts authorize one Helio app — is
the **PARTNER** model, and **Adobe requires PARTNER apps to pass a partner
certification process before production deployment against external accounts**
(verified against the partner/embed quickstart + provisioning FAQ). Mapping to
the plan:

- App creation + OAuth configuration (redirect URI, scopes, app id/secret) is
  **self-serve** in the Acrobat Sign admin console (Acrobat Sign API → API
  Applications). A CUSTOMER app (or an uncertified PARTNER app tested within the
  developer's own account) yields working dev credentials immediately → **dev,
  L1–L4, and the batch-end merge are NOT gated** (hidden-first).
- **Partner certification gates only the visible flip** — the review-clearance
  gate the `oauth_review` lane captures. It gates neither L4 nor the merge.

**oauth_review is correct.** Confidence: high (explicit CUSTOMER-vs-PARTNER
model + certification requirement in official docs). Divergence to note on the
wave-board: the tail is Adobe **partner certification** (enterprise e-sign
security/compliance review), which can run longer than DocuSign's streamlined
Go-Live but is a normal partner program, not an indefinite Meta/TikTok-class
stall.

### 4.6 `required_config_fields`

`[oauth.client_id, oauth.client_secret]` — Adobe's application id + secret.
Supplied per environment through integration-service config (`config/` locally
+ the Helm Secret in `deploy/`, kept in sync per the Config Sync hard rule);
never in the bundle. Fully-absent config → `configured: false` (Connect
disabled, safe to ship hidden); a *partial* config fails service startup — so
id and secret land together (lane 1). Adobe app credentials are shard/account
specific, so lane 1 records which data center the Helio partner app lives on.

---

## 5. Helio provider bundle plan (`integrations/providers/adobe_sign/provider.yaml`, hidden-first)

Directory / `key:` = `adobe_sign` (axis ③). Bundle sketch (final field
spellings confirmed against `provider-yaml.md` + `provider-gen --check` at
build time). **Assumes Option A (declarative)**; Option B replaces the OAuth
host handling with a compiled adapter and drops `refresh_url` in favor of the
adapter, but keeps the same `credential.fields` / `metadata_capture` sources.

```yaml
schema: helio.provider/v1
key: adobe_sign
go_name: AdobeSign

presentation:
  name: Adobe Acrobat Sign
  description_key: adobe_sign
  consent_domain: adobesign.com
  visible: false          # hidden-first; visible flip gated on L5 + partner certification
  order: 0                # batch lead assigns at go-live

auth:
  type: oauth
  owner: individual        # consent authenticates an Adobe Sign user; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://secure.adobesign.com/public/oauth/v2   # generic host, NO shard
    token_url: https://secure.adobesign.com/oauth/v2/token         # Option A: generic host (stage-1 L2 must confirm)
    token_exchange_style: form_secret     # client creds in FORM BODY (NOT form_basic — the DocuSign divergence)
    pkce: none                            # confidential client
    scopes: [agreement_read:self, agreement_write:self, agreement_send:self, library_read:self, user_login:self]
    display_scopes: [agreement_read, agreement_write, agreement_send, library_read]
    single_active_token: false
    # NO refresh_lease — Adobe refresh tokens are NON-rotating (60-day idle reset),
    # so there is no per-credential rotation hazard (unlike DocuSign/SignNow).
    # NET-NEW FIELD (Option A): refresh_url points the refresh POST at /oauth/v2/refresh
    # (different path from token). oauthManifest has no refresh_url on this base
    # (manifest.go:81-91) and token_refresh.go:153 reuses def.OAuth.TokenURL — so this
    # key fails strict-decode until the field + token_refresh.go threading are added.
    refresh_url: https://secure.adobesign.com/oauth/v2/refresh

identity:
  source: token_response     # no cheap userinfo needed for identity; token response suffices,
                             # OR source: userinfo with url https://secure.adobesign.com/oauth/v2/userinfo
  stable_key: /userId        # stage-1: confirm which identity field Adobe returns (userinfo /userId/email)
  label_candidates: [/email, /userId]

connection:
  mode: isolated
  disconnect_mode: local_only            # revoke exists but is shard-hosted; local_only is the safe choice (§4.4)
  runtime_strategy: standard_oauth
  # base_uri captured from the token response's api_access_point (Salesforce instance_url pattern).
  # NET-NEW FIELD: connectionManifest has no metadata_capture and KnownFields(true) rejects it (§4.2 item 1).
  metadata_capture:
    base_uri: /api_access_point          # static RFC-6901 pointer into the TOKEN response (not a deriver)

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
    # NET-NEW SOURCE: connection.metadata.base_uri absent from knownCredentialSources
    # (validate.go:53-59) → validateCredentials rejects it until added (§4.2 item 2).
    base_uri: connection.metadata.base_uri
    account_key: connection.account_key

tool:
  name: adobe-sign          # axis ② (dashed anycli id); ②≠③ → toolToProvider entry required
  kind: oauth
```

**Capability growth needed (Helio side) — verified against this base, honest
accounting:**

1. **`connection.metadata_capture` manifest field** accepting a
   token-response JSON Pointer — absent from `connectionManifest`
   (`manifest.go:109-113`); `KnownFields(true)` rejects the sketch. Same field
   Salesforce/DocuSign need; **Adobe's variant is the simplest** (static token-
   response pointer, no userinfo GET, no array selection).
2. **`connection.metadata.base_uri` credential source** — absent from
   `knownCredentialSources` (`validate.go:53-59`); `validateCredentials` rejects
   it today.
3. **`refresh_url` on `oauthManifest` + `token_refresh.go` threading**
   (**Option A only**) — `token_refresh.go:153` reuses `def.OAuth.TokenURL`; a
   `refresh_url` (when set) must override it so refresh hits `/oauth/v2/refresh`.
   **Contingent on the §4.3 stage-1 L2 host probe:** if the exchange/refresh
   must be shard-hosted, item 3 is replaced by **Option B's compiled
   `adapter_adobesign.go`**, which owns the shard-aware exchange + refresh +
   `base_uri` capture instead.

**Genuinely reused / shipped (not net-new):** `form_secret` token exchange,
`pkce: none`, `local_only` disconnect, `standard_oauth` runtime strategy,
`isolated` connection mode, the generic `authorize_url`. **NOT shipped — do not
treat as such:** `metadata_capture`, the `connection.metadata.base_uri`
credential source, and (Option A) `refresh_url`.

- **`toolToProvider` divergence (required):** add `"adobe-sign":
  "adobe_sign"` to `helio-cli/internal/toolcred/resolver.go` — verified: on this
  base `ProviderFor` is a plain map lookup returning the input unchanged on miss
  (no mechanical dash→underscore normalization; OQ1 has not landed), so without
  this entry the token gateway is queried under `adobe-sign` and fails. This is
  one of the master plan's 23 mechanical dash↔underscore entries.
- **Shared surfaces** touched at batch end (per master plan §2): anycli
  `register.go` + pin bump, `provider-gen` (5 projections), `providerIcons.ts`
  append (`ui/helio-app/src/integrations/icons/adobe_sign.svg`) + any i18n label
  strings, plugin docs publish, and the integration-service config append
  (client id/secret) landed by lane 1. No `tool.group`.
- **AI-facing docs:** an `adobe-sign` sub-doc under
  `agents/plugins/heliox/skills/tool/` describing the send / track / retrieve /
  cancel loop, the two-step file send (transient upload → agreement), and the
  60-day idle re-auth note.

---

## 6. Test plan — five layers

| Layer | Adobe Sign specifics | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fake of the v6 API: assert base path `{base_uri}api/rest/v6`, `Authorization: Bearer` header; `transientDocuments` multipart upload → id; `agreement send` two-step (transient-then-create AND `--library-id` single-step); `agreement list/get/members`; `agreement cancel` PUT `/state` body `state:CANCELLED`; `combinedDocument` download to `--out`; `library list/get`; plain + `--json` error rendering (401/404). The `api_access_point` capture and the OAuth host handling are **Helio-side** concerns, unit-tested there, not in anycli. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN` + `ANYCLI_CRED_BASE_URI` (the shard host) from a real Adobe Sign **developer** account; run `document upload`, `agreement send`, `agreement get`, `agreement list`, `library list` against the account's shard. Proves field names + the two-step send + request shapes match the live v6 API. **Also the §4.3 stage-1 host probe** (does the generic host accept token exchange + refresh, or is the shard mandatory?) — this decides Option A vs B and MUST run before Helio-side dev commits to a shape. | **Yes** — Adobe Acrobat Sign developer account (free dev tier) + a token minted from a registered app (lane 1). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (5 projections) green **only after the §4.2/§4.3 growth lands** (the `metadata_capture` field + `connection.metadata.base_uri` source, plus Option A's `refresh_url` field OR Option B's adapter registration — without them the bundle fails strict-decode / the credential-source allowlist); the new metadata-capture (and/or refresh_url / adapter) unit tests green; helio-cli + integration-service suites green; `toolToProvider` resolver test covers `adobe-sign`→`adobe_sign`. On-branch: local `replace` to the anycli branch + local regen (not committed). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"adobe_sign"`, seeding `access_token` (+ short `expires_at`) **and** `refresh_token` **and** the captured `base_uri` metadata, then `heliox tool adobe-sign -- agreement list`. Exercises token gateway → refresh (against `/oauth/v2/refresh`, Option A) → metadata-injected `base_uri` → anycli path. Seed `base_uri` explicitly since L4 bypasses the connect-time capture. The short-`expires_at` forces the refresh path — the single best test of the §4.3 divergence. | **Yes** — a real developer access + refresh token and the shard `base_uri` (lane 1 dev app). |
| **L5** full connect (pre-flip, once) | `heliox tool adobe-sign auth` → real Adobe consent on the dev/partner app → confirm `oauth_connected` event → confirm `api_access_point` captured as `base_uri` on the connection → run `agreement list` unseeded through the new connection. Human-in-the-loop (oauth L5). Gates the visible flip together with **partner certification clearance**. | **Yes** — human consent on a real Adobe Sign account (lane 3) + partner certification (lane 1 review) before the flip. |

**Layers needing externally supplied credentials:** L2, L4, L5 (an Adobe
Acrobat Sign developer account + the lane-1 partner/dev app; L5 additionally
needs a human consent session and partner certification before the visible
flip). L1 and L3 are fully self-contained. **L2 additionally carries the
stage-1 host-probe that selects Option A vs Option B — the earliest gate.**

---

## 7. Summary of decisions & required growth

- **Type:** `service`; package `internal/tools/adobesign/`; anycli id
  `adobe-sign`, provider key `adobe_sign` → **`toolToProvider` entry required**
  (dash↔underscore; verified no mechanical normalization on this base). No
  group.
- **API:** Acrobat Sign REST API v6 — agreements (send/list/get/members/cancel/
  download), library documents (list/get), transient documents (upload). File
  send is **two calls** (transient upload → agreement). Base host is the
  per-account shard `api_access_point`, injected as `base_uri`; anycli makes no
  OAuth or discovery call.
- **Auth:** Authorization Code Grant, confidential client, **`form_secret`**
  token exchange (form-body client creds — NOT Basic), scopes
  `agreement_read/write/send:self library_read:self user_login:self` (no
  `offline_access`), 1-hour access token (`expires_in` present, no
  `assumed_ttl`), **non-rotating** refresh token with 60-day idle expiry (**no
  `refresh_lease`**), `local_only` disconnect.
- **Three verified divergences from the shipped eSign tools, all Helio-side:**
  (1) `form_secret` not `form_basic`; (2) API base host is the per-account
  `api_access_point` shard → **`metadata_capture` static token-response pointer
  `/api_access_point`** (Salesforce pattern, simpler than DocuSign's userinfo
  deriver) + **`connection.metadata.base_uri` credential source** — both
  net-new on this base (`manifest.go:109-113` / `validate.go:53-59`); (3)
  **refresh is a different path (`/oauth/v2/refresh`) on a possibly shard-
  specific host**, which `token_refresh.go:153`'s single fixed `token_url`
  cannot express.
- **Option A vs B decision gate (§4.3), decided at stage-1 L2:** if a generic
  OAuth host serves exchange + refresh for all shards → **Option A
  (declarative):** add a `refresh_url` field + `metadata_capture`; bundle stays
  `standard_oauth`, no adapter. If exchange/refresh must be shard-hosted →
  **Option B (narrow compiled `adapter_adobesign.go`), required** — owns the
  shard-aware exchange, `/oauth/v2/refresh`, and `base_uri` capture. Do not
  assume A silently.
- **oauth_review:** Adobe **PARTNER** (multi-tenant) app requires **partner
  certification** before production; self-serve dev/test app is immediate, so
  the gate is on the **visible flip only**, not dev/L4/merge. Lane correct
  (high confidence). Wave-board note: partner-certification tail, longer than
  DocuSign Go-Live but a normal partner program.
- **Open items for lane 1 / batch lead:** (a) the §4.3 host probe result (A vs
  B); (b) which Adobe data center the Helio partner app is registered on (app
  credentials are shard/account specific); (c) confirm the identity field
  Adobe returns (`token_response` vs a `/oauth/v2/userinfo` GET) at stage-1.
