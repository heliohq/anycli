# Brex — per-tool design (`heliox tool brex`)

Scratch design for the `tool/brex` batch branch. Batch lead strips this at
batch-end. Catalog row 143: Product **Brex**, anycli id `brex`, provider key
`brex`, auth lane **oauth_review**, wave 2, category Finance.

Every claim below was verified against Brex's official developer docs
(`developer.brex.com`, July 2026) and the actual repo code, not inherited from
the catalog or the audit. The load-bearing decision — **is `oauth_review`
viable at all for a multi-tenant AI teammate when Brex has no self-service
client registration?** — is confronted head-on in §3, and every divergence from
the catalog/audit is recorded in §7.

---

## 0. Lane verdict (the question the review demanded)

**Verdict: BUILD on the `oauth_review` lane, `service` type, `standard_oauth`
runtime strategy. The audit's `oauth_review` classification is CORRECT and is
upgraded from `medium` to `high` confidence after verifying the official docs.
The api_key user-token path is real but is single-account-only and is rejected
as the primary lane for a multi-tenant teammate (§3.3).**

The audit's own rubric (`oauth-audit.md`): *"a human review, partner-program,
verification, or publish gate before external accounts can authorize →
`oauth_review`."* Brex fits this exactly, just with a different **shape** of gate
than a marketplace publish review:

- **Multi-tenant authorization-code OAuth genuinely exists.** Verified at
  `https://developer.brex.com/guides/partner_authentication`: the Authorization
  Code Grant is *"a web-based redirection flow to request permission from the
  Brex client to access their account"* — the customer signs in and is
  *"prompted to authorize or deny your application's access to their account"*,
  and may grant a subset of requested scopes. One registered partner app is
  authorized by arbitrary Brex customer accounts. This is the multi-tenant
  authorization-code flow the rubric requires — **not** a per-instance or
  developer-only token.
