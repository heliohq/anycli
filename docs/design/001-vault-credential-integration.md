# Vault Credential Integration Design

**Date:** 2026-03-31
**Status:** Draft
**Scope:** AnyCLI credential system redesign + Rollout vault service integration

## 1. Background

### Current State

AnyCLI currently manages credentials through local JSON files at `~/.anycli/credentials/<tool>.json`. The format is a flat `map[string]string` keyed by environment variable name (e.g., `{"GH_TOKEN": "ghp_xxx"}`), stored with `0600` permissions.

The credential loading is hardcoded in `internal/exec/exec.go:loadCredential()`, which reads the local file and injects the value into `ctx.Env`. The `AuthConfig` struct supports only two modes: `"managed"` (AnyCLI stores the token) and `"self"` (tool manages its own auth).

### Problem

When AnyCLI runs inside a Rollout runtime pod, credentials should come from the Rollout vault service rather than local files. The vault provides encrypted storage (AES-256-GCM + AWS KMS), automatic OAuth token refresh, and user/workspace credential merging. The current architecture has no way to plug in an external credential source.

Additionally, the current `AuthConfig` only supports a single environment variable (`env_var` field), which is insufficient for tools that need multiple credential fields (e.g., AWS requires `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY`) or non-env-var injection methods (e.g., writing credential files, passing CLI arguments).

### Goals

1. Enable AnyCLI to fetch credentials from the vault service when running in a Rollout pod
2. Support three distinct credential injection modes: environment variable, CLI argument, and file patching
3. Support built-in API client services for tools that have no native CLI
4. Clean break from the current auth schema — no backward compatibility required (project has not shipped yet)
5. Keep tool definitions declarative — adding a new tool should require only a JSON file (except for Mode 3 built-in services and rare file format patchers)

## 2. Architecture Overview

```
+------------------+     +------------------+
|   Tool Definition |     | Vault Service    |
|   (JSON)          |     | (HTTP API)       |
+--------+---------+     +--------+---------+
         |                         |
         v                         v
+------------------------------------------+
|            exec.Run() Pipeline           |
|                                          |
|  1. Load Definition                      |
|  2. Resolve Binary (or built-in service) |
|  3. Resolve Credentials ◄── NEW STAGE    |
|     vault (if configured) → local → skip |
|  4. Apply Credential Bindings ◄── NEW    |
|     env / arg / file inject              |
|  5. Run Before Hooks                     |
|  6. Execute                              |
|  7. Run After Hooks                      |
|  8. Cleanup (clear cache, clean files)   |
|  9. Return                               |
+------------------------------------------+
```

Credential resolution is extracted from the before-hooks layer into its own dedicated pipeline stage (steps 3-4), replacing the current inline `loadCredential()` call in `exec.go`.

## 3. Three Auth Modes

### Mode 1: Native CLI reads credential files

The native CLI expects credentials at a specific file path (e.g., `~/.config/gh/hosts.yml`). AnyCLI fetches the credential value and writes it to the expected location.

**Preference order:** If the CLI also supports environment variable auth, prefer env injection (Mode 2) and avoid file writes. File injection is only used when no env var alternative exists.

**File patching:** Most credential files use standard formats (YAML, JSON, TOML, INI). AnyCLI provides built-in format handlers that patch specific fields via dot-path notation. For truly exotic formats, a per-tool Go patcher can be registered as an escape hatch.

**Credential isolation in vault mode:** File injection in vault mode requires the tool to support an **alternate config path** — either through an environment variable (e.g., `KUBECONFIG`) or a CLI flag (e.g., `--config`). AnyCLI:
1. Writes credentials to a temporary file under `~/.anycli/tmp/<tool>/` (0600 permissions)
2. Points the tool to the temp file via the env var or flag declared in the inject config
3. Cleans up the temporary file after execution completes (regardless of success or failure, using `defer`)

If the tool has no mechanism to redirect its config path, file injection in vault mode is **not supported** for that tool. The definition author must find an env var or arg alternative, or use a built-in service (Mode 3) instead. AnyCLI will not write to or modify user-owned config files in vault mode — no backup+restore, no in-place patching of fixed paths.

