# Recurly — per-tool design (`tool/recurly`)

**Batch tool:** Recurly (master-plan catalog row 175). **Wave:** 3. **Auth lane:** `api_key`.
**Naming (all three axes identical — no resolver divergence):**

| Axis | Value |
|---|---|
| ① CLI command word (`tool.command`) | `recurly` (flat, ungrouped) |
| ② anycli tool id (`definitions/tools/<id>.json`, `name`) | `recurly` |
| ③ provider catalog key (bundle dir / `key:`) | `recurly` |

Go package (stage-2 rule): `internal/tools/recurly/` (no dash, no leading digit → id used verbatim).
Because ② == ③, **no `toolToProvider` entry** is added in `helio-cli/internal/toolcred/resolver.go`.

This is a scratch design file on the `tool/recurly` branch worktree; the batch lead strips it at batch-end.

---

## 1. Audit verdict vs. official docs — api_key lane CONFIRMED

Master-plan row 175 and the 2026-07-21 OAuth audit (row 177) both keep Recurly in `api_key`
("no viable multi-tenant path"). I verified this against Recurly's official docs and it holds:

- Recurly V3 uses **HTTP Basic authentication with a private API key as the username and a blank
  password** — there is no authorization-code OAuth flow, no registered "Recurly app," and no
  multi-tenant consent surface. A key is minted by each merchant inside their own Recurly site
  (Integrations → API Credentials) and is **scoped to that one site**. There is no shared client
  that arbitrary customer sites could authorize.
- Sources: Recurly V3 API reference `https://recurly.com/developers/api/v2021-02-25/`,
  Authentication guide `https://dev.recurly.com/docs`, V3 Upgrade Guide
  `https://recurly.com/developers/guides/v3-upgrade-guide.html`, versioning
  `https://dev.recurly.com/docs/versioning`.

**Divergence recorded:** none from the catalog's lane. One implementation wrinkle the catalog does
not capture: Recurly has **two regional hosts** — `https://v3.recurly.com` (US) and
`https://v3.eu.recurly.com` (EU) — selected by the merchant's data-center region, not encoded in the
key. This is handled with an optional `region` credential field (§3), not a lane change.

---

## 2. Official API surface this tool wraps, and why

**API:** Recurly V3 REST API, version pinned `v2021-02-25` (JSON-only; V2/XML is legacy and not
wrapped). Host `https://v3.recurly.com` (US) / `https://v3.eu.recurly.com` (EU). Rate limit
2000 req/min/site; `429` carries `Retry-After` and every response carries `X-RateLimit-*`.

**What an AI teammate actually does with Recurly** drives endpoint selection. Recurly is a
subscription-billing/recurring-revenue platform; a teammate's real jobs are *"look up this
customer and their subscription state," "why did this invoice fail / is this account past due,"
"what plans and coupons exist," "change/cancel/pause a subscription," "pull recent
transactions."* That is read-dominant with a curated set of lifecycle writes. Recurly IDs accept a
human-friendly alias form the tool must expose: `code-<account_code>`, `number-<invoice_number>`,
`uuid-<subscription_uuid>` — so a teammate can address an account by its business code without a
prior lookup.

Wrapped resources (path forms are the site-scoped V3 shapes; `{id}` accepts the alias prefixes above):

- **accounts** — `GET /accounts`, `GET /accounts/{account_id}`, `POST /accounts`,
  `PUT /accounts/{account_id}`, `GET /accounts/{account_id}/balance`,
  `GET /accounts/{account_id}/billing_info`. The customer entry point.
- **subscriptions** — `GET /subscriptions`, `GET /subscriptions/{id}`,
  `GET /accounts/{account_id}/subscriptions`, `POST /subscriptions`, `PUT /subscriptions/{id}`,
  `PUT /subscriptions/{id}/cancel`, `PUT /subscriptions/{id}/pause`,
  `PUT /subscriptions/{id}/resume`, `DELETE /subscriptions/{id}` (terminate). Core lifecycle.
