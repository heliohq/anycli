# Fillout — per-tool design (`heliox tool fillout`)

Scratch design file on branch `tool/fillout` (both worktrees). Batch lead strips
it at batch-end. Catalog row 216: Fillout / id `fillout` / key `fillout` /
`oauth_review` / Wave 3 / Forms & Surveys.

**Ground truth verified 2026-07-22 against official Fillout docs:**
- OAuth applications: https://www.fillout.com/help/oauth-applications
- REST API: https://www.fillout.com/help/fillout-rest-api (endpoint index confirmed
  via the Help Center API reference: Get forms / Get form metadata / Get all
  submissions / Get submission by id / Delete submission by id / Create a webhook /
  Remove a webhook / Create submissions).

**Audit reconciliation.** Audit row 218 verdict: `oauth_review`, high confidence —
"OAuth app creation is self-serve (Settings > Developer > OAuth integrations), but
public OAuth apps 'may require review and approval from the Fillout team' before
they can be used by other users." Official docs confirm this verbatim. **No
divergence** — lane `oauth_review` stands: dev-mode app (own account) is self-serve
and unblocks L4/L5-on-own-account; the review clearance gates only the visible flip
for external accounts. Standard hidden-first applies.

---

## 1. API surface wrapped, and why

An AI teammate's job with Fillout is **reading what people submitted** and, less
often, **pushing rows in / wiring notifications**: pull a form's responses to
summarize leads/intake/survey results, fetch a single submission a human referenced,
enumerate the account's forms, read a form's question schema so answers can be
interpreted, occasionally create a submission, and manage the webhook that would
notify Helio of new responses. That set maps exactly onto Fillout's small, stable
public REST surface. There is no reason to wrap anything else — Fillout has no
builder/design API worth automating from an assistant.

Base URL (**per-connection, geo-dependent — see §4**): `{base}/v1/api`, where
`{base}` is `https://api.fillout.com` (US, default), `https://eu-api.fillout.com`
(EU data residency), or a self-host/dedicated host. Auth: `Authorization: Bearer
<token>` where the token is either a dashboard API key (L2 harness) or an OAuth
access token (production). Rate limit: 5 req/s per account/key.

| Verb (anycli) | Method + path | Purpose |
|---|---|---|
| `form list` | `GET /v1/api/forms` | List all forms in the account |
| `form get <formId>` | `GET /v1/api/forms/{formId}` | Form metadata: name + question/field schema |
| `submission list <formId>` | `GET /v1/api/forms/{formId}/submissions` | List responses (paged/filtered) |
| `submission get <formId> <submissionId>` | `GET /v1/api/forms/{formId}/submissions/{submissionId}` | One response |
| `submission create <formId>` | `POST /v1/api/forms/{formId}/submissions` | Create submission(s) |
| `submission delete <formId> <submissionId>` | `DELETE /v1/api/forms/{formId}/submissions/{submissionId}` | Delete a response |
| `webhook create` | `POST /v1/api/webhook/create` | Register a webhook on a form |
| `webhook delete` | `POST /v1/api/webhook/delete` | Remove a webhook |

`submission list` exposes Fillout's documented query params as flags: `--limit`,
`--after-date`, `--before-date`, `--offset`, `--status` (e.g. `finished` vs
`in_progress`), `--include-edit-link`, `--sort`, `--search`. These are pass-through
to the query string; the service does not invent filters the API doesn't have.

## 2. anycli definition (stage-1 form decision + shape)

**Type: `service`.** No official Fillout CLI exists; the `cli`-type rubric fails on
the first clause, so this is a `service`-type tool against the HTTP API — the same
choice as the 21 other service tools and every forms sibling (typeform, jotform,
formstack, tally, paperform, surveymonkey).

- **Definition:** `definitions/tools/fillout.json`, `name: "fillout"`, `type:
  "service"`, `description: "Fillout forms and submissions (OAuth)"`.
- **Go package:** `internal/tools/fillout/` (id has no dashes → package `fillout`),
  registered `RegisterService("fillout", &fillout.Service{})` in
  `internal/tools/register.go`.
