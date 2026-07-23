# Google Ads — per-tool design (`heliox tool google ads`)

Scratch design for the `google_ads` provider (master plan row 133, Wave 2,
`oauth_review`, Marketing). Batch-lead strips this file at batch end. Follows
`.claude/skills/helio-tool-provider/SKILL.md` and the master plan
`docs/design/008-300-integrations-rollout-plan.md`.

Status: design only. No code in this doc is committed to either repo yet.

---

## 0. Naming (the three axes) — verified against the master plan §3 row 133

| Axis | Value | Where it lives |
|---|---|---|
| ① CLI command word (group-scoped) | `ads` | bundle `tool.command`; `toolGroups["google"]` entry `{Command: "ads", …}` |
| ② anycli tool id (global-unique, dash-case) | `google-ads` | `definitions/tools/google-ads.json` `name`; `RegisterService("google-ads", …)` |
| ③ provider catalog key (underscore-case) | `google_ads` | `integrations/providers/google_ads/` dir name + `key:` |

- Surfaced as **`heliox tool google ads -- …`** (design 303 grouped family), NOT a
  flat `heliox tool google-ads`. Every new Google-family tool MUST declare
  `tool.group: google` (master plan §3); the group word (axis ①) is the short
  form `ads`, exactly the `outlook` ↔ `microsoft-outlook` precedent where the
  group word differs from the anycli id.
- ②↔③ diverge (`google-ads` vs `google_ads`), so **one** `toolToProvider`
  entry is required: `"google-ads": "google_ads"` in
  `helio-cli/internal/toolcred/resolver.go`. This is one of the 23 mechanical
  dash↔underscore pairs the master plan §3 counts; it is NOT the non-mechanical
  `search-console` case.
- Go package name (stage 2): `googleads` (drop the dash), file
  `internal/tools/googleads/`. Only the definition filename and the
  `RegisterService` string carry the exact dashed id `google-ads`.

---

## 1. What an AI teammate does with Google Ads → which API surface

An AI teammate on Google Ads is overwhelmingly a **reporting/analysis and
light-management** actor, not a campaign builder. Concretely: "how did my
campaigns perform last week", "which keywords are burning budget with no
conversions", "pause the underperforming ad group", "what's my current daily
budget on campaign X". That maps to **one** official surface — the
**Google Ads API** (`googleads.googleapis.com`), REST/JSON transcoding of the
gRPC service — driven by **GAQL** (Google Ads Query Language), plus a handful
of targeted `:mutate` calls for management.

### Official API (verified 2026-07-22)

- **Product API:** Google Ads API. REST base `https://googleads.googleapis.com/v24`
  (v24 current; Google ships a new version ~quarterly and supports roughly the
  latest 3 — pin the version as a package constant and bump on the deprecation
  cadence, never track "latest" implicitly).
- **Auth model (two credentials — this is the crux, see §3):**
  1. A per-user OAuth2 access token with the **`https://www.googleapis.com/auth/adwords`** scope
     (`Authorization: Bearer …`). This is a **sensitive scope** → Google OAuth
     app verification is required before arbitrary external Google accounts can
     consent → this is one half of the `oauth_review` lane.
  2. An **app-level developer token** (`developer-token: …` header), a
     22-char string minted from a Google Ads **manager (MCC)** account's API
     Center. New tokens default to **Explorer** access (2,880 ops/day against
     production) or, when auto-review fails, **Test Account** access; a
     manual review promotes the token to **Basic** (test + production, no
     daily cap on the ops that matter) or **Standard**. This developer-token
     review is the *other* half of `oauth_review` — and it gates only the
     **visible flip**, never dev/L4 (a Test/Explorer token is enough to build
     and L4-seed against test accounts). As of early 2026 Google has a
     public backlog on Basic/Standard applications — front-run it in human
     lane 1.
  3. Optional `login-customer-id: <MCC id, digits only>` header when operating
     through a manager account.