- **invoices** — `GET /invoices`, `GET /invoices/{invoice_id}`,
  `GET /accounts/{account_id}/invoices`, `GET /invoices/{invoice_id}/line_items`,
  `PUT /invoices/{invoice_id}/collect` (retry collection). Dunning / past-due answers.
- **transactions** — `GET /transactions`, `GET /transactions/{id}`,
  `GET /accounts/{account_id}/transactions`. Payment/decline history.
- **plans** — `GET /plans`, `GET /plans/{plan_id}`. Catalog lookups (plan_code addressing).
- **coupons** — `GET /coupons`, `GET /coupons/{coupon_id}`. Discount lookups.
- **line_items** — `GET /line_items`, `GET /accounts/{account_id}/line_items`.
- **sites** — `GET /sites`, `GET /sites/{site_id}`. Used by the verifier/identity path (§4), and
  as a `list` for multi-site keys.

Excluded from the first cut: full plan CRUD, coupon redemption/creation, gift cards, measured-usage
add-ons, purchases/checkout, credit-payment application. Payments mutations (refunds, account
redaction/GDPR `redact_account`) are deliberately **omitted** from the initial verb set — high-blast-
radius financial writes are not what a teammate should reach for by default; add later behind a
narrower review if a real workflow needs them.

---

## 3. anycli definition (stage-1 form + shape)

**Type: `service`** (stage-1 rubric). No official Recurly CLI exists; the only integration surface is
the HTTP API. This matches 21/23 shipped definitions and every other billing tool in the program
(chargebee/adyen/paddle/lemon-squeezy are all `service`).

**`definitions/tools/recurly.json`** — single credential, injected as env; the service builds the
Basic header itself (blank-password Basic is not expressible as a plain header-value injection, and
the version `Accept` header is provider-specific), so the definition just delivers the raw key:

```json
{
  "name": "recurly",
  "type": "service",
  "description": "Recurly subscription billing as a tool (V3 REST API, private API key)",
  "auth": {
    "credentials": [
      { "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "RECURLY_API_KEY"} },
      { "source": {"field": "region"},
        "inject": {"type": "env", "env_var": "RECURLY_REGION"} }
    ]
  }
}
```

`region` is optional at the resolver level (absent → service defaults to US host). anycli passes
through whatever the credential map contains; the field-absent case is normal, not an error.

**`internal/tools/recurly/`** — copy the `internal/tools/notion/` shape (the reference service impl):
a cobra tree grouped by resource, a `BaseURL`/`HC`/`Out`/`Err` struct so unit tests point at an
`httptest.Server`, and the documented exit-code contract (0 success, 1 runtime/API failure via typed
`apiError`, 2 usage/parse). Command tree (verbs are read-first; `--json` on every leaf):

```
recurly account list|get|balance|billing-info
recurly subscription list|get|create|change|cancel|pause|resume|terminate
recurly invoice list|get|line-items|collect
recurly transaction list|get
recurly plan list|get
recurly coupon list|get
recurly line-item list
recurly site list|get
```

Cross-cutting flags: `--account` (accepts `code-…`), `--limit`, `--state`/`--type` filters,
`--begin-time`/`--end-time`, cursor `--cursor` (Recurly pagination is cursor-based via the
`Link` header / `has_more` + `next`). The service:
- sets `Authorization: Basic base64(api_key + ":")`, `Accept: application/vnd.recurly.v2021-02-25`,
  `Content-Type: application/json`;
- selects host from `RECURLY_REGION` (`eu` → `v3.eu.recurly.com`, else `v3.recurly.com`);
- surfaces Recurly's typed error envelope (`{ "error": { "type", "message", "params" } }`) into the
  `--json` structured error and a plain-text line; maps `429` to a retryable exit-1 with the
  `Retry-After` echoed.

**JSON output shape:** pass Recurly's resource JSON through unwrapped for `get`; for `list`, emit
`{ "data": [ …resources… ], "has_more": bool, "next": "<cursor>" }` (provider-neutral envelope
matching the built-in-service list convention other tools use). Never print secrets.

