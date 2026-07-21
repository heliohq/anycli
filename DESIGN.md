# Tool Design: Google Analytics (`google-analytics` / `google_analytics`)

**Catalog row:** #118 · Wave 2 · Analytics · auth lane `oauth_review`
**Branches:** anycli `tool/google-analytics` (this worktree) · Helio `tool/google-analytics`
**Scratch file:** committed on-branch; the batch lead strips it at batch-end.

## 1. Scope: what an AI teammate does with Google Analytics

An AI teammate's real GA workload is **read-only reporting**: "how much traffic
did we get last week and from where", "top pages this month vs last", "how many
users are on the site right now", "conversion events by campaign". It is not
property administration (creating properties/streams) and not data ingestion
(Measurement Protocol). Two consequences:

- **GA4 only.** The Universal Analytics reporting APIs were shut down by Google
  (UA data access ended July 1, 2024); GA4's Data API v1 is the only live
  reporting surface. No UA fallback path exists or is built.
- **Read-only scope.** Everything below runs on
  `https://www.googleapis.com/auth/analytics.readonly`. Requesting only this
  scope also minimizes the Google verification burden (§3).

### API surface wrapped (all official, verified)

| Verb | API | Endpoint | Why |
|---|---|---|---|
| `property list` | Admin API v1beta | `GET https://analyticsadmin.googleapis.com/v1beta/accountSummaries` | Every Data API call needs a numeric property id; accountSummaries returns all accounts + their property summaries in one paginated call — the discovery step an agent must run first. |
| `report run` | Data API v1beta | `POST https://analyticsdata.googleapis.com/v1beta/properties/{id}:runReport` | The core reporting verb: dimensions × metrics over date ranges, filters, ordering, pagination. |
| `report realtime` | Data API v1beta | `POST https://analyticsdata.googleapis.com/v1beta/properties/{id}:runRealtimeReport` | "Who's on the site right now" — last 30 min (60 for GA 360). |
| `report metadata` | Data API v1beta | `GET https://analyticsdata.googleapis.com/v1beta/properties/{id}/metadata` | Lists the valid dimension/metric API names (incl. custom definitions) for a property — agents need this to construct valid `report run` calls instead of guessing names. |

Deliberately **not** wrapped in v1: `batchRunReports` (an agent loops `report
run`), `runPivotReport` (post-processable from `runReport` output),
`checkCompatibility` (metadata + a failed run's explicit error message cover
it), audience exports (alpha-adjacent, niche), all Admin API write surfaces
(would force `analytics.edit`, a second sensitive scope with no teammate use
case). Docs refs:
- Data API: https://developers.google.com/analytics/devguides/reporting/data/v1/rest
- Admin API: https://developers.google.com/analytics/devguides/config/admin/v1/rest

Requirement that bites: both `analyticsdata.googleapis.com` and
`analyticsadmin.googleapis.com` must be **enabled on the lane-1 dev app's
Google Cloud project**, or every call 403s regardless of token scopes.

## 2. anycli definition

**Stage-1 rubric: `service` type.** No official Google Analytics CLI binary
exists (gcloud does not front GA4 reporting). Service implementation against
the two REST APIs, exactly like the nine existing Google Workspace tools.

- Definition: `definitions/tools/google-analytics.json`

```json
{
  "name": "google-analytics",
  "type": "service",
  "description": "Google Analytics (GA4) reporting as a tool (user access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "GOOGLE_ANALYTICS_ACCESS_TOKEN"}
      }
    ]
  }
}
```

- Service package: `internal/tools/googleanalytics/` (dashes dropped per
  master plan §3 Go-package rule; precedent `microsoft-calendar` →
  `microsoftcalendar`). Registered in `internal/tools/register.go` as
  `RegisterService("google-analytics", &googleanalytics.Service{})` —
  registration rides the **batch-end** merge; the definition JSON and the
  package merge freely mid-batch.
- Struct shape copies the `calendar`/`notion` precedent: `BaseURL` overrides
  for **both** API hosts (`DataBaseURL`, `AdminBaseURL` — two upstreams, so
  two override fields), `HC`, `Out`/`Err`, injected `sleep` for retry tests.
