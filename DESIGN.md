# Tool design: SerpAPI (`serpapi`)

**Catalog row:** #268 · SerpAPI · anycli id `serpapi` · provider key `serpapi` · auth lane `api_key` · Wave 3 · SEO & Web Data
**Audit verdict:** "no viable multi-tenant path — stays api_key per rubric" (oauth-audit.md row 268). **Verified against official docs: correct.** SerpApi has no OAuth2 authorization server; the only credential is the per-account private API key ([serpapi.com/search-api](https://serpapi.com/search-api): `api_key` — "Parameter defines the SerpApi private key to use"). The api_key lane stands.

**One material divergence found (recorded in §4):** SerpApi authenticates by **query parameter only** — no HTTP header auth exists in the official docs — while Helio's `auth.type: api_key` verification path (`declarativeManualTokenVerifier`) injects the token exclusively via a bundle-declared **header**. The bundle plan below grows the generic reviewed capability rather than writing a provider adapter.

## 1. What an AI teammate does with SerpAPI, and the API surface wrapped

SerpApi is a live-SERP retrieval service: one GET endpoint, ~70 pluggable engines. An AI teammate uses it to:

- **Search the live web with structured results** (Google organic, knowledge graph, answer boxes) for research and fact-finding.
- **SEO work**: rank checking for a keyword + location, competitor visibility, related searches, People-Also-Ask mining.
- **Monitoring**: news (`google_news`), shopping/price checks (`google_shopping`), local results (`google_maps`), jobs (`google_jobs`), video (`youtube`), scholar (`google_scholar`), trends (`google_trends`).
- **Quota awareness**: check remaining searches before burning a batch (Account API is free to call).

Official surface wrapped (all verified against serpapi.com docs):

| Endpoint | Method | Auth | Why |
|---|---|---|---|
| `GET https://serpapi.com/search` | GET | `api_key` query param | The whole product. `engine` selects the vertical; `q` + `location`/`gl`/`hl`/`device`/`start`/`num` are the common knobs; every engine adds its own params. |
| `GET https://serpapi.com/searches/{search_id}.json` | GET | `api_key` query param | Search Archive API — re-fetch a prior search free of charge for up to 31 days (`search_metadata.id` from any search response). Lets the teammate re-read results without spending quota. |
| `GET https://serpapi.com/locations.json` | GET | **none** (free) | Locations API — resolve a human place name to the `canonical_name` the `location` param requires. |
| `GET https://serpapi.com/account.json` | GET | `api_key` query param | Account API — plan, `total_searches_left`, hourly rate limit. Free; doubles as the credential smoke test. |

Deliberately **not** wrapped: per-engine bespoke subcommands (70 engines, each with a distinct param set — a closed per-engine CLI would be unmaintainable and instantly stale) and the HTML output mode (`output=html` is useless to an agent consuming JSON; the raw page is reachable via `search_metadata` URLs if ever needed).

## 2. anycli definition

### Stage-1 rubric: `service` type

No official SerpApi CLI binary exists (official artifacts are language SDKs: `google-search-results` gems/packages). All four `cli`-type conditions fail at the first one → **`service` type**, implemented in `internal/tools/serpapi/` against the HTTP API. Matches 21-of-23 precedent.

### Definition `definitions/tools/serpapi.json`

```json
{
  "name": "serpapi",
  "type": "service",
  "description": "SerpApi as a tool (search engine results API, private API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "api_key"},
        "inject": {"type": "env", "env_var": "SERPAPI_API_KEY"}
      }
    ]
  }
}
```

- Credential field name `api_key` matches the Helio bundle's `credential.fields` key (§5) and the harness env `ANYCLI_CRED_API_KEY`.
- Env var `SERPAPI_API_KEY` follows the ecosystem convention (LangChain et al.); SerpApi's own skill docs use `SERPAPI_KEY` — the anycli-internal env name is our contract with our own service code, not the provider's, so the more widely recognized `SERPAPI_API_KEY` wins.

### Package & registration

- Package `internal/tools/serpapi/` (id has no dashes — package name == id).
- `RegisterService("serpapi", &serpapi.Service{})` in `internal/tools/register.go` — **batch-end shared surface; not merged mid-batch**. The definition JSON and package code are generation-inert and merge freely.
- `Service` struct per design 003 §3: `BaseURL string` (default `https://serpapi.com`), `HC *http.Client`, `Out`/`Err io.Writer`; cobra tree built per `Execute`; credentials only from the resolved env map; missing `SERPAPI_API_KEY` → exit 1 with explicit message.

### Subcommand tree

```
serpapi search    -q <query> [--engine google] [--location <canonical>] [--gl us] [--hl en]
                  [--google-domain google.com] [--device desktop|tablet|mobile]
                  [--num N] [--start N] [--no-cache] [--param key=value ...]
serpapi archive   get <search_id>
serpapi locations [--q <text>] [--limit N]
serpapi account
```

- **`search`** is one generic command: first-class flags for the cross-engine common params, plus a repeatable `--param key=value` escape hatch for engine-specific params (`tbm`, `ludocid`, `data_id`, `num` quirks, …). `--engine` is passed through **unvalidated** (no hardcoded engine whitelist — SerpApi adds engines continuously; an unknown engine fails with the provider's own error). This keeps the tool orthogonal: one axis = engine, one axis = params.
- **`archive get`** appends `.json` and fetches the archived search — free re-read within 31 days.
- **`locations`** needs no credential per official docs; the command still runs under the tool so the teammate has one place to resolve `location` values. It does not send `api_key`.
- **`account`** emits the Account API response **with the `api_key` field redacted** — the official response echoes the private key (`api_key` field), and passing it through would leak the secret into the agent transcript. This is the one deviation from verbatim passthrough, same class as bitly's `qr image` envelope exception.

### JSON output & error contract

- Success: provider JSON passthrough to stdout + newline (bitly/notion precedent), except the `account` redaction above. Root `--json` flag accepted for uniformity (always-on).
- Exit codes: 0 success · 1 runtime/API failure (typed `apiError` carrying SerpApi's top-level `"error"` string from non-2xx JSON bodies) · 2 usage/parse errors. `--json` renders the structured error envelope.
- **401 → `execution.CredentialRejected`** (invalid/revoked API key), triggering the engine's stale-marking for `(serpapi, account)` — bitly `client.go` precedent.

## 3. Credential fields & auth flow (verified)

- **Registration model:** self-serve — sign up at serpapi.com, free tier included; the private key lives at dashboard → Your Account → API key (`https://serpapi.com/manage-api-key`). No app registration, no redirect URI, no human lane-1 dependency (api_key lane, as scheduled).
- **Token semantics:** one long-lived private key per account; no scopes, no expiry, no refresh cycle; rotation is manual in the dashboard. Rate limits are plan-based (`account_rate_limit_per_hour`).
- **Wire placement:** `api_key` **query parameter only**. Official docs (Search API, Account API, Search Archive API) document no header alternative — divergence handling in §4.
- **Identity for verification:** `GET /account.json` returns `account_id` (stable), `account_email` (label) — a perfect verify-first identity source, and free to call.

## 4. Helio provider bundle plan

### Naming axes (master plan §3)

| Axis | Value |
|---|---|
| ① CLI command word | `serpapi` (flat command, no group) |
| ② anycli tool id | `serpapi` |
| ③ provider catalog key | `serpapi` |

② == ③ → **no `toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`. No `tool.group`/`tool.command` in the bundle.

### The divergence and its resolution

Helio's `api_key` runtime strategy (`manual_api_token`) verifies the pasted key by GETing `identity.url` with `req.Header.Set(auth.api_key.header, token)` — header injection is the only compiled path, and `provider-gen` requires `auth.api_key.header` to be a valid header name. SerpApi accepts no header. Options considered:

1. ~~Narrow adapter (`service/adapter_serpapi.go`)~~ — rejected: the gap is not provider-specific. Query-param API keys are a recurring shape across the 160-tool api_key lane.
2. ~~`auth.type: credentials` / `manual_credentials` (mongodb precedent)~~ — rejected: it skips verify-first (mongodb only does so because it has **no** HTTPS identity endpoint; SerpApi has one), and its identity path still needs compiled per-provider code (`dsnHostIdentityDeriver` is mongodb-specific) — worse UX *and* service code anyway.
3. **Chosen: grow the generic reviewed capability** — exactly what `references/provider-yaml.md` prescribes ("first check … whether the generic capability set should grow one more reviewed enum value instead"). Add a key-placement enum to the api_key policy:
   - `manifest.go` `apiKeyManifest`: add `in: header|query` (default `header`, existing bundles unchanged) and `param` (query-param name, required iff `in: query`; `header` required iff `in: header`).
   - `validate.go`: closed-enum validation for the new fields.
   - `model/catalog.go` APIKey policy type + `provider-gen` render: project the new fields.
   - `manual_token_verifier.go`: `in: query` branch sets the param on `identity.url` instead of a header. (Awareness: the key then appears in the request URL; that is SerpApi's own only-supported auth shape.)
   - Synthetic test-fixture coverage alongside the existing `acme` api_key fixture.

   This lands as a small integration-service change with tests, in this tool's Helio-side branch, before the bundle can pass `provider-gen` locally.

### `integrations/providers/serpapi/provider.yaml` (hidden-first)

```yaml
schema: helio.provider/v1
key: serpapi
go_name: SerpAPI

presentation:
  name: SerpApi                # official brand styling
  description_key: serpapi
  consent_domain: serpapi.com
  visible: false               # hidden until anycli pin ships + L5 passes (flip is the go-live change)
  order: <unoccupied at batch end>

auth:
  type: api_key
  owner: individual
  api_key:
    in: query                  # new reviewed capability (§4)
    param: api_key
    setup_url: https://serpapi.com/manage-api-key

identity:
  source: userinfo
  url: https://serpapi.com/account.json
  stable_key: /account_id
  label_candidates: [/account_email]

connection:
  mode: isolated
  disconnect_mode: local_only  # no provider-side key revoke API; rotation is manual in the dashboard
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    api_key: token.access_token          # single secret rides the existing user-token write path
    account_key: connection.account_key

tool:
  name: serpapi
  kind: api-key
```

- `credential_input` omitted → the client's implicit single-token default connect form (per `credentialInputManifest` doc comment); `auth.api_key.setup_url` covers "where to get the key".
- The key is stored via the write-only `POST /connections/credentials` API into Vault (manual-token contract); nothing secret in the bundle.
- Icon (separate, manual): `ui/helio-app/src/integrations/icons/serpapi.svg` + `providerIcons.ts` — **batch-end registration**; SVG itself merges freely.
- AI-facing docs: provider sub-doc under `agents/plugins/heliox/skills/tool/` (serpapi usage: engine selection, `--param` escape hatch, archive-for-free pattern, quota check) — publish rides the batch-end plugin bump.
- Generated projections: run `go run ./cmd/provider-gen` + `--check` locally for validation only — **not committed** (master plan §2; batch lead owns the canonical regen). Local uncommitted `replace github.com/heliohq/anycli => <this worktree>` in `helio-cli/go.mod` is the sanctioned L4 build path.

## 5. Test plan (five layers)

| Layer | What runs | External credentials? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in anycli. httptest fakes assert: `api_key` injected as query param on `/search`, `/searches/<id>.json`, `/account.json`; `locations` sends **no** key; `--param k=v` passthrough and flag→param mapping; provider-JSON passthrough; `account` output has `api_key` redacted; non-2xx `{"error": …}` → exit 1 plain + `--json` envelope; **401 → `CredentialRejected`** regression; usage errors → exit 2. TDD: tests written first per anycli AGENTS.md. | No |
| **L2** real-API harness | `make build-harness`; `ANYCLI_CRED_API_KEY=<key> anycli serpapi -- account` (identity + quota), `… -- search -q "coffee" --engine google --num 5`, `… -- archive get <id from the search>`, `… -- locations --q austin --limit 3`. Success = real SerpApi data. Mandatory before the pin bump. | **Yes — SerpApi API key from the account pool** (self-serve free tier, 100 searches/month, is sufficient) |
| **L3** generation + suites | Local `go run ./cmd/provider-gen` + `--check` against the branch bundle (uncommitted); integration-service `go test ./...` (including the new `in: query` verifier + generator tests); `cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/` with the local anycli `replace`. Branch is *expected* to fail CI's `provider-gen --check` until batch-end regen. | No |
| **L4** singleton + seed | `make run-singleton`; `POST /internal/test-only/connections/seed` with `provider: serpapi`, real ObjectID identities, and `access_token: <real SerpApi key>` (non-expiring key → seed `access_token` only, no `refresh_token`/`expires_at`); then `heliox tool serpapi -- search -q "helio.im" --num 3` through the real token gateway. api_key providers are seedable (only minted ones are rejected). | **Yes — same real key** (L4 success = seeded key reaching the live API) |
| **L5** key-entry connect flow | The api_key L5 path (master plan §2 lane 3, agent-drivable): connect link → paste key in the real connect UI → stored via `POST /connections/credentials`, verified against `account.json` via the new query-param verifier → connection shows connected in `GET /connections` → one **unseeded** live `heliox tool serpapi -- search …`. Runs after batch-end merge; gates the visible flip. | **Yes — same real key**; agent-browser drivable, human fallback |

**Definition of done** (per master plan §2): all five layers green, docs published, icon registered, then `presentation.visible: true` + regen as the single go-live change.

## 6. Open items for the batch lead

- The §4 generic `in: query` capability is a shared integration-service change other query-param-key tools in the api_key lane will want — coordinate so it lands once, not N times.
- `presentation.order`: pick an unoccupied value at batch-end merge.
- i18n: `tools.desc.serpapi` description key needs locale strings (all 9 locales) before the visible flip.
