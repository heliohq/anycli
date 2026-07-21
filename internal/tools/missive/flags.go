package missive

import (
	"net/url"

	"github.com/spf13/cobra"
)

// addBodyFlags wires the shared request-body flags for write verbs. The global
// --json flag is reserved for the error-envelope toggle (all data output is
// already JSON), so the inline payload flag is --body; --file reads a path, or
// - for stdin.
func addBodyFlags(cmd *cobra.Command, inline, file *string) {
	cmd.Flags().StringVar(inline, "body", "", "request body as an inline JSON object")
	cmd.Flags().StringVar(file, "file", "", "read the request body from a file path, or - for stdin")
}

// setBoolFilter sets key=true on q only when v is set. Missive's mailbox and
// boolean filters are presence flags (inbox=true), never inbox=false.
func setBoolFilter(q url.Values, key string, v bool) {
	if v {
		q.Set(key, "true")
	}
}
