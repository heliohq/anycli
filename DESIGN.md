# Melio — per-tool design (`heliox tool melio`)

Scratch planning doc on branch `tool/melio`. Batch lead strips it at batch-end.

- **anycli id (axis ②):** `melio`
- **provider catalog key (axis ③):** `melio`
- **CLI command word (axis ①):** `melio` (flat, ungrouped)
- **Auth lane:** `oauth_review` · **Wave:** 3 · **Category:** Payments & Commerce
- **Catalog row:** 179 of `008-300-integrations-rollout-plan.md`
- **Tool form:** `service` type (no official Melio CLI; HTTP API only)

Melio (meliopayments.com) is a US-only B2B bill-pay / accounts-payable + accounts-receivable
platform: enter bills, resolve vendors, choose a funding source, schedule a payment
(ACH / card / wire / paper check), and send/track invoices. It exposes an embedded
**partner** API for platforms.

---

## 0. Research constraint (read first) — official docs are partner-gated

**I could not reach Melio's official API reference from this environment.** The developer
portal (`docs.melio.com`, `developer.melio.com`) is behind a **Cloudflare WAF that hard-blocks**
this network — not a solvable JS challenge but an outright "you have been blocked" (verified with
a real Chrome via the chrome-cdp skill, Ray ID `a1f5e6791b35cf1a`, and with browser-header curl:
HTTP 403). The portal is additionally **partner-gated**: full reference + `client_id`/`client_secret`
require a Melio partner developer account. Every marketing/partner surface
(`meliopayments.com/partners`, the LLM-info page) routes to a "Become a partner" lead form rather
than self-serve docs, and third-party API aggregators list the endpoints as empty.

