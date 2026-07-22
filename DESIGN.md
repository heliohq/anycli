# OneSignal — `heliox tool onesignal` (per-tool design)

**Catalog row:** 210 · anycli id `onesignal` · provider key `onesignal` · auth lane
`api_key` · wave 3 · category "Social & Media" (see §7 note — it is really a
push/customer-messaging platform).
**Scope:** scratch design file on branch `tool/onesignal`; stripped by the batch
lead at batch end. Follows the `helio-tool-provider` pipeline (SKILL.md) and the
008-300 master plan §2/§3.

---

## 1. Auth-lane verification (official docs vs catalog & audit)

**Verdict: `api_key` is correct — confirmed, no divergence to record.**

The audit (`oauth-audit.md` row 212) marks OneSignal "no viable multi-tenant
path → api_key". Verified against OneSignal's official docs:

- **No OAuth authorization-code flow of any kind.** OneSignal authenticates with
  static, dashboard-issued secret API keys — there is no `authorize`/`token`
  endpoint, no client registration, no consent screen. (Keys & IDs, REST API
  overview.) The audit rubric's bar (a multi-tenant authorization-code app that
  arbitrary customer accounts can authorize) is not met. Stays `api_key`.
- **Three credential objects exist; two matter here:**
  - **App ID** — public UUID v4 identifying one OneSignal app. "Safe to use in
    client-side SDK initialization" — **not a secret**, but **required to scope
    every API call** (request body `app_id` for sends, path/query `app_id` for
    reads and segments).
  - **App API Key** (the modern REST API Key, value prefix `os_v2_app_`) — a
    **private secret scoped to a single app**, used for "sending messages,
    creating users, reading stats". This is the token the AI teammate operates
    with.
  - **Organization API Key** — org-wide secret for *creating apps* and *managing
    other API keys* (Create/View **apps**, Create/Delete/Rotate keys). An AI
    teammate does **not** provision apps or rotate keys, so this key is **out of
    scope**; the tool operates entirely with an App API Key + App ID that the
    user pastes for a specific app.
- **Self-serve.** Both key types are created directly in the dashboard
  (Settings → Keys & IDs) with no registration or review; shown once, then
  copied. This is exactly the api_key account-pool model — the test account
  yields the key with zero human app-registration lane (L4/L5 need no lane-1
  app).

**Auth wire shape (load-bearing, verified on live docs):**

- Base URL **`https://api.onesignal.com`** (current). The legacy
  `https://onesignal.com/api/v1` host still resolves but the service targets the
  current host.
- Header **`Authorization: Key <APP_API_KEY>`** — note the scheme word is
  **`Key`**, *not* `Bearer` or `Basic`. (Legacy examples used `Basic`; current
  docs across create-message, view-messages, and segments all use `Key `.) Plus
  `Content-Type: application/json` on writes.
- `app_id` travels in the **body** (POST /notifications), the **query**
  (GET /notifications), or the **path** (`/apps/{app_id}/segments`) depending on
  endpoint — never in the header.

Divergence recorded: **none** — the audit and catalog match the official docs.
The one thing to carry forward is the **two-field** credential model (key +
app_id) and the **`Key ` auth scheme**, both handled below.

---

## 2. Which API surface the tool wraps, and why

Driven by what an AI teammate actually does with OneSignal — send and audit
customer/push messages for one app — the tool wraps the **App-API-Key-scoped
messaging surface** of the OneSignal REST API and deliberately omits the
org-scoped and high-cardinality-export surfaces.

