# Tool design — Intercom

Catalog row 21: Intercom | anycli id `intercom` | provider key `intercom` | lane `oauth_review` | Wave 1 | Support.
Branches: anycli `tool/intercom`, Helio `tool/intercom`. Scratch design file — stripped by the batch lead at batch end.

## 1. What an AI teammate does with Intercom, and the API surface that serves it

Intercom is a customer-support workspace: a shared inbox of **conversations**, a
**contacts/companies** CRM, **tickets**, and a **Help Center** of articles. An AI
teammate's real jobs are support triage and support-adjacent research:

- read and search the inbox ("any open conversations about billing since Monday?"),
- act on a conversation (reply as the team, leave an internal note, close/snooze/open,
  assign to an admin or team, tag),
- look up who a customer is (contact + company records, create/update them, attach notes),
- work tickets (create from a conversation or fresh, search, update state, reply),
- search and maintain Help Center articles (find the doc to link a customer; draft/update),
- proactive outreach (send an admin-initiated message to a contact),
- orient in the workspace (who are the admins/teams, what tags exist, who am I).

All of this is the official **Intercom REST API** at `https://api.intercom.io`
(`Authorization: Bearer <token>`, versioned via the `Intercom-Version` header; current
reference version 2.15; docs: https://developers.intercom.com/docs/references/rest-api/api.intercom.io).
`api.intercom.io` routes to the workspace's region (US/EU/AU) automatically, so the
service needs no regional base-URL handling.

Wrapped endpoints (verbs per resource, chosen against the jobs above — not API-complete):

| Resource | Endpoints |
|---|---|
| conversation | `GET /conversations` (list), `POST /conversations/search`, `GET /conversations/{id}`, `POST /conversations/{id}/reply` (comment or note, `admin_id`), `POST /conversations/{id}/parts` (`close` / `snoozed` / `open` / `assignment`), `POST /conversations/{id}/tags`, `DELETE /conversations/{id}/tags/{tag_id}` |
| contact | `GET /contacts`, `POST /contacts/search`, `GET /contacts/{id}`, `POST /contacts`, `PUT /contacts/{id}`, `POST /contacts/{id}/notes`, `POST /contacts/{id}/tags` |
| company | `GET /companies`, `GET /companies/{id}`, `POST /companies` (upsert by `company_id`) |
| ticket | `POST /tickets`, `POST /tickets/search`, `GET /tickets/{id}`, `PUT /tickets/{id}`, `POST /tickets/{id}/reply`; `GET /ticket_types` |
| article | `GET /articles`, `GET /articles/{id}`, `GET /articles/search?phrase=…&state=…`, `POST /articles`, `PUT /articles/{id}`; `GET /help_center/collections` |
| message | `POST /messages` (admin-initiated outbound message) |
| admin | `GET /me`, `GET /admins`, `GET /admins/{id}` |
| team | `GET /teams`, `GET /teams/{id}` |
| tag | `GET /tags`, `POST /tags` (create/update) |

Deliberately out of scope for v1: data events, segments, subscriptions/webhooks
management, news items, data exports, visitors, Fin/AI endpoints. They serve
integration builders more than a support teammate; add on demand.

Pagination: Intercom is cursor-based (`starting_after` in `pages.next`, `per_page`);
search endpoints take `pagination: {per_page, starting_after}` in the POST body.
Exposed as `--per-page` / `--starting-after` flags; the provider's `pages` object is
passed through in output so the model can continue.

## 2. anycli definition

**Stage-1 form decision: `service` type.** No official agent-friendly Intercom CLI
exists (the rubric's cli-type conditions all fail). Standard `internal/tools/`
service against the REST API, like 21 of 23 existing definitions.

`definitions/tools/intercom.json`:

```json
{
  "name": "intercom",
  "type": "service",
  "description": "Intercom as a tool (workspace OAuth token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "INTERCOM_ACCESS_TOKEN"}
      }
    ]
  }
}
```

Service package `internal/tools/intercom/` (id has no dashes → package name is the
id), registered in `internal/tools/register.go` as `RegisterService("intercom", …)`
**at batch end only** (register.go is a shared surface; the definition JSON and the
package merge freely mid-batch).

Shape follows the notion/bitly template and design 003 §3 conventions:

- `Service{BaseURL, HC, Out, Err}`; zero values default to production
  (`https://api.intercom.io`) — httptest servers plug in for tests.
- Client pins `Intercom-Version: 2.15` and `Accept: application/json` on every
  request; auth `Authorization: Bearer` from `env["INTERCOM_ACCESS_TOKEN"]`;
  missing credential is exit 1 with an explicit message.
- Cobra tree built per `Execute` call; non-interactive; flags only.
- Exit codes: 0 success, 1 runtime/API failure, 2 usage/parse errors.

Subcommand tree (resource → verb, mirroring §1):

```
intercom conversation list|search|get|reply|note|close|open|snooze|assign|tag|untag
intercom contact      list|search|get|create|update|note|tag
intercom company      list|get|upsert
intercom ticket       create|search|get|update|reply|type-list
intercom article      list|get|search|create|update|collection-list
intercom message      send
intercom admin        me|list|get
intercom team         list|get
intercom tag          list|create
```

Notes:
- `conversation reply` posts `message_type: comment`; `conversation note` posts
  `message_type: note` — separate verbs, both on `POST /conversations/{id}/reply`,
  because "answer the customer" vs "internal note" must never be confusable for an
  agent. `--admin-id` selects the acting teammate (default: the `/me` admin id,
  fetched lazily when the flag is absent).
- `search` verbs take `--query '<json>'` (the raw Intercom query object — the API's
  operator/AND/OR model is itself JSON and models handle it well) plus convenience
  flags for the common cases (`--state`, `--email`, `--updated-since`) that compile
  into a query object; `--query` and convenience flags are mutually exclusive.
- **JSON output**: provider JSON passed through verbatim on stdout (+ newline), per
  design 003 §3. Errors: Intercom returns non-2xx with
  `{"type":"error.list","errors":[{"code","message"}]}` — rendered as a one-line
  stderr message carrying code + message, exit 1; `--json` accepted for uniformity
  (output is always JSON).

## 3. Auth — verifying the `oauth_review` lane against official docs

Verified against https://developers.intercom.com/docs/build-an-integration/learn-more/authentication/setting-up-oauth
and https://developers.intercom.com/docs/build-an-integration/learn-more/authentication/installing-uninstalling-apps:

- **Registration model**: apps are created self-serve in the Developer Hub; every
  development workspace gets OAuth via the "Use OAuth" toggle. But **all public
  apps — listed and unlisted — must pass Intercom App Store review** before
  workspaces outside the developer's own can authorize; review checks scopes and
  the OAuth flow (up to 7 business days). **The audit verdict and the
  `oauth_review` lane are confirmed correct.** Dev-mode work (the developer's own
  dev workspace) needs no review, so L2/L4/L5 dry runs are unblocked; only the
  visible flip waits on review clearance, exactly as the master plan schedules.
- **Flow**: authorization-code. Authorize `https://app.intercom.com/oauth` with
  `client_id`, `state` (+ optional `redirect_uri`, HTTPS-only). **No `scope`
  parameter exists** — scopes are checkbox-configured on the app in the Developer
  Hub and frozen at review time.
- **Token exchange**: `POST https://api.intercom.io/auth/eagle/token`, form-encoded
  `code`, `client_id`, `client_secret` (secret in body, no Basic auth) → JSON
  `{token_type: "Bearer", token, access_token}` (`access_token` duplicates `token`).
- **Token semantics**: **no refresh token, no `expires_in`** — the token is
  non-expiring until revoked/uninstalled; re-auth requires the full flow again.
- **Identity**: `GET https://api.intercom.io/me` returns the authorizing admin
  (`id`, `email`, `email_verified`) plus the workspace `app`
  (`id_code`, `name`, `region`).
- **Revocation**: `POST https://api.intercom.io/auth/uninstall` with
  `Authorization: Bearer <token>` uninstalls the app / revokes the token.

Credential field consumed by anycli: `access_token` only (plus `account_key`,
projected for multi-account resolution like every bundle).

### Divergences / implementation checks recorded

1. **Provider-side revoke is out of the declarative capability set.** Intercom's
   uninstall endpoint authenticates via the Bearer header; integration-service's
   `declarativeOAuthRevoker` (`service/revoke.go`) intentionally speaks only the
   RFC-7009 form-POST dialect (token in the form body, none/basic/form client
   auth). Choice: `disconnect_mode: local_only` (the notion precedent — same
   non-expiring-token, no-refresh OAuth shape) rather than a narrow adapter for a
   cosmetic-cleanup call. If provider-side uninstall is later wanted, the right
   move per the skill is growing the reviewed capability set (a bearer-header
   token-delivery enum) — flagged for the batch lead, not built here.
2. **Regional authorize hosts.** Authorize pages are regional
   (`app.intercom.com` / `app.eu.intercom.com` / `app.au.intercom.com`) while
   `api.intercom.io` self-routes. The bundle carries one `authorize_url`; v1 ships
   the US host. Known limitation: EU/AU-hosted workspaces may fail the consent hop
   (docs call out Google-SSO admins failing on a wrong regional host). Recorded
   for the wave board; fix would be provider-level authorize-host selection, a
   platform capability, not per-tool.
3. **Authorize URL extra params.** Intercom documents only
   `client_id`/`state`/`redirect_uri`; the standard flow appends
   `response_type=code`. Expected harmless (standard OAuth servers ignore or
   accept it) — verify at L5 and record if Intercom rejects it.
4. **Exchange response carries no `expires_in`** — token gateway must treat the
   token as non-expiring (same path Notion exercises today); L4 seeds
   `access_token` only, per the skill's non-expiring-token seeding guidance.

## 4. Helio provider bundle plan

Naming axes (master plan §3): ① CLI command `intercom` (flat command, no group) ·
② anycli id `intercom` · ③ provider key `intercom`. ② == ③ → **no
`toolToProvider` entry** in `helio-cli/internal/toolcred/resolver.go`.

`integrations/providers/intercom/provider.yaml` (hidden-first; lands at batch end
with the single canonical `provider-gen` run — local regens are validation-only and
never committed):

```yaml
schema: helio.provider/v1
key: intercom
go_name: Intercom

presentation:
  name: Intercom
  description_key: intercom
  consent_domain: intercom.com
  visible: false          # oauth_review: flip gated on App Store review clearance + L5
  order: <assigned at batch end>

auth:
  type: oauth
  owner: assistant        # workspace-level install, notion precedent
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://app.intercom.com/oauth
    token_url: https://api.intercom.io/auth/eagle/token
    token_exchange_style: form_secret     # code+client_id+client_secret form body
    pkce: none
    display_scopes: [read_conversations, write_conversations, read_contacts,
                     write_contacts, read_tickets, write_tickets, read_articles,
                     write_articles, read_admins, read_tags, write_tags]
    single_active_token: false
    refresh_lease: none                   # no refresh token exists

identity:
  source: userinfo
  url: https://api.intercom.io/me
  stable_key: /app/id_code                # workspace identity, not admin identity
  label_candidates: [/app/name, /email, /app/id_code]

connection:
  mode: isolated
  disconnect_mode: local_only             # see divergence 1
  runtime_strategy: standard_oauth        # zero service-side Go

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: intercom
  kind: oauth
```

`display_scopes` mirror the Developer Hub checkbox selection (scopes are app-config,
not authorize-time — §3); keep the app's actual selection and this list in lockstep
at review submission. `standard_oauth` needs **zero** integration-service code.
Client id/secret land via lane 1 as per-provider appends to `config/` + the Helm
Secret in `deploy/` together (Config Sync rule); dev credentials arrive as
uncommitted local `config/cloud.yaml` entries for L4.

Batch-end shared surfaces this tool contributes: `register.go` entry, anycli tag +
pin bump, bundle + one `provider-gen` run, icon
`ui/helio-app/src/integrations/icons/intercom.svg` + `providerIcons.ts` append,
provider sub-doc under `agents/plugins/heliox/skills/tool/` + plugin version bump.
No resolver-map entry, no experiment flag (GA once visible).

## 5. Test plan — the five layers

| Layer | What runs | External credentials needed |
|---|---|---|
| L1 | anycli `go test ./...`: httptest fakes assert method/path/query, `Authorization: Bearer`, `Intercom-Version: 2.15` header, search-body construction (query JSON + convenience-flag compilation + pagination), verbatim JSON passthrough, `error.list` rendering + exit codes (0/1/2), missing-credential exit 1. TDD: tests first per anycli AGENTS.md. | None |
| L2 | Dev harness against the **real** API: `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli intercom -- admin me`, then one read (`conversation list`), one search, one write+cleanup (`tag create`, `contact create`/`update`). Mandatory before the pin bump. | **Yes** — an Intercom dev-workspace Access Token (a private-app token has identical Bearer semantics to an OAuth token; lane 2 supplies the dev workspace) |
| L3 | Local-only `provider-gen` + `provider-gen --check` against the branch bundle (regens NOT committed); `helio-cli` built with a local, uncommitted `go.mod` `replace` to this anycli worktree; both repos' unit suites green. | None |
| L4 | Singleton + `POST /internal/test-only/connections/seed` with `provider: "intercom"` and `access_token` only (no refresh/expiry — non-expiring token, nothing to refresh-exercise), then `heliox tool intercom -- conversation list` etc. reaching the live API through the real token gateway. | **Yes** — a real token from the dev workspace (lane 1's dev-mode app or the private-app token); real seeded org/assistant identities from local Mongo |
| L5 | Human-in-the-loop (lane 3), tool still hidden: `heliox tool intercom auth` → connect link → real consent on the dev workspace (pre-review, only the developer's own workspaces can authorize — sufficient for L5) → `oauth_connected` event on channel → one unseeded live run. Also verifies divergence 3 (`response_type=code` tolerance) and identity extraction (`/app/id_code`). | **Yes** — registered Intercom app (client id/secret configured in integration-service) + a dev workspace admin login |

Visible flip: after L5 **and** Intercom App Store review clearance (oauth_review
lane), as the single `visible: true` + regenerate change.