- Exit-code contract (notion precedent): 0 success · 1 runtime/API failure ·
  2 usage/parse. 401/403 append the reconnect scope hint (calendar precedent)
  and 401 classifies via `execution.RejectCredential` so the host can
  invalidate cached tokens.

### Cobra tree

```
google-analytics
├── property
│   └── list [--page-size N] [--page-token T] [--json]
└── report
    ├── run --property <id> --metrics m1,m2 [--dimensions d1,d2]
    │       [--start-date 28daysAgo] [--end-date today]
    │       [--filter dim==value ...] [--filter-json '<FilterExpression>']
    │       [--order-by metric:m1:desc|dimension:d1:asc ...]
    │       [--limit N] [--offset N] [--json]
    ├── realtime --property <id> --metrics m1[,m2] [--dimensions ...]
    │       [--minutes-ago N] [--json]
    └── metadata --property <id> [--kind dimensions|metrics|all]
            [--search substr] [--json]
```

Conventions:
- `--property` accepts `123456` or `properties/123456` (normalized).
- Date values pass **native API forms verbatim** (`YYYY-MM-DD`, `NdaysAgo`,
  `yesterday`, `today`); defaults `28daysAgo`..`today` — the calendar
  precedent of passing native API values through, not inventing a dialect.
- `--filter` is repeatable sugar for ANDed string-equality dimension filters;
  `--filter-json` passes a raw Data API `FilterExpression` for everything
  else. Mutually exclusive; both optional.
- `--json` emits the provider response body via the shared strict-JSON
  `emit`/`sanitizeJSON` pattern (always-parseable stdout); default output is a
  compact human-readable table (dimension columns then metric columns), since
  report responses' header/rows split is hostile to eyeballs.
- No interactive prompts anywhere (repo rule).

## 3. Credentials & auth flow (`oauth_review` — verified)

**Audit note:** `oauth-audit.md` has no Google Analytics row — by design: the
audit's scope was the 250 pre-audit `api_key`-lane tools, and GA has been
`oauth_review` in the catalog from the start (master plan §4 notes: Google
sensitive/restricted scopes). Verified independently against official docs:

- **Registration model:** self-serve OAuth client in the Google Cloud console;
  one multi-tenant web-application client serves all Helio users. Dev/test
  mode works pre-review (unverified-app warning + user cap), so **L4 is not
  gated on review** — matching the lane-1 model.