| Endpoint (base `https://api.onesignal.com`) | Method | Why an AI teammate needs it |
|---|---|---|
| `/notifications` | POST | Send a push / email / SMS message (the core action). Body carries `app_id`, `target_channel`, one targeting method (`included_segments` **or** `include_aliases`/`include_subscription_ids`/`include_phone_numbers`/`email_to` **or** `filters`), and `contents`. |
| `/notifications?app_id=&limit=&offset=` | GET | List recent messages (max/default 50, desc by `queued_at`) — "did my announcement go out, to how many?" |
| `/notifications/{id}?app_id=` | GET | View one message's delivery stats (successful/failed/converted) after a send. |
| `/notifications/{id}?app_id=` | DELETE | Cancel a scheduled/not-yet-delivered message. |
| `/apps/{app_id}/segments` | POST | Create an audience segment (filters) to target future sends. |
| `/apps/{app_id}/segments` | GET | List segments — so the AI can pick a valid `included_segments` value before sending. |
| `/apps/{app_id}/segments/{id}` | DELETE | Remove a segment. |
| `/users` (POST) · `/users/by/{alias}/{id}` (GET) | POST/GET | Create/read a user by external alias — set properties / tags used for targeting. |

**Deliberately excluded:**
- **Apps** (create/view apps) and **API-key** management — require the
  **Organization API Key** the tool does not hold, and are provisioning actions
  outside a teammate's job.
- **Data-export CSV** endpoints (view CSV of subscriptions/events) — bulk
  analytics dumps, not a conversational teammate action; large, async, S3-links.
- **Live Activities / template CRUD** — deferrable; can grow later as verbs
  without changing the credential model.

The through-line: everything the tool exposes is reachable with **one App API
Key + App ID**, returns small JSON, and maps to a sentence a teammate would say
("send a push to Subscribed Users", "did it deliver", "make a segment").

---

## 3. anycli definition (stage-1 form decision + shape)

**Form: `service` type.** No official OneSignal CLI exists that is
non-interactive, `--json`-capable, and provisionable into the runtime image, so
per the SKILL stage-1 rubric this is a `service`-type definition implemented in
`internal/tools/onesignal/` against the HTTP API (the default; 21/23 shipped
tools are service type). Go package name `onesignal` (id has no dashes / leading
digit).

**Two-credential injection** (the notable shape — unlike single-token bitly/slack):

```jsonc
// definitions/tools/onesignal.json
{
  "name": "onesignal",
  "type": "service",
  "description": "OneSignal as a tool (App API Key + App ID)",
  "auth": {
    "credentials": [
      { "source": {"field": "app_api_key"},
        "inject": {"type": "env", "env_var": "ONESIGNAL_APP_API_KEY"} },
      { "source": {"field": "app_id"},
        "inject": {"type": "env", "env_var": "ONESIGNAL_APP_ID"} }
    ]
  }
}
```

The service reads both env vars, sets `Authorization: Key $ONESIGNAL_APP_API_KEY`
on every request, and **auto-injects `$ONESIGNAL_APP_ID`** into the body/query/
path so the AI never passes `--app-id` (the app is fixed per connection). This
mirrors the multi-field injection precedents (zoominfo, mixpanel, servicenow) —
two `CredentialBinding` entries, both `env`.

**Cobra tree (resource-grouped, `--json` everywhere, notion shape):**

```
onesignal message send    --channel push|email|sms
                          (--segment NAME... | --subscription-id ID... |
                           --email ADDR... | --phone E164... | --filters JSON)
                          --heading TEXT --content TEXT [--name STR] [--send-after TS]
onesignal message list    [--limit N] [--offset N]
onesignal message get     --id NID
onesignal message cancel  --id NID
onesignal segment create  --name STR --filters JSON
onesignal segment list
onesignal segment delete  --id SID
onesignal user upsert     --alias-label STR --alias-id STR [--properties JSON] [--tags JSON]
onesignal user get        --alias-label STR --alias-id STR
```

**JSON output shape** — provider-neutral envelope, same convention as the
service precedents:
- success: the provider JSON passed through, e.g. `message send` →
  `{"id":"<notification-uuid>","recipients":N,"external_id":null}`.
- `message list` → `{"messages":[{id,name,queued_at,successful,failed,...}],
  "total_count":N,"limit":50,"offset":0}` (fields projected from the provider
  page).
- Exit codes: `0` success, `1` runtime/API failure via typed `apiError` with a
  `--json` error envelope, `2` usage/parse. Validate the "exactly one targeting
  method" rule client-side and fail `2` before the call (docs: "Only one
  targeting method is allowed per message"), so the AI gets an actionable parse
  error, not a provider 400.

