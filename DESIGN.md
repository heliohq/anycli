# Tool design: Semrush (`semrush`)

Scratch per-tool design for the 300-integrations rollout (master plan row 265;
3-hold batch, SEO & Web Data). Batch lead strips this file at batch-end.

- anycli tool id (axis ②): `semrush`
- provider catalog key (axis ③): `semrush`
- CLI command word (axis ①): `semrush` (flat command, no group; id == key ==
  command, so **no** `toolToProvider` entry is needed)
- Go package: `internal/tools/semrush/`
- Auth lane: `api_key` (confirmed against official docs — see §2)
- Branches: anycli `tool/semrush`, Helio `tool/semrush`

## 1. Official API surface and what we wrap

Source of truth: https://developer.semrush.com (verified 2026-07-21).

Semrush ships several API families with **different subscriptions and
version-scoped keys** (release notes, 2026-07-15: "v3 API keys can be used
only with Version 3 APIs; v4 API keys can be used only with Version 4 APIs"):

| Family | Version | Auth | Format | Subscription |
|---|---|---|---|---|
| SEO API — overview/domain/keyword/url reports | v3 (`GET https://api.semrush.com/`) | `key=` query param (v3 key) | semicolon CSV | Business + paid API-units add-on |
| SEO API — backlinks reports | v3 (`GET https://api.semrush.com/analytics/v1/`) | `key=` query param | semicolon CSV | same |
| SEO API — Backlinks v4 (2026-06-11), Keyword v4 (2026-06-09) | v4 (`api.semrush.com/apis/v4/…`) | `Authorization: Apikey <v4 key>` header or `key=` | JSON | same |
| Projects API | v3 (deprecated) / v4 (2026-07-02) | key | JSON | same |
| Trends API | v3 | key | CSV | separate Trends subscription |
| Local API (Listing Mgmt, Map Rank Tracker) | v4 | Apikey header / OAuth | JSON | Local plans |
| Balance check | `GET https://www.semrush.com/users/countapiunits.html?key=<v3 key>` | `key=` query | plain text number (e.g. `1,000`) | free, 0 units |

**What an AI teammate actually does with Semrush**: competitive SEO research —
"what does competitor X rank for", "estimate a domain's organic traffic",
keyword research (volume / CPC / difficulty / related / questions), and
backlink profile analysis. That is exactly the SEO API report families:
overview + domain + keyword + url + backlinks. Rank tracking (Projects),
listing management (Local), and clickstream traffic (Trends) are separate
products/subscriptions and are **out of scope for v1**.

**v3 vs v4 decision (recorded divergence).** The official docs recommend the
v4 Backlinks/Keyword APIs for new integrations, but (a) v4 has **no Domain
API** — domain/overview reports, the tool's core value, are v3-only and NOT
deprecated; and (b) keys are strictly version-scoped, while Helio's manual
credential storage is single-secret (design 317 D5: exactly one required
input field stored in the token payload) — we cannot collect a v3 *and* a v4
key in one connection. Therefore **v1 wraps the v3 SEO API end-to-end with
the single v3 API key**, including the v3 keyword and backlinks reports
(deprecated 2026-06 but "remain operational temporarily" per official release
notes). Migration to v4 (backlinks/keyword, and domain when Semrush ships a
Domain v4) is an explicit v2 follow-up that requires a reconnect with a v4
key; the 3-hold **pre-verify gate must re-confirm the v3 keyword/backlinks
families are still operational** at build time — if Semrush has shut them
down by then, v1 scope shrinks to overview/domain/url reports and the v4-key
question escalates to the batch lead.

### Report types wrapped (v1)

All `GET https://api.semrush.com/` unless noted; costs are API units/line
from official docs. Exact `type=` strings re-verified in L2.

| Subcommand | `type=` | Units/line |
|---|---|---|
| `domain overview <domain>` | `domain_rank` (one db); `--all-databases` → `domain_ranks` | 10 |
| `domain history <domain>` | `domain_rank_history` | 10 |
| `domain organic <domain>` | `domain_organic` | 10 |
| `domain paid <domain>` | `domain_adwords` | 20 |
| `domain competitors <domain>` (`--paid`) | `domain_organic_organic` / `domain_adwords_adwords` | 40 |
| `domain pages <domain>` | `domain_organic_unique` | 10 |
| `keyword overview <phrase>` | `phrase_this` (one db); `--all-databases` → `phrase_all` | 10 |
| `keyword batch <phrase>... --database` | `phrase_these` | 10 |
| `keyword related <phrase>` | `phrase_related` | 40 |
| `keyword broad <phrase>` | `phrase_fullsearch` | 20 |
| `keyword questions <phrase>` | `phrase_questions` | 40 |
| `keyword difficulty <phrase>...` | `phrase_kdi` | 50 |
| `keyword organic-results <phrase>` | `phrase_organic` | 10 |
| `keyword paid-results <phrase>` | `phrase_adwords` | 20 |
| `url organic <url>` / `url paid <url>` | `url_organic` / `url_adwords` | 10 / 20 |
| `backlinks overview <target>` | `backlinks_overview` (base `/analytics/v1/`) | 40/request |
| `backlinks list <target>` | `backlinks` (base `/analytics/v1/`) | 40 |
| `backlinks refdomains <target>` | `backlinks_refdomains` (base `/analytics/v1/`) | 40 |
| `backlinks anchors <target>` | `backlinks_anchors` (base `/analytics/v1/`) | 40 |
| `backlinks pages <target>` | `backlinks_pages` (base `/analytics/v1/`) | 40 |
| `backlinks competitors <target>` | `backlinks_competitors` (base `/analytics/v1/`) | 40 |
| `units` | `countapiunits.html` (host `www.semrush.com`) | free |

Deferred (possible later verbs, not v1): `domain compare` (`domain_domains` —
awkward `<sign>|<type>|<domain>` encoding), `rank` / `rank_difference`
(Semrush Rank / Winners-Losers — rarely what a teammate needs), historical
`display_date` variants beyond pass-through, PLA/shopping reports.

## 2. Auth flow (verified against official docs)

- **Registration model**: no app registration, no OAuth client, no review.
  The v3 API key is issued per Semrush account (Subscription info → API
  units). API access requires a **Business subscription plus the paid API
  units add-on** — the reason this tool sits in 3-hold (account
  procurement); lane 2 must supply the funded test account.
- **Token semantics**: static bearer-style secret in the `key=` query
  parameter; no expiry, no refresh, no scopes. Access is all-or-nothing per
  account; every request debits the account's API-unit balance. Rate limit:
  10 requests/second.
- **OAuth?** Semrush OAuth (device grant RFC 8628 + a support-issued
  "Semrush Auth" flow) exists only for the Local API / deprecated Projects
  OAuth variant — not for the SEO API, and there is no self-serve
  multi-tenant client registration. The 2026-07-21 oauth-audit verdict ("no
  viable multi-tenant path — stays api_key") **matches the official docs**;
  no divergence.

## 3. anycli definition + service implementation

**Stage-1 rubric: `service` type.** No official Semrush CLI exists → wrap the
HTTP API in `internal/tools/semrush/` (register as
`RegisterService("semrush", &semrush.Service{})` in
`internal/tools/register.go` — registration rides the batch-end merge; the
package itself and `definitions/tools/semrush.json` merge freely).

`definitions/tools/semrush.json`:

```json
{
  "name": "semrush",
  "type": "service",
  "description": "Semrush SEO analytics (domain, keyword, backlink research; v3 API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "SEMRUSH_API_KEY"}
      }
    ]
  }
}
```

**Service shape** — copy `internal/tools/notion/` (cobra tree grouped by
resource, `BaseURL`/`HC`/`Out`/`Err` struct for httptest injection, exit-code
contract 0/1/2, `--json` structured error envelope, TDD with httptest fakes).
Two injectable base URLs: reports (`https://api.semrush.com`) and balance
(`https://www.semrush.com`); backlinks reports go to `<reports>/analytics/v1/`.

**CSV→JSON is the core value-add.** Semrush v3 returns semicolon-delimited
CSV with human-readable headers; the service parses it and emits JSON rows
keyed by snake_cased header names — generic header mapping, no hardcoded
per-report column tables:

```json
{"report": "domain_organic", "database": "us", "row_count": 2,
 "rows": [{"keyword": "…", "position": 3, "search_volume": 74000, "cpc": 1.5, "…": "…"}]}
```

Numeric-looking cells parse to numbers; everything else stays string.

**Error dialect** (Semrush returns errors as plain text `ERROR NN :: MESSAGE`,
typically with HTTP 200 — must be sniffed from the body):

- `ERROR 50 :: NOTHING FOUND` → **exit 0** with `rows: []` and a `note`
  field — "no data" is a valid answer for an agent, not a failure.
- `ERROR 120/121/122` (wrong/malformed key) and `ERROR 130` (API disabled) →
  `execution.RejectCredential(...)` (stale-credential feedback loop; bitly
  401 precedent), exit 1.
- `ERROR 131/132/134` (limits/zero balance) and all other `ERROR NN` → typed
  `apiError` `{code: NN, message}` envelope, exit 1.
- Usage/parse problems → exit 2.

**Unit-cost safety**: every line costs units, and Semrush's server default is
10,000 lines/request. The service always sends an explicit `display_limit`,
**default 10**; more rows require an explicit `--limit`. Shared flags:
`--database` (default `us`), `--limit`, `--offset`, `--columns`
(export_columns override), `--filter`, `--sort`, `--date`, `--positions`
(new|lost|rise|fall), backlinks `--target-type` (root_domain|domain|url,
default root_domain).

## 4. Helio provider bundle plan

`integrations/providers/semrush/provider.yaml` — hidden-first
(`presentation.visible: false`; flips only after L5 + pin bump land).

**Why not `manual_api_token`**: the declarative verifier
(`service/manual_token_verifier.go`) GETs a JSON identity endpoint with the
token in a bundle-declared **header**. Semrush v3 has neither: the key rides
a query parameter, and the only free verification endpoint
(`countapiunits.html`) returns a plain-text number, and there is no identity
("who am I") endpoint at all. **Why not mongodb's no-verify path**: unlike a
DSN, Semrush *does* have a cheap provider-side check, and the manual path's
posture is verify-first; skipping a real check would be a silent-downgrade.

**Plan**: `auth.type: credentials` + `connection.runtime_strategy:
manual_credentials`, plus a narrow compiled verifier in integration-service —
`semrushAPIUnitsVerifier` (new file `service/manual_semrush_verifier.go`):

- GET `https://www.semrush.com/users/countapiunits.html?key=<key>` (free).
- Body starting `ERROR` (or non-2xx) → invalid credential (maps to the
  existing `invalid_provider_credential` 400); numeric body → valid, and the
  remaining-units figure goes into the identity map (never the key).
- Account key/label: countapiunits returns no account identity, so derive
  from the secret's tail: `account_key = "key-<last4>"`, label
  `"API key ····<last4>"` (readable, deterministic, non-reversible; the
  mongodb OQ2 "human-readable" spirit — a full hash is what it forbids).
- Registry wiring: `composeProviderRegistration`'s
  `RuntimeStrategyManualCredentials` case currently hardwires
  `dsnHostIdentityDeriver{}` for every manual_credentials provider; it grows
  a per-provider deriver map (`mongodb → dsnHostIdentityDeriver`, `semrush →
  semrushAPIUnitsVerifier`, unknown → compose-time error), mirroring
  `composeExplicitOAuthRegistration`'s per-strategy switch. Alternative
  considered: a new reviewed strategy value (`semrush_units_check`) — more
  enum surface for the same one-provider dispatch; batch lead may overrule.

Bundle sketch:

```yaml
schema: helio.provider/v1
key: semrush
go_name: Semrush

presentation:
  name: Semrush
  description_key: semrush
  consent_domain: semrush.com
  visible: false

auth:
  type: credentials
  owner: individual
  credential_input:
    fields:
      - name: api_key
        label_key: semrush_api_key
        secret: true
        required: true
    setup_url: https://developer.semrush.com/api/get-started/authorization  # verify exact "where is my v3 key" page at impl time

identity:
  source: strategy   # compiled verifier: countapiunits check + key-tail account key

connection:
  mode: isolated
  disconnect_mode: local_only   # Semrush has no key-revoke API for v3 keys
  runtime_strategy: manual_credentials

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_key: token.access_token       # single secret in the token payload (317 D5)
    account_key: connection.account_key

tool:
  name: semrush
  kind: api-key
```

- `required_config_fields`: none (manual strategies take no server config).
- Seven shared surfaces at batch end: `register.go` entry, anycli tag + pin
  bump, bundle + one `provider-gen` run (five projections), **no**
  `toolToProvider` entry, icon `ui/helio-app/src/integrations/icons/semrush.svg`
  + `providerIcons.ts` append, provider sub-doc
  `agents/plugins/heliox/skills/tool/semrush.md` (+ i18n `tools.desc.semrush`,
  `tools.credentialField.semrush_api_key`) + plugin bump/publish.
- provider-gen runs **locally only** on this branch for validation; regens
  are not committed (master plan §2).
- The sub-doc must teach unit economics: per-line costs, default limit 10,
  `units` to check balance before big pulls.

## 5. Test plan (five layers)

| Layer | What runs | External credentials? |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes assert query construction (`key`, `type`, `display_limit` always present, `database` default), semicolon-CSV→JSON mapping, numeric coercion, `ERROR NN ::` dialect (50→empty-success, 120/122/130→RejectCredential, 132→apiError), both plain and `--json` rendering, exit codes 0/1/2, backlinks `/analytics/v1/` base + `target_type`, `units` plain-number parse | No |
| L2 | `make build-harness`; `ANYCLI_CRED_API_KEY=<v3 key> anycli semrush -- domain overview example.com`, `keyword overview …`, `backlinks overview …`, `units` against the real API. Also re-confirm here: v3 keyword/backlinks families still operational (pre-verify), exact `type=` strings, ERROR bodies-with-HTTP-200 behavior, countapiunits format (`1,000` comma handling) | **Yes — funded v3 API key** (Business + API-units add-on; lane 2 / 3-hold pre-verify) |
| L3 | Local (uncommitted) `go run ./cmd/provider-gen` + `--check` against the branch bundle; integration-service suite (new verifier unit tests: httptest countapiunits fake — numeric, `ERROR 120`, non-2xx; account-key last-4 derivation; registry composition for the deriver map); helio-cli build + `go test ./cmd/heliox/cmds/tool/` with local `replace github.com/heliohq/anycli => <anycli worktree>` (uncommitted) | No |
| L4 | Singleton; seed via `POST /internal/test-only/connections/seed` with `provider: semrush`, `access_token: <real v3 key>`, no refresh_token/expires_at (non-expiring static secret class); then `heliox tool semrush -- domain overview example.com` must return live data through the real token gateway | **Yes — same real key** |
| L5 | api_key-lane key-entry sweep (master plan §2, agent-drivable): connect link → paste key in the real connect UI (`POST /connections/credentials`, which exercises the compiled countapiunits verification) → connection listed in `GET /connections` → one **unseeded** live `heliox tool semrush -- …` run. Runs per-batch after batch-end merge; gates the visible flip | **Yes — same real key** |

Definition of done additions: icon registered, sub-doc published (batch
publish), visible flip only after L5 (single go-live change:
`presentation.visible: true` + regen).

## 6. Risks / open questions

1. **3-hold account procurement** (why this tool is holdbacked): the v3 key
   needs Business + paid API units; pre-verify gate confirms the pool
   account before dev starts. L2/L4/L5 are blocked without it.
2. **v3 deprecation creep**: keyword/backlinks v3 are deprecated-but-
   operational (2026-06). If shut off before build, shrink v1 scope to
   overview/domain/url and escalate the v4-key (version-scoped keys,
   single-secret storage) question.
3. **Account-key derivation from key tail** (`key-<last4>`): no provider
   identity exists; collision across two keys sharing a last-4 within the
   same assistant+provider is astronomically unlikely but would upsert-merge
   the rows. Accepted; batch lead may prefer a longer tail.
4. **Registry dispatch shape** (per-provider deriver map vs a new reviewed
   strategy enum) — proposal in §4; harmless either way for the bundle.