- **Review gate:** `analytics.readonly` is a **sensitive** scope → Google
  sensitive-scope verification (brand verification, scope justification, demo
  video) before arbitrary external accounts can consent cleanly
  (https://developers.google.com/identity/protocols/oauth2/production-readiness/sensitive-scope-verification).
  **Clarification vs the catalog's blanket note, not a divergence:** it is
  *not* a restricted scope, so **no CASA/third-party security assessment** —
  this review is the light end of `oauth_review` (typically days, not the
  weeks-long restricted-scope clock). Lane stays `oauth_review`; only the
  expected review latency is better than the Google-restricted worst case.
- **Token semantics:** standard Google OAuth2 — authorize
  `https://accounts.google.com/o/oauth2/v2/auth` with
  `access_type=offline&prompt=consent` (refresh token issuance), token
  exchange `https://oauth2.googleapis.com/token` (`form_secret`), ~1 h access
  tokens, long-lived refresh token, revoke at
  `https://oauth2.googleapis.com/revoke`. Identical to the shipped
  `google_calendar` bundle; `standard_oauth` covers it — **no service
  adapter**.
- **Credential fields** (bundle `credential.fields`, same as all google_*):
  `access_token: token.access_token`, `account_key: connection.account_key`.
  Identity from `https://openidconnect.googleapis.com/v1/userinfo` `/sub`.

## 4. Helio provider bundle plan

`integrations/providers/google_analytics/provider.yaml`, modeled byte-for-byte
on `google_calendar` except scopes/naming:

- **Axes (master plan §3):** ① CLI command word `analytics` under
  `tool.group: google` → `heliox tool google analytics`; ② anycli id
  `google-analytics` (`tool.name`); ③ provider key `google_analytics`
  (directory + `key:`). ②↔③ is a mechanical dash↔underscore pair → one
  `toolToProvider` entry `"google-analytics": "google_analytics"` in
  `helio-cli/internal/toolcred/resolver.go` — **unless** master-plan open
  question 1 (mechanical normalization in `ProviderFor`/`ToolFor`) lands
  first, in which case no entry is needed. Batch lead confirms at merge.
- `go_name: GoogleAnalytics`; `experiment: google_tools` (family precedent —
  every non-gmail google_* bundle carries it; batch lead may drop it if the
  family gate is retired).
- `presentation`: name "Google Analytics", `visible: false` (hidden-first —
  flip is a separate go-live change gated on L5 **and** review clearance),
  `order`: next free slot after the google block (calendar=31…), fixed at
  batch-end regen.
- `auth`: `type: oauth`, `owner: individual`,
  `required_config_fields: [oauth.client_id, oauth.client_secret]`; oauth
  block identical to google_calendar (`form_secret`, `pkce: none`,
  `authorize_params: {access_type: offline, prompt: consent}`, google revoke
  block) with scopes
  `[openid, email, profile, https://www.googleapis.com/auth/analytics.readonly]`
  and `display_scopes: [openid, email, profile, analytics.readonly]`.
- `identity`: `source: userinfo`, google userinfo URL, `stable_key: /sub`,
  `label_candidates: [/email, /name, /sub]`.
- `connection`: `mode: isolated`, `disconnect_mode: provider_revoke`,
  `runtime_strategy: standard_oauth` (zero integration-service Go).
- `tool`: `name: google-analytics`, `command: analytics`, `group: google`,
  `kind: oauth` (microsoft `outlook` ↔ `microsoft-outlook` is the
  command≠name precedent).
- Not in the bundle (per rules): client id/secret (lane 1 lands them in
  `config/` + `deploy/` Helm Secret together, before L5), icon
  (`ui/helio-app/src/integrations/icons/google_analytics.svg` +
  `providerIcons.ts`, batch-end), AI docs
  (`agents/plugins/heliox/skills/tool/google/` sub-doc extension, batch-end
  publish).
- **provider-gen is run locally for L3/L4 validation only; regenerated
  projections are NOT committed on this branch** (master plan §2 — branch CI
  is expected red on `provider-gen --check` until batch-end). helio-cli builds
  against this anycli worktree via a **local, uncommitted** `go.mod replace`.

## 5. Test plan (five layers)

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | anycli unit tests, TDD-first, in `internal/tools/googleanalytics/`: httptest fakes for both hosts asserting request shape (URL/property normalization, runReport body incl. dateRanges/filters/orderBys, Bearer header from `GOOGLE_ANALYTICS_ACCESS_TOKEN`), pagination, table + `--json` rendering, error envelope, exit codes 0/1/2, 401 `RejectCredential` + scope hint. `go test ./...` green before any real API call. | None |
| L2 | Dev harness against the **real** APIs: `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli google-analytics -- property list`, then `report run/realtime/metadata` against the pool property. Mandatory before the pin bump. | **Yes** — lane-2 test Google account with a GA4 property receiving traffic, plus a `analytics.readonly` user token minted from the lane-1 dev client (or OAuth Playground pre-registration). Both APIs enabled on the dev project. |
| L3 | Local `provider-gen` + `provider-gen --check` against the branch bundle (not committed); anycli + helio-cli + integration-service unit suites; `toolToProvider` round-trip test. | None |
| L4 | Singleton + `POST /internal/test-only/connections/seed` (provider `google_analytics`, real identities) seeding `access_token` + `refresh_token` with a short `expires_at` to force the refresh-and-write-back path; then `heliox tool google analytics -- report run …` returning real GA data through the token gateway. helio-cli built with the local `replace`. | **Yes** — lane-1 dev client id/secret as uncommitted local `config/cloud.yaml` entries + real access/refresh tokens from the test account. |
| L5 | Human lane 3, post batch-end merge, still hidden: `heliox tool google analytics auth` → real Google consent → `oauth_connected` event on the channel → unseeded live `report run`. Gates the visible flip together with sensitive-scope review clearance. | **Yes** — human consent session on the pool Google account; lane-1 config appends landed in `config/` + `deploy/`. |

**Definition of done** follows the master plan §2: this branch delivers
code-complete-hidden (L1–L4 proven on-branch); batch-end merge, L5 sweep, docs
publish, icon, and the review-cleared visible flip complete it.
