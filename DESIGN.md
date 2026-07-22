# DocuSign — per-tool design (`tool/docusign`)

Scratch design for the DocuSign integration under `heliox tool`, per the
`helio-tool-provider` pipeline and the 298-tool master rollout plan
(`docs/design/008-300-integrations-rollout-plan.md`).

- **Catalog row 43** — Product: DocuSign · anycli id `docusign` · provider key
  `docusign` · auth lane **oauth_review** · **Wave 1** · category Scheduling & eSign.
- **Three axes (all identical — no divergence):** ① CLI command word `docusign`
  · ② anycli id `docusign` · ③ provider key `docusign`. No `toolToProvider`
  entry, no grouped-family `tool.group`, no `tool.command`.
- **Go package:** `internal/tools/docusign/` (id has no dashes).

Every fact below was verified against DocuSign's official developer
documentation (Authorization Code Grant, `/oauth/userinfo`, eSignature REST API
v2.1, Go-Live). Where the official docs refine the catalog/audit assumption,
the divergence is recorded inline.

---

## 1. What an AI teammate does with DocuSign → which API surface

DocuSign's product is **electronic signature**: send documents out for
signature, track who has signed, and retrieve the completed/signed PDF. An AI
teammate acting for a user overwhelmingly wants to:

- **Send a document for signature** (create + send an *envelope*, from an
  uploaded document or a reusable *template*).
- **Check status** — "has the contract been signed?" — envelope status and
  per-recipient signing status.
