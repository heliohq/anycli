package tools

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// lintSideEffectAnnotation mirrors the anycli.side_effect annotation key used
// by Inspect (root package). Kept as a literal here because the root-package
// constant is unexported and this package must not import it (import cycle).
const lintSideEffectAnnotation = "anycli.side_effect"

// TestServiceToolTreeLint mechanically enforces the design-318 command-tree
// contracts over EVERY registered service tool, taken through the
// Service.NewCommandTree seam:
//
//	(a) every runnable leaf (RunE or Run non-nil, no subcommands, Hidden
//	    included) carries an explicit anycli.side_effect annotation with
//	    value "true" or "false";
//	(b) no command with subcommands (group command) carries the annotation;
//	(c) no registry tool id contains "." (keeps the "<tool id>." action
//	    prefix unambiguous);
//	(d) no two runnable leaves of one tool derive the same action id
//	    ("<tool id>." + command path below root, spaces -> "_");
//	(e) no cobra command name contains "_" or "." (keeps action-id
//	    derivation reversible);
//	(f) group commands' RunE is nil or help-only (a group with a real
//	    executing body would escape the approval gate: Inspect classifies
//	    it as non-runnable, yet real execution would call the provider).
//
// A failure here fails the build; fix the offending tool package, never this
// test.
func TestServiceToolTreeLint(t *testing.T) {
	names := ServiceNames()
	if len(names) == 0 {
		t.Fatal("no built-in service tools registered — registry seam broken")
	}
	for _, tool := range names {
		t.Run(tool, func(t *testing.T) {
			svc, err := GetService(tool)
			if err != nil {
				t.Fatalf("tool %q: get service: %v", tool, err)
			}
			root := svc.NewCommandTree()
			if root == nil {
				t.Fatalf("tool %q: NewCommandTree returned nil", tool)
			}
			for _, violation := range lintServiceTree(tool, root) {
				t.Error(violation)
			}
		})
	}
}

// lintServiceTree walks one tool's command tree and returns every design-318
// contract violation as a human-readable message naming the offending
// tool/command. An empty slice means the tree is clean.
func lintServiceTree(tool string, root *cobra.Command) []string {
	var violations []string

	// (c) registry tool id must not contain ".".
	if strings.Contains(tool, ".") {
		violations = append(violations, fmt.Sprintf("tool %q: registry tool id contains '.' — the \"<tool id>.\" action prefix must stay unambiguous (design 318)", tool))
	}

	actionIDs := map[string]string{} // derived action id -> command path
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		path := cmd.CommandPath()

		// (e) command names must not contain "_" or ".".
		if name := cmd.Name(); strings.ContainsAny(name, "_.") {
			violations = append(violations, fmt.Sprintf("tool %q, command %q: command name %q contains '_' or '.' — forbidden, action-id derivation must stay reversible (design 318)", tool, path, name))
		}

		annotation, annotated := cmd.Annotations[lintSideEffectAnnotation]

		if cmd.HasSubCommands() {
			// Group command (leaf <=> no subcommands, regardless of RunE).
			// (b) groups must not carry the side-effect annotation.
			if annotated {
				violations = append(violations, fmt.Sprintf("tool %q, group command %q: must not carry %s annotation (got %q) — only runnable leaves are annotated (design 318)", tool, path, lintSideEffectAnnotation, annotation))
			}
			// (f) group RunE must be nil or help-only; Run has no help-only
			// form, so a non-nil Run on a group is always a violation.
			if cmd.Run != nil {
				violations = append(violations, fmt.Sprintf("tool %q, group command %q: has non-nil Run — group commands must have nil or help-only RunE (design 318)", tool, path))
			}
			if cmd.RunE != nil {
				if v := checkGroupRunEHelpOnly(tool, cmd); v != "" {
					violations = append(violations, v)
				}
			}
			for _, sub := range cmd.Commands() {
				walk(sub)
			}
			return
		}

		// Leaf command. Only runnable leaves (RunE or Run non-nil) are on
		// the design-318 traversal face; Hidden is included.
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}

		// (a) explicit annotation with value "true" | "false".
		if !annotated {
			violations = append(violations, fmt.Sprintf("tool %q, runnable leaf %q: missing explicit %s annotation — every runnable leaf must declare \"true\" or \"false\" (design 318)", tool, path, lintSideEffectAnnotation))
		} else if annotation != "true" && annotation != "false" {
			violations = append(violations, fmt.Sprintf("tool %q, runnable leaf %q: %s = %q — value must be exactly \"true\" or \"false\" (design 318)", tool, path, lintSideEffectAnnotation, annotation))
		}

		// (d) derived action ids must be unique within the tool.
		action := deriveActionID(tool, root, cmd)
		if prev, dup := actionIDs[action]; dup {
			violations = append(violations, fmt.Sprintf("tool %q: runnable leaves %q and %q both derive action id %q — action ids must be unique per tool (design 318)", tool, prev, path, action))
		} else {
			actionIDs[action] = path
		}
	}
	walk(root)
	return violations
}

