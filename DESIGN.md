# Tool design: DataForSEO (`dataforseo`)

Scratch per-tool design for the 300-integrations rollout (master plan
`docs/design/008-300-integrations-rollout-plan.md`, catalog row 269, Wave 3,
category "SEO & Web Data"). Batch lead strips this file at batch end.

- anycli tool id (axis ②): `dataforseo`
- Helio provider catalog key (axis ③): `dataforseo`
- CLI command word (axis ①): `dataforseo` (flat command, no `tool.group`)
- Auth lane: `api_key` (confirmed against official docs — see §3)
- Worktrees: anycli `tool/dataforseo` (this repo), Helio `tool/dataforseo`

Because axis ② == axis ③, **no `toolToProvider` entry** is needed in
`helio-cli/internal/toolcred/resolver.go`.

## 1. What this tool is for (AI-teammate jobs)

DataForSEO is a metered, pay-per-request SEO data API. An AI teammate uses it
to answer real marketing/SEO questions without a browser:

1. **Rank checking** — "where does example.com rank for X?" → live Google SERP.
2. **Keyword research** — search volume, CPC, difficulty, intent, and idea
   expansion for content planning.
3. **Domain / competitor research** — a domain's ranked keywords, its organic
   visibility overview, and who its SERP competitors are.
4. **Backlink profile analysis** — summary metrics, top backlinks, referring
   domains, anchor text distribution.
5. **One-page technical check** — instant on-page audit of a single URL.
6. **Cost/account awareness** — balance and rate limits (free endpoint), so an
   agent can check funds before running a large metered job.

Everything above is served by DataForSEO **Live** endpoints (synchronous,
single request/response). The task-based (`task_post`/`tasks_ready`/`task_get`)
queue mode and the site-crawl OnPage flow are **out of scope for v1**: they are
async and stateful (submit → poll → fetch), a poor fit for one-shot CLI
invocations, and every v1 job above has a live equivalent. Merchant, App Data,
Business Data, Content Analysis, AI Optimization, and Databases families are
also out of v1 scope (niche relative to the core SEO jobs; additive later).

## 2. Official API surface (verified 2026-07-21 against docs.dataforseo.com)

Base URL `https://api.dataforseo.com/v3`. All POST endpoints take a JSON
**array of task objects**; Live endpoints accept exactly one task per call.
Rate limits: 2000 calls/min, 30 concurrent.

Wrapped endpoints (all Live / synchronous):

| Job | Endpoint |
|---|---|
| SERP | `POST /v3/serp/google/organic/live/advanced` |
| Search volume | `POST /v3/keywords_data/google_ads/search_volume/live` |
| Keyword ideas | `POST /v3/dataforseo_labs/google/keyword_ideas/live` |
| Keyword suggestions | `POST /v3/dataforseo_labs/google/keyword_suggestions/live` |
| Keyword difficulty | `POST /v3/dataforseo_labs/google/bulk_keyword_difficulty/live` |
| Search intent | `POST /v3/dataforseo_labs/google/search_intent/live` |
| Domain overview | `POST /v3/dataforseo_labs/google/domain_rank_overview/live` |
| Ranked keywords | `POST /v3/dataforseo_labs/google/ranked_keywords/live` |
| Domain competitors | `POST /v3/dataforseo_labs/google/competitors_domain/live` |
| Backlinks summary | `POST /v3/backlinks/summary/live` |
| Backlinks list | `POST /v3/backlinks/backlinks/live` |
| Referring domains | `POST /v3/backlinks/referring_domains/live` |
| Anchors | `POST /v3/backlinks/anchors/live` |
| On-page check | `POST /v3/on_page/instant_pages` |
| Account/limits | `GET /v3/appendix/user_data` (free) |
| Locations/languages helper | `GET /v3/serp/google/locations`, `GET /v3/serp/google/languages` |

**Error dialect (verified, `/v3/appendix/errors`):** the API returns HTTP 200
for almost everything; the only HTTP-level errors are **401** (bad
credentials), **402** (billing), **404** (unknown endpoint), **500**. Real
errors ride in the body: top-level and per-task `status_code` — `20000` "Ok."
(and `20100` "Task Created.") are success; `40xxx` are client errors (`40100`
not authorized, `40200`/`40210` payment/funds, `40202`/`40209` rate limits,
`40501`/`40503` invalid field/POST structure), `50xxx` server-side. The service
implementation MUST inspect body `status_code`, never HTTP status alone.

