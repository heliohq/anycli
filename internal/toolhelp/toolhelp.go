// Package toolhelp renders the capability-discovery help face for a service
// tool's in-process cobra command tree.
//
// cobra's default help lists a command's *direct children* only, so a tool
// whose commands are grouped under nouns advertises the nouns and hides every
// callable command one level down. An AI consumer reads a command list as an
// exhaustive statement of what the integration covers: a missing entry is not
// read as "look deeper", it is read as "not supported". The face here
// therefore walks the whole tree, lists only the callable leaves, and states
// the leaf count as an explicit exhaustiveness claim.
//
// The renderer applies to anycli's own in-process ("service") command trees.
// Binary-passthrough tools (github wrapping gh, lark wrapping lark-cli) exec
// the real binary and never reach this package, so anycli never puts an
// exhaustiveness claim on help output it did not produce.
package toolhelp

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// helpHint is the closing line of the flattened face: coverage is complete
// here, depth is one command away.
const helpHint = "Run `<leaf> --help` for its flags and their ranges."

// Leaves returns every AI-callable form of root, depth-first in cobra's own
// child order (sorted by default).
//
// A leaf is a command with no subcommands that has a Run or RunE body. Three
// things are excluded:
//
//   - Hidden commands — an ops/onboarding surface, not an AI-callable one.
//     (This is intentionally a different cut from the tree lint, which
//     counts Hidden commands as leaves because it needs full policy coverage.)
//   - cobra's auto-injected "help" and "completion" commands.
//   - non-runnable leaves (no Run and no RunE) — nothing to call.
//
// A RUNNABLE root is itself one of the callable forms and is returned first.
// A tree can be runnable at the root and still carry subcommands — a host's
// built-in `browser` is exactly that shape, where the root passthrough is the
// primary capability and `list` / `connect` are side entrances. Counting only
// the children there would print an exhaustiveness claim over a set that omits
// the tool's main capability, which is a FALSE claim and therefore worse than
// printing no claim at all.
//
// A runnable root with no sub-leaves returns nothing: that is a single-command
// tool, cobra's own help already describes it completely, and a one-row
// "complete list" would add no coverage information (see Render's fallback).
func Leaves(root *cobra.Command) []*cobra.Command {
	if root == nil {
		return nil
	}
	sub := subLeaves(root)
	if len(sub) == 0 {
		return nil
	}
	if isRunnable(root) {
		return append([]*cobra.Command{root}, sub...)
	}
	return sub
}

// subLeaves walks the tree BELOW root and returns its callable leaves.
func subLeaves(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var walk func(cmd *cobra.Command, atRoot bool)
	walk = func(cmd *cobra.Command, atRoot bool) {
		for _, sub := range cmd.Commands() {
			if sub.Hidden {
				continue
			}
			if atRoot && isCobraInjected(sub) {
				continue
			}
			if sub.HasSubCommands() {
				walk(sub, false)
				continue
			}
			if !isRunnable(sub) {
				continue
			}
			out = append(out, sub)
		}
	}
	walk(root, true)
	return out
}

// isRunnable reports whether cmd has a body to execute.
func isRunnable(cmd *cobra.Command) bool {
	return cmd.Run != nil || cmd.RunE != nil
}

// isCobraInjected reports whether name is one of the two commands cobra adds
// to a root by itself. Both are shell/CLI plumbing, not provider capability.
func isCobraInjected(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "help", "completion":
		return true
	default:
		return false
	}
}

// LeafPath returns a leaf's command path relative to root ("post search"),
// which is what the caller types after the tool name.
//
// For the root itself (the runnable-root form, see Leaves) that is the
// argument spec of its Use line — "[--browser <name|id>] -- <agent-browser
// args...>" — which is likewise what the caller types after the tool name.
func LeafPath(root, leaf *cobra.Command) string {
	if leaf == root {
		return rootCallForm(root)
	}
	return strings.TrimPrefix(leaf.CommandPath(), root.CommandPath()+" ")
}

