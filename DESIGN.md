# Tool design — Later (row 208)

Scratch design file for branch `tool/later` (both repos). The batch lead strips
this at batch end. English only.

Catalog row (master plan §4): `#208 | Later | anycli id later | provider key
later | oauth_review | wave 3 | Social & Media`.

---

## 0. Headline: the catalog row is wrong, and Later is an at-risk swap candidate

Per the task's independent-judgment mandate ("if the official docs contradict the
catalog's auth lane or the audit verdict, follow the official docs and record the
divergence"), I verified Later against its official documentation before writing
any code. Three findings overturn the catalog row:

1. **There is no `oauth_review` authorization-code OAuth flow for Later.** The
   catalog lists Later as `oauth_review` (a multi-tenant authorization-code app
   gated behind a review/partner program). Later exposes no such flow. It never
   appeared in the 2026-07-21 OAuth audit table (`oauth-audit.md` covered the 250
   pre-audit `api_key` tools; Later was seeded as `oauth_review` and never
   verified). Verification shows the classification has no basis: Later's only
   public API authenticates via **OAuth 2.0 client-credentials (two-legged,
   machine-to-machine)** — a `clientId`/`clientSecret` secret pair POSTed for a
   short-lived JWT, with no user login, consent screen, or redirect. Under the
   audit's own rubric ("a tool moves to an OAuth lane only when the provider
   offers a multi-tenant authorization-code OAuth flow"), this is the **`api_key`
   lane** (a customer-pasted secret pair), not `oauth_review`.

2. **The product the catalog implies has no API at all.** "Later / Social &
   Media", sitting beside Buffer, Hootsuite, Sprout Social, and Typefully,
   implies the Later social-scheduling product (post scheduling, content
   calendar, publishing). That product has **no public developer API, no
   developer partner program, and no third-party OAuth** (Later's own help/
   compliance material and multiple 2026 developer surveys confirm it; Later uses
   OAuth only internally to connect a user's own social accounts to Later's app).
   An AI teammate cannot schedule or publish through Later programmatically.

