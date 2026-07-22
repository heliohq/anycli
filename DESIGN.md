# Plaid — per-tool design

Scratch design for the `plaid` tool provider (catalog row 178). Committed on
branch `tool/plaid`; the batch lead strips it at batch-end.

Axes (master plan §3): ① CLI word `plaid` · ② anycli id `plaid` · ③ provider
key `plaid`. All three are identical, so **no `toolToProvider` divergence
entry** is added. Auth lane per catalog + OAuth audit (row 180): **`api_key`**.
Wave: **3-hold** (final batch of Wave 3), re-laned there for API-feasibility
risk (production access approval + Link-only access-token issuance).

## 1. Divergence from the catalog / audit — none, but the api_key lane is load-bearing

The 2026-07-21 OAuth audit (row 180) keeps Plaid in `api_key` with the note
"no viable multi-tenant path," and that verdict holds against the official
docs. Plaid does **not** expose a customer-authorizes-your-app OAuth2
authorization-code flow of the kind the audit rubric requires. What Plaid
calls "OAuth" in its docs is the *bank*-side OAuth that happens **inside the
Link widget** between the end user and their financial institution — it is not
a way for a third-party app to obtain delegated access to another Plaid
customer's Plaid account. A Plaid integrator authenticates with its own
`client_id` + `secret` (issued per-environment from the Plaid Dashboard). So
the credential Helio stores is a self-serve key pair the user pastes — exactly
the `api_key` manual-credential shape. **No divergence to record in
DESIGN/audit.**

The real risk the 3-hold lane captures is **API feasibility, not auth-lane
mislabelling** — see §7.

## 2. Official API surface wrapped, and why

Verified against `https://plaid.com/docs/api/` (conventions),
`https://plaid.com/docs/sandbox/` (server-side token bypass), and the product
reference pages. Salient facts:

- **Two environments only**: `https://sandbox.plaid.com` and
  `https://production.plaid.com`. The legacy `development` host is retired.
  Items cannot move between environments; sandbox keys ≠ production keys.
- **Every request carries `client_id` + `secret`**, sendable either in the
  JSON body or as the `PLAID-CLIENT-ID` / `PLAID-SECRET` headers. All calls are
  HTTPS `POST`.
- **Item-scoped data requires an `access_token`** — one per linked bank Item.
  In production an `access_token` is minted only by exchanging a `public_token`
  that the **Link** client-side widget produces (`/item/public_token/exchange`).
  In **sandbox**, `/sandbox/public_token/create` mints a `public_token` for an
  arbitrary test institution *fully server-side* with just `client_id` +
  `secret`, so the whole loop is scriptable without any browser.

**What an AI teammate actually does with Plaid** drives the endpoint set. Three
real jobs, in descending robustness:

1. **Institution reference lookups** — `/institutions/get`,
   `/institutions/get_by_id`. These need **no `access_token`**, work in
   production the moment the app is approved, and are the safest always-on
   surface (resolve a routing/institution id, list supported banks, check
   product support per institution).
2. **Read financial data for an Item the user already onboarded** —
   `/transactions/sync` (preferred), `/transactions/get`, `/accounts/get`,
   `/accounts/balance/get`, `/auth/get`, `/identity/get`, `/item/get`,
   `/item/remove`. Each takes an `access_token` the AI supplies **per call**
   (see §3 — the access_token is a runtime argument, not a stored credential),
   because the token is per-bank-Item and is produced by the user's own Link
   integration, not by Helio's connect flow.
3. **End-to-end sandbox development** — `/sandbox/public_token/create` +
   `/item/public_token/exchange`. This lets the AI stand up a complete test
   Item and read from it without any frontend, which is how a developer
   building/​debugging a Plaid integration uses it day to day and is the
   primary L2/L4/L5 exercise path.

Endpoints intentionally **out of scope for v1**: asset reports
(`/asset_report/*`, binary PDF + async polling), payments/transfer
(`/transfer/*`, `/payment_initiation/*` — heavily gated, EU/UK, real money),
investments/liabilities (`/investments/*`, `/liabilities/*` — additive later),
and `/link/token/create` (only useful when a real Link frontend consumes it,
which Helio does not host). Starting narrow keeps the tool honest about what an
AI can drive without a browser.

## 3. anycli definition (stage-1 rubric)

**Type: `service`.** The `cli`-type rubric fails on the first clause: there is
no official, agent-friendly Plaid binary. (`plaid-cli` on GitHub is a
third-party project, not official, and is oriented at interactive Link.) So we
implement `service` type in `internal/tools/plaid/` against the HTTPS API —
consistent with 21 of 23 shipped tools.

**Credential bindings (3, all `env`):**

| source field  | env var           | role                                  |
|---------------|-------------------|---------------------------------------|
| `client_id`   | `PLAID_CLIENT_ID` | sent as `PLAID-CLIENT-ID` header      |
| `secret`      | `PLAID_SECRET`    | sent as `PLAID-SECRET` header         |
| `environment` | `PLAID_ENV`       | selects base URL (`sandbox`\|`production`) |

