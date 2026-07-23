# Tool design: Dropbox Sign (`dropbox-sign`)

Scratch design for the `tool/dropbox-sign` batch branch (both repos). Committed
on-branch per the master plan; the batch lead strips it at batch end. Scope is
the per-tool design only — it does not restate the pipeline skill or the master
plan.

- **Catalog row:** #217 — Product "Dropbox Sign", anycli id `dropbox-sign`,
  provider key `dropbox_sign`, auth lane `oauth_review`, Wave 3, category
  Scheduling & eSign.
- **Audit verdict:** row 219 — OAuth **yes**, lane `oauth_review`, confidence
  **high**, evidence `https://developers.hellosign.com/docs/guides/app-approval/overview`.
- **Naming axes (all three):** ① CLI command word `dropbox-sign` (flat, no
  group) · ② anycli id `dropbox-sign` · ③ provider key `dropbox_sign`.
- **Product note:** "HelloSign" is the legacy brand; the API host and dev docs
  still live under `hellosign.com` (`api.hellosign.com`, `app.hellosign.com`,
  `developers.hellosign.com`). The product is Dropbox Sign. Do **not** conflate
  with **Dropbox** (row 45, key `dropbox`, Storage) — separate product, separate
  OAuth app, separate host (`dropbox.com`). They are **not** a design-303 family
  (no shared OAuth umbrella), so `dropbox-sign` ships as a **flat** command, not
  `heliox tool dropbox sign`. No `tool.group`.

Everything below was verified against Dropbox Sign's official developer docs on
2026-07-22 (see §8 Sources); nothing was inherited from the catalog or audit
without a source check. No divergence from the audit's `oauth_review` verdict was
found — it is confirmed.

---

## 1. What an AI teammate does with Dropbox Sign, and the API surface it needs

Dropbox Sign is an e-signature platform. An AI teammate's real jobs are:
**send a document out for signature, track whether it's signed, chase signers,
and pull the completed signed PDF back** — plus reuse of saved templates. That
maps to a narrow slice of the Dropbox Sign REST API v3 (base
`https://api.hellosign.com/v3/`), chosen by job-to-be-done, not by wrapping the
whole surface:

| Job | Endpoint (v3) | Verb |
|---|---|---|
| Send a document for signature (upload file or file URL) | `POST /signature_request/send` | `signature-request send` |
| Send using a saved reusable template | `POST /signature_request/send_with_template` | `signature-request send-with-template` |
| List signature requests (status board) | `GET /signature_request/list` | `signature-request list` |
| Get one request's status + signer state | `GET /signature_request/{id}` | `signature-request get` |
| Download the signed document (PDF/ZIP) | `GET /signature_request/files/{id}` | `signature-request files` |
| Remind a signer (resend email) | `POST /signature_request/remind/{id}` | `signature-request remind` |
| Cancel an incomplete request | `POST /signature_request/cancel/{id}` | `signature-request cancel` |
| List reusable templates | `GET /template/list` | `template list` |
| Get one template (roles/fields) | `GET /template/{id}` | `template get` |
| Account identity / quota | `GET /account` | `account get` |

