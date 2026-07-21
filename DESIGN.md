# Tool design: BoldSign (`boldsign`)

**Catalog row:** #221 · anycli id `boldsign` · provider key `boldsign` · auth lane `oauth_light` · wave 3 · Scheduling & eSign.
**Branches:** anycli `tool/boldsign` (this worktree), Helio `tool/boldsign` (`2helio/.claude/worktrees/tool-boldsign`).
**Status of this doc:** per-tool design scratch file (master plan §2); the batch lead strips it at batch end.

## 1. Verification against official docs (independent, not inherited)

Sources verified directly:

- OAuth guide: https://developers.boldsign.com/authentication/oauth-2-0/
- OIDC discovery (authoritative endpoint/scope list): https://account.boldsign.com/.well-known/openid-configuration
- Send document: https://developers.boldsign.com/documents/send-document
- List documents: https://developers.boldsign.com/documents/list-documents
- Revoke: https://developers.boldsign.com/documents/revoke-document · Remind: https://developers.boldsign.com/documents/send-reminder
- Send from template / list templates: https://developers.boldsign.com/documents/send-document-from-template/ , https://developers.boldsign.com/template/list-templates/
- PHP SDK endpoint inventory (cross-check of paths): https://github.com/boldsign/boldsign-php-sdk

**Auth lane verdict: `oauth_light` CONFIRMED.** OAuth app creation is self-serve in the BoldSign web app (API → OAuth Apps → Create App; account admin required, API access must be in the plan). No review, approval, or marketplace step exists. Authorization-code grant with **mandatory PKCE** (`plain` and `S256` both supported), user consent, refresh tokens via `offline_access`. This matches the audit verdict; no divergence on the lane.

**Divergences / additions vs the audit row:**

1. The audit note implies identity comes only from the `id_token`; the OIDC discovery document shows a real **userinfo endpoint**: `https://account.boldsign.com/connect/userinfo`. We use it for identity resolution (standard `identity.source: userinfo`), so no id_token parsing is needed.
2. Discovery also exposes an RFC 7009 **revocation endpoint** `https://account.boldsign.com/connect/revocation` (not mentioned in the audit). We use it for `disconnect_mode: provider_revoke` — no adapter needed.
3. **Refresh tokens are single-use (rotating)**: each refresh returns a new refresh token and invalidates the old one; lifetime 30 days absolute (default, configurable to sliding per app). This forces `refresh_lease: credential` (serialize refreshes per credential) — concurrent refreshes racing on a rotated token would strand the connection. Set the dev app to **sliding** expiration so long-lived assistant connections survive past 30 days.

Key OAuth facts (all from official docs/discovery):

| Item | Value |
|---|---|
| Authorize URL | `https://account.boldsign.com/connect/authorize` |
| Token URL | `https://account.boldsign.com/connect/token` (form-encoded POST) |
| PKCE | required; S256 supported → use `s256` |
| Client auth at token endpoint | `client_secret_post` and `client_secret_basic` both supported → `form_secret` |
| Refresh token | `offline_access` scope; single-use rotating; 30d absolute (default) or sliding |
| Access token TTL | 3600 s, `Bearer` |
| Userinfo | `https://account.boldsign.com/connect/userinfo` (OIDC: `sub`, `email`, `name`) |
| Revocation | `https://account.boldsign.com/connect/revocation` (client-authenticated) |
| API base | `https://api.boldsign.com` — accepts `Authorization: Bearer <access_token>` (or `X-API-KEY` for key auth, which we do NOT use) |
| Redirect URIs | pre-registered per app; localhost and HTTPS both allowed |
| Sandbox | free developer sandbox account; OAuth apps carry separate sandbox vs production credentials |

Notes: (a) BoldSign docs show a region selector (US/EU); we ship the US base `api.boldsign.com` — a EU-region account is out of scope for this pass and would surface as an API-side 401/404, recorded here as a known limitation. (b) Send is asynchronous: `POST /v1/document/send` returns a `documentId` before the document is fully processed; the CLI documents this in help text rather than polling.

## 2. What an AI teammate does with BoldSign → API surface wrapped

An assistant's real e-sign jobs: send a contract for signature (from a file or a reusable template), check where a signature request stands, chase pending signers, cancel a request, and retrieve the signed artifact + audit trail. That maps to the Documents and Templates groups only — Users/Teams/Contacts/Branding administration is org-admin work, not teammate work, and is deliberately out of scope for v1.

Wrapped endpoints (all verified against official docs / official SDK inventory):

