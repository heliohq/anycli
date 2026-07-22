# Wise — per-tool design (`tool/wise`)

Scratch design for the `wise` external tool provider behind `heliox tool`.
Batch-lead strips this file at batch end. English only.

- **anycli id (axis ②):** `wise`
- **provider catalog key (axis ③):** `wise`
- **CLI command word (axis ①):** `wise` (flat, no group)
- **Auth lane:** `api_key` (Wave 3, Payments & Commerce, master-plan row 177)
- **Tool type:** `service` (no official Wise CLI exists)

## 1. Auth-lane verification against official docs (divergence check)

The catalog lanes Wise as `api_key`; the OAuth audit verdict for Wise (row 179)
is *"no viable multi-tenant path — stays api_key per rubric."* I verified this
against the official docs rather than inheriting it.

Wise **does** publish an OAuth 2.0 flow — but the official Getting-Started /
authentication docs draw a hard line: *"use a personal API token if you're a
small business user automating your own Wise account, or use OAuth 2.0 if you're
a partner building for end customers or a large enterprise."* Partner OAuth
client credentials are **not self-serve** — they are issued only through a Wise
delivery team / `api@wise.com` after a commercial partner relationship. That is
exactly the audit rubric's "OAuth is partner-only with no practical path for a
shared client" case, so the `api_key` lane is **confirmed correct**. No DESIGN
divergence to record for the lane itself.

The self-serve credential is a **personal API token**, minted at
`wise.com → Your Account → Integrations and Tools → API tokens → Add new Token`
(2-step login required), and passed as `Authorization: Bearer <token>`. This is
what the account pool supplies for L2 and what the user pastes at L5.

**One load-bearing constraint that DOES belong in this design** (verified in the
official personal-tokens docs): under PSD2, **EU/UK personal tokens cannot fund
transfers or read balance *statements* via the API**, and several money-movement
and statement endpoints additionally require a Strong Customer Authentication
one-time-token (`x-2fa-approval` challenge/signature flow). A passthrough
`api_key` tool cannot carry the SCA signing dance, and funding is out of an AI
teammate's remit anyway. **This tool is therefore scoped read / monitoring
only** — see §2. Recording this here because it is a real capability boundary,
not an omission.

Sources checked:
- Personal tokens & auth: https://docs.wise.com/api-docs/features/authentication-access/personal-tokens
- Environments (base URLs): https://docs.wise.com/guides/developer/environments
- Activities endpoint (declares `PersonalToken` security): https://docs.wise.com/api-reference/activity/activitylist

## 2. API surface this tool wraps, and why

**What an AI teammate actually does with Wise:** it is a treasury/ops copilot —
"what's our EUR balance?", "did the payout to vendor X land?", "show this week's
account activity", "what would a 5000 USD→EUR transfer cost right now?". That is
**read-and-monitor**, not initiate-and-fund (which is PSD2/SCA-gated and a human
approval action regardless). The verb set below reflects that.

- **Production base URL (TLS-only):** `https://api.wise.com`
- **Sandbox (L2/L5 testing):** V1 `https://api.sandbox.transferwise.tech` is
  deprecating 2026-06-30; new integrations target the **V2 sandbox**
  `https://api.sandbox.wise.tech` (aka `api.wise-sandbox.com`). The service reads
  its base URL from a `--base-url` flag (default production) so the account pool
  can point L2 at whichever sandbox host its test token was minted in — V1 and V2
  tokens are not interchangeable.
- **Auth header:** `Authorization: Bearer <personal_token>`.

| Verb (proposed) | Endpoint | Why |
|---|---|---|
| `profile list` | `GET /v1/profiles` | The token's personal + business profiles; also the **identity/verification** call (§5). Every other call needs a `profileId`. |
| `balance list` | `GET /v4/profiles/{profileId}/balances?types=STANDARD` | Multi-currency balances — the top treasury question. (Balance *amounts* are readable; balance *statements* are PSD2-gated and intentionally omitted.) |
| `balance get` | `GET /v4/profiles/{profileId}/balances/{balanceId}` | Drill into one currency balance. |
| `transfer list` | `GET /v1/transfers?profile={id}&status={status}&offset=&limit=&createdDateStart=&createdDateEnd=` | Monitor outgoing payouts by status/date. |
| `transfer get` | `GET /v1/transfers/{transferId}` | Status of a specific payout. |
| `activity list` | `GET /v1/profiles/{profileId}/activities?status=&since=&until=&size=&nextCursor=` | Unified human-readable account activity feed (cursor-paginated, `size` 1–100). |
| `recipient list` | `GET /v1/accounts?profile={id}&currency={c}` | Look up saved recipient/beneficiary accounts. |
| `quote create` | `POST /v3/profiles/{profileId}/quotes` | Price a hypothetical transfer (rate + fee estimate). A quote is read-ish — it moves no money; it answers "what would this cost". |
| `rate list` | `GET /v1/rates?source={c}&target={c}` | Current/historical exchange rate. |
| `currency list` | `GET /v1/currencies` | Supported currencies / routes reference. |

