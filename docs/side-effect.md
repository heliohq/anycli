# Side Effects: `anycli.side_effect`

Every runnable leaf command of a service tool declares one bit: **may this
invocation issue a mutating provider API call?** The bit rides on the cobra
command as the `anycli.side_effect` annotation, and it is the single thing that
lets a host decide — before anything runs, without a network call and without
resolving a credential — whether an invocation is routine or consequential.

This document is for two readers: the tool author who adds commands and must
classify them, and the host author who consumes the classification.

## Why the bit exists

A service tool's command tree mixes two very different kinds of command. `x post
search` reads. `x post create` speaks publicly in the user's name. Both are one
`Execute` call away, both look identical from outside as an argv slice, and the
consequences of confusing them are asymmetric: a wrongly blocked read wastes a
round trip, a wrongly allowed write cannot be taken back.

Three properties are needed to act on that difference, and only the second is
easy:

1. **The knowledge must live where it is known.** Whether `POST /freeBusy` is a
   query or a mutation is a fact about the provider's API that the person
   writing the command has in front of them and nobody downstream does. Any
   scheme that re-derives it later — an HTTP-verb heuristic, a name regex, a
   hand-maintained list in another repository — is re-deriving something that
   was already known and will drift the first time a command is renamed.

2. **It must be readable without executing.** The decision has to happen before
   the provider call, so the classification cannot be something the command
   reports about itself while running.

3. **It must be exhaustive.** A host that gates on "commands known to write"
   silently allows everything the list forgot. So the absence of the bit cannot
   mean "safe" — it has to mean "assume the worst", and something has to make
   sure the bit is never actually absent.

The annotation satisfies (1) and (2) directly: it is declared inline at the
command definition, in the same expression as the flags and the `Short`, and it
is plain data on a cobra node that anyone holding the tree can read. Point (3)
is split between a safe-side default (below) and a build-breaking lint.

## Facts, not judgments

AnyCLI reports that a command **may mutate**. It never says whether that mutation
needs approval, logging, a confirmation prompt, or a different credential. That
judgment depends on the deployment — who the user is, what the org's risk
appetite is, which actions were pre-authorized — and none of it is knowable from
inside a library that ships the same binary to everyone.

The seam is therefore deliberately thin, and holds in both directions:

- AnyCLI never grows a policy knob, an allow-list, or an "is this dangerous"
  API. `SideEffect` is derived mechanically from an annotation and returned as
  a fact.
- The host never re-derives the bit. It reads what `Inspect` reports and applies
  its own rules on top.

Keeping the judgment out of AnyCLI also keeps it *movable*: a host can start
with an in-process policy and later relocate the same policy to a server-side
executor without AnyCLI changing at all.

## The annotation

```go
Annotations: map[string]string{"anycli.side_effect": "true"}
```

Rules:

- The value is the string `"true"` or `"false"`. Nothing else is legal — not
  `"yes"`, not `"1"`, not an empty string.
- **Only runnable leaves carry it.** A leaf is a command with no subcommands and
  a non-nil `Run`/`RunE`. Group commands (anything with subcommands) must not
  carry it at all.
- `Hidden` leaves carry it too. Hidden means "not advertised to an agent", not
  "exempt from classification" — it is still executable.
- **Absent means `true`.** A leaf that forgot the annotation, a typo'd key, a
  group command a host inspected by accident: all classify as may-mutate. The
  safe side is the only defensible default, because the failure mode of the
  other choice is an unreviewed write.

## How it is read

`anycli.Inspect(tool, argv)` resolves argv against the tool's command tree and
returns the facts about that one invocation. It never invokes `RunE`, never
opens a socket, and never touches a credential:

```go
inv, err := anycli.Inspect("calendar", []string{"events", "create", "--attendee", "a@b.com"})
// inv.Action     == "calendar.events_create"
// inv.SideEffect == true
// inv.Runnable   == true
// inv.Parsed     == true
// inv.Help       == false
// inv.Flags["attendee"].Set == true
```

`SideEffect` is one of several facts, and a host generally needs the others to
use it correctly:

| Fact | Meaning |
| --- | --- |
| `Action` | Stable id: the tool id, a `.`, then the command path below the root with spaces replaced by underscores (`calendar.events_create`). The root resolves to the bare tool id. |
| `SideEffect` | The annotation, with absent = `true`. |
| `Runnable` | argv resolved to a leaf. A typo or a path stopping on a group is **not** runnable — real execution would only print help. |
| `Parsed` | The dry-run flag parse succeeded. When false, `Flags`/`Args` are empty and `Help` is false. |
| `Help` | cobra consumed a built-in `-h`/`--help`, read from the parsed flag value. |
| `Flags` | The full effective flag set (explicit values and cobra defaults), keyed by long name. |

The resolution goes through `internal/dryrun`, a real cobra `Find` +
`ParseFlags` shared with the help short-circuit in `internal/exec`. That sharing is the
point: an executor and an inspector that answer "what would cobra do with this
argv" differently are a gate that can be walked around. In particular, help-ness
is never decided by scanning argv for the token `--help`, which both misses
`post search --help` and fires on `post create --text "--help"`.

A flag-parse failure is not an error — it is the `Parsed=false` fact. `Inspect`
returns an error only for a registry miss or an internal fault. (It discards the
cobra parse error text; a host that wants to show it can re-run `ParseFlags`
itself via `anycli.CommandTree`.)

## Marking a leaf

Three idioms are in use, all equivalent. Pick whichever fits the package.

**Shared vars** — the common case, when a package builds many commands by hand:

```go
// readOnly / writeAction carry the side-effect annotation for a runnable leaf.
// readOnly marks side-effect-free reads (GET, plus POST search/query endpoints
// that only return data); writeAction marks state changes
// (create/update/delete/upsert/add/remove).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)
```

**Inline literal** — when a package has few commands, or when the value deserves
to be read next to the endpoint it describes:

```go
cmd := &cobra.Command{
	Use:         "freebusy",
	Short:       "Query free/busy windows for calendars",
	Annotations: map[string]string{"anycli.side_effect": "false"},
	RunE:        ...,
}
```

**Helper function** — when commands are generated from a table and the
classification is already a parameter:

```go
func sideEffect(write bool) map[string]string {
	return map[string]string{"anycli.side_effect": strconv.FormatBool(write)}
}
```

## Deciding the value

The criterion is **may this command issue a mutating provider API call** — not
"does it usually", not "is it dangerous". Anything that can reach a mutating
endpoint on some code path is `true`.

The HTTP verb is a hint, not the rule:

| Shape | Value | Why |
| --- | --- | --- |
| `GET /things`, `GET /things/{id}` | `false` | Plain read. |
| `POST /things/search`, `POST /freeBusy` | `false` | The provider documents these as pure queries; POST is only there to carry a body too large for a query string. |
| `POST` / `PATCH` / `PUT` / `DELETE` on a resource | `true` | State change. |
| Read-modify-write (`GET` then `PATCH`) | `true` | Classify by the write, not by the first call. |
| Export / download / render to a file | `false` | Provider state is unchanged; the local filesystem is not provider state. |
| A generic escape hatch (`api`, `request`, raw-`--method`) | `true` | The endpoint is chosen at runtime, so the tree cannot prove it reads. Split it if the read half matters: a fixed-`GET` leaf marked `false` next to the general one marked `true`. |
| Anything you had to think about for more than a minute | `true` | The cost of a wrong `false` is unbounded; the cost of a wrong `true` is one extra check. |

Two traps worth naming:

- **Idempotent is not read-only.** An upsert that changes nothing on the second
  call still changed something on the first.
- **"The user asked for it" is not a criterion.** Every command is invoked
  deliberately. The bit describes the provider's state, not the caller's intent.

Write the reason down. The per-tool tests below are the natural place: a
one-line comment naming the endpoint (`// POST /freeBusy — documented pure
query, cannot mutate`) is what a future reviewer needs to re-audit the call
without re-reading the provider docs.

## The tree contracts the classification depends on

`internal/tools/lint_test.go` walks every registered service tool through
`Service.NewCommandTree` and fails the build on any violation. The rules are not
style preferences — each one closes a hole through which a command could reach a
provider without a correct classification.

| Rule | Why |
| --- | --- |
| Every runnable leaf (including `Hidden`) carries an explicit `"true"`/`"false"` | Makes the safe-side default unreachable in practice, so a host is never gating on a guess. |
| No group command carries the annotation | A group's classification is meaningless — it cannot execute. Allowing one invites a host to read the group's value and skip the leaf's. |
| A group's `RunE` is nil or help-only, and its `Run` is nil | **The important one.** `Runnable` is derived from "has no subcommands". A group with a real body is reported as non-runnable — a host skips it as a help path — yet executing it would call the provider. The lint invokes the group's `RunE` with captured output and byte-compares against its own `Help()`. |
| Registry tool ids contain no `.` | Keeps the `"<tool>."` action prefix unambiguous, which is what makes a whole-tool wildcard match soundly implementable as a string prefix. |
| Command names contain no `_` or `.` | Action ids join the path with `_`, so a literal `messages_send` command and a `messages send` path would collide. |
| Action ids are unique within a tool | A host addresses commands by action id; two commands sharing one id means a rule written for either silently governs both. |

The companion test proves each rule actually fires, using synthetic trees that
break exactly one contract each.

## Per-tool pinning

The lint checks that a value exists and is well-formed. It cannot check that the
value is *correct* — that is a claim about the provider's API. Pin it per tool
with a table test that spells out the expected value for every leaf, with the
endpoint in a trailing comment:

```go
want := map[string]string{
	"calendar events list":    "false", // GET /calendars/{cal}/events
	"calendar events create":  "true",  // POST /calendars/{cal}/events
	"calendar events respond": "true",  // GET + PATCH read-modify-write of attendees
	"calendar freebusy":       "false", // POST /freeBusy — documented pure query, cannot mutate
}
```

The map is exhaustive over the tree, so adding a command fails the test until
someone classifies it deliberately. That is the whole value: the failure lands
on the person adding the command, who knows the answer, instead of on a host
weeks later, who does not.

## How a host consumes it

A host does not act on `SideEffect` alone. The pattern that has held up:

**1. Pre-gate on the structural facts.** `Runnable=false` (a typo, or a path
stopping on a group) or `Help=true` means real execution prints usage and stops.
No provider call is possible, so no gate applies — pass through before
consulting any rule.

**2. Match specific rules first.** Rules keyed by action id — exact
(`gmail.messages_send`) or whole-tool wildcard (`gmail.*`) — with optional
conditions over `inv.Flags`. This is where nuance lives: `calendar events create`
is routine until `--attendee` appears, at which point it reaches new people.
Because `Flags` carries effective values (explicit *and* cobra defaults) plus a
`Set` bit, a condition can distinguish "the user asked for this" from "this is
the default".

**3. Fall back to `SideEffect`.** Everything no rule covered routes on the bit:
may-mutate takes the strict branch, read-only takes the permissive one. This is
what makes the scheme exhaustive over thousands of leaves without a
thousand-line rule file — and what makes a newly added command safe on day one,
before anyone has written a rule for it.

**4. Route unparseable invocations on the bit, skipping all rules.** When
`Parsed=false`, no condition can be evaluated, so a conditional rule would match
or miss arbitrarily. Falling straight through to the `SideEffect` default keeps
an unparseable write strict even under a permissive catch-all.

**5. Fail closed on internal faults.** An `Inspect` error or a policy load
failure is a hard stop, never a silent pass-through. A gate that opens when it
breaks is not a gate.

**6. Detect drift mechanically.** `anycli.ServiceTools()` enumerates the service
tools and `anycli.CommandTree(tool)` returns each tree, so a host can walk every
runnable leaf, inspect it, and snapshot the resulting classification. An AnyCLI
upgrade that adds or reclassifies commands then shows up as a snapshot diff in
the host's PR, rather than as a behavior change nobody reviewed.

One more property worth designing around: if a host defers an action for
out-of-band authorization and later replays it, the replay should match on the
literal `{tool, account, argv}` tuple it recorded — not by re-inspecting and
re-evaluating. Otherwise a policy edit between authorization and replay
invalidates an authorization that was already granted.

## Adding a new service tool: checklist

1. Annotate every runnable leaf, `Hidden` included.
2. Leave group commands unannotated, with `RunE` nil or help-only.
3. Avoid `_` and `.` in command names; keep leaf paths unique within the tool.
4. Add the per-tool pinning test with endpoint comments.
5. Run `go test ./...` — the tree lint covers your tool automatically the moment
   it is registered.

## Anti-patterns

- **Annotating a group "for completeness."** It cannot execute; the value is
  noise a host might read instead of the leaf's.
- **Relying on absence to mean read-only.** It means the opposite, and the lint
  exists so absence never happens in the first place.
- **Deciding help-ness by scanning argv.** Use the `Inspect` facts;
  `internal/dryrun` is the only correct answer and it is already shared with the
  executor.
- **Adding a policy hook to AnyCLI.** The bit is a fact. Whatever you were about
  to encode belongs in the host, where the deployment context is.
- **Marking a generic escape hatch `false` because "agents only read with it."**
  The tree cannot prove that, and neither can you.

## Reference

- `inspect.go` — `Inspect`, `ActionInvocation`, `Flag`, `ServiceTools`,
  `CommandTree`
- `internal/dryrun/dryrun.go` — the shared resolve-without-executing seam
- `internal/tools/lint_test.go` — the tree contracts, enforced
- `internal/tools/calendar/side_effect_test.go` — a representative per-tool
  pinning test