// deriveActionID mirrors the frozen design-318 derivation used by Inspect:
// "<tool id>." + the cobra command path below the root with spaces replaced
// by "_"; the root itself derives the bare tool id.
func deriveActionID(tool string, root, cmd *cobra.Command) string {
	if cmd == root {
		return tool
	}
	rel := strings.TrimPrefix(cmd.CommandPath(), root.CommandPath()+" ")
	return tool + "." + strings.ReplaceAll(rel, " ", "_")
}

// checkGroupRunEHelpOnly verifies a group command's RunE is help-only by
// invoking it with captured output and comparing against the command's own
// help rendering. The tree is built with an empty token, so a group RunE that
// did real work would error or produce non-help output. Returns "" when the
// RunE is help-only, a violation message otherwise.
func checkGroupRunEHelpOnly(tool string, cmd *cobra.Command) string {
	path := cmd.CommandPath()

	var want bytes.Buffer
	cmd.SetOut(&want)
	cmd.SetErr(&want)
	if err := cmd.Help(); err != nil {
		return fmt.Sprintf("tool %q, group command %q: rendering reference help failed: %v", tool, path, err)
	}

	var got bytes.Buffer
	cmd.SetOut(&got)
	cmd.SetErr(&got)
	if err := cmd.RunE(cmd, nil); err != nil {
		return fmt.Sprintf("tool %q, group command %q: RunE returned error %q — group RunE must be nil or help-only (design 318)", tool, path, err)
	}
	if got.String() != want.String() {
		return fmt.Sprintf("tool %q, group command %q: RunE output differs from its help output — group RunE must be nil or help-only (design 318)", tool, path)
	}
	return ""
}