Excluded on purpose (out of the teammate's job, or heavy/embedded-only):
embedded signing/`/embedded/*`, team management, API-app CRUD, unclaimed
drafts, bulk-send, fax, and OAuth-app admin. These can be added later; the MVP
is the send→track→download loop that an assistant actually drives.

**Scope coverage of this surface (resolved in §3).** Two of the jobs above are
not covered by the app-owner `request_signature` scope and force the scope/billing
decision up front: the template jobs (`template list/get`, `send-with-template`)
live **only** behind the user-charged `template_access` scope, and
`signature-request list` is bounded by what the OAuth token itself created. §3
settles the scope set so every job listed here is actually authorized; do not
read this table as "all covered by `request_signature`."

**Two implementation nuances that shape the service** (both verified against the
send reference):

1. **`send` is multipart, not JSON.** `POST /signature_request/send` accepts
   documents either as **file uploads** (`files[]`, multipart/form-data) or as
   **remote URLs** (`file_urls[]`) — plural array params per the current official
   send reference (the legacy singular `file[]`/`file_url[]` names are wrong; do
   **not** use them). The endpoint requires **one or the other, not both**. The
   service exposes both: `--file <path>` (repeatable → multipart `files[]` upload)
   and `--file-url <url>` (repeatable → `file_urls[]`, simpler for an agent that
   already has a link). `send-with-template` needs neither — it references a
   stored template plus signer roles.
2. **`test_mode`.** Requests carry an optional `test_mode` **boolean**
   (`type: boolean, default: false`; send `test_mode=true`, **not** the legacy
   integer `1`) that creates non-legally-binding, watermarked requests and is the
   mode used **before app approval** (see §4). The service exposes `--test-mode`
   so L2/L4/L5 can run against real endpoints without spending signature quota or
   requiring a paid plan for the exercise. (Production, non-test calls
   additionally require a paid Dropbox Sign plan — the API returns **402** without
   one; a lane-1/account-pool constraint, noted in §7.)

---

## 2. anycli definition & service

**Tool form: `service` type.** No official Dropbox Sign CLI exists (the
`@dropbox/sign` SDKs are language libraries, not an agent-friendly `--json`
binary that can be provisioned into the runtime image), so the stage-1 rubric
lands on `service`, matching 21/23 shipped definitions and the eSign precedents
(docusign, adobe-sign, signnow, boldsign are all service-type in this program).

- **Definition:** `definitions/tools/dropbox-sign.json`
  ```json
  {
    "name": "dropbox-sign",
    "type": "service",
    "description": "Dropbox Sign (HelloSign) e-signature: send, track, and download signature requests (OAuth token)",
    "auth": {
      "credentials": [
        {
          "source": {"field": "access_token"},
          "inject": {"type": "env", "env_var": "DROPBOX_SIGN_ACCESS_TOKEN"}
        }
      ]
    }
  }
  ```
  anycli stays credential-agnostic: it only declares the field name
  (`access_token`, supplied by Helio's resolver) and how to deliver it. Dropbox
  Sign accepts the OAuth token as `Authorization: Bearer <access_token>`; the
  service reads `DROPBOX_SIGN_ACCESS_TOKEN` and sets that header. (The same v3
  API also supports HTTP Basic with an API key as username — irrelevant here;
  Helio's path is OAuth Bearer only.)

- **Service package:** `internal/tools/dropboxsign/` (Go package name drops the
  dash per the master plan's stage-2 rule; only the definition filename and the
  `RegisterService("dropbox-sign", …)` string carry the exact dashed id).
  Registered in `internal/tools/register.go`'s `init()`.

- **Shape (copy `internal/tools/notion/`):** cobra tree grouped by resource
  (`signature-request`, `template`, `account`) with a `BaseURL`/`HC`/`Out`/`Err`
  struct so httptest fakes can point at a fake server and capture output; the
  documented exit-code contract (0 success, 1 runtime/API failure via a typed
  `apiError`, 2 usage/parse) and a `--json` structured-error envelope.

- **JSON output shape:** provider-neutral passthrough of the Dropbox Sign
  response objects, `--json` for machine consumption. `signature-request get`
  returns the `signature_request` object (`signature_request_id`, `title`,
  `is_complete`, `is_declined`, `signatures[]` with `signer_email_address` /
  `status_code` / `signed_at`, `files_url`, `details_url`). `list` returns
  `{signature_requests: […], list_info: {page, num_pages, num_results,
  page_size}}` — the service surfaces `list_info` so an agent can paginate
  (`--page`, `--page-size`). `files` streams bytes to `--out <path>` (or stdout)
  and prints a small JSON descriptor. Dropbox Sign errors come back as
  `{"error": {"error_msg": "...", "error_name": "..."}}` on 4xx/5xx — the
  service maps these into the typed `apiError` so both plain-text and `--json`
  error rendering match the notion contract.

- **TDD (L1):** `dropbox-sign_test.go` builds an `httptest.Server` fake and
  asserts request shape (multipart body for `send` with `files[]`; form body +
  `file_urls[]` variant; `Authorization: Bearer` header; `test_mode=true` when
  `--test-mode`), pagination flag wiring on `list`, byte streaming on `files`,
  and both text and `--json` error envelopes. Never hits the real API. **Caveat:**
  an httptest fake accepts whatever field names the service sends, so L1 alone
  cannot catch a wrong `files[]`/`file_urls[]` name or the raw array-field
  encoding (`files[0]` vs `files[]`) — that exact multipart wire contract is an
  explicit L2 gate against the live endpoint (§6), not something L1 can validate.

---

## 3. Credential fields & the exact OAuth flow (oauth_review lane — verified)

**Registration model — why `oauth_review` is correct.** Dropbox Sign OAuth apps
are created self-serve in the API dashboard, but **any app implementing an OAuth
workflow must pass the Support Assisted Approval process before it can be used in
production by external users** (official app-approval guide). The reviewer
verifies a *complete, working* Dropbox Sign workflow (mockups are rejected; the
demo must show the full user-experienced flow) and "won't reject any apps
without direct contact." This is a human review gate on external multi-tenant
use → **`oauth_review`, confirmed**, matching the audit. Critically for
scheduling: **dev/test-mode apps work before approval** (approval only lets you
drop `test_mode`), so — exactly as the master plan's lane-1 model assumes —
dev-app creation front-runs the batch and gates L4/L5, while **review clearance
gates only the visible flip**, nothing upstream.

**OAuth 2.0 endpoints & params (verified against the OAuth walkthrough):**

- **Authorize:** `https://app.hellosign.com/oauth/authorize`
  query: `response_type=code`, `client_id`, `state`, `redirect_uri`.
- **Token exchange:** `POST https://app.hellosign.com/oauth/token`
  form body: `grant_type=authorization_code`, `client_id`, `client_secret`,
  `code`, `state`. → **`token_exchange_style: form_secret`** (client id/secret
  and fields in the form body; no HTTP Basic, no PKCE).
- **Token response:**
  ```json
  { "access_token": "...", "token_type": "Bearer",
    "expires_in": 3600, "refresh_token": "...", "account_id": "..." }
  ```
  Access tokens expire (**`expires_in: 3600`**, ~1 hour); a `refresh_token` is
  returned automatically on the first exchange (no `access_type=offline`-style
  opt-in needed). So the token gateway's refresh-and-write-back path is exercised
  and must work.
- **API auth:** `Authorization: Bearer <access_token>` on every `api.hellosign.com/v3` call.

**Scopes (verified — 7 exist, split into two _mutually exclusive_ billing models;
an app picks one model at creation, so its scopes cannot be mixed across models):**
- **App-owner-charged** ("Charge me" — Helio's app account is billed and *is* the
  actor): `basic_account_info` (email/name), `request_signature` ("send requests,
  access statuses and document files"). **No template scope exists in this
  model**, and every request/template the assistant touches belongs to Helio's
  app account, not the connecting user.
- **User-charged** ("Charge users" — the connecting user's paid API plan is billed
  and the *user* is the actor): `account_access` (email/name),
  `signature_request_access` (send/view/update requests + download files),
  `template_access` ("view, create, and modify templates"), `team_access`,
  `api_app_access`.

  **Recommended MVP scopes: `account_access` + `signature_request_access` +
  `template_access` (user-charged model).** This is the set that actually covers
  the full §1 surface *and* is coherent for an AI teammate — it is the correct
  model, not merely the richer one:
  - **Templates force it.** The §1 job list includes `template list/get` and
    `send-with-template`. `/template/*` sits **only** behind `template_access`,
    which exists **only** in the user-charged model; `request_signature` returns
    **403** on those paths. The app-owner set therefore cannot ship §1 as written.
  - **Actor semantics force it.** An AI teammate connects the **user's** Dropbox
    Sign account and acts *as that user* (`owner: individual`, §4). Under
    user-charged scopes the assistant sends from, lists, and reads the connecting
    user's own account and their templates — which is exactly what "send using a
    saved reusable template" means. Under app-owner `request_signature` the
    assistant would instead act as Helio's app and could only ever see Helio's
    own templates/requests, making the template jobs incoherent for a real
    teammate.
  - **`signature-request list` caveat.** `signature_request_access` grants
    list/view/download, but the docs state signature requests "must be made with
    [the] oAuth token in order to access." So `GET /signature_request/list`
    reflects requests the assistant created **through this connection**, not the
    user's entire pre-existing history. That is fine for an assistant status
    board, but the surface must not be described as whole-account history. (This
    is also why `request_signature` alone was never a safe basis for `list` —
    there is no source granting account-wide listing beyond OAuth-token-created
    requests.)

  **Billing tradeoff (explicit stage-1 product decision).** User-charged requires
  the *connecting user* to hold a paid Dropbox Sign API plan (§7.3) — the same
  paid-plan requirement app-owner billing would place on Helio's account, just
  moved to the party whose account is actually being used, which is the right
  place for it. The **only** way to reach the app-owner `basic_account_info` +
  `request_signature` set is to **drop** `template list/get` **and**
  `send-with-template` from the §1 surface (and accept app-as-actor semantics) —
  that is a surface reduction decided at stage 1, not a drop-in scope swap.

  **Changing an app's scopes revokes all existing user authorizations** (forces
  re-consent), so the scope set must be settled before the visible flip, not
  iterated afterward. `display_scopes` mirror the reviewed set for the consent
  card.

**Credential fields:** the bundle declares only
`required_config_fields: [oauth.client_id, oauth.client_secret]`; the real
client id/secret land in integration-service config (`config/` + `deploy/` Helm
Secret, lane 1, per Config Sync). The user token never touches the bundle — it
arrives via the OAuth callback and is stored in Vault; anycli receives only
`access_token` at run time.

### 3.1 The one real capability question: the refresh endpoint quirk

Dropbox Sign documents the **refresh** call at a **different URL than the token
URL**, carrying a query param. The official OAuth walkthrough spells it out
explicitly (verified 2026-07-22) — `?refresh` is shown as part of the URL, not as
optional:

```
curl -X POST 'https://app.hellosign.com/oauth/token?refresh' \
  -F 'grant_type=refresh_token' \
  -F 'refresh_token=<...>'
```

**Two provider-specific deviations live on this one call, not one.** Besides the
`?refresh` URL suffix, the documented refresh curl also **omits `client_id` and
`client_secret` entirely** — unlike the initial code exchange, which sends both
(verified 2026-07-22: the walkthrough's auth-code `curl` carries `client_id` +
`client_secret`; its refresh `curl` carries only `grant_type` + `refresh_token`).
This matters because `token_exchange_style: form_secret` injects `client_id` /
`client_secret` into **every** grant, including refresh. Most OAuth servers ignore
extra form fields, but that is **unverified for Dropbox Sign**, so it is a second
L2 sub-check, not an assumption.

The `standard_oauth` runtime strategy's `standardOAuthExchanger` uses a single
`oauth.token_url` for *both* the initial code exchange and refresh (a plain
`grant_type=refresh_token` POST to `token_url`, no `?refresh`), and under
`form_secret` sends `client_id`/`client_secret` on both. So the decisive
stage-1/L2 gate has **two axes**: (a) **is the documented `?refresh` suffix
mandatory**, or does a plain `POST https://app.hellosign.com/oauth/token` with
`grant_type=refresh_token` (no suffix) also succeed? and (b) **does the refresh
grant tolerate `client_id`/`client_secret` in the body** (as `form_secret`
sends), or does it require them omitted?

**The documentation points at Option B.** The official walkthrough documents
`?refresh` explicitly as part of the refresh URL, with no indication it is
optional — so implementers should **plan for Option B (the reviewed-field growth
below) as the likely path**, and treat Option A as the exception that must be
*proven* at L2, not assumed.

- **Option B (documented path — one reviewed capability field):** if `?refresh`
  is mandatory (as the docs show), grow `standard_oauth` by one reviewed field —
  `oauth.refresh_url` (a full-URL override, e.g.
  `https://app.hellosign.com/oauth/token?refresh`, that `standardOAuthExchanger`
  uses for the refresh grant when present, falling back to `token_url`) — added
  to `provider-gen`'s closed field/enum contract with a unit test. This is the
  same "grow one reviewed enum/field rather than fork an adapter" move that
  adobe-sign (shard exchange) and salesforce (`instance_url` capture) used in
  this program. It is **not** a compiled `service/adapter_*.go` — Dropbox Sign's
  response shape is otherwise standard, so no adapter is warranted. Budget for
  this field growth in planning rather than hoping to avoid it.
- **Second reviewed-capability consideration (credential omission on refresh):**
  if L2 sub-check (b) shows the refresh grant **rejects** `client_id`/`client_secret`
  in the body (i.e. `form_secret`'s injection breaks refresh and the fields must be
  omitted), that is a **second** reviewed-capability growth beyond `oauth.refresh_url`
  — a flag on the refresh path telling `standardOAuthExchanger` to drop the secret
  pair for the refresh grant only. Budget for the *possibility* of both fields, and
  keep it distinct from the URL override — do not fold "omit the secret" into
  `oauth.refresh_url`, they are orthogonal axes. If sub-check (b) shows the extra
  fields are tolerated (the common case), no growth here and `form_secret` stands.
- **Option A (only if L2 proves it — zero platform code):** if the base
  `token_url` turns out to accept the refresh grant *without* the `?refresh`
  suffix (the query param behaving as a no-op) **and** tolerates the injected
  `client_id`/`client_secret`, then `standard_oauth` works unchanged and both
  `oauth.refresh_url` and the credential-omission flag are omitted. This is the
  strictly better outcome, but the docs give no reason to expect it — so it must be
  *demonstrated* at L2 (mint a token, force-expire it, refresh through the plain
  `token_url`, confirm a new access token comes back), never pinned on assumption.

The DESIGN's recommendation: **plan for Option B (the documented `?refresh`
path); run the L2 refresh assertion — both sub-checks (a) the `?refresh` suffix
and (b) the `client_id`/`client_secret` omission — before the anycli pin, and
fall back to Option A's plain, unchanged `standard_oauth` only if L2 proves the
suffix is a no-op *and* the injected secrets are tolerated.** Flagging both axes
at stage 1 (not mid-wave) per the master plan's non-standard-auth-shape risk
guidance.

---

## 4. Helio provider bundle plan (`integrations/providers/dropbox_sign/`)

Hidden-first. `provider.yaml`:

```yaml
schema: helio.provider/v1
key: dropbox_sign
go_name: DropboxSign

presentation:
  name: Dropbox Sign
  description_key: dropbox_sign
  consent_domain: app.hellosign.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <batch-assigned>

auth:
  type: oauth
  owner: individual         # the provider authenticates a person; connection belongs to the assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.hellosign.com/oauth/authorize
    token_url: https://app.hellosign.com/oauth/token
    token_exchange_style: form_secret
    pkce: none
    scopes: [account_access, signature_request_access, template_access]        # user-charged model — covers the full §1 surface incl. templates (see §3)
    display_scopes: [account_access, signature_request_access, template_access]
    single_active_token: false
    refresh_lease: none
    # refresh_url: https://app.hellosign.com/oauth/token?refresh
    #   ^ §3.1: the official docs document ?refresh explicitly, so plan to ship this
    #     (Option B — requires first growing standard_oauth's reviewed oauth.refresh_url
    #     field in provider-gen). Drop it ONLY if the L2 gate proves a plain token_url
    #     refresh (no ?refresh) succeeds.

identity:
  source: userinfo
  url: https://api.hellosign.com/v3/account
  stable_key: /account/account_id
  label_candidates: [/account/email_address, /account/account_id]

connection:
  mode: isolated
  disconnect_mode: local_only     # no documented OAuth token-revocation endpoint
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
  name: dropbox-sign      # axis ② — must equal the anycli definition once visible
  kind: oauth
```

Decisions & rationale:

- **Identity via `userinfo` (`GET /v3/account`), not token_response.** The token
  response carries `account_id`, which would suffice for a stable key with zero
  extra calls — but it is an opaque UUID and would make an ugly connection label.
  `GET /v3/account` (bearer-authenticated, no query params → returns the
  *authenticated* account; authorized by `account_access` in the recommended
  scope set) yields `{account: {account_id, email_address, …}}`,
  giving a real email label. `stable_key: /account/account_id`,
  `label_candidates: [/account/email_address, …]`. This is the standard
  `declarativeIdentityResolver` path (same as gmail/linkedin) — **no capability
  growth**.
- **`disconnect_mode: local_only`.** Dropbox Sign publishes no OAuth
  token-revocation endpoint (revocation is user-driven from Dropbox Sign account
  settings), so Helio disconnect drops the local connection only — matching the
  notion precedent. (Low-risk verification item; `local_only` is the safe default
  if no revoke URL is confirmed.)
- **`runtime_strategy: standard_oauth`** — subject to §3.1. `form_secret` +
  `pkce: none` + auto-returned `refresh_token` are all inside the existing
  `standard_oauth` capability set. The *only* thing that could push a reviewed
  capability field is the refresh-URL quirk (§3.1 Option B); nothing else here is
  provider-specific.

**Axis ②↔③ divergence → resolver entry.** `dropbox-sign` (id) ≠ `dropbox_sign`
(key) is a mechanical dash↔underscore pair, so it needs one line in
`helio-cli/internal/toolcred/resolver.go` `toolToProvider`:
`"dropbox-sign": "dropbox_sign"` (one of the master plan's 23 mechanical pairs).
If open-question-1's mechanical id→key normalization has landed by dev time, this
entry is omitted (the normalizer handles dash→underscore) — the resolver map is
reserved for true irregulars. Coordinate with the settled resolver contract
before writing the entry.

**Config sync (lane 1).** `oauth.client_id` / `oauth.client_secret` for
`dropbox_sign` land together (never partially — a partially configured provider
fails integration-service startup) in `config/` and the `deploy/` Helm Secret,
before this provider's L5 run. All-absent renders `configured: false` and is
safe to ship hidden.

**No `experiment` gating** (GA lane, not a design-090 preview tool).

---

## 5. Adjacent artifacts (batch-end merge, listed for completeness)

- **UI icon:** `ui/helio-app/src/integrations/icons/dropbox_sign.svg` + manual
  registration in `providerIcons.ts` (Dropbox Sign wordmark/glyph — distinct
  from the Dropbox storage icon). Never generated.
- **AI-facing doc:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/` describing the `dropbox-sign` verbs, the
  send-file-vs-file-url choice, `--test-mode`, and the send→track→download loop;
  rides the one-per-batch plugin version bump + marketplace publish.
- **Generation:** `dropbox_sign` bundle joins the single batch-end `provider-gen`
  run; the five projections are committed together by the batch lead. On-branch,
  validate with a local `provider-gen --check` (do **not** commit the regen).

---

## 6. Test plan → the five layers

| Layer | Dropbox-Sign-specific plan | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fake asserts: multipart `files[]` upload on `send`; `file_urls[]` form variant; `Authorization: Bearer`; `test_mode=true` under `--test-mode`; `list` pagination (`list_info`); `files` byte streaming to `--out`; typed `apiError` from `{"error":{"error_name",...}}` in text and `--json`. (Fake accepts any field names → the exact `files[]`/`file_urls[]` name + array-index encoding is an L2 gate, not provable here.) | No |
| **L2** harness real API | `ANYCLI_CRED_ACCESS_TOKEN=<tok> anycli dropbox-sign -- account get`, then `signature-request send --test-mode --file-url <url> --signer ...` → `signature-request get <id>` → `signature-request files <id> --out /tmp/x.pdf`. **Confirm the live multipart wire contract** before the anycli pin: the real endpoint accepts `files[]` / `file_urls[]` (plural) and the exact array-field encoding the service emits (`files[0]` vs `files[]`) — the one detail unit fakes cannot validate. **Also the §3.1 refresh gate (two sub-checks):** (a) whether the documented `?refresh` suffix is mandatory (Option B, expected) or a no-op on the plain `token_url` (Option A); **and (b)** whether the refresh grant succeeds with `client_id`/`client_secret` present in the body (what `form_secret` injects) or requires them **omitted** — the official refresh curl sends neither (see §3.1). | **Yes** — a real Dropbox Sign account + OAuth token from the dev app; `--test-mode` avoids quota/paid-plan spend |
| **L3** generate + suites | `provider-gen --check` green with the bundle; `helio-cli` + integration-service unit suites; if §3.1 Option B, the new `oauth.refresh_url` field's generator + exchanger unit tests. | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` provider `dropbox_sign` with a real `access_token` **and** `refresh_token` + short `expires_at` (force the gateway refresh path — this is where §3.1 must already be correct), then `heliox tool dropbox-sign -- account get` reaches the live API through the token gateway. | **Yes** — real seed token (oauth provider is seedable; not a minted provider) |
| **L5** full connect flow | Once, hidden, before flip: `heliox tool dropbox-sign auth` → complete Dropbox Sign OAuth consent on the **dev/test-mode app** → confirm `oauth_connected` system event → run one unseeded `dropbox-sign -- account get`. Human-in-the-loop (oauth L5). | **Yes** — live consent on a real account (human lane 3) |

**Lane gating recap:** dev-mode app creation (lane 1) gates L4/L5 tokens;
**Support Assisted Approval clearance gates only the `visible: true` flip**, not
dev, L4, or the batch-end merge. Account-pool note: production (non-`test_mode`)
signature calls need a **paid** Dropbox Sign plan — L2/L4/L5 stay in `test_mode`
to avoid that dependency; only a post-approval production smoke would need paid
quota.

---

## 7. Risks / open decisions (flag at stage 1)

1. **Refresh-call quirks (§3.1) — two axes, not one.** The documented refresh
   call deviates from the code exchange on **two** points: (a) it POSTs to
   `…/oauth/token?refresh` (query suffix), and (b) it **omits `client_id`/`client_secret`**
   that `form_secret` would inject. Plan for Option B on axis (a) — one reviewed
   `oauth.refresh_url` field, since the docs show `?refresh` explicitly — and treat
   axis (b) as a **separate potential** reviewed-capability (a refresh-only
   secret-omission flag) that only materializes if L2 shows the injected secrets
   break refresh. Both axes must be proven at L2 before the anycli pin; fall back to
   plain unchanged `standard_oauth` only if L2 proves the suffix is a no-op **and**
   the secrets are tolerated.
2. **Scope/billing model (§3)** — the full §1 surface (`template list/get` +
   `send-with-template`) forces the **user-charged** model (`account_access` +
   `signature_request_access` + `template_access`): the app-owner
   `request_signature` set grants **no** template access (403 on `/template/*`)
   and makes the assistant act as Helio's app rather than the connecting user.
   **MVP recommends user-charged.** The only route to the app-owner model is to
   drop the template jobs from §1 — a surface reduction, not a scope swap. Must
   be settled pre-flip (scope change revokes consents).
3. **Paid-plan requirement for production calls** — under the recommended
   user-charged model this is the *connecting user's* paid Dropbox Sign API plan
   (not Helio's); account-pool/L-layer budget item; `test_mode` covers all
   pre-flip layers.
4. **`local_only` disconnect** — confirm no OAuth revoke endpoint exists;
   `local_only` is the safe default regardless.
5. **Brand/host split** — keep `dropbox_sign` (hellosign.com host) strictly
   separate from `dropbox` (dropbox.com); flat command, no group.

## 8. Sources (official, verified 2026-07-22)

- App approval (oauth_review gate): `https://developers.hellosign.com/docs/guides/app-approval/overview`
- OAuth walkthrough (authorize/token/refresh URLs, params, token response, `expires_in: 3600`): `https://developers.hellosign.com/docs/guides/o-auth/walkthrough.md`
- OAuth overview (7 scopes + billing models, Bearer header, scope-change revocation): `https://developers.hellosign.com/docs/guides/o-auth/overview.md`
- Get Account (identity endpoint `GET /v3/account`, Bearer/Basic auth): `https://developers.hellosign.com/api/reference/operation/accountGet`
- Send Signature Request (multipart `files[]` / `file_urls[]` — plural arrays; `test_mode` boolean `type: boolean, default: false`; 402 without a paid plan for production; security `request_signature`/`signature_request_access`): `https://developers.hellosign.com/api/signature-request/send` (OpenAPI-derived reference; the `/api/reference/operation/signatureRequestSend/` path now 404s)
