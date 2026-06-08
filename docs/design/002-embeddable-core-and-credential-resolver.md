# AnyCLI as an Embeddable Core + Pluggable Credential Resolver

**Date:** 2026-06-08
**Status:** Draft
**Scope:** Reposition AnyCLI as an embeddable Go library; introduce a pluggable `CredentialResolver`; remove the standalone CLI implementation. The credential **schema** and **injection modes** from 001 are kept; the standalone-CLI assumptions in 001 are superseded.

## 1. Background

### Current state

AnyCLI today is a standalone CLI: `main.go` + cobra `cmd/*` (`exec`, `auth`, `install`, `list`, …), an `os.Args[0]`-based shim (`internal/shim`), a binary installer (`internal/installer`, github/npm), and an env-driven credential layer (`internal/credential/provider.go`) that resolves either from a vault HTTP endpoint (`ANYCLI_VAULT_*` → `GET /vault/credentials/effective?tool=`, `internal/credential/vault.go`) or from local JSON files (`internal/credential/local.go`).

The genuinely useful part — load a tool definition, resolve credentials, inject them (env / arg / file), run before/after middleware, exec the underlying binary or a built-in service — is buried in the internal packages and only reachable through the CLI.

### Problem

Helio's `heliox` needs to drive that exec core **in-process** (embedded, not as a subprocess), and to resolve credentials **itself** — per-assistant, from its own sources (stored OAuth in vault, or backend-minted short-lived tokens) — not from AnyCLI's built-in vault/local resolvers. (Consumer design: Helio `docs/design/215` — AI Teammate OAuth integration.)

The standalone-CLI machinery (cobra, shim, installer, the built-in default resolvers) is the wrong layer for embedding and is dead weight for that use.

### Goals

1. AnyCLI is an **embeddable Go library**, with a single entrypoint: `Execute(ctx, tool, args, resolver)`.
2. The credential **source** is injected via a `CredentialResolver`. The resolver only returns in-memory data; AnyCLI owns injection, caching, and lifecycle.
3. **Remove** the standalone CLI implementation. A standalone CLI, if needed later, is a thin cobra shell over the same core.
4. Clean break — AnyCLI has not shipped; no backward compatibility.

Non-goals: re-adding a CLI now; host-injected registries; changing the 001 credential schema.

## 2. Positioning

**AnyCLI is an embeddable core library** for "run an underlying CLI/API tool with injected credentials + middleware." It is **not** a standalone binary.

- The product is: the exec core (`Execute`), the `CredentialResolver` seam, and the declarative tool-definition schema (from 001).
- The **host** (e.g. `heliox`) embeds AnyCLI via Go module `replace github.com/shipbase/anycli => …`, provides a `CredentialResolver`, and calls `Execute`.
- If a standalone CLI is ever wanted again, it is a **thin cobra shell**: it supplies a resolver (e.g. the old HTTP-vault one) and calls `Execute`. The core stays the single source of truth; the shell adds no logic.

```
 host (e.g. heliox)                    AnyCLI core (library)
 ┌───────────────────┐                ┌──────────────────────────────────┐
 │ CredentialResolver │── inject ─────▶│ Execute(ctx, tool, args, resolver) │
 │ (host's sources)   │                │  load def → resolve(resolver)     │
 └───────────────────┘                │  → cache → inject(env|arg|file)   │
                                       │  → middleware → exec binary/svc   │
 future: thin cobra shell ───────────▶ (calls the same Execute)
```

## 3. Core API

```go
// Tool identifies a tool by its definition name. It is a NAMED TYPE (not a bare
// string) for type-safety + discoverability — AnyCLI ships one constant per
// embedded definition. It is NOT a closed compile-time set: validity is checked
// at runtime against the embedded definitions, so adding a tool stays "drop in a
// JSON definition" (001 goal #5), optionally plus a constant. Callers may pass a
// constant (ToolGitHub) or a raw Tool("…") for a definition AnyCLI doesn't ship a
// constant for; an unknown tool is an error from Execute, not a compile error.
type Tool string

const (
    ToolGitHub  Tool = "github"
    ToolSlack   Tool = "slack"
    ToolNotion  Tool = "notion"
    // … one per embedded definition
)

// The ONLY thing that crosses the boundary: the resolver returns in-memory data.
type CredentialResolver interface {
    // Resolve returns the credential fields for one tool, plus when they go stale.
    Resolve(ctx context.Context, tool Tool) (*Credential, error)
}

type Credential struct {
    // Data holds the tool's credential fields. Bindings index into it by the
    // definition's VaultField (e.g. Data["access_token"]). Its shape is the
    // resolver's choice.
    Data map[string]any
    // CacheUntil is when this credential goes stale. The resolver is the only party
    // that knows it (a stored token's expiry, a minted token's TTL, …). AnyCLI uses
    // it to manage its cache; AnyCLI does not decide it.
    CacheUntil time.Time
}

// The embeddable entrypoint. resolver must be non-nil; an unknown tool (no
// embedded definition) returns an error.
func Execute(ctx context.Context, tool Tool, args []string, resolver CredentialResolver) (exitCode int, err error)
```

