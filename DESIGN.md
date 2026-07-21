# Tool design: Typeform

Per-tool design for the `typeform` provider (master plan row 48, Wave 1, Forms &
Surveys). Scratch file on branch `tool/typeform`; the batch lead strips it at
batch end.

- anycli id (axis ②): `typeform`
- provider catalog key (axis ③): `typeform`
- CLI command (axis ①): `typeform` (flat command, no group; ② == ③ → **no**
  `toolToProvider` entry)
- Auth lane: `oauth_light` — **confirmed** against official docs (see §3)

## 1. What an AI teammate does with Typeform, and the API surface chosen

Typeform is a form/survey product. The realistic AI-teammate jobs are:

1. **Read survey results** (the dominant job): pull responses for a form,
   filter by date window / completeness / text query, and summarize for the
   team. → Responses API.
2. **Inspect a form's structure** to interpret answers (field ids, refs,
   types, choice labels) — required to make sense of the `answers` array. →
   Create API `GET /forms/{id}`.
3. **Find the right form**: list/search forms across workspaces. →
   `GET /forms`.
4. **Author or edit forms**: draft a new survey from a team request, tweak
   questions/settings on an existing one. → `POST /forms`,
   `PUT/PATCH /forms/{id}`, `DELETE /forms/{id}`.
5. **Organize**: list/inspect workspaces so form creation lands in the right
   place. → Workspaces endpoints.
6. **Wire notifications**: attach a webhook so new responses reach some
   endpoint. → Webhooks endpoints (`PUT /forms/{id}/webhooks/{tag}` et al.).
7. **Verify identity / whoami**: `GET /me`.

Deliberately **excluded** in v1: Themes and Images APIs (visual asset
management — not teammate work), response **deletion** (`responses:write`,
destructive respondent-data removal; add later only with an explicit ask),
Insights (not part of the documented Create/Responses/Webhooks reference
surface), file/audio-video download endpoints (deferred; large-binary
handling has no precedent need yet).

All endpoints are on base `https://api.typeform.com`, auth
`Authorization: Bearer <token>` (personal access token and OAuth access token
are interchangeable — this is what makes the L2 harness runnable with a plain
personal token).

**EU data-center caveat (recorded divergence):** accounts homed in Typeform's
EU DC must use `https://api.eu.typeform.com` (or `https://api.typeform.eu` for
newer EU accounts); `api.typeform.com` will not serve their data. v1 targets
the global base URL only (matching the single-base-URL shape of every shipped
service tool). The service keeps the notion-precedent `BaseURL` override field,
so an EU base can later be threaded through as a credential/config field
without an API-shape change. Documented as a known limitation in the AI-facing
provider sub-doc.

## 2. anycli definition

**Stage-1 rubric: `service` type.** Typeform ships no official CLI, so the
`cli`-type conditions fail at the first test. Service implementation against
the REST API, following `internal/tools/notion/` as the shape precedent.

`definitions/tools/typeform.json`:

```json
{
  "name": "typeform",
  "type": "service",
  "description": "Typeform forms and survey responses (OAuth token)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "TYPEFORM_TOKEN"}
      }
    ]
  }
}
```

- Package: `internal/tools/typeform/` (id has no dashes → package name is the
  id verbatim).
- Registration: `RegisterService("typeform", &typeform.Service{})` in
  `internal/tools/register.go` — **batch-end shared surface**, not committed
  mid-batch; the definition JSON is conflict-free and merges freely.
- Service struct: notion-precedent `BaseURL` / `HC` / `Out` / `Err` fields so
  unit tests point at `httptest.Server` and capture output.

### Subcommand tree (cobra, grouped by resource)

```
typeform me
typeform form list      [--search s] [--workspace-id id] [--page n] [--page-size n]
                        [--sort-by created_at|last_updated_at] [--order-by asc|desc]
typeform form get       <form_id>
typeform form create    --definition <json|@file>          # POST /forms
typeform form update    <form_id> --definition <json|@file>  # PUT (full)
typeform form patch     <form_id> --patch <json|@file>       # PATCH (partial)
typeform form delete    <form_id>
typeform response list  <form_id> [--page-size n] [--since t] [--until t]
                        [--after tok] [--before tok] [--response-type completed|partial|started]
                        [--query s] [--fields f1,f2] [--answered-fields f1,f2] [--sort spec]
typeform workspace list [--page n] [--page-size n] [--search s]
typeform workspace get  <workspace_id>
typeform workspace create --name <name>
typeform webhook list   <form_id>
typeform webhook get    <form_id> <tag>
typeform webhook set    <form_id> <tag> --url u [--enabled] [--secret s] [--verify-ssl]
                        # PUT /forms/{form_id}/webhooks/{tag} (create-or-update)
typeform webhook delete <form_id> <tag>
```

