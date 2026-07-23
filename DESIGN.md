# DESIGN — ZoomInfo (`zoominfo`)

Scratch design for the ZoomInfo external tool provider, per
`.claude/skills/helio-tool-provider/SKILL.md` and the master plan
`docs/design/008-300-integrations-rollout-plan.md`. Batch lead strips this file
at batch-end.

- **Catalog row (master plan §4):** row 66 — Product ZoomInfo · anycli id
  `zoominfo` · provider key `zoominfo` · **auth lane `api_key`** · wave
  **3-hold** · category Sales Engagement.
- **OAuth audit verdict** (`oauth-audit.md`, row 68 in pre-renumber numbering):
  *"no viable multi-tenant path → api_key. Stays api_key per rubric."* Confirmed
  below against official docs: ZoomInfo has **no OAuth 2.0 at all** (neither
  authorization-code nor client-credentials), so the api_key lane is correct.
- **3-hold reason (§5 / §6 risk log):** account procurement — ZoomInfo has no
  self-serve or free API tier; API access requires a paid ZoomInfo seat with the
  Enterprise API entitlement and Admin-Portal-issued credentials. This gates
  L2/L4/L5 (real-credential layers), not dev. The 3-hold pre-verify gate must
  clear both the account-procurement bullet **and** the auth-shape bullet (this
  doc settles the auth shape).

## 1. Divergence from the naive "api_key" reading — verified auth model

**This is the load-bearing finding.** The catalog's `api_key` lane is correct as
a *scheduling/registration* classification (no OAuth authorize flow, credential
pasted from the account pool), **but ZoomInfo's credential is not a static
bearer key that anycli can inject unchanged.** ZoomInfo authenticates with a
**proprietary JWT exchange**: the stored long-lived credential is exchanged, at
runtime, for a **short-lived (~60 min) access JWT** that is what actually
authorizes data calls. Verified against ZoomInfo's official Python auth client
(`github.com/Zoominfo/api-auth-python-client`,
`zi_api_auth_client/zi_api_auth_client.py`) and the Enterprise API Getting
Started guide.

Two authentication methods exist; **both terminate at
`POST https://api.zoominfo.com/authenticate` and both return a JSON body with a
`jwt` field** used as `Authorization: Bearer <jwt>` on all subsequent data
calls. Access-token lifetime is **60 minutes with no refresh-token mechanism** —
you re-authenticate.

### (a) PKI authentication — RECOMMENDED, the shipped method

Two-step, exactly as the official client's `pki_authentication` /
`_post_and_get_jwt` implements:

1. Build a **client-assertion JWT locally**, signed **RS256** with the user's
   RSA private key. Claims (verbatim from the official client):
   - `aud`: `enterprise_api`
   - `iss`: `api-client@zoominfo.com`
   - `iat`: now
   - `exp`: now + 300s (5 min)
   - `client_id`: `<client_id>`
   - `username`: `<username>`
2. `POST https://api.zoominfo.com/authenticate` with header
   `Authorization: Bearer <client-assertion JWT>` and **no body** → response
   JSON `{"jwt": "<access JWT>"}` (valid 60 min).

Stored credential fields: **`username`, `client_id`, `private_key`** (PEM RSA
private key, multi-line). No human login password is ever stored. This is
ZoomInfo's documented method for programmatic/service integrations and is the
correct shape for an AI teammate connecting on a customer's behalf.

### (b) Username/password authentication — documented fallback, NOT shipped

`POST https://api.zoominfo.com/authenticate` with body `{username, password}` →
`{"jwt": ...}`. Rejected as the primary because it stores the user's actual
ZoomInfo login password (worse blast radius, worse for shared/service use) and
carries no advantage over PKI. Documented here so the auth-shape decision is on
the record; if a customer cannot mint a PKI key, this is the escape hatch and
would add a second `credential_input` variant later — not in scope for the first
landing.

### Consequence for anycli

The `zoominfo` service package performs the `/authenticate` exchange
**in-process on each invocation**, because the access JWT is short-lived and
cannot be a stored static credential. anycli injects the three long-lived PKI
fields; the service mints the client-assertion JWT, calls `/authenticate`, then
calls the requested data endpoint with the returned access JWT. Cost is **two
HTTP requests per `heliox tool zoominfo` invocation** (one auth + one data);
ZoomInfo caps the auth endpoint at **1 req/sec** and standard APIs at 25 req/sec
— fine for interactive AI use. (anycli invocations are one-shot processes, so
cross-invocation JWT caching via the anycli `Cache` seam is out of scope for v1;
the per-call re-auth stays within the 1 rps auth cap.)

