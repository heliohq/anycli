package dryrun

import (
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

// probeTree builds a grouped tree with a string flag whose value can be the
// literal "--help", plus a three-level path.
func probeTree() *cobra.Command {
	run := func(*cobra.Command, []string) error { return nil }

	root := &cobra.Command{Use: "probe"}
	root.PersistentFlags().Bool("json", false, "json output")

	create := &cobra.Command{Use: "create", RunE: run}
	create.Flags().String("text", "", "body")
	post := &cobra.Command{Use: "post"}
	post.AddCommand(create, &cobra.Command{Use: "search", RunE: run})

	members := &cobra.Command{Use: "members"}
	members.AddCommand(&cobra.Command{Use: "list", RunE: run})
	lists := &cobra.Command{Use: "lists"}
	lists.AddCommand(members)

	root.AddCommand(post, lists)
	return root
}

// TestResolve pins the one question both callers ask — "did cobra consume a
// built-in -h/--help?" — against every argv shape where a string scan for the
// token "--help" gives the wrong answer.
func TestResolve(t *testing.T) {
	cases := []struct {
		name string
		args []string

		wantPath   string
		wantParsed bool
		wantHelp   bool
		wantArgs   []string
	}{
		{
			name:       "no args stays on the root",
			args:       nil,
			wantPath:   "probe",
			wantParsed: true,
		},
		{
			name:       "bare help on the root",
			args:       []string{"--help"},
			wantPath:   "probe",
			wantParsed: true,
			wantHelp:   true,
		},
		{
			name:       "shorthand help on the root",
			args:       []string{"-h"},
			wantPath:   "probe",
			wantParsed: true,
			wantHelp:   true,
		},
		{
			name:       "help on a group node",
			args:       []string{"post", "--help"},
			wantPath:   "probe post",
			wantParsed: true,
			wantHelp:   true,
		},
		{
			name:       "help behind positionals still resolves to the leaf",
			args:       []string{"post", "search", "--help"},
			wantPath:   "probe post search",
			wantParsed: true,
			wantHelp:   true,
		},
		{
			name:       "three levels down",
			args:       []string{"lists", "members", "list", "--help"},
			wantPath:   "probe lists members list",
			wantParsed: true,
			wantHelp:   true,
		},
		{
			name:       "an unknown subcommand stops on the parent and stays a positional",
			args:       []string{"post", "bogus", "--help"},
			wantPath:   "probe post",
			wantParsed: true,
			wantHelp:   true,
			wantArgs:   []string{"bogus"},
		},
		{
			name:       "help beside another flag",
			args:       []string{"--json", "--help"},
			wantPath:   "probe",
			wantParsed: true,
			wantHelp:   true,
		},
		{
			name:       "the literal --help as a flag VALUE is not a help request",
			args:       []string{"post", "create", "--text", "--help"},
			wantPath:   "probe post create",
			wantParsed: true,
		},
		{
			name:       "everything after -- is positional",
			args:       []string{"post", "create", "--", "--help"},
			wantPath:   "probe post create",
			wantParsed: true,
			wantArgs:   []string{"--help"},
		},
		{
			name:       "help explicitly disabled",
			args:       []string{"--help=false"},
			wantPath:   "probe",
			wantParsed: true,
		},
		{
			name:     "a flag parse failure is a fact, not an error",
			args:     []string{"post", "create", "--nope"},
			wantPath: "probe post create",
			// Parsed=false forces Help=false and Args=nil: cobra never
			// got far enough to know either.
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Resolve(probeTree(), tc.args)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if got := res.Cmd.CommandPath(); got != tc.wantPath {
				t.Errorf("resolved node = %q, want %q", got, tc.wantPath)
			}
			if res.Parsed != tc.wantParsed {
				t.Errorf("Parsed = %v, want %v", res.Parsed, tc.wantParsed)
			}
			if res.Help != tc.wantHelp {
				t.Errorf("Help = %v, want %v", res.Help, tc.wantHelp)
			}
			if !slices.Equal(res.Args, tc.wantArgs) {
				t.Errorf("Args = %q, want %q", res.Args, tc.wantArgs)
			}
		})
	}
}

// TestResolveNilTree: Resolve is reachable from two packages, so a missing
// tree is an explicit error rather than a nil dereference inside cobra.
func TestResolveNilTree(t *testing.T) {
	if _, err := Resolve(nil, []string{"--help"}); err == nil {
		t.Fatal("expected an error for a nil command tree")
	}
}

// TestResolveNeverRuns proves the dry run is dry: a tree whose RunE would fail
// the test must come back untouched.
func TestResolveNeverRuns(t *testing.T) {
	root := &cobra.Command{Use: "probe"}
	root.AddCommand(&cobra.Command{Use: "boom", RunE: func(*cobra.Command, []string) error {
		t.Fatal("RunE executed during a dry-run resolve")
		return nil
	}})
	for _, args := range [][]string{{"boom"}, {"boom", "--help"}} {
		if _, err := Resolve(root, args); err != nil {
			t.Fatalf("Resolve(%q) returned error: %v", args, err)
		}
	}
}

