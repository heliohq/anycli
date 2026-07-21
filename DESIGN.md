# Google Search Console — per-tool design (catalog row 264)

Scratch design file on branch `tool/search-console`; the batch lead strips it at
batch-end. Master plan: Helio `docs/design/008-300-integrations-rollout-plan.md`
(anycli repo), §2 execution model, §3 naming, row 264. Pipeline: Helio
`.claude/skills/helio-tool-provider/SKILL.md`.

## 0. Naming (the three axes)

| Axis | Value | Where |
|---|---|---|
| ① CLI command word | `search-console` → `heliox tool google search-console` | bundle `tool.command` (group `google`, design 303) |
| ② anycli tool id | `search-console` | `definitions/tools/search-console.json` |
| ③ provider catalog key | `google_search_console` | `integrations/providers/google_search_console/` |

- Go package (§3 rule, dashes dropped): `internal/tools/searchconsole/`.
- ②↔③ divergence: `"search-console": "google_search_console"` — the plan's
  **only non-mechanical** mapping. It goes in
  `helio-cli/internal/toolcred/resolver.go` `toolToProvider` at the batch-end
  merge. Verified 2026-07-21: OQ1 (mechanical dash→underscore normalization) is
  **not implemented** — `ProviderFor` still consults the explicit map only — and
  even if OQ1 lands, this entry stays: the mapping adds the `google_` prefix, so
  it is a true irregular either way.
- `toolGroups` (generated in `providers_gen.go`): the bundle's
  `tool.group: google` + `tool.command: search-console` render it under the
  `google` group; generation is batch-end (do not commit local regens).

## 1. Auth-lane verification (independent check of the catalog)

Catalog row 264 says `oauth_review`. The 2026-07-21 `oauth-audit.md` has **no
row for search-console** — that audit's scope was only the 250 tools sitting in
the `api_key` lane pre-audit; Search Console was assigned `oauth_review` in the
catalog from the start (§4 notes: Google sensitive scopes). Verified against
official docs:

- **Registration model:** one multi-tenant OAuth 2.0 app in the Google Cloud
  console (same Google app family/registration lane 1 already runs for
  `google_calendar` etc.). Self-serve client creation; dev/test-mode works
  pre-verification (100-user cap + "unverified app" warning), so lane-1 dev-app
  creation gates L4 exactly as the plan describes.
- **Protocol:** OAuth 2.0 is the *only* supported protocol for private data
  (`developers.google.com/webmaster-tools/v1/how-tos/authorizing`). The
  separate "Testing Tools API" API-key exception covers public data only and is
  not wrapped by this tool.
- **Scopes:** exactly two — `https://www.googleapis.com/auth/webmasters`
  (read/write) and `https://www.googleapis.com/auth/webmasters.readonly`.
  Neither is **restricted** (no CASA; the restricted list covers
  Gmail/Drive/Fit/Chat/Health/etc. — `support.google.com/cloud/answer/13464325`).
  The read/write `webmasters` scope is **sensitive** → Google sensitive-scope
  verification before external users can grant without caps/warnings. Multiple
  2024+ sources report `webmasters.readonly` was reclassified **non-sensitive**.
- **Verdict:** this tool ships write verbs (sitemap submit/delete), so it
  requests `webmasters` → **`oauth_review` confirmed**. Recorded divergence
  option: a readonly-only variant (drop sitemap submit/delete) would likely
  qualify for `oauth_light` under the audit rubric. Not chosen — submitting a
  sitemap is a core AI-teammate action and the review rides the same Google
  verification submission lane 1 already runs for the other sensitive-scope
  Google tools (Analytics, Ads, YouTube). If the batch's Google verification
  stalls, the fallback is a catalog amendment, not a silent scope downgrade.
- **Token semantics:** standard Google tokens — ~1 h access token, long-lived
  refresh token when `access_type=offline` is requested; `prompt=consent`
  forces refresh-token re-issue on reconnect; refresh tokens are *not* rotated
  on use (no single-active-token lease); revoke via
  `https://oauth2.googleapis.com/revoke`. Identical to the shipped
  `google_calendar` bundle — `standard_oauth`, zero service-side adapter code.

## 2. What an AI teammate does with Search Console → API surface

