# Tool Definition Schema Reference

AnyCLI uses JSON definition files to describe how to install, authenticate, and execute CLI tools. This document is a complete reference for all fields.

## Quick Example

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
    "os_map": {"darwin": "macOS"},
    "ext_map": {"darwin": ".zip"}
  },
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
  },
  "before": [],
  "after": []
}
```

---

## Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Tool identifier. Used as the shim name, registry filename, and `any exec <name>`. |
| `type` | string | no | `"cli"` (default) for external CLI wrappers, `"service"` for built-in API clients. |
| `description` | string | yes | Human-readable description shown in `any list`. |
| `binary` | string | for cli | Binary name to execute. Not needed for `type: "service"`. |
| `resolve` | string | no | How to find the real binary. Empty or `"which"` = search PATH (skipping the shim dir). An absolute path = use that directly. Set by `any install` after download. |
| `source` | object | no | How to download the binary. See [Source](#source). |
| `auth` | object | no | Authentication configuration. See [Auth](#auth). |
| `before` | array | no | Pre-execution middleware rules. See [Rules](#rules). |
| `after` | array | no | Post-execution middleware rules. See [Rules](#rules). |

---

## Source

Defines how `any install` downloads the tool binary. Omit entirely if the tool is built-in (`type: "service"`) or installed via `--conflict-policy link`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"github-release"` or `"npm"`. |
| `repo` | string | yes | Repository identifier. GitHub: `"owner/repo"` (e.g., `"cli/cli"`). npm: package name (e.g., `"wrangler"`). |
| `asset_pattern` | string | for github | Download asset filename template. Placeholders: `{version}`, `{os}`, `{arch}`, `{ext}`. |
| `binary_path` | string | yes | Path to the binary inside the archive (GitHub) or `node_modules/.bin/` (npm). Supports the same placeholders as `asset_pattern`. |
| `version` | string | no | Pin to a specific version. Empty = latest release. |
| `os_map` | object | no | Maps Go `GOOS` values to release naming. Example: `{"darwin": "macOS", "linux": "linux"}`. |
| `ext_map` | object | no | Maps Go `GOOS` values to file extensions. Default is `.tar.gz`. Example: `{"darwin": ".zip"}`. |

### Example: GitHub Release

```json
{
  "type": "github-release",
  "repo": "cli/cli",
  "asset_pattern": "gh_{version}_{os}_{arch}{ext}",
  "binary_path": "gh_{version}_{os}_{arch}/bin/gh",
  "os_map": {"darwin": "macOS", "linux": "linux", "windows": "windows"},
  "ext_map": {"darwin": ".zip", "linux": ".tar.gz", "windows": ".zip"}
}
```

### Example: npm

```json
{
  "type": "npm",
  "repo": "wrangler",
  "binary_path": "wrangler"
}
```

---

## Auth

Defines how credentials are resolved and injected. Contains a single field `credentials` — an array of credential bindings.

```json
{
  "auth": {
    "credentials": [...]
  }
}
```

Omit `auth` entirely if the tool needs no authentication.

### Credential Binding

Each binding pairs a **source** (where to find the credential) with an **inject** (how to deliver it to the tool).

```json
{
  "source": { ... },
  "inject": { ... }
}
```

Tools that need multiple credential fields (e.g., AWS access key + secret key) declare multiple bindings. Bindings sharing the same `vault_tool` are fetched in a single vault call.

---

### Source

Specifies where to find the credential value.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vault_tool` | string | no | Tool name in the vault service (e.g., `"github"`, `"cloudflare"`, `"aws"`). Used in vault mode to fetch from the effective endpoint. |
| `vault_field` | string | no | Field path in the vault credential `data` JSON (e.g., `"access_token"`, `"secret_access_key"`). |
| `local_key` | string | yes | Key in the local credential file (`~/.anycli/credentials/<tool>.json`). Also used as the storage key when `any auth --set` writes credentials. |
| `auth_flag` | string | no | The key name used with `any auth <tool> --set <key>=<value>`. Example: `"token"` means the user runs `any auth gh --set token=ghp_xxx`. If omitted, derived from `local_key` by lowercasing and replacing `_` with `-`. |

**Resolution order at runtime:**
1. Vault mode active → fetch from vault using `vault_tool` + `vault_field`
2. Standalone mode → read from local credential file using `local_key`
3. Neither found → skip silently

### Inject

Specifies how to deliver the credential value to the tool. Three types are supported.

#### Type: `env`

Injects the credential as an environment variable on the child process.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"env"` |
| `env_var` | string | yes | Environment variable name (e.g., `"GH_TOKEN"`, `"AWS_ACCESS_KEY_ID"`). |