| Endpoint | Why |
|---|---|
| `POST /v1/document/send` | send files for signature (JSON body, base64 file entries or `FileUrls`) |
| `POST /v1/template/send?templateId=` | send from a reusable template with role→signer mapping (the most common recurring flow) |
| `GET /v1/document/list` | find/monitor requests (`page`, `pageSize`, `status`, `searchKey`, `transmitType`) |
| `GET /v1/document/properties?documentId=` | status detail per signer |
| `GET /v1/document/download?documentId=` | retrieve the (signed) document PDF |
| `GET /v1/document/downloadAuditLog?documentId=` | retrieve the audit trail PDF |
| `POST /v1/document/remind?documentId=` | nudge pending signers (optional `receiverEmails`, body `Message`) |
| `POST /v1/document/revoke?documentId=` | cancel with required reason (`Message`), 204 |
| `GET /v1/template/list`, `GET /v1/template/properties?templateId=` | discover templates and their roles before `template send` |

Explicitly NOT wrapped in v1: embedded signing/requesting links, prefill/extend-expiry/remove-auth PATCH calls, users/teams/contacts/brands, webhooks. Add later if usage demands.

## 3. anycli definition

**Stage-1 rubric: `service` type.** BoldSign ships no official CLI binary at all, so the `cli`-type conditions fail at the first test. Service implementation against the HTTP API, like 21 of 23 existing definitions.

`definitions/tools/boldsign.json` (bitly is the minimal precedent):

```json
{
  "name": "boldsign",
  "type": "service",
  "description": "BoldSign e-signature as a tool (OAuth 2.0 access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "BOLDSIGN_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single credential field `access_token`, sent as `Authorization: Bearer` — the token-gateway projection (`credential.fields.access_token: token.access_token`) and the L2 harness (`ANYCLI_CRED_ACCESS_TOKEN`) both feed it. We do not implement the `X-API-KEY` path: Helio's credential plane produces OAuth tokens, and a second auth path would be a silent fallback (forbidden).

**Package:** `internal/tools/boldsign/` (id has no dashes; package name == id), registered at batch end as `RegisterService("boldsign", &boldsign.Service{})` in `internal/tools/register.go` (shared surface — batch lead merges). Struct follows the bitly/notion shape: `BaseURL` (default `https://api.boldsign.com`), `HC`, `Out`, `Err`; exit codes 0 success / 1 runtime-API (typed `apiError`) / 2 usage; `--json` structured error envelope.

**Subcommand tree** (cobra, resource-grouped per notion precedent):

```
boldsign document send    --file <path>… | --file-url <url>…  --title <t> [--message <m>]
                          --signer "Name <email>"…  [--signer-type Signer|Reviewer]
                          [--signing-order] [--expiry-days N] [--auto-detect-fields]
                          [--text-tags] [--on-behalf-of <email>] [--disable-emails]
boldsign document list    [--page N] [--page-size N] [--status S…] [--search K]
                          [--transmit-type Sent|Received|Both]
boldsign document get     --id <documentId>
boldsign document download    --id <documentId> --out <path>
boldsign document audit-log   --id <documentId> --out <path>
boldsign document remind  --id <documentId> [--email <e>…] [--message <m>]
boldsign document revoke  --id <documentId> --message <reason>
boldsign template list    [--page N] [--page-size N] [--search K]
boldsign template get     --id <templateId>
boldsign template send    --id <templateId> --title <t> [--message <m>]
                          --role "<roleIndex>:Name <email>"…
                          [--field <fieldId>=<value>]…   # ExistingFormFields prefill
                          [--signing-order] [--on-behalf-of <email>]
```

**JSON output shape:** provider JSON passthrough on stdout for every JSON-returning endpoint (list/properties/send return provider objects — `{"documentId": …}`, `{"pageDetails": …, "result": […]}`). 204 endpoints (remind, revoke) emit a small receipt `{"ok": true, "documentId": "…", "action": "remind|revoke"}`. Binary downloads (`download`, `audit-log`) write bytes to `--out` and emit a receipt `{"ok": true, "path": "…", "bytes": N}` — the bitly `qr image` precedent. `document send` uses the JSON content type with base64 data-URI file entries (official JSON shape) so no multipart writer is needed; `--file` reads and encodes locally, `--file-url` passes through. All flags, no interactivity (AGENTS.md rule).

## 4. Helio provider bundle plan

**Naming axes (master plan §3):** ① CLI command word `boldsign` (flat, no group — independent brand) · ② anycli id `boldsign` · ③ provider key `boldsign`. All three identical → **no `toolToProvider` entry**, no `tool.command`, no `tool.group`. Zero resolver-map work for this tool.