**Cost model:** every non-appendix call is charged (per request or per result;
sample costs observed in official docs: labs keyword_ideas ≈ $0.01, backlinks
summary ≈ $0.02, SERP advanced fractions of a cent — SERP keyword operators
like `site:` incur a 5x multiplier). Every response carries a `cost` field;
the tool surfaces it (§4) so agents stay cost-aware.

## 3. Auth — verified against official docs; catalog lane confirmed

Official docs (`/v3/auth`): DataForSEO supports **HTTP Basic authentication
only** — an API **login** plus an auto-generated API **password** (distinct
from the account password), both shown at `https://app.dataforseo.com/api-access`.
Wire format: `Authorization: Basic base64(login:password)`. No OAuth of any
kind, no token exchange, no scopes, no expiry; the password is regenerable
from the dashboard (rotation = paste the new pair). Registration is self-serve
(free account; API usage is prepaid pay-as-you-go — the test account needs a
small deposit, see §6).

This **confirms the catalog's `api_key` lane and the oauth-audit verdict**
("no viable multi-tenant path — stays api_key"). One material nuance the
catalog's flat "api_key" label hides, recorded here as the only divergence:

> **The credential is a login:password *pair*, not a single token**, and
> Helio's manual-credential machinery enforces the design 317 D5 single-secret
> constraint (`model.validateCredentialInputSchema`: exactly one required
> `credential_input` field stored as the token payload).

**Decision: store one joined secret, `login:password`.** The API login is an
email address (colon-free), so splitting at the first `:` is unambiguous; this
matches DataForSEO's own documented idiom
(`cred="$(printf ${login}:${password} | base64)"`). It fits the existing
single-secret storage, the token gateway's `token.access_token` projection,
and a one-field connect form — zero storage-model changes.

## 4. anycli definition (stage 1–2)

**Type: `service`.** Stage-1 rubric: no official DataForSEO CLI exists (their
official terminal/agent offering is an MCP server, which is explicitly out of
scope for this plan; all CLIs are third-party). Service type, implemented in
`internal/tools/dataforseo/` against the HTTP API.

`definitions/tools/dataforseo.json`:

```json
{
  "name": "dataforseo",
  "type": "service",
  "description": "DataForSEO SEO data APIs — SERP, keyword research, backlinks, on-page (Basic-auth API credentials)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "DATAFORSEO_CREDENTIALS"}
      }
    ]
  }
}
```

`access_token` is the resolver-map field the Helio token gateway already
projects (`token.access_token`) — bitly/slack precedent. The value is the
joined `login:password` pair; the service splits at the first colon and builds
`Authorization: Basic base64(...)` itself. A malformed value (no colon) is a
credential error (`execution.RejectCredential`), not a usage error.

**Package:** `internal/tools/dataforseo/` (id has no dashes), registered as
`RegisterService("dataforseo", &dataforseo.Service{})` in
`internal/tools/register.go` — registration itself rides the **batch-end**
merge; the package and definition JSON merge freely mid-batch.

**Shape (notion is the reference implementation):** `Service` struct with
`BaseURL`/`HC`/`Out`/`Err` seams for httptest; cobra tree grouped by resource;
exit codes 0 success / 1 API-runtime failure (typed `apiError`) / 2
usage-parse; `--json` structured error envelope.

### Command tree

```
dataforseo serp google            --keyword <q> [--location <name|code>] [--language <code>] [--depth N] [--device desktop|mobile]
dataforseo keywords volume        --keywords <a,b,c> [--location ...] [--language ...]
dataforseo keywords ideas         --keywords <a,b,c> [--limit N] [--location ...] [--language ...]
dataforseo keywords suggestions   --keyword <q> [--limit N] [--location ...] [--language ...]
dataforseo keywords difficulty    --keywords <a,b,c> [--location ...] [--language ...]
dataforseo keywords intent        --keywords <a,b,c> [--language ...]
dataforseo domain overview        --target <domain> [--location ...] [--language ...]
dataforseo domain ranked-keywords --target <domain> [--limit N] [--location ...] [--language ...]
dataforseo domain competitors     --target <domain> [--limit N] [--location ...] [--language ...]
dataforseo backlinks summary      --target <domain|url>
dataforseo backlinks list         --target <domain|url> [--limit N]
dataforseo backlinks referring-domains --target <domain|url> [--limit N]
dataforseo backlinks anchors      --target <domain|url> [--limit N]
dataforseo onpage check           --url <absolute-url>
dataforseo meta locations         [--search <substr>]
dataforseo meta languages         [--search <substr>]
dataforseo account
```

