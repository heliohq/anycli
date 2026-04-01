# Credential Lifecycle

How AnyCLI resolves, caches, and injects credentials at runtime.

## Two Operating Modes

AnyCLI detects the mode automatically based on environment variables.

### Standalone Mode

The user manages credentials locally via `any auth`.

```
any auth gh --set token=ghp_xxx
  → writes ~/.anycli/credentials/gh.json
  → {"GH_TOKEN": "ghp_xxx"}
```

Credentials persist on disk until the user overwrites or uninstalls the tool.

### Vault Mode

Credentials are managed by the Rollout platform. AnyCLI fetches them at runtime.

Activated when **all three** environment variables are set:

| Variable | Purpose |
|----------|---------|
| `ANYCLI_VAULT_URL` | Vault service URL |
| `ANYCLI_VAULT_TOKEN` | Clerk API Key for authentication |
| `ANYCLI_WORKSPACE_ID` | Workspace context for credential resolution |

If only some are set, AnyCLI aborts with an error (no silent fallback).

## Execution Flow

Every time a tool runs (e.g., `gh pr list`), AnyCLI executes the full pipeline from scratch. There is no daemon or persistent state in memory — each invocation is an independent process (busybox model).

### Standalone Mode

```
gh pr list
  │
  ├── 1. Load definition (~/.anycli/registry/gh.json)
  │
  ├── 2. Read local credential file (~/.anycli/credentials/gh.json)
  │      Extract value by local_key (e.g., "GH_TOKEN" → "ghp_xxx")
  │
  ├── 3. Inject credential
  │      env:  set GH_TOKEN=ghp_xxx on child process environment
  │      arg:  append --token ghp_xxx to command args
  │      file: patch target file at inject.path
  │
  ├── 4. Execute real gh binary with injected credential
  │
  └── 5. Return exit code
```

Cost: one file read (~/.anycli/credentials/gh.json) + one env var set. Microseconds.

### Vault Mode

```
gh pr list
  │
  ├── 1. Load definition
  │
  ├── 2. Check cache (~/.anycli/cache/<workspace_id>/<token_hash>/github.json)
  │      ├── Cache hit (cache_until > now, not stale) → use cached fields
  │      └── Cache miss or expired → continue to step 3
  │
  ├── 3. Fetch from vault (HTTP GET, 5s timeout)
  │      GET <vault_url>/vault/credentials/effective?workspace_id=xxx&tool=github
  │      ├── Success → extract only bound vault_field values
  │      │             write to cache with cache_until from response
  │      ├── Transient error (5xx, timeout) → use stale cache if available
  │      └── Auth error (4xx) → fail immediately
  │
  ├── 4. Inject credential (same as standalone)
  │      env:  set GH_TOKEN=ghp_xxx on child process
  │      arg:  append flag to args
  │      file: write temp file → set config_env to redirect tool → cleanup after execution
  │
  ├── 5. Execute real gh binary
  │
  ├── 6. On non-zero exit → mark cache stale, print hint to stderr
  │      "[anycli] credentials for "gh" may be stale. retry the same command..."
  │
  └── 7. Return exit code
```

Cost on cache hit: one file read + one env var set. Same as standalone.  
Cost on cache miss: one HTTP request (5s timeout) + one file write + one env var set.

## Credential Injection Types

Three ways to deliver a credential to the wrapped tool. Each happens fresh on every invocation.

### `env` — Environment Variable

```
AnyCLI sets: GH_TOKEN=ghp_xxx in child process environment
Tool reads:  os.Getenv("GH_TOKEN")
After:       process exits, env var gone
```

Nothing persists. Most common method. Works for most CLIs.

### `arg` — Command-Line Argument

```
User runs:     gh pr list
AnyCLI runs:   gh pr list --token ghp_xxx
After:         process exits, args gone
```

Nothing persists. The credential appears in the process argument list (visible in `ps`) but only for the duration of the command.

### `file` — Config File Patch

Behavior differs by mode:

