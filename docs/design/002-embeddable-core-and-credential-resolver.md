# AnyCLI as an Embeddable Core + Pluggable Credential Resolver

**Date:** 2026-06-08
**Status:** Accepted
**Scope:** Reposition AnyCLI as an embeddable Go library; introduce a pluggable `CredentialResolver`; remove the standalone CLI implementation. The injection modes from 001 are kept, while its vault-specific credential-source schema and standalone-CLI assumptions are superseded.

## 1. Background

### Current state

AnyCLI today is a standalone CLI: `main.go` + cobra `cmd/*` (`exec`, `auth`, `install`, `list`, …), an `os.Args[0]`-based shim (`internal/shim`), a binary installer (`internal/installer`, github/npm), and an env-driven credential layer (`internal/credential/provider.go`) that resolves either from a vault HTTP endpoint (`ANYCLI_VAULT_*` → `GET /vault/credentials/effective?tool=`, `internal/credential/vault.go`) or from local JSON files (`internal/credential/local.go`).

The genuinely useful part — load a tool definition, resolve credentials, inject them (env / arg / file), run before/after middleware, exec the underlying binary or a built-in service — is buried in the internal packages and only reachable through the CLI.

### Problem

Helio's `heliox` needs to drive that exec core **in-process** (embedded, not as a subprocess), and to resolve credentials **itself** — per-assistant, from its own sources (stored OAuth in vault, or backend-minted short-lived tokens) — not from AnyCLI's built-in vault/local resolvers. (Consumer design: Helio `docs/design/227-ai-teammate-oauth-integration`.)

The standalone-CLI machinery (cobra, shim, installer, the built-in default resolvers) is the wrong layer for embedding and is dead weight for that use.

### Goals

1. AnyCLI is an **embeddable Go library**: a host constructs an engine (`New(Config)`) and runs tools via `Engine.Execute(ctx, tool, args, resolver)`.
2. The credential **source** is injected via a `CredentialResolver`. The resolver only returns in-memory data; AnyCLI owns injection, caching, and lifecycle.
3. The credential **cache** is consumer-supplied via a `Cache` interface (the host can use a per-process / per-assistant in-memory store); AnyCLI supplies an in-memory default. Tool **definitions** stay internal/embedded — never consumer-supplied.
4. **Remove** the standalone CLI implementation. A standalone CLI, if needed later, is a thin cobra shell over the same core.
5. Clean break — AnyCLI has not shipped; no backward compatibility.

Non-goals: re-adding a CLI now; consumer-supplied tool definitions / host-injected registries.

## 2. Positioning

**AnyCLI is an embeddable core library** for "run an underlying CLI/API tool with injected credentials + middleware." It is the engine **plus the embedded definitions for the tools it supports** — not a generic blank engine, and not a standalone binary.

- The product is: the exec engine (`New` + `Engine.Execute`), the `CredentialResolver` seam, the consumer-supplied `Cache` seam, and the declarative tool-definition schema with its **internal embedded** definition set.
- The **host** (e.g. `heliox`) embeds `github.com/heliohq/anycli`, constructs an engine with `New(Config{Cache})`, provides a `CredentialResolver`, and calls `Engine.Execute`. The host supplies **only** a resolver and (optionally) a cache — never tool definitions.
- If a standalone CLI is ever wanted again, it is a **thin cobra shell**: it supplies a resolver (e.g. the old HTTP-vault one) and calls `Engine.Execute`. The core stays the single source of truth; the shell adds no logic.

```
 host (e.g. heliox)                    AnyCLI core (library)
 ┌────────────────────┐               ┌──────────────────────────────────────┐
 │ CredentialResolver  │── inject ────▶│ e := New(Config{Cache})                │
 │ Cache (optional)    │               │ e.Execute(ctx, tool, args, resolver)   │
 │ (host's sources)    │               │  load embedded def → resolve(resolver) │
 └────────────────────┘               │  → cache → inject(env|arg|file)        │
                                       │  → middleware → exec binary/svc        │
 future: thin cobra shell ───────────▶ (calls the same Engine.Execute)
```