Defaults: `--location "United States"` (code 2840) and `--language en` where
the endpoint requires them — documented in `--help` and overridable;
`--location` accepts either a location name or a numeric location_code.
`meta locations|languages` exists because location/language identifiers are
the #1 friction for agents (server lists fetched live, filtered client-side).
`account` doubles as the free smoke/identity command.

### JSON output shape

Success (stdout, always JSON):

```json
{"cost": 0.0103, "result": [ ...tasks[0].result unwrapped... ]}
```

- The DataForSEO envelope (`version`, `tasks`, echo `data`, …) is stripped;
  agents get `tasks[0].result` plus the metered `cost` — cost is deliberately
  first-class because every call spends real money.
- Error mapping: HTTP 401 or body `status_code` 40100 →
  `execution.RejectCredential` (drives Helio's stale-credential feedback);
  body 40200/40210 → explicit "insufficient DataForSEO balance — top up at
  app.dataforseo.com" error; any other non-2xxxx body code → `apiError`
  carrying `status_code` + `status_message`, exit 1; flag/parse problems exit
  2. `--json` renders the standard structured error envelope.

## 5. Helio provider bundle plan (hidden-first)

`integrations/providers/dataforseo/provider.yaml`, `presentation.visible:
false` until the pin bump lands, L1–L5 pass, and the batch flip. Naming per
master plan §3: key `dataforseo` == id `dataforseo`; flat axis-① command.

Two candidate shapes; **primary recommendation first**:

### Primary: `auth.type: api_key` (verify-first) + one reviewed generic capability

DataForSEO has an ideal HTTPS identity endpoint — `GET /v3/appendix/user_data`
(free; returns `login`, balance, limits; HTTP 401 on bad credentials). The
existing `manual_api_token` path (`declarativeManualTokenVerifier`) verifies
by sending the stored secret **verbatim** in a declared header; DataForSEO
needs `Authorization: Basic base64(secret)` instead. Proposal: grow the closed
capability set by one reviewed enum — e.g. `auth.api_key.scheme:
basic_pair` — meaning "stored secret is `user:pass`; send `Authorization:
Basic base64(secret)` when verifying". This follows the repo doctrine
("grow one more reviewed enum value instead of an adapter") and is
**batch-shared, not DataForSEO-specific**: Twilio (Wave 1), Mailjet, Mux,
Cloudinary, Zamzar and other catalog rows are the same Basic-pair shape.
**Flag to the batch lead / integration-service owner before implementation**
— whichever batch ships first should land the enum once.

Bundle sketch (primary):

```yaml
schema: helio.provider/v1
key: dataforseo
go_name: DataForSEO
presentation:
  name: DataForSEO
  description_key: dataforseo
  consent_domain: dataforseo.com
  visible: false
auth:
  type: api_key
  owner: individual
  api_key:
    header: Authorization
    scheme: basic_pair        # proposed reviewed enum, see above
    setup_url: https://app.dataforseo.com/api-access
  credential_input:
    fields:
      - name: api_credentials
        label_key: dataforseo_api_credentials
        secret: true
        placeholder: "login@example.com:api_password"
        required: true
    setup_url: https://app.dataforseo.com/api-access
identity:
  source: userinfo
  url: https://api.dataforseo.com/v3/appendix/user_data
  stable_key: /tasks/0/result/0/login
  label_candidates: [/tasks/0/result/0/login]
connection:
  mode: isolated
  disconnect_mode: local_only    # no provider-side revoke API; regenerate in dashboard
  runtime_strategy: manual_api_token
resources: {selection: none, discovery: none, enforcement: none}
credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key
tool:
  name: dataforseo
  kind: api-key
```

`account_key`/label = the API login (email) — human-readable, non-secret.
Verify during implementation that `jsonPointerString` accepts RFC 6901 array
indices (`/tasks/0/result/0/login`); if not, that is a second small generic
gap to close (again batch-shared — DataForSEO's envelope style is common).
`required_config_fields` stays empty (manual strategies use no server config;
no lane-1 app registration exists for this tool).

### Fallback: `auth.type: credentials` (mongodb precedent, no verification)

If the batch lead rejects any integration-service change mid-wave: mirror the
mongodb bundle (`runtime_strategy: manual_credentials`, `identity.source:
strategy`, single `credential_input` field as above). Cost: no verify-first
(a typo'd pair surfaces only at first use via `CredentialRejected` stale
feedback) and the compiled `manual_credentials` strategy must derive
account_key from the login half of the pair (today it derives a DSN host —
still a compiled touch). Since both shapes need *some* small service-side
change, the verify-first primary is preferred on both correctness and UX.

### Other Helio-side artifacts (batch-end merge)

- Icon: `ui/helio-app/src/integrations/icons/dataforseo.svg` + manual
  register in `providerIcons.ts`.
- AI-facing docs: `agents/plugins/heliox/skills/tool/dataforseo.md` — must
  cover the metered-cost model (`cost` field, `account` for balance, SERP
  operator 5x multiplier) so agents don't burn balance blindly; plugin
  version bump + publish ride the batch.
- Projections: run `provider-gen` + `--check` **locally only** for
  validation; do NOT commit regenerated projections (master plan §2 — the
  batch lead produces the one canonical regen; this branch is expected to
  fail `provider-gen --check` in CI until batch end).
- helio-cli: local, uncommitted `go.mod` `replace github.com/heliohq/anycli
  => <this worktree>` for on-branch L4 builds; the real pin bump is the
  batch lead's.

## 6. Test plan (five layers)

TDD per anycli AGENTS.md: tests first at every stage. Conventional commits on
`tool/dataforseo` only.

| Layer | What runs | External credentials? |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes assert (a) one-task JSON array body shape per endpoint, (b) `Authorization: Basic <base64 of pair>` built from `DATAFORSEO_CREDENTIALS`, (c) envelope unwrap to `{cost, result}`, (d) HTTP-200-with-body-40501 → `apiError` exit 1, (e) HTTP 401 and body 40100 → `RejectCredential`, (f) 40200/40210 → balance guidance, (g) malformed pair (no colon) → credential error, (h) `--json` error envelope + exit-code contract, (i) flag validation exit 2 | **No** |
| L2 | dev harness against the real API: `ANYCLI_CRED_ACCESS_TOKEN='login:password' anycli dataforseo -- account` (free), then one metered smoke per family: `serp google`, `keywords volume`, `keywords ideas`, `domain overview`, `backlinks summary`, `onpage check`. Mandatory before any pin bump. Budget: cents (~$0.10 total); confirm here whether Backlinks endpoints need the backlinks subscription vs pay-as-you-go on the pool account (user_data exposes `backlinks_subscription_expiry_date`) | **Yes** — lane-2 pool account, funded (small deposit; ~$5 covers the whole program for this tool) |
| L3 | local `provider-gen` + `provider-gen --check` against the branch bundle (uncommitted); `make test-integration-service` (includes the proposed `basic_pair` verifier unit tests + a fake-provider identity fixture with the nested `/tasks/0/result/0/login` pointer); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with the local `replace` | **No** |
| L4 | singleton + `POST /internal/test-only/connections/seed` with `access_token: "login:password"` (non-expiring manual secret — seed `access_token` only, no `refresh_token`/`expires_at`, Slack-token pattern), then `heliox tool dataforseo -- account` and one metered command reaching the live API through the real token gateway. No lane-1 dependency (api_key lane: no app registration) | **Yes** — same pool account |
| L5 | api_key key-entry sweep (master plan §2): open connect link → paste `login:password` in the real connect UI (write-only `POST /connections/credentials`; verify-first hits `user_data` under the primary bundle) → connection shows connected in `GET /connections` → one **unseeded** live command succeeds. Agent-drivable (agent-browser), human fallback. Gates the visible flip | **Yes** — same pool account |

Definition of done tracks the master plan: five layers green, docs published,
icon registered, then the hidden→visible flip + regen as the single go-live
change (batch-end owned).

## 7. Divergences & flags recorded

1. **Lane confirmed, shape refined**: `api_key` per catalog/audit is correct
   (Basic auth only; no OAuth path exists), but the credential is a
   login:password **pair** stored as one joined secret under the design 317
   D5 single-secret constraint.
2. **Proposed shared capability**: `auth.api_key.scheme: basic_pair` reviewed
   enum (+ possible RFC 6901 array-index check in `jsonPointerString`) —
   batch-lead coordination item; benefits multiple catalog rows (Twilio,
   Mailjet, Mux, Cloudinary, Zamzar, …). Fallback (no service change beyond
   account-key derivation): mongodb-style `credentials` bundle without
   verify-first.
3. **Error dialect**: HTTP 200 + in-body `status_code` — the anycli service
   must parse bodies for errors; documented in §2 and pinned by L1 tests.
4. **No resolver entry**: axis ② == axis ③ (`dataforseo`).
5. **Metered API**: docs and output shape make cost first-class; L2/L4/L5
   spend real (cent-scale) money and need a funded pool account.