In standalone mode, file injection writes directly to the target path (same as `any auth` would). This is safe because the user explicitly provided the credential via `any auth`.

### Mode 2: Native CLI accepts credentials via args or env vars

The native CLI accepts credentials through environment variables (e.g., `GH_TOKEN`) or command-line flags (e.g., `--api-key=xxx`). AnyCLI injects the credential into the process environment or appends it to the argument list.

This is the most common and preferred mode. The current `gh` definition already uses this approach.

### Mode 3: Built-in API client (no native CLI)

For services without a native CLI (e.g., Notion, Linear), AnyCLI includes a built-in Go HTTP client compiled into the binary. Each service implements a `Service` interface with its own cobra command tree for subcommand/flag parsing.

## 4. Credential Binding Schema

The current flat `AuthConfig` is replaced by a structured credential binding system.

### 4.1 Definition Schema Changes

**Current `auth` schema (to be deprecated):**
```json
{
  "auth": {
    "type": "managed",
    "env_var": "GH_TOKEN",
    "prompt": "Enter your GitHub personal access token"
  }
}
```

**New `auth` schema (with credentials):**
```json
{
  "auth": {
    "credentials": [
      {
        "source": {
          "vault_tool": "github",
          "vault_field": "access_token",
          "local_key": "GH_TOKEN",
          "auth_flag": "token"
        },
        "inject": {
          "type": "env",
          "env_var": "GH_TOKEN"
        }
      }
    ]
  }
}
```

A definition must have `credentials` (or neither, if the tool needs no auth).

### 4.2 Credential Source

Each binding specifies where to find the credential value and how to expose it for `any auth`.

```go
type CredentialSource struct {
    VaultTool  string `json:"vault_tool"`  // Tool name in vault (e.g., "github")
    VaultField string `json:"vault_field"` // Field path in vault data JSON (e.g., "access_token")
    LocalKey   string `json:"local_key"`   // Key in local credential file (e.g., "GH_TOKEN")
    AuthFlag   string `json:"auth_flag"`   // CLI flag name for `any auth` (e.g., "token", "access-key-id")
}
```

The `auth_flag` field determines the key name used with `any auth <tool> --set <key>=<value>`. For example, `"auth_flag": "token"` means the user runs `any auth gh --set token=ghp_xxx`. If omitted, the flag is derived from `local_key` by lowercasing and replacing underscores with hyphens (e.g., `GH_TOKEN` → `gh-token`).

**Resolution order at runtime:**
1. If vault mode is active (see Section 5.4) → fetch from vault using `vault_tool` + `vault_field`
2. Otherwise → read from `~/.anycli/credentials/<tool>.json` using `local_key`
3. If neither source has the credential → skip silently (tool may work without auth or prompt itself)

### 4.3 Credential Injection Types

#### `env` — Environment variable injection

```json
{
  "inject": {
    "type": "env",
    "env_var": "GH_TOKEN"
  }
}
```

Sets the environment variable on the child process. This is the current behavior for managed auth, and the preferred injection method for most tools.

#### `arg` — Command-line argument injection

```json
{
  "inject": {
    "type": "arg",
    "flag": "--api-key"
  }
}
```

Appends `--api-key <value>` to the argument list before execution. For flags that use `=` syntax, specify `"format": "eq"` to produce `--api-key=<value>`.

#### `file` — Credential file injection

```json
{
  "inject": {
    "type": "file",
    "path": "~/.config/some-tool/credentials.yaml",
    "config_env": "SOME_TOOL_CONFIG",
    "format": "yaml",
    "fields": {
      "default.api_key": "{{.Value}}"
    },
    "mode": "0600"
  }
}
```

Patches credential fields into a file. The `format` field selects the built-in format handler (yaml, json, toml, ini). The `fields` map uses dot-path notation to set specific values without overwriting the entire file.

**Standalone mode:** Writes directly to `path`. If the file does not exist, it is created from the `fields` map alone. If the file exists, only the specified fields are modified.

