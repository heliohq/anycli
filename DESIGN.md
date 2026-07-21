# Tool design: Delighted (`delighted`)

Scratch design doc for the `tool/delighted` branch (batch-lead strips at batch
end). Drives the anycli `delighted` service definition and the Helio
`delighted` provider bundle. Written from the pipeline skill
(`helio-tool-provider`), master plan `008-300` (§2 execution model, §3 naming),
and the Delighted official REST API — verified against the live docs, not
assumed from the catalog.

Catalog row 213: Product **Delighted** · anycli id `delighted` · provider key
`delighted` · auth `api_key` · wave 3 · category Forms & Surveys.

## 1. Audit reconciliation — `api_key` confirmed against official docs

Master-plan lane: `api_key`. OAuth audit row 215 verdict: **"no viable
multi-tenant path → stays api_key"** (compact note, no evidence URL).

Independent verification against the official docs **confirms** the verdict:

- Delighted's REST API authenticates via **HTTP Basic Auth with the API key as
  the username and a blank password**, over HTTPS only. There is **no OAuth 2.0
  authorization-code flow** of any kind — no authorize/token endpoints, no app
  registration, no consent screen. (Source: official API auth section —
  "authenticated via HTTP Basic Auth. Use your API key as the username and
  leave the password blank.")
- The key is **per CX project**: "Each CX project has its own API key." A user
  copies it from the Delighted dashboard (Settings → API). Keys are long-lived
  (no documented expiry); rotation is manual regeneration in the dashboard.

So there is nothing to move; `api_key` is correct. No divergence to record.
**One nuance the catalog's flat `api_key` label hides** and this design must
handle: Delighted's credential is delivered as HTTP **Basic** (key-as-username),
**not** as a plain header token (`X-Api-Key: <key>` / `Authorization: Bearer
<key>`). This does not change the lane, but it changes the Helio-side auth model
(§6) — the existing header-token `manual_api_token` verifier cannot encode
Basic-username auth, and there is no identity endpoint to extract an account
from.

## 2. What an AI teammate does with Delighted → surface selection

Delighted is an NPS/CSAT/CES customer-experience survey platform. An AI teammate
acting as a CX analyst / ops helper does, in rough order of frequency:

1. **Read the score.** Pull current NPS/CSAT/CES aggregates and trend
   (`metrics`).
2. **Read & triage verbatim feedback.** List survey responses, filter by score
   band / time window, retrieve a single response with its comment, person, and
   tags (`survey_responses` list + retrieve).
3. **Annotate feedback.** Add tags / notes to a response during triage
   (`survey_responses` update).
4. **Enroll customers to be surveyed.** Create/update a person and schedule a
   survey, or add them to Autopilot recurring surveys (`people`,
   `autopilot memberships`).
5. **Manage deliverability & compliance.** List bounced / unsubscribed people,
   unsubscribe on request, cancel pending sends, GDPR-delete a person
   (`bounces`, `unsubscribes`, `people` delete, pending survey-request delete).

Reads dominate; writes are enrollment + compliance. This selects the endpoint
set in §3 and keeps write verbs conservative (create/update/delete of people,
tags, and subscription state — no bulk/destructive account operations beyond
what the API itself scopes to a single project key).

## 3. Official API reference (verified)

- **Base URL:** `https://api.delighted.com/v1`
- **Transport:** HTTPS only; JSON request/response (`.json` suffix on every
  path).
- **Auth:** HTTP Basic — `Authorization: Basic base64(<api_key> + ":")` (key =
  username, empty password).
- **Rate limits:** `429 Too Many Requests` with a `Retry-After` header;
  exponential backoff recommended. Each failed request is safely retriable.
- **Identity endpoint:** **none.** There is no `/me`, `/account`, or `/project`
  object. `metrics.json` returns only aggregate numbers (nps, promoter_count,
  …) with no account identifier or name. This is load-bearing for §6.

Endpoints the tool wraps (all under `/v1`, all `.json`):

| Group | Method & path | Purpose |
|---|---|---|
| metrics | `GET /metrics.json` | NPS/CSAT/CES aggregates (`since`, `until`, `trend`) |
| survey responses | `GET /survey_responses.json` | list (`per_page`, `page`, `since`, `until`, `updated_since`, `trend[...]`, `expand[]`, `order`) |
| survey responses | `GET /survey_responses/{id}.json` | retrieve one (`expand[]`) |
| survey responses | `POST /survey_responses.json` | create a response (`person`, `score`, `comment`, …) |
| survey responses | `PUT /survey_responses/{id}.json` | update (tags/notes/properties) |
| people | `GET /people.json` | list people (cursor pagination via `page_info`, `per_page`) |
| people | `POST /people.json` | create/update person + schedule survey ("send to people"; `email`, `name`, `properties[...]`, `send`, `delay`, `channel`) |
| people | `DELETE /people/{id}.json` | GDPR delete a person + their data |
| pending requests | `DELETE /people/{email}/survey_requests/pending.json` | cancel scheduled-but-unsent surveys for a person |
| bounces | `GET /bounces.json` | list bounced people (oldest first) |
| unsubscribes | `GET /unsubscribes.json` | list unsubscribed people |
| unsubscribes | `POST /unsubscribes.json` | unsubscribe a person (`person_email`/`person_id`) |
| autopilot | `GET /autopilot/{email\|sms}/memberships.json` | list Autopilot memberships (all, or one via `person_email`) |
| autopilot | `POST /autopilot/{email\|sms}/memberships.json` | add a person to Autopilot |
| autopilot | `DELETE /autopilot/{email\|sms}/memberships.json` | remove a person from Autopilot |
| autopilot | `GET /autopilot/{email\|sms}.json` | read Autopilot configuration |

Errors are non-2xx with a JSON body carrying a `message`; `401` on a bad/absent
key. (Web snippet and webhook config endpoints exist but are out of scope — not
useful to an AI teammate operating over an API key.)

## 4. anycli definition

### 4.1 Tool form — `service` (stage-1 rubric)

`service` type. No official Delighted CLI exists; the `cli`-type rubric
(official, non-interactive, `--json`-capable, provisionable binary) fails on the
first clause. Implement against the HTTP API in `internal/tools/delighted/`,
following the `bitly` reference shape (single-token Bearer/Basic REST service,
resource-grouped cobra tree, httptest-faked unit tests). 22 of 24 shipped
definitions are `service`.

### 4.2 Naming (all three axes identical → no resolver divergence)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `delighted` | bundle `tool.command` (flat, no group) |
| ② anycli tool id | `delighted` | `definitions/tools/delighted.json`, `RegisterService("delighted", …)` |
| ③ provider catalog key | `delighted` | `integrations/providers/delighted/` |

②≡③, so **no** `helio-cli/internal/toolcred/resolver.go` `toolToProvider`
entry is added (mechanical identity mapping is handled by `ProviderFor`'s
default). Go package: `internal/tools/delighted/` (no dash/digit normalization
needed).

### 4.3 Credential binding

`definitions/tools/delighted.json`:

```json
{
  "name": "delighted",
  "type": "service",
  "description": "Delighted as a tool (NPS/CSAT survey metrics, responses, people)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "DELIGHTED_API_KEY"}
      }
    ]
  }
}
```

- Credential field name **`api_key`** (not `access_token`) — this is a static
  API key, and the Helio bundle's `credential.fields` maps
  `api_key: token.access_token`, so the name is a local anycli↔env convention
  only; the resolver still supplies it from the single stored secret.
- Injected as env var `DELIGHTED_API_KEY`. The service reads it and calls
  `req.SetBasicAuth(key, "")` — the Basic-username scheme lives **entirely
  inside the anycli service**, so nothing in the credential map or token
  gateway needs to know about Basic encoding. This is the clean seam: anycli
  owns request construction; Helio owns storage/resolution of one opaque
  secret.

### 4.4 Command tree (resource-grouped, mirrors `notion`/`bitly`)

```
delighted metrics get            [--since --until --trend]
delighted response list          [--per-page --page --since --until --updated-since --order --expand --trend-key --trend-value]
delighted response get   --id ID [--expand]
delighted response create --person P --score N [--comment --properties-json --tags --channel]
delighted response update --id ID [--tags --notes --properties-json]
delighted people list            [--per-page --page-info]
delighted people send    --email E [--name --properties-json --send --delay --channel]
delighted people delete  --id ID
delighted people cancel-pending --email E        # DELETE .../survey_requests/pending.json
delighted bounces list           [--per-page --page]
delighted unsubscribes list      [--per-page --page]
delighted unsubscribes add --person-email E
delighted autopilot memberships list   --platform email|sms [--person-email]
delighted autopilot memberships add    --platform email|sms --person-email E [--name --properties-json]
delighted autopilot memberships remove --platform email|sms --person-email E
delighted autopilot config get         --platform email|sms
```

Design notes:
- **`--platform email|sms`** models the `/autopilot/{email|sms}/…` path segment
  as a required enum flag rather than two command subtrees — one code path,
  validated against the two literals.
- **JSON output:** every command emits the provider's JSON response verbatim to
  stdout + newline (`bitly`'s `emit`); no client-side reshaping. `--json` is a
  no-op persistent flag accepted for uniformity (output is always JSON).
- **Exit codes** (the documented anycli contract): `0` success; `1`
  runtime/API failure (non-2xx → typed `apiError` carrying Delighted's
  `message`); `2` usage/parse error. **`401` maps to
  `execution.RejectCredential`** so Helio surfaces a re-auth prompt (this is the
  only signal a bad key produces, since connect-time is no-verify — see §6).
  `429` is a plain retryable error surfacing `Retry-After`.
- Pagination flags are passed straight through as query params; the tool does
  **not** auto-paginate (agents page explicitly), matching the built-in
  service conventions.

## 5. Auth flow & credential fields (api_key lane)

- **Registration model:** none. The user generates/reads a per-project API key
  in the Delighted dashboard (Settings → API). No Helio-side OAuth client, no
  `config/`+`deploy/` client id/secret — this is the whole reason api_key tools
  need zero lane-1 app registration.
- **Scopes/semantics:** the key is all-or-anything the project exposes; there is
  no wire-level scope parameter. Long-lived; revoked by regenerating in the
  dashboard.
- **Storage/flow:** the user pastes the key through Helio's write-only
  `POST /connections/credentials`; it is stored in Vault as a single secret and
  served to `heliox tool delighted` through the token gateway, injected as
  `DELIGHTED_API_KEY`. The key never lives in the provider bundle.

## 6. Helio provider bundle plan (`integrations/providers/delighted/`)

### 6.1 The auth-model decision (the one real design choice)

Two facts from §3 rule out the header-token `manual_api_token` path:

1. **Basic-username scheme.** `declarativeManualTokenVerifier`
   (`integration-service/service/manual_token_verifier.go`) verifies a key by
   `GET`ting the bundle's `identity.url` with `req.Header.Set(APIKey.Header,
   token)` — a **raw header value**. It cannot produce
   `Authorization: Basic base64(key+":")`. A Delighted key sent that way is
   rejected.
2. **No identity endpoint.** The same verifier requires `identity.source:
   userinfo` + a `stable_key` JSON Pointer yielding a non-empty account string.
   Delighted exposes no `/me`/account object and `metrics.json` has no
   identifier, so there is nothing to extract even if the scheme fit.

Both push to the **`credentials` / `manual_credentials`** model — the **mongodb
precedent** (`integrations/providers/mongodb/provider.yaml`, design 317): a
single pasted opaque secret, `identity.source: strategy`, stored without a
`stable_key`/userinfo requirement. mongodb's only deriver on `main`
(`dsnHostIdentityDeriver`) parses the secret as a URL and would reject an opaque
Delighted key, so a **small integration-service capability addition** is needed.

**Option A (recommended — minimal, orthogonal): `credentials` /
`manual_credentials`, no provider-side verification, constant account key.**
Add a `constantKeyIdentityDeriver` selected for Delighted (the batch's
deriver-selection seam — cf. the amplitude/crisp/braze per-provider derivers —
is the growth point; on `main` only mongodb's exists, so this design adds one).
It performs **no** HTTP call, returns a fixed account key (e.g. `delighted`) and
a static label (`"Delighted"`), and never puts the secret in the identity map.
Rationale mirrors mongodb OQ1/OQ2: with no identity endpoint, connect-time
verification buys nothing and a bad key surfaces cleanly at first
`heliox tool delighted` call via anycli's `401 → CredentialRejected` (§4.4).
**Documented limitation:** one Delighted **project** per assistant connection —
because the account key is constant, connecting a second project's key replaces
the first. This matches the "each CX project has its own key" reality: an
assistant is bound to one project's key at a time.

**Option B (deferred enhancement): connect-time Basic-auth verify against
`GET /metrics.json`.** Unlike mongodb, Delighted *does* have a cheap validating
endpoint (200 = valid key, 401 = bad key). A `basicUsernameVerifier` variant
could give earlier "bad key" feedback at connect time. But it still needs the
constant-key path (metrics.json yields no identity), so it is Option A **plus** a
Basic-scheme verifier — strictly more machinery for a UX nicety. Recommend
shipping A first; revisit B if connect-time validation is desired program-wide.
(The batch has already grown sibling Basic/Bearer verifier variants — lemlist
`basic_password`, tally Bearer-scheme — so B is low-risk if prioritized, but it
is not required to ship Delighted.)

### 6.2 Bundle sketch (Option A)

```yaml
schema: helio.provider/v1
key: delighted
go_name: Delighted

presentation:
  name: Delighted
  description_key: delighted
  consent_domain: delighted.com
  visible: false          # hidden-first; flip only after L1–L5 green + anycli pin ships
  # order: <pick unoccupied at flip time>

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: delighted_api_key
        secret: true
        placeholder: "your Delighted project API key"
        required: true
    setup_url: https://app.delighted.com/docs/api   # "where to get the key"

identity:
  source: strategy        # no HTTPS userinfo endpoint; constant account key deriver

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_key: token.access_token        # single secret via the existing UpsertUserToken write path
    account_key: connection.account_key

tool:
  name: delighted
  kind: api-key                        # wire-compat tool kind (317 D2); client routes drawer by auth_type
```

- **`credential_input.fields`** is single-secret (one required field) — satisfies
  the D5 generation-time "exactly one required field" check that
  `resolveManualSecret` relies on.
- **No `auth.api_key` block** (that is only for `auth.type: api_key` header-token
  providers) and **no `identity.url`** (only allowed for `userinfo`). The
  generator's closed-field validation enforces both.

### 6.3 Service, config, resolver, icon, docs

- **integration-service:** the only code change is the `constantKeyIdentityDeriver`
  + its selection for `manual_credentials` providers that opt out of DSN parsing
  (small, unit-tested against the `manualTokenVerifier`/deriver interface — a
  synthetic-provider test like `manual_credential_test.go`). No adapter, no
  token-gateway change (single secret reuses the mongodb write path). **No
  `config/`/`deploy/` entries** — api_key/credentials providers carry no client
  id/secret, so integration-service renders `configured: true` with zero
  server config (nothing to sync, Config Sync rule N/A here).
- **resolver:** none (axes ②≡③).
- **icon:** `ui/helio-app/src/integrations/icons/delighted.svg` +
  hand-register in `providerIcons.ts`; add `tools.desc.delighted` /
  `delighted_api_key` i18n strings (all locales) — never generated.
- **AI-facing docs:** provider sub-doc under
  `agents/plugins/heliox/skills/tool/`; bump plugin version + publish
  (batch-end, one publish per batch).

### 6.4 Generation

From `go-services/integration-service`: `go run ./cmd/provider-gen` then
`--check`. Five projections regenerate together (committed by the batch lead at
batch end; on-branch runs are for L3 validation only and are **not** committed,
per master plan §2).

## 7. Test plan (five layers)

| Layer | Scope for Delighted | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in `internal/tools/delighted/`: httptest fake asserts path/method/query per command, `SetBasicAuth(key, "")` header shape, verbatim JSON emit, exit-code contract (0/1/2), `401 → CredentialRejected`, `429`+`Retry-After` surfaced. **No real API.** | No |
| **L2** harness real-API | `ANYCLI_CRED_API_KEY=<real key> anycli delighted -- metrics get`; then `response list`, `people list`, `bounces list`. Proves field name (`api_key`→`DELIGHTED_API_KEY`), Basic-username scheme, and live request shapes. **Mandatory before pin bump.** | **Yes** — a real Delighted project API key (account pool, lane 2) |
| **L3** `provider-gen --check` + suites | Local `provider-gen` + `--check` against the branch bundle; anycli `go test ./...`; integration-service tests incl. the new `constantKeyIdentityDeriver` unit test; helio-cli build with a local `replace` to the anycli branch + `go test ./cmd/heliox/cmds/tool/`. | No |
| **L4** singleton + seed | Start singleton (`env: dev`); `POST /internal/test-only/connections/seed` with `provider: delighted`, `api_key`/`access_token` = the **real** key from L2 (seed is allowed for api_key/credentials providers); then `heliox tool delighted -- metrics get` reaches the live API through the real token gateway. Non-expiring key → seed `access_token` only (no refresh cycle). | **Yes** — same real key |
| **L5** full connect flow | Hidden, pre-flip: open the connect link → paste the key through the real connect UI (`POST /connections/credentials`) → connection shows connected/configured (`GET /connections`) → one **unseeded** `heliox tool delighted -- metrics get` succeeds. This is the **api_key key-entry L5 path** (master plan §2), not the OAuth consent path. | **Yes** — same real key |

Notes:
- Layers needing externally supplied credentials: **L2, L4, L5** (all use one
  real Delighted project API key from the lane-2 account pool). L1/L3 are
  self-contained.
- **No lane-1 OAuth app registration** and **no `config/`+`deploy/` secret** are
  required (api_key/credentials lane) — the only human dependency is the test
  account key.
- Delighted's connect-time model is **no-verify** (Option A), so a wrong key
  passes connect and fails at first use with a `CredentialRejected` prompt —
  L5's success criterion is the *unseeded live run*, which is exactly what
  exercises the real key end-to-end.

## 8. Rollout

Hidden-first: land anycli `delighted` definition+service (batch-merges freely) →
batch-end pin bump + one anycli tag → land bundle (`visible: false`) + the
`constantKeyIdentityDeriver` + `provider-gen` regen (batch lead) → L1–L5 green →
flip `presentation.visible: true` + regenerate as the single go-live change.