// rootCallForm renders the root's own runnable form. A root whose Use carries
// no argument spec takes nothing after the tool name, so the column says so
// rather than rendering an empty cell.
func rootCallForm(root *cobra.Command) string {
	if spec := strings.TrimSpace(strings.TrimPrefix(root.Use, root.Name())); spec != "" {
		return spec
	}
	return "(no arguments)"
}

// flagSections renders the root's own flags, and any flags it inherits from a
// host command, under cobra's two stock headings.
//
// This face REPLACES cobra's help, so anything cobra used to print and this
// does not is information the reader now loses — and by this package's own
// premise, absence reads as non-existence. Nearly every service root carries a
// persistent `--json`, which is the single most load-bearing flag an AI
// consumer has, and the root face is where it is stated once rather than
// repeated on every leaf's inherited-flags block.
func flagSections(root *cobra.Command) string {
	var b strings.Builder
	if usages := visibleFlagUsages(root.LocalFlags()); usages != "" {
		b.WriteString("\nFlags:\n" + usages)
	}
	if usages := visibleFlagUsages(root.InheritedFlags()); usages != "" {
		b.WriteString("\nGlobal Flags:\n" + usages)
	}
	return b.String()
}

// visibleFlagUsages renders set's usage lines minus cobra's auto-injected
// `--help`, which is plumbing rather than provider capability — the same cut
// the leaf walk makes on the injected help/completion COMMANDS. Filtering it
// also keeps the face byte-identical whether or not cobra has already run
// InitDefaultHelpFlag on the tree, which differs between the in-process render
// path and a host that renders from its own help hook.
func visibleFlagUsages(set *pflag.FlagSet) string {
	filtered := pflag.NewFlagSet("", pflag.ContinueOnError)
	set.VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" || f.Hidden {
			return
		}
		filtered.AddFlag(f)
	})
	return filtered.FlagUsages()
}

// Render writes the flattened capability face for root to w:
//
//	<name> — <Short>
//
//	<Long, when set>
//
//	COMMANDS (<n> — complete list)
//	  post create      Create a post
//	  post search      Search recent posts (one page)
//
//	Flags:
//	      --json   single-result JSON; multi-result commands may emit JSONL
//
//	Run `<leaf> --help` for its flags and their ranges.
//
// The count is always derived from the walk, never a written-down constant —
// a stale constant would be exactly the false claim this face exists to
// prevent. The second column is each leaf's Short, echoed verbatim.
//
// A tree with no visible leaves (a single-command tool) falls back to cobra's
// own help: there is nothing to flatten, and printing "COMMANDS (0)" would
// itself be the false-negative signal.
func Render(w io.Writer, root *cobra.Command) error {
	// Render is reachable as public API (anycli.RenderToolHelp) from any
	// embedder, so a missing tree is an explicit error rather than a panic
	// three frames deep in the walk.
	if root == nil {
		return fmt.Errorf("render help: nil command tree")
	}
	leaves := Leaves(root)
	if len(leaves) == 0 {
		root.SetOut(w)
		root.SetErr(w)
		if err := root.Help(); err != nil {
			return fmt.Errorf("render cobra help for %q: %w", root.Name(), err)
		}
		return nil
	}

	var b strings.Builder
	b.WriteString(root.Name())
	if root.Short != "" {
		b.WriteString(" — " + root.Short)
	}
	b.WriteString("\n")

	if long := strings.TrimSpace(root.Long); long != "" {
		b.WriteString("\n" + long + "\n")
	}

	fmt.Fprintf(&b, "\nCOMMANDS (%d — complete list)\n", len(leaves))
	paths := make([]string, len(leaves))
	width := 0
	for i, leaf := range leaves {
		paths[i] = LeafPath(root, leaf)
		if n := len(paths[i]); n > width {
			width = n
		}
	}
	for i, leaf := range leaves {
		if short := leaf.Short; short != "" {
			fmt.Fprintf(&b, "  %-*s  %s\n", width, paths[i], short)
			continue
		}
		fmt.Fprintf(&b, "  %s\n", paths[i])
	}

	b.WriteString(flagSections(root))
	b.WriteString("\n" + helpHint + "\n")

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write help for %q: %w", root.Name(), err)
	}
	return nil
}
