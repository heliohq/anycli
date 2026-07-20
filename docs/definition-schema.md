# Tool Definition Schema Reference

AnyCLI embeds its tool definitions at build time from `definitions/tools/*.json`.
Hosts select a tool by name and supply credentials through the public
`CredentialResolver`; they do not load or override definitions.

## Execution model

A definition describes one of two execution kinds:

- `cli`: run an external binary already provisioned by the host environment;
- `service`: run an in-process API client registered inside AnyCLI.

`anycli.ListTools` exposes a credential-safe public manifest derived from these
definitions. It never exposes injection details or credential values.

## CLI example

```json
{
  "name": "github",
  "description": "GitHub CLI",
  "binary": "gh",
  "resolve": "which",
  "source": {
    "type": "direct",
    "url_template": "https://github.com/cli/cli/releases/download/v{version}/gh_{version}_{os}_{arch}{ext}",
    "binary_path": "gh_{version}_{os}_{arch}/bin/gh",
    "binary_path_map": {"windows": "bin/gh{exe}"},
    "version": "2.96.0",
    "os_map": {"darwin": "macOS", "linux": "linux", "windows": "windows"},
    "arch_map": {"amd64": "amd64", "arm64": "arm64"},
    "ext_map": {"darwin": ".zip", "linux": ".tar.gz", "windows": ".zip"},
    "sha256": {"macOS-arm64": "<64-hex digest>", "...": "..."}
  },
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "GH_TOKEN"}
      }
    ]
  }
}
```

For `github-release` and `npm` sources, `source` is declarative provisioning
metadata: the engine does not download those binaries; the host image or
installation workflow provisions them. A `direct` source additionally enables
lazy install — the engine resolves the binary through the pinned-versions
directory, then PATH, then downloads the pinned archive from the official
`url_template` URL, verifies the mandatory per-platform `sha256`, and unpacks
the single `binary_path` entry (see `internal/exec/binresolve`). When an
upstream ships differently-shaped archives per platform (gh's windows zip has
`bin/gh.exe` at the root, no versioned top dir), `binary_path_map` overrides
`binary_path` for specific Go OS names.

## Service example

```json
{
  "name": "figma",
  "type": "service",
  "description": "Complete PAT-accessible Figma REST API with design context and asset downloads",
  "auth": {
    "credentials": [
      {
        "source": {"field": "access_token"},
        "inject": {"type": "env", "env_var": "FIGMA_ACCESS_TOKEN"}
      }
    ]
  }
}
```

A service definition must have a matching implementation registered inside
AnyCLI. `ListTools` and `New` fail if that definition-to-executor contract is
incomplete.

## Top-level fields

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `name` | string | yes | Stable embedded tool name and execution selector. |
| `type` | string | no | `cli` (or empty) for an external binary; `service` for an in-process client. |
| `description` | string | yes | Human-readable capability description. |
| `binary` | string | for `cli` | Executable name. |
| `resolve` | string | no | Empty or `which` searches `PATH`; an absolute path selects that binary directly. |
| `source` | object | no | Declarative binary-source metadata retained for host tooling. |
| `auth` | object | no | Credential bindings supplied by the host resolver. |
| `before` | array | no | Middleware applied before an external CLI runs. |
| `after` | array | no | Middleware applied after an external CLI runs. |

## Source metadata

| Field | Type | Meaning |
| --- | --- | --- |
| `type` | string | `github-release`, `npm`, or `direct` (lazy install). |
| `repo` | string | Release repository or package name (`github-release` / `npm`). |
| `asset_pattern` | string | Release asset template (`github-release`). |
| `binary_path` | string | Executable path inside the artifact; `{version}`/`{os}`/`{arch}`/`{exe}` expand. |
| `binary_path_map` | object | Per-Go-OS override of `binary_path` for platforms whose archive layout differs. |
| `version` | string | Pinned version (required for `direct`; optional otherwise). |
| `os_map` | object | Go OS name to upstream release name. |
| `arch_map` | object | Go arch name to upstream release name (e.g. `amd64` -> `x64`). |
| `ext_map` | object | Go OS name to archive extension (`.tgz` / `.zip`). |
| `url_template` | string | `direct` only: full official download URL template. |
| `sha256` | object | `direct` only: `<os>-<arch>` platform key to hex digest; mandatory per platform, mismatch aborts the install. |

## Credential bindings

Each `auth.credentials` item maps one resolver field to one injection method:

```json
{
  "source": {"field": "access_token"},
  "inject": {"type": "env", "env_var": "SERVICE_ACCESS_TOKEN"}
}
```

`field` is the key AnyCLI reads from `Credential.Data`; the host decides whether
that value came from a vault, an OAuth refresh, a token gateway, or another
source. Only declared scalar fields are extracted and cached.

An absent field produces an empty binding and is not injected. Authentication
requirements should therefore be enforced by the external CLI or built-in
service with an explicit error; AnyCLI never falls back to a local credential
store.

### Environment injection

```json
{"type": "env", "env_var": "SERVICE_ACCESS_TOKEN"}
```

The value is added only to the child CLI environment or the in-process
service's invocation map. It is not written to the host process environment.

### Argument injection

```json
{"type": "arg", "flag": "--api-key"}
{"type": "arg", "flag": "--api-key", "format": "eq"}
```

The default form appends `--api-key VALUE`; `format: "eq"` appends
`--api-key=VALUE`. Prefer environment or file injection because arguments may
be visible in process listings.

### Ephemeral file injection

```json
{
  "type": "file",
  "path": "~/.config/example/credentials.json",
  "config_env": "EXAMPLE_CONFIG",
  "file_format": "json",
  "fields": {"default.api_key": "{{.Value}}"},
  "mode": "0600"
}
```

| Field | Required | Meaning |
| --- | --- | --- |
| `path` | yes | Existing config to copy as a template, or the logical target when none exists. |
| `config_env` or `config_flag` | yes | Redirects the tool to the ephemeral copy. |
| `file_format` | no | `json` (default), `yaml`, `toml`, `ini`, or `custom`. |
| `fields` | yes | Dot-path to template map; `{{.Value}}` is replaced by the resolved value. |
| `mode` | no | Octal permissions, default `0600`. |
| `patcher` | for `custom` | Registered custom patcher name. |

AnyCLI copies the original config when present, patches an invocation-unique
temporary file, redirects the tool to it, and removes it after execution. It
never patches a user's persistent config file.

## Middleware rules

Rules contain `name`, `rule`, optional `when`, and optional `config` fields.

Supported conditions:

- `has_flag`
- `not_has_flag`
- `exit_code_is`
- `exit_code_not`
- `output_contains`

Supported before rules:

- `set_env`
- `append_flag`
- `prepend_args`

Supported after rules:

- `map_exit_code`
- `ensure_json`

Middleware applies to external CLI definitions. Built-in services own their
command parsing and JSON output contract directly.

## Extension checklist

To add a built-in provider:

1. Add `definitions/tools/<name>.json` with the resolver fields it requires.
2. Add `internal/tools/<name>/` implementing the service interface.
3. Register the service in `internal/tools/register.go`.
4. Add provider contract, error-classification, and credential-redaction tests.
5. Confirm `ListTools` discovers the provider and `go test ./...` passes.

OAuth, PAT creation, token refresh, persistence, account selection, and user
consent remain host responsibilities. AnyCLI receives only the credential
projection needed for a single tool/account execution boundary.