Driving use cases: "how did our site do in search last month", "which queries /
pages dropped after the deploy", "is the new page indexed, and why not", "submit
the new sitemap", "which properties do we have and are the sitemaps healthy".

Wrapped surface (all verified against
`developers.google.com/webmaster-tools/v1/api_reference_index`):

| Verb | Method + path | Why |
|---|---|---|
| query | `POST https://www.googleapis.com/webmasters/v3/sites/{siteUrl}/searchAnalytics/query` | The core value: clicks/impressions/CTR/position by query, page, country, device, date, searchAppearance. |
| sites list | `GET .../webmasters/v3/sites` | Property discovery — every other verb needs a `siteUrl`; also shows permission level. |
| sites get | `GET .../webmasters/v3/sites/{siteUrl}` | Confirm access level for one property. |
| sitemaps list | `GET .../sites/{siteUrl}/sitemaps` | Sitemap health (errors/warnings, last downloaded). |
| sitemaps get | `GET .../sites/{siteUrl}/sitemaps/{feedpath}` | One sitemap's status detail. |
| sitemaps submit | `PUT .../sites/{siteUrl}/sitemaps/{feedpath}` | "Submit the new sitemap" — the tool's key write action. |
| sitemaps delete | `DELETE .../sites/{siteUrl}/sitemaps/{feedpath}` | Remove a stale sitemap. |
| inspect | `POST https://searchconsole.googleapis.com/v1/urlInspection/index:inspect` | "Why isn't this page indexed" — index status, mobile/AMP/rich-result verdicts. |

Deliberately **excluded**: `sites.add` / `sites.delete` (adding a property is
useless without out-of-band site verification, and deleting a property from the
account is a destructive account-management action out of proportion to agent
value — same "safe verbs only" gradient as the calendar tool); the URL Testing
Tools API (separate legacy API-key surface); the deprecated `searchType` field
(use `type`).

Two base URLs are a fact of this API (webmasters v3 + searchconsole v1). The
service takes **two** overridable base-URL fields (`BaseURL`, `InspectBaseURL`)
so httptest can fake both.

API facts the implementation must honor (verified on the searchanalytics.query
reference):

- `siteUrl` is a **path segment and must be URL-escaped** (`https://example.com/`
  → `https%3A%2F%2Fexample.com%2F`); Domain properties use the literal
  `sc-domain:example.com` form. Same for the sitemap `feedpath` segment.
- query body: required `startDate`/`endDate` (YYYY-MM-DD, America/Los_Angeles,
  inclusive); `dimensions[]` ∈ {query, page, country, device, date, hour,
  searchAppearance}; `type` ∈ {web (default), image, video, news, discover,
  googleNews}; `dimensionFilterGroups` (groupType `and` only; operators equals |
  notEquals | contains | notContains | includingRegex | excludingRegex, RE2);
  `rowLimit` 1–25000 (default 1000); `startRow` ≥ 0; `dataState` ∈ {final
  (default), all, hourly_all}; `aggregationType` ∈ {auto, byPage, byProperty,
  byNewsShowcasePanel}. Response rows: `keys[]`, `clicks`, `impressions`,
  `ctr` (0–1), `position`; plus `responseAggregationType` and freshness
  `metadata`. Data window ~16 months; top-rows-only coverage caveat.
- inspect body: `inspectionUrl` + `siteUrl` required, `languageCode` optional;
  only the indexed version is reported (no live test). Per-property quota
  (limits doc): 2 000 inspections/day, 600/min — surface Google's 429 verbatim,
  no client-side throttling.

## 3. anycli definition and service

**Stage-1 form decision: `service` type.** No official Google Search Console
CLI binary exists (gcloud does not cover GSC; the rubric's cli-type conditions
fail at the first test). Matches 21-of-23 precedent.

`definitions/tools/search-console.json`:

