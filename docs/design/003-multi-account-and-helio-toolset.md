# Multi-Account Credential Resolution + the Helio Toolset

**Date:** 2026-06-12
**Status:** Accepted
**Scope:** Make the credential pipeline account-aware (`Resolve` / cache / `Execute` all keyed by `(tool, account)`); add the first real embedded tool definitions and built-in services for the Helio round (design 227 on the Helio side). The 002 embeddable-core positioning is unchanged; this design extends its seams, it does not move them.

## 1. Background

### Current state

Design 002 made AnyCLI an embeddable library: the host supplies a `CredentialResolver` and (optionally) a `Cache`, and calls `Engine.Execute(ctx, tool, args, resolver)`. Everything — the resolver call, the cache key, the stale marking — is keyed by the tool name alone.

The embedded definition set is empty: `definitions/tools/` ships only a README placeholder, and `internal/tools/` has the service registry but zero registered services.

### Problem

Helio's host (`heliox`) resolves credentials per **connected account**, not just per tool: one assistant may have several Slack workspaces or Google accounts connected at once (Helio design 227, multi-account connections). With a tool-keyed pipeline:

- the resolver cannot be told *which* account's credential to return;
- two accounts of the same tool would collide on one cache entry;
- a failure on one account would stale-mark the other's credential.

Separately, the Helio round needs actual tools: five providers with no agent-friendly native CLI (Slack, Notion, Google, Discord, LinkedIn) as built-in services, plus GitHub as a classic cli-type definition over `gh`.

### Goals

1. Thread an **account selector** through the whole pipeline: resolver → cache → engine.
2. Keep the public entry point stable for single-account callers; add an options-carrying variant for multi-account callers.
3. Ship the first embedded definitions + built-in services: `slack`, `notion`, `google`, `discord`, `linkedin` (service type) and `github` (cli type).

Non-goals: per-account credential storage inside AnyCLI (the host's resolver owns account semantics); MS365/msgraph (gated to a later round); interactive auth flows (the host owns OAuth).

## 2. Decisions

### D1 — `Resolve` gains an `account` parameter (clean signature change)

```go
type CredentialResolver interface {
    // Resolve returns the credential fields for one (tool, account), plus
    // when they go stale. account "" selects the resolver's default
    // (primary) account.
    Resolve(ctx context.Context, tool Tool, account string) (*Credential, error)
}
```

AnyCLI is pre-1.0 and its sole consumer is `heliox`, so the signature changes in place — no compat shim, no parallel interface. The empty string is the default-account selector; the resolver decides what "default" means (Helio: the primary connection).

### D2 — cache key = `tool + "\x00" + account`

The `Cache` interface keeps its string key; the **engine** (not the cache implementation) derives the key:

```go
// CacheKey derives the cache map key for one (tool, account). NUL cannot
// appear in either part, so the join is collision-free.
func CacheKey(tool, account string) string { return tool + "\x00" + account }
```

Properties: distinct accounts never share an entry; the default account (`""`) keys as `tool + "\x00"`, distinct from any named account; stale-marking on exec failure hits exactly the `(tool, account)` that failed. Cache implementations stay dumb key-value stores, exactly as in 002.

### D3 — public API: `ExecuteWith` + `ExecOptions`; `Execute` delegates

```go
// ExecOptions carries per-invocation execution options.
type ExecOptions struct {
    // Account selects which connected account's credential to resolve when
    // the host has several for one tool. Empty = the resolver's default.
    Account string
}

func (e *Engine) ExecuteWith(ctx context.Context, tool Tool, args []string, resolver CredentialResolver, opts ExecOptions) (int, error)

// Execute runs with the default account: the short form of ExecuteWith.
func (e *Engine) Execute(ctx context.Context, tool Tool, args []string, resolver CredentialResolver) (int, error)
```

An options struct (rather than a variadic or a second positional string) keeps the entry point extensible — future per-invocation knobs (timeouts, output mode) slot into `ExecOptions` without another signature change.

### D4 — this round's toolset

Six definitions ship embedded under `definitions/tools/`:

| Tool | Type | Injected env | Notes |
|------|------|--------------|-------|
| `slack` | service | `SLACK_BOT_TOKEN` | REST over `api.slack.com/api`; every call checks the HTTP-200-with-`ok:false` dialect |
| `notion` | service | `NOTION_TOKEN` | `Notion-Version: 2022-06-28` pinned |
| `google` | service | `GOOGLE_ACCESS_TOKEN` | Gmail / Calendar / Drive minimal verbs; 401/403 append a missing-scope hint |
| `discord` | service | `DISCORD_BOT_TOKEN` | `Authorization: Bot` (not Bearer) |
| `linkedin` | service | `LINKEDIN_ACCESS_TOKEN`, `LINKEDIN_PERSON_URN` | two bindings; posting requires the person URN |
| `github` | cli | `GH_TOKEN` | wraps `gh` (github-release source, pinned version) |

`msgraph` (MS365) is explicitly gated out of this round.

## 3. Built-in service conventions

Service-type tools follow one template (established by `slack`, mirrored by the rest):

- A package under `internal/tools/<name>/` exporting a `Service` struct that satisfies `tools.Service` by duck typing — service packages never import the `tools` registry, so registration cannot cycle.
- Registration is centralized in `internal/tools/register.go` (`init()` calls `RegisterService` for each built-in); `internal/exec` already imports `internal/tools`, so the embedded registrations are always live.
- Each `Service` carries injectable base URL field(s) and an `HC *http.Client`, zero values defaulting to production endpoints — httptest servers plug in for tests.
- Subcommand/flag parsing is a cobra tree built per `Execute` call. No interactive prompts; flags and env only.
- Credentials arrive only through the resolved `env` map (e.g. `env["SLACK_BOT_TOKEN"]`); a missing credential is exit 1 with an explicit message.
- Success writes the provider's JSON response to stdout verbatim; failure writes a one-line error (including the provider's error code/message) to stderr and returns exit 1. Non-zero exit triggers the engine's stale-marking for exactly that `(tool, account)`.

## 4. What changes / what does not

### Changes

- `internal/credential/resolver.go` — `Resolve(ctx, tool, account)`.
- `internal/credential/cache.go` — exported `CacheKey(tool, account)`; `Cache` docs say "keyed by CacheKey", interface shape unchanged.
- `internal/credential/resolve.go` — `ResolveBindings(ctx, cache, tool, account, bindings, resolver)`.
- `internal/exec/exec.go` — internal `Engine.Execute(ctx, tool, args, resolver, account)`; stale-marking by `CacheKey`.
- `anycli.go` — `ExecOptions` + `ExecuteWith`; `Execute` delegates with `ExecOptions{}`.
- `definitions/tools/*.json` + `internal/tools/{slack,notion,google,discord,linkedin}/` + `internal/tools/register.go` — the toolset.

### Unchanged

- The `Cache` interface shape and consumer-supplied semantics (002).
- The definition schema (001) and the rule that definitions are internal, never consumer-supplied.
- The injection modes (env / arg / file) and middleware engine.

## References

- `docs/design/001-vault-credential-integration.md` — credential schema + injection modes.
- `docs/design/002-embeddable-core-and-credential-resolver.md` — embeddable core + resolver/cache seams.
- Helio `docs/design/227-ai-teammate-oauth-integration/` — the consumer: multi-account connections, token gateway, `heliox tool` portal.
