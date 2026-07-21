# Tool design: Moz (`moz`)

Scratch per-tool design for the 300-integrations rollout (master plan
`docs/design/008-300-integrations-rollout-plan.md`, catalog row 265). Batch:
**3-hold** (account-procurement risk). Committed on `tool/moz`; the batch lead
strips this file at batch end.

| Axis | Value |
|---|---|
| ① CLI command word | `moz` (flat command, no group) |
| ② anycli tool id | `moz` (`definitions/tools/moz.json`, Go package `internal/tools/moz/`) |
| ③ provider catalog key | `moz` (`integrations/providers/moz/`) |

② == ③, so **no `toolToProvider` entry** in
`helio-cli/internal/toolcred/resolver.go` — identity mapping holds.

## 1. Which official API this wraps, and why

**Chosen surface: the current Moz API — JSON-RPC 2.0 at
`POST https://api.moz.com/jsonrpc`** (official docs: `moz.com/api/docs`,
verified 2026-07-21 directly against the live pages).

Verified facts from the official docs:

- Single universal endpoint; every call is an HTTP POST with a JSON-RPC 2.0
  body: `{"jsonrpc":"2.0","id":"<uuid>","method":"<name>","params":{"data":{…}}}`.
  The `id` is required and must be ≥ 24 characters; a V4 UUID is recommended.
- Auth: the API token goes in the **`x-moz-token`** request header. Tokens are
  generated on the Moz API dashboard; up to **5 active tokens per account**;
  no documented expiry (valid until deleted). The Moz API is a **separate
  subscription from Moz Pro**; a **free tier of 50 rows/month** exists (valid
  credit card required at signup, not charged).
- Quota is metered in "rows"; each method page documents its row cost.
  `quota.lookup` is **free** (paths: `api.limits.data.rows`,
  `api.limits.beta.rows`, `api.limits.mozscape.rows`).
- Errors: JSON-RPC `error` object (`code`, `status`, `data.{explanation,issue,key}`,
  `message`) with matching HTTP status. Rate limit: >50 requests producing 4xx
  in 5 minutes → throttled to 50 req/5 min until 30s without 4xx.
- The API is not versioned; breaking changes are limited to Beta-chip methods
  (Beta methods need Starter Medium plan+ and draw a separate quota).

**Not chosen:** the legacy Mozscape Links API (`lsapi.seomoz.com`,
access-id/secret HTTP Basic). Moz's own docs treat it as legacy
(`api.limits.mozscape.rows` is the legacy quota bucket); the new JSON-RPC API
supersedes it and carries the keyword-research surface the legacy API never
had. Building on the legacy pair-credential shape would also force a
two-field credential for no benefit.

### What an AI teammate does with Moz → method selection

An AI teammate uses Moz for four jobs: (a) authority/spam checks on domains
and pages, (b) backlink review, (c) keyword research, (d) ranking-presence
checks. v1 wraps exactly the methods those jobs need (method names verified
verbatim from the official method pages):

| Job | JSON-RPC method | Row cost |
|---|---|---|
| DA/PA/spam for one URL | `data.site.metrics.fetch` | 1/call |
| Same for a batch of URLs | `data.site.metrics.fetch.multiple` | 1/result |
| Brand Authority score | `data.site.metrics.brand.authority.fetch` | 1/call |
| List backlinks to a target | `data.site.link.list` | 1/result |
| Linking root domains | `data.site.linking-domain.list` | 1/result |
| Anchor-text profile | `data.site.anchor-text.list` | 1/result |
| Keyword volume/difficulty/CTR/priority | `data.keyword.metrics.fetch` | ≤4/call |
| Keyword suggestions | `data.keyword.suggestions.list` | 1/result |
| Search intent classification | `data.keyword.search.intent.fetch` | 1/call |
| Keywords a site ranks top-50 for | `data.site.ranking-keyword.list` | 1/result |
| Count of same | `data.site.ranking-keyword.count` | 1/call |
| Top pages by authority | `data.site.top-page.list` | 1/result |
| Quota check | `quota.lookup` | free |
| Index freshness | `metadata.index.fetch` | free |

