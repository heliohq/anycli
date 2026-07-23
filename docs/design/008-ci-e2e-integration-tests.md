# CI E2E Integration Tests

**Date:** 2026-07-24
**Status:** Proposed
**Scope:** Add real-API e2e tests to CI: credentials resolved from Helio's integration token gateway with a single API-key secret, per-tool change detection so only affected tools run, per-service test cases owned by each service package. Library positioning (design 002) and the account-aware pipeline (design 003) are unchanged; e2e consumes them as a normal embedder would.

## 1. Background

CI today (`.github/workflows/ci.yml`) only runs `go build` / `go vet` / `go test` — all offline. Nothing verifies that an embedded tool definition or built-in service actually works against the real provider API. Regressions (provider API drift, broken credential contracts, wrong field names) surface only when a Helio user hits them.

There are ~148 embedded tool definitions (`definitions/tools/*.json`) backed by built-in services under `internal/tools/<pkg>/`. Running all of them on every commit is impossible (speed, provider rate limits), and storing per-field secrets is impossible too (GitHub caps repositories at 100 secrets).

Credential lifecycle (OAuth consent, refresh, rotation) is already solved in production by Helio's integration token gateway: heliox's resolver (`helio-cli/internal/toolcred`) is a plain authenticated `GET /connections/token?provider=X&account=Y` returning `{access_token, expires_at, credential{...}}`, where `credential` is the bundle-projected, provider-neutral AnyCLI input and the server lazy-refreshes before handing tokens out. E2E reuses that instead of building a parallel credential-ops stack.