- **List / search** recent envelopes by status and date ("what's out for
  signature", "what completed this week").
- **Retrieve the signed document** (download the combined completed PDF).
- **Void** an envelope that was sent in error or is no longer needed.
- **List templates** so a send can reference a reusable template by id.

That maps to exactly one DocuSign API: the **eSignature REST API v2.1**, whose
base path is account-specific (see §4). The wrapped resources are:

| Resource | Endpoint (relative to the account base path) | Verb used for |
|---|---|---|
| Envelopes | `POST /envelopes` | create + send (body `status:"sent"`) or create draft |
| Envelopes | `GET /envelopes?from_date=…&status=…` | list / search by status + date |
| Envelopes | `GET /envelopes/{envelopeId}` | one envelope's status |
| EnvelopeRecipients | `GET /envelopes/{envelopeId}/recipients` | per-recipient signing status |
| Envelopes | `PUT /envelopes/{envelopeId}` (`status:"voided"`, `voidedReason`) | void |
| EnvelopeDocuments | `GET /envelopes/{envelopeId}/documents/combined` | download signed PDF |
| Templates | `GET /templates`, `GET /templates/{templateId}` | list / inspect templates |

Plus one **auth-plane** call, `GET /oauth/userinfo`, which is both the identity
source and the account/base-path discovery source (§4).

**Deliberately out of scope for v1** (heavier, lower-frequency, or
compliance-sensitive; add later if demand warrants): DocuSign eSignature "tabs"
authoring beyond template use, bulk send, PowerForms, Connect webhooks, the
Admin API (user provisioning), Rooms, CLM, and Monitor. The v1 surface is the
"send / track / retrieve / void" loop an assistant actually drives.

---

## 2. Tool form decision — `service` type

**Decision: `service` type** (HTTP against the eSignature REST API), per the
skill's stage-1 rubric. DocuSign publishes SDKs (C#, Java, Node, Python, …) but
**no official, non-interactive, `--json`-capable CLI binary** that could be
provisioned into the runtime image, so the `cli`-type criteria fail. Implement
in `internal/tools/docusign/` against the HTTP API, copying the shape of the
`internal/tools/notion/` reference (cobra tree grouped by resource; a
`BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest` server;
exit-code contract 0 success / 1 runtime-or-API failure / 2 usage-or-parse; a
`--json` structured error envelope). This is the default path (21 of 23 shipped
definitions are `service`).

---

## 3. anycli definition + command surface

`definitions/tools/docusign.json` — `type: "service"`, `name: "docusign"`.

**Credential bindings (all injected as env by the resolver-supplied map):**

| field | inject | why |
|---|---|---|
| `access_token` | env `DOCUSIGN_ACCESS_TOKEN` | Bearer for every eSignature call |
| `base_uri` | env `DOCUSIGN_BASE_URI` | account's region host, e.g. `https://na3.docusign.net` (§4) |
| `account_id` | env `DOCUSIGN_ACCOUNT_ID` | API account GUID; part of the base path (§4) |

The service composes the account base path as
`{base_uri}/restapi/v2.1/accounts/{account_id}` and sends
`Authorization: Bearer {access_token}`. `base_uri` and `account_id` are supplied
by the Helio resolver (they were captured at connect time — §4); anycli never
calls `/oauth/userinfo` itself and stays credential-agnostic, exactly as the
embeddable-core contract requires.

**Command tree** (grouped by resource, mirroring notion):

```
docusign envelope send                # POST /envelopes  (--template-id | --document <pdf>, --signer-email, --signer-name, --subject)
docusign envelope list                # GET /envelopes   (--from-date, --status, --count)
docusign envelope get     <id>        # GET /envelopes/{id}
docusign envelope recipients <id>     # GET /envelopes/{id}/recipients
docusign envelope void    <id>        # PUT /envelopes/{id}  status=voided (--reason)
docusign envelope download <id>       # GET /envelopes/{id}/documents/combined  (--out <path>)
docusign template list                # GET /templates
docusign template get     <id>        # GET /templates/{id}
```

**No `whoami` command in v1 — and therefore no `DOCUSIGN_AUTH_HOST` field.**
Account and base-path discovery happens entirely **Helio-side at connect time**
(§4.2 Option A): the connect flow already calls `/oauth/userinfo` for identity,
and the same call captures `base_uri` + `account_id` onto the connection. The
anycli service only ever receives the three sourced fields above and never talks
to the auth host. A runtime `whoami` (calling `/oauth/userinfo` from the service
to let an assistant enumerate accounts and pick a non-default one) is
**deferred**: `/oauth/userinfo` lives on the auth host (`account.docusign.com` /
`account-d.docusign.com`), which is a per-environment config axis, not a token
field or connection-metadata value — so no `credential.fields` sourcing entry
can supply it (§4 credential-bindings table has no such source). Adding `whoami`
later requires first defining how a bundle/config-derived static auth host
reaches the service (distinct from the token-gateway sourcing map); it is not in
scope for v1 and nothing in the v1 loop depends on it.

**JSON output shape** — provider-neutral, `--json` flips every command to a
structured envelope. Illustrative:

- `envelope list --json` →
  `{"envelopes":[{"id":"…","status":"sent","subject":"…","sent_at":"…","recipients":3}], "count": 12}`
- `envelope get <id> --json` →
  `{"id":"…","status":"completed","subject":"…","created_at":"…","completed_at":"…"}`
- `envelope recipients <id> --json` →
  `{"recipients":[{"name":"…","email":"…","status":"completed","signed_at":"…","routing_order":1}]}`
- `envelope send --json` → `{"envelope_id":"…","status":"sent","uri":"…"}`
- errors → `{"error":{"code":"api_error|usage","message":"…","status":409}}` with
  the matching non-zero exit code.

Default (non-`--json`) output is a one-line-per-row human summary. Field names
are provider-neutral snake_case, not DocuSign's raw camelCase, so output reads
consistently with the other 300 tools.

---

## 4. Credential fields + the exact OAuth flow (oauth_review lane verified)

### 4.1 Flow: Authorization Code Grant (confidential client)

DocuSign's user-present multi-tenant flow is the **Authorization Code Grant**
(ACG). Helio is a confidential server-side client (holds the client secret), so
it uses the standard ACG, **not** the Public ACG (that variant is for public
clients and uses PKCE; we don't need it) and **not** JWT Grant (that is the
unattended service-integration path with no user consent — wrong ownership
model for an assistant acting on a specific user's behalf).

Verified endpoints (production; demo host in parentheses):

- **authorize:** `https://account.docusign.com/oauth/auth`
  (`https://account-d.docusign.com/oauth/auth`)
- **token:** `https://account.docusign.com/oauth/token`
  (`https://account-d.docusign.com/oauth/token`)
- **userinfo:** `https://account.docusign.com/oauth/userinfo`
  (`https://account-d.docusign.com/oauth/userinfo`)

**Token endpoint client auth:** HTTP **Basic**, `Authorization: Basic
base64(integration_key ":" secret_key)`, form body `grant_type` + `code` +
`redirect_uri`. → bundle `token_exchange_style: form_basic` (the SignNow
precedent). **PKCE: none** (confidential client).

**Scopes:** `signature extended`.
- `signature` grants the eSignature REST API.
- `extended` is what makes long-lived operation possible: DocuSign access tokens
  live **8 hours**; refresh tokens live **30 days**; and DocuSign issues a **new
  refresh token on every refresh**. With `extended`, each refresh resets the new
  refresh token's life to a full 30 days, so a periodically-used connection
  never expires. `extended` **can only be requested on the initial
  authorization-code exchange**, never on a refresh exchange — the bundle
  requests it once at authorize time, which is exactly what `standard_oauth`
  does. (No `openid` scope is needed; `/oauth/userinfo` works with the
  `signature` token and returns `sub`.)

**Rotating refresh tokens → `refresh_lease: credential`.** Because every refresh
mints a new refresh token, two concurrent refreshes on the same credential can
break the chain and force re-auth. This is the identical hazard SignNow
documented; the fix is the same — serialize refresh per credential via
`OAuthLeaseCredential`. `refresh_lease: credential` under `runtime_strategy:
standard_oauth` is **already in the runtime-contract allowed set** (SignNow's
§4a growth, task landed on main), so DocuSign needs **no** new capability for
this.

**Token TTL:** the token response carries `expires_in: 28800`, so no
`assumed_ttl_seconds` is needed (unlike Salesforce, which returns no expiry).

**Revoke / disconnect:** DocuSign has no plain declarative user-token revoke
endpoint matching the `standard_oauth` revoker shape → `disconnect_mode:
local_only` (the SignNow precedent). Helio forgets the credential; the user can
revoke app access from their DocuSign account.

### 4.2 Account + base-path discovery — the one real design decision

This is where DocuSign differs from every eSign tool shipped so far and from the
Salesforce precedent, so it is called out explicitly.

The eSignature API base path is **account-specific**:
`{base_uri}/restapi/v2.1/accounts/{account_id}`. A DocuSign user can belong to
multiple accounts, each in a **different region** (`na3`, `eu`, `au`, `ca`, …),
so `base_uri` is not a constant. `base_uri` and `account_id` come **only** from
`GET /oauth/userinfo`, which returns:

```json
{ "sub": "…user guid…", "name": "…", "email": "…",
  "accounts": [ { "account_id": "…", "is_default": true,
                  "account_name": "…", "base_uri": "https://na3.docusign.net" }, … ] }
```

The default account is the element **where `is_default == true`** (not
necessarily `accounts[0]`).

**Contrast with Salesforce (`tool/salesforce`):** Salesforce's `instance_url`
is captured with `metadata_capture: { instance_url: /instance_url }` — a static
RFC-6901 JSON Pointer into the **token response**. That does **not** transfer to
DocuSign for two reasons: (a) `base_uri`/`account_id` are in the **userinfo**
response, not the token response; (b) selecting the default account is
"the array element whose `is_default` is true", which a static JSON Pointer
cannot express.

**Recommended — Option A (Helio-side capture, thin anycli):** capture
`base_uri` + `account_id` from the default account **during the identity
resolution the connect flow already performs** (identity `source: userinfo` is
required anyway for `sub`), persist them as connection metadata, and inject both
as credential fields so anycli receives an already-account-scoped base plus an
`access_token` and stays a pure executor.

**This is real, net-new capability cost — not reuse of shipped machinery.** As
sketched, the bundle **cannot generate on `main` today**. Three distinct
integration-service items are required, none present on `main`:

1. **A `connection.metadata_capture` manifest field that accepts a deriver
   source.** `connectionManifest` (`cmd/provider-gen/manifest.go:109-113`) has
   only `mode` / `disconnect_mode` / `runtime_strategy`, and the decoder runs
   `decoder.KnownFields(true)` (`manifest.go:203-204`) — so an unknown
   `metadata_capture:` key is a **strict-decode failure**, not an ignored
   extra. The YAML field, its Go struct member, and the connect-time projection
   that runs the deriver all have to be added.
2. **Two new credential sources** — `connection.metadata.base_uri` and
   `connection.metadata.account_id` (or one generalized `connection.metadata.*`
   source). `knownCredentialSources` (`cmd/provider-gen/validate.go:53-59`) is
   today exactly `{token.access_token, connection.account_key,
   connection.metadata.person_urn, credential.app_id, credential.brand}`, and
   `validateCredentials` (`validate.go:545-562`) rejects any other
   `credential.fields` source as "unsafe" — so the sketched `base_uri` /
   `account_id` sources fail the allowlist until added.
3. **The deriver itself** — a DocuSign userinfo metadata deriver that, given the
   `/oauth/userinfo` response, selects the `is_default` account and emits
   `base_uri` + `account_id`.

**The cited precedents do not cover this (verified against `main`):**

- **Salesforce `metadata_capture` is itself unmerged** — a separate in-flight
  branch, absent from `main` — and even there it is a **static RFC-6901 pointer
  into the token response** (`instance_url: /instance_url`), *not* a userinfo
  deriver. It establishes neither the deriver-sourced `metadata_capture` field
  nor the new credential sources DocuSign needs.
- **The `dsnHostIdentityDeriver` family** (crisp / amplitude / iterable /
  mailerlite / braze) runs on the **`manual_credentials`** path
  (`service/manual_credentials_identity.go` — a `Verify` deriving
  identity/`account_key`/label from a pasted secret). Its output feeds
  `account_key` + the human label, **not** an injected OAuth `credential.fields`
  value on a `standard_oauth` connection. Different path, different output slot.
- **The only OAuth-path precedent for injecting a derived value as a credential
  — LinkedIn `person_urn` — is produced by a COMPILED adapter**
  (`service/adapter_linkedin.go:34` sets `identity["person_urn"]`; the token
  gateway then projects the already-shipped `connection.metadata.person_urn`
  source). That is exactly the compiled-adapter path this design otherwise rules
  out — so "no compiled adapter" and "reuses shipped capabilities" cannot both
  be true.

**So the honest choice is one of two, not "reuse shipped capabilities":**

- **(a) Declarative machinery (recommended for reuse):** add the
  `metadata_capture`-with-deriver field, the two `connection.metadata.*`
  credential sources, and the deriver — justified on its own terms as the
  general "capture-a-value-from-userinfo-onto-the-connection" capability that
  freshdesk-class instance-scoped providers will reuse. Bundle stays
  `standard_oauth`, no `adapter_*.go`.
- **(b) Narrow compiled adapter (LinkedIn precedent, likely shorter):** a
  DocuSign adapter whose `Identity` selects the `is_default` account and
  populates `base_uri` / `account_id` into connection metadata, projected
  through the (still net-new) `connection.metadata.*` credential source(s). This
  reuses the existing adapter seam and drops item 1, but makes the bundle
  non-pure-declarative.

This design **recommends (a)**, but records (b) explicitly so the Helio-side
implementer chooses deliberately rather than discovering the
strict-decode/allowlist failures at generation time.

**Rejected — Option B (anycli self-discovery):** let the docusign service call
`/oauth/userinfo` itself at runtime from just `access_token`. Rejected because
the service would then need the auth host injected anyway (demo vs prod is a
Helio-config axis, not derivable from the token), it would re-fetch userinfo on
every command against DocuSign's per-user hourly userinfo rate limit, and it
pushes account-selection policy into the executor — against the embeddable-core
boundary. Option A captures once at connect time and caches on the connection.

Option A captures once at connect time and caches on the connection, so the
anycli service needs **no** auth-host value and makes **no** runtime
`/oauth/userinfo` call. A future runtime account-enumeration/`--account-id`
selection aid (a `whoami`-style command) is deferred precisely because it would
reintroduce an auth-host dependency the sourcing map cannot express (§3); it is
out of scope for v1.

### 4.3 oauth_review lane — verified, with a divergence noted

The catalog lanes DocuSign **oauth_review**; the audit did not re-examine it
(the audit only covered tools that sat in `api_key`). Verified against the
official Go-Live docs:

- ACG app creation and the demo/developer environment are **fully self-serve**
  — dev-mode client id/secret exist immediately, so **dev, L1–L4, and the
  batch-end merge are not gated** (hidden-first, per the master plan).
- The **production integration key requires a Go-Live promotion review** before
  a demo app can be promoted to production and used against real accounts. This
  is the review-clearance gate that oauth_review captures — it gates only the
  **visible flip**, exactly as the plan states.
- **Divergence to record in DESIGN/wave-board:** DocuSign **streamlined** Go-Live
  in Oct 2025 — the legacy "20 successful API calls" prerequisite is removed for
  the new Apps & Keys experience; promotion now runs built-in validations with
  outcomes *approved instantly* / *needs review (≈48h)* / *not allowed*. The
  oauth_review classification stays correct (a review gate before production
  exists and can require human follow-up), but the tail is typically far
  shorter than the Google/Meta/Microsoft-class weeks — lane 1 should expect
  hours-to-48h, not weeks, for DocuSign. Confidence: high (official Go-Live +
  ACG references).

### 4.4 `required_config_fields`

`[oauth.client_id, oauth.client_secret]` — the integration key and secret key.
Supplied per environment through integration-service config (`config/` locally +
the Helm Secret in `deploy/`, kept in sync per the Config Sync hard rule); never
in the bundle. A fully-absent config renders `configured: false` (Connect
disabled, safe to ship hidden); a *partial* config fails service startup — so id
and secret land together (lane 1).

---

## 5. Helio provider bundle plan (`integrations/providers/docusign/provider.yaml`, hidden-first)

Axis ①/②/③ all `docusign` (no divergence). Bundle sketch (final field spellings
confirmed against `provider-yaml.md` + `provider-gen --check` at build time):

```yaml
schema: helio.provider/v1
key: docusign
go_name: Docusign

presentation:
  name: DocuSign
  description_key: docusign
  consent_domain: docusign.com
  visible: false          # hidden-first; visible flip gated on L5 + Go-Live review clearance
  order: 0                # batch lead assigns at go-live

auth:
  type: oauth
  owner: individual        # consent authenticates a DocuSign user; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://account.docusign.com/oauth/auth
    token_url: https://account.docusign.com/oauth/token
    token_exchange_style: form_basic     # Basic client auth + form body (SignNow precedent)
    pkce: none                           # confidential client
    scopes: [signature, extended]
    display_scopes: [signature, extended]
    single_active_token: false
    refresh_lease: credential            # rotating refresh tokens; serialize per credential (SignNow §4a set)

identity:
  source: userinfo
  url: https://account.docusign.com/oauth/userinfo
  stable_key: /sub
  label_candidates: [/email, /name, /sub]

connection:
  mode: isolated
  disconnect_mode: local_only            # no declarative user-token revoke endpoint
  runtime_strategy: standard_oauth
  # base_uri + account_id captured from the DEFAULT userinfo account at connect
  # time via the docusign userinfo metadata deriver (§4.2 Option A).
  # NET-NEW FIELD: connectionManifest has no metadata_capture and the generator
  # decodes with KnownFields(true) — this key fails strict-decode until the field
  # is added (§4.2 item 1). Assumes Option A(a); Option A(b) drops this block and
  # populates the same metadata from a compiled adapter instead.
  metadata_capture:
    base_uri: docusign_default_account   # deriver output (not a static pointer)
    account_id: docusign_default_account

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
    # NET-NEW SOURCES: connection.metadata.base_uri / .account_id are absent from
    # knownCredentialSources (validate.go:53-59), so validateCredentials rejects
    # them as "unsafe" until added — either as two entries or one generalized
    # connection.metadata.* source (§4.2 item 2).
    base_uri: connection.metadata.base_uri
    account_id: connection.metadata.account_id
    account_key: connection.account_key

tool:
  name: docusign
  kind: oauth
```

- **Host axis (demo vs prod):** the `authorize_url` / `token_url` /
  `identity.url` above are the **production** hosts. The dev/hidden phase runs
  against a demo app; whether that is handled by a second bundle-config value or
  by pointing config at `account-d.docusign.com` during the hidden phase is a
  batch-lead/lane-1 decision at app-creation time (the client id/secret are
  themselves environment-specific). Recorded here as an open item, not silently
  assumed.
- **Capability growth needed (Helio side) — three net-new items, not one, and
  not "shipped" (see §4.2):** (1) a `connection.metadata_capture` manifest field
  that accepts a deriver source — it is **not** on `connectionManifest`
  (`manifest.go:109-113`) and `KnownFields(true)` strict-decode rejects the
  sketch as written; (2) the credential sources
  `connection.metadata.base_uri` / `connection.metadata.account_id` (or one
  generalized `connection.metadata.*`) — absent from `knownCredentialSources`
  (`validate.go:53-59`), so `validateCredentials` rejects them today; (3) the
  userinfo → `is_default` deriver itself. **Genuinely shipped and reused:**
  `form_basic`, `refresh_lease: credential` (SignNow §4a set), userinfo
  identity, and `local_only` disconnect. The `metadata_capture` field and the
  injected metadata credential fields are **not** shipped — do not treat them as
  such. **Open decision carried from §4.2:** build this as declarative machinery
  — Option A(a), recommended — vs. a narrow compiled adapter à la LinkedIn
  `person_urn` — Option A(b), likely shorter but non-declarative. The bundle
  sketch above assumes A(a); either way the two `connection.metadata.*`
  credential sources are net-new.
- **Shared surfaces** touched at batch end (per master plan §2): anycli
  `register.go` + pin bump, `provider-gen` (5 projections), `providerIcons.ts`
  append (`ui/helio-app/src/integrations/icons/docusign.svg`), plugin docs
  publish. **No `toolToProvider` entry** (axes ② == ③). No `tool.group`.
- **AI-facing docs:** a `docusign` sub-doc under
  `agents/plugins/heliox/skills/tool/` describing the send / track / retrieve /
  void loop and the default-account behavior.

---

## 6. Test plan — five layers

| Layer | DocuSign specifics | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fake of the eSignature API: assert base path `{base_uri}/restapi/v2.1/accounts/{account_id}`, `Authorization: Bearer` header, envelope send/list/get/recipients/void/download request shapes, template list/get, and both plain + `--json` error rendering (401/409). The `is_default`-account selection is a **Helio-side** deriver concern (§4.2), unit-tested on that side, not in anycli. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN` + `ANYCLI_CRED_BASE_URI` + `ANYCLI_CRED_ACCOUNT_ID` from a DocuSign **developer (demo)** account; run `envelope list`, `envelope send` to a template, `envelope get`, `template list` against `account-d` / `demo.docusign.net`. Proves field names + request shapes match the live API. No auth-host credential — v1 has no `whoami`. | **Yes** — DocuSign demo account (free); token minted from the demo app (lane 1). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (5 projections) green **only after the §4.2 growth lands** (the new `metadata_capture` field + `connection.metadata.*` credential sources — without them the bundle fails strict-decode / the credential-source allowlist); the new userinfo-capture unit test green (deriver for A(a) or adapter `Identity` for A(b)); helio-cli + integration-service suites green. On-branch: local `replace` to the anycli branch + local regen (not committed). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"docusign"`, seeding `access_token` (+ short `expires_at`) **and** the captured `base_uri`/`account_id` metadata + `refresh_token`, then `heliox tool docusign -- envelope list`. Exercises the token gateway → deriver-populated metadata → anycli path. Seed the metadata explicitly since L4 bypasses the connect-time deriver. | **Yes** — a real demo access token + its refresh token, base_uri, account_id (lane 1 dev app). |
| **L5** full connect (pre-flip, once) | `heliox tool docusign auth` → real DocuSign consent on the demo app → confirm `oauth_connected` event → confirm the userinfo deriver captured `base_uri`/`account_id` on the connection → run `envelope list` unseeded through the new connection. Human-in-the-loop (oauth L5). Gates the visible flip together with **Go-Live review clearance**. | **Yes** — human consent on a real DocuSign account (lane 3 + lane 1 review). |

**Layers needing externally supplied credentials:** L2, L4, L5 (a DocuSign
developer/demo account + the lane-1 dev app; L5 additionally needs a human
consent session and the production Go-Live review before the visible flip). L1
and L3 are fully self-contained.

---

## 7. Summary of decisions & required growth

- **Type:** `service`; package `internal/tools/docusign/`; id/key/command all
  `docusign` (no `toolToProvider`, no group).
- **API:** eSignature REST API v2.1 — envelopes (send/list/get/recipients/void/
  download), templates (list/get). `/oauth/userinfo` is called **Helio-side at
  connect time** for identity + base-path capture, not by anycli at runtime; v1
  has no `whoami`, so anycli receives no auth-host value.
- **Auth:** Authorization Code Grant, confidential client, `form_basic` token
  exchange, scopes `signature extended`, `refresh_lease: credential` (rotating
  refresh; reuses SignNow's shipped allowed-set), `local_only` disconnect,
  8h access token (no `assumed_ttl`). oauth_review = Go-Live production
  promotion review (streamlined Oct 2025 → hours-to-48h, not weeks); gates only
  the visible flip.
- **New Helio capability — three net-new items (§4.2/§5), not "shipped":** (1) a
  `connection.metadata_capture` manifest field accepting a deriver source
  (`connectionManifest` lacks it; `KnownFields(true)` rejects it today); (2) the
  `connection.metadata.base_uri` / `connection.metadata.account_id` credential
  sources (or a generalized `connection.metadata.*`), absent from
  `knownCredentialSources`; (3) the userinfo → `is_default` deriver. **Open
  build choice:** declarative machinery (Option A(a), recommended) vs. a narrow
  compiled DocuSign adapter à la LinkedIn `person_urn` (Option A(b), shorter but
  non-declarative). "No adapter" and "reuses shipped capabilities" are **not**
  both true — the earlier draft's claim is corrected here.
- **Open item for lane 1 / batch lead:** demo-vs-prod host handling during the
  hidden phase (env-specific config vs `account-d` hosts).

---

## 8. Implementation decisions & divergences from this design (recorded at build)

Verified against the **actual `origin/main` code** in both worktrees before
implementing. Two premises in §4.2/§4.3 above did not hold on this base and were
corrected:

- **"`refresh_lease: credential` under `standard_oauth` already landed on main
  (SignNow §4a)" — FALSE on this base.** `runtimeStrategyContracts` still pins
  `standard_oauth` to a **single** `refreshLeaseScope: OAuthLeaseNone` with an
  exact-equality check (`model/runtime_contract.go`), and SignNow's bundle is
  not present. So `refresh_lease: credential` was **not** available to a
  `standard_oauth` DocuSign bundle. (The `OAuthLeaseCredential` *behavior* —
  per-credential refresh serialization — is implemented in `token_refresh.go`;
  only the contract *permission* was missing.)
- **`metadata_capture` field + `connection.metadata.*` credential sources —
  confirmed absent** exactly as §4.2 predicted (`knownCredentialSources`,
  `KnownFields(true)` strict-decode).

**Chosen build: Option A(b) realized as a named `docusign_acg` runtime
strategy** (not A(a)'s declarative `metadata_capture` field, and not
`standard_oauth`). Rationale — minimal orthogonal decomposition on the verified
base:

1. A named strategy carries its **own** capability tuple, so it sets
   `refreshLeaseScope: OAuthLeaseCredential` **without widening the shared
   `standard_oauth` contract** — the cleaner fix than making the golden path's
   lease a set (which the false "SignNow landed" premise assumed already done).
2. The `is_default`-account selection is genuinely compiled logic (array filter,
   not a static pointer — the AGENTS.md forbids unbounded YAML expressions), so
   it must be Go either way. Wiring it through the **shipped LinkedIn
   `person_urn` adapter seam** (`composeExplicitOAuthRegistration` +
   `docusignAdapter`) reuses existing machinery and drops A(a)'s whole net-new
   `metadata_capture`-with-deriver subsystem (manifest field + strict-decode
   plumbing + deriver-selection). Subtract before adding; the reusable manifest
   field had **zero** other consumers on this base (YAGNI).
3. Net-new items actually landed (all reused/extended existing seams, no new
   declarative subsystem): the `docusign_acg` strategy + contract entry; the two
   `connection.metadata.base_uri` / `.account_id` credential sources (generator
   allowlist + runtime allowlist + token-gateway projection); the
   `docusignAdapter` identity resolver; and a generalized
   `promotedIdentityMetadataKeys` promotion in the callback (subsumes the
   person_urn special-case).

**Efficiency note vs §4.2 sketch:** the adapter makes **no second userinfo
fetch** — the declarative resolver already returns the full userinfo map as the
identity blob, so `accounts[]` is read from it directly (one GET at connect).

**Bootstrap note:** the `docusign_acg` contract entry intentionally leaves the
reserved-`provider` binding **unbound**. Binding `model.ProviderDocusign` (a
generated const) in the hand-written contract would deadlock the generator (the
model package it depends on could not compile before the const it emits exists).
The strategy is single-provider by construction, so the guard adds no safety a
review would not already catch.

**Not done (correctly, per hidden-first):** no integration-service `config/` +
`deploy/` client-id/secret entries — DocuSign has no dev app credentials yet, a
**fully-absent** config renders `configured:false` (safe hidden), and a
*partial* config fails startup, so id+secret land together via lane 1 before L5.
No plugin version bump / marketplace publish — that rides the batch-end merge
(one publish per batch), per master plan §2. The 5 provider-gen projections were
regenerated locally for L3/--check validation but **not committed** (batch lead
owns the one canonical regen); the helio-cli build used a local uncommitted
`replace` against the anycli worktree.
