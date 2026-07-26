package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/heliohq/anycli/internal/credential"
	"github.com/heliohq/anycli/internal/registry"
	"github.com/heliohq/anycli/internal/tools"
)

// Prose the test tree carries, asserted verbatim so a test that "passes" on
// the wrong node cannot go unnoticed: each string appears on exactly one node.
const (
	rootLong   = "PROBE-ROOT-PROSE: tool-wide conventions live here."
	postLong   = "PROBE-POST-PROSE: the post noun groups create and search."
	searchLong = "PROBE-SEARCH-PROSE: searches the last 7 days; --limit floor is 10."
	deepLong   = "PROBE-DEEP-PROSE: the third level down, four tokens from the tool name."
)

// erroringResolver stands in for an unconnected tool: every resolution fails,
// exactly as it does when the host has no connection for the tool.
type erroringResolver struct {
	calls int
}

func (r *erroringResolver) Resolve(context.Context, credential.Tool, string) (*credential.Credential, error) {
	r.calls++
	return nil, fmt.Errorf("no connection for this tool")
}

// treeService is a service tool with a realistic grouped command tree: the
// callable commands live one to three levels below the root, one leaf takes a
// string flag whose VALUE can be the literal "--help", and every node carries
// distinguishable prose.
//
// build overrides that default tree for the cases that need a different SHAPE
// (see siblingGroupTree); nil keeps the default.
type treeService struct {
	build    func() *cobra.Command
	executed [][]string
	env      []map[string]string
}

func (s *treeService) Execute(_ context.Context, args []string, env map[string]string) (tools.ExecutionResult, error) {
	s.executed = append(s.executed, args)
	s.env = append(s.env, env)
	return tools.ExecutionResult{}, nil
}

func (s *treeService) NewCommandTree() *cobra.Command {
	if s.build != nil {
		return s.build()
	}
	run := func(*cobra.Command, []string) error { return nil }

	root := &cobra.Command{Use: "probe", Short: "Probe built-in service", Long: rootLong}
	root.PersistentFlags().Bool("json", false, "single-result JSON")

	create := &cobra.Command{Use: "create", Short: "Create a post", RunE: run}
	create.Flags().String("text", "", "post body")
	search := &cobra.Command{Use: "search", Short: "Search recent posts (one page)", Long: searchLong, RunE: run}
	post := &cobra.Command{Use: "post", Short: "Posts", Long: postLong}
	post.AddCommand(create, search)

	// Three levels below the root, so "cmd1 cmd2 --help" has somewhere
	// deeper to resolve to than the first group.
	members := &cobra.Command{Use: "members", Short: "List members"}
	members.AddCommand(&cobra.Command{Use: "list", Short: "List the members of a list", Long: deepLong, RunE: run})
	lists := &cobra.Command{Use: "lists", Short: "Lists"}
	lists.AddCommand(members)

	root.AddCommand(post, lists, &cobra.Command{Use: "me", Short: "Show the connected user", RunE: run})
	return root
}

// Prose for the sibling-group tree, likewise unique per node.
const (
	siblingPostListLong = "PROBE-SIBLING-POST-LIST: the leaf the caller actually asked about."
	siblingGroupLong    = "PROBE-SIBLING-GROUP: an unrelated top-level group that shares a name."
)

// siblingGroupTree is the shape that makes a LEADING `--help` re-resolve into a
// different top-level node: a `list` group at the root AND a `post list` leaf.
//
// `--help post list` loses "post" to stripFlags; the surviving "list" then
// matches the top-level `list` GROUP, so argv resolves there with "post" left
// over as a positional. "post" is not a child of `list`, so a
// children-of-the-resolved-node check passes and the note fires — denying
// `post`, a command that plainly exists, right above an unrelated group's help.
// This is the real-corpus shape (`activecampaign --help contact list`).
func siblingGroupTree() *cobra.Command {
	run := func(*cobra.Command, []string) error { return nil }

	root := &cobra.Command{Use: "probe", Short: "Probe built-in service"}
	post := &cobra.Command{Use: "post", Short: "Posts"}
	post.AddCommand(&cobra.Command{Use: "list", Short: "List posts", Long: siblingPostListLong, RunE: run})

	list := &cobra.Command{Use: "list", Short: "Audience lists", Long: siblingGroupLong}
	list.AddCommand(&cobra.Command{Use: "get", Short: "Retrieve one list", RunE: run})

	root.AddCommand(post, list)
	return root
}

