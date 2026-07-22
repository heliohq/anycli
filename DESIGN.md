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
docusign whoami                       # GET /oauth/userinfo — accounts + base_uri (debug/selection aid)
docusign envelope send                # POST /envelopes  (--template-id | --document <pdf>, --signer-email, --signer-name, --subject)
docusign envelope list                # GET /envelopes   (--from-date, --status, --count)
docusign envelope get     <id>        # GET /envelopes/{id}
docusign envelope recipients <id>     # GET /envelopes/{id}/recipients
docusign envelope void    <id>        # PUT /envelopes/{id}  status=voided (--reason)
docusign envelope download <id>       # GET /envelopes/{id}/documents/combined  (--out <path>)
docusign template list                # GET /templates
docusign template get     <id>        # GET /templates/{id}
```

Note `docusign whoami` reads `/oauth/userinfo`, which lives on the **auth host**
(`account.docusign.com` / `account-d.docusign.com`), not on `base_uri`. The
service therefore needs the auth host too for `whoami` only; it is injected as a
fourth field `DOCUSIGN_AUTH_HOST` (production `account.docusign.com` by default,
overridden per environment — see §4/§6). The signing commands never touch the
auth host.

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
required anyway for `sub`), and inject both as credential fields — the same
end-shape as Salesforce (`credential.fields` carrying an account-scoped base and
an injected `access_token`), just sourced from userinfo. Because default-account
selection exceeds a static pointer, this needs a **small reviewed capability**:
a DocuSign userinfo metadata deriver that, given the `/oauth/userinfo` response,
picks the `is_default` account and emits `base_uri` + `account_id`. This is the
same class of narrow, per-provider deriver the pipeline has already added many
times (crisp keypair, amplitude first-colon-split, iterable region-prefix,
mailerlite, braze DSN-host, …) and keeps anycli a pure executor. **This is the
proposed integration-service capability growth for this tool** (Helio-side,
`service/` — the bundle stays `standard_oauth`, no compiled `adapter_*.go`).

**Rejected — Option B (anycli self-discovery):** let the docusign service call
`/oauth/userinfo` itself at runtime from just `access_token`. Rejected because
the service would then need the auth host injected anyway (demo vs prod is a
Helio-config axis, not derivable from the token), it would re-fetch userinfo on
every command against DocuSign's per-user hourly userinfo rate limit, and it
pushes account-selection policy into the executor — against the embeddable-core
boundary. Option A captures once at connect time and caches on the connection.

`whoami` still exposes `/oauth/userinfo` at runtime as a **debug/selection
aid** (so an assistant can see all accounts and pick a non-default one via a
future `--account-id`), which is why the service also accepts the injected
`DOCUSIGN_AUTH_HOST`; it does not depend on that call for normal operation.

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
  # time via the docusign userinfo metadata deriver (§4.2 Option A capability).
  metadata_capture:
    base_uri: docusign_default_account   # deriver output (not a static pointer)
    account_id: docusign_default_account

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
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
- **Capability growth needed (Helio side):** exactly one — the DocuSign userinfo
  metadata deriver (§4.2). No compiled `service/adapter_*.go`; the bundle stays
  `standard_oauth`. Everything else (`form_basic`, `refresh_lease: credential`,
  userinfo identity, `local_only` disconnect, `metadata_capture` + injected
  credential fields) reuses existing, shipped capabilities.
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
| **L1** anycli unit | `httptest` fake of the eSignature API: assert base path `{base_uri}/restapi/v2.1/accounts/{account_id}`, `Authorization: Bearer` header, envelope send/list/get/recipients/void/download request shapes, template list/get, and both plain + `--json` error rendering (401/409). Fake `/oauth/userinfo` for `whoami` and for the Option-A deriver's default-account selection (`is_default` true not at index 0). | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN` + `ANYCLI_CRED_BASE_URI` + `ANYCLI_CRED_ACCOUNT_ID` (+ `ANYCLI_CRED_AUTH_HOST` for `whoami`) from a DocuSign **developer (demo)** account; run `envelope list`, `envelope send` to a template, `envelope get`, `template list` against `account-d` / `demo.docusign.net`. Proves field names + request shapes match the live API. | **Yes** — DocuSign demo account (free); token minted from the demo app (lane 1). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (5 projections) green; the new userinfo-deriver unit test green; helio-cli + integration-service suites green. On-branch: local `replace` to the anycli branch + local regen (not committed). | No |
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
  download), templates (list/get), plus `/oauth/userinfo` for identity + base
  path.
- **Auth:** Authorization Code Grant, confidential client, `form_basic` token
  exchange, scopes `signature extended`, `refresh_lease: credential` (rotating
  refresh; reuses SignNow's shipped allowed-set), `local_only` disconnect,
  8h access token (no `assumed_ttl`). oauth_review = Go-Live production
  promotion review (streamlined Oct 2025 → hours-to-48h, not weeks); gates only
  the visible flip.
- **Only new Helio capability:** a DocuSign userinfo **metadata deriver** that
  selects the `is_default` account and emits `base_uri` + `account_id`
  (§4.2 Option A). No compiled adapter; bundle stays `standard_oauth`.
- **Open item for lane 1 / batch lead:** demo-vs-prod host handling during the
  hidden phase (env-specific config vs `account-d` hosts).