**Standalone:**
```
AnyCLI patches: ~/.config/some-tool/credentials.yaml (in place)
Tool reads:     ~/.config/some-tool/credentials.yaml
After:          file stays on disk (user explicitly provided the credential)
```

**Vault mode:**
```
AnyCLI copies:  ~/.config/some-tool/credentials.yaml → ~/.anycli/tmp/gh-xxx/credentials.yaml-123
AnyCLI patches: the temp copy
AnyCLI sets:    TOOL_CONFIG=/path/to/temp/copy (via config_env)
Tool reads:     $TOOL_CONFIG
After:          temp file deleted by cleanup (defer)
```

In vault mode, the original config file is never modified. The tool is redirected to a temporary copy via `config_env` or `config_flag`. The temp copy is deleted after execution regardless of success or failure.

If the tool has no mechanism to redirect its config path (no `config_env` or `config_flag`), file injection is not supported in vault mode. Use `env` or `arg` instead.

## Cache Behavior

Cache only exists in vault mode. Standalone mode reads credentials directly from local files.

### Cache Location

```
~/.anycli/cache/<workspace_id>/<token_hash>/<vault_tool>.json
```

- `workspace_id`: isolates credentials across workspaces
- `token_hash`: first 8 chars of SHA-256 of vault token, isolates across sessions/users
- `vault_tool`: one file per tool (e.g., `github.json`, `aws.json`)

### Cache Entry

```json
{
  "fetched_at": "2026-04-01T01:16:52Z",
  "cache_until": "2026-04-01T01:26:52Z",
  "stale": false,
  "fields": {
    "access_token": "ghp_xxx"
  }
}
```

- `cache_until`: TTL from vault. OAuth: token expiry minus 60s. Others: 10 minutes.
- `stale`: set to true when a command fails with non-zero exit. Triggers refetch on next call but preserved as fallback if vault is unreachable.
- `fields`: only the `vault_field` values referenced by credential bindings. Sensitive fields like `refresh_token` that no binding references are never cached.

### Cache Decision Tree

```
Is cache file present?
  ├── No → fetch from vault
  └── Yes
       ├── stale == true → fetch from vault (use stale as fallback on error)
       ├── cache_until < now → fetch from vault (use stale as fallback on error)
       └── cache_until > now → use cached fields ✓
```

### TTL by Credential Type

| Vault type | `cache_until` | Rationale |
|------------|--------------|-----------|
| `oauth` | token expiry - 60s | Vault auto-refreshes on fetch; 60s buffer on top of vault's 30s buffer |
| `token` | now + 10min | Allows rotation/revocation to propagate within a reasonable window |
| `keypair` | now + 10min | Same as token |
| `multi_field` | now + 10min | Same as token |

### Stale Marking

When a CLI command exits with non-zero code:

1. Cache is marked `stale: true` (not deleted)
2. Stderr hint printed: `[anycli] credentials for "gh" may be stale. retry...`
3. Next invocation attempts a vault refetch
4. If vault is unreachable, stale cache is used as fallback (better than no credential)

This is deliberately conservative — not all failures are auth-related, but the cost of one extra vault fetch is negligible.

## Persistence Summary

| What | Standalone | Vault Mode |
|------|-----------|------------|
| Tool definitions (`~/.anycli/registry/`) | Persists | Persists |
| Local credentials (`~/.anycli/credentials/`) | Persists | Not used |
| Vault cache (`~/.anycli/cache/`) | Not used | Persists (with TTL) |
| Injected env vars | Process lifetime | Process lifetime |
| Injected args | Process lifetime | Process lifetime |
| Injected temp files | N/A (writes to real path) | Deleted after execution |

In a Rollout pod, `~/.anycli` lives under `/root` which is backed by a persistent volume (JuiceFS). This means cache and tool definitions survive pod restarts, reducing cold-start latency. Credentials are never stored in local files in vault mode — they exist only in the cache (with TTL) and in memory during execution.