**New anycli dependency:** RS256 signing needs a JWT/crypto library
(`github.com/golang-jwt/jwt/v5`, or hand-rolled `crypto/rsa` + `crypto/x509` PEM
parse + base64url). This is the first anycli tool that signs a JWT; flag it at
stage-1 as a new module dependency for review.

## 2. Which official API surface & endpoints, and why

Base URL **`https://api.zoominfo.com`**. ZoomInfo is migrating the Legacy
Enterprise API (`api-docs.zoominfo.com`) to a New API (`docs.zoominfo.com`) with
the **same JWT auth model** and the same core Search/Enrich/Lookup/Usage
surface; the concrete request/response field shapes MUST be re-confirmed against
`docs.zoominfo.com/reference` at stage-1/stage-2 (Legacy is deprecating — build
against current paths). The endpoints below are the stable, documented surface.

Driven by **what an AI teammate actually does with ZoomInfo** — B2B GTM /
sales-intelligence: find and enrich people and companies. ZoomInfo's own
recommended workflow is a **two-stage Search → Enrich** pattern (Search finds
candidate record IDs and consumes **no credit**; Enrich pulls full profiles and
**consumes a credit per record**).

| Verb (anycli) | Method + path | Why an AI teammate needs it |
|---|---|---|
| `contact search` | `POST /search/contact` | Find prospect candidates by title/company/location; returns record IDs + hints, **no credit**. Default discovery step. |
| `contact enrich` | `POST /enrich/contact` | Full contact profile (email/phone/etc.) for up to 25 IDs (or match keys); **consumes credits**. The core enrichment action. |
| `company search` | `POST /search/company` | Find company candidates by name/domain/industry; returns IDs, **no credit**. |
| `company enrich` | `POST /enrich/company` | Full firmographics for up to 25 company IDs; **consumes credits**. |
| `lookup` | `GET /lookup/...` (input/output field discovery) | Enumerate valid Search filters and Enrich `outputFields` so the AI builds valid requests instead of guessing. Read-only, no credit. |
| `usage` | `GET /usage` | Report remaining credits / request limits against the monthly allotment. Lets the AI check cost before enriching. No credit. |

**Credit-cost design consideration (product nuance, note for stage-2):** Enrich
is real money (one credit per newly enriched record, free re-enrichment of the
same record for 12 months). The AI-facing surface should make **Search the
default discovery path** and **Enrich an explicit, ID-driven action**, and the
`--json` envelope for enrich should surface credit consumption from the response
so the assistant can report cost. Not a hard-block/`--confirm` gate in v1, but
the docs must state clearly that enrich spends credits.

Out of scope for v1 (mention as follow-ups, not built): Intent/Scoops/WebSights,
Bulk endpoints, Compliance, and the separate User-Management CRUD API — none are
needed for the core "find + enrich" teammate use, and Bulk adds an async
job-polling shape.

## 3. anycli definition

- **Tool form (stage-1 rubric): `service` type.** No official ZoomInfo CLI
  binary exists; auth requires custom in-process JWT minting + exchange against
  the HTTP API. Fails every `cli`-type criterion → `service`, implemented in
  `internal/tools/zoominfo/` against `https://api.zoominfo.com`. (21/23 current
  definitions are service type.)
- **Definition** `definitions/tools/zoominfo.json` (axis ②
  `name: "zoominfo"`), `type: "service"`, one-line description. **Auth block —
  three-field injection** (mongodb's single-field injection is the shape
  precedent; here there are three env injects):
  - `source.field: username`  → `inject env ZOOMINFO_USERNAME`
  - `source.field: client_id` → `inject env ZOOMINFO_CLIENT_ID`
  - `source.field: private_key` → `inject env ZOOMINFO_PRIVATE_KEY`
  (`private_key` is a multi-line PEM; env injection carries newlines fine. If a
  future review prefers a file inject for the key, anycli's `inject type: file`
  path is available — env is simpler and sufficient.)
- **Service package** `internal/tools/zoominfo/` registered as
  `RegisterService("zoominfo", &zoominfo.Service{})` in
  `internal/tools/register.go`. Copy `internal/tools/notion/`'s shape: cobra tree
  grouped by resource (`contact`, `company`) + top-level `lookup`/`usage`, a
  `BaseURL`/`HC`/`Out`/`Err` struct so tests point `HC`/`BaseURL` at an
  `httptest.Server`, and the documented exit-code contract (0 success, 1
  runtime/API failure via typed `apiError`, 2 usage/parse) with a `--json`
  structured error envelope. An internal `authenticate()` helper mints the
  client-assertion JWT and performs the exchange; every command calls it first.