An alternative considered and rejected: driving tests through the heliox CLI itself (with an `ANYCLI_PATH` passthrough seam so the PR's anycli binary is exercised). It works, but couples anycli's tests to heliox's CLI surface, coarsens assertions to subprocess output parsing, and tests an integration face that belongs in the heliox repo. Resolving credentials from the gateway while keeping tests in-process gets the same credential-ops win without the coupling.

## 2. Decisions

### D1 — Credentials: Helio-managed, one self-rotating API-key secret

A dedicated **e2e assistant** in a dedicated Helio org holds all test connections: each service is connected once through Helio's normal connect flow (OAuth consent, API-key entry) by a human and granted to that assistant; the gateway owns provider-token refresh and rotation from then on. Counterpart accounts are just multiple connections of the same tool (e.g. two Gmail test users), selected by their connection account label.

The anycli repository stores exactly one secret: `HELIO_E2E_API_KEY` — the e2e assistant's AI-user Clerk `ak_*` key, the same kind of key the runtime injects into assistant pods. `GET /connections/token` requires the caller to *be* an assistant AI user (`integration-service`: the caller IS the assistant whose connections are resolved), which is why the key must belong to the e2e assistant, not to a human user or daemon.

**Key lifetime**: runtime keys expire after 48h by design (design 206); a static secret would die in two days. The key therefore self-rotates: a human bootstraps once with a live key from the e2e assistant's runtime; from then on the nightly job (D6) exchanges the current key for a fresh one via `POST /users/ai/<id>/api-key-refresh` (the runtime self-renewal endpoint — a key can only rotate itself; the AI-user id comes from `GET /user/me` with the same key, so nothing is hardcoded) and writes it back with `gh secret set`. This needs a repo token with `secrets:write` — one write-back for one key, not per-provider machinery. If CI is down past the 48h window the chain breaks and a human re-bootstraps (minutes of work).

Consequences accepted:

- E2E depends on Helio API availability; acceptable because e2e is non-blocking (D8).
- The key's blast radius is every connection granted to the e2e assistant; its org must contain only dedicated test accounts, never production data. The 48h TTL is kept — no long-lived key is minted, so the production security model is not weakened.
- Tools with no connection granted are **skipped, not failed** — services onboard by connecting an account, not by editing a secret.
- Providers whose connect flow Helio does not support yet are out of e2e scope until Helio supports them (no side-channel credentials).

### D2 — E2E resolver: direct token-gateway client

A shared test helper package implements `anycli.CredentialResolver` against the gateway, mirroring `helio-cli/internal/toolcred`:

- `GET /connections/token?provider=<key>&account=<name>` with `Authorization: Bearer $HELIO_E2E_API_KEY`; the response's `credential` map is the provider-neutral AnyCLI input, used directly as `anycli.Credential.Data`.
- `(tool, account)` maps straight onto the design-003 selector: `""` selects the primary connection; a connection's account label (as shown/renamed in Helio) selects a counterpart account. Selection misses surface as the gateway's 404/409 (with candidate list), not silent fallbacks.
- Tokens are cached in-process until `expires_at` (minus a safety margin), matching toolcred's TTL behavior.
- A "not connected" gateway response surfaces as a distinguishable error so tests `t.Skip` uniformly instead of failing.
- The anycli-id ↔ provider-key mapping (`drive`→`google_drive`, `bill-com`→`bill_com`, …) lives in helio-cli's internal `toolToProvider` table and is not importable; the helper carries a copy with a comment pointing at the source of truth. The table is static and small (~25 entries); promote it to an exported anycli package later only if drift actually bites.

The base URL comes from `HELIO_E2E_API_BASE` (defaulting to the production API base). `ANYCLI_CRED_*` in `cmd/anycli` stays as-is for human debugging.

**Local credential override.** The helper also accepts credentials from environment variables (`ANYCLI_E2E_CRED_<ACCOUNT>_<FIELD>`, e.g. `ANYCLI_E2E_CRED_PRIMARY_ACCESS_TOKEN`), taking precedence over the gateway. This exists for the anycli-first sequencing gap (see D9): the author of a new tool holds a real token during development and runs the same e2e cases locally against it before the provider exists in Helio at all. CI never sets these variables — in CI the gateway is the only source.

**Risk — endpoint privatization.** Helio may someday restrict `/connections/token`. Note the dependency is shared: heliox itself (including BYOA daemons outside Helio's network) is a public-path client of the same endpoint with the same key kind, so any restriction must ship a heliox replacement path — and e2e, being "a heliox impersonating an assistant", follows that path. Two guards: (1) the endpoint dependency is confined to this one helper package, so a contract change has a one-file blast radius; (2) anycli e2e must be registered alongside heliox in any Helio-side consumer inventory for this endpoint. Worst-case fallbacks, in order: self-hosted runner inside the network → drive tests through heliox's replacement mechanism → self-managed credential store (the rejected alternative).

### D3 — heliox and Helio server: zero changes

Nothing is added to heliox or the Helio server for this design. The gateway endpoint, lazy refresh, and account selection already exist as production behavior; e2e is just another authenticated client of them. (A heliox-side `ANYCLI_PATH` execution seam remains a reasonable idea *for the heliox repo's own integration tests* — pinning a known anycli binary — but it is not part of this design.)

### D4 — Test layout: per-service, build-tagged, closed-loop

- Each service owns `internal/tools/<pkg>/e2e_test.go` guarded by `//go:build e2e`. Regular `go test ./...` never touches them.
- CI runs `go test -tags e2e ./internal/tools/<pkg>/...` for each selected tool.
- **Closed-loop cases**: write operations are chained so one case's output is the next case's input and the chain ends by deleting what it created — e.g. create record → update it → verify → delete it → verify gone. The delete test *is* the cleanup.
- **Counterpart accounts** verify real effect: gmail primary sends to secondary, then the secondary account's credential reads the inbox and asserts receipt. Same pattern for any send/receive or collaboration semantics.
- All created data uses the prefix `anycli-e2e-<run_id>-` so leftovers from interrupted runs are identifiable.
- Missing credentials → `t.Skip` with an explicit message.

### D5 — Change detection: manifest-driven mapping

Definition filenames and package names disagree (`adobe-sign.json` vs `internal/tools/adobesign/`), so path-string guessing is out. A small checked-in manifest (or `go generate`d table) maps:

```
tool name ↔ definitions/tools/<file>.json ↔ internal/tools/<pkg>/
```

A CI script diffs `git diff --name-only $BASE...HEAD` against the manifest and emits the affected tool list as the job matrix:

- change under `definitions/tools/<x>.json` or `internal/tools/<x>/**` → run tool *x*'s e2e;
- change to shared code (`anycli.go`, `internal/exec`, `internal/credential`, `internal/middleware`, `internal/registry`, `definitions/embed.go`) → run a fixed **smoke subset** (5–10 representative tools covering both tool types and both single- and multi-field credential shapes), not all 148;
- docs/workflow-only changes → no e2e.

### D6 — Triggers

- **Same-repo pull requests** and **pushes to `main`**: selective run per D5. Fork PRs get no secrets by GitHub's rules and are explicitly excluded (`if: github.event.pull_request.head.repo.full_name == github.repository`).
- **Nightly schedule**: full smoke sweep across every tool connected to the e2e assistant. Serves four jobs: connection health monitoring (know a connection died or a provider drifted before a code change trips over it), sweep-cleanup of `anycli-e2e-*` leftovers older than a day, **pending-tool reporting** (the "has e2e tests but no gateway connection" list from D9, so the debt stays visible), and **API-key rotation** — exchange the current `HELIO_E2E_API_KEY` for a fresh 48h key and `gh secret set` it back (D1). The rotation step runs even when tests fail, and its own failure pages loudly: a missed rotation is 24h from a dead key.

### D7 — Concurrency control

Real accounts are shared mutable state; concurrent runs must not stomp each other.

- **Workflow level**: every e2e matrix job sets `concurrency: { group: e2e-<tool>, cancel-in-progress: false }`. Concurrency groups are repository-scoped, so PR runs, main-push runs, and nightly jobs all serialize through the same per-tool queue. GitHub keeps one running + one pending run per group (a newer pending run supersedes the older pending one — latest commit wins); running jobs are never cancelled mid-flight, so closed loops complete and don't strand data.
- **Token level**: refresh is server-side in the gateway (single point, provider-aware), so concurrent e2e runs cannot race a refresh_token from the client side at all; the helper additionally caches tokens in-process until `expires_at`.
- **Data level**: tools sharing one credential family (e.g. Google family on the same Workspace users) may run concurrently — the `anycli-e2e-<run_id>-` prefix plus closed-loop ownership keeps them from colliding, since each test only touches artifacts it created. If a family proves collision-prone in practice, its tools share one concurrency group name instead of per-tool groups.

### D8 — E2E does not block merge

The e2e workflow is not a required status check. Third-party APIs flake, rate-limit, and go down; one provider outage must not freeze the team. Failures are visible on the PR and in nightly reports; tightening to required-check status is a future decision once stability is proven.

### D9 — The anycli-first sequencing gap

New tools usually land in anycli before Helio finishes the provider-side integration (catalog entry, connect flow, bundle projection). Until then the gateway has nothing to connect, so gateway-backed e2e cannot run — yet first merge is exactly when real-API verification matters most. The lifecycle:

1. **Author verifies locally before merge**: the tool author already holds a real token (they built the tool with it); they run the tool's e2e cases locally via the D2 env override. The cases are exercised against the real API before the PR lands, even though Helio has never heard of the provider.
2. **Merged, pending**: in CI the tool's tests skip (no connection); the nightly pending-tool report (D6) lists it so the gap is visible, not forgotten.
3. **Helio integrates → connect → automatic pickup**: once the provider exists in Helio, a human connects a test account to the e2e assistant; the next nightly starts exercising the tool with no anycli-side change, and any definition↔bundle field mismatch surfaces there.

### D10 — Rollout

Infrastructure first (gateway resolver helper, manifest, workflow, the e2e assistant and its bootstrapped key), then services onboard incrementally — each service adds its `e2e_test.go` and connects a test account granted to the e2e assistant independently. No flag-day migration; tools without tests or connections simply don't run.

## 3. Out of scope

- Any credential storage or acquisition in this repo — the gateway owns credential lifecycle (D1); providers Helio cannot connect yet are out of e2e scope.
- Fork-PR e2e (`pull_request_target` is a known security foot-gun; not worth it).
- Load/performance testing against providers; e2e asserts correctness only.
- heliox↔anycli integration testing (the `ANYCLI_PATH` seam idea) — belongs in the heliox repo.
