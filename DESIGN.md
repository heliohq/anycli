# Tool design: Ahrefs

**Catalog row:** 266 | anycli id `ahrefs` (axis ②) | provider key `ahrefs` (axis ③) | CLI word `ahrefs` (axis ①, flat — no group) | lane `oauth_review` | batch **3-hold** | category SEO & Web Data.

Scratch per-tool design for the `helio-tool-provider` pipeline. Lives on branch `tool/ahrefs`; the batch lead strips it at batch end.

## 1. Independent verification of the catalog / audit (official docs)

Sources read directly (2026-07-21):

- API v3 intro: <https://docs.ahrefs.com/api/docs/introduction.md>
- OpenAPI spec index: <https://docs.ahrefs.com/reference/index.json> (+ per-tool specs `site-explorer.json`, `keywords-explorer.json`, `serp-overview.json`, `batch-analysis.json`, `subscription-info.json`)
- Ahrefs Connect OAuth guide: <https://docs.ahrefs.com/ahrefs-connect/docs/oauth-guide.md>
- Connect approval process: <https://docs.ahrefs.com/ahrefs-connect/docs/approval-process.md>
- Connect application: <https://docs.ahrefs.com/ahrefs-connect/docs/how-to-apply.md>
- Connect API guide: <https://docs.ahrefs.com/ahrefs-connect/docs/api-guide.md>
- Plan availability: <https://help.ahrefs.com/en/articles/6559232-about-api-v3>

**Auth-lane verdict: `oauth_review` is CONFIRMED for the Ahrefs Connect path**, matching the 2026-07-21 OAuth audit line for row 266 fact-for-fact:

- OAuth 2.0 Authorization Code + PKCE (S256 mandatory params `code_challenge` / `code_challenge_method=s256`).
- Authorize: `https://app.ahrefs.com/web/oauth/authorize` with `response_type=code`, `client_id`, `scope=apiv3-integration-apps` (single fixed scope), `redirect_uri`, `state`.
- Token exchange: `POST https://ahrefs.com/oauth/token`, `application/x-www-form-urlencoded`, params `grant_type=authorization_code`, `client_id`, `client_secret` (web apps; server-side only), `redirect_uri`, `code`, `code_verifier` → maps to bundle `token_exchange_style: form_secret`.
- Token response: `access_token`, `expires_in`, `token_type: Bearer`, `scope` — **no refresh token, ever**; access tokens live 1 year, then the user must re-authorize. No documented revoke endpoint. No userinfo/identity endpoint.
- **Not self-serve**: HubSpot application form → eligibility review → build in Inactive (dev) mode → verification (connection-flow screenshots, unedited OAuth video, API-usage review, unit-conservation practices) → Active. New products need an **Ahrefs Enterprise subscription** before activation. This is squarely the review-clearance lane; per the master plan, review gates only the visible flip, and dev-mode (Inactive) apps can run the OAuth flow with free test queries — so L4/L5-dev are possible pre-review.

**Divergences from the plan/audit to record (docs are authoritative):**

1. **Direct API keys are no longer Enterprise-only.** The plan's §"Test-account cost and ToS" note ("Ahrefs gates API v3 behind Enterprise plans") is stale: the official help article now states API v3 is available on **Lite and higher** plans (Lite = 100k units/mo, max 100 rows/request), keys self-created in Account settings → API keys by workspace owners/admins, sent as `Authorization: Bearer <key>`. This materially lowers the 3-hold account-procurement risk: the test-account pool needs a Lite subscription, not Enterprise. It also means an **`api_key` re-lane is a viable fallback** if 3-hold pre-verify finds Connect eligibility (application form + Enterprise requirement for activation) unattainable for Helio — the wire surface (`api.ahrefs.com/v3`, Bearer header) is byte-identical for both credential kinds, so the anycli tool is unaffected by the choice; only the Helio bundle would change (`runtime_strategy: manual_credentials`, mongodb-style `credential_input`, verify via the free `subscription-info/limits-and-usage` endpoint). Recommendation: keep the catalog's `oauth_review` lane, surface this option at the 3-hold pre-verify gate.
2. **No refresh tokens** (audit already notes this): the standard seed guidance "seed a short `expires_at` to exercise the refresh path" does not apply — there is no refresh path. Bundle must declare `refresh_lease: none`; L4 seeds `access_token` only (Slack-style). Expiry after 1 year surfaces as a 401 → reconnect, same as a revoked token.
3. **No identity/userinfo endpoint** exists anywhere in the docs, and the token response carries no account identifier — the declarative identity resolver has nothing standard to point at. See §4 for the chosen mitigation (this is the one place Ahrefs falls short of the `standard_oauth` golden path).