- **Go package name (stage-2 naming):** `zoominfo` (id has no dash/leading
  digit) → `internal/tools/zoominfo/`.
- **JSON output shape:** `--json` on every subcommand; provider-neutral envelope
  echoing ZoomInfo response data (search → `{ "data": [...], "maxResults": N }`;
  enrich → `{ "data": [...], "creditsConsumed": N }` mapped from the response;
  errors → the notion-style `{ "error": { "code", "message" } }`). Non-`--json`
  mode prints human-readable text. Never hit the real API from unit tests.

## 4. Credential fields & exact auth flow

**Stored credential (PKI):** `username`, `client_id`, `private_key`. All entered
by the user through the connect UI (§5); none are ever in the bundle. Runtime
per invocation:

```
inject username, client_id, private_key (env)
  → build client-assertion JWT: RS256(private_key), claims
      {aud:"enterprise_api", iss:"api-client@zoominfo.com",
       iat:now, exp:now+300s, client_id, username}
  → POST https://api.zoominfo.com/authenticate
       Authorization: Bearer <client-assertion JWT>   (no body)
  → parse {"jwt": <access JWT>}   (60-min lifetime)
  → call data endpoint (e.g. POST /enrich/contact) with
       Authorization: Bearer <access JWT>
```

**Registration model:** an admin generates the PKI key pair (client ID + private
key) in the **ZoomInfo Admin Portal**; there is no developer console, no OAuth
app, no redirect URI, no scopes, and no partner review (the review-lane
machinery of lane 1 does **not** apply). "Scopes" are governed by the account's
ZoomInfo entitlements, not by the credential.

**Token semantics:** access JWT = 60 min, no refresh token → re-mint each call.
Client-assertion JWT = 5 min (only used for the immediate exchange). 401 = bad or
expired JWT.

## 5. Helio provider bundle plan (hidden-first)

**Naming (all three axes identical → zero divergence):**

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `zoominfo` (flat, not grouped) | bundle `tool.command`/`tool.name` |
| ② anycli tool id | `zoominfo` | `definitions/tools/zoominfo.json` |
| ③ provider catalog key | `zoominfo` | bundle dir `integrations/providers/zoominfo/` |

No `toolToProvider` entry (id == key; mechanical, no dash). No grouped family.

**`integrations/providers/zoominfo/provider.yaml`** — modeled on `mongodb`
(the manual-credentials / `type: credentials` precedent), extended to
**multi-field**:

- `presentation.visible: false` (hidden-first; flip is the single go-live change
  after L5).
- `auth.type: credentials`, `owner: individual`.
- `auth.credential_input.fields`: three fields — `username` (label, not secret,
  the ZoomInfo login/email), `client_id` (secret), `private_key`
  (secret, multi-line PEM, placeholder `-----BEGIN PRIVATE KEY-----…`).
  `setup_url` → ZoomInfo Admin Portal API-key page.
- `connection.mode: isolated`, `disconnect_mode: local_only`,
  `runtime_strategy: manual_credentials`.
- `identity.source: strategy` — **account_key = `username`** (human-readable,
  never a hash), derived by a first-field / named-field strategy deriver
  (mongodb uses DSN host; the several multi-field manual tools use a named
  field). No cheap HTTPS userinfo endpoint returns identity, so identity is
  derived from the entered `username`, not fetched.
- `resources.selection/discovery/enforcement: none`.
- **Credential projection — capability to confirm/grow:** mongodb stores a
  *single* secret in `token.access_token`. ZoomInfo stores **three** fields, so
  the bundle needs the **multi-field `manual_credentials`** projection (all
  three fields packed into the credential payload and delivered to anycli as the
  credential map). Confirm the multi-field manual-credentials capability exists
  on integration-service `main` (the mixpanel / braze / servicenow multi-field
  manual tools established it); if this base predates it, grow it exactly as
  those did — no ZoomInfo-specific adapter. **No token-gateway change** and **no
  `service/adapter_*.go`**: the JWT exchange lives entirely in the anycli service,
  so from Helio's side ZoomInfo is a plain stored-credentials provider.