The service sends the id/secret as headers (cleaner than body-merging into
every request), and picks the base host from `PLAID_ENV` — defaulting to
`sandbox` if unset, and rejecting any value other than `sandbox`/`production`
with a usage (exit 2) error rather than silently guessing (Architecture hard
rule: fail fast, no silent fallback).

**The access_token is a per-invocation flag (`--access-token`), NOT a stored
credential.** This is the load-bearing design decision. Plaid's model cleanly
separates *app* credentials (`client_id`/`secret`, stable, one pair per
environment) from *Item* tokens (`access_token`, one per linked bank,
issued by Link). Helio's connection stores the former; the latter is data the
AI passes at call time (obtained from the user's Link integration, or minted by
`item exchange-public-token` in sandbox). Modelling the access_token as a
connection credential would be a category error — a connection is one Plaid
app, not one bank account, and a user has many Items under one app.

**Subcommand tree (cobra, grouped by resource; every leaf `--json`-capable,
non-interactive):**

```
plaid institutions get            --count --offset --country-codes
plaid institutions get-by-id      --institution-id --country-codes
plaid accounts get                --access-token
plaid accounts balance            --access-token
plaid auth get                    --access-token
plaid transactions sync           --access-token [--cursor] [--count]
plaid transactions get            --access-token --start-date --end-date
plaid identity get                --access-token
plaid item get                    --access-token
plaid item remove                 --access-token
plaid item exchange-public-token  --public-token          # → access_token
plaid sandbox public-token-create --institution-id --products   # sandbox only
```

`sandbox public-token-create` refuses (exit 2) when `PLAID_ENV=production` —
the endpoint does not exist there, and an explicit refusal beats a confusing
404 passthrough.

**JSON output shape** follows the `notion` reference: a `BaseURL`/`HC`/`Out`/
`Err` struct so tests point `HC` at an `httptest.Server`; `--json` emits a
structured envelope, and errors render as a typed `apiError` envelope carrying
Plaid's own `error_type` / `error_code` / `error_message` / `request_id` (Plaid
returns those on every 4xx, and surfacing `error_code` verbatim — e.g.
`INVALID_ACCESS_TOKEN`, `ITEM_LOGIN_REQUIRED`, `PRODUCT_NOT_READY` — is what
lets the AI self-correct). Exit codes: 0 success, 1 runtime/API failure, 2
usage/parse. Success prints a compact provider-neutral summary in plain mode.

## 4. Auth flow verification (api_key lane)

- **Registration model**: self-serve. Anyone signs up at the Plaid Dashboard
  and immediately gets **sandbox** `client_id` + `secret` (free, no review).
  **Production** keys require Plaid to approve a production-access request
  (business/compliance review) — this is the gate the 3-hold lane exists for
  (§7). Development keys no longer exist.
- **Token semantics**: `client_id`/`secret` are long-lived, non-expiring, and
  environment-scoped. There is **no refresh cycle** — so seeding (L4) seeds the
  static pair only, never a `refresh_token`/`expires_at` (integration-testing
  reference, "non-expiring" class). `access_token`s are also long-lived but are
  Item-scoped runtime data, not part of the stored connection.
- **Scopes**: none in the OAuth sense. Product access is gated per-Item at
  Link time and per-app by Plaid's product enablement, not by a scope string
  Helio manages.

This is a clean fit for the manual-credential `api_key` shape; no OAuth
exchanger, PKCE, or token gateway refresh path is involved.

## 5. Helio provider bundle plan (hidden-first)

Directory `integrations/providers/plaid/provider.yaml`, `key: plaid`,
`presentation.visible: false`. Shape modelled on the shipped `mongodb`
manual-credential bundle, extended to **multiple** credential-input fields.

- `auth.type: credentials`, `owner: individual`.
- `credential_input.fields`:
  - `client_id` — `secret: false`, required, placeholder from a Plaid app id.
  - `secret` — `secret: true`, required.
  - `environment` — `secret: false`, required, values `sandbox`|`production`
    (default `sandbox`); a constrained/enum field if the connect form supports
    one, otherwise a validated text field.
  - `setup_url`: `https://dashboard.plaid.com/developers/keys`.
- `connection.mode: isolated`, `disconnect_mode: local_only`,
  `runtime_strategy: manual_credentials`.
- `identity.source: strategy` — **no-verify by default** (mongodb precedent):
  Plaid has no zero-arg identity endpoint, and the honest account label is
  environment-scoped and human-readable, e.g. `sandbox · client 6a1b…` (env +
  a short non-secret fingerprint of `client_id`), **never a hash of the
  secret** and never the secret itself (OQ2 human-readable constraint; secret
  never enters connection metadata).
  - *Optional verifier growth (recommended, not required for hidden):* a
    `plaidCredentialVerifier` capability that does one `POST /institutions/get`
    (`count:1, offset:0, country_codes:["US"]`) at connect time to validate the
    id/secret pair without any Item. Worth adding for connect UX, but the
    no-verify path is sufficient to ship hidden and matches mongodb.