## 2. API surface wrapped (what an AI teammate actually does with Ahrefs)

Base: `https://api.ahrefs.com/v3/<tool>/<endpoint>`, `Authorization: Bearer <token>`, `Accept: application/json`. Rows-returning endpoints require `select` (comma-separated field list) and support `where`/`order_by`/`limit`/`offset` filter grammar. Errors are non-2xx with body `{"error": "<string>"}`; 401 invalid/expired token, 403 plan/permission, 429 rate limit (default 60 req/min). Every paid request costs ≥50 API units from the connected account (Connect: the *user's* units), so the CLI must keep `select` minimal and default `limit` low.

An AI teammate uses Ahrefs to answer: "how strong is this domain?", "who links to us / to a competitor?", "what keywords does a site rank for?", "is this keyword worth targeting?", "who ranks for it?", "compare these domains", "how many units do we have left?". That maps to five read-only endpoint groups (28+ Site Explorer endpoints exist; we wrap the high-signal subset, not the catalog):

| Verb | Endpoint(s) | Why |
|---|---|---|
| `domain overview` | `site-explorer/domain-rating` (+`backlinks-stats`, `metrics`) | The #1 ask: DR, backlinks/refdomain counts, traffic + keyword counts for a target. `date` (required) defaults to today. |
| `backlinks list` | `site-explorer/all-backlinks` | Who links to a target; supports `--mode`, `--where`, `--limit`. |
| `backlinks broken` | `site-explorer/broken-backlinks` | Lost-link reclamation workflow. |
| `refdomains` | `site-explorer/refdomains` | Referring-domains view (cheaper than raw backlinks for "who links"). |
| `keywords organic` | `site-explorer/organic-keywords` | What a site ranks for. |
| `pages top` | `site-explorer/top-pages` | Best-performing pages of a target. |
| `competitors` | `site-explorer/organic-competitors` | Competitive set discovery. |
| `keyword overview` | `keywords-explorer/overview` | Volume/KD/CPC etc. for explicit `--keywords` + `--country` (required). |
| `keyword ideas` | `keywords-explorer/matching-terms`, `related-terms`, `search-suggestions` | Keyword research fan-out. |
| `keyword volume-history` | `keywords-explorer/volume-history` | Trend for one keyword. |
| `serp` | `serp-overview/serp-overview` | Top-100 SERP for `--keyword` + `--country`. |
| `batch` | `POST batch-analysis/batch-analysis` | Up to 100 targets in one request — the unit-efficient "compare these domains" path the Connect docs explicitly recommend. |
| `usage` | `subscription-info/limits-and-usage` | **Free** (0 units); plan, unit limits/usage, reset date. Doubles as the connection health/verify probe. |

Deliberately **out**: Site Audit, Rank Tracker, Brand Radar, Web Analytics, GSC Insights, Social Media Management, Management (project CRUD — write-shaped, project-bound, low agent value); crawled-pages/anchors/history long-tail (available later behind the same client with near-zero marginal cost). The tool is read-only end to end — pure GETs plus the read-only batch POST.

## 3. anycli definition and service

**Stage-1 rubric: `service` type.** No official Ahrefs CLI binary exists at all, so the `cli`-type conditions fail at the first test. Service package against the HTTP API, per the 21-of-23 default.

**Definition** `definitions/tools/ahrefs.json` (conflict-free, merges mid-batch; `register.go` entry rides the batch-end merge):

```json
{
  "name": "ahrefs",
  "type": "service",
  "description": "Ahrefs SEO data as a tool (API v3, OAuth/API-key bearer token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "AHREFS_API_TOKEN"}
      }
    ]
  }
}
```

One credential field `access_token` — identical wire semantics whether the token was minted by Ahrefs Connect OAuth or pasted as a personal API key (both are `Authorization: Bearer`), which keeps the anycli side invariant under a possible api_key re-lane (§1 divergence 1).

**Service package** `internal/tools/ahrefs/` (Go package `ahrefs` — no dashes/digits to normalize), copying the bitly/notion shape: `Service{BaseURL, HC, Out, Err}`, `Execute(ctx, args, env)` reading `AHREFS_API_TOKEN`, cobra root with `SilenceUsage/SilenceErrors`, exit codes 0/1/2, typed `apiError` decoding `{"error": string}`, `--json` structured error envelope, provider-JSON passthrough on stdout.

Command tree (resource-grouped like notion; all output raw provider JSON):

```
ahrefs domain overview --target <t> [--date YYYY-MM-DD] [--json]
ahrefs backlinks list|broken --target <t> [--mode <m>] [--select ...] [--where ...] [--limit N] [--offset N] [--order-by ...]
ahrefs refdomains --target <t> [...same row flags]
ahrefs keywords organic --target <t> [...row flags]
ahrefs pages top --target <t> [...row flags]
ahrefs competitors --target <t> [...row flags]
ahrefs keyword overview --keywords k1,k2 --country us [...row flags]
ahrefs keyword ideas --keyword <k> --country us [--kind matching|related|suggestions] [...row flags]
ahrefs keyword volume-history --keyword <k> --country us [--from --to]
ahrefs serp --keyword <k> --country us [--select ...] [--date ...] [--top-positions N]
ahrefs batch --targets a.com,b.com [--select ...]   # POST batch-analysis
ahrefs usage [--json]
```

Design points:

- **Unit safety by default**: every rows endpoint gets a curated default `select` (the fields an agent actually reads) and default `--limit 10`; `--select` overrides for power use. Never default to the full field list — Ahrefs bills per field × row.
- `--where` passes the Ahrefs filter expression string through verbatim (documented grammar: <https://docs.ahrefs.com/api/docs/filter-syntax.md>); the CLI does not invent its own filter DSL.
- `domain overview` fans out to up to three GETs (`domain-rating`, `backlinks-stats`, `metrics`) and merges into one JSON object; `--cheap` flag limits to `domain-rating` only. `date` defaults to today (UTC), since Ahrefs requires it.
- 429 and `{"error":...}` bodies surface verbatim in the error envelope with the HTTP status; no retry loop in v1 (agents can re-invoke).

**JSON output shape:** provider-JSON passthrough (bitly/notion precedent) — Ahrefs already returns clean keyed JSON (`{"domain_rating": {...}}`, `{"backlinks": [...]}`); the merged `domain overview` object and the error envelope `{"error": {"code": ..., "message": ..., "status": ...}}` are the only tool-fabricated shapes.

## 4. Helio provider bundle plan

`integrations/providers/ahrefs/provider.yaml` — held to the batch-end merge; `provider-gen` run locally for validation only, projections **not** committed from this branch.

- Axes: ① `tool.command` unset (flat command `heliox tool ahrefs`, no family/group), ② `tool.name: ahrefs`, ③ key/dir `ahrefs`. ② == ③ → **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.
- `presentation`: name `Ahrefs`, `consent_domain: ahrefs.com`, **`visible: false`** (hidden-first; flip additionally gated on Connect review clearance — oauth_review).
- `auth`: `type: oauth`, `owner: individual` (an Ahrefs workspace seat is a personal/company account, bitly-style, not per-assistant), `required_config_fields: [oauth.client_id, oauth.client_secret]`.
- `auth.oauth`:
  - `authorize_url: https://app.ahrefs.com/web/oauth/authorize`
  - `token_url: https://ahrefs.com/oauth/token`
  - `token_exchange_style: form_secret` (form body + client_secret param — verified §1)
  - `pkce: s256` (mandatory per official guide)
  - fixed wire scope `apiv3-integration-apps` (single all-or-nothing scope); `display_scopes` as capability slugs `[view_seo_metrics, view_backlinks, view_keywords]` (bitly precedent for display-only scopes)
  - `single_active_token: false`, `refresh_lease: none` (no refresh tokens exist — §1 divergence 2)
- `identity` — **the one non-golden-path item** (§1 divergence 3): no userinfo endpoint, no id in the token response. Plan A: `source: userinfo` pointed at the free `https://api.ahrefs.com/v3/subscription-info/limits-and-usage` with `stable_key: /limits_and_usage/subscription` and `label_candidates: [/limits_and_usage/subscription]` — verifies the token live at connect time for 0 units, but the "stable key" is the plan name, so two connected Ahrefs accounts on the same plan under one assistant would collide on `account_key`. Acceptable for v1 (one Ahrefs workspace per assistant is the real usage), documented in the bundle comment. If review of the generic resolver shows the plan-name key is unacceptable, fallback is a narrow `identityResolver`-only adapter — but per the skill, first ask whether the generic capability set should grow (e.g. a `static`/`singleton` account-key mode) instead of forking an adapter.
- `connection`: `mode: isolated`, `disconnect_mode: local_only` (no documented revoke endpoint), `runtime_strategy: standard_oauth`.
- `credential.fields`: `access_token: token.access_token`, `account_key: connection.account_key`.
- `tool`: `name: ahrefs`, `kind: oauth`.
- No `experiment` flag planned (hidden-first suffices); revisit only if partner-account gating is imposed at pre-verify.
- Config: lane 1 lands `ahrefs` client id/secret in integration-service `config/` + Helm Secret in `deploy/` together (Config Sync rule); registration queues during Wave 3 per the 3-hold plan. Until then the provider renders `configured: false`, which is safe hidden.
- Icon: `ui/helio-app/src/integrations/icons/ahrefs.svg` + manual `providerIcons.ts` append (batch-end shared surface). Docs: provider sub-doc under `agents/plugins/heliox/skills/tool/` + one plugin bump per batch.

**If pre-verify re-lanes to api_key** (fallback, §1): same anycli tool untouched; bundle becomes mongodb-style `auth.type: credentials` + `credential_input` (single secret field `api_token`, `setup_url` → Ahrefs "API keys creation and management" doc), `runtime_strategy: manual_credentials`, identity verification via the same free `limits-and-usage` endpoint, `access_token: token.access_token` mapping unchanged.

## 5. Test plan (five layers)

| Layer | What runs for ahrefs | External creds needed? |
|---|---|---|
| **L1** | anycli `go test ./...`: httptest fakes asserting per-endpoint request shape (path, `Authorization: Bearer`, required `target`/`country`/`date`/`keyword` params, default `select` + `limit`, `where` passthrough, batch POST body), `{"error": ...}` → exit 1 + `--json` envelope, 401 wording, usage errors → exit 2. TDD: tests first per anycli AGENTS.md. | No |
| **L2** | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<key> anycli ahrefs -- usage` (free), then `domain overview --target ahrefs.com --cheap`, one `backlinks list --limit 3`, one `keyword overview --keywords seo --country us`, one `serp --keyword seo --country us`, `batch --targets ahrefs.com,example.com` — real `api.ahrefs.com` data back. Prefer free-test-query budget; keep paid calls minimal (each ≥50 units). | **Yes — lane 2**: Ahrefs account (Lite+ per current docs) with an API key. Gated on 3-hold pre-verify account procurement. |
| **L3** | Local-only `provider-gen` + `provider-gen --check` against the branch bundle (projections NOT committed); helio-cli build + `go test ./cmd/heliox/cmds/tool/` with uncommitted `go.mod` `replace github.com/heliohq/anycli => ../../../anycli/.claude/worktrees/tool-ahrefs`; integration-service unit suite. Branch CI is expected red on `--check` until batch-end regen. | No |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with `provider: "ahrefs"`, `access_token` = a real token, **no** `refresh_token`/short `expires_at` (no refresh cycle exists — Slack-style seed); then `heliox tool ahrefs -- usage` and one paid call through the real token gateway. | **Yes**: a real token. Personal API key works wire-identically for the data plane; an OAuth-minted token from the lane-1 dev (Inactive) Connect app is preferred once registered. Lane-1 dev app gates the OAuth-flavored L4. |
| **L5** | Human-in-the-loop (oauth lane): `heliox tool ahrefs auth` → connect link → Ahrefs consent (`app.ahrefs.com`) → `oauth_connected` event on the channel → one unseeded live run. Runs against the hidden provider in the per-batch sweep; the **visible flip additionally waits for Connect review clearance** (Enterprise + verification — approval-process materials are a lane-1 deliverable, incl. the unedited OAuth video). | **Yes — lanes 1+2+3**: registered Connect app (dev creds in local uncommitted `config/cloud.yaml`, then landed in config/deploy by lane 1), real Ahrefs account, human consent. |

Definition of done follows the master plan §2: L1–L5 green + docs published + icon registered + visible flip (flip gated on Connect review for this tool).

## 6. Open items for the batch lead / 3-hold pre-verify

1. Confirm Helio's own eligibility for Ahrefs Connect (application form; Enterprise subscription required at activation) — if unattainable, take the documented api_key fallback (§4) via the catalog-amendment mechanism; lane stays as-catalogued until then.
2. Decide the identity plan-name `stable_key` caveat (§4): accept for v1 vs grow a generic account-key mode in `standard_oauth`.
3. Update the plan's stale "Enterprise-only API v3" risk note (§1 divergence 1) when the 3-hold pre-verify record is written.