- **Optional connect-time verifier (recommended):** unlike mongodb's no-verify
  DSN, ZoomInfo has a clean verification endpoint — `POST /authenticate`. A
  `manual_credentials` verifier that mints the client-assertion JWT and calls
  `/authenticate`, rejecting on non-200, gives immediate "bad key" feedback at
  connect time instead of stale first-use failure. This mirrors the
  moz/semrush/loops verifier-capability precedents. If the verifier capability
  cannot be reused cleanly, fall back to no-verify (mongodb path) and accept
  stale feedback — not a blocker for hidden landing.
- `experiment:` empty (GA path; no design-090 gate needed).

**Config (Config Sync):** none. There is no client id/secret to place in
`config/` + `deploy/` — the credential is 100% user-supplied. This provider is
`configured: true` with no environment config, so it is safe to ship hidden and
flip without any integration-service Secret append. (Lane 1 does not touch this
tool.)

**UI icon:** `ui/helio-app/src/integrations/icons/zoominfo.svg` + register in
`providerIcons.ts` (manual). Add `tools.desc.zoominfo` i18n label across locales.

**AI-facing docs:** provider sub-doc under
`agents/plugins/heliox/skills/tool/`, bumped + published on the batch-end merge.
Must state the Search-then-Enrich workflow and that **enrich consumes credits**.

## 6. Test plan — five layers (external-credential dependency marked)

| Layer | What it proves for ZoomInfo | External creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fake for `/authenticate` (returns `{"jwt":…}`) + `/search/*` + `/enrich/*`. Assert: client-assertion JWT is **valid RS256** (verify with a test public key) and carries the exact claims; `Authorization: Bearer <access jwt>` on data calls; request bodies (IDs, `outputFields`); `--json` and plain error rendering; exit codes 0/1/2. | **No** — self-signed test RSA key pair, fake server. |
| **L2** dev harness real API (`anycli zoominfo -- contact enrich …`, creds via `ANYCLI_CRED_USERNAME` / `ANYCLI_CRED_CLIENT_ID` / `ANYCLI_CRED_PRIVATE_KEY`) | Field names, JWT signing, and request shapes actually match the live ZoomInfo API and return real data. **Mandatory before pin bump.** | **YES** — paid ZoomInfo seat + Admin-Portal PKI key. **This is the 3-hold account-procurement gate.** |
| **L3** `provider-gen --check` + both repos' unit suites | Bundle projects cleanly (five files), closed field/enum contract, id==key, HTTPS setup URL; helio-cli builds against the `replace`-pinned anycli branch. | **No.** |
| **L4** singleton + `POST /internal/test-only/connections/seed` + `heliox tool zoominfo -- …` | A seeded multi-field manual credential (username/client_id/private_key) reaches the live API through the real token gateway. Confirm the seed path accepts the multi-field manual payload (api_key/manual providers are seedable; servicenow's endpoint+secret multi-field seed is the precedent). Seed a **real** PKI key so the live call succeeds. | **YES** — same real PKI key as L2; success = live data, not a mock. |
| **L5** api_key connect-UI path (pre-flip, once) | Open connect link → enter **username + client_id + private_key** through the real connect drawer (stored via `POST /connections/credentials`, optionally verified via `/authenticate`) → connection shows connected/`configured` in `GET /connections` → one **unseeded** live `heliox tool zoominfo` command succeeds. api_key L5 is agent-drivable (human fallback on UI breakage); **verify the connect drawer supports a multi-line PEM secret field.** | **YES** — real PKI key; the master-plan §2 key-entry L5 checklist. |

**Layers needing externally supplied credentials: L2, L4, L5** — all bottlenecked
on the single 3-hold procurement item (one paid ZoomInfo Enterprise-API seat
with a PKI key). No OAuth app, no review clock. L1 and L3 are fully
agent-runnable with no ZoomInfo account.

## 7. Open items to settle at stage-1/stage-2

1. Re-confirm exact New-API paths and request/response field names against
   `docs.zoominfo.com/reference` (Legacy `api-docs.zoominfo.com` is deprecating;
   auth model is unchanged either way).
2. New anycli dependency for RS256 JWT signing (`golang-jwt/jwt/v5` vs hand-rolled
   `crypto/rsa`) — pick and flag for review.
3. Confirm the multi-field `manual_credentials` projection + (optional) verifier
   capability exist on integration-service `main`; grow per mixpanel/servicenow
   precedent if the base predates them.
4. Multi-line PEM entry in the connect UI secret field — verify before the L5
   sweep.
