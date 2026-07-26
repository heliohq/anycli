// Capability-discovery help face (design 335). AnyCLI flattens a service
// tool's cobra tree to its callable leaves and states the leaf count, because
// a consumer that reads a partial command list treats the missing entries as
// "not supported" rather than "look deeper". The renderer here is exported so
// a host embedding AnyCLI (heliox) can render its own built-in command trees
// with the identical shape instead of forking a second renderer.
package anycli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/heliohq/anycli/internal/toolhelp"
)

// RenderToolHelp writes root's flattened capability face to w: the tool line,
// its Long, every callable form under a "COMMANDS (<n> — complete list)"
// heading with the command's Short echoed verbatim, and a pointer to
// "<leaf> --help" for depth. Hidden commands and cobra's auto-injected
// help/completion commands are excluded; the count is always derived from the
// walk. A tree with no visible leaves falls back to cobra's own help.
//
// A tree whose ROOT is itself runnable (heliox's `browser` passthrough, with
// `list` / `connect` beneath it) counts the root's own form as one of the
// commands — the claim would otherwise be exhaustive over a set missing the
// tool's primary capability, and a false completeness claim is worse than
// none.
func RenderToolHelp(w io.Writer, root *cobra.Command) error {
	return toolhelp.Render(w, root)
}