**Vault mode:** If the original file at `path` exists, it is **copied** to a temporary file under `~/.anycli/tmp/<tool>/` first, then the credential fields are patched into the copy. This preserves any non-credential settings the tool requires (e.g., region, output format, protocol preferences). If the original file does not exist, the temp file is created from the `fields` map alone — this limits the temp file to credential-only content, which is sufficient when the tool has no pre-existing config.

The temp file path is then set via the environment variable named in `config_env` (or the flag in `config_flag`) to redirect the tool. The temp file is cleaned up after execution. If neither `config_env` nor `config_flag` is set, the definition is invalid for vault mode and AnyCLI will abort with an error explaining that the tool cannot safely receive file-injected credentials.

For rare cases where the file format is not yaml/json/toml/ini, a custom Go patcher can be registered (see Section 7.2).

### 4.4 Multi-Field Credentials

Tools requiring multiple credential fields declare multiple bindings. All bindings with the same `vault_tool` share a single vault fetch — the result is cached and each binding extracts its own `vault_field`.

```json
{
  "auth": {
    "credentials": [
      {
        "source": {"vault_tool": "aws", "vault_field": "access_key_id", "local_key": "AWS_ACCESS_KEY_ID", "auth_flag": "access-key-id"},
        "inject": {"type": "env", "env_var": "AWS_ACCESS_KEY_ID"}
      },
      {
        "source": {"vault_tool": "aws", "vault_field": "secret_access_key", "local_key": "AWS_SECRET_ACCESS_KEY", "auth_flag": "secret-access-key"},
        "inject": {"type": "env", "env_var": "AWS_SECRET_ACCESS_KEY"}
      }
    ]
  }
}
```

### 4.5 Updated Definition Examples

**gh (Mode 2 — env var injection):**
```json
{
  "name": "gh",
  "description": "GitHub CLI with agent-friendly defaults",
  "binary": "gh",
  "source": {
    "type": "github-release",
    "repo": "cli/cli",
    "asset_pattern": "gh_{version}_{os}_{arch}{ext}",
    "binary_path": "gh_{version}_{os}_{arch}/bin/gh",
    "os_map": {"darwin": "macOS", "linux": "linux", "windows": "windows"},
    "ext_map": {"darwin": ".zip", "linux": ".tar.gz", "windows": ".zip"}
  },
  "auth": {
    "credentials": [
      {
        "source": {"vault_tool": "github", "vault_field": "access_token", "local_key": "GH_TOKEN", "auth_flag": "token"},
        "inject": {"type": "env", "env_var": "GH_TOKEN"}
      }
    ]
  },
  "before": [],
  "after": []
}
```

Note: the current `inject-auth` before rule (`set_env` with `from: "credentials"`) is removed. Credential injection is now a dedicated pipeline stage, not a middleware rule.

**wrangler (Mode 2 — env var injection):**
```json
{
  "name": "wrangler",
  "description": "Cloudflare CLI for Workers and Pages",
  "binary": "wrangler",
  "source": {
    "type": "npm",
    "repo": "wrangler",
    "binary_path": "wrangler"
  },
  "auth": {
    "credentials": [
      {
        "source": {"vault_tool": "cloudflare", "vault_field": "access_token", "local_key": "CLOUDFLARE_API_TOKEN", "auth_flag": "token"},
        "inject": {"type": "env", "env_var": "CLOUDFLARE_API_TOKEN"}
      }
    ]
  },
  "before": [],
  "after": []
}
```

**notion (Mode 3 — built-in service):**
```json
{
  "name": "notion",
  "type": "service",
  "description": "Notion API client",
  "auth": {
    "credentials": [
      {
        "source": {"vault_tool": "notion", "vault_field": "token", "local_key": "NOTION_TOKEN", "auth_flag": "token"},
        "inject": {"type": "env", "env_var": "NOTION_TOKEN"}
      }
    ]
  }
}
```

## 5. Vault Integration

### 5.1 Vault Effective Endpoint

