# Tool design: Delighted (`delighted`) — BLOCKED (provider sunset)

Scratch design doc for the `tool/delighted` branch (batch-lead strips at batch
end). Drives the anycli `delighted` service definition and the Helio
`delighted` provider bundle. Written from the pipeline skill
(`helio-tool-provider`), master plan `008-300` (§2 execution model, §3 naming),
and the Delighted official REST API.

Catalog row 213: Product **Delighted** · anycli id `delighted` · provider key
`delighted` · auth `api_key` · wave 3 · category Forms & Surveys.

## 0. Status: BLOCKED — DO NOT BUILD. Catalog divergence: provider fully sunset

**Verdict: do not implement this tool. Row 213 (Delighted) must be dropped from
the catalog.** The rest of this document is retained only as the evidence trail
and as the (now-moot) reference design that reached this conclusion.

### 0.1 The divergence (official docs override the catalog)

My instructions require following the provider's **official documentation** over
the catalog and recording any divergence here. The official docs say the product
no longer exists:

- **Delighted (the Qualtrics-owned NPS/CSAT product) was fully sunset on
  June 30, 2026, and access was terminated / the product completely shut down on
  July 1, 2026.** Today is **2026-07-22** — the platform and its REST API have
  been dead for ~3 weeks. Sources (all official or official-adjacent):
  - Official **Delighted Sunset FAQ** (Help Center, article 840): "The Delighted
    platform and product will be discontinued. It will cease to operate…" —
    timeline: annual renewals stopped 2025-07-01, monthly ended 2026-05-31,
    **final sunset 2026-06-30**, access terminated / fully shut down 2026-07-01,
    then **all account data is permanently deleted**.
    <https://delighted-help-160b834b9adfde78ebef1528.helpscoutdocs.com/article/840-delighted-sunset-faq>
  - Official API **client repositories**, all archived with an identical banner
    **"[DEPRECATED] Delighted API … Client — Delighted is sunset June 30, 2026"**:
    `delighted/delighted-node`, `delighted/delighted-ruby`, `delighted/delighted-php`,
    and the `delighted` npm package. These are the vendor's own SDKs; their
    deprecation is the vendor's own statement that the API is gone.
  - The Qualtrics sunset notice and the vendor sunset landing page
    (`delighted.com/?page_id=25877`) carry the same message.

