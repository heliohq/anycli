# AnyCLI - Agent Guidelines

## Project Overview

AnyCLI is an embeddable Go library (design 002): the engine plus the embedded definitions for the tools it supports. A host (e.g. Helio's `heliox`) embeds it in-process, supplies a `CredentialResolver` (and optionally a `Cache`), and calls `Engine.Execute`. AnyCLI loads the matching embedded tool definition, injects credentials (env / arg / ephemeral file), runs middleware, and execs the underlying binary or built-in service. It is **not** a standalone CLI, and tool definitions are **not** consumer-supplied — they live embedded inside AnyCLI.

## Tech Stack

- Language: Go
- Build: Go modules
- Formatting: `gofmt`

## Language Rule

- **All content must be in English** — code, comments, documentation, commit messages, PR descriptions, and error messages. No exceptions.

## Confidentiality

- **This repository is PUBLIC.** Never expose internal design or engineering detail: internal design-doc numbers and section titles, internal repo/service/skill paths, ticket ids, or downstream product features. A reader outside the org cannot resolve them, and they leak architecture and roadmap.
- Describe the mechanism in this repo's own terms instead — name the file, the annotation, the contract. The thing itself, never the internal review that decided it.

## Development Rules

- Write tests first, then implement
- Run tests before marking any task complete
- Follow existing code patterns
- Keep it simple — no over-engineering
- **No interactive prompts** — all input must come from flags or environment variables. AnyCLI is designed for agents, not humans typing into terminals.

## Command Help (design 335)

A tool's `--help` is the only place an agent learns what the integration covers, and it reads a partial list as "not supported". So the coverage face must be exhaustive, and it must say that it is.

`--help` is generated, and *which* help you get depends on the node argv resolves to:

- **root** (`<tool> --help`) — `internal/toolhelp` flattens the whole tree to its callable leaves and states the derived count. Never write that count down anywhere.
- **any deeper node** (`<tool> post search --help`) — cobra's own help for that node: its flags, and its `Long` if it has one.

Keep the root face lean. It is the coverage surface — the exhaustive leaf list is the payload, and prose above it pushes the list down. Depth belongs on the leaf, where it is fetched only when someone is about to run that command.

`Short` is required and is what the flattened list echoes, so make it carry real information (`post replies` says "one page, last 7 days"). `Long` is optional; write one when there is a provider fact the flags cannot express — a pagination window, an API-tier gate, a cheaper path to the same answer. There is no coverage requirement on `Long` and no lint enforcing one: a rule like that gets satisfied by placeholder prose, which is worse than an empty field.

Per-tool guides live in the Helio repo under `agents/marketplace/heliox/skills/tool/`, read on demand. Design 335 §D1 records why they were not folded into `Long` — the migration was implemented in full and rejected, because a service root's `Long` renders on the coverage face.

Help-ness is decided by `internal/dryrun`, a real cobra `Find` + `ParseFlags` shared with `Inspect` (design 318) — never by scanning argv for the token `--help`, which both misses `post search --help` and fires on `post create --text "--help"`. Neither path resolves credentials, so an unconnected tool can still answer both (design 335 D3). Binary-passthrough tools (`github`, `lark`) have no tree: their help comes from the wrapped binary and anycli must not stamp a completeness claim on it.

## Side Effects

Every runnable leaf of a service tree declares `anycli.side_effect` (`"true"` | `"false"`): may this command issue a mutating provider API call? A host reads it through `Inspect` — before execution, no network, no credential — to decide what an invocation deserves. Absent means `true`: the failure mode of the other default is an unreviewed write. Group commands carry no annotation and their `RunE` must be nil or help-only, because `Runnable` is derived from "has no subcommands" and a group with a real body would execute while reporting as a help path. `internal/tools/lint_test.go` enforces all of it over every registered tool; pin the *values* per tool in a table test with the endpoint in a trailing comment.

AnyCLI reports the fact and never the judgment — no policy knob, no allow-list, no "is this dangerous" API. See [docs/side-effect.md](docs/side-effect.md) for the classification criterion, the boundary cases, and the consumption pattern.

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
