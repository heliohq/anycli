// Action-fact inspection for service tools (design 318). Inspect dry-runs a
// tool's cobra command tree to report *facts* about one invocation — the
// stable action id, whether the command may mutate provider state, the
// effective flag values — without executing anything and without any network
// call. AnyCLI stays neutral: it never judges whether an action needs
// approval; that judgment lives entirely in the consumer's policy layer.
package anycli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/heliohq/anycli/internal/tools"
)

// sideEffectAnnotation is the cobra Annotations key carrying a command's
// side-effect fact: "true" | "false". Absent = true (safe-side default).
const sideEffectAnnotation = "anycli.side_effect"

// ActionInvocation is the set of facts Inspect reports about one service-tool
// invocation. All fields are facts derived from a dry-run parse of the tool's
// command tree; none of them are judgments.
type ActionInvocation struct {
	// Action is the stable action id, mechanically derived:
	// "<tool id>." + the resolved cobra command path with spaces replaced
	// by "_" (e.g. "gmail.messages_send"). When argv resolves no deeper
	// than the tree root, Action is the tool id itself.
	Action string
	// SideEffect reports whether this command may issue a mutating provider
	// API call, read from Annotations["anycli.side_effect"] on the resolved
	// command. Unannotated = true.
	SideEffect bool
	// Parsed reports whether the dry-run flag parse on the resolved command
	// succeeded. When false, Flags and Args are empty and Help is false.
	Parsed bool
	// Runnable reports whether argv resolved to a leaf command (a command
	// with no subcommands). A typo path or a path stopping on a group
	// command (has subcommands, regardless of RunE) is not runnable — real
	// execution would only print help/usage.
	Runnable bool
	// Help reports whether cobra consumed a built-in -h/--help flag during
	// the dry-run parse. It is derived from the parsed flag value, never
	// from scanning argv tokens. Parsed=false forces Help=false.
	Help bool
	// Flags is the full effective flag set of the resolved command (local +
	// inherited persistent, i.e. cobra Flags()), keyed by long name.
	Flags map[string]Flag
	// Args are the positional arguments remaining after the subcommand path
	// and flag parse.
	Args []string
}

// Flag is one effective flag value from a dry-run parse (explicitly passed or
// the cobra default).
type Flag struct {
	// Value is the scalar effective value; always "" when IsSlice is true.
	Value string
	// Values is the array-typed effective value (SliceValue.GetSlice());
	// always nil when IsSlice is false.
	Values []string
	// Set reports whether the flag appeared explicitly in argv.
	Set bool
	// IsSlice reports whether the underlying pflag.Value implements
	// pflag.SliceValue — the mechanical scalar/array predicate. Consumers
	// must use IsSlice, not Type, to decide array-ness.
	IsSlice bool
	// Type is cobra's flag.Value.Type(); for bool detection and display
	// only.
	Type string
}

// Inspect resolves args against tool's command tree and reports the action
// facts for that invocation. It never executes the command and never makes a
// network call. A flag-parse failure of the command itself is not an error
// (Parsed=false); an error is returned only for registry misses or internal
// faults.
func Inspect(tool string, args []string) (ActionInvocation, error) {
	root, err := CommandTree(tool)
	if err != nil {
		return ActionInvocation{}, fmt.Errorf("inspect %s: %w", tool, err)
	}
	// Resolve argv to the deepest reachable command. Find's error (unknown
	// subcommand at a root with nil Args) only duplicates the fact that the
	// path stopped on a command with subcommands — the returned command is
	// still the deepest resolved node, so the error is intentionally
	// discarded in favor of Runnable=false.
	cmd, rest, _ := root.Find(args)

	inv := ActionInvocation{
		Action:     actionID(tool, root, cmd),
		SideEffect: cmd.Annotations[sideEffectAnnotation] != "false",
		Runnable:   !cmd.HasSubCommands(),
	}

	// Dry-run flag parse on the resolved node, regardless of Runnable —
	// Runnable and Parsed are independent facts. RunE is never invoked.
	cmd.InitDefaultHelpFlag()
	if err := cmd.ParseFlags(rest); err != nil {
		return inv, nil
	}
	inv.Parsed = true
	helpVal, err := cmd.Flags().GetBool("help")
	if err != nil {
		return ActionInvocation{}, fmt.Errorf("inspect %s: read built-in help flag: %w", tool, err)
	}
	inv.Help = helpVal
	inv.Args = cmd.Flags().Args()
	inv.Flags = collectFlags(cmd)
	return inv, nil
}

// ServiceTools returns the registry tool ids of all built-in service tools,
// sorted. It is the public enumeration face over the internal registry for
// cross-module consumers (internal/tools is not importable outside this
// module).
func ServiceTools() []string {
	return tools.ServiceNames()
}

// CommandTree returns a freshly built cobra command tree for a service tool,
// forwarding the internal Service.NewCommandTree seam. The tree is built with
// empty credentials and is for dry-run parsing and traversal only — its
// commands must never be executed. An unknown tool id is an error.
func CommandTree(tool string) (*cobra.Command, error) {
	svc, err := tools.GetService(tool)
	if err != nil {
		return nil, fmt.Errorf("command tree: %w", err)
	}
	return svc.NewCommandTree(), nil
}

// actionID derives the stable action id from the resolved command: the tool
// id plus the cobra command path below the root, spaces replaced by "_".
func actionID(tool string, root, cmd *cobra.Command) string {
	if cmd == root {
		return tool
	}
	path := strings.TrimPrefix(cmd.CommandPath(), root.CommandPath()+" ")
	return tool + "." + strings.ReplaceAll(path, " ", "_")
}

// collectFlags snapshots the resolved command's full effective flag set
// (local + inherited persistent — ParseFlags has already merged persistent
// flags into Flags()), keyed by long name.
func collectFlags(cmd *cobra.Command) map[string]Flag {
	out := make(map[string]Flag)
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		fl := Flag{Set: f.Changed, Type: f.Value.Type()}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			fl.IsSlice = true
			fl.Values = sv.GetSlice()
		} else {
			fl.Value = f.Value.String()
		}
		out[f.Name] = fl
	})
	return out
}