Consequence for this design: the **auth *shape*** and the **bundle *strategy*** below are grounded
in facts I could confirm from official/first-party sources (listed in §7); the **exact host,
`authorize`/`token` URLs, scope strings, refresh semantics, and resource paths are NOT publicly
verifiable from here** and are marked `‹stage-1›` throughout. They MUST be captured from the Melio
partner developer account at pipeline **stage 1** (before the anycli dev branch writes request
shapes), exactly the account that lane-1 dev-mode app creation provisions for an `oauth_review`
tool. This is not a blocker for *planning* — it is the normal front-loaded dependency the master
plan §2/§5 assigns to `oauth_review` lane-1. It **is** a hard gate on writing L1/L2 code (do not
guess Melio's request shapes) and on L4/L5.

**Recommendation:** keep Melio on its Wave-3 schedule but flag it at stage 1 as
"docs + credentials both partner-gated" so lane-1 provisions the partner dev account one batch
ahead. If, at stage 1, the partner API turns out to have **no self-serve/sandbox path at all**
(pure managed-partnership, à la the risk bullets that pushed Bill.com to the 3-hold batch), raise a
catalog-amendment PR to move Melio to 3-hold or swap it — do not force an unrunnable L2/L4.

---

## 1. Auth-lane verification — independent, against the official rubric

The master plan lists Melio as `oauth_review`. **Melio is absent from the OAuth audit table**
(`oauth-audit.md`) because that audit only re-examined the 250 tools that sat in `api_key` *before*
2026-07-21; Melio was already `oauth_review` pre-audit, so no verdict row exists. I therefore
re-derived the lane from first principles against the audit rubric ("multi-tenant
authorization-code OAuth; self-serve → `oauth_light`; human review / partner-program / verification
gate → `oauth_review`; per-instance/partner-only/absent → `api_key`"):

- **Multi-tenant authorization-code OAuth exists.** Confirmed: Melio's developer platform advertises
  "Authentication", "SSO / Social login", and an "OAuth playground" (apitracker.io listing), and a
  third-party connector describes the flow as "approve on Melio's authorize screen … the app holds
  only a scoped token it can refresh." So one registered partner app + per-customer authorize +
  scoped refreshable tokens → **not** `api_key`.
- **A partner/review gate precedes external authorization.** Melio is an embedded-finance
  money-movement platform; client credentials are issued only after partner onboarding (the
  "Become a partner" application + agreement), and its docs are gated behind that partnership. This
  is the textbook `oauth_review` shape (peer to Stripe/Ramp/Mercury/Brex in the audit: OAuth is real
  but client registration is partner-reviewed, not self-serve).

**Verdict: `oauth_review` is correct.** No divergence from the catalog. (Recorded here per the task's
"record the divergence in DESIGN.md" instruction — there is none; the only note is that the audit
table's silence is *expected*, not an omission.) The review gate blocks only the **visible flip**
(master plan §2), never dev / L4 / batch-end merge; Melio ships **code-complete but hidden** in its
Wave-3 batch.

---

## 2. Which official API surface this tool wraps, and why

Driven by what an **AI teammate** actually does with Melio — "pay this vendor", "what do we owe
this month", "chase this unpaid invoice" — not by mirroring the whole partner API. Melio's domain
model (confirmed from partner-page copy + accounting-sync docs) is **organizations/accounts →
vendors → bills → payments (+ funding sources) → invoices**. The AI-relevant, mostly read-plus-
scoped-write surface:

| Resource | AI teammate use | Verbs (planned) |
|---|---|---|
| **Bills** (accounts payable) | "list unpaid bills", "what's due this week", "show bill X" | `bill list`, `bill get`, (`bill create` — write, gated) |
| **Vendors** | "who is vendor X", "list vendors", resolve a payee before paying | `vendor list`, `vendor get`, (`vendor create`) |
| **Payments** | "show payment status", "list scheduled payments", (schedule a payment) | `payment list`, `payment get`, (`payment create` — money movement, see §3 safety) |
| **Funding sources** | pick which bank/card a payment draws from | `funding-source list` |
| **Invoices** (accounts receivable) | "list open invoices", "which invoices are overdue", "resend invoice X" | `invoice list`, `invoice get` |
| **Account / organization** | identity + connection labeling; L5 verify | `account get` (whoami) |

**Why these and not more:** they are the nouns an AP/AR teammate reasons over, they map 1:1 onto
provider-neutral JSON an agent can consume, and read verbs are safe to ship first. Webhooks (the
platform advertises them) are **out of scope** for a `heliox tool` passthrough — heliox is
request/response, not a subscriber; ingest belongs to a Helio service, not this tool.

Exact paths/versioning are `‹stage-1›` (likely `https://api.meliopayments.com/v{n}/...`, unverified).

---

## 3. anycli definition

**Type: `service`** — no official Melio binary exists (rules out `cli` type per SKILL.md stage-1
rubric); implement in `internal/tools/melio/` against the HTTP API. Package name `melio`
(id has no dashes/leading digit → no normalization).

- `definitions/tools/melio.json`: `name: "melio"`, `type: "service"`, one-line description, and a
  single `auth` credential binding — `access_token` (field) injected as an env var
  (`MELIO_ACCESS_TOKEN`, `type: env`) that the service reads and sends as
  `Authorization: Bearer <token>` (exact header/scheme `‹stage-1›`; Bearer is the OAuth default).
  Partner apps commonly also require a static `x-account-...`/partner header — **if** stage-1
  confirms one, add it as a second binding sourced from a config field, not a user credential.
- `internal/tools/melio/`: cobra tree grouped by resource, copying the **notion service** shape
  (the reference impl per anycli-development.md): `BaseURL`/`HC`/`Out`/`Err` struct so tests point
  at an `httptest` server; exit-code contract 0 success / 1 API-or-runtime failure (typed
  `apiError`) / 2 usage; `--json` on every subcommand emitting a structured envelope (and a
  structured `--json` error envelope). Non-interactive, `--json`-first, agent-consumable.

**Subcommands (verbs):** `bill list|get`, `vendor list|get`, `payment list|get`, `funding-source
list`, `invoice list|get`, `account get`. Money-moving writes (`payment create`, `bill create`)
are **deferred to a second pass** — Melio is real-money AP; ship read-first, and gate any
create/schedule verb behind explicit confirmation semantics and a stage-1 review of Melio's
idempotency-key requirement before enabling.

**JSON output shape:** provider-neutral, matching the sibling tools — top-level `{ "data": [...] }`
for list verbs, `{ "data": {...} }` for get verbs, pagination surfaced as flags
(`--limit`/`--cursor` per Melio's real paging, `‹stage-1›`), and the `--json` error envelope on
failure. No raw Melio response passthrough.

---

## 4. Credential fields & the exact auth flow (oauth_review)

**Registration model:** one Melio **partner app** (Helio-owned), registered via Melio's partner
onboarding — **reviewed**, not self-serve; yields `client_id` + `client_secret`. Dev/sandbox app
creation is expected to precede review (standard for `oauth_review`), which is what makes L4 runnable
before the visible flip — but this needs stage-1 confirmation that Melio offers a dev/sandbox tier.

**Flow (authorization code):**
1. `heliox tool melio auth` mints a connect intent; user is redirected to Melio's
   `authorize` endpoint `‹stage-1›` with `client_id`, `redirect_uri`, `scope`, `state`
   (+ PKCE if Melio supports it — `‹stage-1›`, default `pkce: none` until confirmed).
2. User consents on Melio's authorize screen (per-organization); Melio calls back with `code`.
3. integration-service exchanges `code` at the `token` endpoint `‹stage-1›` for
   `access_token` (+ `refresh_token`, since third-party sources confirm the token is "refreshable").
4. Token gateway serves `access_token` to the resolver; anycli injects it as `MELIO_ACCESS_TOKEN`.

**Credential fields the bundle declares** (never real values — those go in integration-service
config): `required_config_fields: [oauth.client_id, oauth.client_secret]`. If stage-1 reveals a
partner-level static header/key, add it as one more required config field (not a user credential).

**Token semantics:** scoped, refreshable access token; expiry + refresh-token rotation `‹stage-1›`.
The bundle `refresh_lease` value (`credential` if Melio rotates refresh tokens per-refresh, else a
non-rotating variant) is decided once stage-1 confirms the token response — same axis the Xero /
Sage / FreshBooks bundles already exercise, so **no new integration-service capability is expected**
(the `standard_oauth` `refresh_lease` allowed-set already carries these values). Confirm at stage 1
before assuming reuse.

**Scopes:** Melio's scope strings are `‹stage-1›`. `display_scopes` in the bundle will list the AP/AR
read (+ later write) scopes actually granted.

---

## 5. Helio provider bundle plan (hidden-first)

`integrations/providers/melio/provider.yaml`, modeled on the `notion` standard_oauth bundle
(directory name = key = `melio`; generator enforces equality). Skeleton (values marked `‹stage-1›`
are filled once the partner docs are readable):

```yaml
schema: helio.provider/v1
key: melio
go_name: Melio

presentation:
  name: Melio
  description_key: melio
  consent_domain: meliopayments.com            # ‹stage-1› confirm authorize host
  visible: false                               # hidden-first (SKILL.md stage 4/10)
  order: <next>

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: ‹stage-1›
    token_url: ‹stage-1›
    token_exchange_style: form_secret            # ‹stage-1› (form_secret|form_basic|json_basic)
    pkce: none                                   # ‹stage-1›
    display_scopes: [ ‹stage-1› ]
    single_active_token: false
    refresh_lease: ‹stage-1›                     # credential | none, per token rotation

identity:
  source: token_response                         # or userinfo (account GET) — ‹stage-1›
  stable_key: ‹stage-1›                          # e.g. /organizationId or /accountId
  label_candidates: [ ‹stage-1› ]                # e.g. /businessName, /email

connection:
  mode: isolated
  disconnect_mode: local_only                    # or declarative revoke if Melio has a revoke endpoint
  runtime_strategy: standard_oauth               # golden path — zero provider Go expected

resources: { selection: none, discovery: none, enforcement: none }

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: melio
  kind: oauth
```

- **Three axes:** ① `melio` ② `melio` ③ `melio` — all identical, so **no `toolToProvider`
  resolver entry** (identity holds) and no grouped `tool.group`.
- **runtime_strategy `standard_oauth`.** Melio is a standard authorization-code + refreshable-Bearer
  provider — the golden path composes the exchanger + declarative identity resolver + revoker with
  **zero provider-specific Go**. Reach for a `service/adapter_melio.go` **only** if stage-1 uncovers
  a non-standard response dialect (200-with-error, partner-header token exchange, per-org base URL).
  Flag that possibility at stage 1; do not pre-build an adapter.
- **Config Sync:** `oauth.client_id`/`oauth.client_secret` land in **both** `config/` and the
  `deploy/` Helm Secret together (partial config fails integration-service startup) — lane-1's
  landing, before Melio's L5. A fully-absent config renders `configured: false` (safe hidden).
- **UI icon:** `ui/helio-app/src/integrations/icons/melio.svg` + register in `providerIcons.ts`
  (manual, never generated) + i18n label for `description_key: melio`.
- **AI-facing doc:** provider sub-doc under `agents/plugins/heliox/skills/tool/`, published on the
  batch-end plugin version bump.

---

## 6. Test plan — five layers

| Layer | Melio-specific plan | External creds needed? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: `httptest` fake for each verb (bill/vendor/payment/funding-source/invoice/account); assert request path, injected `Authorization: Bearer`, plain + `--json` error envelopes, exit codes. No live API. | No |
| **L2** harness real-API | `ANYCLI_CRED_ACCESS_TOKEN=<partner-sandbox token> anycli melio -- bill list` etc. against Melio's **real sandbox** — the mandatory gate before pinning. **Blocked on the partner developer/sandbox account** (§0). | **Yes** — partner sandbox account + token (lane 2 / lane 1) |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` against the `melio` bundle; `helio-cli` + integration-service unit suites. Point `helio-cli/go.mod` at the anycli branch via local `replace` (uncommitted). | No |
| **L4** singleton + seeded token | Start singleton (`env: dev`); `POST /internal/test-only/connections/seed` a **real** access_token (+ refresh_token, short `expires_at` to force the refresh path) for provider `melio` against a real seeded assistant/org; run `heliox tool melio -- bill list` through the real token gateway. Success = live Melio data returned. | **Yes** — real access token from the registered dev app (lane 1) |
| **L5** full connect flow | `heliox tool melio auth` → Melio authorize consent on the dev/sandbox app → `oauth_connected` event → unseeded live `heliox tool melio -- account get`. Human-in-the-loop (`oauth_review` → lane 3). Runs once, still hidden, before the visible flip; visible flip **also** gated on Melio partner-review clearance. | **Yes** — human consent on a real Melio account + review clearance for flip |

Credential-gated layers: **L2, L4, L5** — all depend on lane-1 partner app creation and lane-2 test
account, which for Melio are the same partner-onboarding gate. L1/L3 are agent-runnable now (with
`‹stage-1›` request shapes resolved first).

---

## 7. Sources & open items

**Confirmed (first-party / official):**
- Melio partner/embedded platform, AP+AR object model (bills, vendors, payments, funding sources,
  invoices, accounting sync w/ QuickBooks/Xero/NetSuite): `meliopayments.com/partners`,
  `meliopayments.com/llm-info/`, `meliopayments.com/accounts-payable/`.
- Developer platform exists (API Reference, Webhooks, Sandbox, Authentication, SSO/Social login,
  OAuth playground, GraphQL playground, API Explorer, Postman/Insomnia, OpenAPI/Swagger specs, free
  dev account): apitracker.io/a/meliopayments; portal host `docs.melio.com` (openbankingtracker).
- OAuth 2.0 authorize screen + scoped refreshable token (third-party connector description).

**`‹stage-1›` open items (must resolve from the partner dev account before dev):** API host +
version; `authorize`/`token` URLs; `token_exchange_style`; PKCE support; scope strings; token
expiry + refresh rotation → `refresh_lease` value; identity source + `stable_key`/labels; resource
paths + pagination params; whether a partner-level static header is required; whether a
dev/sandbox tier exists; revoke endpoint (→ `disconnect_mode`).

**Risk flag for stage 1 / batch lead:** docs **and** credentials are both partner-gated and the
portal WAF-blocks automated access. If no self-serve dev/sandbox path materializes, this is a
3-hold-class "API access / account procurement" risk (peer to Bill.com) — amend the catalog rather
than merge an unrunnable L2/L4.
