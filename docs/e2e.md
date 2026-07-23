# E2E Tests: Operations Runbook

Design: [design 008](design/008-ci-e2e-integration-tests.md).

## How it works

- Per-service tests live in `internal/tools/<pkg>/e2e_test.go` behind the
  `e2e` build tag; the normal test suite never runs them.
- Credentials come from Helio's integration token gateway: the e2e helper
  (`internal/e2e`) calls `GET /connections/token` as the **e2e assistant**,
  using `HELIO_E2E_API_KEY`. Nothing provider-specific is stored in this
  repository.
- CI (`.github/workflows/e2e.yml`) runs only the tools affected by a change
  (same-repo PRs and pushes to `main`), plus a nightly full sweep.
- Blocking is per tool (design 008 D8): the policy table in
  `internal/e2e/affected/affected.go` assigns `skip` / `warn` (default) /
  `required`. warn failures are visible but never block; required failures
  fail the `e2e-gate` job. Branch protection may register **only
  `e2e-gate`** — never individual matrix jobs. Promote a tool to required
  after a stable streak; demote to skip when its provider is known-broken
  (leave a dated comment saying why).

## One-time bootstrap (human)

1. Create a dedicated Helio org + assistant for e2e. Test accounts only —
   never production data.
2. Connect each service's test account(s) through the normal Helio connect
   flow and grant them to the e2e assistant. For counterpart-account tests
   (e.g. gmail), connect two accounts and note their account labels.
3. Obtain the assistant's current `HELIO_API_KEY` (a Clerk `ak_*` key from
   its runtime). Set repository secret `HELIO_E2E_API_KEY` to it.
4. Set repository variable `HELIO_E2E_API_BASE` to the API base heliox uses
   (including any `/v1` prefix).
5. Create a fine-grained PAT with `secrets: write` on this repository; set
   it as repository secret `E2E_SECRETS_PAT`.

The key self-rotates nightly (48h TTL, refreshed every 24h by the
`rotate-key` job). If CI is down for more than 48h the chain breaks —
repeat step 3.

## Running locally

Against the gateway (tools already connected in Helio):

    HELIO_E2E_API_KEY=ak_... HELIO_E2E_API_BASE=https://... make e2e TOOL=attio

With a hand-held token, before the provider exists in Helio (design 008 D9):

    ANYCLI_E2E_CRED_PRIMARY_ACCESS_TOKEN=<token> make e2e TOOL=attio

Multi-field credentials use one variable per field
(`ANYCLI_E2E_CRED_PRIMARY_<FIELD>`); counterpart accounts use the account
label (`ANYCLI_E2E_CRED_SECONDARY_<FIELD>`, selected by passing account
"secondary" to `e2e.RunTool`).

## Adding e2e tests for a service

1. Create `internal/tools/<pkg>/e2e_test.go` with `//go:build e2e`, package
   `<pkg>_test`. Copy the pattern from `internal/tools/attio/e2e_test.go`:
   one read smoke test, then closed-loop write chains (create → verify →
   delete → verify gone). Name every created object with `e2e.Prefix()`.
2. Verify locally with a real token (env override above) before merging.
3. After the provider is connected in Helio, the nightly sweep picks the
   tool up automatically. Until then its tests skip and appear in the
   nightly "E2E pending" summary.
