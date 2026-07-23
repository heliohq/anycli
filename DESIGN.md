# Tool provider design — SendGrid

- **anycli id (axis ②):** `sendgrid`
- **provider catalog key (axis ③):** `sendgrid`
- **CLI command word (axis ①):** `sendgrid` (flat command, no group)
- **auth lane:** `api_key` (catalog row 52; OAuth audit row 52 — "no viable multi-tenant path", stays `api_key`). NB: the catalog's "api_key lane" is the OAuth-vs-key **bucket**, not the provider bundle's `auth.type` axis — the bundle uses internal `auth.type: credentials` on the `manual_credentials` / `strategy`-identity lane (the shipped `mongodb` base), see §4/§5.
- **wave / category:** Wave 1 · Email & Messaging
- **go package (stage 2):** `internal/tools/sendgrid`
- **RegisterService string:** `sendgrid`

All three naming axes are identical (`sendgrid`/`sendgrid`/`sendgrid`), so **no
`toolToProvider` divergence entry** is added in `resolver.go`, no `tool.command`
override, and no `tool.group`. Simplest naming shape — the same all-identical
axes as the shipped `bitly` / `notion` tools on this base (the email-lane
siblings `loops` / `postmark` / `resend` are parallel-program branches, not yet
on this base).

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
- **Reading a key's own scopes is itself scope-gated (corrected — verify at
  stage 1).** An earlier draft asserted `GET /v3/scopes` is a "universal"
  endpoint readable by *any* valid key. **That is wrong.** Reading the API-Keys
  scope list is a permission a `mail.send`-only Restricted key does **not**
  carry, so such a key returns **`403` Access Forbidden** on `GET /v3/scopes`,
  not `200`. This is well documented: SendGrid's endpoint reference lists a
  `403` response for that route, SendGrid support and multiple integrator
  threads (Courier/Workato/Strapi) confirm a restricted key `403`s there, and
  `403` is the *healthy* signal — RFC 9110 §15.5.4 `403` means the server
  authenticated the key but refuses the request for insufficient scope, whereas
  a genuinely dead/missing key returns **`401` Unauthorized**. The
  `401`-vs-`403` split is the load-bearing fact the verifier design in §4 rests
  on, and it is a **stage-1/L2 gate**, not a settled premise: test `GET
  /v3/scopes` (and any candidate identity endpoint) with **both** a Full Access
  key **and** a real `mail.send`-only Restricted key before building the
  verifier (§6 L2).

**Divergence on the auth lane: none.** Official docs agree with both the catalog
lane (`api_key`) and the audit verdict. The only sub-claim that changed on
review is the endpoint-readability property above (`/v3/scopes` is scope-gated,
not universal); the `api_key` lane itself is unaffected and correct.

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
subset the email-lane siblings (`loops` / `postmark` / `resend` / `brevo`, all
parallel-program branches — not on this base) expose. Endpoints, all verified
against the official v3 reference:

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
operations. Following the thin-surface principle the parallel-program email
siblings adopt, v1 ships **without** a generic `sendgrid api <method> <path>`
passthrough — add one only if a concrete teammate need appears; do not pre-build
the tree.

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
  first-class-flags-plus-`--json`-escape-hatch shape is the same pattern the
  email-lane siblings adopt.
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
- **Scopes endpoint is itself scope-gated — `401`-vs-`403` is the validity
  signal.** `GET /v3/scopes` returns `{"scopes":["mail.send",…]}` (`200`) **only
  for a key that carries API-Keys read permission** (Full Access, or a Restricted
  key that included the scopes/API-Keys read scope). A least-privilege
  `mail.send`-only Restricted key **lacks** that permission and returns **`403`
  Access Forbidden** — the key authenticated fine, it just may not read the
  scope list. A missing/invalid/revoked key returns **`401` Unauthorized**. The
  same gating applies to `/v3/user/*` (email/profile/account), so there is **no**
  read endpoint a `mail.send`-only key can `200` on. The verifier (§4) therefore
  does **not** rely on reading any endpoint for a restricted key; it treats the
  `401`-vs-`403` distinction itself as the connect-time validity signal:
  `200` → verified (+scopes captured), `403` → valid-but-restricted (still
  connects), `401` → rejected. Verifying this exact status split with a real
  restricted key is the §6 L2 gate.