- **Shape:** copy the `notion` reference — a cobra tree grouped by resource
  (`form`, `submission`, `webhook`), a `Service` struct exposing
  `BaseURL`/`HC`/`Out`/`Err` so httptest fakes can point at a fake server and
  capture output, the documented exit-code contract (0 success / 1 API-or-runtime
  failure via typed `apiError` / 2 usage-parse), and a `--json` structured error
  envelope. All list/get verbs emit provider-neutral JSON (the passthrough of
  Fillout's own JSON, not a reshaped model) so the assistant reads one consistent
  shape.
- **Credential injection (two fields):**
  1. `access_token` → `inject env FILLOUT_ACCESS_TOKEN`; the service sends it as
     `Authorization: Bearer $FILLOUT_ACCESS_TOKEN`.
  2. `api_base` → `inject env FILLOUT_API_BASE`; the service uses it as the request
     host and **defaults to `https://api.fillout.com` when the field is absent**
     (the documented default host). The default is not a silent fallback that hides
     a broken contract: the L2 harness runs a US dashboard key with no base, and the
     production OAuth path *always* injects the real `base_url` (§4), so an EU/self-
     host connection is never quietly served the wrong host — it is served its own.

`auth.credentials[]` therefore has two bindings. This mirrors the credential-map
contract exactly: the definition only *names* `access_token`/`api_base`; the host
(Helio resolver) decides their values.

## 3. Credential fields & the exact OAuth flow (verified vs official docs)

Fillout OAuth is a **thin, non-RFC-shaped authorization-code flow** — self-serve app
creation, no PKCE, no scopes, no refresh, a custom token endpoint, and a
**per-connection API base_url returned in the token response**. Every field below is
from the official OAuth doc.

- **App registration:** dashboard → account name → Settings → Developer →
  "OAuth integrations" → Create app (name + icon + redirect URIs). `client_id`
  ("public ID"), `client_secret` (shown once). Self-serve = dev-mode works
  immediately for the developer's own account; **external-account authorization may
  require Fillout review** → the `oauth_review` gate on the visible flip only.
- **Authorize:** `GET https://build.fillout.com/authorize/oauth` with exactly
  `client_id`, `redirect_uri`, `state`. **No `response_type`, no `scope`** are
  documented. Helio's `buildOAuthAuthorizeURL` unconditionally appends
  `response_type=code` and appends `scope` only when scopes are declared — so with
  `scopes: []` the emitted URL is
  `…/authorize/oauth?client_id&redirect_uri&response_type=code&state`. Fillout is
  expected to ignore the extra `response_type` (standard behavior); **L5 verifies
  the real consent screen accepts it** (stage-1/L2 risk noted below).
- **Token exchange:** `POST https://server.fillout.com/public/oauth/accessToken`,
  body `code`, `client_id`, `client_secret`, `redirect_uri` → **`token_exchange_style:
  form_secret`** (form-encoded, secret in body; the default). Response is exactly
  `{ "access_token": "...", "base_url": "https://api.fillout.com" }` — **no
  `token_type`, no `expires_in`, no `refresh_token`.** Non-expiring token, like
  Notion/Slack → `refresh_lease: none`, no refresh path to exercise.
  - *Stage-1/L2 risk:* the custom `/public/oauth/accessToken` endpoint may be strict
    about the extra `grant_type=authorization_code` the standard `form_secret`
    exchanger adds. If L2/L5 shows Fillout rejects it, the fix is a narrow exchanger
    tolerance in the adapter (§4), not a new bundle enum. Flagged at stage 1, not
    mid-wave.
- **Revoke:** `DELETE https://server.fillout.com/public/oauth/invalidate` with the
  Bearer token. The initial hidden bundle ships `disconnect_mode: local_only` (like
  Notion/Microsoft) — a provider-side declarative DELETE-with-bearer revoker is a
  post-hidden enhancement, not a blocker.

## 4. Helio provider bundle plan (`integrations/providers/fillout/provider.yaml`)

**Three axes (all identical → no `toolToProvider` entry needed):**
① CLI command word `fillout` · ② anycli id `fillout` · ③ provider key `fillout`.
Flat command (`heliox tool fillout -- …`), no group.

**Two genuinely non-standard axes drive the bundle design:**

**(a) Per-connection `base_url` capture + projection.** The token response carries
`base_url` (US `api.fillout.com` / EU `eu-api.fillout.com` / self-host). This is the
*exact* Salesforce `instance_url` / Adobe Sign `base_uri` situation, and this branch
base's closed `CredentialSource` allowlist (`token.access_token`,
`connection.account_key`, `connection.metadata.person_urn`, `credential.app_id`,
`credential.brand`) has **no** representable value for it. The bundle must capture
`base_url` from the token response into connection metadata and project it to the
runtime as the `api_base` credential field. Plan: **reuse the token-response
metadata-capture + base-url credential-source capability landed for Salesforce
(instance_url) / Adobe Sign (base_uri) on `main`** once the branch rebases onto it;
if absent at merge time, grow the same narrow, reviewed capability (one metadata
capture keyed by a bundle JSON pointer + one `connection.metadata.base_url`
credential source). Do **not** hardcode `api.fillout.com` server-side and hide the
EU/self-host contract — that is the forbidden silent-downgrade path.

**(b) No account identity.** Fillout exposes **no** userinfo/whoami endpoint and the
token response has **no** per-account identifier (only `access_token` + `base_url`,
and `base_url` is shared across accounts in a geo). The declarative resolver
hard-errors on an empty stable key, so `identity.source: token_response|userinfo` is
not usable.

Because (a) and (b) both fall outside the closed `standard_oauth` capability set,
the reviewed pattern is a **narrow strategy adapter** `service/adapter_fillout.go`
(precedents: slack/discord/linkedin/x), `runtime_strategy: fillout` /
`identity.source: strategy`. The adapter:
- reuses `standardOAuthExchanger(form_secret)` (adding grant_type tolerance only if
  the stage-1/L2 risk materializes), and **persists `base_url`** from the raw token
  response to connection metadata;