TDD (anycli AGENTS.md): write `*_test.go` first per resource — `httptest` fake asserting request path,
the injected `Authorization: Basic` header, the `Accept` version header, host selection by region,
alias-prefix passthrough (`code-bob`), and both plain + `--json` error rendering. Never hit the live
API from a unit test. Register in `internal/tools/register.go` `init()` via
`RegisterService("recurly", &recurly.Service{})` (this is the one anycli shared surface — batch-end).

---

## 4. Helio provider bundle plan (`integrations/providers/recurly/provider.yaml`, hidden-first)

- `key: recurly`, `presentation.visible: false` (hidden-first; flip is the single go-live change
  after L5). Directory name == `key` (generator enforces equality).
- `tool.name: recurly` (axis ②). No `tool.command`/`tool.group` — flat, ungrouped provider.
- **Auth type `api_key` / manual-token bundle** (NOT `standard_oauth`): the user pastes their
  Recurly private API key; it enters through the write-only `POST /connections/credentials` API and
  lands in Vault. The bundle declares only reviewed verification metadata — it never carries the key.
- **Credential fields:** `api_key` (required, secret) + `region` (optional enum `us|eu`, default
  `us`). This is a multi-field manual-credentials bundle (precedent: mixpanel / zoominfo /
  paypal multi-field manual credentials); if the pinned integration-service on the batch base
  already supports optional-field manual credentials, no growth is needed — **verify on base first**.
- **Verification + identity deriver (capability check, likely narrow growth):** the connect flow
  must validate the key and derive a stable account label. Recurly's natural identity is the **site
  subdomain** from `GET /sites` (first site) — a single-field Basic-**username** scheme against a
  fixed versioned host. Precedents to reuse or extend:
  - If a generic single-field "Basic-username, blank-password" verifier already exists (chargebee
    added a *two*-field site-templated Basic verifier; mailjet added a Basic verifier), check whether
    it can be parameterized with the `Accept` version header + region host. If not, add a narrow
    `recurlySiteVerifier` capability (precedent shape: `postmarkServerVerifier`,
    `sendgridScopesVerifier`, `lemonSqueezyVerifier`, `courierBrandsVerifier`) that calls
    `GET /sites`, treats 2xx as valid, and returns the site subdomain as the identity label +
    stable `account_key`. The verifier doubles as the identity deriver (no separate userinfo call).
  - Region must flow into the verifier's host selection exactly as it does into anycli's — same
    `us|eu` mapping — so a EU-region key is validated against the EU host.
- `connection.disconnect_mode`: local revoke only (no provider-side token revocation endpoint for
  private keys) — `none`/local, matching other api_key bundles.
- **Config (`config/` + `deploy/` Helm Secret):** manual api_key bundles need **no client
  id/secret** — nothing to append to integration-service OAuth config. This provider renders
  `configured` from the user-supplied key alone. (Confirm: no `required_config_fields`.)
- Icon: `ui/helio-app/src/integrations/icons/recurly.svg` + hand-register in `providerIcons.ts`
  (never generated). i18n label strings for the two credential fields.
- AI-facing doc: provider sub-doc under `agents/plugins/heliox/skills/tool/`, plugin version bump +
  marketplace publish (batch-end).

**Generation:** from `go-services/integration-service`, `go run ./cmd/provider-gen` then
`--check`; the five projections are committed together **by the batch lead at batch-end** — this
branch must NOT commit regenerated projections (master-plan §2), and CI `provider-gen --check` is
expected red on this branch until the batch-end regen.

---

## 5. Test plan → the five layers