### Responsibility split

- **Resolver (host's job):** return in-memory `Data` + `CacheUntil`. Where the data comes from (vault, on-demand mint, anything) and how it is obtained is entirely the host's business. **The resolver never learns how the data is injected.**
- **AnyCLI core (internal, not exposed):**
  - **Inject** per the tool definition's `Inject` spec — env var / CLI arg / file. The injection method is AnyCLI's internal consumption logic and is not visible to the resolver.
  - **Cache** by `CacheUntil`; serve cached until then; re-call `Resolve` when expired; `mark-stale` + re-resolve on exec failure.
  - **Lifecycle:** file-type injection writes an **ephemeral temp file** and redirects the underlying tool to it (`config_env` / `config_flag`), then cleans up. Resolver-supplied credentials are never written into a persistent user config.
  - **Middleware** (before/after rules) and **exec** of the underlying binary or built-in service.

This is the line that was previously blurred by the `isVaultMode` flag: "where credentials come from" (resolver) is now fully separated from "how they are consumed and how long they live" (AnyCLI). There is no `Managed` flag — resolver-supplied credentials are always treated as ephemeral/managed.

## 4. What is kept / what is removed

### Kept — the core

- `internal/registry` — declarative tool-definition schema (`Definition` / `CredentialBinding` / `CredentialSource` / `CredentialInject`, from 001) + loading from the **embedded** definition set.
- `internal/credential` — injection (env/arg/file) + format patchers + cache + mark-stale; **plus** the new `CredentialResolver` interface and `Credential` type.
- `internal/middleware` — the before/after rule engine.
- `internal/exec` — refactored into the public `Execute(ctx, tool, args, resolver)`.
- `internal/tools` — built-in service-type tools (for providers with no native CLI).
- `definitions/` — embedded tool definitions.

### Removed — the standalone CLI (re-addable later as a thin shell)

- `main.go`, `cmd/*` (cobra: root / exec / auth / install / list / uninstall / update / version).
- `internal/shim` — `os.Args[0]`-based tool-name dispatch (a CLI invocation concern).
- `internal/installer` — `any install` fetching tool binaries from github/npm to disk. In embedded use, the host / pod image provisions the underlying binaries.
- The **built-in default resolvers**: the env-driven vault mode (`provider.go` `GetVaultConfig` / `IsVaultMode`), the HTTP `effective?tool=` client (`vault.go`), and the local-file resolver (`local.go`). The core now takes an injected resolver instead.

## 5. The `effective?tool=` HTTP contract

001 defined a vault HTTP contract (`GET /vault/credentials/effective?workspace_id=&tool=` → `{credentials: [{data, cache_until, status, …}]}`). With this design that **HTTP client leaves the core** (it was the standalone resolver). The contract does not disappear — it becomes the shape a **vault-backed resolver produces**:

- Helio's `heliox` resolver fetches/mints and returns `Credential{Data, CacheUntil}`; Helio's vault/broker serves the `effective?tool=` shape on its side (Helio `docs/design/215`).
- A future standalone CLI shell can re-introduce the HTTP client as its resolver.

The core only ever knows `Credential{Data, CacheUntil}`.

## 6. Tool definitions

Tool definitions stay declarative JSON (001 schema) and are **embedded** (`definitions/`). Hosts rely on the embedded set. Letting a host inject a custom registry is a possible future addition, **not** in scope now.

## 7. Clean break

AnyCLI has not shipped. The removed packages are deleted outright — no deprecation, no compatibility shims. The 001 credential schema (bindings / injection modes / file formats) is unchanged.

## 8. Open / future

- A standalone CLI shell — a thin cobra wrapper over `Execute`, supplying the HTTP-vault resolver — if/when a non-embedded use appears.
- Host-injected registry (custom tool definitions) if a host needs definitions outside the embedded set.
- Service-type tools' own auth flows.

## References

- `docs/design/001-vault-credential-integration.md` — credential schema + three injection modes (kept).
- `docs/credential-lifecycle.md`, `docs/definition-schema.md`.
- Helio `docs/design/215` — AI Teammate OAuth integration (the consumer; defines the host-side `CredentialResolver` implementation and the `effective?tool=` server side).