- Master plan §3 already **DROPPED** two tools on exactly this "API viability"
  reasoning — Medium ("write API effectively closed") and Wave ("API restricted
  to invited partners"). **A fully-terminated product is a strictly stronger cut
  than either.** This design mirrors the §3 "Seed corrections" treatment and asks
  the batch lead / master-plan owner to remove row 213 the same way.

### 0.2 Why it cannot ship as-is (consequences)

1. **Zero value to an AI teammate.** Every command would hit a dead API. There is
   no account any user could connect and no data any request could return. A
   shipped `heliox tool delighted` returns nothing but connection/transport
   errors for every user — the opposite of the "human-natural colleague with a
   working tool" bar.
2. **The §7 test plan is unexecutable — permanently.** L2/L4/L5 all require "a
   real Delighted project API key from the lane-2 account pool," but **no new
   Delighted account can be provisioned** (sign-up and renewals are closed) and
   the **live API no longer responds**. So L2's "Mandatory before pin bump" gate
   and L5's "unseeded live run" success criterion **can never be met**. Per
   anycli TDD rules a tool that cannot pass its own required live tests must not
   be pinned or flipped visible.

### 0.3 Required action

- **Escalate to the batch lead / master-plan owner to drop Delighted from catalog
  row 213**, mirroring the §3 Medium/Wave "Seed corrections" treatment. Do not
  bump the anycli pin, do not land the provider bundle, do not flip visible.
- **If a Forms-&-Surveys NPS/CSAT slot is still wanted**, retarget to a *live*
  platform the lane-2 account pool can actually provision (the market successors
  now positioning for ex-Delighted users — e.g. Zonka Feedback, SurveyVista,
  Customer Thermometer, or a Qualtrics-native path). That is a **new catalog row
  with its own design doc**, not a rename of this one.
- **Leave `tool/delighted` unmerged.** No anycli definition, no Helio bundle, no
  `provider-gen` regen, no pin bump.

Everything below §0 is the pre-sunset reference analysis (API shape and auth
model were verified before the sunset was confirmed). **It is retained for the
record only and must not be used to build the tool.** Two review corrections are
folded in where they touch factual claims (the deriver-selection mechanism in
§6.1 and the rate-limit contract in §3/§4.4); both are otherwise moot under §0.

## 1. Audit reconciliation — `api_key` (pre-sunset reference)

Master-plan lane: `api_key`. OAuth audit row 215 verdict: **"no viable
multi-tenant path → stays api_key"** (compact note, no evidence URL).

Independent verification against the (then-live) official docs confirmed the
verdict:

- Delighted's REST API authenticated via **HTTP Basic Auth with the API key as
  the username and a blank password**, over HTTPS only. There was **no OAuth 2.0
  authorization-code flow** of any kind — no authorize/token endpoints, no app
  registration, no consent screen.
- The key was **per CX project**: "Each CX project has its own API key," copied
  from the Delighted dashboard (Settings → API). Long-lived, manual rotation.

So the lane was `api_key`, delivered as HTTP **Basic** (key-as-username), not a
plain header token. (All moot under §0 — the API no longer accepts any key.)

## 2. What an AI teammate would have done with Delighted (pre-sunset reference)

Delighted was an NPS/CSAT/CES customer-experience survey platform. An AI teammate
acting as a CX analyst / ops helper would have, in rough order of frequency:

1. **Read the score.** Pull current NPS/CSAT/CES aggregates and trend
   (`metrics`).
2. **Read & triage verbatim feedback.** List survey responses, filter by score
   band / time window, retrieve a single response (`survey_responses`).
3. **Annotate feedback.** Add tags / notes to a response (`survey_responses`
   update).
4. **Enroll customers to be surveyed.** Create/update a person and schedule a
   survey, or add them to Autopilot (`people`, `autopilot memberships`).
5. **Manage deliverability & compliance.** List bounced / unsubscribed people,
   unsubscribe, cancel pending sends, GDPR-delete a person.

## 3. Official API reference (pre-sunset reference)

- **Base URL:** `https://api.delighted.com/v1` (now non-responsive).
- **Transport:** HTTPS only; JSON request/response (`.json` suffix on every
  path).
- **Auth:** HTTP Basic — `Authorization: Basic base64(<api_key> + ":")` (key =
  username, empty password).
- **Rate limits (corrected per review — minor finding):** Delighted's official
  rate-limit guidance documented a **client-side fixed exponential backoff**
  (retry after 1s, then 2s, then 4s) against an average of ~100 req/min, and
  stated failed requests are safely retriable. It did **not** document a
  `Retry-After` **response header**. The earlier draft's claim that the tool
  "surfaces `Retry-After`" was wrong; a `429` handler should follow the
  documented fixed-schedule backoff and surface `Retry-After` **only if actually
  present**. (Moot under §0.)
- **Identity endpoint:** **none.** No `/me`, `/account`, or `/project` object;
  `metrics.json` returned only aggregate numbers with no account identifier. This
  was load-bearing for the §6 auth model.

Endpoints the tool would have wrapped (all under `/v1`, all `.json`):

| Group | Method & path | Purpose |
|---|---|---|
| metrics | `GET /metrics.json` | NPS/CSAT/CES aggregates (`since`, `until`, `trend`) |
| survey responses | `GET /survey_responses.json` | list (`per_page`, `page`, `since`, `until`, `updated_since`, `trend[...]`, `expand[]`, `order`) |
| survey responses | `GET /survey_responses/{id}.json` | retrieve one (`expand[]`) |
| survey responses | `POST /survey_responses.json` | create a response |
| survey responses | `PUT /survey_responses/{id}.json` | update (tags/notes/properties) |
| people | `GET /people.json` | list people (cursor pagination via `page_info`) |
| people | `POST /people.json` | create/update person + schedule survey |
| people | `DELETE /people/{id}.json` | GDPR delete a person + their data |
| pending requests | `DELETE /people/{email}/survey_requests/pending.json` | cancel scheduled-but-unsent surveys |
| bounces | `GET /bounces.json` | list bounced people |
| unsubscribes | `GET /unsubscribes.json` | list unsubscribed people |
| unsubscribes | `POST /unsubscribes.json` | unsubscribe a person |
| autopilot | `GET /autopilot/{email\|sms}/memberships.json` | list Autopilot memberships |
| autopilot | `POST /autopilot/{email\|sms}/memberships.json` | add a person to Autopilot |
| autopilot | `DELETE /autopilot/{email\|sms}/memberships.json` | remove a person from Autopilot |
| autopilot | `GET /autopilot/{email\|sms}.json` | read Autopilot configuration |

## 4. anycli definition (pre-sunset reference — not to be implemented)

### 4.1 Tool form — `service`

`service` type (no official CLI). Would follow the `bitly` reference shape
(single-token Basic REST service, resource-grouped cobra tree, httptest-faked
unit tests) in `internal/tools/delighted/`.

### 4.2 Naming (all three axes identical → no resolver divergence)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `delighted` | bundle `tool.command` (flat) |
| ② anycli tool id | `delighted` | `definitions/tools/delighted.json` |
| ③ provider catalog key | `delighted` | `integrations/providers/delighted/` |

②≡③, so no `toolToProvider` resolver entry would be added.

### 4.3 Credential binding (reference)

`api_key` field injected as env var `DELIGHTED_API_KEY`; the service would call
`req.SetBasicAuth(key, "")`, keeping the Basic-username scheme entirely inside
anycli. Helio stores/serves one opaque secret.

### 4.4 Command tree (reference)

```
delighted metrics get            [--since --until --trend]
delighted response list          [--per-page --page --since --until --updated-since --order --expand --trend-key --trend-value]
delighted response get   --id ID [--expand]
delighted response create --person P --score N [--comment --properties-json --tags --channel]
delighted response update --id ID [--tags --notes --properties-json]
delighted people list            [--per-page --page-info]
delighted people send    --email E [--name --properties-json --send --delay --channel]
delighted people delete  --id ID
delighted people cancel-pending --email E
delighted bounces list           [--per-page --page]
delighted unsubscribes list      [--per-page --page]
delighted unsubscribes add --person-email E
delighted autopilot memberships list   --platform email|sms [--person-email]
delighted autopilot memberships add    --platform email|sms --person-email E [--name --properties-json]
delighted autopilot memberships remove --platform email|sms --person-email E
delighted autopilot config get         --platform email|sms
```

Design notes (reference):
- **`--platform email|sms`** models the path segment as a required enum flag.
- **JSON output:** verbatim provider JSON to stdout.
- **Exit codes:** `0` success; `1` runtime/API failure; `2` usage/parse error.
  `401 → execution.RejectCredential`. **`429`:** follow the documented fixed
  exponential backoff (1s/2s/4s); surface `Retry-After` **only if the response
  actually carries it** (corrected per the minor finding — the API does not
  document sending that header).

## 5. Auth flow & credential fields (reference)

- **Registration model:** none — user reads a per-project key from the dashboard.
- **Scopes:** none at the wire level; long-lived key.
- **Storage/flow:** pasted via `POST /connections/credentials`, stored as a
  single Vault secret, injected as `DELIGHTED_API_KEY`.

## 6. Helio provider bundle plan (reference — not to be landed)

### 6.1 The auth-model decision (with review correction)

The header-token `manual_api_token` path was ruled out (Basic-username scheme;
no identity endpoint), pushing to the **`credentials` / `manual_credentials`**
model (the mongodb precedent, design 317): a single pasted opaque secret,
`identity.source: strategy`, no `stable_key`/userinfo requirement.

**Deriver selection — declarative, NOT a provider-name switch (major finding
correction).** The earlier draft proposed a `constantKeyIdentityDeriver`
"selected for Delighted" but never stated *how* selection happens. On `main`,
`composeProviderRegistration()` hardcodes `manual: dsnHostIdentityDeriver{}` for
the **entire** `RuntimeStrategyManualCredentials` case, with no per-provider
seam — and Delighted's proposed bundle is byte-shape-identical to mongodb's, so
nothing declarative distinguishes "derive host from DSN" (mongodb) from "constant
key" (Delighted). Resolving that by a hardcoded `switch definition.Provider`
would be exactly the "discriminator field on an overloaded model" smell the repo
CLAUDE.md forbids.

The **correct** design (had this shipped) is a **declarative selector the
registry/generator reads from the bundle**, not the provider name:

- Introduce a distinct **`identity.source` value** — e.g. `identity.source:
  constant` (constant account key, no HTTP) **vs** `identity.source: strategy`
  reserved for the DSN-deriving path (mongodb) — **or** an explicit
  `identity.deriver_kind` enum field on the provider model.
- `composeProviderRegistration()` then branches on that declared value to pick
  `constantKeyIdentityDeriver{}` vs `dsnHostIdentityDeriver{}`, and
  `provider-gen` validates the field against a closed set. No `switch` on
  `definition.Provider` anywhere.

This is moot under §0 (the tool is dropped), but it is the shape any future
constant-key `manual_credentials` provider in this batch must use.

### 6.2 Bundle sketch (reference, Option A)

```yaml
schema: helio.provider/v1
key: delighted
go_name: Delighted

presentation:
  name: Delighted
  description_key: delighted
  consent_domain: delighted.com
  visible: false

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
    setup_url: https://app.delighted.com/docs/api

identity:
  source: constant        # declarative selector → constantKeyIdentityDeriver (see §6.1)

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
    api_key: token.access_token
    account_key: connection.account_key

tool:
  name: delighted
  kind: api-key
```

### 6.3 Service, config, resolver, icon, docs (reference)

Would have needed the declarative `constantKeyIdentityDeriver` (§6.1) in
integration-service, an icon + i18n strings, and an AI-facing sub-doc. **None of
this should be created** — the tool is dropped.

### 6.4 Generation (reference)

`provider-gen` regen would be committed by the batch lead at batch end. **Not
applicable** — no bundle is landed.

## 7. Test plan — UNEXECUTABLE (provider dead)

**This plan cannot be run and must not gate anything. It is recorded to show
exactly why the tool is blocked, per §0.2.** L2/L4/L5 each require a real
Delighted project API key from the lane-2 account pool; **no new account can be
provisioned and the live API does not respond**, so the required live layers can
**never** pass.

| Layer | Scope | External creds | Executable now? |
|---|---|---|---|
| **L1** anycli unit | httptest fake asserts path/method/query, `SetBasicAuth(key,"")`, verbatim emit, exit codes, `401→CredentialRejected`, `429` backoff | No | Would run against a *fake*, but pointless — ships a tool that hits a dead API |
| **L2** harness real-API | live `metrics get` etc. against `api.delighted.com` | **Yes** | **NO — API dead; account unprovisionable.** Gate "mandatory before pin bump" can never be met |
| **L3** `provider-gen --check` + suites | local generation/build checks | No | Would run, but validates a bundle that must not land |
| **L4** singleton + seed | seed real key → live call through token gateway | **Yes** | **NO — same dead-API blocker** |
| **L5** full connect flow | paste key via real connect UI → **unseeded** live `metrics get` | **Yes** | **NO — the "unseeded live run" success criterion is unreachable** |

## 8. Rollout — CANCELLED

**Do not roll out.** No hidden-first landing, no pin bump, no visible flip. The
action is §0.3: escalate to drop catalog row 213 and, if a Forms-&-Surveys NPS
slot is still wanted, open a new design doc targeting a live provider.
