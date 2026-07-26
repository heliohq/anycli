package toolhelp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// leafCmd builds a runnable leaf.
func leafCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
}

// groupCmd builds a help-only group command.
func groupCmd(use string, subs ...*cobra.Command) *cobra.Command {
	g := &cobra.Command{
		Use:  use,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	g.AddCommand(subs...)
	return g
}

// sampleTree mirrors the real shape: nouns at the top, callable commands one
// or two levels down, plus every kind of command the walk must drop.
func sampleTree() *cobra.Command {
	root := &cobra.Command{Use: "x", Short: "X built-in service"}

	hiddenLeaf := leafCmd("probe", "Ops probe")
	hiddenLeaf.Hidden = true
	hiddenGroup := groupCmd("internal", leafCmd("dump", "Dump state"))
	hiddenGroup.Hidden = true

	root.AddCommand(
		leafCmd("me", "Show the connected X user"),
		groupCmd("post",
			leafCmd("create", "Create a post"),
			leafCmd("search", "Search recent posts (one page)"),
			groupCmd("thread", leafCmd("create", "Create a thread")),
		),
		hiddenLeaf,
		hiddenGroup,
		&cobra.Command{Use: "placeholder", Short: "Not runnable"},
		// cobra's own injections, as they appear after Execute.
		&cobra.Command{Use: "help", Short: "Help about any command", Run: func(*cobra.Command, []string) {}},
		groupCmd("completion", leafCmd("bash", "Generate the autocompletion script for bash")),
	)
	return root
}

func leafPaths(root *cobra.Command) []string {
	var out []string
	for _, leaf := range Leaves(root) {
		out = append(out, LeafPath(root, leaf))
	}
	return out
}

// TestLeaves proves the walk reaches every depth and drops exactly the four
// excluded shapes: hidden commands, cobra's help/completion, group commands,
// and non-runnable leaves.
func TestLeaves(t *testing.T) {
	got := leafPaths(sampleTree())
	want := []string{"me", "post create", "post search", "post thread create"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Leaves = %q, want %q", got, want)
	}
}

// TestRenderFlattensWithExhaustivenessClaim is the regression for the incident
// in design 335: help must show the callable commands, not the nouns, and must
// say the list is complete.
func TestRenderFlattensWithExhaustivenessClaim(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleTree()); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	got := buf.String()

	want := strings.Join([]string{
		"x — X built-in service",
		"",
		"COMMANDS (4 — complete list)",
		"  me                  Show the connected X user",
		"  post create         Create a post",
		"  post search         Search recent posts (one page)",
		"  post thread create  Create a thread",
		"",
		"Run `<leaf> --help` for its flags and their ranges.",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("Render output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestRenderCountIsDerived proves the claim tracks the tree instead of a
// written-down constant: adding one leaf moves the number by one.
func TestRenderCountIsDerived(t *testing.T) {
	root := sampleTree()
	var before bytes.Buffer
	if err := Render(&before, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(before.String(), "COMMANDS (4 — complete list)") {
		t.Fatalf("want a 4-leaf claim, got:\n%s", before.String())
	}

	root.AddCommand(leafCmd("bookmarks", "List bookmarks"))
	var after bytes.Buffer
	if err := Render(&after, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(after.String(), "COMMANDS (5 — complete list)") {
		t.Fatalf("want a 5-leaf claim, got:\n%s", after.String())
	}
	if !strings.Contains(after.String(), "bookmarks") {
		t.Fatalf("new leaf missing from the list:\n%s", after.String())
	}
}

// TestRenderIncludesRootLong keeps the tool-level mental model (design 335 D1,
// service-root Long) on the coverage face.
func TestRenderIncludesRootLong(t *testing.T) {
	root := sampleTree()
	root.Long = "Reads are cheap on timeline; prefer it over search for your own posts."
	var buf bytes.Buffer
	if err := Render(&buf, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, root.Long) {
		t.Fatalf("root Long missing from help:\n%s", got)
	}
	if idx, claim := strings.Index(got, root.Long), strings.Index(got, "COMMANDS ("); idx > claim {
		t.Fatalf("root Long must precede the command list:\n%s", got)
	}
}

// TestRenderIncludesRootFlags: this face REPLACES cobra's help, which printed
// the root's flags. Nearly every anycli service root carries a persistent
// `--json`, and dropping it here would make it undiscoverable for an
// unconnected tool — `<leaf> --help` is not help-only, so it still resolves
// credentials and fails. By this package's own premise, that absence would read
// as "there is no JSON output".
func TestRenderIncludesRootFlags(t *testing.T) {
	root := sampleTree()
	root.PersistentFlags().Bool("json", false, "single-result JSON; multi-result commands may emit JSONL")
	// cobra injects this on a real Execute; it is plumbing, not capability.
	root.InitDefaultHelpFlag()

	var buf bytes.Buffer
	if err := Render(&buf, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	got := buf.String()

	want := strings.Join([]string{
		"x — X built-in service",
		"",
		"COMMANDS (4 — complete list)",
		"  me                  Show the connected X user",
		"  post create         Create a post",
		"  post search         Search recent posts (one page)",
		"  post thread create  Create a thread",
		"",
		"Flags:",
		"      --json   single-result JSON; multi-result commands may emit JSONL",
		"",
		"Run `<leaf> --help` for its flags and their ranges.",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("Render output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestRenderIncludesInheritedFlags: a host that mounts the tree under its own
// command chain (heliox does) must still see the flags it inherits, under
// cobra's own second heading.
func TestRenderIncludesInheritedFlags(t *testing.T) {
	host := &cobra.Command{Use: "heliox"}
	host.PersistentFlags().String("args-file", "", "read arguments from a file")
	root := sampleTree()
	root.Flags().String("account", "", "account to use")
	host.AddCommand(root)

	var buf bytes.Buffer
	if err := Render(&buf, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "\nFlags:\n      --account string   account to use\n") {
		t.Fatalf("local flag missing:\n%s", got)
	}
	if !strings.Contains(got, "\nGlobal Flags:\n      --args-file string   read arguments from a file\n") {
		t.Fatalf("inherited flag missing:\n%s", got)
	}
	if strings.Index(got, "Flags:") < strings.Index(got, "COMMANDS (") {
		t.Fatalf("flags must follow the command list:\n%s", got)
	}
	if strings.Index(got, helpHint) < strings.Index(got, "Global Flags:") {
		t.Fatalf("the depth pointer must close the face:\n%s", got)
	}
}

// TestRenderOmitsFlagSectionWhenNoFlags keeps an empty "Flags:" heading — a
// claim that the tool takes no flags — off a tree that simply has none.
func TestRenderOmitsFlagSectionWhenNoFlags(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sampleTree()); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if strings.Contains(buf.String(), "Flags:") {
		t.Fatalf("empty flag section rendered:\n%s", buf.String())
	}
}

// TestRenderShortEchoedVerbatim guards the second column against rewriting.
func TestRenderShortEchoedVerbatim(t *testing.T) {
	root := &cobra.Command{Use: "probe", Short: "Probe"}
	short := "List replies (comments) in a post's conversation (one page, last 7 days)"
	root.AddCommand(groupCmd("post", leafCmd("replies", short)))

	var buf bytes.Buffer
	if err := Render(&buf, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(buf.String(), "post replies  "+short+"\n") {
		t.Fatalf("Short not echoed verbatim:\n%s", buf.String())
	}
}

// TestRenderSingleCommandTreeFallsBackToCobra: with nothing to flatten, an
// invented "COMMANDS (0 — complete list)" would be the very false-negative
// signal this face exists to remove.
func TestRenderSingleCommandTreeFallsBackToCobra(t *testing.T) {
	root := &cobra.Command{
		Use:   "solo",
		Short: "One command only",
		Long:  "Long form for solo.",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	var buf bytes.Buffer
	if err := Render(&buf, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "complete list") {
		t.Fatalf("no exhaustiveness claim expected for a leaf-less tree:\n%s", got)
	}
	if !strings.Contains(got, "Long form for solo.") {
		t.Fatalf("want cobra's own help, got:\n%s", got)
	}
}

// TestLeavesCountsRunnableRoot pins the shape heliox's built-in `browser`
// tool has: the root itself is the primary capability (a passthrough) and the
// subcommands are side entrances. Walking children only would claim a
// "complete list" that omits the main thing the tool does — a false claim,
// which design 335 treats as worse than no claim.
func TestLeavesCountsRunnableRoot(t *testing.T) {
	root := &cobra.Command{
		Use:   "browser [--browser <name|id>] -- <agent-browser args...>",
		Short: "Drive the user's paired local Chrome through agent-browser",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(leafCmd("connect", "Mint a connect link"), leafCmd("list", "List paired browsers"))

	got := leafPaths(root)
	want := []string{"[--browser <name|id>] -- <agent-browser args...>", "connect", "list"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Leaves = %q, want %q", got, want)
	}

	var buf bytes.Buffer
	if err := Render(&buf, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "COMMANDS (3 — complete list)") {
		t.Fatalf("runnable root not counted:\n%s", out)
	}
	if !strings.Contains(out, "-- <agent-browser args...>") {
		t.Fatalf("root passthrough form missing from the list:\n%s", out)
	}
}

// TestLeavesRunnableRootWithoutArgSpec renders a column value rather than an
// empty cell when the root's Use carries no argument spec.
func TestLeavesRunnableRootWithoutArgSpec(t *testing.T) {
	root := &cobra.Command{Use: "probe", Short: "Probe", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(leafCmd("list", "List things"))

	var buf bytes.Buffer
	if err := Render(&buf, root); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "COMMANDS (2 — complete list)") {
		t.Fatalf("runnable root not counted:\n%s", out)
	}
	if !strings.Contains(out, "(no arguments)") {
		t.Fatalf("root form rendered as an empty cell:\n%s", out)
	}
}

// TestLeavesIgnoresRunnableGroupParent: a noun that merely prints its own help
// (cobra's convention for a group) is not a callable capability, so it must
// not inflate the count.
func TestLeavesIgnoresRunnableGroupParent(t *testing.T) {
	root := &cobra.Command{Use: "x", Short: "X"}
	root.AddCommand(groupCmd("post", leafCmd("create", "Create a post")))
	got := leafPaths(root)
	if strings.Join(got, "|") != "post create" {
		t.Fatalf("Leaves = %q, want [post create]", got)
	}
}

// TestRenderNilTree keeps the exported seam (anycli.RenderToolHelp) from
// panicking three frames into the walk when an embedder hands it nothing.
func TestRenderNilTree(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil); err == nil {
		t.Fatal("Render(nil) must return an error, not panic or print a claim")
	}
	if buf.Len() != 0 {
		t.Fatalf("Render(nil) wrote output: %q", buf.String())
	}
}