- `credential.fields`: multi-field projection mapping `client_id` / `secret` /
  `environment` to the stored token payload (paypal/zoominfo multi-field
  precedent), plus `account_key: connection.account_key`.
- **Config: none.** Unlike oauth bundles, `api_key` needs **zero**
  `required_config_fields` — the user brings their own Plaid keys — so there are
  **no `config/` or `deploy/` appends** and no Config-Sync surface for this
  tool. (This also removes Plaid from lane-1's config-landing critical path.)

### Integration-service capability dependency (flag at stage 1)

The worktree base's manual-credentials identity path hard-requires exactly one
input field (`manual_credentials_identity.go` / `manual_credential.go`:
`len(definition.CredentialInput.Fields) != 1`). Plaid's **3-field** credential
therefore depends on the **multi-field `manual_credentials` capability**
(client_id + secret + environment), the same growth paypal / zoominfo / mixpanel
/ braze introduced on their branches. Because Plaid runs in the **final** batch
of Wave 3, that capability will almost certainly already be on `main`; the
Plaid bundle should **reuse** it if present and only **grow** it (multi-field
policy + an environment-aware fingerprint identity deriver) if it has not
landed. Do not fork a parallel path — extend the reviewed one.

## 6. UI + docs (batch-end merge)

- Icon: `ui/helio-app/src/integrations/icons/plaid.svg` + hand-registered in
  `providerIcons.ts` (never generated).
- i18n: `tools.desc.plaid` / credential-field labels across all locales.
- AI-facing sub-doc under `agents/plugins/heliox/skills/tool/` — must state
  plainly: connection stores app keys; `--access-token` is per-Item and comes
  from the user's Link integration (or `sandbox public-token-create` in
  sandbox); institution lookups need no access_token.

## 7. 3-hold pre-verify verdict (API feasibility)

Feasibility **passes with a documented production caveat** — no swap. Findings:

- **Sandbox is fully server-side integrable** with self-serve free keys: the
  `/sandbox/public_token/create` → `/item/public_token/exchange` → read loop
  needs no browser, so L1/L2/L4/L5 are all exercisable end-to-end.
- **Production has two real limits, stated honestly (no silent fallback):**
  (a) production `client_id`/`secret` require Plaid's production-access approval
  — a human/business gate, so the **visible flip** for production usage waits on
  that clearance (analogous to how oauth_review's flip waits on review, though
  here it is an account-provisioning gate, not an app-review gate); (b) minting
  a production `access_token` requires the **Link** widget, which Helio's
  connect flow does not host — so production Item reads only work when the user
  supplies an `access_token` from their own Link integration. The
  always-available production surface with no access_token is the **institution
  reference** endpoints.
- **Recommendation:** ship the tool for its genuinely usable surfaces
  (institution lookups anywhere; the full loop in sandbox; Item reads with a
  user-supplied access_token) and document the production access-token
  constraint in the AI sub-doc. This is more valuable than a swap and does not
  overstate capability.

## 8. Test plan — five layers

| Layer | What it proves for Plaid | External creds? |
|---|---|---|
| **L1** | anycli unit tests: `httptest` fake asserts `PLAID-CLIENT-ID`/`PLAID-SECRET` headers, base-URL selection by `PLAID_ENV`, `access_token` in body for Item calls, `--json` + `apiError` envelope (incl. `error_code` passthrough), exit-code contract, and `sandbox`/`production` env validation. | No |
| **L2** | dev harness against the **real Plaid sandbox** API: `ANYCLI_CRED_CLIENT_ID` / `ANYCLI_CRED_SECRET` / `ANYCLI_CRED_ENVIRONMENT=sandbox`, run the real loop — `sandbox public-token-create` (ins_109508, products `transactions`) → `item exchange-public-token` → `transactions sync` / `institutions get`. | **Yes** — free self-serve Plaid **sandbox** keys |
| **L3** | `provider-gen --check` (five projections) + both repos' unit suites; multi-field manual_credentials capability present/grown. | No |
| **L4** | singleton + `POST /internal/test-only/connections/seed` seeding the **static** `client_id`/`secret`/`environment=sandbox` (no refresh_token), then `heliox tool plaid -- institutions get` and `-- transactions sync --access-token <sandbox-token>`. Note: seed must accept the **multi-field** credential (paypal precedent); if the seed path is single-field on `main` at run time, that is the same multi-field growth as §5. | **Yes** — same sandbox keys |
| **L5** | api_key key-entry path (agent-drivable, master plan §2): open connect link → paste `client_id`/`secret`/`environment=sandbox` → connection shows connected/configured (`GET /connections`) → one **unseeded** live sandbox command (`institutions get`) through the real token gateway. Production connect is deferred to production-access clearance (§7). | **Yes** — same sandbox keys |

Layers needing externally-supplied credentials: **L2, L4, L5**, all satisfiable
with **one free, self-serve Plaid sandbox key pair** — no paid tier, no review,
no production approval. L1 and L3 need none. This keeps the whole dev→L5 loop
agent-runnable inside the 3-hold batch; only the *production* visible flip is
gated on Plaid's account-side production-access approval.