| Layer | What it proves for Recurly | External credential needed? |
|---|---|---|
| **L1** | `go test ./...` in anycli: httptest fakes assert path, `Authorization: Basic base64(key:)`, `Accept: application/vnd.recurly.v2021-02-25`, region→host selection, alias-prefix passthrough, list envelope, typed-error + 429 rendering. | No (fakes only). |
| **L2** | Dev harness `ANYCLI_CRED_API_KEY=<key> [ANYCLI_CRED_REGION=eu] anycli recurly -- account list --limit 1` (and `subscription list`, `plan list`) against the **real** Recurly API — proves field names, Basic-username injection, version header, and pagination match live. | **Yes** — a real Recurly **site + private API key** (sandbox site is free/self-serve; account-pool lane). |
| **L3** | `provider-gen --check` (run locally on-branch only) + both repos' unit suites (`helio-cli` build with a local uncommitted `go.mod replace` → anycli branch; integration-service verifier/registry tests). | No. |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with `provider:"recurly"`, `access_token:<real key>` (api_key providers are seedable; non-expiring key → seed access_token only, no refresh/expiry), then `heliox tool recurly -- account list` reaches the live API through the real token gateway. | **Yes** — same real key as L2, plus a real seeded org/assistant identity. |
| **L5** | Pre-flip, hidden: the **api_key key-entry** path (master-plan §2 api_key L5, not the OAuth checklist) — open the connect link → paste the key (+region) via the real connect UI → `POST /connections/credentials` stores it, the `recurlySiteVerifier` validates against `GET /sites` and the connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool recurly` live command succeeds. That unseeded run is the completion signal. | **Yes** — real key + the account-pool site; agent-drivable (agent-browser), human fallback on UI breakage. |

Layers needing externally supplied credentials: **L2, L4, L5** (all satisfied by one self-serve
Recurly sandbox site + its private API key from the account-pool lane; a EU sandbox additionally
exercises region routing but is optional). L1/L3 are hermetic.

**Definition of done** (master-plan §2): all five layers green, AI doc published, icon registered,
then `presentation.visible: true` + regenerate as the single go-live change — done only after the
visible flip; until then this is "code-complete (hidden)."

---

## 6. Implementation divergences from this design (recorded per task rule)

Verified against the batch base (Helio worktree at origin/main, 164 commits behind the sibling
billing branches; anycli base = the 23 shipped tools) rather than the assumed capabilities in §3/§4.
Three divergences, all driven by base reality, none by the official docs (which the impl matches):

1. **Bundle is `api_key` / `manual_api_token`, not a two-field `manual_credentials` bundle (§4).**
   The base's `manual_credentials` path (`resolveManualSecret`) enforces **exactly one** credential
   field (a D5 generation-time check) and hard-wires the DSN-host identity deriver — unusable for
   Recurly's opaque key. So Recurly ships as the (previously provider-less) `manual_api_token`
   family: `auth.type: api_key`, `identity.source: userinfo` → `GET /sites`, verified by a compiled
   verifier. This is the correct "verified against an identity endpoint" family for Recurly.

2. **`recurlySiteVerifier` added (§4 anticipated this).** The base's `declarativeManualTokenVerifier`
   injects the raw token as a bearer header value and hard-codes `Accept: application/json` — it
   cannot express Recurly's Basic-**username** scheme (key as username, blank password) or the
   mandatory `Accept: application/vnd.recurly.v2021-02-25` header (406 otherwise). A shared
   identity-endpoint core (`verifyViaIdentityEndpoint`) was extracted from the declarative verifier;
   `recurlySiteVerifier` reuses it with a Basic-username + version-header decorator and is selected
   for the recurly provider via `manualAPITokenVerifier`. It derives the site subdomain (`/data/0/subdomain`)
   as the stable account key + label.

3. **Region/EU deferred → US-only initial cut (§1/§3 region field dropped from the shipped path).**
   The single-secret connect path cannot store a second `region` value, and the closed
   credential-source allowlist has no region source — delivering region needs multi-field
   manual-credentials + a metadata source (both being added in parallel unmerged branches; not
   depended on here). So the `region` credential binding was dropped from the anycli definition to
   match the US-only bundle (otherwise the helio-cli pinned-anycli projection check fails: the tool
   would advertise a `region` credential the platform can never supply). The anycli **service** keeps
   its region host-selection (`hostForRegion`, `EnvRegion`, tested) so EU support is a definition
   binding + bundle region-field re-add with **no service change**. An EU-region key fails
   verification against the US host rather than silently hitting the wrong data center (fail-fast, not
   a silent fallback).
