# Embedded tool definitions

Real Helio tool definitions live here as `<name>.json` and are loaded by
`LoadBundled` via the embedded filesystem in `../embed.go`. They are internal
to AnyCLI — never consumer-supplied. The design 003 toolset ships here:
slack / notion / google / discord / linkedin (service type, implemented under
`internal/tools/<name>/`) and github (cli type, wrapping `gh`).