Verb conventions match the built-in service conventions (design 003 §3):
`list/get/create/update/delete`; `webhook set` reflects the API's upsert-by-tag
semantics. `--definition`/`--patch` accept inline JSON or `@file`, mirroring
how other service tools pass large JSON bodies non-interactively.

### Output & error contract

- Exit codes: 0 success; 1 runtime/API failure (typed `apiError` wrapping
  Typeform's `{code, description, details, help}` error body); 2 usage/parse.
- `--json` global flag: raw provider JSON on success (list envelopes keep
  `total_items` / `page_count` / `items` verbatim so agents can paginate);
  structured error envelope on failure. Plain-text rendering otherwise, per
  the notion precedent.
- Pagination is surfaced, not auto-drained: `form list` exposes
  `--page/--page-size` (max 200); `response list` exposes the token cursors
  `--after/--before` (max page size 1000) — the agent drives traversal.
- Response items are passed through untransformed: `answers[].field.{id,ref,type}`
  plus typed values. `form get` gives the field dictionary an agent needs to
  join against; no lossy flattening in v1.

## 3. Auth: `oauth_light` verified against official docs

Source: `https://www.typeform.com/developers/get-started/applications/` and
`.../get-started/scopes/` (fetched 2026-07-21).

- **Registration model:** fully self-serve — admin panel → Developer Apps →
  "Register a new app" (name, website URL, redirect URIs). No review gate
  documented; client_secret shown once, regeneratable. **Audit verdict
  (oauth_light, high confidence) confirmed; no divergence.**
- **Flow:** standard RFC 6749 authorization code.
  - Authorize: `https://api.typeform.com/oauth/authorize` with `client_id`,
    `redirect_uri`, space-delimited `scope`, optional `state`.
  - Token: `POST https://api.typeform.com/oauth/token`, URL-encoded form body
    carrying `client_id` + `client_secret` (no Basic-auth option documented)
    → `token_exchange_style: form_secret`.
  - **PKCE: not supported/documented** → `pkce: none`.
- **Token semantics:** access tokens expire (default **1 week**). A refresh
  token is issued **only when the `offline` scope is requested**. Refresh
  (`grant_type=refresh_token`, client id+secret in form body) **rotates** the
  refresh token — the old one is invalidated and each refresh response
  carries a new one, so the token gateway's refresh-and-write-back (A3) must
  persist the rotated refresh token. This is the standard_oauth path already
  exercised by other providers; L4 deliberately re-verifies rotation here
  (§6). Refresh `scope` can only narrow, never widen — adding scopes later
  requires a reconnect, so the scope list below is chosen complete for the v1
  surface up front.
- **Scopes requested:**
  `offline accounts:read forms:read forms:write responses:read workspaces:read workspaces:write webhooks:read webhooks:write`.
  Excluded: `responses:write` (destructive delete-responses; no v1 command),
  `themes:*`, `images:*` (out of surface).
- **Identity:** `GET https://api.typeform.com/me` (needs `accounts:read`).
  Officially documented fields: `alias`, `email`, `language`; multiple
  non-official references also show `user_id`. Bundle plan uses
  `stable_key: /user_id` with `label_candidates: [/email, /alias]` — **the
  presence of `user_id` in the live `/me` payload is an explicit L2
  verification item**; if absent, fall back to `stable_key: /email` before the
  bundle merges.
- **Revocation:** no OAuth token-revocation endpoint documented →
  `disconnect_mode: local_only` (notion/microsoft precedent).

## 4. Helio provider bundle plan

`integrations/providers/typeform/provider.yaml` (held to batch-end merge; the
five provider-gen projections regenerate together then — run locally for
validation only, never committed from this branch):

```yaml
schema: helio.provider/v1
key: typeform
go_name: Typeform

presentation:
  name: Typeform
  description_key: typeform
  consent_domain: typeform.com
  visible: false          # hidden-first; flip is the separate go-live change
  order: 130

auth:
  type: oauth
  owner: assistant
  required_config_fields: [oauth.client_id, oauth.client_secret]
  oauth:
    authorize_url: https://api.typeform.com/oauth/authorize
    token_url: https://api.typeform.com/oauth/token
    token_exchange_style: form_secret
    pkce: none
    scopes:
      - offline            # refresh token; access tokens live ~1 week
      - accounts:read      # GET /me identity
      - forms:read
      - forms:write
      - responses:read
      - workspaces:read
      - workspaces:write
      - webhooks:read
      - webhooks:write
    single_active_token: false
    refresh_lease: none

identity:
  source: userinfo
  url: https://api.typeform.com/me
  stable_key: /user_id     # L2 verification item; fallback /email (§3)
  label_candidates: [/email, /alias]

connection:
  mode: isolated
  disconnect_mode: local_only    # no documented revoke endpoint
  runtime_strategy: standard_oauth

resources:
  selection: none
  discovery: none
  enforcement: none

credential:
  fields:
    access_token: token.access_token
    account_key: connection.account_key

tool:
  name: typeform
  kind: oauth
```

Notes:

- `runtime_strategy: standard_oauth` — Typeform is exactly the golden path
  (form-body client secret, userinfo identity, no revoke); zero
  integration-service Go.
- Axis alignment: ① `typeform` (flat, no `tool.command`/`tool.group`), ②
  `typeform`, ③ `typeform` — identical, so **no `toolToProvider` entry** in
  `helio-cli/internal/toolcred/resolver.go`.
- No `experiment` gating (GA rollout path, hidden-first only).
- Batch-end shared surfaces owed by this tool: `register.go` entry, provider
  bundle + one provider-gen run, icon SVG registration in
  `ui/helio-app/src/integrations/providerIcons.ts`
  (`icons/typeform.svg`, hand-registered), provider sub-doc under
  `agents/plugins/heliox/skills/tool/` + plugin version bump.
- Lane-1 dependency: dev app registration (self-serve, minutes) must yield
  client id/secret before on-branch L4; values go in local, uncommitted
  `config/cloud.yaml` only, with the committed `config/` + `deploy/` Helm
  Secret appends landing via lane 1 before L5.

## 5. Build/validation mechanics on this branch

- TDD per anycli AGENTS.md: httptest fakes asserting request path/method/query,
  `Authorization: Bearer` injection from `TYPEFORM_TOKEN`, body shape for
  writes, both plain and `--json` error rendering, exit-code contract.
- helio-cli is built against this worktree via a **locally uncommitted**
  `go.mod` `replace github.com/heliohq/anycli => <this worktree>`.
- `provider-gen` / `provider-gen --check` run locally for validation only;
  regenerated projections are **not committed** (batch lead owns the canonical
  regen). This branch is expected to fail `provider-gen --check` in CI until
  batch end.

## 6. Test plan — the five layers

| Layer | What runs here | External credentials needed |
|---|---|---|
| L1 | `go test ./...` in anycli: unit tests for every subcommand against `httptest` fakes (request shape, auth header, pagination params, error envelope, exit codes) | none |
| L2 | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<token> anycli typeform -- me / form list / form get / response list / form create+delete / webhook set+delete` against the **real** API. Also the §3 verification item: confirm `user_id` in `/me`. | **YES** — a Typeform **personal access token** from the test-account pool (lane 2); no OAuth app needed at this layer |
| L3 | local `provider-gen` + `--check` against the branch bundle; anycli + helio-cli + integration-service unit suites with the `replace` in place | none |
| L4 | singleton + `POST /internal/test-only/connections/seed` with a **real OAuth access_token + refresh_token** and a deliberately short `expires_at`, then `heliox tool typeform -- form list`. Must prove: (a) token gateway serves the seeded token to anycli; (b) the refresh path fires and **writes back the rotated refresh token** (Typeform invalidates the old one — a second run after refresh is the check); (c) live data returned | **YES** — dev-app client id/secret (lane 1, local uncommitted `config/cloud.yaml`) + a token pair minted from the dev app against the test account |
| L5 | while still hidden: `heliox tool typeform auth` → connect link → real consent on the test account → `oauth_connected` event on the channel → one **unseeded** live run. Human-in-the-loop (oauth lane), per-batch sweep | **YES** — registered dev app with the committed config appends landed (lane 1) + test-account login (lane 2) |

Only after L5 passes does `presentation.visible: true` + regenerate ship as the
single go-live change.

## 7. Open items / risks

1. `/me` `user_id` field presence — verified at L2; fallback `stable_key:
   /email` (§3).
2. EU data-center accounts are unsupported in v1 (global base URL only);
   documented limitation, `BaseURL` seam kept for a later EU option (§1).
3. Rate limit is 2 req/s per token — surface Typeform's 429/`RATE_LIMITED`
   error verbatim through the `apiError` envelope; no client-side retry loop
   in v1 (fail fast, agent decides).
4. Very recent responses (~last 30 min) may be missing from `response list`
   per official docs — stated in the AI-facing sub-doc so agents don't treat
   an empty window as "no responses".