`integrations/providers/boldsign/provider.yaml` (hidden-first; `standard_oauth`, zero service-side Go — nothing in BoldSign's flow falls outside the declarative capability set):

```yaml
schema: helio.provider/v1
key: boldsign
go_name: BoldSign

presentation:
  name: BoldSign
  description_key: boldsign
  consent_domain: boldsign.com
  visible: false          # hidden-first; flip is the single go-live change
  order: <batch-lead assigned>

auth:
  type: oauth
  owner: individual        # provider sees a person; consent is per-user
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://account.boldsign.com/connect/authorize
    token_url: https://account.boldsign.com/connect/token
    token_exchange_style: form_secret     # client_secret_post, per official docs
    pkce: s256                            # PKCE is mandatory (audit + docs agree)
    scopes:
      - openid
      - email
      - profile
      - offline_access                    # refresh token
      - BoldSign.Documents.All
      - BoldSign.Templates.All
    display_scopes: [openid, email, profile, offline_access,
                     BoldSign.Documents.All, BoldSign.Templates.All]
    single_active_token: false
    refresh_lease: credential             # refresh tokens are single-use rotating
    revoke:
      url: https://account.boldsign.com/connect/revocation
      client_auth: form
      token: refresh_token
      fallback_token: access_token
      token_type_hint: none

identity:
  source: userinfo
  url: https://account.boldsign.com/connect/userinfo
  stable_key: /sub
  label_candidates: [/email, /name, /sub]

connection:
  mode: isolated
  disconnect_mode: provider_revoke
  runtime_strategy: standard_oauth

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: boldsign
  kind: oauth
```

Scope rationale: `BoldSign.Documents.All` covers send/list/properties/download/remind/revoke; `BoldSign.Templates.All` covers template list/properties and send-from-template (send-from-template creates a document, so both scopes are held; whether `Templates.Read` would suffice for the read half is an L2 verification item — start with `.All`, narrow later if verified). Users/Teams/Contacts/SenderIdentity scopes are omitted — out of wrapped surface.

Lane-1 inputs (human): OAuth app created in the BoldSign dashboard (sandbox credentials for dev), redirect URI = the integration-service callback, refresh-token expiration set to **sliding**, client id/secret distributed as uncommitted local `config/cloud.yaml` entries for L4, and landed in `config/` + `deploy/` Helm Secret together before L5 (Config Sync rule). Not an `oauth_review` tool — nothing gates the visible flip except L5.

**Other Helio-side artifacts (batch-end surfaces except where noted):** icon `ui/helio-app/src/integrations/icons/boldsign.svg` + manual `providerIcons.ts` registration; AI-facing sub-doc `agents/plugins/heliox/skills/tool/boldsign.md` (send/track/remind/revoke/template flows, async-send caveat); heliox plugin version bump + marketplace publish ride the batch. Per master plan §2: provider-gen projections are run **locally only** for L3 validation and never committed from this branch; `helio-cli/go.mod` gets a local, uncommitted `replace github.com/heliohq/anycli => <this worktree>` for the L4 build.

## 5. Test plan (five layers)

| Layer | What runs for boldsign | External credentials needed |
|---|---|---|
| L1 | `go test ./...` in anycli: httptest fakes assert path+query shape (`/v1/document/send` JSON body with base64 file entry, `documentId` query params, template `roles` array), `Authorization: Bearer` injection, provider-JSON passthrough, 204→receipt rendering, binary download to `--out`, exit codes 0/1/2, `--json` error envelope. TDD: tests first (AGENTS.md). | None |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real OAuth token> anycli boldsign -- document list --page 1`, then a real `document send` (sandbox PDF, self as signer), `document get/remind/revoke`, `template list`. Verifies bearer auth is accepted by `api.boldsign.com`, field names match the live API, and whether `Templates.All` is required for `template send`. | **Yes** — BoldSign sandbox account (lane 2) + registered OAuth app (lane 1); the access token must be minted once via a manual PKCE authorize (the sandbox API key cannot substitute — the tool is bearer-only) |
| L3 | Local (uncommitted) `provider-gen` + `provider-gen --check` against the branch bundle; anycli suite + `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with the local `replace`. Branch is expected to fail `--check` in CI until batch-end regen — do not commit local regens. | None |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with provider `boldsign`, seeding **both** `access_token` and `refresh_token` with a deliberately short `expires_at` — this deliberately exercises the refresh-with-rotation path under `refresh_lease: credential` (the riskiest part of this provider). Then `heliox tool boldsign -- document list --page 1` must return live data, and a second run must succeed on the rotated refresh token. | **Yes** — dev app client id/secret in local uncommitted `config/cloud.yaml` (lane 1) + a real token pair from the sandbox app |
| L5 | Human-in-the-loop (lane 3, oauth lane): `heliox tool boldsign auth` → connect link → BoldSign consent (sandbox account) → `oauth_connected` event on the channel → one unseeded live `document send` + `document revoke` round trip. Config landed in `config/` + `deploy/` beforehand. Only after this does `presentation.visible: true` + regenerate ship as the single go-live change. | **Yes** — real sandbox login by a human |

Definition of done per master plan §2: L1–L5 green, docs published, icon registered, visible flip shipped.