// captureHelp redirects the help output into a buffer for the test's duration.
func captureHelp(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := helpOut
	helpOut = &buf
	t.Cleanup(func() { helpOut = orig })
	return &buf
}

// TestExecute_HelpFollowsASwappedStdout pins the ONE thing the helpOut hook
// cannot: that production help resolves os.Stdout at call time.
//
// Every other test here injects the sink through helpOut, so all of them would
// still pass with the sink bound at package init — and internal/e2e's
// captureOutput does not use the hook. It swaps the os.Stdout GLOBAL for an
// os.Pipe around engine.ExecuteWith and returns what the pipe received. An
// init-time `var helpOut io.Writer = os.Stdout` holds the *os.File from before
// the swap, so `e2e.RunTool(t, "<tool>", "--help")` would return "" while the
// whole face went to the original fd and leaked onto the CI log — a silent
// absence read as a fact, which is the exact failure mode design 331 exists to
// remove. So this test reproduces the harness's mechanism rather than the hook.
func TestExecute_HelpFollowsASwappedStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	svc := credentialedService(t, "probe-stdout-swap")
	engine, _ := newTestEngine(t)
	resolver := &erroringResolver{}

	// Production shape: no hook installed, so helpWriter must reach for the
	// global itself.
	orig := helpOut
	helpOut = nil
	t.Cleanup(func() { helpOut = orig })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	drained := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		r.Close()
		drained <- string(b)
	}()

	oldStdout := os.Stdout
	os.Stdout = w
	exitCode, execErr := engine.Execute(context.Background(), "probe-stdout-swap", []string{"--help"}, resolver, "")
	os.Stdout = oldStdout
	w.Close()
	got := <-drained

	if execErr != nil {
		t.Fatalf("Execute returned error: %v", execErr)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if len(svc.executed) != 0 {
		t.Errorf("service Execute ran %d times, want 0", len(svc.executed))
	}
	if !strings.Contains(got, "COMMANDS (4 — complete list)") {
		t.Fatalf("help face did not reach the swapped os.Stdout (captured %d bytes): %q", len(got), got)
	}
}

// credentialedService installs a definition with credential bindings plus a
// service implementation carrying the default probe tree.
func credentialedService(t *testing.T, tool string) *treeService {
	t.Helper()
	return credentialedTree(t, tool, nil)
}

// credentialedTree is credentialedService with an explicit tree builder (nil =
// the default probe tree).
func credentialedTree(t *testing.T, tool string, build func() *cobra.Command) *treeService {
	t.Helper()
	useDefinitions(t, map[string]*registry.Definition{
		tool: {
			Name: tool,
			Type: "service",
			Auth: &registry.AuthConfig{Credentials: []registry.CredentialBinding{
				{
					Source: registry.CredentialSource{Field: "access_token"},
					Inject: registry.CredentialInject{Type: "env", EnvVar: "PROBE_TOKEN"},
				},
			}},
		},
	})
	svc := &treeService{build: build}
	tools.RegisterService(tool, svc)
	return svc
}