**Deliberately excluded** (out of scope for an `api_key` passthrough teammate):
creating/funding transfers (`POST /v1/transfers`, `POST /v3/.../payments`),
batch groups, balance statements, and any endpoint requiring the SCA
`x-2fa-approval` one-time-token. If a future design wants money-movement it needs
partner OAuth + an SCA-signing adapter — a separate, non-`api_key` effort.

`profileId` is a required, per-invocation AI parameter (`--profile`) on every
profile-scoped verb, resolved by the AI from `profile list` — never baked into
the credential (a token may see several profiles).

## 3. anycli definition

- **Type decision (stage-1 rubric):** `service`. No official, non-interactive,
  `--json`-capable Wise binary exists to provision into the runtime image, so the
  `cli` branch (github→`gh`, lark→`lark-cli`) does not apply. Implement HTTP
  against the REST API — matching 21/23 shipped definitions.
- **Definition file:** `definitions/tools/wise.json`, `name: "wise"`,
  `type: "service"`, single credential binding:
  `source.field: access_token` → `inject: {type: env, env_var: WISE_API_TOKEN}`.
  (Same minimal single-secret shape as `bitly.json`; the service reads
  `WISE_API_TOKEN` and sets `Authorization: Bearer`.)
- **Go package:** `internal/tools/wise/` (id `wise` has no dash/leading digit, so
  the package name is the id verbatim), registered via
  `RegisterService("wise", &wise.Service{})` in `internal/tools/register.go`.
- **Shape:** copy `internal/tools/notion/` — a cobra tree grouped by resource
  (`profile`, `balance`, `transfer`, `activity`, `recipient`, `quote`, `rate`,
  `currency`), a `BaseURL`/`HC`/`Out`/`Err` struct so tests point at `httptest`,
  and the documented exit-code contract (0 ok / 1 API failure via typed
  `apiError` / 2 usage), with a `--json` structured error envelope.
- **Output shape:** provider-neutral JSON to stdout — pass Wise's JSON through
  for read verbs (it is already clean, agent-friendly JSON); for list verbs emit
  the array (or `{items, nextCursor}` for `activity`, preserving Wise's cursor so
  the AI can page). `--json` on errors yields the standard error envelope. Global
  `--base-url` flag (default `https://api.wise.com`) selects prod vs sandbox host.
- **Money/precision note:** amounts are decimals — render them as JSON numbers
  exactly as Wise returns them (do not reformat/round), so the AI reads the true
  value.

## 4. Credential fields & auth flow

- **Credential kind:** single secret — the personal API token. No client
  id/secret, no refresh token, no expiry (personal tokens are long-lived until
  revoked in the Wise UI).
- **Resolver field name:** `access_token` (the value flows through the existing
  `token.access_token` projection source — zero new `CredentialSource`, zero
  token-gateway change, exactly like the mongodb precedent).
- **L2 env:** `ANYCLI_CRED_ACCESS_TOKEN=<personal_token>` →
  `anycli wise -- profile list`.
- **Registration model:** self-serve, no app registration, no OAuth redirect —
  the account/token pool yields the key directly (this is why `api_key` L5 is
  agent-drivable per master-plan §2).

## 5. Helio provider bundle plan (`integrations/providers/wise/provider.yaml`)

Hidden-first (`presentation.visible: false`) until the anycli pin ships the
`wise` tool and L1–L5 pass.

- **Axes:** `key: wise`, `tool.name: wise`, `tool.command` unset (flat command).
  ②≡③ (`wise`==`wise`), mechanically identical → **no `toolToProvider` entry**
  and no resolver test change. (Consistent with master-plan §3: only the 24
  dashed ids get resolver entries; `wise` is not one.)
- **Auth block:** `auth.type: credentials`, `owner: individual`, one
  `credential_input.field` `api_token` (`secret: true`,
  `label_key: wise_api_token`,
  `setup_url: https://docs.wise.com/api-docs/features/authentication-access/personal-tokens`).
  No `required_config_fields` (no client id/secret) → the provider is never
  `partially configured`, so it renders `configured: true` and is safe to ship
  hidden with zero integration-service config appends (no `config/`+`deploy/`
  Secret work — unlike the oauth lanes).