---

## 4. Credential fields & exact auth flow

**Manual credential entry — no OAuth, two fields.** The user pastes:

| Field | Secret | Purpose |
|---|---|---|
| `app_api_key` | yes | App API Key (`os_v2_app_…`) → `Authorization: Key …`. |
| `app_id` | no (but required) | App UUID → scopes every request; also the account identity. |

Registration/token semantics: keys are **long-lived, non-expiring** secrets
until the user rotates/deletes them in the dashboard — there is **no refresh
cycle** and no `expires_at`. The token gateway serves the stored pair directly
(same class as Slack's non-expiring bot token, but two fields instead of one).
A revoked key surfaces as a `401` at first use (fail-fast via anycli's
`CredentialRejected`), not proactively — acceptable for api_key tools.

Scopes: OneSignal App API Keys are **not scope-parameterized** — one App API Key
grants the full app-level surface (send, read stats, users, segments). So there
are no wire scopes; any display-only capability slugs are cosmetic (see bitly's
`display_scopes` precedent) and optional.

---

## 5. Helio provider bundle plan (`integrations/providers/onesignal/`)

Two-secret-field manual credential → **`runtime_strategy: manual_credentials`**
(the mongodb / servicenow / mixpanel family), **not** `standard_oauth`. Axes all
align: ① CLI word `onesignal`, ② anycli id `onesignal`, ③ provider key
`onesignal` — **no `toolToProvider` entry needed**, flat (non-grouped) command.

Hidden-first (`presentation.visible: false`) until the anycli pin ships the tool
and the full five-layer pass is green.

```yaml
schema: helio.provider/v1
key: onesignal
go_name: OneSignal

presentation:
  name: OneSignal
  description_key: onesignal
  consent_domain: onesignal.com
  visible: false            # flip only after L1–L5 green + pin bumped + icon/i18n/docs

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: app_api_key
        label_key: onesignal_app_api_key
        secret: true
        placeholder: "os_v2_app_..."
        required: true
      - name: app_id
        label_key: onesignal_app_id
        secret: false
        placeholder: "00000000-0000-0000-0000-000000000000"
        required: true
    setup_url: https://documentation.onesignal.com/docs/keys-and-ids

identity:
  source: strategy          # no App-key-readable userinfo returning a friendly name
  # account_key/label = app_id (human-readable UUID, never a hash) — OQ2 precedent

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources: { selection: none, discovery: none, enforcement: none }

# The App API Key (the only secret) rides the single-secret user-token write path
# (token.access_token); the App ID is the account identity, so it projects from
# connection.account_key rather than a second token field. No new
# CredentialSource, no token gateway change. (This supersedes an earlier draft
# that modeled both as token fields — app_api_key: token.app_api_key /
# app_id: token.app_id; the shipped single-secret + account_key form below is
# cleaner and is what integrations/providers/onesignal/provider.yaml carries.)
credential:
  fields:
    app_api_key: token.access_token
    app_id: connection.account_key
    account_key: connection.account_key

tool:
  name: onesignal
  kind: api-key
```

**Capability check (integration-service):**
- **Multi-field `manual_credentials`** + a **value-passthrough identity deriver**
  keyed on `app_id` (account_key = app_id, label = app_id) — already present on
  main (zoominfo/servicenow/mixpanel). **Expected: zero capability growth.**
- **Optional connect-time verifier** (recommended UX, not required): verify the
  pasted pair *together* by calling `GET /notifications?app_id=<app_id>&limit=1`
  with `Authorization: Key <app_api_key>` — a `200` proves both the key is valid
  and the app_id belongs to it. This is the semrush/moz/tally/loops verifier
  pattern. **The one thing to confirm at stage 1:** whether an existing verifier
  capability emits the **`Key `** auth scheme (not `Bearer `/`Basic `) with a
  **query-param** `app_id`. If none does, add a narrow reviewed verifier variant
  (small, enum-style growth) — otherwise fall back to the mongodb **no-verify**
  path (bad pair surfaces at first use). Do not overclaim; pick no-verify if the
  scheme growth isn't already covered.
- **`go-services/` client id/secret config: none.** manual_credentials has no
  Helio-side OAuth client, so lane-1's config-append shared surface does **not**
  apply to this tool (nothing lands in `config/` + `deploy/`).

**UI:** `ui/helio-app/src/integrations/icons/onesignal.svg` (reviewed brand
mark) + append to `providerIcons.ts`; i18n `onesignal_app_api_key`,
`onesignal_app_id`, and `tools.desc.onesignal` across all 9 locales. Manual,
never generated.

---

## 6. Test plan → five layers

| Layer | OneSignal-specific plan | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `httptest` fake asserts: `Authorization: Key <key>` header on every request; `app_id` auto-injected into POST body / GET query / segment path; "exactly one targeting method" → exit `2` before any HTTP; `--json` success + typed `apiError` envelope for a provider `400`/`401`; `message list` pagination projection. Never hits real API. | No |
| **L2** harness real-API | `ANYCLI_CRED_APP_API_KEY=os_v2_app_… ANYCLI_CRED_APP_ID=… anycli onesignal -- message list --limit 1`, then a real `message send` to a test segment, then `message get --id`. Proves the `Key ` scheme, host, and app_id placement against the live API — mandatory before the pin bump. | **Yes** — an App API Key + App ID from a free OneSignal test app (account pool; self-serve, no app-registration lane). |
| **L3** `provider-gen --check` + suites | Run `provider-gen` + `--check` locally on-branch (five projections regenerate — **not committed** on the tool branch; batch lead produces the canonical regen). `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with a local `replace` → anycli branch. integration-service unit tests for the multi-field bundle + identity deriver (+ verifier if grown). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` with `provider:"onesignal"` and **both** token fields (`app_api_key`,`app_id`); api_key provider is seedable (user-token class). Then `heliox tool onesignal -- message list --limit 1` as the seeded assistant reaches the live API through the real token gateway. **Seed both fields, no `refresh_token`/`expires_at`** (non-expiring key, no refresh path to exercise). | **Yes** — same real key/app_id as L2. |
| **L5** full connect flow | api_key key-entry path (master plan §2, agent-drivable): open connect link → paste App API Key + App ID via `POST /connections/credentials` → connection shows connected/configured in `GET /connections` → one **unseeded** live `message list` through the token gateway succeeds. No OAuth consent (there is none). Run once, still hidden, before the visible flip. | **Yes** — real key/app_id + the app_service connect UI. |

**Credential-gated layers: L2, L4, L5** (all satisfied by a single free
self-serve OneSignal test app — no human OAuth-app-registration lane, no review
clock; api_key wave-3 tool). L1 and L3 are fully offline/agent-automatable.

Rollout: land hidden → pin bump (one anycli tag for the batch) → L1–L4 green →
L5 sweep → flip `presentation.visible: true` + regenerate as the single go-live
change (SKILL stage 10).

---

## 7. Notes / flags for the batch lead

- **Category mismatch (cosmetic, not auth):** the catalog files OneSignal under
  "Social & Media" (row 210), but it is a push-notification / customer-messaging
  platform (closest siblings: Iterable, Courier, Knock, Novu under "Marketing &
  Notifications"). Keep the catalog row as-is (numbering/keys frozen); flagging
  only so the UI `order`/grouping isn't taken as a product signal.
- **Two-field credential, `Key ` scheme** are the only non-boilerplate points —
  both handled by existing multi-field `manual_credentials`; the sole possible
  capability growth is a `Key `-scheme + query-param verifier, and only if we
  want connect-time verification over the no-verify fallback. Decide at stage 1.
- **Organization API Key intentionally unsupported** — app provisioning / key
  rotation are not teammate actions and would require a second, org-wide secret.
  If a future need appears, it is a separate provider decision, not a field on
  this bundle.