```json
{
  "name": "search-console",
  "type": "service",
  "description": "Google Search Console as a tool (user access token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "SEARCH_CONSOLE_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Single credential field `access_token`, matching the bundle's
`credential.fields` and the calendar precedent (`CALENDAR_ACCESS_TOKEN`).
Registered in `internal/tools/register.go` as
`RegisterService("search-console", &searchconsole.Service{})` — registration
rides the **batch-end** merge; the definition JSON + service package merge
freely mid-batch.

**Service shape** (`internal/tools/searchconsole/`, copying the
`calendar`/`notion` skeleton): `Service{BaseURL, InspectBaseURL, HC, Out, Err}`;
`Execute(ctx, args, env)` fails fast when `SEARCH_CONSOLE_ACCESS_TOKEN` is
unset; cobra root `search-console` with persistent `--json`; runnable group
commands (unknown subcommand = failure, not help-with-exit-0); exit codes 0
success / 1 runtime-API (typed apiError) / 2 usage; 401/403 append the
reconnect scope hint; `--json` errors render the structured error envelope.

Command tree and flags:

```
search-console sites list
search-console sites get      --site <siteUrl>
search-console sitemaps list  --site <siteUrl>
search-console sitemaps get   --site <siteUrl> --sitemap <feedUrl>
search-console sitemaps submit --site <siteUrl> --sitemap <feedUrl>
search-console sitemaps delete --site <siteUrl> --sitemap <feedUrl>
search-console query          --site <siteUrl> --start <YYYY-MM-DD> --end <YYYY-MM-DD>
                              [--days N]                # convenience: last N days (PT), mutually exclusive with --start/--end
                              [--dimensions query,page,...] [--type web|image|video|news|discover|googleNews]
                              [--filter dim:op:expr]... [--row-limit N] [--start-row N]
                              [--data-state final|all|hourly_all]
                              [--aggregation auto|byPage|byProperty|byNewsShowcasePanel]
search-console inspect        --site <siteUrl> --url <pageUrl> [--language <bcp47>]
```

`--site` accepts both URL-prefix (`https://example.com/`) and
`sc-domain:example.com` forms verbatim; the service owns path escaping.
`--filter` is repeatable `dimension:operator:expression` (one AND group —
the API supports nothing else). Flag values pass native API enum values
through verbatim (calendar precedent: no re-invented vocabularies).

**JSON output shape** (`--json`): the provider response body normalized, not
re-modeled — `sites list` → `{"sites":[...]}` (siteEntry rows: `siteUrl`,
`permissionLevel`); `sitemaps list` → `{"sitemaps":[...]}`; `query` →
`{"rows":[{"keys":[...],"clicks":..,"impressions":..,"ctr":..,"position":..}],
"responseAggregationType":"...", "metadata":{...}}` (metadata passed through
when present); `inspect` → `{"inspectionResult":{...}}`; mutations
(`sitemaps submit|delete` return empty bodies) → `{"ok":true,"site":...,
"sitemap":...}`. Default (no `--json`) is a compact human summary; query rows
render as aligned columns with ctr as a percentage.

## 4. Helio provider bundle plan

`integrations/providers/google_search_console/provider.yaml` — clones the
shipped `google_calendar` bundle (the whole Google family is `standard_oauth`
with userinfo identity), differing only in key/name/order/scopes/tool block:

- `schema: helio.provider/v1`, `key: google_search_console`,
  `go_name: GoogleSearchConsole`.
- `experiment: google_tools` — every google-group tool carries the same
  design-090 flag; batch lead confirms the flag choice at merge.
- `presentation`: name "Google Search Console", `consent_domain:
  accounts.google.com`, **`visible: false`** (hidden-first; flip is a separate
  go-live change gated on L5 + Google review clearance), `order`: next free
  slot in the google block (batch-end decision).
- `auth`: `type: oauth`, `owner: individual`, `required_config_fields:
  [oauth.client_id, oauth.client_secret]`; oauth block identical to
  google_calendar (`authorize_url` accounts.google.com/o/oauth2/v2/auth,
  `token_url` oauth2.googleapis.com/token, `token_exchange_style: form_secret`,
  `pkce: none`, `authorize_params: {access_type: offline, prompt: consent}`,
  `single_active_token: false`, `refresh_lease: none`, revoke via
  oauth2.googleapis.com/revoke with refresh-token-first fallback) — scopes:

  ```yaml
  scopes:
    - openid
    - email
    - profile
    - https://www.googleapis.com/auth/webmasters
  display_scopes: [openid, email, profile, webmasters]
  ```

  (`webmasters` supersedes `webmasters.readonly`; requesting both is
  redundant.)
- `identity`: `source: userinfo`, url
  `https://openidconnect.googleapis.com/v1/userinfo`, `stable_key: /sub`,
  `label_candidates: [/email, /name, /sub]`.
