# AnyCLI - Agent Guidelines

## Project Overview

AnyCLI wraps authenticated cloud service CLIs/APIs into agent-friendly interfaces with automatic credential injection and middleware hooks.

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
- Every CLI tool must support `--help` and `--json` flags
- No short flags (e.g. no `-y`, `-m`). Use full names like `--conflict-policy`
- Use predictable exit codes: 0 for success, non-zero for failure
- All output should be composable via stdin/stdout pipes

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
├── README.md          # Project overview
├── main.go            # Entry point (busybox-style shim detection)
├── cmd/               # CLI commands (install, exec, auth, list, uninstall, update)
├── definitions/       # Bundled tool definitions (go:embed)
├── internal/
│   ├── config/        # Directory management (~/.anycli/)
│   ├── exec/          # Execution pipeline
│   ├── installer/     # Binary downloader (GitHub Releases, npm)
│   ├── middleware/     # Before/after hook engine
│   ├── registry/      # Wrapper definition CRUD
│   └── shim/          # Busybox-style delegation
├── website/public/    # Landing page (Cloudflare Pages)
├── Makefile           # Cross-platform builds
└── .github/workflows/ # CI/CD
```
