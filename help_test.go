package anycli

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var claimRe = regexp.MustCompile(`COMMANDS \((\d+) — complete list\)`)

// TestRenderToolHelpClaimMatchesEveryTree walks every registered service tool
// and proves the exhaustiveness claim is derived, not written down: the number
// in the heading must equal the number of listed leaves, for all 150-odd
// trees. A hardcoded count anywhere would fail here.
func TestRenderToolHelpClaimMatchesEveryTree(t *testing.T) {
	names := ServiceTools()
	if len(names) == 0 {
		t.Fatal("no built-in service tools registered — registry seam broken")
	}
	for _, tool := range names {
		t.Run(tool, func(t *testing.T) {
			root, err := CommandTree(tool)
			if err != nil {
				t.Fatalf("command tree: %v", err)
			}
			var buf bytes.Buffer
			if err := RenderToolHelp(&buf, root); err != nil {
				t.Fatalf("render help: %v", err)
			}
			out := buf.String()
			match := claimRe.FindStringSubmatch(out)
			if match == nil {
				// Leaf-less trees fall back to cobra's own help and must
				// not carry a claim at all.
				if strings.Contains(out, "complete list") {
					t.Fatalf("malformed exhaustiveness claim:\n%s", out)
				}
				return
			}
			claimed, err := strconv.Atoi(match[1])
			if err != nil {
				t.Fatalf("unparsable claim %q: %v", match[1], err)
			}
			listed := countListedLeaves(out)
			if claimed != listed {
				t.Fatalf("claim says %d commands, %d listed", claimed, listed)
			}
			if claimed == 0 {
				t.Fatal("a zero-leaf claim must never be printed")
			}
		})
	}
}

// countListedLeaves counts the indented command rows between the COMMANDS
// heading and the trailing hint line.
func countListedLeaves(out string) int {
	n := 0
	inList := false
	for _, line := range strings.Split(out, "\n") {
		if claimRe.MatchString(line) {
			inList = true
			continue
		}
		if !inList {
			continue
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		if strings.HasPrefix(line, "  ") {
			n++
		}
	}
	return n
}

// TestRenderToolHelpSurfacesNestedLeaves is the incident regression from
// design 335: `x --help` used to advertise the noun "post" and hide the 36
// commands beneath it, so two agents concluded the X integration had no
// search. The flattened face must name the callable commands.
func TestRenderToolHelpSurfacesNestedLeaves(t *testing.T) {
	root, err := CommandTree("x")
	if err != nil {
		t.Fatalf("command tree: %v", err)
	}
	var buf bytes.Buffer
	if err := RenderToolHelp(&buf, root); err != nil {
		t.Fatalf("render help: %v", err)
	}
	out := buf.String()
	for _, leaf := range []string{"post search", "post replies", "user search", "timeline home"} {
		if !strings.Contains(out, leaf) {
			t.Errorf("leaf %q missing from the x capability face:\n%s", leaf, out)
		}
	}
	if !strings.Contains(out, "complete list") {
		t.Errorf("missing exhaustiveness claim:\n%s", out)
	}
}

// TestRenderToolHelpSurfacesRootFlags: the flattened face replaces cobra's
// help, which used to end with the root's Flags block. Almost every service
// root defines a persistent `--json`, and it is the single most load-bearing
// flag an AI consumer has — the root face is where it gets stated, since a
// reader that stops at the capability list never opens a leaf.
func TestRenderToolHelpSurfacesRootFlags(t *testing.T) {
	root, err := CommandTree("x")
	if err != nil {
		t.Fatalf("command tree: %v", err)
	}
	if root.LocalFlags().Lookup("json") == nil {
		t.Skip("x no longer defines a root --json flag")
	}
	var buf bytes.Buffer
	if err := RenderToolHelp(&buf, root); err != nil {
		t.Fatalf("render help: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\nFlags:\n") {
		t.Fatalf("no flag section on the capability face:\n%s", out)
	}
	if !strings.Contains(out, "--json") {
		t.Fatalf("root --json missing from the capability face:\n%s", out)
	}
}

// TestRenderToolHelpExcludesCobraInjectedCommands: `completion` and `help` are
// shell plumbing, and their presence in the old output was two of the eleven
// lines an agent had to reason over.
func TestRenderToolHelpExcludesCobraInjectedCommands(t *testing.T) {
	root, err := CommandTree("x")
	if err != nil {
		t.Fatalf("command tree: %v", err)
	}
	// Force cobra to inject both, as it does on a real Execute.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	var buf bytes.Buffer
	if err := RenderToolHelp(&buf, root); err != nil {
		t.Fatalf("render help: %v", err)
	}
	for _, line := range listedRows(buf.String()) {
		if strings.HasPrefix(line, "help") || strings.HasPrefix(line, "completion") {
			t.Errorf("cobra-injected command listed as a capability: %q", line)
		}
	}
}

// TestRenderToolHelpExcludesHiddenLeaves keeps the ops/onboarding surface off
// the AI-callable face (design 335 D2).
func TestRenderToolHelpExcludesHiddenLeaves(t *testing.T) {
	root := &cobra.Command{Use: "probe", Short: "Probe"}
	visible := &cobra.Command{Use: "list", Short: "List things", RunE: func(*cobra.Command, []string) error { return nil }}
	hidden := &cobra.Command{Use: "secret", Short: "Ops only", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(visible, hidden)

	var buf bytes.Buffer
	if err := RenderToolHelp(&buf, root); err != nil {
		t.Fatalf("render help: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "COMMANDS (1 — complete list)") {
		t.Fatalf("hidden leaf leaked into the claim:\n%s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("hidden leaf listed:\n%s", out)
	}
}

// listedRows returns the trimmed command rows of a rendered capability face.
func listedRows(out string) []string {
	var rows []string
	inList := false
	for _, line := range strings.Split(out, "\n") {
		if claimRe.MatchString(line) {
			inList = true
			continue
		}
		if !inList {
			continue
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		rows = append(rows, strings.TrimSpace(line))
	}
	return rows
}