## 3. Core API

```go
// Tool identifies a tool by its definition name. It is a named type for
// type-safety and discoverability. ListTools reports the embedded definition
// set; an unknown name is an Execute error, not a compile error.
type Tool string

// Config carries the consumer-supplied initialization. It carries ONLY a Cache —
// tool definitions are internal to AnyCLI (embedded) and are never
// consumer-supplied. There is no Registry / tool-defs field.
type Config struct {
    // Cache is the credential cache the engine uses. Optional: a nil Cache
    // installs the in-memory default.
    Cache Cache
}

// New constructs an Engine from cfg. A nil cfg.Cache installs an in-memory cache.
func New(cfg Config) (*Engine, error)

// Engine is the embeddable core. Execute loads the embedded definition for tool,
// resolves and injects its credentials via resolver, runs middleware, and execs
// the underlying binary or built-in service. resolver must be non-nil; an unknown
// tool (no embedded definition) returns an error.
func (e *Engine) Execute(ctx context.Context, tool Tool, args []string, resolver CredentialResolver) (exitCode int, err error)

// Cache is the credential cache, consumer-supplied so a host can use a
// per-process / per-assistant in-memory store instead of any on-disk cache. The
// engine interprets freshness (CacheUntil / Stale via CacheEntry.IsValid); the
// implementation only stores and retrieves, keyed by (tool, account).
type Cache interface {
    Get(tool string) (*CacheEntry, bool)
    Set(tool string, entry *CacheEntry)
    MarkStale(tool string)
}

// NewMemoryCache returns the in-memory Cache the engine installs by default.
func NewMemoryCache() Cache

// The ONLY thing that crosses the resolver boundary: in-memory data.
type CredentialResolver interface {
    // Resolve returns the credential fields for one tool/account pair, plus
    // when they go stale. An empty account selects the host's default.
    Resolve(ctx context.Context, tool Tool, account string) (*Credential, error)
}

type Credential struct {
    // Data holds the tool's credential fields. Bindings index into it by the
    // definition's source field (e.g. Data["access_token"]). Credential
    // bindings are string-valued by contract.
    Data map[string]string
    // CacheUntil is when this credential goes stale. The resolver is the only party
    // that knows it (a stored token's expiry, a minted token's TTL, …). AnyCLI uses
    // it to manage its cache; AnyCLI does not decide it.
    CacheUntil time.Time
}
```

### Responsibility split