```json
{"type": "env", "env_var": "GH_TOKEN"}
```

This is the **preferred injection method**. Most CLIs support env var authentication.

#### Type: `arg`

Appends a flag with the credential value to the command arguments.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"arg"` |
| `flag` | string | yes | Flag name (e.g., `"--api-key"`, `"--token"`). |
| `format` | string | no | `""` (default): `--flag value`. `"eq"`: `--flag=value`. |

```json
{"type": "arg", "flag": "--api-key"}
{"type": "arg", "flag": "--api-key", "format": "eq"}
```

#### Type: `file`

Writes credential fields into a config file.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"file"` |
| `path` | string | yes | Target file path (supports `~`). Example: `"~/.config/some-tool/credentials.yaml"`. |
| `file_format` | string | yes | `"yaml"`, `"json"`, `"toml"`, `"ini"`, or `"custom"`. |
| `fields` | object | yes | Map of dot-path to value template. Example: `{"default.api_key": "{{.Value}}"}`. `{{.Value}}` is replaced with the resolved credential. |
| `mode` | string | no | File permission. Default: `"0600"`. |
| `config_env` | string | no | Env var to override the tool's config path (vault mode only). Required for vault mode unless `config_flag` is set. |
| `config_flag` | string | no | CLI flag to override the tool's config path (vault mode only). Alternative to `config_env`. |
| `patcher` | string | no | Name of a registered custom patcher. Only used with `file_format: "custom"`. |

**Vault mode behavior:** Credentials are written to a temporary file under `~/.anycli/tmp/<tool>/`. The tool is redirected to the temp file via `config_env` or `config_flag`. Temp files are cleaned up after execution. If neither `config_env` nor `config_flag` is set, vault mode will reject the definition with an error.

**Standalone mode behavior:** Credentials are written directly to `path`.

```json
{
  "type": "file",
  "path": "~/.config/some-tool/credentials.yaml",
  "config_env": "SOME_TOOL_CONFIG",
  "file_format": "yaml",
  "fields": {"default.api_key": "{{.Value}}"},
  "mode": "0600"
}
```

---

## Rules

Middleware rules run before or after the tool binary executes. Each rule has an optional `when` condition.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Identifier for the rule. |
| `rule` | string | yes | Rule type (see below). |
| `when` | object | no | Condition map. All conditions must pass (AND logic). Omit to always run. |
| `config` | object | no | Rule-specific parameters. |

### Conditions (`when`)

| Key | Value | Description |
|-----|-------|-------------|
| `has_flag` | string | True if the flag exists in args. |
| `not_has_flag` | string | True if the flag is absent. |
| `exit_code_is` | number | True if exit code equals value (after rules only). |
| `exit_code_not` | number | True if exit code differs. |
| `output_contains` | string | True if stdout contains substring (after rules only). |

### Before Rules

| Rule | Config | Description |
|------|--------|-------------|
| `set_env` | `env_var` (string), `value` (string) | Sets an environment variable to a static value. |
| `append_flag` | `flag` (string) | Appends a flag to the argument list. |
| `prepend_args` | `args` (array of strings) | Prepends arguments to the front of the argument list. |

### After Rules

| Rule | Config | Description |
|------|--------|-------------|
| `map_exit_code` | `mapping` (object: string exit code -> new code) | Remaps exit codes. |
| `ensure_json` | (none) | Wraps non-JSON stdout as `{"output": "..."}`. |

### Example

```json
{
  "before": [
    {
      "name": "force-json",
      "rule": "append_flag",
      "when": {"not_has_flag": "--json"},
      "config": {"flag": "--json"}
    }
  ],
  "after": [
    {
      "name": "normalize-exit",
      "rule": "map_exit_code",
      "config": {"mapping": {"2": 1}}
    }
  ]
}
```

---

## Complete Examples

### GitHub CLI (env var injection, GitHub Release)

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

### Wrangler (env var injection, npm)

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

### AWS CLI (multi-field credentials)

```json
{
  "name": "aws",
  "description": "AWS CLI",
  "binary": "aws",
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

### Built-in Service (no external binary)

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