- `connection`: `mode: isolated`, `disconnect_mode: provider_revoke`,
  `runtime_strategy: standard_oauth` — **zero integration-service code**; no
  adapter (nothing about this provider leaves the standard Google shape).
- `resources`: all `none`.
- `credential.fields`: `access_token: token.access_token`, `account_key:
  connection.account_key`.
- `tool`: `name: search-console`, `command: search-console`, `group: google`,
  `kind: oauth`.

Because Google verification is per-app-per-scope-set, adding the `webmasters`
scope to the existing Helio Google OAuth app (vs a separate client) is a lane-1
call — the bundle is agnostic (it names only config field slots); flag to the
batch lead that the verification submission must include this scope.

Helio-side companions (batch-end unless noted): resolver entry (§0); the five
provider-gen projections (single batch-end regen — never committed from this
branch); icon `ui/helio-app/src/integrations/icons/google_search_console.svg` +
manual `providerIcons.ts` registration; AI-facing sub-doc under
`agents/plugins/heliox/skills/tool/` + one plugin version bump per batch;
lane-1's `oauth.client_id`/`client_secret` config appends to `config/` +
`deploy/` Helm Secret together (Config Sync), landing before L5.

## 5. Test plan (the five layers)

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | anycli unit tests, httptest fakes for BOTH bases: assert Bearer header injection; **siteUrl/feedpath path escaping** (URL-prefix and `sc-domain:` forms); query body construction (dates, dimensions, filters → dimensionFilterGroups, rowLimit/startRow/dataState/aggregation); inspect body; sitemaps submit PUT with empty-body 204 handling; 401/403 scope-hint rendering; `--json` vs plain output; exit codes 0/1/2; unknown-subcommand failure. TDD: tests first. | None |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli search-console -- sites list`, then `query --site … --days 28 --dimensions query`, `sitemaps list/submit/delete` (submit+delete a scratch sitemap to keep the property clean), `inspect` on a known-indexed URL. Mandatory before the pin bump. | **Yes** — lane 1/2: a Google account holding a verified Search Console property with ≥ a few days of data, and a `webmasters`-scoped access token minted from the dev-mode app (OAuth playground against the dev client works). |
| L3 | Local-only `provider-gen` + `provider-gen --check` against the branch bundle (regens NOT committed; branch expectedly red in CI on this check until batch-end); helio-cli built with a **locally uncommitted** `go.mod` `replace github.com/heliohq/anycli => ../../../anycli/.claude/worktrees/tool-search-console`; `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`; integration-service unit suite. | None |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with `provider: google_search_console`, real `access_token` **and** `refresh_token`, short `expires_at` to force the refresh-and-write-back path; then `heliox tool google search-console -- query --site …` must return live data. Requires lane-1 dev client id/secret as uncommitted local `config/cloud.yaml` entries. | **Yes** — dev-app client id/secret (lane 1) + real token pair from the L2 test account; real seeded org/user/assistant ids. |
| L5 | Human-in-the-loop (lane 3): `heliox tool google search-console auth` → connect link → real Google consent → `oauth_connected` event → unseeded live run. Runs after batch-end merge + lane-1 config landing; the **visible flip** additionally waits on Google sensitive-scope verification clearance (oauth_review). | **Yes** — human consent session on the pooled test account; verified dev app. |

Definition of done follows the master plan §2: all five layers green, docs
published, icon registered, then `visible: true` + regenerate as the single
go-live change.

## 6. Recorded divergences / open flags

1. **oauth-audit.md contains no search-console row** — not a divergence: the
   audit scoped only pre-audit api_key-lane tools. Lane source is the catalog
   itself; independently re-verified here (§1) and upheld.
2. **`webmasters.readonly` is non-sensitive (2024 reclassification)** — a
   readonly-only tool could plausibly ride `oauth_light`. Chosen: keep write
   verbs + `webmasters` + `oauth_review` (§1). Revisit only if Google review
   stalls.
3. **Sensitive scope, not restricted** — Google *verification*, no CASA
   security assessment; cheaper end of the review lane.
4. **Shared vs separate Google OAuth app** for the `webmasters` scope
   addition — lane-1/batch-lead decision (§4).
5. **`experiment: google_tools`** inherited from family precedent — batch lead
   confirms the gating flag at merge.
