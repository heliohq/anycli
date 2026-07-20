# Embedded tool definitions

Real Helio tool definitions live here as `<name>.json` and are loaded by
`LoadBundled` via the embedded filesystem in `../embed.go`. They are internal
to AnyCLI — never consumer-supplied. The original design 003 toolset and later
additions ship here:
slack / notion / gmail / discord / linkedin / x / figma / mongodb (service
type, implemented under `internal/tools/<name>/`) and github / lark (cli type,
wrapping the official `gh` and `lark-cli` binaries).

The `mongodb` definition is the first non-HTTP service tool: its single
resolver field is `connection_string` (a full MongoDB DSN, injected as
`MONGODB_CONNECTION_STRING`) rather than an access token. It is also the
first `source.type: "direct"` definition: the service wraps the official
mongosh binary, pinned by version with a mandatory per-platform sha256 table,
lazily installed from downloads.mongodb.com on first use (see
`internal/exec/binresolve`).