Deliberately deferred (documented so scope growth is a decision, not drift):
`data.site.metrics.histories.fetch`, `data.site.metrics.distributions.fetch`,
link status/filter variants (`data.site.link.filter.*`,
`data.site.link.status.*`), `data.site.link.intersect.fetch`,
recently-gained/lost linking-domain filters, `data.site.redirect.fetch`,
`data.global.top-domain/page.list`, and the entire **Moz Local**
(`local.*`) family — a separate product most Moz API accounts don't carry.

## 2. anycli definition

**Stage-1 rubric: `service` type.** Moz ships no official CLI at all, so the
`cli`-type conditions fail at the first test. Service implementation in
`internal/tools/moz/` against the JSON-RPC API.

`definitions/tools/moz.json`:

```json
{
  "name": "moz",
  "type": "service",
  "description": "Moz SEO data API (Domain Authority, backlinks, keyword metrics)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "MOZ_API_TOKEN"}
      }
    ]
  }
}
```

`access_token` matches the bundle's credential projection (single secret via
`token.access_token`, the mongodb/bitly precedent), so the L2 harness variable
is `ANYCLI_CRED_ACCESS_TOKEN`.

### Service shape (`internal/tools/moz/`)

Follow the bitly/notion pattern: `Service{BaseURL, HC, Out, Err}` (BaseURL
default `https://api.moz.com/jsonrpc`; tests point it at `httptest`), duck-typed
`Execute`, cobra tree with `SilenceUsage/SilenceErrors`, exit codes 0/1/2, and
`--json` accepted for uniformity (output is always the provider JSON).

One `client.go` helper owns the JSON-RPC envelope: generates a fresh UUIDv4
request id per call, sets `x-moz-token` + `Content-Type: application/json`,
POSTs `{jsonrpc, id, method, params: {data: <payload>}}`, and unwraps the
response: on `result`, print it (passthrough + newline); on JSON-RPC `error`,
surface `message` + `data.explanation`; HTTP 401/403 →
`execution.RejectCredential` (stale-credential feedback loop). JSON-RPC error
objects carry `status` mirroring the HTTP code, so the HTTP status is the
discriminator.

Subcommand tree (verbs are thin mappings onto the table above):

```
moz site metrics --site <url> [--scope url|subdomain|domain]     # fetch; >1 --site → fetch.multiple
moz site brand-authority --site <url>
moz site top-pages --site <url> [--limit N]
moz link list --site <url> [--scope] [--limit N]
moz link domains --site <url> [--limit N]                        # linking-domain.list
moz link anchors --site <url> [--limit N]                        # anchor-text.list
moz keyword metrics --keyword <kw> [--locale en-US]              # volume+difficulty+CTR+priority
moz keyword suggestions --keyword <kw> [--limit N]
moz keyword intent --keyword <kw>
moz ranking-keywords list --site <url> [--limit N]
moz ranking-keywords count --site <url>
moz quota [--path api.limits.data.rows]
moz index                                                        # metadata.index.fetch
```

List commands expose the API's paging (`--limit`, page cursor/offset flag per
the method's documented paging fields) and default to a small page (25) —
every returned row bills quota, so no unbounded fan-out by default.

TDD per anycli AGENTS.md: httptest fakes assert method name, `params.data`
shape, `x-moz-token` header, id length ≥ 24, and both plain and `--json`
error rendering; never hit the real API from unit tests. Registration in
`internal/tools/register.go` stays on this branch until the batch-end merge.

## 3. Credential fields and auth flow (audit verified)

