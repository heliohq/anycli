# AnyCLI

**One CLI to authenticate them all.**

AnyCLI wraps authenticated cloud service CLIs/APIs (GitHub, Cloudflare, AWS, GCP, etc.) into agent-friendly interfaces with automatic credential injection and middleware hooks.

## Why?

LLMs are trained on billions of CLI examples. They already know `git`, `curl`, `jq`, `docker`, and thousands of other tools. MCP forces agents to learn new schemas from scratch, consuming 10-32x more tokens and costing 17x more per operation — with lower reliability.

CLI is the natural interface between agents and the world. [Read the full rationale.](./WHY_ANY_CLI.md)

## Principles

- **`--help` is the schema** — Agents read help text, not protocol definitions
- **Structured output** — `--json` by default for machine-readable results
- **No interaction** — All input via flags; agents can't type into prompts
- **Composable** — Pipes and stdin/stdout; every tool is a building block
- **Predictable** — Clear exit codes and error messages

## Getting Started

```bash
curl -fsSL https://anycli.dev/install.sh | sh
any install gh
any auth gh --set token=ghp_xxx
gh pr list
```

## Commands

```
any install <tool>       Install a CLI wrapper (downloads binary + creates shim)
any install <tool> --conflict-policy link   Wrap existing binary without downloading
any uninstall <tool>     Remove a wrapper
any list                 List available and installed wrappers
any exec <tool> [args]   Run a tool through the middleware pipeline
any auth <tool>          Configure authentication
any update               Update any to the latest version
```

## Documentation

- [Credential Lifecycle](docs/credential-lifecycle.md) — how credentials are resolved, cached, and injected at runtime
- [Tool Definition Schema](docs/definition-schema.md) — field reference for writing tool definitions
- [Design: Vault Credential Integration](docs/design/001-vault-credential-integration.md) — architecture for vault-backed credential injection

## License

Apache License 2.0
