# Braze — per-tool design (`heliox tool braze`)

**Batch:** Wave 2 · Marketing · catalog row 124
**Auth lane:** `api_key` (REST API key, `Authorization: Bearer`) — audit-confirmed, no divergence
**Naming axes:** ① CLI word `braze` · ② anycli id `braze` · ③ provider key `braze` (all identical → **no `toolToProvider` entry**, no resolver divergence)
**Tool form:** `service` type (no official non-interactive Braze CLI to wrap)
**Go package:** `internal/tools/braze/` (id has no dashes/leading digit → package == id)
**Status:** design only. Hidden-first when implemented (`presentation.visible: false`). Batch lead strips this file at batch-end.

---

## 1. What an AI teammate does with Braze, and which API surface that maps to

Braze is a customer-engagement / cross-channel messaging platform (push, email, SMS, in-app). What a marketing-oriented AI teammate actually does against it splits into two intents, and the tool wraps **both**, because "how did the campaign do" and "send this campaign" are equally native asks for a marketing teammate:

1. **Read / report / discover** (primary, safest, unconditional): "how did last week's campaign perform", "pull the DAU trend", "which Canvases are live", "what segments exist", "show me this user's profile". → Braze **Export** endpoints (all read).
2. **Act — trigger and schedule messages, look up/update users** (secondary, permission-gated): "send the win-back campaign to this user", "schedule the Friday broadcast", "track this event for user X". → Braze **Messages** + **User data** endpoints (write).

The write surface is deliberately in-scope but **guard-railed by Braze's own per-key permission model** (§3): the stored REST API key carries exactly the endpoint permissions its creator granted, so a read-only key simply `403`s on a send verb — the credential is the guardrail, not tool-side allow-listing. Destructive/bulk/admin surfaces are excluded from v1 regardless (§6).

### Official API inventory (verified against `braze.com/docs/api`)

| Braze API category | This tool uses it? | Why |
|---|---|---|
| **Export** (campaigns, Canvas, segments, sends, KPI, events, purchases, sessions, user profiles) | **Yes — core** | The read/analytics/discovery surface. What an AI reports on and inspects. |
| **Messages** (send, campaign/Canvas trigger, schedule, scheduled-broadcasts list) | **Yes — secondary, permission-gated** | "Send/schedule this message." Core marketing-teammate action; gated by key scope. |
| **Templates** (email templates, Content Blocks — list/info) | **Yes — read** | Discover what content exists before sending. Read-only slice. |
| **Subscription groups** (status list/update) | **Yes — narrow** | "Is this user subscribed / unsubscribe them." Common support-adjacent ask. |
| **User data** (`/users/track`, `/users/export/ids`) | **Yes — bounded** | Look up a profile (export) and identify/track (write, gated). |
| Catalogs (create/update items), Media Library, Cloud Data Ingestion, SCIM, Preference Center builder, `/users/delete`, `/users/alias`, bulk imports | **No** | Admin/ops/destructive surfaces outside a teammate's engagement workflow (§6). |

### Region is a first-class credential input, not a constant (the design pivot)

Braze is a **regional, multi-instance** product. A workspace lives on exactly one cluster, and the REST host differs per cluster. Verified host list (`braze.com/docs/api/basics`, `.../braze_instances`):

| Instance | REST endpoint |
|---|---|
| US-01 … US-08 | `https://rest.iad-0X.braze.com` |
| US-10 | `https://rest.us-10.braze.com` |
| EU-01 | `https://rest.fra-01.braze.eu` |
| EU-02 | `https://rest.fra-02.braze.eu` |
| AU-01 | `https://rest.au-01.braze.com` |
| ID-01 | `https://rest.id-01.braze.com` |
| JP-01 | `https://rest.jp-01.braze.com` |
| KR-01 | `https://rest.kr-01.braze.com` |

Two consequences that shape the whole design:

- **There is no safe default host.** Unlike Mixpanel (3 regions) or Amplitude (US/EU with a US majority default), Braze has **~15 cluster hosts (the instance table above) and no "most projects are here" default that works**. A wrong host is a total failure for that workspace, and — because Braze's REST key is only known to its own cluster — a right key on the wrong host is a `401` indistinguishable from a dead key. So the host **must be captured at connect time and stored with the credential**, never guessed at runtime.
- **Storing the host with the credential is the design win, not a papercut.** Because the endpoint travels with the credential, `account_key`/label can be the **host** (`rest.iad-05.braze.com`) — readable and stable, the mongodb-DSN-host precedent — and Braze structurally avoids Amplitude's "EU-trap" (a stored-but-region-blind credential that 401s on the default host with no self-correction path). Region correctness is proven by the stored endpoint, not by a runtime `--region` retry hint.