AnyCLI calls the vault service's pod-facing endpoint to fetch credentials:

```
GET /vault/credentials/effective?workspace_id=<id>&tool=<vault_tool>
Authorization: Bearer <cli_token_jwt>
```

**Current response format** (from vault controller `respondEffective`):
```json
{
  "credentials": [
    {
      "id": "...",
      "tool": "github",
      "source": "user",
      "type": "oauth",
      "data": {"access_token": "ghp_xxx", "refresh_token": "..."},
      "metadata": {"account_name": "...", "provider": "...", "env_var": "..."},
      "status": "active"
    }
  ]
}
```

### 5.2 Required Vault-Side Changes

**1. Add `tool` query parameter to effective endpoint**

Filter the response to return only the requested tool's credential. Without this parameter, the current behavior (return all) is preserved.

**2. Add `cache_until` field to effective response**

The current effective response does not include any expiry information. The `ExpiresAt` field exists on the Credential model but is never populated by the vault service. OAuth expiry lives inside the encrypted `data` blob (`OAuthData.Expiry`).

After the vault performs auto-refresh during `GetEffective`, it knows the token's new expiry. It should compute and return a `cache_until` timestamp:

- **OAuth credentials:** `OAuthData.Expiry - 60s` (caller-side buffer on top of vault's 30s refresh buffer)
- **Token/multi_field/keypair credentials:** current time + 10 minutes (bounded TTL to allow rotation and revocation to propagate within a reasonable window)

Updated response format:
```json
{
  "credentials": [
    {
      "id": "...",
      "tool": "github",
      "source": "user",
      "type": "oauth",
      "data": {"access_token": "ghp_xxx"},
      "metadata": {"env_var": "GH_TOKEN"},
      "status": "active",
      "cache_until": "2026-03-31T18:00:00Z"
    }
  ]
}
```

### 5.3 Vault Client in AnyCLI

A lightweight HTTP client that:

1. Reads vault mode env vars (see Section 5.4)
2. Makes a single HTTP GET to `/vault/credentials/effective?workspace_id=<id>&tool=<vault_tool>`
3. Parses the response and extracts only the fields needed for injection (per `vault_field` in each binding), never storing the full vault `data` blob

No external dependencies. Uses Go's `net/http` and `encoding/json` from the standard library.

**Failure contract:**

| Scenario | Behavior |
|----------|----------|
| Vault unreachable / timeout | Use stale cache if available (ignore `cache_until`). If no cache exists, fail with error to stderr and exit code 1. |
| HTTP timeout | 5 second hard deadline (`http.Client.Timeout`). No retries — the agent will retry the whole command. |
| HTTP 4xx (auth error, bad request) | Fail immediately, no cache fallback. These indicate misconfiguration, not transient failure. |
| HTTP 5xx (server error) | Use stale cache if available. If no cache, fail with error. |
| Empty response (tool not in vault) | Skip silently, same as "no credential found". |

The key principle: **stale cache is better than no execution.** A potentially-expired credential might still work; no credential definitely won't. Stale cache fallback only applies to network/server errors, never to auth errors (4xx) which indicate the vault intentionally rejected the request.

### 5.4 Vault Mode Detection

Vault mode is activated when **all three** of the following environment variables are set:

| Variable | Purpose | Set by |
|----------|---------|--------|
| `ANYCLI_VAULT_URL` | Vault service base URL | Init script / pod env |
| `ANYCLI_VAULT_TOKEN` | CLIToken JWT for auth (required because the pod effective endpoint extracts `user_id` from CLIToken; RuntimeToken does not contain `user_id` and will result in a 401) | Init script / pod env |
| `ANYCLI_WORKSPACE_ID` | Workspace ID for effective resolution | Init script / pod env |

**Partial configuration is an error.** If some but not all three vars are set, AnyCLI aborts with a clear error message listing which vars are missing. This prevents silent fallback to local mode when vault was intended but misconfigured.

When none of the three are set, AnyCLI operates in standalone mode (current local file behavior).

### 5.5 Credential Cache

AnyCLI is busybox-style — each invocation is a separate process with no shared memory. Caching is file-based.

**Cache location:** `~/.anycli/cache/<workspace_id>/<vault_tool>.json`

The cache path includes `workspace_id` because the effective credential depends on workspace context — different workspaces may have different workspace-scoped credentials, and the same user may switch between workspaces. User identity is implicitly scoped by `ANYCLI_VAULT_TOKEN` (the CLIToken contains the user ID), and within a pod the token does not change, so user-level partitioning is not needed.

**Cache format:**
```json
{
  "fetched_at": "2026-03-31T10:00:00Z",
  "cache_until": "2026-03-31T18:00:00Z",
  "fields": {
    "access_token": "ghp_xxx"
  }
}
```

The `fields` map contains **only the extracted `vault_field` values** needed for injection — never the full vault `data` blob. Sensitive material like refresh tokens that are not referenced by any binding's `vault_field` are never written to disk.

**Cache logic on each invocation:**
1. Read `~/.anycli/cache/<workspace_id>/<vault_tool>.json`
2. If file exists AND `cache_until` > now AND not `stale` → use cached `fields`
3. Otherwise → attempt fetch from vault:
   - Success → extract only needed fields, write to cache file (0600 permissions), use fresh data
   - Failure (timeout, 5xx) AND stale cache exists → use stale cached `fields` (better than nothing)
   - Failure AND no cache exists → fail with error

**Cache invalidation:** Cache is **not** deleted on every non-zero exit — that would destroy the stale-cache fallback needed for vault outages (see Section 5.3 failure contract). Instead, cache is **marked stale** by setting `cache_until` to the current time:

```json
{"fetched_at": "...", "cache_until": "2026-03-31T10:05:00Z", "stale": true, "fields": {...}}
```

On the next invocation, the cache logic sees `cache_until < now`, triggers a vault fetch. If the fetch succeeds, the cache is updated with fresh data. If the fetch fails (timeout, 5xx), the stale cache is used as fallback.

The `stale` flag is set when a CLI execution completes with a non-zero raw exit code (before after-hook remapping). This is deliberately conservative — not all failures are auth-related, but marking stale is a low-cost signal that triggers a refetch attempt on the next call without destroying the fallback.

### 5.6 Auth Failure Handling

When a CLI execution fails and the cache is marked stale, AnyCLI writes a hint to stderr:

```
[anycli] credentials for "<tool>" may be stale. retry the same command to fetch fresh credentials.
```

AnyCLI does **not** implement automatic retry. The agent (Claude/Codex) sees this message and naturally retries the command, which triggers a vault refetch attempt (with stale-cache fallback if vault is unavailable).

## 6. `any auth` Command Changes

### 6.1 Standalone Mode (no vault)

Credentials are set via `--set <auth_flag>=<value>` pairs:

```bash
# Single credential:
any auth gh --set token=ghp_xxx

# Multi-field:
any auth aws --set access-key-id=AKIA... --set secret-access-key=wJal...
```

The `auth` command supports `--set`, `--json`, and `--help`. Each `--set` value is matched against the `auth_flag` field in the tool's credential bindings. Unknown keys are rejected with an error listing the valid flags.

With `--json`, all output is JSON — both success and error:
```json
// Success:
{"ok": true, "tool": "gh", "keys": ["GH_TOKEN"]}

// Error (unknown key):
{"ok": false, "error": "unknown auth key \"foo\", valid keys: token", "tool": "gh"}

// Error (vault mode):
{"ok": false, "error": "credentials managed by vault service", "tool": "gh"}
```

When `--json` is set, no prose is written to stdout or stderr. Exit code is 0 on success, 1 on error.

This replaces the current fixed `--token` flag. The Cobra wiring is simple: `auth` registers only `--set` as a string-slice flag, loads the tool definition to validate keys, and writes to `~/.anycli/credentials/<tool>.json`.

The credential file format is identical to today — a `map[string]string` keyed by `local_key` values from the credential bindings:

```json
{
  "GH_TOKEN": "ghp_xxx"
}
```

### 6.2 Vault Mode

When vault mode is active, `any auth` rejects credential writes:

```
$ any auth gh --set token=ghp_xxx
Error: credentials for "gh" are managed by vault service.
Configure credentials via the platform dashboard.

$ any auth gh --set token=ghp_xxx --json
{"ok": false, "error": "credentials managed by vault service", "tool": "gh"}
```

AnyCLI treats vault credentials as read-only. Credential lifecycle (create, update, delete, OAuth flows) is managed by the Rollout platform.

## 7. Built-in Services (Mode 3)

### 7.1 Service Interface

```go
// Service is the interface for built-in API client services.
type Service interface {
    // Execute runs the service with the given arguments and credentials.
    // The env map contains resolved credentials (e.g., {"NOTION_TOKEN": "xxx"}).
    Execute(ctx context.Context, args []string, env map[string]string) (int, error)
}
```

Each service is a Go package under `internal/tools/<name>/` that implements this interface. Internally, each service builds its own cobra command tree for subcommand and flag parsing.

**Note:** Mode 3 services only support `env` inject type in their credential bindings. Since there is no external binary, `arg` injection (appending CLI flags) and `file` injection (writing credential files) are meaningless. The service implementation reads credentials directly from the `env` map parameter.

### 7.2 Service and Patcher Registry

```go
// internal/tools/registry.go
var services = map[string]Service{}
var patchers = map[string]CredentialPatcher{}

func RegisterService(name string, svc Service) {
    services[name] = svc
}

func RegisterPatcher(name string, p CredentialPatcher) {
    patchers[name] = p
}
```

Service and patcher packages are imported in `internal/tools/imports.go` using blank imports to trigger `init()` registration:

```go
// internal/tools/imports.go
package tools

import (
    _ "github.com/shipbase/anycli/internal/tools/notion"
    _ "github.com/shipbase/anycli/internal/tools/linear"
)
```

This file is the single place to add new built-in services. The `main.go` entry point imports `internal/tools` (which transitively imports all service packages).

Custom file patchers (for exotic credential file formats) use the same registration mechanism:

```go
// CredentialPatcher handles non-standard credential file formats.
type CredentialPatcher interface {
    // Patch writes credential values to the tool's config file and returns
    // a cleanup function. The cleanup function is called after execution
    // (in vault mode) to remove any credential data written by Patch.
    // Each call to Patch returns its own independent cleanup handle,
    // so concurrent invocations do not share state.
    //
    // path: target file path (expanded from inject.path)
    // fields: mapping of dot-path → resolved credential value
    // mode: file permission (from inject.mode, default 0600)
    Patch(path string, fields map[string]string, mode os.FileMode) (cleanup func() error, err error)
}
```

When a file inject binding specifies `"format": "custom"` and `"patcher": "<name>"`, the engine delegates to the registered patcher instead of the built-in format handlers. The returned cleanup function is registered alongside the built-in format handler cleanups in the pipeline's deferred cleanup chain.

### 7.3 Exec Pipeline Branching

```go
func Run(name string, args []string) (int, error) {
    def, err := registry.Load(name)
    // ...

    // Resolve credentials (vault → local → skip)
    creds := resolveCredentials(def)

    // Apply credential bindings
    ctx, cleanup := applyBindings(def, creds)
    defer cleanup() // clean up temp files from file injection in vault mode

    if def.Type == "service" {
        // Built-in service: dispatch to registered Service implementation
        svc := tools.GetService(def.Name)
        return svc.Execute(context.Background(), args, ctx.Env)
    }

    // External CLI: resolve binary, run middleware pipeline
    binaryPath, err := resolveBinary(def)
    // ... (existing pipeline: before hooks → execute → after hooks)
}
```

### 7.4 Directory Structure

```
internal/tools/
  registry.go       # Service + Patcher registration
  imports.go        # Blank imports for init() registration
  notion/
    service.go       # Notion API client (implements Service)
  linear/
    service.go       # Linear API client (implements Service)
```

Built-in format handlers for file patching live in the credential package:

```
internal/credential/
  provider.go        # CredentialProvider interface + resolution logic
  vault.go           # Vault HTTP client
  local.go           # Local file reader
  cache.go           # File-based cache
  inject.go          # Binding application (env / arg / file)
  format/
    yaml.go          # YAML field patcher
    json.go          # JSON field patcher
    toml.go          # TOML field patcher
    ini.go           # INI field patcher
```

## 8. Exec Pipeline (Revised)

The full pipeline after this redesign:

```
exec.Run(name, args)
  │
  ├── 1. registry.Load(name)
  │      Load tool definition from ~/.anycli/registry/<name>.json
  │
  ├── 2. resolveCredentials(def)
  │      For each credential binding:
  │        if vault mode active:
  │          check cache → if valid, use cached fields
  │          else → HTTP GET vault effective endpoint (by vault_tool)
  │          extract only vault_field values from response data
  │          write extracted fields to cache (with cache_until)
  │        else:
  │          read ~/.anycli/credentials/<name>.json
  │          extract local_key
  │      Group by vault_tool to avoid duplicate fetches
  │
  ├── 3. applyBindings(def, resolvedCreds) → returns cleanup func
  │      For each credential binding:
  │        env  → ctx.Env[env_var] = value
  │        arg  → ctx.Args = append(ctx.Args, flag, value)
  │        file → standalone: patch target file directly
  │              vault mode: write temp file, set config_env/config_flag, register cleanup
  │
  ├── 4. Branch on def.Type
  │      ├── "service" → tools.GetService(name).Execute(ctx, args, env)
  │      └── "" / "cli" → continue to step 5
  │
  ├── 5. resolveBinary(def)
  │      Find real binary (absolute path or PATH search, skip shim dir)
  │
  ├── 6. middleware.RunBefore(def.Before, ctx)
  │
  ├── 7. Execute binary
  │      ├── No after hooks → passthrough (streaming stdin/stdout/stderr)
  │      └── Has after hooks → buffered capture
  │
  ├── 8. middleware.RunAfter(def.After, ctx) [if buffered]
  │
  ├── 9. On non-zero raw exit code + vault cache exists → mark cache stale, hint to stderr
  │      (uses raw exit code from process, before any exit code remapping by after hooks)
  │
  ├── 10. cleanup() — remove temp credential files (vault mode file inject)
  │
  └── 11. Return final exit code
```

## 9. Init Script Changes

The session init script (`pty-bridge/scripts/claude-init.py`) currently has a TODO for vault credential injection. With this design, the init script's responsibility is simplified to:

1. **Set environment variables** for AnyCLI vault mode:
   ```bash
   export ANYCLI_VAULT_URL="http://vault-service:8096"
   export ANYCLI_VAULT_TOKEN="<cli_token_jwt>"
   export ANYCLI_WORKSPACE_ID="<workspace_id>"
   ```

2. **Ensure AnyCLI is installed** and `~/.anycli/bin` is in `PATH`

3. **Install tool definitions** (run `any install <tool>` for each tool the user has configured)

The init script does **not** pre-fetch credentials. All credential fetching happens lazily at execution time through AnyCLI's vault client.

## 10. Definition Schema

The project has not shipped yet, so there is no backward compatibility requirement. The old `AuthConfig` struct and bundled definitions (`gh.json`, `wrangler.json`) are replaced outright.

### 10.1 Updated Go Structs

```go
type Definition struct {
    Name        string        `json:"name"`
    Type        string        `json:"type,omitempty"` // "" (default, = "cli") or "service"
    Description string        `json:"description"`
    Binary      string        `json:"binary,omitempty"`
    Resolve     string        `json:"resolve,omitempty"`
    Source      *SourceConfig `json:"source,omitempty"`
    Auth        *AuthConfig   `json:"auth,omitempty"`
    Before      []Rule        `json:"before,omitempty"`
    After       []Rule        `json:"after,omitempty"`
}

type AuthConfig struct {
    Credentials []CredentialBinding `json:"credentials,omitempty"`
}

type CredentialBinding struct {
    Source CredentialSource `json:"source"`
    Inject CredentialInject `json:"inject"`
}

type CredentialSource struct {
    VaultTool  string `json:"vault_tool,omitempty"`
    VaultField string `json:"vault_field,omitempty"`
    LocalKey   string `json:"local_key"`
    AuthFlag   string `json:"auth_flag,omitempty"`
}

type CredentialInject struct {
    Type      string            `json:"type"`                // "env", "arg", "file"
    EnvVar    string            `json:"env_var,omitempty"`   // for type "env"
    Flag      string            `json:"flag,omitempty"`      // for type "arg"
    Format    string            `json:"format,omitempty"`    // for type "arg": "" (space-separated) or "eq" (=)
    Path      string            `json:"path,omitempty"`      // for type "file"
    ConfigEnv string            `json:"config_env,omitempty"`// for type "file" in vault mode
    FileFormat string           `json:"file_format,omitempty"` // for type "file": "yaml", "json", "toml", "ini", "custom"
    Fields    map[string]string `json:"fields,omitempty"`    // for type "file": dot-path → template
    Patcher   string            `json:"patcher,omitempty"`   // for type "file" with file_format "custom"
    Mode      string            `json:"mode,omitempty"`      // for type "file": file permission, default "0600"
}
```

The old `AuthConfig` fields (`type`, `env_var`, `prompt`, `command`) are removed entirely. All existing bundled definitions and `cmd/auth.go` are rewritten to match.

### 10.2 Standalone Operation

When vault mode env vars are not set, the entire vault integration is dormant. AnyCLI reads credentials from local files, `any auth` writes local files. No vault client code is invoked.

## 11. Vault-Side Changes Summary

| Change | Endpoint | Description |
|--------|----------|-------------|
| Add `tool` query parameter | `GET /vault/credentials/effective` | Filter by tool name, return single credential |
| Add `cache_until` to response | `GET /vault/credentials/effective` | Computed expiry hint for client-side caching (OAuth: expiry - 60s; token/keypair/multi_field: now + 10min) |

Both changes are additive and backward-compatible. Existing callers that don't pass `tool` or ignore `cache_until` are unaffected.

## 12. New Go Packages

| Package | Purpose |
|---------|---------|
| `internal/credential/` | Credential provider interface, vault client, local reader, cache, binding applicator |
| `internal/credential/format/` | Built-in file format handlers (yaml, json, toml, ini) |
| `internal/tools/` | Service interface, patcher interface, registration, blank imports |
| `internal/tools/<name>/` | Per-service implementations (Mode 3) |

## 13. Testing Strategy

### Unit Tests

- `internal/credential/`: Provider resolution logic, cache behavior (including workspace-scoped paths), vault response parsing, binding application for all three inject types
- `internal/credential/`: Cache stores only extracted fields (not full vault blob), verify refresh tokens are never cached
- `internal/credential/format/`: Each format handler tested with create-new-file and patch-existing-file scenarios
- `internal/credential/`: File inject cleanup in vault mode (temp files removed after execution)
- `internal/tools/`: Service dispatch, patcher dispatch
- `internal/exec/`: Pipeline integration with new credential stage, backward compat with old auth schema
- Vault mode detection: all three vars set (active), none set (standalone), partial set (error)

### Integration Tests

- Full pipeline test with mock vault HTTP server: definition load → vault fetch → cache write → credential injection → binary execution
- Cache invalidation test: first call caches, second call uses cache, simulate failure then verify cache cleared, third call fetches fresh
- File inject isolation test: vault mode writes temp file, verify original config untouched after execution

### Manual Verification

- Standalone mode: `any install gh` → `any auth gh --set token=ghp_xxx` → `gh pr list` works
- Vault mode: set vault env vars + mock vault → `gh pr list` fetches from vault
- Cache expiry: set short `cache_until`, verify refetch after expiry
- Partial vault config: set only `ANYCLI_VAULT_URL`, verify clear error about missing vars