3. **The one API Later does publish is a different product, read-only, and not
   self-serve.** The only public API is the **Later Influence Reporting API**
   (`reporting.api.later.com`, formerly Mavrck at `api.mavrck.co`) — an
   influencer-marketing **analytics/reporting** surface. It is read-only
   (campaign performance, instances), server-side only ("not available from
   browser applications"), and **credentials are issued only on request through a
   Later account team** (enterprise/partner gate — no self-serve portal). The v1
   Mavrck API is deprecating within ~6 months of the docs' publication.

**Verdict.** Later as specified in the catalog (`oauth_review` social scheduling)
is **not integrable** — the surface does not exist. The only integrable
interpretation is the Later Influence Reporting API as an **`api_key`
(client-credentials) read-only analytics tool**, and even that is blocked by
account procurement (no self-serve credentials; requires a paid Later Influence
enterprise account). This is the exact profile the master plan already re-laned
to the **3-hold** holdback batch:

- **API feasibility** (§6 risk "API access regressions"): thin/closed/mismatched
  surface — same class as Otter.ai ("no general public developer API"), Loom, and
  Framer.
- **Account procurement** (§6 risk "Test-account cost and ToS"): no self-serve or
  free tier; credentials gated behind an account rep — same class as ZoomInfo,
  Sprout Social, Ahrefs, and Moz.

**Recommendation to the batch lead / master plan:**

1. **Re-lane `oauth_review` → `api_key`** in the catalog (client-credentials
   secret pair; no authorize redirect exists). Record in the §6 amendment log.
2. **Move Later Wave 3 → 3-hold** and run it through the 3-hold **pre-verify
   gate**: (a) confirm a Later Influence Reporting API test account can be
   procured, and (b) confirm the reporting surface is worth shipping given the
   scheduling product an AI teammate would actually want is absent.
3. **If pre-verify fails** (no test account, or reporting-only deemed
   low-value), **swap Later out** via the catalog-amendment mechanism (§6 risk
   #2 / OQ5), holding the 298 total and continuous numbering. A same-category
   self-serve swap with a real API would be a Social & Media scheduler that
   actually publishes an API (the audit already lists several such providers).

The rest of this document specifies the **integrable interpretation** (the
Reporting API tool under `api_key`) concretely, so that if pre-verify clears the
account-procurement gate the build is ready to execute with zero further design
work. Everything below is conditioned on that gate clearing.

---

## 1. API surface wrapped — and why

An AI teammate's realistic job-to-be-done with "Later" is **pull social/creator
campaign performance into a report** (the scheduling job is impossible — no API).
So the tool wraps the **Later Influence Reporting API v2** and nothing else:

| Verb (AI intent) | Method + path | Notes |
|---|---|---|
| authenticate (internal) | `POST /oauth/token` | JSON `{clientId, clientSecret}` → `{"jwt": "<JWT>"}`; short-lived, `exp` embedded; re-auth on 401 |
| `instances` | `GET /v2/instances` | list the reporting instances (accounts/workspaces) the credential can see; needed to obtain `instanceIds` |
| `campaigns` | `GET /v2/campaigns/performance` | campaign performance; query params `metrics` (e.g. `engagements`, `impressions`), `startDate`, `endDate`, `instanceIds`, `limit`, `sortProperty`, `sortDirection` |

Base host: `https://reporting.api.later.com`. All data calls are `GET` (read-only);
`/oauth/token` is the only `POST` and is internal to auth, never an AI verb. The
v2 host is authoritative; v1 (`api.mavrck.co`) is explicitly deprecating and is
out of scope. `/api-reference` may expose additional read endpoints; stage-1
research before the dev branch pins the exact set — the two above are the
confirmed spine and are sufficient for the analytics job-to-be-done.

Why not more: there are no write/mutation or scheduling endpoints to wrap; adding
speculative surface violates the "subtract before adding" rule.

## 2. anycli definition

**Tool form (stage-1 rubric): `service` type.** No official Later CLI exists; the
`cli`-type gate fails immediately. Implement `service` against the HTTP API in
`internal/tools/later/` (Go package `later` — id has no dashes, so the package
name equals the id). Matches 21 of 23 shipped definitions.

**Axes (naming registry §3):**

| Axis | Value |
|---|---|
| ① CLI command word | `later` (flat; not a family/group product) |
| ② anycli tool id | `later` |
| ③ provider catalog key | `later` |

Identity holds across all three (`later` == `later` == `later`), so **no
`toolToProvider` divergence entry** and no `resolver.go` change — verified against
`helio-cli/internal/toolcred/resolver.go` (`toolToProvider` currently holds only
the Microsoft and Google divergences; identity keys are absent by design).

**`definitions/tools/later.json`** — the credential the service consumes is the
**derived JWT** (the token gateway performs the client-credentials exchange
service-side; see §4), injected as a bearer env var. This mirrors the `x`
definition shape (env-injected token), not a raw-secret injection:

```json
{
  "name": "later",
  "type": "service",
  "description": "Later Influence Reporting API (read-only analytics)",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "LATER_JWT"}
      }
    ]
  }
}
```

**Subcommands / verbs** (built-ins under `internal/tools/later/`, cobra-style,
`--json`-first, non-interactive per anycli AGENTS.md):

- `later instances` — `GET /v2/instances`; flags: none (optionally `--limit`).
- `later campaigns --metrics=engagements,impressions --start=YYYY-MM-DD --end=YYYY-MM-DD [--instance-ids=…] [--limit=N] [--sort=…] [--sort-dir=asc|desc]`
  — `GET /v2/campaigns/performance`.

**JSON output shape** — provider-neutral, agent-tuned (003 §3 conventions):
each verb prints a top-level object `{"instances": [...]}` or
`{"campaigns": [...]}` with flattened metric keys; errors go to stderr with a
non-zero exit; a 401 surfaces as an `auth`-classified error (the resolver treats
that as a credential/mint failure, see `run.go`). The service exposes an
`auth_error.go` mapping (401/403 → typed auth error) exactly like `internal/tools/x/auth_error.go`.

**TDD (anycli AGENTS.md, tests first):** `internal/tools/later/*_test.go` with
`httptest` fakes for `/oauth/token`, `/v2/instances`, `/v2/campaigns/performance`
— covering the JWT-exchange-then-call path, 401 re-auth classification,
pagination/`limit`, and metric/date query-param encoding. `register.go` gains one
`RegisterService("later", …)` line (the only shared-registry edit; batch-end
merge per §2).

## 3. Credential fields & exact auth flow

Later's `/oauth/token` is **OAuth 2.0 client-credentials** — the provider issues a
`clientId` + `clientSecret` pair; the app exchanges them for a short-lived JWT.
There is no user, no redirect, no refresh token (re-exchange the secret pair on
expiry / 401). Credential shape is therefore a **two-field customer-pasted secret
pair**, i.e. the `api_key` lane's manual-credentials model — the same multi-field
`manual_credentials` shape already used for ZoomInfo (and the NetSuite/four-cred
precedent flagged in §6).

Flow, end to end:

1. Connect UI: the user pastes `clientId` and `clientSecret` (obtained from their
   Later account team) into the credential drawer → stored via the write-only
   `POST /connections/credentials` API (never touches the bundle).
2. Token gateway (service-side): on a `GET /connections/token`, exchange the
   stored pair at `POST https://reporting.api.later.com/oauth/token` with
   `Content-Type: application/json` body `{"clientId": …, "clientSecret": …}`,
   read `{"jwt": …}`, cache it against the JWT's `exp`, and project it as the
   provider-neutral `access_token` the anycli definition injects as `LATER_JWT`.
   On 401 from a data call, re-exchange.
3. Identity/verification: exchange-then-`GET /v2/instances` at connect time both
   **verifies** the secret pair (bad creds → non-200 from `/oauth/token`) and
   yields the account label (an instance name/id). This is a small
   integration-service capability (a "Later client-credentials verifier + JWT
   exchange" — see §5), the one piece of non-standard behavior that a narrow
   adapter would own.

This is a genuine capability need, flagged at stage 1 (not mid-wave), per §6's
instruction to flag non-standard auth shapes early. It is materially lighter than
Bill.com/NetSuite: a single JSON secret-pair exchange returning a bearer JWT.

## 4. Helio provider bundle plan

`integrations/providers/later/provider.yaml`, **hidden-first**
(`presentation.visible: false`), modeled on the `mongodb` manual-credentials
precedent but with two secret fields and a service-side exchange verifier:

- **`schema: helio.provider/v1`**, `key: later`, `go_name: Later`.
- **`presentation`**: `name: Later`, `description_key: later`,
  `consent_domain: later.com`, `visible: false`. `order` chosen from an unoccupied
  slot only at the visible flip.
- **`auth`**: `type: credentials`, `owner: individual`,
  `credential_input.fields`:
  - `client_id` (`label_key: later_client_id`, `secret: false`, `required: true`)
  - `client_secret` (`label_key: later_client_secret`, `secret: true`,
    `required: true`), `setup_url` → the Later Influence Reporting API
    getting-started page (where an account requests credentials).
- **`identity`**: `source: strategy` (no user-facing `userinfo`; the account
  label is derived from `/v2/instances` after the client-credentials exchange).
- **`connection`**: `mode: isolated`, `disconnect_mode: local_only`,
  `runtime_strategy: manual_credentials`.
- **`resources`**: `selection/discovery/enforcement: none`.
- **`credential.fields`**: `access_token: token.access_token` (the derived JWT),
  `account_key: connection.account_key`. The stored secret pair rides the
  existing user-token write path; the gateway derives the JWT.
- **`tool`**: `name: later`, `kind: api-key` (wire-compat value; clients route
  the credential drawer by `auth_type`, matching mongodb).

Axis naming per master plan §3: ① `later`, ② `later`, ③ `later` — all identical,
no group, no divergence.

**Config (Config Sync hard rule):** unlike an `oauth_review` bundle, there are **no
Helio-owned `oauth.client_id`/`oauth.client_secret`** to register — the secret
pair is the *customer's*, entered at connect time. So integration-service needs no
per-provider client credentials in `config/` or `deploy/`; a provider with all
config fields absent renders `configured: false` only when it *declares* required
config, which this bundle does not. This removes Later from lane-1's config-append
surface entirely (a simplification the `oauth_review` misclassification hid).

**UI icon:** `ui/helio-app/src/integrations/icons/later.svg` + register in
`providerIcons.ts` (manual, batch-end). **AI-facing docs:** a `later` sub-doc
under `agents/plugins/heliox/skills/tool/`, making explicit that this is
read-only Later Influence reporting, not social scheduling — so an assistant never
attempts an impossible publish.

## 5. integration-service capability

`standard_oauth` needs zero service code, but this bundle is not standard OAuth. It
needs one narrow, testable capability (a `service/adapter_later.go`-style seam, or
a reusable "client-credentials JSON exchange" verifier if one already exists on
main by batch time):

- **`laterClientCredentialsVerifier` + JWT exchange**: given the stored
  `{client_id, client_secret}`, POST `/oauth/token`, on success read `jwt`, then
  `GET /v2/instances` for the identity label; project the JWT as
  `token.access_token`. On any non-200, return a typed "invalid credentials"
  connect error. Idempotent, unit-testable with `httptest`.

Check main first for an existing client-credentials exchange capability (several
prior tools — e.g. hotjar's client-credentials, sproutClientVerifier — established
this shape); reuse if the field mapping fits, grow minimally if not. This is the
single justified piece of non-standard service code.

## 6. Test plan — five layers

| Layer | For Later | Needs external creds? |
|---|---|---|
| **L1** anycli unit | `go test ./...` green: `internal/tools/later/*_test.go` with `httptest` fakes for `/oauth/token`, `/v2/instances`, `/v2/campaigns/performance`; asserts JWT-exchange path, 401 re-auth classification, query-param encoding, `--json` shape. Definition parse test. | No |
| **L2** real-API harness | `anycli later -- instances` and `… campaigns …` against the REAL `reporting.api.later.com`, creds from `ANYCLI_CRED_*`. **This is the gating layer** — it can only run with a real Later Influence account's `clientId`/`clientSecret`. | **Yes — blocked on account procurement (3-hold pre-verify)** |
| **L3** generation + suites | `provider-gen` + `provider-gen --check` locally on-branch (expected red in CI until batch-end merge, per §2); `helio-cli` + `integration-service` unit suites green, incl. the new verifier's tests. | No |
| **L4** singleton + seed | Singleton + `POST /internal/test-only/connections/seed` seeding the derived JWT (or the secret pair, exercising the exchange), then `heliox tool later -- instances` through the real token gateway. Requires a valid credential to seed, so it inherits L2's dependency. | **Yes — same account dependency** |
| **L5** connect flow | `api_key` L5 path (master plan §2): open the connect link → paste `clientId`/`clientSecret` in the real drawer → connection shows connected/configured (`GET /connections`) → one unseeded live `later instances` through the token gateway. Agent-drivable (no OAuth consent) with human fallback. | **Yes — same account dependency** |

**Credential-supply summary:** L1 and L3 are fully agent-runnable with no external
credentials. **L2, L4, and L5 all block on a single external dependency: a Later
Influence Reporting API account with issued `clientId`/`clientSecret`.** Because
those credentials are not self-serve (account-team gated), this dependency is the
crux of the 3-hold pre-verify gate. If procurement fails, L2 cannot pass, the tool
cannot reach definition-of-done, and Later is swapped per §0 recommendation 3.

## 7. Rollout

Hidden-first regardless (`visible: false`), like every provider. But the honest
path is: **do not start the dev branch until the 3-hold pre-verify clears the
account-procurement gate.** If it clears, the build is a small `api_key`
service-type tool (this document is execution-ready). If it does not, Later is a
catalog swap, not a build — and the cheapest correct outcome is to recognize that
now rather than ship a hidden tool no assistant can connect.