### Rate limits (verified, `braze.com/docs/api/api_limits`)

Default is a high **250,000 requests/hour** for most endpoints, but several are much tighter and endpoint-specific (e.g. `/users/track` and the `/messages/send`, `/campaigns/trigger/send`, `/canvas/trigger/send` families have their own per-endpoint limits, some as low as ~250/min for non-Connected-Content sends). An AI firing successive analytics or send calls can hit these, so `429` is surfaced as a **distinct, retryable** signal. Braze does **not** return `Retry-After`; instead every response (including the `429`) carries `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset` (the window-reset time as UTC **epoch seconds**). The tool carries `X-RateLimit-Reset` as the back-off hint on the `rateLimit` envelope (§2.4) so the host knows *when* to retry, not a permanent failure.

## 2. anycli definition

### 2.1 Type & id
`service` type — there is no official, non-interactive, `--json`-capable Braze binary to provision into the runtime image (the stage-1 rubric's `cli`-type test fails on the first clause), so we implement the HTTP surface directly (matching 21/23 shipped definitions). Definition file `definitions/tools/braze.json`; service registered `RegisterService("braze", &braze.Service{})` in `internal/tools/register.go`; Go package `internal/tools/braze/`.

### 2.2 Credential binding (definition `auth.credentials`)

Braze needs **two logical values**: the REST API key (secret, Bearer) and the REST endpoint (non-secret, host). Helio's token gateway projects exactly **one** stored secret to AnyCLI (design 317 D5 single-token face — see §5). So the two values are folded into **one stored field**, and — following the mongodb precedent exactly — that field is a **DSN-shaped URL that embeds the key in userinfo and the cluster in the host**:

```
https://<REST_API_KEY>@rest.iad-05.braze.com
```

This is the zero-new-capability shape (§5): the host is `url.Parse`-derivable for `account_key` by the **existing** `dsnHostIdentityDeriver`, and the anycli service reconstructs both halves from it. Definition:

```json
{
  "name": "braze",
  "type": "service",
  "description": "Braze customer engagement — read campaign/Canvas/segment/KPI analytics, discover content, trigger and schedule messages, look up users (REST API key, Bearer auth)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "connection_string"},
        "inject": {"type": "env", "env_var": "BRAZE_CREDENTIALS"}
      }
    ]
  }
}
```

`connection_string` field name reuses the mongodb credential-source key (a DSN-shaped secret), so no new `CredentialSource` allowlist entry is needed (§5). The service parses `BRAZE_CREDENTIALS`: `u := url.Parse(v)`, `apiKey := u.User.Username()`, `baseURL := u.Scheme + "://" + u.Host` (userinfo stripped). A missing key half or an unparseable/hostless value is a fatal config error → exit **2** with static guidance that **never echoes the secret**. The host is validated against the known Braze cluster suffixes (`*.braze.com` / `*.braze.eu`) with a clear error naming the expected shape — a mistyped cluster fails loudly at parse, not as a silent `401`.

### 2.3 Subcommands / verbs

A resource-grouped cobra tree (notion's shape). The API key and base host come from the credential, never per-call flags — the AI passes only the analytical/message parameters.

**Read / export / discovery (core — all GET unless noted):**

| Command | Braze endpoint | Notes |
|---|---|---|
| `braze campaigns list` | `GET /campaigns/list` | paginated campaign inventory (id + name) |
| `braze campaigns details` | `GET /campaigns/details` | `--campaign-id` (req) |
| `braze campaigns series` | `GET /campaigns/data_series` | `--campaign-id`, `--length`, `--ending-at` — campaign analytics |
| `braze sends series` | `GET /sends/data_series` | `--campaign-id`, `--send-id`, `--length` — send analytics |
| `braze canvas list` | `GET /canvas/list` | Canvas inventory |
| `braze canvas details` | `GET /canvas/details` | `--canvas-id` (req) |
| `braze canvas series` | `GET /canvas/data_series` | `--canvas-id`, `--length`, `--ending-at`, `--starting-at` |
| `braze canvas summary` | `GET /canvas/data_summary` | rollup analytics for a Canvas |
| `braze segments list` | `GET /segments/list` | segment inventory |
| `braze segments details` | `GET /segments/details` | `--segment-id` (req) |
| `braze segments series` | `GET /segments/data_series` | `--segment-id`, `--length` — size over time |
| `braze kpi dau` / `mau` / `new-users` / `uninstalls` | `GET /kpi/{dau,mau,new_users,uninstalls}/data_series` | `--length`, `--ending-at` |
| `braze events list` | `GET /events/list` | custom-event names (discovery primitive) |
| `braze events series` | `GET /events/data_series` | `--event`, `--length`, `--unit` |
| `braze purchases series` | `GET /purchases/{quantity_series,revenue_series}` | revenue/quantity over time |
| `braze sessions series` | `GET /sessions/data_series` | app sessions time-series |
| `braze templates email list` / `info` | `GET /templates/email/list` · `/info` | `--template-id` for info |
| `braze content-blocks list` / `info` | `GET /content_blocks/list` · `/info` | Content Block discovery |
| `braze users export` | **POST** `/users/export/ids` | `--external-id`(rep)/`--email`/`--braze-id`, `--fields` — profile lookup |

**Act — messaging + user data (secondary, permission-gated — all POST):**

| Command | Braze endpoint | Notes |
|---|---|---|
| `braze messages send` | `POST /messages/send` | immediate message; content in body (`--body` raw JSON) |
| `braze messages schedule` | `POST /messages/schedule/create` | `--schedule` + message body |
| `braze messages scheduled-list` | `GET /messages/scheduled_broadcasts` | upcoming scheduled campaigns/Canvases |
| `braze campaigns trigger` | `POST /campaigns/trigger/send` | `--campaign-id` + recipients/trigger props |
| `braze canvas trigger` | `POST /canvas/trigger/send` | `--canvas-id` + recipients/trigger props |
| `braze subscription status-get` | `GET /subscription/user/status` | `--external-id` — subscription-group state |
| `braze subscription status-set` | `POST /subscription/status/set` | `--subscription-group-id`, `--state` |
| `braze users track` | `POST /users/track` | `--attributes`/`--events`/`--purchases` raw JSON — identify/track |

For the POST verbs, complex Braze request bodies (message objects, trigger properties, attribute arrays) are passed **through as raw JSON** from the AI (`--body '<json>'` / `--attributes '<json>'`) rather than re-modeled — Braze's message/trigger schema is large and versioned; do not reinvent it. The service only assembles the envelope, auth, and host.

### 2.4 JSON output shape

Pass Braze's JSON response through on stdout verbatim + newline (notion/bitly convention) — every listed endpoint returns structured JSON. Errors use notion's typed envelope: a non-2xx maps to `apiError` → exit **1** with `{"error":{...}}` (`--json` renders the structured envelope); usage/parse (bad flags, malformed `BRAZE_CREDENTIALS`) exit **2**; success exits **0**. Three statuses are surfaced **distinctly** from the generic `4xx` bucket because the AI must react differently to each:

- **`401`** → `credential` kind, `CredentialRejected` flag set (via `execution.RejectCredential`, the notion/drive `classifyCredentialError` precedent). Braze `401` = key invalid/revoked, or **the key is for a different cluster than the stored endpoint** — but since the endpoint is stored *with* the key (§1), a `401` here is unambiguously a bad/revoked key, not a region mismatch (Braze avoids Amplitude's ambiguous-401 trap entirely). Permanent until reconnect.
- **`403`** → `permission` kind, a **distinct** signal: the key is valid but **lacks the endpoint permission** for this verb (e.g. a read-scoped key hitting `messages/send`). The error string says exactly that and names the missing capability, so the AI/user knows to reconnect with a broader-scoped key rather than assuming the key is dead. Does **not** set `CredentialRejected` (the key is live for its own scope).
- **`429`** → `rateLimit` kind, `transient`. Braze does **not** send `Retry-After`; the back-off hint is `X-RateLimit-Reset` (UTC epoch seconds, the window-reset time), which is carried on the `rateLimit` envelope (optionally alongside `X-RateLimit-Remaining`) so the host can wait until reset and retry.

All three exit **1** (they are `apiError`s); the distinct kind lets the host/AI choose reconnect-key vs. broaden-scope vs. wait-and-retry.

## 3. Credential fields & exact auth flow

### 3.1 Verified auth model (official docs)
- **Mechanism:** `Authorization: Bearer YOUR-REST-API-KEY` on every request (`braze.com/docs/api/basics`). Not Basic, not a query param.
- **Registration model:** a **REST API key** is created in the Braze dashboard under **Settings → APIs and Identifiers → Create API Key**. At creation the user names it, optionally allowlists IPs, and **assigns endpoint permissions** (e.g. `campaigns.data_series`, `messages.send`, `users.export.ids`, `users.track`). Permissions and IP allowlist are **immutable after creation** — changing scope means creating a new key. It is a per-workspace, manually-created key, not an OAuth grant.
- **Token semantics:** the key does **not expire**; it is valid until manually revoked/deleted. **No refresh cycle** — a static credential. → at L4 seed the secret only, never `refresh_token`/`expires_at`.
- **Endpoint requirement:** every call needs the workspace's cluster REST host; the key is only valid on its own cluster. Captured once at connect (§3.3), stored with the key.

### 3.2 Why `api_key`, and divergence check vs. the audit
The 2026-07-21 OAuth audit (row 126) kept Braze `api_key`: "no viable multi-tenant path." **Official docs confirm this; I record no divergence.** Braze's REST API authenticates only with a manually-created, per-workspace REST API key. There is **no self-serve, multi-tenant authorization-code OAuth client** that an arbitrary customer's workspace can authorize for programmatic REST access (Braze's OAuth/SSO surfaces are for dashboard login and SCIM provisioning, not the messaging/export REST API). So `api_key` is correct; **axes ①==②==③ and no `toolToProvider` mapping is added.**

### 3.3 Connect-time input (Helio `credential_input`)
**One field** — a DSN-shaped secret carrying both the key and the cluster (§2.2, §5):

| Field | Secret | Required | Notes |
|---|---|---|---|
| `connection_string` | **yes** | yes | `https://<REST_API_KEY>@<cluster-rest-host>`, e.g. `https://a1b2c3d4-...@rest.iad-05.braze.com` |

`setup_url`: `https://www.braze.com/docs/api/api_key/` (how to create a scoped REST key) — the AI-facing sub-doc and the field placeholder additionally spell out where to read the cluster REST host (`Settings → APIs and Identifiers`, or `braze.com/docs/api/basics` instance table). The secret enters through the write-only `POST /connections/credentials` API and is stored in Vault; nothing secret touches the bundle.

## 4. Helio provider bundle plan (`integrations/providers/braze/provider.yaml`)

Naming: ① `tool.command` implicit `braze` · ② `tool.name: braze` · ③ dir/`key: braze`. Flat command (no family group). `presentation.visible: false` initially (hidden-first).

```yaml
schema: helio.provider/v1
key: braze
go_name: Braze

presentation:
  name: Braze
  description_key: braze
  consent_domain: braze.com
  visible: false            # hidden-first; flip is the single go-live change
  order: <unoccupied>

auth:
  type: credentials
  owner: individual
  credential_input:
    setup_url: https://www.braze.com/docs/api/api_key/
    fields:
      - name: connection_string
        label_key: braze_connection_string
        secret: true
        required: true       # exactly one required field (design 317 D5)
        placeholder: "https://<REST_API_KEY>@rest.iad-05.braze.com"

identity:
  source: strategy           # credential-derived; NO connect-time network call, no static host
  # account_key = label = the cluster REST host, derived from the DSN by the
  # existing dsnHostIdentityDeriver (mongodb precedent). No new deriver needed.

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources: {selection: none, discovery: none, enforcement: none}

credential:
  fields:
    connection_string: token.access_token   # single stored secret → the one AnyCLI field
    account_key: connection.account_key

tool:
  name: braze
  kind: api-key             # 317 D2 wire-compat; client routes the connect drawer by auth_type
```

**Adapter?** No compiled `service/adapter_*.go` — Braze is a static-credential provider with no OAuth lifecycle and no programmatic key-revoke-by-value endpoint (disconnect is `local_only`: delete the stored credential; the user revokes the key in the Braze dashboard). It rides the declarative `manual_credentials` strategy.

## 5. Integration-service dependencies to confirm at stage 1

**Primary design = zero new integration-service capability (subtract-before-adding).** Braze deliberately does **not** follow Mixpanel's/Amplitude's "new deriver + deriver-selection mechanism" path. Because the credential is expressed as a **DSN-shaped URL** (`https://<key>@<host>`), it lands on the **already-shipped mongodb path** unchanged:

- `runtime_strategy: manual_credentials` + `identity.source: strategy` is the mongodb bundle shape (on `main`).
- `composeProviderRegistration` hardwires `dsnHostIdentityDeriver` for every `manual_credentials` provider (`service/provider_registry.go`, the `RuntimeStrategyManualCredentials` branch). That deriver `url.Parse`s the secret, extracts the **host** via `firstDSNHost`, and returns `account_key = label = host` with the secret excluded from identity — **exactly** what Braze wants (`account_key = rest.iad-05.braze.com`). Verified against `service/manual_credentials_identity.go`.
- The projection reuses the existing `token.access_token` and `connection.account_key` `CredentialSource`s (`model/catalog.go`); `connection_string` is **not** a `CredentialSource` — it is only the credential-map field **key** (matching the anycli definition's `source.field`), so **no new allowlist entry is added**, identical to the mongodb bundle (`connection_string: token.access_token`). **No allowlist growth, no relaxation of the D5 single-required-field invariant** (the two pitfalls Mixpanel §9 documented for multi-field shapes).

So if the mongodb `manual_credentials` path is on `main` at stage 1 (it is), Braze needs **bundle + anycli service only** — no integration-service code, no `config/`+`deploy/` change (`manual_credentials` declares no `required_config_fields`, so the Config-Sync rule has nothing to sync). Confirm at stage 1: (a) the `manual_credentials` + `dsnHostIdentityDeriver` branch is unchanged on `main`, and (b) `firstDSNHost` returns the plain host for a `https://user@host` input with no port (it does — it strips userinfo via `url.Parse` before host extraction). Add one integration-service unit test asserting the Braze DSN shape derives `account_key = rest.iad-05.braze.com` with the key never entering the identity map.

**Documented alternative (rejected for v1): packed-JSON + a `braze` deriver kind.** If a sibling in the same batch (amplitude/crisp/servicenow) lands the bundle-declared **deriver-selection mechanism** (amplitude §5.1), Braze *could* instead store JSON `{"rest_endpoint","api_key"}` and register a `braze` deriver kind that pulls `account_key` from the endpoint host. This is a marginally tidier stored value but **adds a dependency on a shared, still-in-flight mechanism** and buys nothing over the DSN shape — the DSN already yields a readable host `account_key` through code that is on `main` today. Per the subtract-before-adding rule, the DSN shape wins; this alternative is recorded only so a batch that has *already* standardized on the deriver-selector can migrate Braze into it consistently later, not as a v1 requirement.

**Why no connect-time verifier.** `identity.source: strategy` performs **no** provider-side request at connect (mongodb OQ1 no-verify). A live probe would need a valid key+host pair and a low-risk read endpoint; more importantly, a static declarative `identity.url` cannot honor Braze's ~13-host residency (the same residency argument as §1). So identity is credential-derived, and a wrong key surfaces at **first tool use** as a distinct `401` (§2.4) — the mongodb precedent exactly. `mode: isolated` so each connected Braze workspace is its own connection, keyed by its cluster host.

## 6. Explicitly out of scope (v1)

- **Destructive/identity-mutating user ops:** `/users/delete`, `/users/alias/new`, `/users/identify`, `/users/merge` — irreversible; not a teammate action.
- **Catalog writes, Media Library, Preference Center builder, Cloud Data Ingestion, SCIM:** admin/ops surfaces, not engagement workflow.
- **Bulk data imports / high-volume `/users/track` batch loads:** instrumentation/ETL jobs, not teammate actions; also the tightest rate-limit tier.
- **Content-Block / template *creation* and *update*:** v1 exposes template/Content-Block **read** only; authoring is a heavier, review-worthy surface added later if demanded.
- Any of these is a separate command group + (for writes) a deliberately broader key scope, designed then — never smuggled into v1.

## 7. Test plan — five layers

| Layer | What runs | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in `internal/tools/braze/`: `httptest` fakes for each verb's path. Assert: **base host is reconstructed from the DSN** (`https://KEY@rest.iad-05.braze.com` → requests hit `rest.iad-05.braze.com`, and an EU DSN → `rest.fra-01.braze.eu`) — this proves multi-cluster correctness with **no** live non-US credential; `Authorization: Bearer <key>` header on every request with the key taken from userinfo and **never** logged; GET query-param mapping (`campaign_id`, `length`, `ending_at`, …) and POST raw-JSON body passthrough (`messages/send`, `users/track`, triggers); `users/export/ids` is POST; JSON passthrough on stdout. Error envelopes: plaintext + `--json` for **`401` (`credential`, `CredentialRejected=true`)**, **`403` (`permission`, `CredentialRejected=false`, message names the missing scope)**, and **`429` (`rateLimit`, `X-RateLimit-Reset` epoch-seconds hint populated — the fake sets `X-RateLimit-Reset`/`X-RateLimit-Remaining`, never `Retry-After`, matching the live API)** — all distinct from the generic `4xx`/`5xx` bucket. Malformed/hostless `BRAZE_CREDENTIALS` → exit 2, secret never echoed. Exit-code contract 0/1/2. | **No** |
| **L2** harness real API | `make build-harness` then `ANYCLI_CRED_CONNECTION_STRING='https://<key>@rest.<cluster>.braze.com' anycli braze -- campaigns list` (and one `campaigns series`, one `kpi dau`, and — with a send-scoped key on a sandbox workspace — one `messages send` to a test user) against a **real Braze workspace**. Mandatory before pin bump — proves the DSN parse, Bearer assembly, host reconstruction, and param/body mapping match the live API. | **Yes** — real Braze workspace + a scoped REST API key + its cluster host |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` (five projections); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service unit suite (incl. the one Braze-DSN `dsnHostIdentityDeriver` assertion from §5). On-branch: local `replace` in `helio-cli/go.mod` → anycli branch; local regen **not committed** (batch lead owns the canonical regen; the branch is expected to fail `provider-gen --check` in CI until batch-end, per master plan §2). | **No** |
| **L4** singleton + seed | Singleton in `env: dev`; `POST /internal/test-only/connections/seed` with `provider: braze`, `access_token` = the DSN string (single-secret seed — no new capability; confirm the seed stores it and `dsnHostIdentityDeriver` derives `account_key = rest.<cluster>.braze.com`), then `heliox tool braze -- campaigns list` and one `kpi dau`. Proves token-gateway → anycli path serves the credential and reaches the live API. Non-expiring key ⇒ seed the secret only, no `refresh_token`/`expires_at`. | **Yes** — same real workspace key seeded |
| **L5** connect flow (api_key key-entry path) | Hidden tool, before visible flip: open the connect link → paste the `https://<key>@rest.<cluster>.braze.com` DSN through the real connect UI (stored via `POST /connections/credentials`; **identity credential-derived — no connect-time verifier call**, so a well-formed DSN connects immediately) → connection shows connected/configured with `account_key = rest.<cluster>.braze.com` in `GET /connections` → one **unseeded** live `heliox tool braze -- campaigns list` through the real token gateway succeeds; a deliberately-wrong key is confirmed to surface as a distinct `401` on first call, and a read-scoped key on `messages send` as a distinct `403`. Agent-drivable (api_key L5 lane; agent-browser through the connect UI); human fallback on UI breakage. | **Yes** — real workspace key (account pool) |

**Externally-supplied credentials needed:** L2, L4, L5 — one real Braze workspace with a REST API key (ideally two keys: one read-scoped, one send-scoped, to exercise the `403` permission path) and its cluster REST host. Braze offers free sandbox/developer workspaces, so the account-pool lane can procure this without a paid plan.

## 8. Definition of done (per master plan §2)

L1–L5 green · AI-facing sub-doc published under `agents/plugins/heliox/skills/tool/` (new `braze` page: the read + act verb tables; **the exact `connection_string` DSN shape and where to find the cluster REST host**; that the key's **permissions are the guardrail** — a `403` means "reconnect with a broader-scoped key," a `401` means "the key is wrong/revoked"; the rate-limit note with back-off-on-`429` guidance; and that user-profile/messaging writes act on live customer data) + plugin version bump (batch-end) · UI icon `ui/helio-app/src/integrations/icons/braze.svg` + `providerIcons.ts` registration · i18n `tools.desc.braze` + `braze_connection_string` label key across all locales · then the visible flip (`presentation.visible: true` + regenerate) as the single go-live change. **No `toolToProvider` entry** (axes identical). Until the flip: code-complete (hidden).

---

### Sources (official, verified 2026-07-22)
- Auth (Bearer REST API key), request basics, instance/host table: https://www.braze.com/docs/api/basics/
- Creating & scoping REST API keys (immutable permissions, per-endpoint): https://www.braze.com/docs/api/api_key/
- Instance → REST endpoint mapping: https://www.braze.com/docs/user_guide/administrative/access_braze/braze_instances/
- Messaging endpoints (send / trigger / schedule): https://www.braze.com/docs/api/endpoints/messaging/
- Export endpoints (campaigns / Canvas / segments / KPI / events / purchases / sessions / user profile): https://www.braze.com/docs/api/endpoints/export/
- Rate limits: https://www.braze.com/docs/api/api_limits/