- **Identity / account_key — verify + derive (differs from mongodb):** unlike a
  raw MongoDB DSN, Wise **has** an HTTPS identity endpoint, so we verify the
  pasted token and derive a human account label instead of storing it blind:
  - `connection.runtime_strategy: manual_credentials`.
  - Verifier: `GET https://api.wise.com/v1/profiles` with
    `Authorization: Bearer <token>` (200 ⇒ valid). This is the closest thing to a
    "who am I" call and returns the profile list.
  - `identity.source: strategy` with a small deriver that picks the `PERSONAL`
    profile: `stable_key` = that profile's numeric `id` (coerced to string via
    the existing numeric-stable-key path used by hubspot/kit), `label` = the
    holder's name (`details.firstName + lastName`, business name as fallback).
    Human-readable, never a hash (OQ2 mongodb precedent).
  - **Capability check before adding anything:** a plain Bearer-scheme token
    verifier already exists in integration-service (loops/tally/paddle added the
    reusable Bearer verifier). Reuse it; only the personal-profile identity
    deriver is potentially new. If an equivalent "pick object from list by field"
    deriver already exists, reuse it — do **not** fork a Wise-specific one
    (Code-Health reuse lens). Any growth is a narrow reviewed enum addition, not
    an adapter.
- **credential.fields:** `api_token: token.access_token`,
  `account_key: connection.account_key`.
- **connection:** `mode: isolated`, `disconnect_mode: local_only`.
- **resources:** `selection/discovery/enforcement: none`.
- **UI icon:** `ui/helio-app/src/integrations/icons/wise.svg` + manual
  `providerIcons.ts` register; i18n `tools.desc.wise` + `wise_api_token` label
  across all 9 locales. (Batch-end shared surfaces.)

## 6. Test plan — five layers

| Layer | Wise-specific plan | Needs external creds? |
|---|---|---|
| **L1** | anycli `go test ./...`: `httptest` fake for each verb — assert request path (`/v1/profiles`, `/v4/profiles/{id}/balances`, `/v1/transfers?profile=…&status=…`, `/v1/profiles/{id}/activities?size=…&nextCursor=…`, `POST /v3/profiles/{id}/quotes`), the injected `Authorization: Bearer` header, `--base-url` override, cursor passthrough on `activity`, and both plain + `--json` error rendering (401/403 → typed `apiError`, exit 1). | No |
| **L2** | Dev harness against the **real Wise sandbox** (V2 host): `ANYCLI_CRED_ACCESS_TOKEN=<sandbox personal token> anycli wise -- profile list`, then `balance list --profile <id>`, `activity list --profile <id>`, `quote create`. Proves field name, header, request shapes, and JSON match live. | **Yes** — a sandbox personal token from the account pool (V2 host). |
| **L3** | `provider-gen` + `provider-gen --check` (five projections regen locally, uncommitted); anycli + integration-service + helio-cli unit suites green; helio-cli built with a local `replace` at the anycli branch. | No |
| **L4** | Singleton + `POST /internal/test-only/connections/seed` with `provider: wise`, `access_token: <sandbox token>` (api_key ⇒ seedable, non-expiring ⇒ seed `access_token` only, no refresh), then `heliox tool wise -- balance list --profile <id>` reaches the live sandbox API through the token gateway. | **Yes** — same sandbox token as L2. |
| **L5** | Pre-flip, hidden: `heliox tool wise auth` → open connect link → **paste the personal token** through the real connect UI (`POST /connections/credentials`) → verifier hits `GET /v1/profiles` → connection shows connected/configured (`GET /connections`) → one **unseeded** live `wise -- profile list` succeeds. This is the api_key key-entry L5 path (master-plan §2), agent-drivable via agent-browser with human fallback. | **Yes** — a real (sandbox or production) personal token pasted at connect time. |

L2/L4/L5 all depend on the account pool supplying **one Wise personal token
minted in the environment the test runs against** (V2 sandbox strongly preferred;
V1 tokens will stop working 2026-06-30 and are not valid in V2). No OAuth app
registration is required (no lane-1 dev-app dependency), which is why Wise can run
in the api_key sub-batch of Wave 3 on pure agent throughput up to the L5 sweep.

## 7. Open risks / notes for the implementer

- **PSD2 scope boundary is real, not conservative.** Do not add funding or
  statement verbs to "round out" the tool — those endpoints will 403 on an EU/UK
  personal token and/or demand SCA the passthrough cannot supply. Keep the verb
  set read/monitor as in §2.
- **Sandbox host churn.** Hardcode nothing but the production default; make
  `--base-url` the single knob. Confirm at stage-1/L2 which sandbox host the
  pool's token belongs to (V1 `*.transferwise.tech` vs V2 `*.wise.tech` /
  `wise-sandbox.com`).
- **Numeric identity id.** `stable_key` derives from a numeric profile `id`;
  confirm the existing numeric-stable-key coercion (hubspot/kit) is on the branch
  base before relying on it, else it is a one-line reuse, not a new capability.
- **No divergence entry, no config appends** — Wise is the cheap end of the
  spectrum: identical ②/③, single self-serve secret, no client id/secret. The
  only shared-surface touches are the registry entry, icon, i18n, and docs at
  batch end.