**Lane: `api_key` — confirmed against official docs**; master-plan catalog row
265 and the oauth-audit verdict (audit row 267 — the pre-renumber id; "no
viable multi-tenant path — stays api_key") both hold. (The row numbers differ
because the 2026-07-22 OpenAI/Anthropic removal renumbered the master plan;
Moz is master-plan row 265, still audit row 267.)
The Moz API has **no OAuth surface at all**: registration model is Moz account
→ API subscription (free tier ok) → dashboard-generated bearer-style token in
a custom header. No scopes exist — a token grants the account's full API
quota. No expiry/rotation semantics; revocation = deleting the token in the
dashboard.

Credential = **one secret field** (`access_token` ← the user's Moz API token).
Multi-account is naturally supported (an account can mint 5 tokens; different
Moz accounts are different connections).

### Divergence to flag: token verification cannot use the declarative GET

integration-service's `manual_api_token` path verifies a pasted token by
**GET**ting the bundle's `identity.url` with the declared header
(`service/manual_token_verifier.go`). Moz has **no GET identity endpoint**:
the API is POST-only JSON-RPC. The existing alternative,
`manual_credentials`, is hard-wired to the mongodb DSN-host identity deriver
(`composeProviderRegistration`), which cannot parse an opaque Moz token.

**Recommendation: keep verify-first, add a narrow compiled verifier** —
`quota.lookup` is the purpose-built probe: costs zero quota, requires a valid
token, and returns `result.quota.account_id`, a stable numeric account
identity.

**Auditable evidence for `account_id` (verified 2026-07-22 against the live
official schema).** The Moz docs SPA embeds the method's typed schema in its
`__NEXT_DATA__` JSON. For `quota.lookup` (`actionName.dotNotation ==
"quota.lookup"`), the response definition at JSON path
`props.pageProps.dataParsed.responseDefinition` declares
`properties.quota.properties.account_id`, verbatim:

```json
"account_id": {
  "key": "account_id",
  "description": "The account ID.",
  "type": ["number"],
  "nullable": false,
  "optional": false
}
```

`account_id` is a required (`optional: false`), non-nullable numeric sibling of
`path`/`allotted`/`used`/`reset` inside `result.quota`, and the API schema's
`QuotaObject`/`QuotaUsageObject` definitions both list `account_id` in their
`required` arrays. So the connection's stable account key is the JSON path
`result.quota.account_id`. (The earlier search-only pass could not corroborate
this because the docs are a client-rendered SPA — the schema is not in the
server-rendered HTML body; it lives only in the `__NEXT_DATA__` blob.)

Wire a small `manualTokenVerifier` implementation (POST `quota.lookup` with
`path: api.limits.data.rows`; 2xx → accountKey = `account_id` as string, label
`"Moz account <id>"`; 401/403 → invalid token) selected via a named runtime
strategy, e.g. `moz_api_token`. The on-point precedent is **mongodb's
`manual_credentials` → `dsnHostIdentityDeriver`** (`provider_registry.go`
binds it in the `RuntimeStrategyManualCredentials` case, then the registry
selects it via `registration.manual`): a named runtime strategy that binds a
compiled manual verifier on the `AuthCredentials`/`AuthAPIKey` path, exactly
the shape the moz verifier follows. Moz differs only in that its compiled
verifier *does* perform a provider-side POST probe (verify-first) rather than
mongodb's no-verify DSN-host derivation — a verifier facet, not a new
mechanism. This is the **one integration-service code item** for this tool —
flagged here at stage 1 per master plan §5, and small by design (verifier
facet only; exchanger/revoker stay no-op).

Fallback if the batch lead rejects service code for a 3-hold tool: mongodb's
OQ1 no-verify shape (`credentials` + `manual_credentials`) with a generalized
no-op identity deriver — but with `account_id` now schema-confirmed, this
fallback loses connect-time token validation *and* the real, stable account
key that is the sole cited advantage of the verifier, so it is second choice.

## 4. Helio provider bundle plan

`integrations/providers/moz/provider.yaml` — hidden-first, with the
recommended verify path:

```yaml
schema: helio.provider/v1
key: moz
go_name: Moz

presentation:
  name: Moz
  description_key: moz
  consent_domain: moz.com
  visible: false          # hidden until L5 + 3-hold pre-verify clear
  # order: pick unoccupied at flip time

auth:
  type: api_key
  owner: individual
  api_key:
    header: x-moz-token
    setup_url: https://moz.com/api/dashboard

identity:
  source: strategy        # compiled quota.lookup verifier derives account_id;
                          # no GET userinfo endpoint exists (POST-only JSON-RPC)

connection:
  mode: isolated
  disconnect_mode: local_only   # no provider-side token-revoke API
  runtime_strategy: moz_api_token   # named strategy → compiled verifier (§3)

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: moz
  kind: api-key
```

Notes:

- `required_config_fields` is empty — api_key providers need zero Helio-side
  client secrets, so no `config/` + `deploy/` OAuth appends and no lane-1 app
  registration. The whole lane-1 dependency for moz is the **test account**.
- `identity.source: strategy` requires either the named-strategy verifier
  (recommended) or, if the declarative path is forced, a `userinfo` GET URL
  that does not exist — this is exactly the §3 divergence. If the generator's
  strategy whitelist rejects `moz_api_token` before the service change lands,
  the bundle rides the same commit as the verifier (both are batch-end
  serialized anyway).
- If the verifier lands, moz becomes the **first live `api_key` provider**
  since Figma moved to MCP (today only the synthetic `acme` fixture exercises
  the mode) — expect to dust off that path in L4.
- UI icon: `ui/helio-app/src/integrations/icons/moz.svg` + manual
  `providerIcons.ts` registration (batch-end). AI-facing sub-doc under
  `agents/plugins/heliox/skills/tool/` (batch-end publish).
- Locally regenerated provider-gen projections are **not committed** (master
  plan §2); the branch is expected to fail `provider-gen --check` in CI until
  the batch-end regen.

## 5. Test plan (five layers)

| Layer | What runs for moz | External credential needed |
|---|---|---|
| L1 | `go test ./...` in anycli: httptest fakes for every subcommand (method name, params.data, `x-moz-token`, id ≥24 chars, error envelope incl. JSON-RPC error body, 401 → RejectCredential) | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli moz -- quota`, `-- index`, `-- site metrics --site moz.com`, `-- keyword metrics --keyword seo` against the live API | **yes** — Moz API token from the account pool (free tier suffices: quota/index are free; the smoke calls cost ~5 rows of the 50/month) |
| L3 | local `go run ./cmd/provider-gen` + `--check` against the branch bundle; helio-cli built with the sanctioned **uncommitted** `go.mod replace` to this worktree; `go test ./cmd/heliox/cmds/tool/` + `make test-integration-service` (incl. new verifier unit tests with an httptest Moz fake) | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` (provider `moz`, real ObjectID identities, `access_token` only — no refresh cycle, token gateway serves it directly) → `heliox tool moz -- quota` must return live data through the real token gateway | **yes** — same pool token |
| L5 | api_key key-entry sweep (master plan §2 lane 3): open connect link → paste token in the real connect UI (`POST /connections/credentials`, which triggers the §3 verifier) → connection shows connected in `GET /connections` → one **unseeded** live `heliox tool moz -- site metrics --site moz.com` succeeds | **yes** — pool token; agent-drivable (no OAuth consent) |

Quota-budget note for the 3-hold pre-verify: unlike Ahrefs/Semrush, Moz's
**free tier is sufficient for the entire L2–L5 test ladder** — `quota.lookup`
and `metadata.index.fetch` are free, and the remaining smoke calls cost single
rows against the 50-row/month allotment. The catalog's "meaningful quotas are
paid" risk applies to production usage volume, not to test-account
feasibility; record this at the pre-verify gate. A paid Starter tier is only
needed if the L5 sweep or docs examples want list-heavy runs (each returned
row bills).

Definition of done follows the master plan: L1–L5 green, docs published, icon
registered, then the visible flip (`presentation.visible: true` + regen) as
the single go-live change after the 3-hold pre-verify and L5 pass.