- **Endpoints used:**
  - `GET  /v24/customers:listAccessibleCustomers` — resource names
    (`customers/{id}`) of every account the OAuth user can touch. The
    enumeration primitive: the assistant lists accounts, then targets one.
  - `POST /v24/customers/{customerId}/googleAds:search` — GAQL query, paged
    (`pageToken`/`nextPageToken`, single JSON object response).
  - `POST /v24/customers/{customerId}/googleAds:searchStream` — same GAQL,
    streamed; response is a **JSON array** of result chunks (documented quirk,
    unlike every other endpoint's single object). Preferred for large report
    pulls (no paging round-trips).
  - `POST /v24/customers/{customerId}/campaigns:mutate`,
    `…/campaignBudgets:mutate`, `…/adGroups:mutate`, `…/adGroupAds:mutate` —
    targeted management operations (status changes, budget amount).
- **Why not a broader surface:** everything a teammate needs — entities,
  metrics, segments, change history — is queryable through GAQL against
  `googleAds:search`. There is no separate "reporting API"; GAQL *is* the
  reporting layer. Keyword-planner/asset-generation/experiment services are
  out of scope for v1 (specialist, write-heavy, low teammate demand).

### Divergence check vs catalog / audit

- Catalog auth lane `oauth_review`: **confirmed correct** against official docs
  — but for **two independent** reasons the catalog compresses into one cell:
  (a) Google sensitive-scope OAuth verification for `adwords`, and (b) the
  developer-token Basic/Standard access review. Either alone would justify
  `oauth_review`; both apply. Recorded here per the "verify everything" charge.
- Not in the OAuth audit table (audit covered only pre-audit `api_key` rows;
  Google Ads was already `oauth_review` in the seed), so no audit verdict to
  contradict.

---

## 2. anycli definition & service

### 2.1 Form decision — `service` type

`service` type (not `cli`). Rubric: a `cli` type needs an official,
non-interactive, `--json`-capable, image-provisionable binary. Google ships
**client libraries** (Java/Python/Go/…), not a first-class standalone Ads CLI
that meets the bar. So implement `service` type against the REST API in
`internal/tools/googleads/`, following the `internal/tools/notion/` shape
(cobra tree grouped by resource; `BaseURL`/`HC`/`Out`/`Err` struct for
httptest injection; exit codes 0 success / 1 API-runtime / 2 usage; `--json`
error envelope). This matches the sibling google-family tools (gmail, sheets,
… are all `service`).

### 2.2 Definition JSON (`definitions/tools/google-ads.json`)

Diverges from the gmail template because Google Ads needs **two** injected
credentials (user token + app developer token). Both are delivered by the
host's credential map (see §3 for how the developer token gets there); anycli
stays credential-agnostic — it only declares field names and injection.

```json
{
  "name": "google-ads",
  "type": "service",
  "description": "Google Ads as a tool (user access token + app developer token)",
  "auth": {
    "credentials": [
      { "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "GOOGLE_ADS_ACCESS_TOKEN"} },
      { "source": {"field": "developer_token"},
        "inject": {"type": "env", "env_var": "GOOGLE_ADS_DEVELOPER_TOKEN"} }
    ]
  }
}
```

The service sends `Authorization: Bearer $GOOGLE_ADS_ACCESS_TOKEN` and
`developer-token: $GOOGLE_ADS_DEVELOPER_TOKEN` on every request; a
`--login-customer-id` flag (or `GOOGLE_ADS_LOGIN_CUSTOMER_ID` env, optional)
sets the `login-customer-id` header for MCC operation.

### 2.3 Subcommands / verbs

Resource-grouped cobra tree; read-first, GAQL-centric:

| Command | HTTP | Purpose |
|---|---|---|
| `accounts list` | `GET customers:listAccessibleCustomers` | enumerate reachable customer IDs (the entry point; no `--customer-id` needed) |
| `query --customer-id <id> --gaql "<GAQL>" [--stream] [--page-size N] [--page-token T]` | `googleAds:search` / `:searchStream` | raw GAQL escape hatch — full power for the AI |
| `report --customer-id <id> --resource campaign\|ad_group\|keyword --date-range LAST_30_DAYS [--metrics …] [--segments …]` | `googleAds:searchStream` | convenience that *builds* GAQL for the common "performance" ask so the AI needn't hand-write it |
| `campaigns list --customer-id <id> [--status …]` | `googleAds:search` | list campaigns + key metrics |
| `campaign set-status --customer-id <id> --id <cid> --status ENABLED\|PAUSED` | `campaigns:mutate` | guarded write |
| `budget set --customer-id <id> --id <bid> --amount-micros <n>` | `campaignBudgets:mutate` | guarded write |

- Writes (`set-status`, `budget set`) are minimal and explicit-id only; no
  create/delete in v1. The teammate's job is steering, not building.
- `report` is sugar over `query`: it composes a GAQL `SELECT … FROM … WHERE
  segments.date DURING …` from flags, so the common path is one flag set and
  the escape hatch (`query`) remains for anything else.

### 2.4 JSON output shape

Single JSON object to stdout per command (matching notion precedent):

- `accounts list` → `{"data":{"customers":["customers/1234567890", …]}}`
- `query`/`report`/`*_list` → `{"data":{"results":[…], "fieldMask":"…"}, "nextPageToken":"…"}`
  (searchStream chunks flattened into one `results` array before emit, so the
  streamed-array quirk never leaks to the caller).
- `*:mutate` → `{"data":{"results":[{"resourceName":"customers/…/campaigns/…"}]}}`
- Error → `{"error":{"code":"…","message":"…","details":{…}}}` with exit 1
  (API/runtime) or 2 (usage). Google Ads errors carry a rich `error.details[]`
  with `errors[].errorCode` + `errors[].message`; surface those verbatim under
  `details` — they are the actionable part (e.g. `AuthorizationError`,
  `QueryError`, `QuotaError`).

---

## 3. Credential fields & the exact auth flow

### 3.1 The two-credential problem (central design decision)

Every Google Ads API call needs **both** a per-user OAuth token **and** the
app-level developer token. The OAuth token is standard and rides the golden
`standard_oauth` path unchanged. The developer token is a **shared app secret**
— identical in kind to Discord's `oauth.bot_token`: one Helio-owned value,
never user-entered, that must be delivered server-side into the runtime call.

The Helio credential-source enum is a **closed allowlist**
(`go-services/integration-service/model/catalog.go` — `token.access_token`,
`connection.account_key`, `connection.metadata.person_urn`,
`credential.app_id`, `credential.brand`). **There is no `config.*` source
today.** Discord solves its shared-token need with a *compiled adapter*
(`service/adapter_discord.go`, `runtime_strategy: discord_bot_install`), which
mints/serves the bot token outside the generic credential map.

**Two options; recommend Option A.**

- **Option A — grow the golden path by one reviewed enum value (recommended).**
  Keep `connection.runtime_strategy: standard_oauth` (the whole OAuth lifecycle
  — authorize, `adwords` scope, refresh, revoke — is 100% standard). Add:
  - a new required config field `oauth.developer_token` (or a dedicated
    `google_ads.developer_token`) to `knownConfigFields` +
    `provider_configuration.go` startup validation, sourced from
    integration-service config like `oauth.bot_token` is;
  - a new closed credential source `config.developer_token` (named arm, not a
    generic vault read — consistent with the enum's "named values a specific
    arm sets" contract) that the token gateway projects into the outbound
    credential map from that config value.

  Then the bundle's `credential.fields` maps `developer_token:
  config.developer_token`, the resolver/token-gateway returns it alongside
  `access_token`, and anycli injects it as `GOOGLE_ADS_DEVELOPER_TOKEN`. This
  is exactly the skill's guidance: prefer growing "one more reviewed enum
  value" over an adapter when the gap isn't truly provider-shaped. The gap here
  is generic ("deliver an app-level config secret to the runtime"), so it
  belongs in the shared capability, and Meta Ads (row 134, same batch, same
  app-token shape) reuses it immediately.

- **Option B — a `google_ads` compiled adapter.** Mirrors Discord. Heavier,
  provider-specific, and doesn't generalize to Meta Ads. Reserve only if
  review surfaces a reason Option A's config-sourced field can't satisfy the
  token gateway's contract. The master plan budgets adapters for a "handful" of
  review-lane tools; spend that budget on genuinely non-standard *lifecycles*
  (Bill.com sessions, Mastodon per-instance registration), not on a static
  header value.

**This capability growth is the one non-golden-path element of this tool and
must be built + reviewed in integration-service before L4.** It is the analog
of the per-tool capability growths already shipped this program (salesforce
`instance_url` capture, posthog `form_public`, etc.).

### 3.2 OAuth flow (standard, mirrors gmail)

- `authorize_url: https://accounts.google.com/o/oauth2/v2/auth`
- `token_url: https://oauth2.googleapis.com/token`, `token_exchange_style: form_secret`, `pkce: none`
- `authorize_params: {access_type: offline, prompt: consent}` (required to mint
  and re-mint a refresh token)
- `scopes: [openid, email, profile, https://www.googleapis.com/auth/adwords]`
- `single_active_token: false`, `refresh_lease: none`
- `revoke: {url: https://oauth2.googleapis.com/revoke, token: refresh_token, fallback_token: access_token, client_auth: none}`
- **Identity:** `source: userinfo`,
  `url: https://openidconnect.googleapis.com/v1/userinfo`, `stable_key: /sub`,
  `label_candidates: [/email, /name, /sub]` — the account_key is the Google
  user, identical to gmail. Customer IDs are **not** an identity/selection axis
  at connect time; they're discovered at runtime via `accounts list`
  (`resources.selection: none`, like gmail). The teammate picks the customer
  per call via `--customer-id`.

### 3.3 Config fields the environment must supply

`auth.required_config_fields: [oauth.client_id, oauth.client_secret,
oauth.developer_token]` (Option A). Values land in integration-service config
(`config/` locally + the `deploy/` Helm Secret together, per Config Sync) via
human lane 1 — id/secret/developer-token in one change, since a *partially*
configured provider fails startup while an *all-absent* one renders
`configured: false` and ships safely hidden.

Note: Google is piloting "cloud-managed access levels" that let callers omit
the developer-token header (access governed by a GCP org). If that GAs, Option
A's `developer_token` field can later become optional — but it is a pilot today
("continue to use the API Center"), so v1 assumes the header is required.

---

## 4. Helio provider bundle plan (`integrations/providers/google_ads/provider.yaml`)

Hidden-first. Shape below (Option A):

```yaml
schema: helio.provider/v1
key: google_ads
go_name: GoogleAds

presentation:
  name: Google Ads
  description_key: google_ads
  consent_domain: accounts.google.com
  visible: false            # hidden-first; flip is the single go-live change after L5 + review
  order: <next in Marketing block>

auth:
  type: oauth
  owner: individual
  required_config_fields: [oauth.client_id, oauth.client_secret, oauth.developer_token]
  oauth:
    authorize_url: https://accounts.google.com/o/oauth2/v2/auth
    token_url: https://oauth2.googleapis.com/token
    token_exchange_style: form_secret
    pkce: none
    authorize_params: {access_type: offline, prompt: consent}
    scopes: [openid, email, profile, https://www.googleapis.com/auth/adwords]
    display_scopes: [openid, email, profile, adwords]
    single_active_token: false
    refresh_lease: none
    revoke: {url: https://oauth2.googleapis.com/revoke, client_auth: none, token: refresh_token, fallback_token: access_token, token_type_hint: none}

identity:
  source: userinfo
  url: https://openidconnect.googleapis.com/v1/userinfo
  stable_key: /sub
  label_candidates: [/email, /name, /sub]

connection:
  mode: isolated
  disconnect_mode: provider_revoke
  runtime_strategy: standard_oauth        # OAuth lifecycle unchanged; developer_token rides the new config source

resources: {selection: none, discovery: none, enforcement: none}

credential:
  fields:
    access_token: token.access_token
    developer_token: config.developer_token   # NEW closed source (§3.1 Option A)
    account_key: connection.account_key

tool:
  name: google-ads      # axis ② (anycli id)
  command: ads          # axis ① (group word) → heliox tool google ads
  group: google
  kind: oauth
  experiment: google_tools    # design-090 gate, consistent with the other non-gmail google-family members
```

Cross-file obligations landing with the bundle at batch end:
- `toolToProvider["google-ads"] = "google_ads"` in `resolver.go` (+ test).
- `toolGroups["google"]` gains `{Command: "ads", Name: "google-ads",
  DisplayName: "Google Ads", AuthType: "oauth", Experiment: "google_tools",
  Hidden: false, CredentialFields: ["access_token","developer_token","account_key"]}`
  (generated by provider-gen; must stay aligned with `toolToProvider`).
- UI icon `ui/helio-app/src/integrations/icons/google_ads.svg` +
  `providerIcons.ts` registration (manual, never generated).
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` + plugin version
  bump/publish.
- Five provider-gen projections regenerated together (never committed on the
  tool branch — batch lead produces the canonical regen).

---

## 5. Test plan → the five layers

| Layer | What it proves for google-ads | Needs external creds? |
|---|---|---|
| **L1** anycli `go test ./...` | httptest fakes for `listAccessibleCustomers`, `googleAds:search`/`:searchStream` (assert the streamed-array flatten), `campaigns:mutate`; both header injections (`Authorization` + `developer-token`); `--json` error envelope decoding Google's `error.details[].errors[]`; exit-code contract. | No |
| **L2** dev harness vs REAL API (`ANYCLI_CRED_ACCESS_TOKEN=… ANYCLI_CRED_DEVELOPER_TOKEN=… anycli google-ads -- accounts list`) | field names + injection + request shape actually match the live API. Run against a **Google Ads test account** (MCC-created, serves no ads) with a **Test/Explorer** developer token — sufficient here, no Basic access needed. | **Yes**: a Google user OAuth token with `adwords` scope + a developer token + a test customer id. Human lane 2 (account pool) + lane 1 (dev app). |
| **L3** `provider-gen --check` + both repos' unit suites | bundle strict-decodes; the new `config.developer_token` source + `oauth.developer_token` config field pass the closed-enum validators; `toolToProvider`/`toolGroups` alignment; helio-cli builds with a local `replace` at the anycli branch. | No |
| **L4** singleton + seed + `heliox tool google ads -- accounts list` | seeded OAuth token (from a Google Ads **test** account, `access_token` + `refresh_token` + short `expires_at` to force the refresh path) reaches the live API through the token gateway; **and** the new config-sourced `developer_token` is delivered end-to-end (the L4 success signal is a real API response, impossible without the developer token wired). Requires the §3.1 capability to be merged in integration-service. | **Yes**: same as L2, plus the developer token present in local uncommitted `config/cloud.yaml` (lane 1). |
| **L5** full `tool auth` → Google consent → unseeded run, once before the visible flip | the real connect UX: authorize URL with `adwords` scope, callback, `/sub` identity extraction, `oauth_connected` event, then an unseeded live `accounts list` + one `report`. Human-in-the-loop (Google 2FA/consent defeats automation). Gated additionally on the **developer-token Basic-access review** clearing (Standard not required) for the visible flip. | **Yes**: a real Google account with a live Google Ads account + the Basic-access developer token. Lane 1 (review) + lane 3 (human consent). |

Externally-supplied-credential layers: **L2, L4, L5** (all need a real Google
OAuth token, a developer token, and a Google Ads customer/test account). L1/L3
are hermetic.

Definition of done (per master plan §2): all five layers green, docs published,
icon registered, then `presentation.visible: true` + regenerate as the single
go-live change — the flip additionally gated on the developer-token Basic-access
review, decoupled from dev via hidden-first.

---

## 6. Risks / open items specific to google-ads

1. **The `config.developer_token` capability (§3.1) is a real, reviewed change
   to integration-service's closed credential-source enum**, not a bundle-only
   tool. It must land + be reviewed before L4. Recommend building it generically
   ("app-level config secret → credential map") so Meta Ads (row 134) reuses it
   the same batch. Flag at stage 1 as an adapter/capability candidate per the
   master plan's §5 guidance.
2. **Developer-token review backlog (2026).** Basic/Standard applications are
   backlogged. Hidden-first means zero waste — code lands complete-but-hidden on
   a Test/Explorer token; only the flip waits on Basic. Track in the wave board.
3. **API version churn.** Pin `v24` as a constant; Google deprecates ~quarterly
   (3-version window). A version bump is a small, isolated follow-up, not a
   rewrite.
4. **`login-customer-id` for MCC users.** Exposed as an optional flag/env; not
   part of identity or the connection key. Most single-account teammates never
   set it; agency/MCC users do. Documented in the AI-facing sub-doc.
5. **Write scope.** v1 ships read + status/budget mutate only. No create/delete.
   Revisit only on demonstrated teammate demand.
