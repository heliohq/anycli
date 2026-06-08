# Embedded tool definitions

Real Helio tool definitions live here as `<name>.json` and are loaded by
`LoadBundled` via the embedded filesystem in `../embed.go`. They are internal
to AnyCLI — never consumer-supplied. None ship yet; they are added in a later
round. This file keeps the directory present so the `go:embed` of this folder
compiles with zero definitions.