// TestLintServiceTreeDetectsViolations proves each lint rule actually fires,
// using synthetic trees that each break exactly one design-318 contract.
func TestLintServiceTreeDetectsViolations(t *testing.T) {
	leaf := func(use, sideEffect string) *cobra.Command {
		c := &cobra.Command{
			Use:  use,
			RunE: func(*cobra.Command, []string) error { return nil },
		}
		if sideEffect != "" {
			c.Annotations = map[string]string{lintSideEffectAnnotation: sideEffect}
		}
		return c
	}
	helpOnlyGroup := func(use string, subs ...*cobra.Command) *cobra.Command {
		g := &cobra.Command{
			Use:  use,
			RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		}
		g.AddCommand(subs...)
		return g
	}

	cases := []struct {
		name string
		tool string
		root func() *cobra.Command
		want string // substring of exactly one expected violation; "" = clean
	}{
		{
			name: "clean tree passes",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				r.AddCommand(helpOnlyGroup("things", leaf("list", "false"), leaf("create", "true")))
				return r
			},
			want: "",
		},
		{
			name: "a: runnable leaf missing annotation",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				r.AddCommand(leaf("list", ""))
				return r
			},
			want: `runnable leaf "probe list": missing explicit anycli.side_effect annotation`,
		},
		{
			name: "a: hidden runnable leaf missing annotation is still checked",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				hidden := leaf("secret", "")
				hidden.Hidden = true
				r.AddCommand(hidden)
				return r
			},
			want: `runnable leaf "probe secret": missing explicit anycli.side_effect annotation`,
		},
		{
			name: "a: annotation with bogus value",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				r.AddCommand(leaf("list", "yes"))
				return r
			},
			want: `anycli.side_effect = "yes" — value must be exactly "true" or "false"`,
		},
		{
			name: "a: Run-only leaf is runnable and checked",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				r.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
				return r
			},
			want: `runnable leaf "probe list": missing explicit anycli.side_effect annotation`,
		},
		{
			name: "non-runnable leaf is skipped",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				r.AddCommand(&cobra.Command{Use: "placeholder"})
				return r
			},
			want: "",
		},
		{
			name: "b: group carrying the annotation",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				g := helpOnlyGroup("things", leaf("list", "false"))
				g.Annotations = map[string]string{lintSideEffectAnnotation: "false"}
				r.AddCommand(g)
				return r
			},
			want: `group command "probe things": must not carry anycli.side_effect annotation`,
		},
		{
			name: "c: tool id containing a dot",
			tool: "pro.be",
			root: func() *cobra.Command { return &cobra.Command{Use: "probe"} },
			want: `registry tool id contains '.'`,
		},
		{
			name: "d: sibling leaves deriving the same action id",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				// Path "messages send" and literal name "messages_send"
				// collide after space->underscore derivation. The literal
				// name also trips rule (e); rule (d) is asserted here.
				r.AddCommand(
					helpOnlyGroup("messages", leaf("send", "true")),
					leaf("messages_send", "true"),
				)
				return r
			},
			want: `both derive action id "probe.messages_send"`,
		},
		{
			name: "e: command name containing an underscore",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				r.AddCommand(leaf("do_thing", "true"))
				return r
			},
			want: `command name "do_thing" contains '_' or '.'`,
		},
		{
			name: "e: command name containing a dot",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				r.AddCommand(leaf("v1.things", "true"))
				return r
			},
			want: `command name "v1.things" contains '_' or '.'`,
		},
		{
			name: "f: group with a real executing RunE",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				g := &cobra.Command{
					Use: "things",
					RunE: func(cmd *cobra.Command, _ []string) error {
						fmt.Fprintln(cmd.OutOrStdout(), "pretend provider call")
						return nil
					},
				}
				g.AddCommand(leaf("list", "false"))
				r.AddCommand(g)
				return r
			},
			want: `RunE output differs from its help output`,
		},
		{
			name: "f: group with an erroring RunE",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				g := &cobra.Command{
					Use: "things",
					RunE: func(*cobra.Command, []string) error {
						return fmt.Errorf("boom")
					},
				}
				g.AddCommand(leaf("list", "false"))
				r.AddCommand(g)
				return r
			},
			want: `RunE returned error "boom"`,
		},
		{
			name: "f: group with a non-nil Run",
			tool: "probe",
			root: func() *cobra.Command {
				r := &cobra.Command{Use: "probe"}
				g := &cobra.Command{Use: "things", Run: func(*cobra.Command, []string) {}}
				g.AddCommand(leaf("list", "false"))
				r.AddCommand(g)
				return r
			},
			want: `has non-nil Run`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations := lintServiceTree(tc.tool, tc.root())
			if tc.want == "" {
				if len(violations) != 0 {
					t.Fatalf("want clean tree, got violations: %v", violations)
				}
				return
			}
			for _, v := range violations {
				if strings.Contains(v, tc.want) {
					return
				}
			}
			t.Fatalf("no violation containing %q; got: %v", tc.want, violations)
		})
	}
}

// TestDeriveActionID pins the frozen design-318 derivation rule on synthetic
// trees, including the root-resolves-to-tool-id edge.
func TestDeriveActionID(t *testing.T) {
	root := &cobra.Command{Use: "probe"}
	group := &cobra.Command{Use: "messages"}
	send := &cobra.Command{Use: "send"}
	group.AddCommand(send)
	root.AddCommand(group)

	cases := []struct {
		name string
		cmd  *cobra.Command
		want string
	}{
		{name: "root derives bare tool id", cmd: root, want: "probe"},
		{name: "group derives single segment", cmd: group, want: "probe.messages"},
		{name: "nested leaf joins path with underscores", cmd: send, want: "probe.messages_send"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveActionID("probe", root, tc.cmd); got != tc.want {
				t.Fatalf("deriveActionID = %q, want %q", got, tc.want)
			}
		})
	}
}
