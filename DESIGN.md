# Adobe Acrobat Sign — per-tool design (`tool/adobe-sign`)

Scratch design for the Adobe Acrobat Sign integration under `heliox tool`, per
the `helio-tool-provider` pipeline and the 298-tool master rollout plan
(`docs/design/008-300-integrations-rollout-plan.md`). Batch-lead strips this
file at batch end.

## Implementation divergences (as built — read before the batch-end regen)

Two mechanism divergences from §4.2/§4.3 surfaced when building against the
actual integration-service code; both were resolved cleanly and are recorded
here per the ground-truth rule.

1. **base_uri capture is via the identity blob, not a direct adapter write.**
   §4.2 said "the adapter writes `base_uri` into connection metadata directly
   during exchange." The `tokenExchanger.Exchange` / `identityResolver.Identity`
   facets have no access to the `Connection`, so the adapter cannot write
   metadata directly. As built, `adapter_adobesign.go`'s `Identity` returns
   `base_uri` in the identity map, and `callbackConnectionMetadata` promotes it
   to a top-level `Connection.Metadata["base_uri"]` — mirroring the existing
   `person_urn` promotion exactly. Net-new credential source
   `connection.metadata.base_uri` + `TokenResult.BaseURI` projection as planned.

2. **Code exchange targets the generic host + captures api_access_point from the
   token RESPONSE (not the redirect param).** §4.3 said the exchange host comes
   from the redirect's `api_access_point`. On this base the redirect query
   params are NOT plumbed to the exchanger (`authorizationCodeGrant` carries only
   code/redirect_uri/verifier, and the callback DTO does not capture the param).
   Threading that param through the public callback DTO → session → grant →
   exchanger is a cross-cutting change beyond a narrow adapter. As built, the
   adapter exchanges against the bundle's shardless `token_url`
   (`secure.adobesign.com/oauth/v2/token`) and reads `api_access_point` from the
   **token response** (the Salesforce `instance_url` pattern). **Refresh IS
   shard-aware** as designed: `token_refresh.go` now threads the `Connection` and
   targets `{base_uri}oauth/v2/refresh`.

   **Lane-1 / stage-1 residual (was the DESIGN's "generic-host probe"):** confirm
   against a real Adobe partner/dev account whether `secure.adobesign.com/oauth/
   v2/token` accepts the code exchange for all shards (returning
   `api_access_point`). If it does NOT — i.e. exchange is strictly shard-hosted —
   the redirect-param → exchanger plumbing becomes required before L5. Refresh is
   already shard-hosted and correct either way. This is the one L2/L4-only item;
   L1/L3 are self-contained and green.

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
library_read:self user_login:self`.

**`offline_access` must NOT be added — official-docs divergence, rebutting a
review finding (verified 2026-07-22).** A review of this design flagged the
missing `offline_access` scope as a blocker, quoting Adobe text that a
refresh token is issued *"only if `offline_access` is included ... in your GET
request to `/authorize`."* **That text is from a different Adobe OAuth system
and does not apply here.** Two facts settle it against the official docs:

1. **Acrobat Sign's native OAuth returns `refresh_token` unconditionally — no
   `offline_access` involved.** The Acrobat Sign developer guide's
   "Getting the access token" step states the token response is *"a JSON body
   containing the access token **and the refresh token**"* with **no**
   `offline_access` in the flow, and Adobe's own sample authorize URL scope is
   literally `user_login:self+agreement_send:self+agreement_write:self+agreement_read:self+agreement_retention:self+library_read:self`
   — `offline_access` does not appear. The `/oauth/v2/refresh` guide (§4.3)
   likewise never mentions it. The word `offline_access` appears **nowhere** in
   the Acrobat Sign developer guide (OAuth, Getting Started, or Managing OAuth
   Tokens pages).
2. **The quoted `offline_access` requirement is from Adobe IMS, a distinct
   OAuth stack this design explicitly rejects (§5 identity).** The
   *"must include `offline_access`"* rule lives in the **Adobe Developer
   Console / IMS User Authentication** guide (`developer.adobe.com/developer-console/...`,
   host `ims-na1.adobelogin.com`) — the OAuth used by Creative Cloud / Analytics
   / Experience Platform APIs. Acrobat Sign's REST API v6 uses its **own**
   OAuth (`secure.adobesign.com/public/oauth/v2`, shard `api_access_point`
   hosts, `/oauth/v2/token` + `/oauth/v2/refresh`) — the whole §4 flow — **not**
   IMS. The finding conflated the two Adobe stacks.

**Adding `offline_access` would be actively harmful, not merely redundant.**
Adobe requires *"scopes provided in the Authorization URL [to] match exactly
with the scopes provided in the Adobe Sign application,"* and `offline_access`
is **not** a selectable Acrobat Sign API-Application scope (the scope catalog is
`user_login` / `agreement_*` / `library_*` / `widget_*` / `workflow_*` with
`self`/`group`/`account` modifiers — no `offline_access`). Requesting a scope
the app does not enable yields an authorize-time error. So the correct scope set
stays exactly the five `:self` scopes above; the refresh apparatus in
§4.1/§4.3/§5/§6 operates on the `refresh_token` Acrobat Sign issues *directly*
in the token response, as verified. The finding is **not adopted**; this
divergence is recorded per the ground-truth rule (official docs override the
review verdict).

**Token TTL.** Access token `expires_in: 3600` (1 hour) is present in the token
response, so **no `assumed_ttl_seconds` is needed** (contrast Salesforce, which
returns no expiry). `oauthManifest` has no `assumed_ttl` field on this base
anyway (verified, `cmd/provider-gen/manifest.go:81-91`) — and Adobe doesn't
need it.

**Refresh token is NON-rotating (verified).** Adobe reuses the *same* refresh
token across refreshes; each use resets its idle-expiry to 60 days from now.
The token expires only after **60 days of inactivity**. Consequences:

- **`refresh_lease: none` — explicit, not omitted, and not `credential`.**
  Unlike DocuSign/SignNow (which mint a new refresh token every refresh and
  therefore serialize per credential to avoid breaking the chain), Adobe's
  non-rotating token has no such hazard — so the *correct* lease is `none`, not
  `credential`. But `refresh_lease` is a **mandatory enum, not an omittable
  field**: `oauthManifest.RefreshLease` (`manifest.go:90`) carries no
  `omitempty`, and `validateAuth` (`validate.go:249`) runs
  `oneOf(RefreshLease, "none", "credential", "provider")`, so an omitted key
  decodes to `""` and fails `provider-gen --check` with
  `auth.oauth.refresh_lease "" is invalid`. All 19 shipped OAuth bundles set it
  explicitly (18 `none`, 1 `provider`). So set it **explicitly to `none`**
  (→ `OAuthLeaseNone`, `validate.go:441-443`); do **not** leave it unset. The
  divergence from the sibling eSign tools is the *value* (`none`, not their
  `credential`), not the omission.
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

**The one unconditional net-new bundle-side capability (verified, not
"shipped"):**

- **The `connection.metadata.base_uri` credential source is absent from the
  allowlist.** `knownCredentialSources` (`cmd/provider-gen/validate.go:53-59`)
  is exactly `{token.access_token, connection.account_key,
  connection.metadata.person_urn, credential.app_id, credential.brand}`;
  `validateCredentials` rejects any other `credential.fields` source as unsafe.
  This is required **regardless** of how `base_uri` gets into connection
  metadata — it is the only way anycli receives `base_uri`.

**Who writes `base_uri` into connection metadata — the adapter, NOT a declarative
`metadata_capture` field.** The §4.3 compiled adapter is **mandatory** for this
tool (the shard-hosted exchange forces it), and during exchange it *already*
reads `api_access_point`. So the adapter writes `base_uri` into connection
metadata **directly** — the same value the refresh-path change (§4.3 shape (a))
reads back. That makes a declarative `connection.metadata_capture` manifest field
**redundant** for Adobe: declaring both the adapter *and* `metadata_capture`
would be two mechanisms for one job (non-orthogonal). So:

- **`metadata_capture` is NOT required growth for this tool.** It is retained
  here only as a note: *if* a future declarative simplification lands (a generic
  host that accepts exchange/refresh for all shards, removing the adapter),
  `metadata_capture.base_uri: /api_access_point` (a static RFC-6901 pointer into
  the token response — the simplest Salesforce-`instance_url` variant, no
  userinfo GET, no array selection) would be the declarative alternative. It is
  **not** built for v1.

Adobe is strictly **simpler on the capture axis** than DocuSign (a static
token-response value vs. a userinfo GET + `is_default` array-selection deriver),
but with the mandatory adapter the capture is not even a declarative field — the
adapter owns it.

### 4.3 Per-account OAuth **host** for token exchange + refresh (compiled adapter — the documented-required path)

This is where Adobe diverges from **every** OAuth tool shipped so far, and it is
the tool's defining risk. The official docs are **not ambiguous**: both the
code→token exchange and the refresh are **shard-hosted on `api_access_point`**
(verified against the Getting Started and Managing OAuth Tokens guides):

- **Token exchange is shard-hosted.** The Getting Started guide POSTs the
  code→token exchange to the account shard: `POST /oauth/v2/token` with
  `Host: api.na1.adobesign.com` — i.e. the `api_access_point`. The redirect
  hands back `api_access_point` *before* the exchange precisely so the app
  targets the account shard.
- **Refresh is a different path on the same shard host.** The Managing OAuth
  Tokens guide states the refresh POST *"must use the api_access_point (shard)
  … as the base URL"*: `POST /oauth/v2/refresh` with
  `Host: api.na1.adobesign.com`. It is both a **different path** (`/refresh`,
  not `/token`) **and** a **per-account host**. Adobe's own guidance: *"Always
  dynamically use the `api_access_point` host returned during the access token
  exchange (rather than hardcoding `na1`)."*
- Only the **authorize** link is shard-less (generic host, §4.1). Everything
  after the redirect is shard-hosted.

integration-service **cannot** express this declaratively, and — critically —
**the exchange path and the refresh path are two different, independently-wired
mechanisms.** They must be grown separately:

- **Code→token exchange** is strategy-dispatched. `composeProviderRegistration`
  (`provider_registry.go:78`) switches on `RuntimeStrategy`; a compiled adapter
  reached via `composeExplicitOAuthRegistration` supplies the `tokenExchanger`
  facet, so the exchange host *can* be overridden by the adapter.
- **Refresh is NOT strategy-dispatched.** `refreshOAuthToken`
  (`token_refresh.go:16`) calls `requestOAuthRefresh(ctx, provider, def, td)`
  (signature at line 148) — it passes **only** `provider, def, td`, never the
  `Connection`, even though `conn` is in scope at the call site. Inside,
  `requestOAuthRefresh` builds `endpoint := oauth2.Endpoint{TokenURL:
  def.OAuth.TokenURL}` (line 153) and refreshes through `golang.org/x/oauth2`
  (appending `grant_type=refresh_token` to that fixed URL). It **never consults
  the registry, never sees the adapter, and never sees
  `connection.metadata.base_uri`.** The `tokenExchanger` interface
  (`provider_adapter.go:15-20`) exposes **only** `Exchange` (authorization_code);
  the slack/x/linkedin/discord adapters implement `Exchange` + `Identity` and
  nothing else, and `composeExplicitOAuthRegistration` (`provider_registry.go:130`)
  wires **only** `exchanger` + `identity` + `revoker` into `oauthProviderRuntime`
  (struct at line 29). **There is no refresh facet anywhere in the adapter
  mechanism.**

So an adapter alone makes only the **initial code-exchange** shard-aware. Refresh
still POSTs to the fixed placeholder `token_url` (`secure.adobesign.com/oauth/v2/token`),
which per Adobe fails for shard accounts (community 401 reports on cross-shard
`.../oauth/v2/token`) — and the correct `/oauth/v2/refresh` *path* is never
reached either, since `x/oauth2` appends the grant to the fixed `token_url`
rather than a distinct refresh URL. A fixed `token_url` — or a fixed net-new
`refresh_url` — still **cannot** encode a per-account shard host. A purely
declarative bundle is therefore **not viable** for Adobe, **and the compiled
adapter does not cover refresh.**

**→ The required path is a compiled adapter for exchange+identity PLUS net-new
refresh-path plumbing in `token_refresh.go`.** Adapter precedent (exchange +
identity only): `adapter_slack.go` / `adapter_linkedin.go` / `adapter_x.go`,
wired via `composeExplicitOAuthRegistration`. `service/adapter_adobesign.go`
would: read `api_access_point` from the redirect callback / token response,
perform the code→token exchange against `{api_access_point}`, do the
shard-hosted `GET /users/me` identity call (§5), and **populate `base_uri` into
connection metadata directly** (so refresh can later read it — see below and
§4.2). The bundle still declares a schema-required `token_url` (a generic
placeholder to satisfy `validateHTTPSURL`), but the adapter overrides the host
at runtime with the captured shard.

**The refresh path is net-new work the adapter does NOT do.** Because refresh is
not strategy-dispatched, making it shard-aware requires changing
`token_refresh.go` itself. Two viable shapes (pick one at implementation):

- **(a) Thread the `Connection` into the refresh call.** Change
  `requestOAuthRefresh` to accept `conn` (already in scope in `refreshOAuthToken`),
  resolve the refresh host from `connection.metadata.base_uri`, and target
  `{base_uri}oauth/v2/refresh` explicitly (both a per-account host **and** the
  distinct `/refresh` path — neither of which the current fixed-`TokenURL` +
  `x/oauth2` grant-append can produce). This is the smaller change and reuses the
  `base_uri` the adapter already persisted to connection metadata.
- **(b) Add a per-strategy `refresher` facet** to the adapter interface
  (alongside `tokenExchanger`/`identityResolver`), wire it into
  `oauthProviderRuntime` + `composeExplicitOAuthRegistration`, and dispatch to it
  from `refreshOAuthToken` when present. More surface, but keeps all shard logic
  inside the adapter.

Either way, **both the shard host and the `/oauth/v2/refresh` path are net-new in
`token_refresh.go`** — this is not covered by reusing the slack/x adapter
mechanism. This is the master plan's anticipated "a handful of the review lane
need a narrow adapter" (§5, Bill.com-class) case, but for Adobe the "narrow
adapter" is understated: it is an adapter **plus** a refresh-path change.

**Adapter selection is by `RuntimeStrategy`, a closed enum — so Option B needs a
net-new runtime strategy.** `provider_registry.go` routes `standard_oauth` to
the declarative `standardOAuthExchanger{}` (line 80); only the fixed set
`{linkedin_oidc, x_exclusive_grant, slack_self_built_app, discord_bot_install}`
routes to compiled adapters via `composeExplicitOAuthRegistration`. Adobe's
adapter therefore requires a **new `model.RuntimeStrategy` value** (e.g.
`adobe_sign_shard`), added to the enum + both `provider_registry.go` switches +
the runtime-contract validation. **`runtime_strategy: standard_oauth` is
wrong** for this tool — that is the crux the earlier "Option A stays
standard_oauth" framing got backwards.

**Stage-1 L2 probe (narrowed, not a decision gate).** The host question is
*settled* by the docs (shard-hosted); build Option B. The only residual stage-1
check is the **inverse**: does a generic host *also* accept the exchange/refresh
for all shards (which would enable a future declarative simplification)? Treat
that strictly as a note for later — **do not** hold Option B on it. Either way,
`base_uri` reaches anycli as a credential field and the anycli service is
unchanged — all shard/OAuth complexity is Helio-side.

### 4.4 Disconnect / revoke

Adobe has `POST /oauth/v2/revoke`, but it is **shard-hosted** (verified — same
`api_access_point` base as refresh) and integration-service's declarative
revoker uses a fixed `revoke.url`. So set **`disconnect_mode: local_only`** (the
SignNow / DocuSign precedent): Helio forgets the credential; the user revokes
app access from their Adobe account. Since the §4.3 adapter already owns the
shard host, a shard-aware revoke could later be folded into it — but
`local_only` is the correct, safe v1 choice; defer the active revoke.

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
build time). **Uses the §4.3 compiled adapter (`adapter_adobesign.go`) — the
documented-required path** — for the shard-hosted **exchange** and the
`GET /users/me` **identity** (the two facets the adapter interface exposes). The
shard-hosted **refresh** is NOT the adapter's job — it is net-new plumbing in
`token_refresh.go` (§4.3), because the refresh path is not strategy-dispatched
and the adapter has no refresh facet. Consequences that shape the sketch:
`token_url` is declared only to satisfy the schema and is host-overridden at
runtime **for exchange**; `runtime_strategy` is a **net-new compiled strategy**,
not `standard_oauth`; and there is **no** `refresh_url` (a fixed URL cannot reach
a per-account shard — refresh is fixed instead in `token_refresh.go` by reading
`connection.metadata.base_uri`).

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
    authorize_url: https://secure.adobesign.com/public/oauth/v2   # generic host, NO shard (shardless authorize)
    token_url: https://secure.adobesign.com/oauth/v2/token         # schema-required placeholder only; adapter_adobesign.go
                                                                   # overrides the host to {api_access_point} at runtime (§4.3)
    token_exchange_style: form_secret     # client creds in FORM BODY (NOT form_basic — the DocuSign divergence)
    pkce: none                            # confidential client
    scopes: [agreement_read:self, agreement_write:self, agreement_send:self, library_read:self, user_login:self]
    # NO offline_access: that scope belongs to Adobe IMS OAuth, NOT Acrobat Sign's native OAuth (§4.1). Acrobat
    # Sign returns refresh_token directly in the token response with no offline_access; offline_access is not a
    # selectable Acrobat Sign app scope and requesting it fails the exact-scope-match check. Verified 2026-07-22.
    display_scopes: [agreement_read, agreement_write, agreement_send, library_read]
    single_active_token: false
    refresh_lease: none    # MANDATORY enum (validate.go:249; RefreshLease has no omitempty, manifest.go:90) — an omitted
                           # key decodes to "" and fails provider-gen --check. Adobe refresh tokens are NON-rotating
                           # (60-day idle reset), so the correct value is `none`, NOT `credential` (unlike DocuSign/SignNow).
    # NO refresh_url: refresh is shard-hosted on {api_access_point}/oauth/v2/refresh (§4.3), which a fixed
    # token_url/refresh_url cannot express — adapter_adobesign.go owns the shard-aware exchange + refresh.

identity:
  # VERIFIED against the official token guide: the token response carries NO identity field —
  # only access_token/refresh_token/token_type/expires_in/api_access_point/web_access_point. So
  # `source: token_response` with `/userId` would FAIL identity extraction at runtime (there is no
  # /userId in the response). There is also NO Acrobat-Sign OIDC userinfo on the OAuth host; the v6
  # caller-identity call is `GET /users/me` on the SHARD host {api_access_point} — shard-hosted like
  # refresh, so no fixed userinfo url can reach it. Identity is therefore owned by the §4.3 compiled
  # adapter (adapter_adobesign.go's identityResolver does the shard-hosted GET /users/me and returns
  # the account key + label directly), selected by the net-new runtime_strategy below — exactly as
  # x/slack's adapters own identity. So identity is adapter-owned, not declarative:
  source: strategy           # compiled resolver (shard GET /users/me); stage-1 confirms exact field (userId / email)
  # Declarative alternative (rejected for v1): Adobe IMS userinfo
  # https://ims-na1.adobelogin.com/ims/userinfo/v2 — but it is IMS-region-hosted and requires
  # openid/email/profile scopes NOT in the Acrobat Sign scope set above, a worse fit than the
  # adapter's shard GET /users/me.

connection:
  mode: isolated
  disconnect_mode: local_only            # revoke exists but is shard-hosted; local_only is the safe choice (§4.4)
  runtime_strategy: adobe_sign_shard      # NET-NEW compiled strategy (NOT standard_oauth): routes to adapter_adobesign.go
                                          # via composeExplicitOAuthRegistration (§4.3). Needs a runtimeStrategyContracts
                                          # entry pinning provider=adobe_sign + the oauth/owner/mode/disconnect tuple,
                                          # plus the model.RuntimeStrategy enum value + both provider_registry.go switches.
  # base_uri = api_access_point, present in the TOKEN response. The mandatory adapter_adobesign.go writes it into
  # connection metadata DIRECTLY during exchange — NO declarative `metadata_capture` field (that would be a second
  # mechanism for one job; §4.2). The refresh-path change (§4.3 shape (a)) reads base_uri back from connection metadata.

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
    # NET-NEW SOURCE: connection.metadata.base_uri absent from knownCredentialSources
    # (validate.go:53-59) → validateCredentials rejects it until added (§4.2; §5 growth item 3).
    base_uri: connection.metadata.base_uri
    account_key: connection.account_key

tool:
  name: adobe-sign          # axis ② (dashed anycli id); ②≠③ → toolToProvider entry required
  kind: oauth
```

**Capability growth needed (Helio side) — verified against this base, honest
accounting:**

1. **Compiled `service/adapter_adobesign.go` (exchange + identity only) + a
   net-new `model.RuntimeStrategy` (`adobe_sign_shard`)** — the §4.3
   documented-required path. `provider_registry.go` routes `standard_oauth` to
   the declarative `standardOAuthExchanger{}`; only a strategy in
   `composeExplicitOAuthRegistration`'s closed set reaches a compiled adapter.
   Growth: the enum value, a `runtimeStrategyContracts` entry (pinning
   provider=`adobe_sign` + the oauth/owner/mode/disconnect tuple), both
   `provider_registry.go` switches, and the adapter body. The adapter body covers
   **only the two facets the interface exposes** (`provider_adapter.go:15-27`):
   the shard-hosted code→token `Exchange` and the shard-hosted `GET /users/me`
   `Identity`; it also writes `base_uri` into connection metadata during exchange.
   Precedent mechanism: `adapter_slack.go` / `adapter_x.go`.
2. **Net-new refresh-path plumbing in `service/token_refresh.go`** — the
   part the adapter does **not** cover. Refresh is not strategy-dispatched:
   `requestOAuthRefresh` (`token_refresh.go:148`) receives only `provider, def,
   td` and refreshes against the fixed `def.OAuth.TokenURL` (line 153), never
   seeing the `Connection` or the adapter. To make refresh shard-aware, either
   **(a)** thread `conn` into `requestOAuthRefresh` and target
   `{connection.metadata.base_uri}oauth/v2/refresh` (smaller change; reuses the
   `base_uri` item-1 already persisted), or **(b)** add a per-strategy
   `refresher` facet to the adapter interface + `oauthProviderRuntime` +
   `composeExplicitOAuthRegistration` and dispatch to it from `refreshOAuthToken`.
   **Both the per-account host AND the distinct `/oauth/v2/refresh` path are
   net-new here** — the current fixed-`TokenURL` + `x/oauth2` grant-append
   produces neither. This is the single most-understated piece of prior drafts.
3. **`connection.metadata.base_uri` credential source** — absent from
   `knownCredentialSources` (`validate.go:53-59`); `validateCredentials` rejects
   it today. Required regardless — it is how anycli receives `base_uri`, and how
   the item-2 refresh change reads the shard host. The **sole unconditional
   bundle-side** growth.

**NOT growth (explicitly): `metadata_capture`.** Since the item-1 adapter is
mandatory and writes `base_uri` into connection metadata directly, a declarative
`connection.metadata_capture` field is redundant (two mechanisms for one job,
non-orthogonal — §4.2). Do **not** build the `connectionManifest.metadata_capture`
field for this tool; it is retained only as a future declarative alternative if
the adapter is ever removed.

**No `refresh_url`:** an earlier draft proposed a declarative `refresh_url` as
an "Option A." The official docs make exchange + refresh **shard-hosted**
(§4.3), which a fixed URL cannot express, so `refresh_url` is *not* the path —
the item-2 `token_refresh.go` change owns refresh instead.

**Genuinely reused / shipped (not net-new):** `form_secret` token exchange,
`pkce: none`, `local_only` disconnect, `isolated` connection mode, the generic
`authorize_url`, and the adapter **mechanism for exchange + identity ONLY**
(`tokenExchanger`/`identityResolver` + `composeExplicitOAuthRegistration`,
already used by slack/x/linkedin/discord). **The adapter mechanism does NOT
cover refresh** — that is net-new `token_refresh.go` work (item 2). **NOT
shipped — do not treat as such:** the net-new `adobe_sign_shard` runtime strategy
+ `adapter_adobesign.go` body, the `token_refresh.go` shard-aware refresh, and
the `connection.metadata.base_uri` credential source. **`standard_oauth` is NOT
usable here** (it forces the declarative exchanger).

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
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN` + `ANYCLI_CRED_BASE_URI` (the shard host) from a real Adobe Sign **developer** account; run `document upload`, `agreement send`, `agreement get`, `agreement list`, `library list` against the account's shard. Proves field names + the two-step send + request shapes match the live v6 API. The §4.3 host question is already settled by the docs (exchange + refresh are shard-hosted → compiled adapter); L2 optionally records whether a generic host *also* accepts exchange/refresh (a future-simplification note only, not a gate). | **Yes** — Adobe Acrobat Sign developer account (free dev tier) + a token minted from a registered app (lane 1). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (5 projections) green **only after the §4.2/§4.3 growth lands** (the `connection.metadata.base_uri` credential source + the net-new `adobe_sign_shard` runtime strategy & `adapter_adobesign.go` registration + the `token_refresh.go` shard-aware refresh change — without the credential source the bundle fails the allowlist, without the strategy it fails the runtime-strategy contract; NO `metadata_capture` field is added, the adapter writes `base_uri` directly); the new adapter + refresh-path unit tests green; helio-cli + integration-service suites green; `toolToProvider` resolver test covers `adobe-sign`→`adobe_sign`. On-branch: local `replace` to the anycli branch + local regen (not committed). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"adobe_sign"`, seeding `access_token` (+ short `expires_at`) **and** `refresh_token` **and** the `base_uri` connection metadata, then `heliox tool adobe-sign -- agreement list`. Exercises token gateway → the **`token_refresh.go` shard-aware refresh** (POST `{connection.metadata.base_uri}oauth/v2/refresh`, §4.3 item 2 — NOT the adapter; the adapter has no refresh facet) → metadata-injected `base_uri` → anycli path. Seed `base_uri` explicitly since L4 bypasses the connect-time capture the adapter would do. The short-`expires_at` forces the refresh path — the single best test of the §4.3 shard-hosted-refresh divergence, and the specific test that catches a refresh still pointed at the fixed placeholder `token_url`. | **Yes** — a real developer access + refresh token and the shard `base_uri` (lane 1 dev app). |
| **L5** full connect (pre-flip, once) | `heliox tool adobe-sign auth` → real Adobe consent on the dev/partner app → confirm `oauth_connected` event → confirm `api_access_point` captured as `base_uri` on the connection → run `agreement list` unseeded through the new connection. Human-in-the-loop (oauth L5). Gates the visible flip together with **partner certification clearance**. | **Yes** — human consent on a real Adobe Sign account (lane 3) + partner certification (lane 1 review) before the flip. |

**Layers needing externally supplied credentials:** L2, L4, L5 (an Adobe
Acrobat Sign developer account + the lane-1 partner/dev app; L5 additionally
needs a human consent session and partner certification before the visible
flip). L1 and L3 are fully self-contained. **L2 additionally validates the
compiled-adapter request shapes against the live shard, and optionally notes
whether a generic host also accepts exchange/refresh (future simplification
only — the shard-hosted adapter path is already fixed by the docs).**

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
  `agreement_read/write/send:self library_read:self user_login:self` (**no
  `offline_access`** — that is an Adobe *IMS* scope, not an Acrobat Sign native
  OAuth scope; Acrobat Sign returns `refresh_token` directly in the token
  response, and requesting `offline_access` would fail the exact-scope-match
  check. §4.1 records this as an official-docs divergence rebutting a review
  finding), 1-hour access token (`expires_in` present, no
  `assumed_ttl`), **non-rotating** refresh token with 60-day idle expiry
  (**`refresh_lease: none`, set explicitly — mandatory enum, not omittable**),
  `local_only` disconnect.
- **Three verified divergences from the shipped eSign tools, all Helio-side:**
  (1) `form_secret` not `form_basic`; (2) API base host is the per-account
  `api_access_point` shard → the mandatory adapter writes `base_uri` into
  connection metadata during exchange (no declarative `metadata_capture` field —
  that would be a redundant second mechanism), and the **only** net-new
  bundle-side capability is the **`connection.metadata.base_uri` credential
  source** (`validate.go:53-59`); (3) **both token exchange and refresh are
  shard-hosted on `api_access_point`** (exchange `POST /oauth/v2/token`, refresh
  `POST /oauth/v2/refresh` — both with `Host: api.na1.adobesign.com`), which
  `token_refresh.go:153`'s single fixed `token_url` cannot express. Exchange is
  fixed by a compiled `adapter_adobesign.go` behind a net-new `adobe_sign_shard`
  runtime strategy (NOT `standard_oauth`); **refresh is fixed by separate net-new
  plumbing in `token_refresh.go`** (the adapter interface has no refresh facet —
  the refresh path is not strategy-dispatched), reading the shard host from
  `connection.metadata.base_uri` to target `{base_uri}oauth/v2/refresh`.
- **OAuth host (§4.3), settled by the official docs:** token exchange **and**
  refresh are shard-hosted on `api_access_point`, which no fixed
  `token_url`/`refresh_url` can express. The required path is **two distinct
  net-new pieces, not one**: (i) a **compiled `adapter_adobesign.go` behind a
  net-new `adobe_sign_shard` runtime strategy** (NOT `standard_oauth`; that
  forces the declarative exchanger) owning the shard-aware **exchange** + the
  `GET /users/me` **identity** + writing `base_uri` to connection metadata; and
  (ii) **net-new refresh-path plumbing in `token_refresh.go`** — because refresh
  is not strategy-dispatched and the adapter interface (`provider_adapter.go:15-27`)
  has **no refresh facet**, the shard host and the `/oauth/v2/refresh` path are
  fixed by threading `connection.metadata.base_uri` into `requestOAuthRefresh`
  (or adding a refresher facet). There is **no declarative "Option A"** — a
  stage-1 probe for whether a generic host *also* accepts exchange/refresh is
  only a note for a possible future simplification, not a gate on v1.
- **oauth_review:** Adobe **PARTNER** (multi-tenant) app requires **partner
  certification** before production; self-serve dev/test app is immediate, so
  the gate is on the **visible flip only**, not dev/L4/merge. Lane correct
  (high confidence). Wave-board note: partner-certification tail, longer than
  DocuSign Go-Live but a normal partner program.
- **Open items for lane 1 / batch lead:** (a) stage-1 confirmation of the exact
  identity field from the shard-hosted `GET /users/me` (`userId` vs `email`) for
  the adapter's resolver — the token response carries no identity, and there is
  no Acrobat-Sign OIDC userinfo on the OAuth host (verified §5); (b) which Adobe
  data center the Helio partner app is registered on (app credentials are
  shard/account specific); (c) optional: a stage-1 note on whether a generic
  host *also* accepts exchange/refresh (future declarative simplification only —
  does not change the v1 adapter path).
