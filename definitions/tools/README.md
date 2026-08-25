# Embedded tool definitions

Real Helio tool definitions live here as `<name>.json` and are loaded by
`LoadBundled` via the embedded filesystem in `../embed.go`. They are internal
to AnyCLI — never consumer-supplied. The original design 003 toolset and later
additions ship here:
slack / notion / gmail / discord / linkedin / x / figma / mongodb / supabase
(service type, implemented under `internal/tools/<name>/`) and github / lark
(cli type, wrapping the official `gh` and `lark-cli` binaries).

The `mongodb` definition is the first non-HTTP service tool: its single
resolver field is `connection_string` (a full MongoDB DSN, injected as
`MONGODB_CONNECTION_STRING`) rather than an access token. It is also the
first `source.type: "direct"` definition: the service wraps the official
mongosh binary, pinned by version with a mandatory per-platform sha256 table,
lazily installed from downloads.mongodb.com on first use (see
`internal/exec/binresolve`).

The `supabase` definition is also a binary-backed service. It injects an OAuth
access token as `SUPABASE_ACCESS_TOKEN` and wraps a pinned official Supabase
CLI release with an explicit command tree. Allowed command paths keep their
side-effect classifications, while all following arguments and flags are
forwarded verbatim for the official CLI to validate. The `db query` and
`storage cp` leaves are always classified as side-effectful. The service omits
local stack lifecycle, migration, login, and arbitrary command paths; callers
choose official experimental, confirmation, and Function Docker/API behavior
with the corresponding upstream flags.