- resolves identity by returning a **constant synthetic account key** (e.g.
  `"fillout"`) + static label (e.g. `"Fillout account"`), making the connection
  `mode: isolated`, single-per-assistant — honest given Fillout offers nothing to
  distinguish accounts on; re-authorization upserts the same row;
- projects `access_token` + the persisted `base_url` (as `api_base`) into the
  credential map.

*Fallback if a lighter surface is preferred:* if `main` already carries both the
Salesforce base-url capability **and** a "synthetic/constant identity" enum for
isolated connections, this can ship declaratively (`standard_oauth` +
`identity.source: strategy`-less) with zero new Go. The adapter is the safe default
because it bundles both provider-specific axes in one reviewed unit. **Confirm
capability state on the branch's actual merge base at implementation stage 1** — do
not assume the sibling-batch capabilities are present.

**Bundle skeleton (hidden-first):**
```yaml
schema: helio.provider/v1
key: fillout
go_name: Fillout
presentation:
  name: Fillout
  description_key: fillout
  consent_domain: fillout.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <next>
auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://build.fillout.com/authorize/oauth
    token_url: https://server.fillout.com/public/oauth/accessToken
    token_exchange_style: form_secret
    pkce: none
    scopes: []              # Fillout OAuth has no scope concept
    display_scopes: []
    single_active_token: false
    refresh_lease: none
identity:
  source: strategy          # no userinfo / no id in token response
connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: fillout # narrow adapter (base_url capture + synthetic identity)
resources: { selection: none, discovery: none, enforcement: none }
credential:
  fields:
    access_token: token.access_token
    api_base: connection.metadata.base_url   # projected by the adapter
    account_key: connection.account_key
tool:
  name: fillout
  kind: oauth
```

**Config (Config Sync hard rule):** `oauth.client_id` / `oauth.client_secret` for
`fillout` land in **both** `config/` (local) and the `deploy/` Helm Secret together
(lane-1 output; id+secret always in the same change — a partially-configured
provider fails integration-service startup). Absent-everything renders
`configured: false` and is safe to ship hidden.

**Generate + non-service surfaces:** `provider-gen` + `--check` (five projections,
committed together at batch end — **not** on this tool branch). UI icon
`ui/helio-app/src/integrations/icons/fillout.svg` + manual `providerIcons.ts`
registration. AI-facing sub-doc under `agents/plugins/heliox/skills/tool/`
documenting the verbs, base-url note, and the "one Fillout account per assistant"
identity model. i18n `description_key` string.

## 5. Test plan → the five layers

| Layer | What runs for Fillout | External creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: `internal/tools/fillout` httptest fakes assert request path (`/v1/api/forms/{id}/submissions`), `Authorization: Bearer` header, `api_base` host selection (default vs injected EU host), submission-list query-flag mapping, and both plaintext + `--json` error rendering. Pure TDD, no network. | none |
| **L2** | dev harness against the **real** Fillout API: `ANYCLI_CRED_ACCESS_TOKEN=<dashboard API key> anycli fillout -- form list` and `submission list <formId>`. Proves field names, Bearer injection, `/v1/api` paths, and the grant/`response_type` risks are moot for the data plane. Mandatory before the pin bump. | **yes** — a real Fillout dashboard API key (account-pool, lane 2) |
| **L3** | `provider-gen --check` + integration-service + helio-cli unit suites (adapter unit test: `base_url` capture from a fake token response, synthetic account-key, `api_base` projection). Run locally on-branch; the tool branch is expected to fail `--check` in CI until batch end. | none |
| **L4** | singleton + `POST /internal/test-only/connections/seed` (provider `fillout`, `access_token` only — non-expiring, no refresh, so **no** `expires_at`/`refresh_token`; seed `base_url` metadata so `api_base` projects) → `heliox tool fillout -- form list` reaches the live API through the token gateway. | **yes** — a real Fillout access token/API key + a real seeded assistant/org identity |
| **L5** | one full connect flow before the visible flip, on the **dev-mode app / own account** (review clearance is not required to run L5, only to flip external-visible): `heliox tool fillout auth` → Fillout consent → `oauth_connected` system event → unseeded `form list`. Verifies the authorize URL (extra `response_type=code`), the custom token endpoint (extra `grant_type`), `base_url` capture, and the synthetic-identity connect UX end-to-end — the risks L1–L4 cannot cover. | **yes** — a real Fillout account for live consent (human-in-the-loop, oauth L5, lane 3) |

**Layers needing externally supplied credentials: L2, L4, L5** (L2/L4 an API key or
access token from the account pool; L5 a live Fillout account for OAuth consent).
L1 and L3 are fully self-contained.

**Definition of done:** all five layers green → docs published + icon registered →
**review clearance** obtained → flip `presentation.visible: true` + regenerate as the
single go-live change.