// TestExecute_ServiceHelpDiscrimination is the core table: for every argv
// shape, is this a help request, and was the credential resolver called?
//
// The discriminator is cobra's own parse (internal/dryrun), never a scan of
// argv for the token "--help" — which is why the last four rows behave the way
// they do. `--text "--help"` and `-- --help` are real invocations whose
// arguments happen to contain the string; `--help=false` explicitly disables
// help. A scanning predicate would answer "help" to all three and silently
// swallow a post the user meant to publish.
func TestExecute_ServiceHelpDiscrimination(t *testing.T) {
	cases := []struct {
		name string
		args []string
		// wantHelp: help printed, exit 0, no credential resolution, no
		// service Execute. false: the normal path, resolver called.
		wantHelp bool
		// contains / notContains are checked against the captured help
		// output (only meaningful when wantHelp).
		contains    []string
		notContains []string
		// tree overrides the default probe tree for the cases that need a
		// different command SHAPE; nil keeps the default.
		tree func() *cobra.Command
	}{
		{
			name:     "bare long help renders the flattened root face",
			args:     []string{"--help"},
			wantHelp: true,
			// 4 leaves: post create, post search, lists members list, me.
			contains:    []string{"COMMANDS (4 — complete list)", "post search", "lists members list", "me", rootLong, "--json"},
			notContains: []string{searchLong, "note:"},
		},
		{
			name:        "bare short help renders the same face",
			args:        []string{"-h"},
			wantHelp:    true,
			contains:    []string{"COMMANDS (4 — complete list)", "post search"},
			notContains: []string{"note:"},
		},
		{
			name:     "group node help is cobra's own, not the root face",
			args:     []string{"post", "--help"},
			wantHelp: true,
			contains: []string{postLong, "create", "search"},
			// The exhaustiveness claim belongs to the root face only; a
			// group's cobra help lists direct children, so claiming
			// completeness over it would be the original incident.
			notContains: []string{"complete list", rootLong, "note:"},
		},
		{
			name:     "leaf help prints the leaf Long",
			args:     []string{"post", "search", "--help"},
			wantHelp: true,
			contains: []string{searchLong, "probe post search"},
			// This is THE case the string predicate got wrong: positional
			// args ahead of --help made it "not help-only", so it fell
			// through to credential resolution and an unconnected tool
			// could never read the prose.
			notContains: []string{"complete list", postLong, "note:"},
		},
		{
			name:        "three levels down resolves to the deepest node",
			args:        []string{"lists", "members", "list", "--help"},
			wantHelp:    true,
			contains:    []string{deepLong, "probe lists members list"},
			notContains: []string{"complete list", "note:"},
		},
		{
			name:     "unknown subcommand falls back to the parent with a note",
			args:     []string{"post", "bogus", "--help"},
			wantHelp: true,
			contains: []string{
				`note: "bogus" is not a command under post; showing help for post.`,
				postLong,
			},
			notContains: []string{"complete list"},
		},
		{
			name:     "unknown top-level subcommand notes against the tool name",
			args:     []string{"bogus", "--help"},
			wantHelp: true,
			contains: []string{
				`note: "bogus" is not a command under probe; showing help for probe.`,
				"COMMANDS (4 — complete list)",
			},
		},
		// The next four shapes all leave a positional behind at a node WITH
		// subcommands, which is the note's trigger — but in every one of them
		// that leftover token names a command that really exists. cobra's
		// stripFlags runs inside Find before the help flag is registered, so a
		// bare `--help` swallows the following token during the subcommand
		// search and it resurfaces as a positional. Noting it would print a
		// false-absence claim directly above output listing the very command
		// it denied.
		{
			name:     "a help flag mid-path does not deny the command it swallowed",
			args:     []string{"post", "--help", "search"},
			wantHelp: true,
			contains: []string{postLong, "search"},
			// The note would read: "search" is not a command under post —
			// printed immediately above the line that lists `search`.
			notContains: []string{"note:"},
		},
		{
			name:     "a leading help flag does not deny a real top-level command",
			args:     []string{"--help", "post", "search"},
			wantHelp: true,
			contains: []string{"COMMANDS (4 — complete list)", "post search"},
			// The note would read: "post" is not a command under probe.
			notContains: []string{"note:"},
		},
		{
			name:     "a help flag mid-path holds at three levels too",
			args:     []string{"lists", "--help", "members", "list"},
			wantHelp: true,
			contains: []string{"members"},
			// The note would read: "members" is not a command under lists.
			notContains: []string{"note:", "complete list"},
		},
		{
			// The children-of-the-resolved-node guard is NOT enough: a
			// leading `--help` can steal the first path token and let the
			// survivor re-resolve into an unrelated top-level group, where
			// the stolen token legitimately is not a child. Real corpus
			// shape: `activecampaign --help contact list` denied `contact`
			// while `activecampaign contact list` exists.
			name:     "a leading help flag does not deny a command after re-resolving into a sibling group",
			args:     []string{"--help", "post", "list"},
			tree:     siblingGroupTree,
			wantHelp: true,
			// Resolution lands on the top-level `list` group, so its own
			// cobra help is what prints.
			contains: []string{siblingGroupLong, "get"},
			// The note would read: "post" is not a command under list —
			// while `post list` is right there in the same tree.
			notContains: []string{"note:", siblingPostListLong},
		},
		// cobra injects `help` and `completion` into the root inside
		// Execute, so the freshly built dry-run tree does not have them —
		// but `x help post` and `x completion bash` are real, working
		// invocations. Denying them is a false-absence claim about a
		// command the caller can successfully run.
		{
			name:        "cobra's injected help command is not denied",
			args:        []string{"help", "--help"},
			wantHelp:    true,
			contains:    []string{"COMMANDS (4 — complete list)"},
			notContains: []string{"note:"},
		},
		{
			name:        "cobra's injected completion command is not denied",
			args:        []string{"completion", "--help"},
			wantHelp:    true,
			contains:    []string{"COMMANDS (4 — complete list)"},
			notContains: []string{"note:"},
		},
		{
			name:     "an empty leftover token is not a subcommand claim",
			args:     []string{"", "--help"},
			wantHelp: true,
			contains: []string{"COMMANDS (4 — complete list)"},
			// "" names nothing and is a typo of nothing; the note has no
			// correction to offer, only noise.
			notContains: []string{"note:"},
		},
		{
			name:     "a leaf positional is an argument, not a path typo",
			args:     []string{"post", "search", "kittens", "--help"},
			wantHelp: true,
			contains: []string{searchLong},
			// "kittens" is search's own positional argument. Calling it
			// "not a command" would be the false signal in the other
			// direction.
			notContains: []string{"note:"},
		},
		{
			name:        "help beside another flag is still help",
			args:        []string{"--json", "--help"},
			wantHelp:    true,
			contains:    []string{"COMMANDS (4 — complete list)"},
			notContains: []string{"note:"},
		},
		{
			name:     "help after another flag is still help",
			args:     []string{"--help", "--json"},
			wantHelp: true,
			contains: []string{"COMMANDS (4 — complete list)"},
		},
		{
			name:     "the literal string --help as a flag VALUE is a real post",
			args:     []string{"post", "create", "--text", "--help"},
			wantHelp: false,
		},
		{
			name:     "everything after -- is positional, including --help",
			args:     []string{"post", "create", "--", "--help"},
			wantHelp: false,
		},
		{
			name:     "help explicitly disabled is not a help request",
			args:     []string{"--help=false"},
			wantHelp: false,
		},
		{
			name:     "no args is a normal invocation",
			args:     nil,
			wantHelp: false,
		},
		{
			name:     "a plain command path is a normal invocation",
			args:     []string{"post", "search", "--limit", "10"},
			wantHelp: false,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := fmt.Sprintf("probe-table-%d", i)
			svc := credentialedTree(t, tool, tc.tree)
			out := captureHelp(t)
			engine, _ := newTestEngine(t)
			resolver := &erroringResolver{}

			exitCode, err := engine.Execute(context.Background(), tool, tc.args, resolver, "")
			got := out.String()

			if !tc.wantHelp {
				if err == nil {
					t.Fatalf("expected the normal path to fail credential resolution; exit=%d out=%q", exitCode, got)
				}
				if exitCode != 1 {
					t.Errorf("exit code = %d, want 1", exitCode)
				}
				if resolver.calls == 0 {
					t.Error("credential resolver was NOT called — a real invocation was mistaken for help")
				}
				if got != "" {
					t.Errorf("help rendered for a non-help invocation:\n%s", got)
				}
				if len(svc.executed) != 0 {
					t.Errorf("service Execute ran despite failed credentials: %v", svc.executed)
				}
				return
			}

			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if exitCode != 0 {
				t.Errorf("exit code = %d, want 0", exitCode)
			}
			if resolver.calls != 0 {
				t.Errorf("credential resolver called %d times, want 0 — help must work before a tool is connected", resolver.calls)
			}
			if len(svc.executed) != 0 {
				t.Errorf("service Execute ran %d times, want 0 — help is answered off the command tree", len(svc.executed))
			}
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("help output missing %q:\n%s", want, got)
				}
			}
			for _, unwanted := range tc.notContains {
				if strings.Contains(got, unwanted) {
					t.Errorf("help output unexpectedly contains %q:\n%s", unwanted, got)
				}
			}
		})
	}
}