- **Resolver (host's job):** return in-memory `Data` + `CacheUntil`. Where the data comes from (vault, on-demand mint, anything) and how it is obtained is entirely the host's business. **The resolver never learns how the data is injected.**
- **Cache (host's job, optional):** store and retrieve `CacheEntry` per `(tool, account)`. The host chooses the storage (per-process, per-assistant, shared); the engine decides freshness. A nil `Config.Cache` installs the in-memory default.
- **AnyCLI core (internal, not exposed):**
  - **Resolve tool definitions** from the **internal embedded** set (never from `Config`).
  - **Inject** per the tool definition's `Inject` spec — env var / CLI arg / file. The injection method is AnyCLI's internal consumption logic and is not visible to the resolver.
  - **Cache** by `CacheUntil`: serve cached until then; re-call `Resolve` when expired; mark stale after explicit service credential rejection or a failed external CLI execution — all through the supplied `Cache`.
  - **Lifecycle:** file-type injection writes an **ephemeral temp file** and redirects the underlying tool to it (`config_env` / `config_flag`), then cleans up. Resolver-supplied credentials are never written into a persistent user config.
  - **Middleware** (before/after rules) and **exec** of the underlying binary or built-in service.

This is the line that was previously blurred by the `isVaultMode` flag: "where credentials come from" (resolver) is now fully separated from "how they are consumed and how long they live" (AnyCLI). There is no `Managed` flag — resolver-supplied credentials are always treated as ephemeral/managed.

## 4. What is kept / what is removed

### Kept — the core

- `internal/registry` — provider-neutral declarative tool-definition schema (`Definition` / `CredentialBinding` / `CredentialSource` / `CredentialInject`).
- `definitions/` — the **internal embedded-definitions mechanism**: a `go:embed` of the `tools/` directory plus the `LoadBundled(name)` and `ListBundled()` loaders. This mechanism is never consumer-supplied.
- `internal/credential` — injection (env/arg/file) + format patchers + the cache + mark-stale; **plus** the `CredentialResolver` interface, the `Credential` type, and the consumer-supplied `Cache` interface with an in-memory default (`NewMemoryCache`).
- `internal/middleware` — the before/after rule engine.
- `internal/exec` — refactored into the `Engine` type (`NewEngine(cache)` + `Engine.Execute`), exposed publicly via `New(Config)` + `Engine.Execute`.
- `internal/tools` — built-in service-type tools (for providers with no native CLI).

### Removed — the standalone CLI (re-addable later as a thin shell) + the CLI-shaped cache

- `main.go`, `cmd/*` (cobra: root / exec / auth / install / list / uninstall / update / version).
- `internal/shim` — `os.Args[0]`-based tool-name dispatch (a CLI invocation concern).
- `internal/installer` — `any install` fetching tool binaries from github/npm to disk. In embedded use, the host / pod image provisions the underlying binaries.
- The **built-in default resolvers**: the env-driven vault mode (`provider.go` `GetVaultConfig` / `IsVaultMode`), the HTTP `effective?tool=` client (`vault.go`), and the local-file resolver (`local.go`). The core now takes an injected resolver instead.
- The **hardcoded on-disk file cache** (`~/.anycli/cache/<tool>.json`, `ReadCache` / `WriteCache` / `MarkStale` over the filesystem, and `config.CacheDir`). It was a CLI artifact. The `cache_until`-driven semantics are kept behind the `Cache` interface; the engine installs an in-memory default and the host may supply its own.
- The **bundled example tool definitions** (`definitions/gh.json`, `definitions/wrangler.json`) and their `Tool` constants (`ToolGitHub` / `ToolWrangler`). The embedded-definitions mechanism itself is kept (see above); only the example payloads are removed.

## 5. Host credential gateway

001 defined a vault HTTP contract inside AnyCLI. With this design that **HTTP client leaves the core**: a host-specific resolver owns whatever network or storage contract produces `Credential{Data, CacheUntil}`.

- Helio's `heliox` resolver calls integration-service's `/connections/token` gateway and returns its provider-neutral string credential projection as `Credential{Data, CacheUntil}` (Helio design 227).
- A future standalone CLI shell can re-introduce the HTTP client as its resolver.

The core only ever knows `Credential{Data, CacheUntil}`.

## 6. Tool definitions

Tool definitions are **internal to AnyCLI**. They stay declarative JSON and are **embedded** in the binary under `definitions/tools/` via `go:embed`, loaded by `LoadBundled`. They are **not** consumer-supplied: the embedder provides only a `CredentialResolver` and an optional `Cache` — never tool definitions, and there is **no** consumer-supplied registry seam (no `Config.Registry`, no host-injected definition set).

AnyCLI is "the engine + the definitions for the tools it supports," not a generic blank engine. Supported definitions are added internally under `definitions/tools/`; the host cannot add or replace them at runtime.

## 7. Clean break

AnyCLI had not shipped when this clean break landed. The removed packages were deleted outright — no deprecation or compatibility shims. Credential bindings and injection modes remain, while the source selector is reduced to the provider-neutral resolver `field`; standalone-only `vault_tool`, `local_key`, and `auth_flag` fields are gone.

## 8. Open / future

- A standalone CLI shell — a thin cobra wrapper over `Engine.Execute`, supplying the HTTP-vault resolver — if/when a non-embedded use appears.
- Additional host-supplied resolver implementations; OAuth and PAT collection remain outside AnyCLI.

## References

- `docs/design/001-vault-credential-integration.md` — superseded vault/local architecture; retained as historical context for the three injection modes.
- `docs/credential-lifecycle.md`, `docs/definition-schema.md`.
- Helio `docs/design/227-ai-teammate-oauth-integration` — consumer-side connection and token-gateway architecture.
