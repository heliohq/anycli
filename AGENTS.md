# AnyCLI - Agent Guidelines

## Project Overview

AnyCLI is an embeddable Go library (design 002): the engine plus the embedded definitions for the tools it supports. A host (e.g. Helio's `heliox`) embeds it in-process, supplies a `CredentialResolver` (and optionally a `Cache`), and calls `Engine.Execute`. AnyCLI loads the matching embedded tool definition, injects credentials (env / arg / ephemeral file), runs middleware, and execs the underlying binary or built-in service. It is **not** a standalone CLI, and tool definitions are **not** consumer-supplied — they live embedded inside AnyCLI.

## Tech Stack

- Language: Go
- Build: Go modules
- Formatting: `gofmt`

## Language Rule

- **All content must be in English** — code, comments, documentation, commit messages, PR descriptions, and error messages. No exceptions.

## Development Rules

- Write tests first, then implement
- Run tests before marking any task complete
- Follow existing code patterns
- Keep it simple — no over-engineering
- **No interactive prompts** — all input must come from flags or environment variables. AnyCLI is designed for agents, not humans typing into terminals.

## Code Style

- Prefer simple, readable code over clever abstractions
- Use predictable exit codes: 0 for success, non-zero for failure
- Embedded tool definitions should target `--json` output and non-interactive flags so agents can consume results

## Git Conventions

- Commit format: `type(scope): message`
- Types: `feat`, `fix`, `refactor`, `chore`, `ci`, `docs`, `test`
- Prefer small, atomic commits — each commit should be the smallest unit of change that doesn't break integrity (builds pass, tests pass)
- One logical change per commit; split unrelated changes into separate commits
- **Do not commit unless the user explicitly asks** — never auto-commit

## Project Structure

```
anycli/
├── AGENTS.md          # Agent guidelines (this file)
├── CLAUDE.md          # Symlink -> AGENTS.md
├── WHY_ANY_CLI.md     # Rationale: why CLI over MCP
├── README.md          # Embeddable API overview
├── anycli.go          # Public library API: Config, New, Engine.Execute, Cache, CredentialResolver
├── definitions/       # Embedded tool definitions (go:embed) — internal to AnyCLI, not consumer-supplied
├── internal/
│   ├── config/        # Directory helpers (binary PATH resolution)
│   ├── credential/    # Credential resolver seam, binding/injection, cache interface + in-memory default
│   ├── exec/          # Execution pipeline (Engine)
│   ├── middleware/    # Before/after hook engine
│   ├── registry/      # Tool-definition schema
│   └── tools/         # Built-in service-type tools + custom patchers
├── Makefile           # Library build/vet/test targets
└── .github/workflows/ # Go-library CI (build + vet + test)
```
