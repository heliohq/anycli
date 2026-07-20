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
    // Resolve returns the credential fields for one tool/account pair, plus
    // when they go stale. An empty account selects the host's default.
    Resolve(ctx context.Context, tool Tool, account string) (*Credential, error)
}

type Credential struct {
    Data       map[string]string // credential fields; bindings index into it by field name
    CacheUntil time.Time      // when this credential goes stale (drives the cache)
}
```

### Cache (host-supplied, optional)

```go
type Cache interface {
    Get(key string) (*CacheEntry, bool)
    Set(key string, entry *CacheEntry)
    MarkStale(key string)
}
```

`CacheEntry` stores only the extracted credential fields plus `CacheUntil`/`Stale`. The engine derives a collision-free key from `(tool, account)`. Supply your own cache or pass `nil` to use the built-in in-memory implementation.

### Tool discovery

`ListTools` returns credential-safe manifests for every embedded tool in deterministic name order. Discovery also validates that service definitions have registered implementations and CLI definitions declare a binary.

```go
tools, err := anycli.ListTools()
```

### Tools

`Tool` is `type Tool string`; pass a raw `anycli.Tool("…")` whose name matches
an embedded definition. The built-in service tools are `slack`, `notion`,
`gmail`, `discord`, `linkedin`, `x`, `figma`, and `mongodb`; `github` wraps
the `gh` binary. An unknown tool is an error from `Execute`, not a compile
error.

The Figma service uses personal access tokens and exposes every PAT-compatible
operation in the pinned official OpenAPI catalog: 47 operations across files,
projects, comments/reactions, libraries, variables, Dev Mode resources,
webhooks, library analytics, payments, and oEmbed. Every operation has a named
command and is also callable by operation ID through `figma api call`.

Agent-oriented commands accept Figma URLs directly: `figma context metadata`,
`context design`, `context figjam`, `context screenshot`, and `context
variables`. `figma assets download` and `download-fills` materialize signed
render/image-fill URLs without forwarding the PAT to the asset host. Run
`figma capabilities` for the machine-readable boundary: PAT REST has no general
native-canvas create/edit/delete API, so hosted-MCP operations such as
`use_figma`, new-file/design generation, uploads, shaders, and native Code
Connect mappings require Figma's hosted MCP or a Figma plugin bridge.

The `gmail` service projects the Gmail API v1 `users.*` resource namespaces
(`profile`, `messages`, `messages attachments`, `threads`, `drafts`, `labels`)
plus the synthetic `reply` / `forward` verbs, which assemble the threading
headers (`In-Reply-To` / `References` / `threadId`) and the quoted original
inside the tool. `--query` passes native Gmail search syntax through verbatim;
`messages modify` batches multiple ids via `batchModify`; send/reply support
`--body`/`--body-file`, `--html`, and repeated `--attach` (multipart MIME up
to 25MB). Every command supports `--json`; list commands page with `--max` and
`--page-token`. It replaces the retired `google` tool (per-app split, Helio
design 303).

The `x` service supports OAuth 2.0 user-context identity and user lookup,
recent post search, timelines, post/reply/thread/repost management, simple
JPEG/PNG/WebP uploads for posts or DMs, alt text, and legacy Direct Messages.
Every list/search command retrieves one explicit page; callers pass the
returned token with `--next-token`. XChat and chunked video/GIF uploads are not
part of this surface.

The `mongodb` service is a thin wrapper around the official MongoDB Shell
(wraps mongosh 2.9.2) connecting with a standard connection string
(`mongodb://` or `mongodb+srv://`) resolved into `MONGODB_CONNECTION_STRING`.
It exposes exactly two commands: `eval '<mongosh JS>'` and `ping`. Database
selection happens in the script (`db.getSiblingDB(...)`); `db` is
pre-connected via a `connect(process.env.MONGODB_CONNECTION_STRING)` prelude,
so the DSN never appears on the command line. mongosh flags are fixed
(`--nodb --quiet --norc --json=relaxed`) and not passed through — the script
travels as a fused `--eval=` token, so `--shell` and other flags are
unreachable. Output is relaxed extended JSON. The first invocation lazily
installs mongosh from downloads.mongodb.com (sha256-verified, file-locked)
into the pinned-versions directory unless a mongosh is already on PATH.
"authentication failed" stderr rejects the credential; permission errors
("not authorized") and transport failures do not. Output redacts the
connection string and its password.

## Documentation

- [Why AnyCLI](WHY_ANY_CLI.md) — CLI vs MCP for agents
- [Credential Lifecycle](docs/credential-lifecycle.md) — how credentials are resolved, cached, and injected at runtime
- [Tool Definition Schema](docs/definition-schema.md) — field reference for the embedded tool definitions
- [Design 001: Vault Credential Integration](docs/design/001-vault-credential-integration.md) — superseded historical vault/local design
- [Design 002: Embeddable Core + Credential Resolver](docs/design/002-embeddable-core-and-credential-resolver.md) — the library architecture
- [Design 003: Multi-account + Helio toolset](docs/design/003-multi-account-and-helio-toolset.md) — account-aware execution and built-in services
- [Design 005: Tool Manifest Discovery](docs/design/005-tool-manifest-discovery.md) — deterministic discovery and definition/executor validation
- [Design 006: Figma REST Service](docs/design/006-figma-rest-service.md) — PAT-authenticated Figma command surface

## License

Apache License 2.0