- **The gate is a manual partner-credential-issuance step, not self-service.**
  *"Contact our developer support team… to be issued a client ID and client
  secret for partner authentication."* Redirect URIs are registered with Brex at
  that time and the `redirect_uri` *"must match exactly one of the addresses that
  was provided to Brex when the credentials were set up."* This human issuance
  gate is precisely why the tool is `oauth_review` and not `oauth_light` — it is
  the **same class of gate as Zendesk** (audit row 19: a global OAuth client
  *"developers cannot create themselves — you must request one… and await
  approval"*), which the audit also placed in `oauth_review`.

**Why this does not break Helio's model.** The manual issuance gate maps cleanly
onto the plan's hidden-first, three-lane execution model (master plan §2):

- Lane 1 (OAuth app registration/review queue) already owns *"creating provider
  apps, redirect URIs, scope requests"* by contacting the provider — Brex's
  "email developer support for a client_id/secret" is exactly this lane's job,
  just done over email instead of a self-serve console. One partner app is
  registered once; every Helio-managed customer connection rides that single
  `client_id`/`client_secret`.
- Brex offers a **staging** auth server
  (`accounts-api.staging.brexapps.com/oauth2/default`) and staging credentials,
  so **dev/test credential issuance gates L4/L5** while the production-partner
  credential turnaround **gates only the visible flip** — the standard
  hidden-first decoupling. Code lands complete-but-hidden in Wave 2 regardless of
  the production-credential clock.

The only residual risk is scheduling: the issuance turnaround is a human email
loop with Brex, not an instant console toggle, so lane 1 must open the Brex
partner-credential request **early** (it front-runs dev by a wave anyway). If
production partner credentials genuinely never arrive, the tool simply waits in
code-complete/hidden state — zero code waste, exactly the plan's stall
mitigation. **No catalog change**: row 143 stays `brex | brex | brex |
oauth_review | 2 | Finance`.

---

## 1. What an AI teammate does with Brex — and the API surface that serves it

A Helio teammate connected to a customer's Brex account is a **finance /
spend-ops colleague**: it answers "what did we spend, on which cards, by whom,
against which budget", reconciles expenses, and reports balances. It is
read-mostly. It does **not** issue cards, move money, or run onboarding — those
are high-blast-radius or platform-onboarding concerns outside an assistant's job
and outside hidden-first scope.

That intent maps onto the **Brex REST API** (`https://api.brex.com`, JSON,
`Authorization: Bearer <token>`). The tool wraps exactly the resources the
colleague reaches for, grouped by Brex's API products:

| Group | Endpoints wrapped (all `GET`) | Why |
|---|---|---|
| `account` | `/v2/accounts/card`, `/v2/accounts/cash`, `/v2/accounts/cash/:id` | Balances across card + cash accounts. |
| `transaction` | `/v2/transactions/card/primary`, `/v2/transactions/cash/:id` | The core "what did we spend" ledger. |
| `expense` | `/v1/expenses`, `/v1/expenses/card`, `/v1/expenses/:id` | Expense reconciliation + receipt/memo status (read). |
| `card` | `/v2/cards`, `/v2/cards/:id` | Which cards exist, limits, status. |
| `user` (Team API) | `/v2/users`, `/v2/users/me`, `/v2/users/:id` | Who spent / cardholder lookup; `me` also serves identity (§3.4). |
| `budget` | `/v2/budgets`, `/v2/budgets/:id`, `/v2/spend_limits` | Budget vs. actual reporting. |
| `department` / `location` | `/v2/departments`, `/v2/locations` | Dimension lookups for grouping spend. |
| top-level `get` | arbitrary `GET /<path>` passthrough | Long-tail reads without a per-resource verb. |

Write verbs (update expense memo, create budget, etc.) are **deliberately
deferred** past hidden-first — subtract before adding; they are a post-L2
enhancement once the read posture is proven, and several require elevated scopes.

Cross-cutting conventions (verified against Brex docs):
- **Pagination**: cursor-based — request `cursor` + `limit`, responses carry
  `{ "items": [...], "next_cursor": "..." }`. Surfaced as `--cursor` / `--limit`;
  `--json` returns the envelope verbatim.
- **Auth**: every call sends `Authorization: Bearer <access_token>` against
  `api.brex.com` (production only — there is no separate data host per account;
  the OAuth token scopes the request to the connected Brex account server-side).
- **Errors**: Brex returns a JSON error body with a 4xx/5xx; the service maps it
  to the typed `apiError` exit-code contract below.

### Why NOT a cli-type tool

Brex ships no official first-party CLI. `service` type against the REST API is
the only option and matches 21/23 shipped definitions. Stage-1 rubric: `service`.

---

## 2. anycli definition (data plane)

- **Type**: `service`. Package `internal/tools/brex/` (id has no dashes → Go
  package `brex`), registered `RegisterService("brex", &brex.Service{})` in
  `internal/tools/register.go`. Definition file `definitions/tools/brex.json`.
- **Shape**: copy the `internal/tools/notion/` reference — a cobra tree grouped
  by resource (`account`, `transaction`, `expense`, `card`, `user`, `budget`,
  `department`, `location`) plus a top-level `get`, a `BaseURL`/`HC`/`Out`/`Err`
  struct so tests point at an `httptest` server, and the documented exit-code
  contract: **0** success, **1** runtime/API failure (typed `apiError` from
  Brex's error body), **2** usage/parse error. `--json` emits a structured error
  envelope.
- **Auth injection** — the token gateway projects the connection's bearer token
  into `token.access_token`; the bundle `credential.fields.access_token` maps it,
  and the anycli definition injects it as an env var the service reads and sends
  as `Authorization: Bearer <token>`:

  ```json
  {
    "name": "brex",
    "type": "service",
    "description": "Brex accounts, transactions, expenses, cards, and budgets (OAuth)",
    "auth": {
      "credentials": [
        { "source": {"field": "access_token"},
          "inject": {"type": "env", "env_var": "BREX_ACCESS_TOKEN"} }
      ]
    }
  }
  ```

  Brex accepts the OAuth access token via `Authorization: Bearer <token>` against
  `api.brex.com` — the same header the docs use for user tokens (`bxt_…`) and for
  partner OAuth access tokens.

- **L1 TDD**: `httptest.Server` fakes per resource assert request path, method,
  the injected `Authorization: Bearer` header, `--cursor`/`--limit` params, the
  `next_cursor` pagination envelope, and both text and `--json` error rendering.
  No live API in unit tests.

---

## 3. Credentials & the exact auth flow (the load-bearing decision)

### 3.1 Chosen flow — Brex Partner OAuth 2.0 (authorization code)

Verified at `https://developer.brex.com/guides/partner_authentication`. Brex's
auth server is an **OpenID-Connect-compliant** server (Okta-style path layout):

| Piece | Value |
|---|---|
| Auth server base (prod) | `https://accounts-api.brex.com/oauth2/default` |
| Auth server base (staging) | `https://accounts-api.staging.brexapps.com/oauth2/default` |
| Authorize URL | `…/v1/authorize` (params: `client_id`, `response_type=code`, `redirect_uri`, `scope`, `state` — `state` **must be > 8 chars**) |
| Token URL (exchange **and** refresh) | `…/v1/token` |
| Revoke URL | `…/v1/revoke` |
| Client auth at token endpoint | `client_id` + `client_secret` **in the form body** (not HTTP Basic) — verified from the docs' `curl` examples → maps to the existing `token_exchange_style: form_secret`. |
| Scopes | `openid` (required for the OIDC/identity request) + `offline_access` (**required to receive a refresh token**) + space-delimited API scopes for the resources in §1. |
| PKCE | *"Also supported, but not required."* → `pkce: none` (standard confidential-client flow; the partner secret is server-side in integration-service). |
| access_token | bearer, **1-hour** lifetime (`"expires in 3600 seconds"`). |
| refresh_token | **90-day** lifetime; refreshing does not require re-consent until expiry. |
| API data host | `https://api.brex.com` (production). |

### 3.2 Lane confirmation is in §0; this section is the mechanics

The lane decision (viable, `oauth_review`, unchanged) is fully argued in §0. What
remains here is the one **technical** item that decides whether Brex needs any
integration-service capability growth or is pure stock `standard_oauth`.

### 3.3 Why not the api_key user-token path

Brex also exposes a **self-service user token** (`developer/settings` in the
dashboard → *Create Token*, `bxt_` prefix, `Authorization: Bearer`). It is
tempting because it needs no partner-credential email loop. It is **rejected as
the primary lane** and no catalog change is proposed, because:

- It is created by *"an account admin or card admin"* inside **one** Brex
  account and covers only **that** account's data — it is the *"developer
  authentication"* path for building against *your own* Brex org. A multi-tenant
  AI teammate connecting to arbitrary customer accounts cannot use one org's
  pasted token for another org.
- It has **no refresh mechanism** and *"will expire if not used to make an API
  call for 90 days"* — a pasted long-lived secret, not a per-connection
  refreshable grant. That is the `api_key` credential model, a different lane.

Recorded as considered-and-rejected. (If a future single-account-only Brex use
case appears, it would be a *separate* `manual_api_token` bundle, not this one.)

### 3.4 Identity

Brex is OIDC-compliant and the `openid` scope is requested, so the connected
account's identity is available two ways:

- **Preferred: OIDC userinfo.** `identity.source: userinfo` with the userinfo
  endpoint from the well-known document
  (`…/oauth2/default/.well-known/openid-configuration`), `stable_key: /sub`. This
  is a zero-extra-scope, standard OIDC identity and is the same `userinfo`
  identity source the docusign bundle uses.
- **Alternative: `GET /v2/users/me`** (Team API) returns the authenticated
  user's `id`, usable as `identity.source: api` if a Brex-API-side key is
  preferred over the OIDC `sub`.

Decision: **`userinfo` / `/sub`** for hidden-first (no dependency on a
data-API response shape, and `openid` is already in the scope set). The exact
userinfo URL is read from the well-known document at stage 1 (§7 open item),
since the docs point to well-known as *"the source of truth"* rather than
printing the userinfo URL inline.

### 3.5 The one load-bearing technical fact — token-response `expires_in` is documented ABSENT

**This is the single fact that decides capability growth.** The official docs
were re-verified for this revision. The **authorization-code** token-response
example lists only `access_token`, `refresh_token`, `token_type: "bearer"` and
**deliberately omits `expires_in`**, while the **client-credentials** example (a
different grant we do not use) *explicitly shows* `expires_in: 3600`. Brex prints
the field in one grant and drops it in the other — that is a **structured signal
that the omission is intentional, not a truncation artifact**. The expected case
is therefore that the auth-code token response carries **no** `expires_in`.

Two cases, both already precedented in this program:

1. **Expected case — `expires_in` is absent** (as documented). This is the
   **exact Stripe/Salesforce failure mode**: a nil persisted `Expiry` makes
   `needsRefresh()` read the token as non-expiring, the refresh never fires, and
   every connection 401s ~1 h after connect. The fix is the already-designed
   **assumed-TTL** capability — bundle field `oauth.access_token_ttl_seconds:
   3600` projected to `OAuthEndpoints.AssumedAccessTokenTTL`, synthesizing the
   documented 1-hour expiry at exchange time (a returned value always wins when
   present, so it is a fallback, never an override). Salesforce (`assumed_ttl`)
   and Stripe (`access_token_ttl_seconds`) add this on sibling branches; **that
   field is NOT yet on this worktree's `oauthManifest`** (verified —
   `provider-gen` would reject it today), so Brex carries a **real ordering
   dependency**: the assumed-TTL capability must merge to Brex's Wave-2 base
   before Brex's bundle can set the field. See §4.
2. **Unlikely case — `expires_in` IS returned** after all (the live Okta
   `/v1/token` returns it despite the doc example). Then Brex is **pure stock
   `standard_oauth`** with no growth: the generic exchanger reads `expires_in`,
   persists a real `Expiry`, refresh fires at ~1 h. This is now the *fallback*
   reading, not the baseline.

So the plan is: **assume the assumed-TTL path (case 1) is required; keep the L2
confirmation against the real staging `…/v1/token` endpoint as the gate that
flips to case 2 only if `expires_in` unexpectedly appears.** No new bespoke
adapter either way — this is the skill's "grow one reviewed enum/field, not an
adapter" guidance; here Brex **consumes** the salesforce/stripe field rather than
authoring it.

### 3.6 Disconnect — real OAuth revoke (`provider_revoke`)

Brex exposes a standard **`…/v1/revoke`** endpoint, so the bundle sets
`connection.disconnect_mode: provider_revoke` with a declarative
`auth.oauth.revoke:` block. This is a **hard validator rule, not a preference**:
`provider_revoke` on `standard_oauth` REQUIRES the revoke block
(`validate.go:503-506`), and `revoke` is **not** a valid mode value (the closed
enum is `provider_revoke | local_only | strategy`, `validate.go:402`). The
declarative revoker is best-effort — a failed revoke never blocks disconnect (the
mandatory part is the vault delete + status flip).

Revoke request shape, cross-checked against `oauthRevokeManifest` and the gmail
OIDC bundle (not invented shorthand):

- **URL**: `https://accounts-api.brex.com/oauth2/default/v1/revoke` (prod) — an
  **Okta-hosted** auth server (`/oauth2/default/v1/*` is Okta's layout).
- **`client_auth: form`** — `client_id` + `client_secret` in the request body,
  the same confidential-client style as the `form_secret` token exchange. The
  in-tree declarative revoker (`service/revoke.go`) POSTs
  `application/x-www-form-urlencoded` and appends `client_id`/`client_secret` to
  the form when `client_auth: form`. Okta's `/v1/revoke` is RFC 7009 and accepts
  exactly this.
- **`token: refresh_token`** — revoke the 90-day refresh grant on disconnect.

**Divergence recorded (see §7).** Brex's *doc example* for `/v1/revoke` renders a
**JSON-shaped body** (`token`/`client_id`/`client_secret`) and, tellingly,
specifies **no `Content-Type`** — a doc-quality gap, not a stated contract.
Because the server is Okta (whose real `/v1/revoke` is form-urlencoded RFC 7009),
the declarative **form** revoker is the correct first choice, to be **confirmed at
L2** against the staging endpoint. The in-tree revoker only emits form bodies (its
own comment: providers needing "non-form bodies… remain explicit compiled
strategies"), so if L2 proves Brex genuinely requires a JSON body, the fallback is
`disconnect_mode: local_only` (drop the revoke block) — **never** a bespoke JSON
revoke adapter for hidden-first.

### 3.7 Config fields (integration-service, per environment)

`auth.required_config_fields: [oauth.client_id, oauth.client_secret]`, supplied
via integration-service config (`config/` + the `deploy/` Helm Secret together —
Config Sync hard rule). `client_id`/`client_secret` = the Brex-issued **partner**
credentials (from lane 1's developer-support request). Never in the bundle. A
fully unset provider renders `configured:false` (Connect disabled, safe hidden);
a **partial** config fails integration-service startup — so id + secret land in
the same change, before this provider's L5.

---

## 4. Helio integration-service capability growth

**Expected: ONE reused field — the assumed-TTL capability (§3.5), consumed not
authored.** Every other axis is already stock `standard_oauth`: form-body client
auth (`form_secret`), declarative `userinfo`/`/sub` identity, refresh-token
write-back, and the declarative `provider_revoke` block (§3.6). But because the
official docs document `expires_in` as **absent** from the auth-code response
(§3.5), the expected build needs `oauth.access_token_ttl_seconds: 3600` — and
that field is **not yet on this worktree's `oauthManifest`** (verified). It is the
salesforce/stripe assumed-TTL capability landing on sibling branches; Brex
**consumes** it (one bundle field), it does not author it.

So Brex carries a **probable ordering dependency**, not a merely conditional one:
the batch lead should sequence Brex's bundle behind the assumed-TTL landing on
its Wave-2 base. The L2 check against the real staging `…/v1/token` endpoint is
the confirmation gate — it flips this to "no growth" only in the unlikely event
`expires_in` turns out to be present. No Brex-specific adapter either way.

Refresh-token rotation: Brex refresh tokens are **not** documented as rotating on
each exchange (unlike Stripe), so the default refresh write-back suffices; no
per-credential lease is required. (Confirm at L2 — if rotation is observed, set
`refresh_lease: credential`, the existing `OAuthLeaseCredential`, no new code.)

---

## 5. Helio provider bundle plan (`integrations/providers/brex/provider.yaml`)

Naming axes — **no ②↔③ divergence**, so **no `toolToProvider` entry** and no
grouped-command word:

| Axis | Value |
|---|---|
| ① CLI command word | `brex` (flat) → `heliox tool brex` |
| ② anycli tool id | `brex` |
| ③ provider catalog key / bundle dir | `brex` |

`ProviderFor("brex")` falls through to identity (`brex`), and `ToolFor`
likewise — nothing to register in `resolver.go`.

Bundle (hidden-first — `visible: false`):

```yaml
schema: helio.provider/v1
key: brex
go_name: Brex

presentation:
  name: Brex
  description_key: brex
  consent_domain: brex.com
  visible: false          # hidden-first; flip true only after L5 + production partner-credential issuance
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual        # per-account user-authorized token; connection belongs to the assistant.
                           # NOT `assistant` — that triggers the app-bot org-admin gate in oauth_start.go.
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://accounts-api.brex.com/oauth2/default/v1/authorize
    token_url: https://accounts-api.brex.com/oauth2/default/v1/token
    token_exchange_style: form_secret     # client_id + client_secret in the form body
    pkce: none
    display_scopes: [openid, offline_access, <api scopes for §1 resources>]
    single_active_token: false
    # access_token_ttl_seconds: 3600       # EXPECTED (§3.5): the official docs OMIT expires_in from the auth-code
    #                                      # token response, so Brex almost certainly needs the synthesized 1-hour
    #                                      # expiry. NOT yet in this base's oauthManifest — depends on the
    #                                      # salesforce/stripe assumed-TTL field landing on Brex's Wave-2 base first
    #                                      # (§4). Uncomment once that capability merges; a returned value always wins.
    # refresh_lease: credential            # ADD ONLY IF §4 L2 proves refresh-token rotation
    revoke:                                # §3.6 — Okta-style RFC 7009 endpoint; disconnect revokes the refresh grant
      url: https://accounts-api.brex.com/oauth2/default/v1/revoke
      client_auth: form                    # client_id + client_secret in the request body (matches form_secret exchange)
      token: refresh_token

identity:
  source: userinfo                          # OIDC; userinfo URL from the well-known doc (§3.4, stage-1 confirm)
  stable_key: /sub
  label_candidates: [/email, /name, /sub]

connection:
  mode: isolated
  disconnect_mode: provider_revoke          # real …/v1/revoke (§3.6). provider_revoke on standard_oauth REQUIRES the
                                            # auth.oauth.revoke block above (validate.go:503-506); `revoke` is not a
                                            # valid mode. Fall back to local_only (and DROP the revoke block) only if
                                            # L2 proves the form revoker can't target the endpoint (§3.6).
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
  name: brex
  kind: oauth
```

Non-generated companions landing on the batch-end merge:
- UI icon `ui/helio-app/src/integrations/icons/brex.svg` + a `providerIcons.ts`
  registration (manual, never generated).
- i18n `description_key: brex` label string.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` describing the
  verbs, `--cursor`/`--limit`/`--json` conventions, and the read-mostly posture.
- The five `provider-gen` projections regenerate together (**never committed on
  this tool branch** — batch lead owns the one canonical regen; the branch is
  expected to fail `provider-gen --check` in CI until batch end, per master plan
  §2).

---

## 6. Test plan — five layers

| Layer | Brex-specific content | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fakes per resource: assert path/method, `Authorization: Bearer` injection, `--cursor`/`--limit`, the `next_cursor` envelope, and typed `apiError` in text + `--json`. | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli brex -- transaction card-primary --limit 3` against a **real Brex account** (a `bxt_` user token from the account pool doubles as a valid `api.brex.com` bearer for the *data-plane* harness). Proves field names, Bearer injection, pagination, error shape. **This layer also resolves the §3.5 `expires_in` question** by exercising the real staging `…/v1/token` endpoint with the staging partner app and inspecting the token response. | **Yes** — a Brex account/token (account pool) + a staging partner app (lane 1). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` accept the bundle (only after the assumed-TTL field is on the base, per §3.5/§4 expected case); integration-service unit test for the assumed-TTL exchange path (a no-`expires_in` Brex-shaped response persists a non-nil `Expiry` ≈ `now+1h`); `helio-cli` builds against the anycli branch via local `replace`; both repos' unit suites green. | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` a `brex` connection with `access_token` + `refresh_token` (+ an aged expiry per §3.5 so the next `heliox tool brex -- account cash` forces the token gateway's refresh-and-write-back through `…/v1/token`). Success = live Brex data, not a replayed seed. | **Yes** — a real OAuth access+refresh pair from the staging partner app (dev-cred issuance gates this; lane 1 distributes `client_id`/`client_secret` as uncommitted local `config/cloud.yaml` entries). |
| **L5** full connect | Once, hidden, pre-flip: `heliox tool brex auth` → consent on the Brex authorize screen (staging or prod partner app) → `oauth_connected` system event → one **unseeded** live `heliox tool brex` run. Human-in-the-loop (oauth L5, plan lane 3). | **Yes** — live Brex consent on a real account; human consent session. |

Rollout: land hidden + generated + L1–L4 green; run L5 while hidden; then flip
`visible: true` + regenerate as the single go-live change — and, because Brex is
`oauth_review`, only after **production partner credentials** have been issued by
Brex developer support (issuance gates the flip, never dev/L4/merge).

---

## 7. Divergences & open items recorded (independent-judgment check)

- **Lane viability was a real question, and the answer is YES.** The review
  correctly flagged that Brex has **no self-service client registration**
  (`client_id`/`client_secret` are issued only by emailing Brex developer support
  with pre-registered redirect URIs) and that self-serve **user tokens** cover
  only the developer's own account. Verified both against the official docs. The
  resolution (§0/§3): the multi-tenant authorization-code flow is real and
  first-class; the manual issuance is a **partner-onboarding gate of the same
  class as Zendesk's global-OAuth-client request** (audit row 19 → `oauth_review`),
  handled by lane 1 over email and decoupled from dev by hidden-first + a staging
  auth server. `oauth_review` is **viable and correct**; the api_key user-token
  path is single-account-only and rejected as the primary lane (§3.3).
- **Audit confidence upgraded `medium` → `high`.** The audit's `medium` was on
  the multi-tenant-vs-self-only question; the official docs resolve it in favor
  of a genuine multi-tenant authorization-code flow. Lane unchanged; no §6
  catalog amendment needed.
- **The `oauth_review` "review" is credential issuance, not a marketplace publish
  review.** Behaviorally identical for Helio (a human gate before external
  accounts can be served; gates the visible flip, not dev), but recorded so the
  batch lead knows lane 1's Brex task is an **email loop with Brex developer
  support** (start early), not a self-serve console submission.
- **`expires_in` is documented ABSENT from the auth-code response — the
  assumed-TTL path is the EXPECTED case, not a conditional fallback (§3.5/§4).**
  The official docs show `expires_in: 3600` in the client-credentials example and
  deliberately omit it from the authorization-code example — a structured signal
  the omission is intentional. So Brex almost certainly needs the salesforce/stripe
  assumed-TTL field (`oauth.access_token_ttl_seconds: 3600`), which is **not yet on
  this worktree's `oauthManifest`** (verified — `provider-gen` rejects it today).
  This is a **probable ordering dependency**: the batch lead sequences Brex behind
  the assumed-TTL landing. **L2** against the real staging token endpoint is the
  gate that would flip it to "no growth" only if `expires_in` unexpectedly appears.
- **Userinfo URL is read from the well-known document at stage 1**, since the
  docs point to well-known as the source of truth rather than printing the
  userinfo endpoint inline.
- **Revoke body format is a documented divergence; refresh-token rotation is an
  L2 confirm (§3.6, §4).** The bundle ships `disconnect_mode: provider_revoke`
  with a declarative **form** revoke block (Okta RFC 7009 `…/v1/revoke`). Brex's
  doc example renders a JSON-shaped revoke body with **no `Content-Type`**; since
  the server is Okta and the in-tree revoker (`service/revoke.go`) only emits form
  bodies, the form revoker is the first choice, confirmed at L2 — fall back to
  `local_only` (drop the block) only if L2 proves a JSON body is mandatory, never
  a bespoke JSON adapter. Refresh-token rotation defaults to no lease; add
  `refresh_lease: credential` only if the real API proves rotation.
- **No `toolToProvider` entry** (id == key == `brex`), and **no grouped
  command** (Brex is not a corporate family in this catalog).
