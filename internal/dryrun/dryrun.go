// Package dryrun resolves one argv against a cobra command tree without
// running anything: it finds the deepest reachable command, parses that
// command's flags, and reports what cobra itself made of the arguments. RunE
// is never invoked, no network call is made, and no credential is touched.
//
// This is the single seam for "what would cobra do with these args", shared by
// the design-318 action-fact inspector (anycli.Inspect) and the design-331
// help short-circuit in internal/exec. Both need the same question answered —
// most sharply "did cobra consume a built-in -h/--help?" — and the only
// correct answer comes from cobra's own parse. Scanning argv for the string
// "--help" is wrong in both directions: it misses `post search --help` (a help
// request with positional args ahead of it) and it fires on `post create
// --text "--help"`, `post create -- --help` and `--help=false`, none of which
// are help requests. Keeping one implementation here keeps the inspector and
// the executor from ever disagreeing about the same argv.
//
// Constraint this puts on tool authors: a command must not use -h as another
// flag's shorthand (cobra's InitDefaultHelpFlag panics on the redefinition)
// and must not define a non-bool flag named "help" (the bool read then fails
// and Resolve returns an error). Neither is a new rule, and neither gets a
// wider blast radius here: cobra's own execute() makes the same two calls on
// the same resolved node, unconditionally on every invocation rather than
// only on a help path, and fails identically.
package dryrun

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Resolution is what a dry-run parse learned about one argv.
type Resolution struct {
	// Cmd is the deepest command argv resolved to. A path that stops on a
	// group command or on a typo resolves to that group (or the root),
	// which is exactly the node whose help cobra would print.
	Cmd *cobra.Command
	// Parsed reports whether the flag parse on Cmd succeeded. When false,
	// Help is false and Args is nil — cobra never got far enough to know.
	Parsed bool
	// Help reports whether cobra consumed a built-in -h/--help flag. It is
	// read from the parsed flag value, never from scanning argv tokens.
	Help bool
	// Args are the positional arguments left after the subcommand path and
	// the flag parse.
	Args []string
}

// Resolve dry-runs args against root and reports the resolution. A flag-parse
// failure of the command itself is not an error — it is the Parsed=false fact.
// An error is returned only for an internal fault (a nil tree, or a built-in
// help flag that is somehow not a bool).
func Resolve(root *cobra.Command, args []string) (Resolution, error) {
	if root == nil {
		return Resolution{}, fmt.Errorf("dry-run resolve: nil command tree")
	}
	// Find's error (unknown subcommand at a root with nil Args) only
	// duplicates the fact that the path stopped on a command with
	// subcommands — the returned command is still the deepest resolved
	// node, so the error is intentionally discarded in favor of the
	// Cmd/Args facts.
	cmd, rest, _ := root.Find(args)

	res := Resolution{Cmd: cmd}
	cmd.InitDefaultHelpFlag()
	if err := cmd.ParseFlags(rest); err != nil {
		return res, nil
	}
	res.Parsed = true
	help, err := cmd.Flags().GetBool("help")
	if err != nil {
		return Resolution{}, fmt.Errorf("read built-in help flag: %w", err)
	}
	res.Help = help
	res.Args = cmd.Flags().Args()
	return res, nil
}
