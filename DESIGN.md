# Jotform tool — per-tool design

Scratch design for the `jotform` external tool provider (catalog row 49, Wave 1,
Forms & Surveys). Batch lead strips this file at batch-end. English only.

- **anycli id (axis ②):** `jotform`
- **provider catalog key (axis ③):** `jotform`
- **CLI command word (axis ①):** `jotform` (flat command, no group)
- **auth lane:** `api_key`
- **Helio worktree:** `/Users/wenfeng/workspace/helio/2helio/.claude/worktrees/tool-jotform` (branch `tool/jotform`)
- **anycli worktree:** `/Users/wenfeng/workspace/helio/anycli/.claude/worktrees/tool-jotform` (branch `tool/jotform`)

All three naming axes are identical, so **no `toolToProvider` divergence entry**
is added (`ProviderFor("jotform")` returns `"jotform"` by identity). The Go
package under `internal/tools/` is `jotform` (no dash, no leading digit — nothing
to normalize).

---

## 1. Auth lane — verified against official docs

**Verdict: `api_key`. Audit row 49 confirmed, no divergence.**

The 2026-07-21 OAuth audit put Jotform at `api_key` ("no viable multi-tenant
path"). Verified against the official reference (`https://api.jotform.com/docs/`):

- Jotform authenticates with a **per-account API key**, delivered either as the
  HTTP header `APIKEY: <key>` or the query parameter `?apiKey=<key>`. There is no
  authorization-code OAuth server, no registered-app model, and no multi-tenant
  consent flow. A key is minted by the account owner at **My Account → Settings →
  API → Create New Key**, is long-lived (no expiry, no refresh), and is scoped to
  that single account.
- Keys carry an **access level chosen at creation** — **Read Only** (read/download
  only) or **Full Access** (add / edit / delete). A read-only key returns
  `401 "You're not authorized to use ..."` on any write. This is a per-key
  permission, not an OAuth scope, and cannot be widened at request time.
- Regional bases exist: US `https://api.jotform.com`, EU `https://eu-api.jotform.com`,
  HIPAA `https://hipaa-api.jotform.com`. A key only works against its account's
  region (an EU key `401`s on the US base).

This is a textbook manual-token provider: user pastes one long-lived secret,
delivered as a fixed provider-declared header. It maps onto integration-service's
existing **`auth.type: api_key` / `runtime_strategy: manual_api_token`** path with
**zero capability growth** (see §4). Official docs do **not** contradict the audit
lane — no DESIGN divergence to record.

---

## 2. API surface — what an AI teammate does with Jotform, and the endpoints that back it

Jotform is a form/survey builder; a teammate's real jobs are **triaging incoming
responses**, **reporting on submissions**, and **populating/creating forms**. The
wrapped surface is the v1 REST API on `https://api.jotform.com` (v1 is
read-centric; writes require a Full Access key). Endpoints chosen to cover exactly
those jobs, nothing speculative:

| Teammate job | Endpoint(s) | Notes |
|---|---|---|
| Identity / verify key / account health | `GET /user`, `GET /user/usage` | `/user` doubles as the connect-time verifier (§4). `/usage` returns API-call quota + submission/upload counts. |
| Find the right form | `GET /user/forms` | Lists forms with id, title, status, counts. Form id is the number in the form URL. Supports `offset`/`limit`/`filter`/`orderBy`. |
| Inspect a form | `GET /form/{id}`, `GET /form/{id}/questions` | `/questions` returns each field's **qid** — required to read/write submission values correctly. |
| Read responses | `GET /form/{id}/submissions`, `GET /user/submissions`, `GET /submission/{id}` | Per-form and account-wide. Support `offset`/`limit`/`filter`/`orderBy`. |
| Reports & files | `GET /form/{id}/reports`, `GET /user/reports`, `GET /form/{id}/files` | Shareable report views and uploaded-file listings. |
| Organize | `GET /user/folders` | Folder tree for large accounts. |
| Create a response | `POST /form/{id}/submissions` | Full Access key. Values keyed by qid: `submission[<qid>][<subfield>]=...`. |
| Edit / delete a response | `POST /submission/{id}`, `DELETE /submission/{id}` | Full Access key. |

Out of scope for v1 (kept deliberately small): form *creation/editing*
(`PUT /form`, `/form/{id}/properties` — heavy, low teammate value early),
webhooks (inbound plane, not `heliox tool`), and the EU/HIPAA bases (see §6).
The qid dependency means the natural read/write recipe is
`user/forms → form/{id}/questions → …/submissions`; the AI-facing doc (§5)
teaches that sequence explicitly.

---

## 3. anycli definition & service

### 3.1 Type: `service` (per stage-1 rubric)

No official Jotform CLI binary exists that is non-interactive, `--json`-capable,
and provisionable into the runtime image. Jotform ships language SDKs
(`jotform-api-go`, `-python`, …), not a CLI. → **`service` type**, HTTP against
`api.jotform.com`, following the `notion`/`bitly` in-repo reference shape.

### 3.2 Definition JSON (`definitions/tools/jotform.json`)

```json
{
  "name": "jotform",
  "type": "service",
  "description": "Jotform as a tool (API key)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "JOTFORM_API_KEY"}
      }
    ]
  }
}
```

The credential field is **`access_token`** to match integration-service's
provider-neutral projection (`credential.access_token` → the resolver's
`access_token` key; §4). anycli injects it as env `JOTFORM_API_KEY`; the service
sends it as the raw `APIKEY` request header (Jotform's custom scheme — **no
`Bearer` prefix**). Registered in `internal/tools/register.go`:
`RegisterService("jotform", &jotform.Service{})`.

### 3.3 Service package (`internal/tools/jotform/`)

Copy the `notion` service shape: `Service{BaseURL, HC, Out, Err}` for httptest
injection; a cobra tree grouped by resource; documented exit codes (0 success,
1 runtime/API failure via typed `apiError`, 2 usage/parse error); `--json`
structured-error envelope; stdout carries provider JSON verbatim.

Command tree (verbs mirror the §2 jobs):

```
heliox tool jotform -- user                         # GET /user
heliox tool jotform -- usage                         # GET /user/usage
heliox tool jotform -- form list [--limit --offset --filter --orderby]   # GET /user/forms
heliox tool jotform -- form get <formID>             # GET /form/{id}
heliox tool jotform -- form questions <formID>       # GET /form/{id}/questions
heliox tool jotform -- form submissions <formID> [--limit --offset --filter --orderby]
heliox tool jotform -- submission list [--limit --offset --filter --orderby]   # GET /user/submissions
heliox tool jotform -- submission get <submissionID>       # GET /submission/{id}
heliox tool jotform -- submission create <formID> --field <qid>=<value> [...]  # POST /form/{id}/submissions
heliox tool jotform -- submission edit <submissionID> --field <qid>=<value> [...] # POST /submission/{id}
heliox tool jotform -- submission delete <submissionID>    # DELETE /submission/{id}
heliox tool jotform -- report list [--form <formID>]       # GET /user/reports | /form/{id}/reports
heliox tool jotform -- folder list                         # GET /user/folders
```

**Auth header:** every request sets `APIKEY: $JOTFORM_API_KEY` and
`Accept: application/json`. Query params (`offset`/`limit`/`filter`/`orderBy`) map
to flags. `submission create/edit` translate repeated `--field qid=value` (and
`qid:subfield=value` for composite fields like name/address) into Jotform's
`submission[qid][subfield]` form-encoding.

**JSON output shape:** stdout is Jotform's native envelope verbatim
(`{"responseCode":200,"message":"success","content":{...}}`) — the AI reads
`content`. Non-2xx (or `responseCode != 200`) becomes a typed `apiError` → exit 1,
rendered on stderr as `--json` `{"error":{...}}` or plain text. A read-only-key
`401` (`"You're not authorized to use ..."`) is surfaced **verbatim** so the AI
can tell the user to mint a Full Access key — no silent downgrade (repo Hard Rule).

**TDD (L1):** `*_test.go` per resource builds an `httptest.Server` fake and
asserts request path, `APIKEY` header presence/value, query/form encoding, and
both text + `--json` error rendering. Never hit the live API from a unit test.

---

## 4. Helio provider bundle (`integrations/providers/jotform/provider.yaml`)

**Hidden-first**, `presentation.visible: false`. Jotform is the **first real live
user of the `api_key` auth type** — integration-service's `manual_api_token`
verifier + `auth.type: api_key` path exists on main today but is exercised only by
the synthetic `acme` fixture (Figma, its former sole user, retired to the Figma
MCP). **No capability growth is required**: the declarative verifier already sets
the raw token as the bundle-declared header
(`req.Header.Set(definition.APIKey.Header, token)` in
`service/manual_token_verifier.go`) and GETs the declared identity endpoint — an
exact fit for `APIKEY` + `GET /user`.

```yaml
schema: helio.provider/v1
key: jotform
go_name: Jotform

presentation:
  name: Jotform
  description_key: jotform
  consent_domain: jotform.com
  visible: false            # flip only after L1–L5 pass + anycli pin ships + icon + locales
  # order: <pick unoccupied at flip time>

auth:
  type: api_key
  owner: individual
  api_key:
    header: APIKEY          # Jotform's custom header; verifier sends the raw key (no Bearer)
    setup_url: https://www.jotform.com/myaccount/api

identity:
  source: userinfo
  url: https://api.jotform.com/user
  stable_key: /content/username        # immutable account handle; verified GET /user body
  label_candidates: [/content/name, /content/email, /content/username]

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
    access_token: token.access_token   # user's key stored via POST /connections/credentials → Vault
    account_key: connection.account_key

tool:
  name: jotform
  kind: api-key            # wire-compat ToolKind value; clients route the key-entry drawer
```

**Why no `auth.credential_input` block:** for `auth.type: api_key`, credential_input
is optional (`validate.go` requires it only for `type: credentials`); absent = the
client's implicit single-token entry field. Jotform is one opaque key, so the
implicit schema is correct — the mongodb bundle only needed an explicit field
because a DSN is a differently-labeled secret under `type: credentials`.

**Connect-time verification:** `POST /connections/credentials` runs the
declarative verifier before any Vault write — `GET https://api.jotform.com/user`
with `APIKEY: <pasted key>`. Success extracts `account_key = content.username` and
a human label from `content.name`/`content.email`. A bad or read-only-but-still-
valid key that returns 200 connects; an outright invalid key `401`s and connect
fails fast with the provider status. No client secret, no `required_config_fields`
(the `manual_api_token` branch in `validateConfigContract` forbids them).

**UI icon (Helio side, manual):** `ui/helio-app/src/integrations/icons/jotform.svg`
+ register in `providerIcons.ts`. Never generated.

**i18n:** `tools.desc.jotform` (+ any label keys) across all locales before flip.

---

## 5. AI-facing docs

Add a provider sub-doc under `agents/plugins/heliox/skills/tool/` (`jotform.md` or
a section), then bump the plugin version (`bump-version.sh`) and publish
(`publish-to-marketplace.sh`) at batch-end. Must teach:

- Connect once (`heliox tool jotform auth` → paste key), then read/write.
- The **qid recipe**: `form list` → `form questions <id>` (grab qids) →
  `form submissions <id>` / `submission create <id> --field <qid>=…`.
- **Full Access vs Read Only**: writes (`submission create/edit/delete`) need a
  Full Access key; a read-only key returns Jotform's `401 not authorized` verbatim.
- Pagination via `--limit`/`--offset`/`--filter`/`--orderby`.
- Note: submissions created via API do **not** fire the form's email
  notifications/integrations (Jotform behavior, set expectations).

---

## 6. Open questions / known limitations

1. **Regional bases (EU / HIPAA).** The verifier `identity.url` and the anycli
   service base are pinned to US `api.jotform.com`. An EU/HIPAA-residency account's
   key `401`s there, so v1 **scopes to US accounts**. Options for a follow-up: a
   `--region eu|hipaa` service flag + `JOTFORM_API_BASE` override for the data
   plane, but the bundle verifier is a single fixed URL — cross-region verify would
   need either a second bundle or a small verifier capability. Documented as a
   limitation at flip; not blocking Wave 1.
2. **Read-only keys connect successfully.** Verification only proves the key
   authenticates (`GET /user` is a read), so a read-only key passes connect and
   only fails later at first write with Jotform's 401. This is acceptable
   (matches the "surface the provider error verbatim" rule); the AI doc sets
   expectations up front.

---

## 7. Test plan — five layers (skill `references/integration-testing.md`)

| Layer | What runs | External creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` in anycli: httptest fakes assert path, `APIKEY` header, query/form encoding, text + `--json` error envelope, exit codes. | **No** |
| **L2** real-API harness | `make build-harness`; `ANYCLI_CRED_ACCESS_TOKEN=<real key> anycli jotform -- user` and `-- form list`, `-- form questions <id>`, a read submission, and (with a Full Access key) a `submission create` against a throwaway form on `api.jotform.com`. Proves field name / header / request shape against the live API. | **Yes** (real Jotform key from the account pool) |
| **L3** generation + suites | From `go-services/integration-service`: `go run ./cmd/provider-gen` then `--check`; both repos' unit suites (`cd helio-cli && go build ./... && go test ./cmd/heliox/cmds/tool/`). On-branch, point `helio-cli/go.mod` at the anycli branch via local `replace` (uncommitted). Bundle stays hidden, so `provider-gen --check` is expected red in CI until batch-end regen — do not commit local regens. | **No** |
| **L4** singleton + seed | Start singleton (`env: dev`), `POST /internal/test-only/connections/seed` with a **real** Jotform key as `access_token` (api_key is a seedable user-token provider; **seed `access_token` only** — no refresh/expiry, the key is long-lived), using a real seeded org/assistant identity. Then `heliox tool jotform -- user` / `-- form list` resolves through the token gateway and hits the live API. | **Yes** (real key to seed) |
| **L5** connect path (pre-flip, once) | **api_key key-entry path** (agent-drivable per master plan §2): open the connect link → paste the key through the real connect UI → stored via `POST /connections/credentials`, verified against `GET /user` → connection shows connected/configured in `GET /connections` → one **unseeded** live `heliox tool jotform -- user` succeeds. Human fallback only on UI breakage. | **Yes** (real key + connect UI) |

**Layers needing externally supplied credentials:** L2, L4, L5 (all need one real
Jotform API key from the test-account pool; a Full Access key to also exercise the
write verbs). L1 and L3 need none.

**Definition of done:** all five layers green → docs published + icon registered →
flip `presentation.visible: true` + regenerate as the single go-live change.
