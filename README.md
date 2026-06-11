# AnyCLI

**An embeddable Go core for running authenticated CLI/API tools with injected credentials.**

AnyCLI is a library, not a standalone CLI. A host program (e.g. Helio's `heliox`) embeds it in-process, supplies a `CredentialResolver` for its own credential sources, and calls `Execute`. AnyCLI loads the matching embedded tool definition, resolves and injects credentials (env var / CLI flag / ephemeral config file), runs before/after middleware, and execs the underlying binary or built-in service.

The supported tools and their definitions live **inside** AnyCLI (embedded JSON). The embedder never supplies tool definitions — only a credential resolver and an optional cache.

## Why?

LLMs are trained on billions of CLI examples. They already know `git`, `gh`, `curl`, `jq`, and thousands of other tools. MCP forces agents to learn new schemas from scratch, consuming far more tokens per operation with lower reliability. CLI is the natural interface between agents and the world. [Read the full rationale.](./WHY_ANY_CLI.md)

AnyCLI's job is the missing piece: let an agent run those tools **authenticated**, without the host shipping long-lived secrets to disk. The host owns where credentials come from; AnyCLI owns how they are injected and how long they live.

## Embeddable API

```go
import "github.com/heliohq/anycli"

// 1. Construct an engine. Cache is optional; nil uses an in-memory default.
engine, err := anycli.New(anycli.Config{
    Cache: myCache, // optional: implements anycli.Cache
})
if err != nil {
    return err
}

// 2. Run a tool. The resolver supplies credentials for this tool; AnyCLI
//    loads the embedded definition, injects, and execs.
exitCode, err := engine.Execute(ctx, anycli.Tool("gh"), []string{"pr", "list"}, resolver)
```

### Config

```go
type Config struct {
    // Cache is the credential cache the engine uses to avoid re-resolving on
    // every call. Optional — nil installs an in-memory cache. The on-disk
    // cache that the old standalone CLI used is gone; the host decides the
    // storage (per-process, per-assistant, shared, …) by supplying a Cache.
    Cache Cache
}
```

### CredentialResolver (host-supplied)

The resolver is the **only** thing that crosses the boundary into AnyCLI. It returns in-memory data; AnyCLI owns injection, caching, and lifecycle. Where the data comes from (stored OAuth, an on-demand minted token, anything) is entirely the host's business — the resolver never learns how the data is injected.

```go
type CredentialResolver interface {
    // Resolve returns the credential fields for one tool, plus when they go stale.
    Resolve(ctx context.Context, tool Tool) (*Credential, error)
}

type Credential struct {
    Data       map[string]any // credential fields; bindings index into it by field name
    CacheUntil time.Time      // when this credential goes stale (drives the cache)
}
```

### Cache (host-supplied, optional)

```go
type Cache interface {
    Get(tool string) (*CacheEntry, bool)
    Set(tool string, entry *CacheEntry)
    MarkStale(tool string)
}
```

`CacheEntry` stores only the extracted credential fields plus `CacheUntil`/`Stale`. Supply your own (e.g. an in-memory map keyed per assistant) or pass `nil` to use the built-in in-memory cache.

### Tools

`Tool` is `type Tool string`. AnyCLI ships **no** tool-name constants yet — the supported-tool definitions are added internally to AnyCLI in a later round. Pass a raw `anycli.Tool("…")` whose name matches an embedded definition; an unknown tool is an error from `Execute`, not a compile error.

## Documentation

- [Why AnyCLI](WHY_ANY_CLI.md) — CLI vs MCP for agents
- [Credential Lifecycle](docs/credential-lifecycle.md) — how credentials are resolved, cached, and injected at runtime
- [Tool Definition Schema](docs/definition-schema.md) — field reference for the embedded tool definitions
- [Design 001: Vault Credential Integration](docs/design/001-vault-credential-integration.md) — credential schema and injection modes
- [Design 002: Embeddable Core + Credential Resolver](docs/design/002-embeddable-core-and-credential-resolver.md) — the library architecture

## License

Apache License 2.0
