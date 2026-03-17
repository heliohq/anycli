# AnyCLI - Agent Guidelines

## Project Overview

AnyCLI makes every tool agent-native by wrapping existing CLIs, APIs, or services into lightweight, agent-friendly command-line interfaces.

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

## Code Style

- Prefer simple, readable code over clever abstractions
- Every CLI tool must support `--help`, `--json`, and `-y` (non-interactive) flags
- Use predictable exit codes: 0 for success, non-zero for failure
- All output should be composable via stdin/stdout pipes

## Git Conventions

- Commit format: `type(scope): message`
- Types: `feat`, `fix`, `refactor`, `chore`, `ci`, `docs`, `test`
- Prefer small, atomic commits — each commit should be the smallest unit of change that doesn't break integrity (builds pass, tests pass)
- One logical change per commit; split unrelated changes into separate commits

## Project Structure

```
anycli/
├── AGENTS.md          # Agent guidelines (this file)
├── CLAUDE.md          # Symlink -> AGENTS.md
├── WHY_ANY_CLI.md     # Rationale: why CLI over MCP
├── README.md          # Project overview
└── ...
```
