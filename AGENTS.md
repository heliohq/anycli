# AnyCLI - Agent Guidelines

## Project Overview

AnyCLI is an embeddable Go library ([docs/design/002](docs/design/002-embeddable-core-and-credential-resolver.md)): a host embeds it in-process, supplies a `CredentialResolver`, and calls `Engine.Execute`. AnyCLI loads the matching definition, injects credentials (env / arg / ephemeral file), runs middleware, and execs the binary or built-in service. Not a standalone CLI; definitions are embedded here, not consumer-supplied.

## Language Rule

- **All content must be in English** — code, comments, documentation, commit messages, PR descriptions, and error messages. No exceptions.

## Confidentiality

- **This repository is PUBLIC.** Never expose internal design-doc numbers or section titles, internal repo/service/skill paths, ticket ids, or downstream product features: an outside reader cannot resolve them, and they leak architecture and roadmap.
- State the mechanism in this repo's own terms — the file, the annotation, the contract — never the internal review that decided it.

## Development Rules

- Write tests first, then implement
- Run tests before claiming done
- **No interactive prompts** — input comes from flags or env vars; AnyCLI serves agents, not humans at terminals
- Definitions target `--json` output and non-interactive flags so agents can consume results
- Exit codes are load-bearing: 0 success, 2 usage/param error (bad flag combo, invalid JSON, unknown subcommand), 1 runtime/API failure

## Command Help

`--help` is the only place an agent learns what a tool covers, and a partial list reads as "not supported": the coverage face must be exhaustive and say so. `<tool> --help` gets the flattened face from `internal/toolhelp` — every callable leaf plus the derived count, which is never written down anywhere. Any deeper node (`post search --help`) gets cobra's own help: that node's flags and its `Long`.

Keep the root face lean: the leaf list is the payload, prose above it pushes the list down — which is also why long-form guides live outside this repo (folding them into `Long` was tried and rejected).

`Short` is required and is what the flattened list echoes, so make it carry real information (`post replies`: "one page, last 7 days"). `Long` is optional — a provider fact the flags cannot express: a pagination window, an API-tier gate, a cheaper path. No lint enforces one; that rule gets satisfied by placeholder prose, worse than an empty field.

Help-ness comes from `internal/dryrun` — a real cobra `Find` + `ParseFlags`, shared with `Inspect` — never from scanning argv for `--help`, which misses `post search --help` and fires on `post create --text "--help"`. Neither resolves credentials, so an unconnected tool still answers. Binary-passthrough tools (`github`, `lark`) have no tree: help comes from the wrapped binary, and anycli must not claim completeness over it.

## Side Effects

Every runnable leaf declares `anycli.side_effect` (`"true"` | `"false"`): may this command issue a mutating provider API call? A host reads it through `Inspect`: before execution, no network, no credential. Absent means `true` — the other default's failure mode is an unreviewed write. Groups carry no annotation and their `RunE` must be nil or help-only: `Runnable` means "has no subcommands", so a group with a body would execute while reporting as a help path. `internal/tools/lint_test.go` enforces this; pin the *values* per tool in a table test, endpoint in a trailing comment.

AnyCLI reports the fact, never the judgment — no policy knob, no allow-list. See [docs/side-effect.md](docs/side-effect.md) for the criterion, boundary cases, and consumption pattern.

## Git Conventions

- `type(scope): message`, types `feat` `fix` `refactor` `chore` `ci` `docs` `test`
- Smallest change that keeps builds and tests passing, one logical change per commit
- **Do not commit unless the user explicitly asks** — never auto-commit

## Map

Root files are the public API: `anycli.go` (Config, New, `Engine.Execute`), `manifest.go` (ListTools), `inspect.go` (action facts without executing), `help.go`, `resolve.go` (binary pre-warming). `definitions/` holds the embedded tool definitions, `cmd/anycli/` is a dev harness rather than the product, and `internal/` is the engine — `ls internal` for the packages.
