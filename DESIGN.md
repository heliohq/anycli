# Tool design: Delighted (`delighted`) — BUILT HIDDEN, BLOCKED at visible flip (provider sunset)

Scratch design doc for the `tool/delighted` branch (batch-lead strips at batch
end). Drives the anycli `delighted` service definition and the Helio
`delighted` provider bundle. Written from the pipeline skill
(`helio-tool-provider`), master plan `008-300` (§2 execution model, §3 naming),
and the Delighted official REST API.

Catalog row 213: Product **Delighted** · anycli id `delighted` · provider key
`delighted` · auth `api_key` · wave 3 · category Forms & Surveys.

## 0. Status: BUILT HIDDEN, BLOCKED at the visible flip — provider fully sunset

**Verdict: the credential-free layers (L1 anycli unit, L3 provider-gen --check +
both repos' suites) are built and green, and the tool ships hidden
(`presentation.visible: false`). But it can NEVER pass the live layers (L2/L4/L5)
and therefore must NEVER flip visible: the Delighted product is permanently
sunset and its production REST API returns HTTP 410 Gone. This is a genuine,
permanent BLOCK on go-live, not a pending-credentials wait.**

Why build the hidden scaffold at all instead of dropping the row outright: the
hidden-first model (pipeline skill stage 10, master plan §2 "Hidden-first
rollout") already separates "code merged and L1–L3 green" from "flipped visible
after L5". Landing the anycli definition + service + unit tests and the hidden
Helio bundle keeps the branch a complete, reviewable, credential-free-green
artifact and records — in code and in this doc — exactly why the live layers can
never run, rather than leaving a silent gap. The visible flip is the single
gated step, and it is blocked forever here. An earlier revision of this doc
recommended dropping row 213 entirely; that recommendation is superseded by the
hidden-first build below, but the drop remains a reasonable master-plan-owner
call (see §0.3) since the tool can never serve a live request.

The reference design in §1–§8 was verified against the (then-live) official API
before the sunset was confirmed; it is retained and now realised as the shipped
hidden implementation, with the live-layer block called out throughout.

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
  - **Live re-verification (2026-07-22):** the production REST base
    `GET https://api.delighted.com/v1/metrics.json` now returns **HTTP 410
    Gone** — a deliberate "permanently removed" tombstone, not a transient
    outage. This is direct, first-hand confirmation (stronger than "does not
    respond") that the API is decommissioned. The official Sunset FAQ was also
    re-read the same day and re-confirms: platform "completely deprecated and
    shut down" as of 2026-07-01, with all account data deleted after the final
    sunset date.

- Master plan §3 already **DROPPED** two tools on exactly this "API viability"
  reasoning — Medium ("write API effectively closed") and Wave ("API restricted
  to invited partners"). **A fully-terminated product is a strictly stronger cut
  than either.** This design mirrors the §3 "Seed corrections" treatment and asks
  the batch lead / master-plan owner to remove row 213 the same way.

### 0.2 What ships, and why the live layers can never run (consequences)

1. **Ships hidden, never visible.** The provider bundle lands with
   `presentation.visible: false`; hidden is a presentation axis, not a
   runnability gate, so the branch is a complete credential-free-green artifact.
   The visible flip (the single gated step) must never happen: a visible
   `heliox tool delighted` would return nothing but transport/410 errors for
   every user, the opposite of the "human-natural colleague with a working tool"
   bar. No account can be connected and no request can return data.
2. **The §7 live test layers are unexecutable — permanently, not credential-
   gated.** L2/L4/L5 all require "a real Delighted project API key from the
   lane-2 account pool," but **no new Delighted account can be provisioned**
   (sign-up and renewals are closed) and the **live API returns HTTP 410 Gone**.
   So L2's "mandatory before pin bump" gate and L5's "unseeded live run" success
   criterion **can never be met by any credential**. The credential-free layers
   (L1 unit, L3 provider-gen --check + suites) are green; the live layers are
   permanently blocked. Per anycli/pipeline rules the tool must therefore never
   flip visible.

### 0.3 Required action

- **Ship hidden; never flip visible.** Land the anycli definition + service +
  unit tests and the hidden Helio bundle (this branch). Do NOT set
  `presentation.visible: true`, and gate any future visible flip on the (never-
  satisfiable) L5 live run.
- **A master-plan owner may still drop catalog row 213 entirely** — mirroring the
  §3 Medium/Wave "Seed corrections" cuts — since the tool can never serve a live
  request. Dropping the row and keeping this hidden scaffold are both defensible;
  the choice is theirs. This doc no longer *requires* the drop.
- **If a Forms-&-Surveys NPS/CSAT slot is still wanted**, retarget to a *live*
  platform the lane-2 account pool can actually provision (the market successors
  now positioning for ex-Delighted users — e.g. Zonka Feedback, SurveyVista,
  Customer Thermometer, or a Qualtrics-native path). That is a **new catalog row
  with its own design doc**, not a rename of this one.

Everything below §0 is the API analysis (shape and auth model were verified
against the official docs before the sunset was confirmed) and is now realised
as the shipped hidden implementation. The live layers remain permanently blocked
per §0.2. Two review corrections are folded in where they touch factual claims
(the deriver-selection note in §6.1 and the rate-limit contract in §3/§4.4).

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

## 4. anycli definition (implemented — hidden)

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

## 6. Helio provider bundle (implemented — hidden)

### 6.1 The auth-model decision (and why NO capability growth)

The header-token `manual_api_token` path is ruled out: its
`declarativeManualTokenVerifier` does a live GET against the bundle's identity
endpoint at connect time, and Delighted has (a) no identity endpoint and (b) a
dead API — every connect would fail. So the bundle uses the
**`credentials` / `manual_credentials`** model (the mongodb precedent, design
317): a single pasted opaque secret, `identity.source: strategy`, no
`stable_key`/userinfo requirement, and NO provider-side verification at connect
(the `manual_credentials` case performs no HTTP at connect).

**No integration-service capability growth.** On `main`,
`composeProviderRegistration()` hardcodes `manual: dsnHostIdentityDeriver{}` for
the entire `RuntimeStrategyManualCredentials` case. `dsnHostIdentityDeriver`
parses the secret as a URL and derives the account key from its host — which
does not fit Delighted's opaque, non-DSN API key. An earlier revision proposed
growing a declarative `identity.source: constant` selector + a
`constantKeyIdentityDeriver` so the key wouldn't be parsed as a DSN. **That
capability growth is deliberately NOT done here**, for two reasons:

1. **It only ever manifests at connect time (L5), which is permanently
   blocked.** The dsn-host mismatch would surface as a connect-time
   `manualCredentialFormatError`, but no connect can ever run against a dead
   API / unprovisionable account. Growing shared, load-bearing service code to
   fix a path that can never execute is over-engineering (repo CLAUDE.md:
   "solve the immediate problem", "subtract before adding"). The immediate
   problem is a credential-free-green hidden scaffold, and the mongodb bundle
   shape delivers exactly that: `provider-gen --check` validates it statically,
   and the registry builds it (no key is parsed at build time).
2. **The tool can never go visible anyway** (§0.2), so the "correct" deriver
   would serve no live request even if added.

If a *future*, live constant-key `manual_credentials` provider needs it, the
declarative-selector shape (a closed `identity.source`/`deriver_kind` enum the
generator validates, never a `switch definition.Provider`) is the right design —
but that belongs to that provider's change, not this permanently-blocked one.

### 6.2 Bundle (as landed)

```yaml
schema: helio.provider/v1
key: delighted
go_name: Delighted

presentation:
  name: Delighted
  description_key: delighted
  consent_domain: delighted.com
  visible: false          # PERMANENT: provider sunset, API 410 Gone; never flip

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
  source: strategy        # manual_credentials → dsnHostIdentityDeriver on main
                          # (no live call at connect; see §6.1 — capability
                          # growth intentionally deferred, moot while blocked)

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

### 6.3 Service, config, resolver, icon, docs

- **Service code:** none — `manual_credentials` needs zero integration-service
  code (registry uses the existing `dsnHostIdentityDeriver`; §6.1).
- **Config:** none — `credentials`/`manual_credentials` providers declare no
  `required_config_fields` (no OAuth client id/secret), so there is nothing to
  land in `config/` or `deploy/`.
- **Resolver:** no `toolToProvider` entry — axis ② (`delighted`) ≡ axis ③
  (`delighted`), no divergence (§4.2).
- **Icon + i18n + AI-facing sub-doc:** landed (icon SVG + `providerIcons.ts`
  registration, `tools.desc.delighted` locale strings, provider sub-doc under
  `agents/plugins/heliox/skills/tool/`).

### 6.4 Generation

`provider-gen` regen is run locally to validate the bundle (`provider-gen
--check` green) but the five projected files are **not committed** on this
branch — per master plan §2, the batch lead produces the one canonical regen at
batch end. The bundle + icon + docs ride the batch-end merge.

## 7. Test plan — credential-free layers green; live layers permanently blocked

**L1 and L3 are green. L2/L4/L5 are permanently unexecutable — not
credential-gated.** They each require a real Delighted project API key from the
lane-2 account pool; **no new account can be provisioned and the live API
returns HTTP 410 Gone**, so no credential can ever make them pass. This is the
block on the visible flip.

| Layer | Scope | External creds | Status |
|---|---|---|---|
| **L1** anycli unit | httptest fake asserts path/method/query, `SetBasicAuth(key,"")`, verbatim emit, exit codes, `401→CredentialRejected`, `500` does-not-reject, JSON-flag/platform usage errors | No | **GREEN** — `go test ./...` passes |
| **L2** harness real-API | live `metrics get` etc. against `api.delighted.com` | **Yes** | **PERMANENTLY BLOCKED** — API 410 Gone; account unprovisionable |
| **L3** `provider-gen --check` + both repos' suites | local generation/build checks | No | **GREEN** — bundle validates, both repos build/test |
| **L4** singleton + seed | seed key → live call through token gateway | **Yes** | **PERMANENTLY BLOCKED** — same dead-API blocker |
| **L5** full connect flow | paste key via real connect UI → **unseeded** live `metrics get` | **Yes** | **PERMANENTLY BLOCKED** — unseeded live run unreachable |

## 8. Rollout — hidden only; visible flip permanently blocked

Ship hidden (`presentation.visible: false`) as a normal hidden-first landing:
anycli definition + service + L1 tests, hidden Helio bundle + icon + docs, one
batch-end regen + pin bump by the batch lead. The visible flip (SKILL.md stage
10) is gated on L5, which can never pass — so the tool **never flips visible**.
A master-plan owner may alternatively drop catalog row 213 entirely (§0.3);
either way, no live `heliox tool delighted` ever reaches a user.
