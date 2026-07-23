# Tool design: Mailjet

Scratch design for the `mailjet` external tool provider (catalog row 273).
Batch-lead strips this file at batch end.

- **anycli id (axis ②):** `mailjet`
- **provider catalog key (axis ③):** `mailjet`
- **CLI command word (axis ①):** `mailjet`
- **Auth lane:** `api_key` — catalog row 273, confirmed by the 2026-07-21 OAuth
  audit (row 275: "no viable multi-tenant path → api_key. Stays api_key per
  rubric").
- **Wave / category:** Wave 3, Marketing & Notifications.
- **Tool form:** `service` type.

All three naming axes are identical (`mailjet`), so **no `toolToProvider`
entry is required** — `ProviderFor("mailjet")` falls through to identity.

## 1. Auth-lane confirmation vs catalog / audit

No lane divergence. I re-verified Mailjet's auth model against the official
API docs and it confirms the audit verdict:

- Mailjet authenticates every Email API request with **HTTP Basic auth**,
  API Key (public) as username and Secret Key (private) as password —
  `base64(API_KEY:SECRET_KEY)`. There is no multi-tenant authorization-code
  OAuth flow for third-party apps; credentials are per-account keys minted in
  Account Settings → API Keys. `api_key` lane is correct.
- Source (official): the Mailjet developer guides and the
  `mailjet/api-documentation` repo (`guides/_api-overview.md`) both state
  Basic auth with the key/secret pair and the `--user "$MJ_APIKEY_PUBLIC:$MJ_APIKEY_PRIVATE"`
  cURL shape.

**API host (default EU + documented US-architecture host).** This is a
factual API detail, not an auth-lane change. Mailjet processes data in the
**EU** (single data-residency region — GDPR / ISO27001 is a selling point),
but the API is reachable on **two documented base hosts**: the default
`https://api.mailjet.com`, and `https://api.us.mailjet.com` for accounts that
Mailjet has provisioned on / migrated to its **US architecture**. The credential
(key:secret) carries no host marker, so host selection is an anycli-side flag,
not a credential field (see §3). Primary sources (official Mailjet wrappers,
each documenting the exact US host toggle): the .NET wrapper README —
"If your account has been moved to Mailjet's US architecture, the URL value you
need to set is `https://api.us.mailjet.com`" (`BaseAdress = "https://api.us.mailjet.com"`);
the PHP wrapper README — same sentence with `['url' => "api.us.mailjet.com"]`;
mirrored in the Ruby / Python / Node wrappers' base-URL config.

### Review-finding overrides (verified against official docs)

Two review findings asserted these facts were fabricated. I re-verified each
against Mailjet's **primary-source** material and both findings are factually
incorrect, so per the "follow official docs over the prompt/catalog/audit and
record the divergence" instruction I am **keeping** the original behavior with
citations rather than removing it:

- **"`api.us.mailjet.com` does not exist; Mailjet is EU-only."** *Overridden.*
  The reviewer conflated EU **data residency** (true) with the **API host**
  (a real, documented US host exists). The official Mailjet .NET and PHP
  wrapper READMEs both document `https://api.us.mailjet.com` verbatim for
  US-architecture accounts (quotes above). The `--region us` / `--base-url`
  flag therefore routes to a **real, officially documented** DNS host, not a
  fabricated one, and the L1 host-switch assertion tests real behavior.
- **"`/v3/REST/statistics/recipient-esp` is unconfirmable / wrong shape."**
  *Overridden.* Mailjet's own `api-documentation` repo
  (`guides/_statistics.md`) shows the exact endpoint:
  `GET https://api.mailjet.com/v3/REST/statistics/recipient-esp?CampaignId=$Campaign_ID`
  (per-mailbox-provider deliverability for a campaign). It follows the standard
  `/v3/REST/statistics/<resource>` sub-resource shape (siblings:
  `statistics/link-click`). Kept as-is; see §2 for the confirmed path + param.

## 2. API surface wrapped, and why

Driven by what an AI teammate actually does with Mailjet — a transactional +
marketing email platform. The highest-value, agent-natural verbs, scoped to a
notion-style resource-grouped cobra tree:

| Subcommand (group) | Mailjet endpoint(s) | Why an AI teammate needs it |
|---|---|---|
| `send` | `POST /v3.1/send` (Send API v3.1) | The headline capability: send a transactional email (to/from/subject/text/html, optional `TemplateID` + `Variables`). v3.1 `Messages[]` is the current send contract. |
| `contact list` / `contact get` / `contact create` | `GET/POST /v3/REST/contact`, `GET /v3/REST/contact/{id}` | Look up and create the people the assistant emails. |
| `list list` / `list create` / `list add-contact` | `GET/POST /v3/REST/contactslist`, `POST /v3/REST/listrecipient` | Manage contact lists and (un)subscribe a contact — the audience side of marketing sends. |
| `template list` / `template get` | `GET /v3/REST/template`, `GET /v3/REST/template/{id}/detailcontent` | Discover and inspect reusable email templates to send by `TemplateID`. |
| `message list` / `message get` | `GET /v3/REST/message`, `GET /v3/REST/messagehistory` | Read what was sent and its delivery/engagement events — the "did it arrive / what happened" question. |
| `stat` | `GET /v3/REST/statcounters`, `GET /v3/REST/statistics/recipient-esp?CampaignId=<id>` | Campaign/account delivery + open/click counters, plus per-mailbox-provider deliverability for a campaign. Both paths confirmed verbatim in Mailjet's `api-documentation` repo (`guides/_statistics.md`); `recipient-esp` requires `CampaignId`. |

Deliberately **out of scope for v1** (kept thin, agent-natural — Code Health
"subtract before adding"): campaign-draft authoring/scheduling
(`/v3/REST/campaigndraft` multi-step workflow), sender/domain DNS management,
segmentation formula authoring (`/contactfilter`), Parse API inbound, and the
v4 SMS API. These are heavy human-console workflows, not assistant verbs; add
later only if usage demands.

**Base URL:** `https://api.mailjet.com` default; v3 REST at
`/v3/REST/{resource}`, Send API at `/v3.1/send`. A `--base-url` override (with
`--region us` as documented sugar) switches the host to the
officially-documented `https://api.us.mailjet.com` for accounts on Mailjet's
US architecture (§1 sources). The default EU host is correct for the large
majority of accounts; the credential is host-agnostic so the same key:secret
works against whichever host the account lives on.

## 3. anycli definition

**Type: `service`.** No official Mailjet CLI binary that is non-interactive,
`--json`-capable, and provisionable into the runtime image exists (the
official SDKs are language libraries, not a CLI). Fails the stage-1 `cli`
rubric → implement `service` type against the HTTP API, per 21/23 existing
definitions.

`definitions/tools/mailjet.json`:

```json
{
  "name": "mailjet",
  "type": "service",
  "description": "Mailjet email as a tool: send transactional email, manage contacts/lists/templates, read message stats",
  "auth": {
    "credentials": [
      {
        "source": { "field": "access_token" },
        "inject": { "type": "env", "env_var": "MAILJET_BASIC_AUTH" }
      }
    ]
  }
}
```

**Single injected credential, `MAILJET_BASIC_AUTH`, whose value is the exact
Basic-auth userinfo string `<api_key>:<secret_key>`.** This is the crux of the
design and it exploits a clean coincidence: Mailjet's Basic-auth userinfo *is*
literally `apikey:secretkey`, so one stored secret carries both values with no
second credential field and no lossy encoding. The service builds
`Authorization: Basic base64(MAILJET_BASIC_AUTH)` for every request (DataForSEO
`login:password` Basic precedent).

Service package: `internal/tools/mailjet/` (Go package `mailjet` — id has no
dashes, so package name == id), registered `RegisterService("mailjet", &mailjet.Service{})`
in `internal/tools/register.go`. Copy the `internal/tools/notion/` shape:

- `Service` struct with `BaseURL` / `HC` / `Out` / `Err` seams so unit tests
  point at an `httptest.Server` and capture output.
- Cobra tree grouped by resource (above), `--json` structured output by
  default. Mailjet REST returns `{"Count":N,"Data":[…],"Total":N}`; the
  service unwraps `Data` into clean provider-neutral JSON (array for list,
  object for get) rather than leaking the envelope.
- Exit-code contract identical to notion: `0` success, `1` runtime/API failure
  (typed `apiError`, with `--json` error envelope), `2` usage/parse error.
- `--region us` / `--base-url` flag resolving the host (§2).
- Basic-auth header built in one place from `MAILJET_BASIC_AUTH`.

**JSON output shape (example, `contact list --limit 2`):**

```json
{
  "contacts": [
    {"id": 132, "email": "a@example.com", "name": "A", "is_excluded_from_campaigns": false},
    {"id": 145, "email": "b@example.com", "name": "B", "is_excluded_from_campaigns": false}
  ],
  "count": 2,
  "total": 87
}
```

`send` returns the normalized v3.1 result (per-message status + `MessageID` /
`MessageUUID`). Field names are lower_snake normalized from Mailjet's
PascalCase to match the neutral-JSON convention other definitions use.

## 4. Credential fields & auth flow

**Credential fields:** one — the API Key (public) and Secret Key (private)
pasted as a single value in the form `<api_key>:<secret_key>`. Semantics: HTTP
Basic userinfo. Registration model: the user creates/reads the pair in Mailjet
**Account Settings → API Keys** (the Secret Key is shown once at creation).
No app registration, no scopes, no token expiry — a static key pair. No refresh
cycle.

**Why one field, not two:** the main-branch manual write path
(`integration-service/service/manual_credential.go` → `resolveManualSecret`)
is **single-secret only** — it hard-fails
(`"credential schema is not single-secret"`) when `credential_input.fields`
!= 1, and the D5 generation-time check enforces exactly one required field.
Storage projects that one secret through the existing
`token.access_token` `CredentialSource`. Two separate credential fields are
**not supported on main**, so packing the pair into one field (which is
exactly Basic userinfo) is both the storage-compatible fit and zero new
`CredentialSource` / token-gateway change. This mirrors the DataForSEO
(`login:password`) and Crisp keypair precedents.

**Auth flow (connect → run):**

1. User opens the connect drawer (auth_type `api_key`), pastes
   `<api_key>:<secret_key>`.
2. integration-service **verifies** it: `GET https://api.mailjet.com/v3/REST/apikey`
   with `Authorization: Basic base64(secret)`. `200` = valid pair; `401/403` =
   rejected → `invalid_provider_credential`. This is a real connect-time check
   (Mailjet, unlike mongodb, *has* an HTTPS identity endpoint), so
   `manual_api_token` + a Basic-scheme verifier is the right pairing rather
   than mongodb's no-verify `manual_credentials`.
3. **account_key / label:** the public API Key (the substring left of the
   first colon) — globally unique, stable, human-readable, and **not secret**
   (only the Secret Key is confidential). Deriving from the credential itself
   avoids depending on uncertain response field names, while the GET still
   proves validity. (Fallback if a colon-split deriver is unavailable: extract
   `/Data/0/APIKey` from the verify response as stable_key.)
4. Secret stored in Vault via the standard `writeUserTokenCredential` path
   (`credentials:`-prefixed name is fine; single access-token payload).
5. At runtime the token gateway serves the stored `token.access_token`, the
   resolver injects it as `MAILJET_BASIC_AUTH`, anycli execs the real HTTP
   call.

**integration-service capability — reuse first, grow only if absent.** Main
today ships only `declarativeManualTokenVerifier` (sets one header to the raw
token — cannot base64-encode) and `dsnHostIdentityDeriver` (no-verify). Neither
does Basic-userinfo base64. But the lemlist (`basic_password verifier`, task
#256), DataForSEO (#127), Tally (Bearer-scheme selector, #294) and Crisp
(keypair, #218) work already introduced a **manual-token verifier scheme
selector** for exactly this class. Mailjet should **reuse the existing
Basic-scheme manual-token verifier** (store the userinfo verbatim, the verifier
base64-encodes it into `Authorization: Basic …` and GETs the identity URL). If
that Basic-scheme selector has not yet landed on main when Mailjet's branch
starts, add a minimal `basicAuthManualTokenVerifier` mirroring
`declarativeManualTokenVerifier` but emitting `Authorization: Basic base64(secret)`,
selected by an `api_key.scheme: basic` bundle field. This is the same-knife
capability growth the batch already budgets for Basic-auth api_key tools — flag
it at stage 1, not mid-wave.

## 5. Helio provider bundle plan

`integrations/providers/mailjet/provider.yaml`, **hidden-first**
(`presentation.visible: false`). Axis naming per master-plan §3: key == id ==
command == `mailjet`; independent brand, no corporate-family prefix.

```yaml
schema: helio.provider/v1
key: mailjet
go_name: Mailjet

presentation:
  name: Mailjet
  description_key: mailjet
  consent_domain: mailjet.com
  visible: false   # flip only after L1–L5 green + docs + icon + i18n

auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization        # Basic scheme (see §4 capability note)
    scheme: basic                # verifier base64-encodes the stored userinfo
  credential_input:
    fields:
      - name: api_credentials
        label_key: mailjet_api_credentials
        secret: true
        placeholder: "your-api-key:your-secret-key"
        required: true
    setup_url: https://app.mailjet.com/account/apikeys

identity:
  source: strategy               # account_key = public API key (left of ':')
  url: https://api.mailjet.com/v3/REST/apikey   # verify target (200 = valid)

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: mailjet
  kind: api-key
```

Exact field spellings (`api_key.scheme`, `identity.source: strategy` + a
verify `url`) follow whichever shape the Basic-scheme precedent (lemlist/
dataforseo) already committed to main — the bundle above is the intent; align
keys to the merged capability at implementation time so `provider-gen
--check` passes.

**No service adapter** beyond the shared Basic verifier — Mailjet is a plain
static-key provider, `disconnect_mode: local_only` (Mailjet has no
key-revocation API; disconnect just drops the stored credential). **No OAuth
config** in `config/` or `deploy/` (api_key lane — the user supplies the key;
human lane 1's OAuth-app registration does not apply). A bad key surfaces at
connect via the verify GET, not silently at first use.

Companion artifacts (ride the batch-end merge):
- UI icon `ui/helio-app/src/integrations/icons/mailjet.svg` + register in
  `providerIcons.ts` (Mailjet brand mark).
- i18n: `mailjet` description + `mailjet_api_credentials` label across all
  supported locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/mailjet` (send /
  contact / list / template / message / stat verbs, `region` note, the
  `apikey:secretkey` credential shape), plugin version bump + marketplace
  publish.

## 6. Test plan (five layers)

| Layer | For Mailjet | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: httptest fake for `api.mailjet.com`; assert `Authorization: Basic base64(key:secret)` header, request shapes for send/contact/list/template/message/stat, `Data`-envelope unwrapping, plain-text + `--json` error rendering, exit codes 0/1/2, `--region us` host switch. **No real API.** | No |
| **L2** harness real-API | `MAILJET_BASIC_AUTH="<key>:<secret>" anycli mailjet -- contact list --limit 1` and a `send` to a seed inbox against the live API. Proves field names, Basic header, region, and `Data` shape match reality. | **Yes** — real Mailjet API key pair (free tier, from the account pool). |
| **L3** generation + suites | `provider-gen` + `provider-gen --check`; helio-cli + integration-service unit suites (incl. the Basic-verifier test if capability grown). Run locally on-branch with a `go.mod` replace to the anycli branch; do **not** commit projections. | No |
| **L4** singleton + seed | Seed `POST /internal/test-only/connections/seed` with `provider=mailjet`, `access_token="<key>:<secret>"` (api_key provider → seedable; non-expiring, seed access_token only, no refresh). Then `heliox tool mailjet -- contact list` through the real token gateway. | **Yes** — a real key pair for the seeded token to reach the live API. |
| **L5** connect flow | api_key key-entry path (master-plan §2): open connect link → paste `key:secret` → verify GET returns 200 → connection shows connected/configured (`GET /connections`) → one **unseeded** live `heliox tool mailjet -- …` succeeds. Agent-drivable (agent-browser), human fallback. Run once, hidden, before the visible flip. | **Yes** — real key pair; agent-driven UI. |

Credentialed layers (L2, L4, L5) depend on human lane 2 (test-account pool)
providing one real Mailjet key pair — Mailjet offers a free tier, so
procurement is low-risk. No lane-1 OAuth app registration is needed (api_key).

## 7. Rollout

Ship hidden. Land: anycli tool (L1+L2 green) → pin bump → bundle + Basic-verifier
capability (if needed) + generation (L3) → seed L4 → key-entry L5 → then flip
`presentation.visible: true` + regenerate as the single go-live change. Icon +
i18n + AI-doc must be published before the flip (an assistant must never see a
visible tool with no docs).
