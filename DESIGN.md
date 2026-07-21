# RocketReach — per-tool design (`heliox tool rocketreach`)

Branch: `tool/rocketreach` (both repos). Scratch design file; batch lead strips at batch-end.

- **Catalog row** (008 §4, row 67): Product RocketReach · anycli id `rocketreach` · provider key `rocketreach` · auth `api_key` · Wave 2 · Sales Engagement.
- **Audit verdict** (oauth-audit.md row 69): *"no viable multi-tenant path → api_key. Stays api_key per rubric."*
- **Independent verification of the auth lane** (official docs, below): CONFIRMED `api_key`. RocketReach exposes a single per-user API key sent in an `Api-Key` request header. There is **no** authorization-code OAuth flow, no app-registration/consent surface, no partner client model. RocketReach ships an official **MCP server**, but the master plan explicitly scopes the Figma-style MCP path OUT of this pipeline; we wrap the REST API as an AnyCLI passthrough. No divergence from the catalog/audit to record.

---

## 1. Which official API surface, and why

**Product intent driving the surface.** RocketReach is a contact-enrichment / prospecting database. What an AI teammate actually does with it: (a) *enrich* a known person (name+company, or a LinkedIn URL, or a stored profile id) into verified emails/phones; (b) *find* people by role/company/location to build a prospect list; (c) *look up / find* companies (firmographics); (d) *check remaining credits* before spending, because lookups burn a finite credit balance. The tool wraps exactly the endpoints that serve those jobs and nothing that needs out-of-band infrastructure (webhooks) or a different credit/billing model.

**Base:** `https://api.rocketreach.co`, path prefix `/api/v2`. **Auth:** `Api-Key: <key>` header on every request (query-param `?api_key=` is also accepted but the header is the documented/canonical form and the only one we inject).

**Endpoints wrapped (classic v2):**