// recordingTree builds a tree whose every leaf RunE appends its command path
// to *ran, making "did anything actually execute?" observable. It carries the
// flag shapes that make a string scan for "--help" give the wrong answer: a
// string flag and a slice flag that can take "--help" as their VALUE, a bool
// with a NoOptDefVal, shorthands that cluster with -h, and a leaf with
// DisableFlagParsing (where cobra deliberately does not consume --help).
func recordingTree(ran *[]string) *cobra.Command {
	rec := func(cmd *cobra.Command, _ []string) error {
		*ran = append(*ran, cmd.CommandPath())
		return nil
	}

	root := &cobra.Command{Use: "probe", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolP("json", "j", false, "json output")
	root.PersistentFlags().CountP("verbose", "v", "verbosity")

	create := &cobra.Command{Use: "create", RunE: rec}
	create.Flags().String("text", "", "body")
	create.Flags().StringSlice("tag", nil, "tags")
	create.Flags().Bool("draft", false, "draft")

	search := &cobra.Command{Use: "search", RunE: rec}
	search.Flags().Int("limit", 0, "page size")

	post := &cobra.Command{Use: "post"}
	post.AddCommand(create, search)

	members := &cobra.Command{Use: "members"}
	members.AddCommand(&cobra.Command{Use: "list", RunE: rec})
	lists := &cobra.Command{Use: "lists"}
	lists.AddCommand(members)

	root.AddCommand(
		post,
		lists,
		&cobra.Command{Use: "raw", DisableFlagParsing: true, RunE: rec},
		&cobra.Command{Use: "me", RunE: rec},
	)
	return root
}

// TestResolveHelpImpliesCobraRunsNothing is the safety property the whole
// help short-circuit rests on: whenever Resolve reports Help, running the
// IDENTICAL argv through real cobra invokes no RunE. That is what makes it
// sound for internal/exec to skip credential resolution and print help
// instead — the short-circuit can never swallow an invocation that cobra
// would have treated as a real action.
//
// The converse is deliberately NOT asserted. Shapes like `post create --text
// --help` and `raw --help` are real invocations, and cobra running them is
// the correct outcome; the property only forbids the dangerous direction.
func TestResolveHelpImpliesCobraRunsNothing(t *testing.T) {
	shapes := [][]string{
		nil, {},
		// Plain help, doubled, and explicitly negated.
		{"--help"}, {"-h"}, {"--help", "--help"}, {"-h", "-h"},
		{"--help=true"}, {"--help=false"}, {"-h=false"},
		// The -- terminator makes everything after it positional.
		{"--"}, {"--", "--help"}, {"--", "-h"}, {"--help", "--"},
		{"--", "--", "--help"},
		// "--help" smuggled in as a flag VALUE is not a help request.
		{"post", "create", "--text", "--help"},
		{"post", "create", "--text=--help"},
		{"post", "create", "--tag", "--help"},
		{"post", "create", "--tag", "a", "--tag", "-h"},
		{"post", "create", "--draft", "--help"},
		{"post", "create", "--draft=--help"},
		{"post", "create", "--", "--help"},
		{"post", "create", "--text", "x", "--", "-h"},
		// Shorthand clusters containing h.
		{"-v", "--help"}, {"--verbose", "--help"},
		{"-vh"}, {"-hv"}, {"-jh"},
		// Help ahead of a still-resolvable subcommand path.
		{"post", "--help", "search"}, {"--help", "post", "search"},
		{"--help", "post", "list"},
		// Typos and empty tokens.
		{"post", "bogus", "--help"}, {"bogus", "--help"}, {"", "--help"},
		{"post", "search", "kittens", "--help"},
		// Flag-parse failures.
		{"post", "create", "--nope"}, {"--nope", "--help"}, {"--help", "--nope"},
		// A DisableFlagParsing leaf: cobra does not consume --help here.
		{"raw", "--help"}, {"raw", "-h"}, {"raw", "--", "--help"},
		// Commands cobra injects at Execute time, invisible to the dry run.
		{"help"}, {"help", "post"}, {"help", "--help"},
		{"completion"}, {"completion", "bash"},
		{"completion", "--help"}, {"completion", "bash", "--help"},
		{"__complete", "post", ""},
		// Ordinary invocations that must still reach their RunE.
		{"me"}, {"me", "--help"},
		{"lists", "members", "list", "--help"},
		{"lists", "members", "list"},
		{"post", "create", "--text", "hello"},
		{"post", "search", "--limit", "10"},
	}

	helpShapes, ranShapes := 0, 0
	for _, args := range shapes {
		t.Run(fmt.Sprintf("%q", args), func(t *testing.T) {
			var dryRan []string
			res, err := Resolve(recordingTree(&dryRan), args)
			if err != nil {
				t.Fatalf("Resolve(%q) returned error: %v", args, err)
			}
			// The dry run must stay dry for every shape, not just the
			// two pinned by TestResolveNeverRuns.
			if len(dryRan) != 0 {
				t.Fatalf("dry-run resolve of %q executed %q", args, dryRan)
			}

			var realRan []string
			root := recordingTree(&realRan)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			// SetArgs(nil) makes cobra fall back to os.Args[1:], which
			// under `go test` is the test binary's own flags.
			if args == nil {
				root.SetArgs([]string{})
			} else {
				root.SetArgs(args)
			}
			_, _ = root.ExecuteC()

			if res.Help {
				helpShapes++
				if len(realRan) != 0 {
					t.Errorf("Resolve reported Help for %q, but real cobra ran %q", args, realRan)
				}
			}
			if len(realRan) != 0 {
				ranShapes++
			}
		})
	}

	// Without these the property above could hold vacuously — either
	// because nothing ever resolves to Help, or because the harness cannot
	// observe execution at all.
	if helpShapes == 0 {
		t.Error("no shape resolved to Help; the property held vacuously")
	}
	if ranShapes == 0 {
		t.Error("no shape reached a RunE; the harness cannot observe execution")
	}
}
