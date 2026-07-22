# Tool provider design — SendGrid

- **anycli id (axis ②):** `sendgrid`
- **provider catalog key (axis ③):** `sendgrid`
- **CLI command word (axis ①):** `sendgrid` (flat command, no group)
- **auth lane:** `api_key` (catalog row 52; OAuth audit row 52 — "no viable multi-tenant path", stays `api_key`)
- **wave / category:** Wave 1 · Email & Messaging
- **go package (stage 2):** `internal/tools/sendgrid`
- **RegisterService string:** `sendgrid`

All three naming axes are identical (`sendgrid`/`sendgrid`/`sendgrid`), so **no
`toolToProvider` divergence entry** is added in `resolver.go`, no `tool.command`
override, and no `tool.group`. Simplest naming shape (same as `loops`,
`postmark`, `resend`).

---

## 1. Verification of the catalog against official docs

The catalog and the OAuth audit both place SendGrid in the `api_key` lane. I
independently checked the official Twilio SendGrid v3 API and confirm this is
correct — **no divergence to record on the auth lane**:

- **Auth model.** The v3 API authenticates with a single scheme:
  `Authorization: Bearer <API_KEY>` on every request
  ([authentication docs](https://www.twilio.com/docs/sendgrid/api-reference/how-to-use-the-sendgrid-v3-api/authentication)).
  There is **no OAuth authorization-code flow, no `/authorize` or `/token`
  endpoint, and no client-registration / multi-tenant app model**. Keys are
  minted in-app (Settings → API Keys, `https://app.sendgrid.com/settings/api_keys`)
  and revoked from the same screen. This is a per-account bearer key with no
  multi-tenant authorization path — matching the audit rubric for `api_key`
  exactly. (SendGrid is a Twilio product; Twilio's own account uses auth
  tokens, not OAuth, and SendGrid inherits the key model. Nothing here is an
  OAuth candidate.)
- **Key scoping / token semantics.** Keys are long-lived (no expiry, no refresh
  cycle) and carry a **scope set** chosen at creation:
  - **Full Access** — all endpoints except billing + Email Validation.
  - **Restricted Access** — a custom subset (e.g. a `mail.send`-only key).
  - **Billing Access** — billing endpoints only.
  A least-privilege AI-teammate deployment would use a **Restricted key with
  `mail.send`** (± marketing scopes) — this shapes the identity design in §4.

**Divergence on the auth lane: none.** Official docs agree with both the catalog
lane (`api_key`) and the audit verdict.

**One host divergence worth recording (data residency).** SendGrid offers EU
data residency: the default/global host is `https://api.sendgrid.com` and the EU
host is `https://api.eu.sendgrid.com`
([EU data residency](https://www.twilio.com/en-us/blog/send-emails-in-eu)).
Residency is bound to the **key** (an EU subuser's key must call the EU host;
calling the global host routes data globally). v1 defaults to the global host
with an explicit region override — see §3 and §7.

---

## 2. Which API surface this tool wraps, and why

**Base URL:** `https://api.sendgrid.com/v3` (all endpoints carry the `/v3/…`
path). EU override: `https://api.eu.sendgrid.com/v3`.

SendGrid's v3 API is very large (mail send, marketing campaigns/automations,
contacts, lists/segments, suppressions, stats, subusers, IP pools, domain/link
authentication, webhooks, alerts, teammates, SSO). An AI teammate's real jobs
with SendGrid are **transactional send + audience/deliverability hygiene**: send
a (possibly templated) email, add/look-up a marketing contact, check what
bounced/unsubscribed, and read sending stats. The heavy authoring/admin surface
(IP management, domain auth, webhooks, teammates, campaign builders) is
account-setup territory a human configures in the SendGrid UI — not what a
teammate drives from chat.

So the tool wraps the **send + hygiene + read core**, mirroring the shaped
subset the sibling `loops` / `postmark` / `resend` / `brevo` tools expose.
Endpoints, all verified against the official v3 reference:

| Command | Method + path | Purpose |
|---|---|---|
| `sendgrid scopes` | `GET /v3/scopes` | Verify key + list its granted scopes (also the identity endpoint, §4) |
| `sendgrid mail send` | `POST /v3/mail/send` | Send an email (raw content **or** `template_id` + `dynamic_template_data`) |
| `sendgrid template list` | `GET /v3/templates?generations=dynamic` | List dynamic transactional templates (ids for `mail send`) |
| `sendgrid template get` | `GET /v3/templates/{id}` | Template detail incl. versions |
| `sendgrid contact upsert` | `PUT /v3/marketing/contacts` | Add/update marketing contacts (async → `job_id`) |
| `sendgrid contact search` | `POST /v3/marketing/contacts/search/emails` | Look up contacts by email address(es) |
| `sendgrid list ls` | `GET /v3/marketing/lists` | Marketing lists (ids for targeting) |
| `sendgrid suppression bounces` | `GET /v3/suppression/bounces` | Bounced addresses |
| `sendgrid suppression unsubscribes` | `GET /v3/suppression/unsubscribes` | Global unsubscribes |
| `sendgrid suppression blocks` | `GET /v3/suppression/blocks` | Blocked addresses |
| `sendgrid stats` | `GET /v3/stats?start_date=…` | Aggregated email stats |
| `sendgrid sender list` | `GET /v3/verified_senders` | Verified sender identities (valid `from` addresses) |

**Deliberately out of scope for v1** (documented, not wrapped): marketing
campaigns/automations authoring, single-sends, segments builder, subusers, IP
pools/warmup, domain/link authentication, event-webhook + inbound-parse
settings, alerts, teammates, SSO. These are low-frequency admin/authoring
operations. Following the `loops` precedent, v1 ships **without** a generic
`sendgrid api <method> <path>` passthrough — add one only if a concrete teammate
need appears; do not pre-build the tree.

### Request/response quirks (load-bearing for the impl)

- **`mail send` returns `202 Accepted` with an EMPTY body** and the tracking id
  in the **`X-Message-Id` response header** (`Content-Length: 0`)
  ([X-Message-Id](https://www.twilio.com/docs/sendgrid/glossary/x-message-id)).
  The service must treat `202` as success, **not** try to JSON-decode the empty
  body, and synthesize a useful stdout object, e.g.
  `{"status":"accepted","message_id":"<X-Message-Id>"}`. `202` is API-layer
  acceptance only (not delivery) — the tool reports acceptance honestly and does
  not claim delivery. Building this wrong (expecting a JSON body) would fail on
  every send and an L1 fake would lock in the wrong contract — exactly the quirk
  L2 exists to catch.
- **`mail send` body** (v3 Mail Send schema): `personalizations[]` (recipients),
  a verified `from`, `subject`, and `content[]` (`type` + `value`). Templated
  sends use `template_id` + per-personalization `dynamic_template_data` (and may
  omit `subject`/`content`). The CLI exposes ergonomic flags for the common case
  — `--to` (repeatable), `--from`, `--from-name`, `--subject`, `--text`,
  `--html`, `--template-id`, `--data '<json>'` (dynamic template data), `--cc`,
  `--bcc`, `--reply-to` — and builds the `personalizations`/`content` arrays
  itself, plus a `--json '<full v3 body>'` escape hatch for advanced sends
  (multiple personalizations, attachments, categories, send_at, ASM). This
  mirrors `loops`' first-class-flags approach.
- **`contact upsert` is asynchronous and eventually consistent.**
  `PUT /v3/marketing/contacts` returns `202` with a JSON `{job_id}` — the
  contacts are **queued**, not yet stored
  ([add-or-update-a-contact](https://www.twilio.com/docs/sendgrid/api-reference/contacts/add-or-update-a-contact)).
  The command surfaces the `job_id` verbatim and its help text states the
  caller must use `contact search` to confirm the contact exists (no
  silent "created" claim — matches the repo's no-silent-success stance). Body
  requires at least one identifier (`email`, `phone_number`, or external id) per
  contact; the CLI exposes `--email`, `--first-name`, `--last-name`, repeatable
  `--custom-field key=value`, and `--json` for bulk.
- **Scopes endpoint is universal.** `GET /v3/scopes` returns
  `{"scopes":["mail.send","alerts.read",…]}` for **any** valid key (it reports
  the key's own scopes; not itself scope-gated) and `401` on an invalid key.
  This is the only endpoint every valid key — including a restricted
  `mail.send`-only key — can read, which is why it is both the verify and the
  identity endpoint (§4).

---

## 3. anycli definition

**Type: `service`** (per stage-1 rubric). No official non-interactive
`--json`-capable SendGrid binary exists to wrap (there are language SDKs, not a
CLI); the tool is a thin cobra tree over the v3 REST API — identical shape to
`bitly` / `notion` / `loops`. 21 of 23 shipped definitions are `service` type;
this is one more.

### `definitions/tools/sendgrid.json`

```json
{
  "name": "sendgrid",
  "type": "service",
  "description": "SendGrid as a tool (transactional mail send, templates, marketing contacts, suppressions, stats; API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "SENDGRID_API_KEY"}
      }
    ]
  }
}
```

The credential field name is `api_key` (matches the Helio bundle's
`credential.fields` key, §5). anycli injects it as `SENDGRID_API_KEY`; the
service builds `Authorization: Bearer <SENDGRID_API_KEY>` itself on every
request.

### `internal/tools/sendgrid/` (service impl)

Copy the `bitly`/`loops` package shape (the in-tree `service`-type + Bearer-auth
precedent):

- `sendgrid.go` — `Service{ BaseURL, HC, Out, Err }`, `Execute(ctx, args, env)`
  reads `SENDGRID_API_KEY` (empty → stderr + exit 1), builds the cobra root,
  wires subcommands. `DefaultBaseURL = "https://api.sendgrid.com/v3"`. A
  persistent `--region {global,eu}` flag (env fallback `SENDGRID_REGION`) swaps
  the base to `https://api.eu.sendgrid.com/v3` when `eu` — the only per-request
  host divergence; default `global`.
- `client.go` — one `call(ctx, method, path, query, payload)` helper:
  `Authorization: Bearer <token>`, `Accept: application/json`, JSON body when
  present. Non-2xx → typed `apiError` (SendGrid error bodies are
  `{"errors":[{"field","message"}]}` — extract `message`(s) for the failure
  text); **401 → `execution.RejectCredential`** so the token gateway learns the
  key is dead. **`202` with empty body is a success path** (not an error, not a
  decode) — the `mail send` handler reads `X-Message-Id` from the response
  header and emits a synthetic acceptance object.
- Resource files: `mail.go` (`mail send`), `template.go`, `contact.go`
  (`contact upsert|search`), `list.go`, `suppression.go`
  (`bounces|unsubscribes|blocks`), `stats.go`, `sender.go`, `scopes.go`.
- `harness_test.go` + per-file `_test.go` — `httptest.Server` fakes asserting
  method, path, query, `Authorization: Bearer …` header, request bodies
  (personalizations/content assembly from flags, template send, contact upsert),
  the **`202`/empty-body/`X-Message-Id` mail-send success path**, `401`
  →`RejectCredential`, `--json` error envelope, exit codes. **Never hits the
  real API** (that is L2). TDD: tests first, per anycli AGENTS.md.

**JSON output shape.** Every command emits the provider's JSON response verbatim
to stdout (+ trailing newline), passthrough style — exactly like `bitly`/`loops`
— **except** `mail send`, whose provider response is an empty `202` body, so it
emits a synthesized `{"status":"accepted","message_id":"<X-Message-Id>"}`. A
persistent `--json` flag is accepted for uniformity. **Exit codes:** `0`
success · `1` runtime/API failure (typed `apiError`; `401` also rejects the
credential) · `2` usage/parse errors (cobra).

### `internal/tools/register.go`

Add the import and `RegisterService("sendgrid", &sendgrid.Service{})`. This is a
**batch-serialized shared surface** (master plan §2) — merged by the batch lead
at batch end, not mid-batch. The per-tool `definitions/tools/sendgrid.json` and
the `internal/tools/sendgrid/` package are generation-inert and merge freely
before then.

---

## 4. Credential fields and the exact auth flow

**Credential:** one field — the SendGrid API key (`api_key`), a long-lived
bearer token. No secret pair, no refresh, no expiry.

**Connect-time flow (Helio side, `manual_api_token` strategy):**

1. User pastes the API key into the connect form (single secret; stored via the
   write-only `POST /connections/credentials` path into Vault — never in the
   bundle).
2. integration-service **verifies before storing** by GETing
   `https://api.sendgrid.com/v3/scopes` with `Authorization: Bearer <key>`.
   - **Success (`200`)** → `{"scopes":[…]}`; the key is valid and its scope set
     is captured into connection identity metadata.
   - **`401/403`** → verifier maps to `invalid_provider_credential`; connect
     fails cleanly before anything reaches Vault.
3. Runtime: the token gateway serves the stored key; heliox injects it into
   anycli's credential map (`api_key`), anycli sets `SENDGRID_API_KEY`, and the
   service sends `Authorization: Bearer <key>` to the live API.

### The two capability gaps SendGrid hits (and why a bespoke verifier is right)

The stock `declarativeManualTokenVerifier`
(`service/manual_token_verifier.go`) on this worktree base does **two** things
that SendGrid breaks:

1. It sets the identity header to the **raw token**
   (`req.Header.Set(APIKey.Header, token)`) — no `Bearer ` prefix. SendGrid
   requires `Authorization: Bearer <key>`, so the raw token would be rejected
   `401` even for a valid key. (No `scheme: bearer` capability exists on this
   base — I grepped `model/catalog.go`, `cmd/provider-gen/manifest.go`,
   `manual_token_verifier.go`; the `loops`/`tally` "Bearer scheme" growth has
   not landed here.)
2. It **requires a non-empty string** at `Identity.StableKey` (errors otherwise).
   `GET /v3/scopes` returns `{"scopes":[…]}` — an **array, no identity string**.
   And SendGrid exposes **no** universally-readable identity: `/v3/user/email`,
   `/v3/user/profile`, `/v3/user/username`, `/v3/user/account` are all
   **scope-gated** and return `403` for a least-privilege `mail.send`-only key,
   which is exactly the key a security-conscious teammate deployment uses. So
   there is no field the declarative verifier can point `stable_key` at that
   works for every valid key.

Both gaps mean the declarative path can't serve SendGrid as-is. The right move
is a **small provider-registered verifier**, following the established program
pattern (`courierBrandsVerifier`, `sproutClientVerifier`, and the fullstory /
moz / semrush verifiers are all registered per-provider in
`service/provider_registry.go` alongside the default
`declarativeManualTokenVerifier`). Proposed **`sendgridScopesVerifier`**:

- `GET {Identity.URL}` (`…/v3/scopes`) with `Authorization: Bearer <token>`.
- `200` → **valid**. Capture granted scopes into the returned `identity`
  metadata map (useful signal for the UI/AI: is `mail.send` present?).
- `401/403` → `manualTokenHTTPError{status}` so `manual_credential.go` maps it to
  `invalid_provider_credential` (that mapping already exists, lines 81-84).
- **Identity (account_key + label) — best-effort, degrade honestly.** After the
  `200`, attempt a best-effort `GET /v3/user/email` (or `/v3/user/account`):
  - **succeeds** (full-access or user-scoped key) → label = the account email;
    `account_key` = the SendGrid account/user identity (so two keys from the
    same account upsert to **one** connection — the newer key supersedes, which
    is the desired behavior).
  - **`403`** (restricted `mail.send`-only key, no user scope) → fall back to a
    generic label (`"SendGrid"`, or `"SendGrid (EU)"` if region-tagged) and a
    **deterministic** `account_key` derived from the key (truncated SHA-256 of
    the token — stable, one-way, keeps idempotent replace working). This is a
    deliberate, documented deviation from the repo's "human-readable, never a
    hash" account-key preference, justified because SendGrid publishes **no**
    universally-readable stable id for a least-privilege key. See §7 open
    decision 1.

This is the honest "fail fast, no silent fallback" shape: a valid key always
connects (even a restricted one), an invalid key is rejected pre-Vault, and the
label is as good as the key's scopes allow. The **runtime path is unaffected**
by all of this — anycli always builds its own `Bearer` header; the verifier
touches **connect-time verification/identity only**.

**Reuse-first note.** If a sibling Bearer-scheme `scheme` capability
(`loops`/`tally`) lands on main in the same batch, SendGrid could reuse it for
the *validity* check — but SendGrid still needs the no-identity-string +
best-effort-label handling that the declarative verifier cannot express, so a
bespoke `sendgridScopesVerifier` is the right call **regardless**. Coordinate
with the batch lead so exactly one branch owns any shared `scheme` growth.

---

## 5. Helio provider bundle plan (`integrations/providers/sendgrid/provider.yaml`)

Hidden-first (`presentation.visible: false`). `manual_api_token` runtime
strategy (the api_key manual path) with a **provider-registered verifier**
(§4) — no OAuth, no client secret.

```yaml
schema: helio.provider/v1
key: sendgrid
go_name: SendGrid

presentation:
  name: SendGrid
  description_key: sendgrid
  consent_domain: sendgrid.com
  visible: false          # flip only after L5 (master plan §2 hidden-first)
  # order: <pick an unoccupied slot at flip time>

auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    setup_url: https://app.sendgrid.com/settings/api_keys

identity:
  source: userinfo
  url: https://api.sendgrid.com/v3/scopes   # universal verify endpoint (§4)
  # No stable_key/label_candidates: the sendgridScopesVerifier derives
  # account_key + label itself (scopes has no identity string).

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

# Single secret stored through the existing UpsertUserToken write path
# (token.access_token), exactly like mongodb/loops — zero new CredentialSource.
credential:
  fields:
    api_key: token.access_token
    account_key: connection.account_key

tool:
  name: sendgrid
  kind: api-key
```

**Axis mapping.** ① CLI word `sendgrid` · ② anycli id `sendgrid` · ③ key
`sendgrid` — all identical → no `tool.command`, no `tool.group`, **no
`toolToProvider` entry**.

**Generation.** From `go-services/integration-service`: `go run ./cmd/provider-gen`
then `--check`. Five projections regenerate together and are committed by the
**batch lead at batch end** — per the master plan §2 and the hard constraint,
this agent runs provider-gen **locally for validation only** and does **not**
commit the regenerated projections. The tool branch is expected to fail
`provider-gen --check` in CI until the batch-end merge.

**Config.** `manual_api_token` needs **no** `required_config_fields` and no
integration-service OAuth config (no client id/secret) — so there is **no
`config/` + `deploy/` Secret append** for SendGrid (the user's own key is the
only credential, entered at connect and stored in Vault). This sidesteps human
lane 1 entirely.

**UI + docs (non-generated, per-tool):**
- Icon: `ui/helio-app/src/integrations/icons/sendgrid.svg` + register in
  `providerIcons.ts` (manual append; batch-serialized shared surface).
- i18n: `tools.desc.sendgrid` (and `tools.scopes.*` if display scopes are shown)
  across locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` documenting the
  command tree, the **`mail send` template-id + `dynamic_template_data`
  contract**, the `202`/`X-Message-Id` "accepted ≠ delivered" semantics, the
  **verified-`from`-address requirement** (`mail send` fails if `from` is not a
  verified sender — point to `sender list`), and the **async `contact upsert`
  → `job_id` → `contact search` to confirm** pattern; plugin version bump +
  marketplace publish ride the batch-end merge.

---

## 6. Test plan — the five layers

| Layer | What it proves for SendGrid | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: `sendgrid` service + definition unit tests against `httptest` fakes — asserts method/path/query, `Authorization: Bearer <key>` header, request bodies (personalizations/content assembly from `--to/--from/--subject/--text/--html`, template send with `dynamic_template_data`, contact upsert), the **`202`/empty-body/`X-Message-Id` mail-send success path**, `--region eu` host swap, SendGrid `{"errors":[…]}` error rendering (plain + `--json`), `401`→`RejectCredential`, exit codes 0/1/2. | No (fakes) |
| **L2** | Dev harness against the **real** SendGrid API: `ANYCLI_CRED_API_KEY=<key> anycli sendgrid -- scopes` (expect `{"scopes":[…]}`), then `sender list`, `mail send` to a real inbox (expect `202` + a real `X-Message-Id`), `template list`, `contact upsert` + `contact search`, `suppression bounces`, `stats`. Proves field names + Bearer injection + the `202`/empty-body handling + request shapes match the live API. | **Yes** — a real SendGrid account API key + a verified sender (test-account pool, master plan lane 2) |
| **L3** | `provider-gen --check` (run locally on-branch; expected red in CI until batch end) + both repos' unit suites: `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`, and `make test-integration-service` including the new `sendgridScopesVerifier` test (200→identity, 401/403→`invalid_provider_credential`, restricted-key best-effort fallback). Point `helio-cli/go.mod` at the anycli branch via a **local, uncommitted** `replace`. | No |
| **L4** | Singleton + seed a real key through the token gateway: `POST /internal/test-only/connections/seed` with `provider: sendgrid`, `access_token: <real key>`, a **real** seeded org/assistant identity → `201`; then `heliox tool sendgrid -- scopes` and `heliox tool sendgrid -- mail send …` reach the live API. Seeds `access_token` only (no refresh/expiry — non-expiring key). | **Yes** — real key + verified sender (same as L2) |
| **L5** | One full connect flow, still hidden, via the **api_key key-entry path** (master plan §2, human lane 3, agent-drivable): open the connect link → paste the key through the real connect UI → integration-service verifies it via `sendgridScopesVerifier` against `GET /v3/scopes` (identity/label derived) → connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool sendgrid -- scopes` (and a real `mail send`) succeeds through the real token gateway. | **Yes** — real key + verified sender |

**Externally-supplied-credential layers: L2, L4, L5** — all need one real
SendGrid account API key **plus a verified sender identity** from the
test-account pool (a `mail send` L2/L5 with an unverified `from` returns `403`,
so the sender verification is part of the account setup, not an API bug). L1 and
L3 are hermetic. There is **no OAuth app registration** (human lane 1 does not
apply); the only external dependency is the account/key + verified sender.

**Rollout (master plan §2 / skill stage 10):** land hidden with L1–L4 green +
the `sendgridScopesVerifier` capability; run the L5 key-entry sweep at batch
end; then flip `presentation.visible: true` + regenerate as the single go-live
change.

---

## 7. Open decisions

1. **Identity for restricted keys (the account-key deviation).** A least-privilege
   `mail.send`-only key can't read `/v3/user/*` (all `403`), so no
   human-readable account identity is available. Recommended (§4): best-effort
   `/v3/user/email` for a nice label when scopes allow, else generic label +
   **deterministic token-hash `account_key`** to preserve idempotent replace.
   The alternative — hard-requiring a user-scoped key — would reject exactly the
   least-privilege keys we want to encourage, so it's rejected. Revisit only if
   the token-hash account key proves confusing in the UI.
2. **EU data residency host.** v1 defaults to the global host with an anycli
   `--region eu` runtime flag (§3). Two follow-ups if EU demand appears: (a) the
   integration-service verifier host is fixed to `api.sendgrid.com`, so an
   **EU-only** subuser key may not verify against the global host — the bundle
   would then need a region-selectable identity host (a small capability), and
   (b) the region must persist from connect through to runtime injection (a
   connection-metadata field feeding the anycli `--region` flag), rather than
   being re-specified per call. Deferred; global-first covers the common case.
3. **Generic passthrough / builder surface.** Marketing campaigns, single-sends,
   segments, subusers, IP/domain auth, webhooks are out of v1 scope. Revisit
   with a single generic `sendgrid api <method> <path>` passthrough only if a
   concrete teammate need appears — do not pre-build the tree (loops precedent).
4. **`contact upsert` async confirmation.** v1 surfaces the raw `job_id` and
   documents the `contact search`-to-confirm pattern rather than auto-polling
   the import-status endpoint. A `contact upsert --wait` that polls
   `GET /v3/marketing/contacts/imports/{job_id}` is a possible ergonomic
   follow-up; kept out of v1 to keep the surface thin and non-blocking.