// TestExecute_UnconnectedToolReadsLeafProse is the regression this change
// exists for (design 331 D1+D3): the per-command Long is where the provider
// API knowledge lives and `<leaf> --help` is the only way to read it, so a
// tool the user has not connected must still be able to answer it. Before the
// fix, `post search --help` had positional args ahead of the flag, failed the
// help-only string predicate, resolved credentials, and died.
func TestExecute_UnconnectedToolReadsLeafProse(t *testing.T) {
	svc := credentialedService(t, "probe-unconnected")
	out := captureHelp(t)
	engine, _ := newTestEngine(t)
	resolver := &erroringResolver{}

	exitCode, err := engine.Execute(context.Background(), "probe-unconnected", []string{"post", "search", "--help"}, resolver, "")
	if err != nil {
		t.Fatalf("Execute returned error for an unconnected tool: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if resolver.calls != 0 {
		t.Errorf("credential resolver called %d times, want 0", resolver.calls)
	}
	if len(svc.executed) != 0 {
		t.Errorf("service Execute ran %d times, want 0", len(svc.executed))
	}
	if got := out.String(); !strings.Contains(got, searchLong) {
		t.Fatalf("leaf prose missing for an unconnected tool:\n%s", got)
	}
}

// TestExecute_RealInvocationKeepsCredentialError: the help short-circuit must
// not blunt the failure an actually-unconnected real call has to produce.
func TestExecute_RealInvocationKeepsCredentialError(t *testing.T) {
	svc := credentialedService(t, "probe-real")
	out := captureHelp(t)
	engine, _ := newTestEngine(t)
	resolver := &erroringResolver{}

	exitCode, err := engine.Execute(context.Background(), "probe-real", []string{"post", "search", "--limit", "10"}, resolver, "")
	if err == nil {
		t.Fatal("expected a credential resolution error")
	}
	if !strings.Contains(err.Error(), `credential resolution failed for "probe-real"`) {
		t.Errorf("error lost its shape: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if resolver.calls == 0 {
		t.Error("credential resolver was not called")
	}
	if len(svc.executed) != 0 {
		t.Errorf("service Execute ran despite failed credentials: %v", svc.executed)
	}
	if out.Len() != 0 {
		t.Errorf("help rendered for a real invocation:\n%s", out.String())
	}
}

// TestExecute_HelpFaceIsConnectionIndependent: the face is the same whether or
// not the tool is connected, so an AI never sees two shapes.
func TestExecute_HelpFaceIsConnectionIndependent(t *testing.T) {
	svc := credentialedService(t, "probe-connected")
	out := captureHelp(t)
	engine, _ := newTestEngine(t)
	resolver := fixedResolver{data: map[string]string{"access_token": "token"}}

	exitCode, err := engine.Execute(context.Background(), "probe-connected", []string{"--help"}, resolver, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if len(svc.executed) != 0 {
		t.Errorf("service Execute ran %d times, want 0", len(svc.executed))
	}
	if !strings.Contains(out.String(), "COMMANDS (4 — complete list)") {
		t.Errorf("connected tool got a different help face:\n%s", out.String())
	}
}

// TestExecute_BinaryPassthroughNeverReachesTheTreePath: github (gh) and lark
// (lark-cli) have no in-process cobra tree — anycli downloads the pinned
// release and passes through, so their help is printed by that binary. AnyCLI
// must not render a face (and therefore never stamps an exhaustiveness claim
// on output it did not produce, design 331 D2), and must still skip credential
// resolution so a bare `--help` is reachable before the tool is connected.
func TestExecute_BinaryPassthroughNeverReachesTheTreePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	cases := []struct {
		name        string
		args        []string
		wantResolve bool
	}{
		{name: "bare long help skips credentials", args: []string{"--help"}, wantResolve: false},
		{name: "bare short help skips credentials", args: []string{"-h"}, wantResolve: false},
		{name: "repeated help flags skip credentials", args: []string{"--help", "-h"}, wantResolve: false},
		// Without a tree there is nothing that can tell these from a real
		// invocation, so they keep the normal credential path.
		{name: "subcommand help takes the normal path", args: []string{"pr", "list", "--help"}, wantResolve: true},
		{name: "help beside another flag takes the normal path", args: []string{"--help", "--json"}, wantResolve: true},
		{name: "no args takes the normal path", args: nil, wantResolve: true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupHome(t)
			truePath := trueBinary(t)
			tool := fmt.Sprintf("probe-passthrough-%d", i)
			useDefinitions(t, map[string]*registry.Definition{
				tool: {
					Name:    tool,
					Binary:  "true",
					Resolve: truePath,
					Auth: &registry.AuthConfig{Credentials: []registry.CredentialBinding{
						{
							Source: registry.CredentialSource{Field: "access_token"},
							Inject: registry.CredentialInject{Type: "env", EnvVar: "PROBE_TOKEN"},
						},
					}},
				},
			})
			out := captureHelp(t)
			engine, _ := newTestEngine(t)
			resolver := &erroringResolver{}

			exitCode, _ := engine.Execute(context.Background(), tool, tc.args, resolver, "")
			if tc.wantResolve {
				if resolver.calls == 0 {
					t.Error("credential resolver was not called on the normal passthrough path")
				}
				if exitCode != 1 {
					t.Errorf("exit code = %d, want 1", exitCode)
				}
			} else {
				if resolver.calls != 0 {
					t.Errorf("credential resolver called %d times, want 0", resolver.calls)
				}
				if exitCode != 0 {
					t.Errorf("exit code = %d, want 0", exitCode)
				}
			}
			if out.Len() != 0 {
				t.Fatalf("anycli rendered a help face for a binary-passthrough tool:\n%s", out.String())
			}
		})
	}
}