---

## 3. anycli definition

**Type: `service`** (per stage-1 rubric). No official non-interactive
`--json`-capable SendGrid binary exists to wrap (there are language SDKs, not a
CLI); the tool is a thin cobra tree over the v3 REST API — identical shape to the
shipped `bitly` / `notion` service tools on this base. Most shipped definitions
are `service` type; this is one more.

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

Copy the `bitly`/`notion` package shape (the in-tree `service`-type + Bearer-auth
precedent on this base):

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
  text); **only `401` → `execution.RejectCredential`** (dead/revoked key) so the
  token gateway learns the key is unusable. A runtime **`403` is a normal scope
  error** (the key is valid but lacks the scope for *this* operation, or the
  `from` isn't a verified sender) → plain `apiError`/exit 1, **not** a credential
  rejection — mirroring the connect-time `401`-vs-`403` split (§4). **`202` with
  empty body is a success path** (not an error, not a decode) — the `mail send`
  handler reads `X-Message-Id` from the response header and emits a synthetic
  acceptance object.
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
to stdout (+ trailing newline), passthrough style — exactly like `bitly`/`notion`
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

**Connect-time flow (Helio side, `manual_credentials` strategy):**

1. User pastes the API key into the connect form (single secret; stored via the
   write-only `POST /connections/credentials` path into Vault — never in the
   bundle).
2. integration-service **verifies before storing** by GETing
   `https://api.sendgrid.com/v3/scopes` with `Authorization: Bearer <key>` and
   **branches on the exact status code** (the `401`-vs-`403` split from §2):
   - **`200`** → `{"scopes":[…]}`; the key is valid **and** carries scope-read
     permission. Capture the scope set into connection identity metadata.
   - **`403`** → the key **authenticated** but lacks scope-read permission — this
     is the healthy signal for a least-privilege `mail.send`-only Restricted key.
     Treat as **valid-but-restricted**: the connection still succeeds (no scopes
     captured). This is the key case the design exists to support.
   - **`401`** → missing/invalid/revoked key → `invalid_provider_credential`;
     connect fails cleanly before anything reaches Vault.
3. Runtime: the token gateway serves the stored key; heliox injects it into
   anycli's credential map (`api_key`), anycli sets `SENDGRID_API_KEY`, and the
   service sends `Authorization: Bearer <key>` to the live API.

The design goal is **"a valid key always connects, even a restricted one; only a
dead key is rejected."** Because `/v3/scopes` (and every `/v3/user/*` endpoint)
is scope-gated (§2), there is **no read endpoint a `mail.send`-only key can
`200` on** — so the verifier must **not** demand a positive read; it uses the
`403` (authenticated-but-forbidden) as its own positive-validity signal and
reserves rejection for `401` alone. Mapping `403 → invalid_provider_credential`
(as the shared `manual_credential.go` path does for both `401` **and** `403`
whenever the verifier hands it a `manualTokenHTTPError`) would reject exactly the
recommended least-privilege key — that is the connect-time regression this design
must avoid.

### Why the `manual_credentials` lane + a compiled verifier

**Lane choice — internal `auth.type: credentials`, not `api_key`.** The catalog/
audit bucket SendGrid in the "`api_key`" auth *lane* (a user-supplied key, no
OAuth) — correct and unchanged. But the provider bundle's internal `auth.type` is
a different axis. On this base `auth.type: api_key` is coupled to the
**declarative userinfo path**: `declarativeManualTokenVerifier`
(`service/manual_token_verifier.go`) GETs `identity.url` with the
bundle-declared header and extracts a **readable `stable_key`** JSON Pointer from
the response — the shape `bitly`/`notion` use because their key can read a
`/user`-style endpoint carrying a stable account id. A least-privilege
`mail.send`-only SendGrid key can read **no** such id (`/v3/scopes` and every
`/v3/user/*` return `403`, §2), so there is nothing to point `stable_key` at.

That omission is fatal at generation time, not runtime: `provider-gen`'s
`validateIdentity` (`cmd/provider-gen/validate.go`) enforces, for **any**
`identity.source` other than `strategy`, a non-empty `stable_key` JSON Pointer
**and** non-empty `label_candidates` — keyed on `identity.source` alone,
independent of the runtime strategy or which verifier is registered. A `userinfo`
manifest with those omitted therefore **fails `go run ./cmd/provider-gen`
outright** ("identity.stable_key must be a non-empty JSON Pointer") before any
`--check` diff. `identity.source: strategy` is the **one** source
`validateIdentity` exempts from that requirement, and it additionally **forbids
`identity.url`** for non-`userinfo` sources — so the verify endpoint must live in
the compiled verifier regardless.

The correct in-tree shape for "user-pasted secret + bespoke liveness check + no
readable stable id" is therefore the **`manual_credentials` lane with
`identity.source: strategy`** — the exact base the shipped **`mongodb`** bundle
uses (`integrations/providers/mongodb/provider.yaml`). That is the only such
provider on this worktree; the `courier`/`braze`/`crisp` siblings the review
cites as precedents are **parallel-program branches, not present on this base**
(verified: `ls integrations/providers/` shows neither) — so `mongodb` is the
precedent this design follows.

**The one enumerated integration-service change.** `manual_credentials` verifiers
are selected in `composeProviderRegistration` (`service/provider_registry.go`),
whose `case model.RuntimeStrategyManualCredentials` **hardcodes**
`dsnHostIdentityDeriver{}` today — a no-HTTP host deriver
(`service/manual_credentials_identity.go`) that would never call SendGrid.
SendGrid needs a verifier that actually GETs `/v3/scopes`, so that one case is
changed to **dispatch by provider key** — `sendgrid → sendgridScopesVerifier{}`,
default `→ dsnHostIdentityDeriver{}` — mirroring how
`composeExplicitOAuthRegistration` already switches by strategy. No new
interface, no `registration.manual` plumbing change (the field already carries a
`manualTokenVerifier`; `newProviderRegistry` already routes `AuthCredentials` to
`manualTokenRuntime`), and **no `validateIdentity` relaxation**.

**How `403` becomes a success (not a rejection).** The shared
`ManualCredentialService.Connect` (`service/manual_credential.go`) drives the
verifier for both `api_key` and `credentials` auth types and maps a returned
`manualTokenHTTPError` whose status is `401` **or** `403` to
`invalid_provider_credential`. The verifier controls the outcome purely by
**which error (if any) it returns**: it must **not** return a
`manualTokenHTTPError` for `403`. `sendgridScopesVerifier.Verify(ctx, client,
definition, token)` (the `manualTokenVerifier` interface,
`service/manual_token_verifier.go`) therefore:

- `GET https://api.sendgrid.com/v3/scopes` — its **own** compiled URL (the bundle
  carries none, since `identity.source: strategy` forbids `identity.url`; the
  verifier holds the constant exactly as `linkedinAdapter`/`xAdapter` hold their
  `meURL`) — with `Authorization: Bearer <token>`, `Accept: application/json`.
- **`200`** → **valid + scope-readable.** Decode `{"scopes":[…]}` into the
  returned `identity` map (useful UI/AI signal: is `mail.send` present?). Then a
  **best-effort** `GET /v3/user/email` for a human label: on success → `label` =
  account email, `accountKey` = that stable account identity (two keys from one
  account upsert to **one** connection, newer supersedes); on `403` → fall
  through to the restricted-label path below.
- **`403`** → **valid-but-restricted.** Return **`nil` error** (success) — **not**
  a `manualTokenHTTPError`, so `manual_credential.go` does not reject it. No
  scopes captured; `label` = a generic `"SendGrid"` (`"SendGrid (EU)"` if
  region-tagged); `accountKey` = a **deterministic** value derived from the key
  (truncated SHA-256 of the token — stable, one-way, keeps idempotent replace
  working). This is a deliberate, documented deviation from `mongodb`'s
  human-readable (DSN-host) account key, justified because SendGrid publishes
  **no** stable id a least-privilege key can read — the very reason the
  `strategy`/fingerprint lane fits and the `userinfo`/readable-`stable_key` lane
  (the retired figma PAT `/v2/me` shape) cannot. See §7 open decision 1.
- **`401`** → return `manualTokenHTTPError{status: 401}` so `manual_credential.go`
  maps it to `invalid_provider_credential`. **This is the only reject path.**
- **Other non-2xx (5xx / network)** → wrap and return a plain error → the caller
  surfaces `Internal` (transient), not a credential rejection.

This is the honest "fail fast, no silent fallback" shape done correctly for a
scope-gated provider: a valid key always connects (restricted included via the
`403` path), only a genuinely dead `401` key is rejected pre-Vault, and the label
is as good as the key's scopes allow. The **runtime path is unaffected** — anycli
always builds its own `Bearer` header from the injected `api_key`, and the
compiled verifier owns its own header too, so **no** shared `scheme`/Bearer
capability is needed and there is **no** dependency on any parallel-program
branch.

---

## 5. Helio provider bundle plan (`integrations/providers/sendgrid/provider.yaml`)

Hidden-first (`presentation.visible: false`). `manual_credentials` runtime
strategy with `identity.source: strategy` and a **provider-registered compiled
verifier** (§4) — the same base the shipped `mongodb` bundle uses; no OAuth, no
client secret. (Internal `auth.type: credentials`; the catalog's "api_key lane"
is the OAuth-vs-key bucket, a different axis — see §4.)

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

# auth.type credentials (design 317 D5): the user pastes a single SendGrid API
# key. Unlike bitly/notion (api_key + userinfo), a least-privilege mail.send-only
# key can read no stable account id (/v3/scopes and /v3/user/* 403), so there is
# no readable stable_key — the mongodb-style credentials/strategy lane, verified
# by a compiled verifier (§4), is the correct shape.
auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: sendgrid_api_key
        secret: true
        required: true
        placeholder: "SG.xxxxxxxxxxxxxxxxxxxxxx"
    # Points at where to mint the key (Settings → API Keys), not the key format.
    setup_url: https://app.sendgrid.com/settings/api_keys

identity:
  source: strategy
  # No url/stable_key/label_candidates. `strategy` is the one source
  # validateIdentity exempts from the stable_key/label_candidates requirement,
  # and it forbids identity.url — so the verify endpoint lives in the compiled
  # sendgridScopesVerifier (§4), which owns its own /v3/scopes URL and branches
  # on status itself: 200 captures scopes + best-effort email label; 403 is a
  # valid-but-restricted success with a deterministic fingerprint account_key;
  # 401 is the only rejection.

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

# The pasted key is stored in the user token payload (design 317 D5: single
# secret through the existing UpsertUserToken write path) as token.access_token
# and projected back out under the anycli field name `api_key` — the same
# credential.fields shape the shipped mongodb bundle uses (it stores
# connection_string the same way). Zero new CredentialSource, zero token-gateway
# change. account_key is the verifier-derived fingerprint (§4).
credential:
  fields:
    api_key: token.access_token
    account_key: connection.account_key

tool:
  name: sendgrid
  kind: api-key   # wire-compat value (317 D2); clients route the drawer by auth_type
```

**Axis mapping.** ① CLI word `sendgrid` · ② anycli id `sendgrid` · ③ key
`sendgrid` — all identical → no `tool.command`, no `tool.group`, **no
`toolToProvider` entry**.

**Generation.** From `go-services/integration-service`: `go run ./cmd/provider-gen`
then `--check`. With `identity.source: strategy` the manifest **passes
`provider-gen` validation and generates locally** — `validateIdentity` exempts
`strategy` from the `stable_key`/`label_candidates` requirement, so the five
projections emit cleanly (this is the concrete fix for the earlier `userinfo`
shape, which failed generation outright). Those regenerated projections are
committed by the **batch lead at batch end** — per the master plan §2 and the
hard constraint, this agent runs provider-gen **locally for validation only** and
does **not** commit them. Only `provider-gen --check` (the committed-diff gate)
is expected red in CI until the batch-end merge; generation itself is green.

**Config.** `manual_credentials` needs **no** `required_config_fields` and no
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
| **L1** | anycli `go test ./...`: `sendgrid` service + definition unit tests against `httptest` fakes — asserts method/path/query, `Authorization: Bearer <key>` header, request bodies (personalizations/content assembly from `--to/--from/--subject/--text/--html`, template send with `dynamic_template_data`, contact upsert), the **`202`/empty-body/`X-Message-Id` mail-send success path**, `--region eu` host swap, SendGrid `{"errors":[…]}` error rendering (plain + `--json`), `401`→`RejectCredential` **and `403`→plain error (no credential rejection)**, exit codes 0/1/2. | No (fakes) |
| **L2** | Dev harness against the **real** SendGrid API. **First, the blocker gate:** run `scopes` with **both** a Full Access key **and** a `mail.send`-only Restricted key and **record the actual status codes** — the design predicts `200` (Full) and `403` (Restricted), and also that `/v3/user/email` `403`s on the Restricted key; the `sendgridScopesVerifier` branches (§4) are gated on this result and must be corrected here if SendGrid behaves differently. Then, with a Full Access key: `ANYCLI_CRED_API_KEY=<key> anycli sendgrid -- scopes` (expect `{"scopes":[…]}`), `sender list`, `mail send` to a real inbox (expect `202` + a real `X-Message-Id`), `template list`, `contact upsert` + `contact search`, `suppression bounces`, `stats`. Proves field names + Bearer injection + the `202`/empty-body handling + request shapes match the live API — **and** that a restricted key still connects. | **Yes** — a real SendGrid account with **two** keys (one Full Access, one `mail.send`-only Restricted) + a verified sender (test-account pool, master plan lane 2) |
| **L3** | `provider-gen --check` (run locally on-branch; expected red in CI until batch end) + both repos' unit suites: `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`, and `make test-integration-service` including the new `sendgridScopesVerifier` test with all three branches: **`200`** → verified + scopes captured (+ best-effort label); **`403`** → **valid-but-restricted success** (connection created, generic label, deterministic token-hash `account_key`, **not** rejected); **`401`** → `invalid_provider_credential`. The `403`-is-success assertion is the regression test for the connect-time blocker. Point `helio-cli/go.mod` at the anycli branch via a **local, uncommitted** `replace`. | No |
| **L4** | Singleton + seed a real key through the token gateway: `POST /internal/test-only/connections/seed` with `provider: sendgrid`, `access_token: <real key>`, a **real** seeded org/assistant identity → `201`; then `heliox tool sendgrid -- scopes` and `heliox tool sendgrid -- mail send …` reach the live API. Seeds `access_token` only (no refresh/expiry — non-expiring key). | **Yes** — real key + verified sender (same as L2) |
| **L5** | One full connect flow, still hidden, via the **credentials key-entry connect path** (master plan §2, human lane 3, agent-drivable): open the connect link → paste the key through the real connect UI → integration-service verifies it via `sendgridScopesVerifier` against `GET /v3/scopes` (`200`→scopes captured, `403`→valid-but-restricted, identity/label derived per §4) → connection shows connected/configured in `GET /connections` → one **unseeded** `heliox tool sendgrid -- scopes` (and a real `mail send`) succeeds through the real token gateway. **Run the connect once with the Full Access key and once with the `mail.send`-only Restricted key** — both must reach connected/configured (the restricted one via the `403` path); a genuinely bogus key must be rejected `invalid_provider_credential`. | **Yes** — real key(s) + verified sender |

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
   `mail.send`-only key `403`s on both `/v3/scopes` and every `/v3/user/*`
   endpoint (§2), so no human-readable account identity is available — the key is
   proven valid by the `403` itself, not by any readable field. Recommended (§4):
   best-effort `/v3/user/email` for a nice label when scopes allow (the `200`
   path), else generic label + **deterministic token-hash `account_key`** to
   preserve idempotent replace. The alternative — hard-requiring a user-scoped
   key (option (a): verify via `/v3/scopes` and reject anything that isn't `200`)
   — would reject exactly the least-privilege keys we want to encourage, so it's
   rejected in favor of the `403`-is-valid contract. This no-readable-id reality
   is exactly why the bundle sits on the `credentials` + `identity.source:
   strategy` lane (like `mongodb`) rather than the `api_key` + `userinfo` lane
   (like `bitly`/`notion`, which have a readable `stable_key`) — see §4/§5.
   Revisit only if the token-hash account key proves confusing in the UI.
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
   concrete teammate need appears — do not pre-build the tree (the thin-surface
   principle the parallel-program email siblings adopt).
4. **`contact upsert` async confirmation.** v1 surfaces the raw `job_id` and
   documents the `contact search`-to-confirm pattern rather than auto-polling
   the import-status endpoint. A `contact upsert --wait` that polls
   `GET /v3/marketing/contacts/imports/{job_id}` is a possible ergonomic
   follow-up; kept out of v1 to keep the surface thin and non-blocking.