| Job | Method + path | Notes |
|---|---|---|
| Account / credits (also the **verify** endpoint) | `GET /api/v2/account/` | Returns the `UserModel`: `id` (int), `first_name`, `last_name`, `email`, `state`, `credit_usage[]` (`credit_type`/`allocated`/`used`/`remaining`, `"inf"` when unlimited), `rate_limits[]`. Cheap, non-consuming — the teammate calls it to check budget before lookups. |
| Person lookup (enrich) | `GET /api/v2/person/lookup` | Query by `name`+`current_employer`, or `linkedin_url`, or `id` (profile id from a prior search). **Asynchronous**: returns a lookup object with `status` ∈ `complete`/`searching`/`waiting`/`progress`/`failed`. Emails/phones populate as `status` reaches `complete`. Credits are only charged on a match. |
| Person lookup status | `GET /api/v2/person/checkStatus?ids=<id>[,<id>…]` | Poll the async lookups above. (RocketReach recommends webhooks over polling, but webhooks need an inbound endpoint the runtime does not have, so polling is the agent-usable path.) |
| Person search | `POST /api/v2/person/search` | JSON body `{"query":{…}, "start":N, "page_size":M}`. Returns matching profiles (name/title/employer/**profile id**) but **no contact info** — the agent then `person lookup --id <profileId>` to enrich a chosen result. |
| Company lookup | `GET /api/v2/company/lookup` | Firmographics by `name` or `domain`. |
| Company search | `POST /api/v2/company/search` | JSON body query; returns matching companies. |

**Deliberately excluded:** `POST /api/v2/person/bulkLookup` (results are delivered only to a caller-hosted webhook — no synchronous return the CLI can hand back; single lookup + `checkStatus` covers the agent case). The starred **Universal API** family (`/reference/…universal…`) is a newer unified-credit surface layered over the same account; it is a follow-up (own credit semantics), not the first cut — classic v2 is stable, fully documented, and what every current "How do I…" KB article uses.

**Sources:**
- People Lookup / Search / Status / Account references — https://docs.rocketreach.co/reference/people-lookup-api , https://docs.rocketreach.co/reference/people-search-api , https://docs.rocketreach.co/reference/rocketreach-api-account
- Company Lookup / Search — https://docs.rocketreach.co/reference/company-lookup-api , https://docs.rocketreach.co/reference/company-search-api
- Getting-started (auth header, key generation) — https://knowledgebase.rocketreach.co/hc/en-us/articles/33969455800091
- OpenAPI index — https://docs.rocketreach.co/llms.txt

---

## 2. AnyCLI definition (stage-1 form + shape)

**Type: `service`** (stage-1 rubric). No official RocketReach CLI binary exists; the surface is a plain JSON HTTP API. This is the default and matches 21/23 shipped definitions. So: `definitions/tools/rocketreach.json` (`"type":"service"`, `"name":"rocketreach"`) + a Go service package `internal/tools/rocketreach/` registered as `RegisterService("rocketreach", &rocketreach.Service{})` in `internal/tools/register.go`. Go package name = anycli id with no normalization needed (`rocketreach` — no dash, no leading digit).

**Definition auth binding.** One `CredentialBinding`: source `field: api_key` → inject `type: env` (e.g. `ROCKETREACH_API_KEY`); the service reads the env var and sets the `Api-Key` header on every outbound request. (Mirrors `slack.json`'s single-field shape; header is set in code, not by a static definition header, exactly like the other service-type tools.)

**Command tree** (cobra, grouped by resource — copy `internal/tools/notion/` shape: `BaseURL`/`HC`/`Out`/`Err` struct so tests point at an `httptest` server; exit codes 0 ok / 1 API-or-runtime failure via typed `apiError` / 2 usage-parse; `--json` structured error envelope):

```
heliox tool rocketreach -- account
heliox tool rocketreach -- person lookup   --name "Jane Doe" --current-employer "Acme"
heliox tool rocketreach -- person lookup   --linkedin-url https://linkedin.com/in/janedoe
heliox tool rocketreach -- person lookup   --id 807344
heliox tool rocketreach -- person status   --ids 807344,807345
heliox tool rocketreach -- person search   --name "Jane Doe" --current-employer Acme --title VP --page-size 10 [--json-query '<raw query JSON>']
heliox tool rocketreach -- company lookup  --domain acme.com
heliox tool rocketreach -- company search  --name Acme [--json-query '<raw query JSON>']
```

- Search verbs accept the common structured filters as repeatable flags AND a `--json-query` escape hatch for the full RocketReach query object (the search schema is broad; flags cover the frequent axes, raw JSON covers the rest — same pattern used by other search-bearing tools in the batch).
- `--json` on every verb. `person lookup` always surfaces the `status` field so the agent knows whether to `person status` next.

**JSON output shape.** Pass provider JSON through under a stable envelope so the agent gets provider-neutral structure without us reshaping RocketReach's payloads:

```json
{ "ok": true, "tool": "rocketreach", "action": "person.lookup",
  "data": { "id": 807344, "status": "complete", "name": "Jane Doe",
            "current_employer": "Acme", "emails": [...], "phones": [...] } }
```
Error envelope (exit 1):
```json
{ "ok": false, "tool": "rocketreach", "action": "person.lookup",
  "error": { "http_status": 401, "code": "unauthorized", "message": "..." } }
```
`account` maps `credit_usage[]` straight through so the agent can read `remaining` (including `"inf"`).

---

## 3. Credential fields & auth flow (api_key lane — verified)

- **Registration model:** self-serve. A RocketReach user with API access generates a key in-app ("Generate API Key"); generating a new key invalidates the previous one. No app registration, no OAuth client, no review — this is exactly why the audit keeps it `api_key`.
- **Credential shape:** one secret string. The user pastes it through Helio's write-only `POST /connections/credentials`; it is stored in Vault and never touches the bundle.
- **Token semantics:** long-lived, non-expiring bearer-style key. No refresh cycle. Injected as the `Api-Key` header. `401` = missing/invalid key; `403` = key lacks permission; `429` = rate/credit limit.
- **Scopes:** none — a key carries the account's full API entitlement gated only by the account's plan + credit balance.

---

## 4. Helio provider bundle plan (`integrations/providers/rocketreach/provider.yaml`, hidden-first)

**Three axes** (008 §3): ① CLI command `rocketreach` (flat, ungrouped) · ② anycli id `rocketreach` · ③ provider key `rocketreach`. All three identical → **no `toolToProvider` divergence entry** and no `providers_gen.go` group. Clean.

This is the standard **`manual_api_token`** header-plus-verify shape (same class as the other api_key contact tools in this wave). The account endpoint doubles as the pre-write verification/identity endpoint, so a bad key is rejected at connect time, not at first tool use.

```yaml
schema: helio.provider/v1
key: rocketreach
go_name: RocketReach

presentation:
  name: RocketReach
  description_key: rocketreach
  consent_domain: rocketreach.co
  visible: false            # hidden-first; flip only after L5 + pinned anycli ships the tool

auth:
  type: api_key
  owner: individual
  api_key:
    header: Api-Key
    setup_url: https://rocketreach.co/api          # where the user generates the key

identity:
  source: userinfo
  url: https://api.rocketreach.co/api/v2/account/
  stable_key: /id                                  # immutable numeric account id (numeric stable_key already supported on main)
  label_candidates: [/email, /first_name]

connection:
  mode: isolated
  disconnect_mode: local_only
  runtime_strategy: manual_api_token

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: rocketreach
  kind: api-key
```

- **No integration-service capability growth required.** `manual_api_token` + `identity.source: userinfo` + header verify is the existing golden path (`service/manual_token_verifier.go`); numeric `stable_key` (`/id`) is already supported on main (hubspot precedent). No adapter — RocketReach's response/lifecycle sits inside the closed capability set. Zero service Go.
- **No client id/secret config** — manual-token providers need none; nothing lands in `config/`/`deploy/` for this provider (no human lane-1 OAuth-app work). This is a pure api_key tool.
- **Icon:** `ui/helio-app/src/integrations/icons/rocketreach.svg` + hand-register in `providerIcons.ts` (manual, never generated).
- **Docs:** provider sub-doc under `agents/plugins/heliox/skills/tool/`; plugin version bump + marketplace publish ride the batch-end merge.

---

## 5. Test plan → five layers

| Layer | What runs for RocketReach | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...`: `httptest` fake for `account`, `person lookup` (assert async `status` surfaced), `person search`/`company search` (assert POST JSON body + `Api-Key` header injected), `person status`, error rendering (plain + `--json`, 401/403/429 → exit 1; usage errors → exit 2). No real network. | No |
| **L2** harness real API | `ANYCLI_CRED_API_KEY=<key> anycli rocketreach -- account`, then `person lookup` / `person search` / `company lookup` against the live API. Proves field names, header injection, and async status handling match reality. Mandatory before the pin bump. | **Yes** — real RocketReach key (account pool, lane 2) |
| **L3** generate + suites | `provider-gen` + `provider-gen --check` (five projections); helio-cli + integration-service `go test`. On-branch: local regen + `go.mod` `replace` → anycli branch (not committed). | No |
| **L4** singleton + seed | `POST /internal/test-only/connections/seed` (`provider: rocketreach`, `access_token: <key>`) — non-expiring key, seed `access_token` only, omit refresh/expires — then `heliox tool rocketreach -- account` reaches the live API through the real token gateway. | **Yes** — real key seeded |
| **L5** full connect | Hidden tool: open connect link → paste the key through the real connect UI (stored via `POST /connections/credentials`, **verified against `/api/v2/account/`** before the Vault write) → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool rocketreach -- account` succeeds. api_key L5 is agent-drivable (agent-browser) per master plan §2, human fallback on UI breakage. Run once before the visible flip. | **Yes** — real key + connect UI |

**Rollout:** land hidden → pin bump ships the anycli `rocketreach` definition → L1–L5 green while hidden → flip `presentation.visible: true` + regenerate as the single go-live change.
