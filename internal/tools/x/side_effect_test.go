package x

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const sideEffectAnnotation = "anycli.side_effect"

// TestSideEffectAnnotationValues pins the anycli.side_effect fact of every
// runnable leaf command per the design-318 may-mutate criterion: true iff the
// command can issue a non-GET provider API call under some input.
func TestSideEffectAnnotationValues(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"me", "false"},
		{"user get", "false"},
		{"user search", "false"},
		{"post get", "false"},
		{"post search", "false"},
		{"post create", "true"},
		{"post reply", "true"},
		{"post thread", "true"},
		{"post delete", "true"},
		{"post quote", "true"},   // creates a new public quote post
		{"post quotes", "false"}, // reads quote posts of a post
		{"post replies", "false"},
		{"post hide", "true"}, // moderation write (PUT hidden=true)
		{"post unhide", "true"},
		{"post liking-users", "false"},
		{"post reposters", "false"},
		{"user followers", "false"},
		{"user following", "false"},
		{"follow create", "true"},
		{"follow delete", "true"},
		{"like create", "true"},
		{"like delete", "true"},
		{"timeline user", "false"},
		{"timeline mentions", "false"},
		{"timeline home", "false"},
		{"repost create", "true"},
		{"repost delete", "true"},
		{"media upload", "true"},
		{"media status", "false"},
		{"media metadata", "true"},
		{"dm list", "false"},
		{"dm get", "false"},
		{"dm history", "false"},
		{"dm send", "true"},
		{"dm delete", "true"},
		{"dm conversation create", "true"},
		{"dm media download", "false"},
	}

	root := (&Service{}).NewCommandTree()
	seen := make(map[string]bool, len(cases))
	for _, tc := range cases {
		t.Run(strings.ReplaceAll(tc.path, " ", "_"), func(t *testing.T) {
			cmd, rest, err := root.Find(strings.Split(tc.path, " "))
			if err != nil {
				t.Fatalf("Find(%q): %v", tc.path, err)
			}
			if len(rest) != 0 {
				t.Fatalf("Find(%q) left unresolved tokens %v", tc.path, rest)
			}
			if cmd.HasSubCommands() {
				t.Fatalf("%q resolved to a group command, want a leaf", tc.path)
			}
			got, ok := cmd.Annotations[sideEffectAnnotation]
			if !ok {
				t.Fatalf("%q has no %s annotation", tc.path, sideEffectAnnotation)
			}
			if got != tc.want {
				t.Errorf("%q %s = %q, want %q", tc.path, sideEffectAnnotation, got, tc.want)
			}
		})
		seen[tc.path] = true
	}

	// The case table must cover the whole tree: every runnable leaf appears
	// exactly once, so a new command cannot land without a pinned verdict.
	walkLeaves(root, nil, func(path []string, cmd *cobra.Command) {
		joined := strings.Join(path, " ")
		if !seen[joined] {
			t.Errorf("runnable leaf %q is missing from the case table", joined)
		}
	})
}

// TestSideEffectAnnotationHygiene enforces the design-318 annotation contract
// on the x tree: every runnable leaf carries an explicit "true"/"false"
// annotation, and no group command carries one.
func TestSideEffectAnnotationHygiene(t *testing.T) {
	root := (&Service{}).NewCommandTree()
	walk(root, nil, func(path []string, cmd *cobra.Command) {
		joined := strings.Join(path, " ")
		if cmd.HasSubCommands() {
			if _, ok := cmd.Annotations[sideEffectAnnotation]; ok {
				t.Errorf("group command %q must not carry %s", joined, sideEffectAnnotation)
			}
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		got, ok := cmd.Annotations[sideEffectAnnotation]
		if !ok {
			t.Errorf("runnable leaf %q lacks an explicit %s annotation", joined, sideEffectAnnotation)
			return
		}
		if got != "true" && got != "false" {
			t.Errorf("runnable leaf %q has %s = %q, want \"true\" or \"false\"", joined, sideEffectAnnotation, got)
		}
	})
}

func walk(cmd *cobra.Command, path []string, visit func(path []string, cmd *cobra.Command)) {
	for _, child := range cmd.Commands() {
		childPath := append(append([]string(nil), path...), child.Name())
		visit(childPath, child)
		walk(child, childPath, visit)
	}
}

func walkLeaves(root *cobra.Command, path []string, visit func(path []string, cmd *cobra.Command)) {
	walk(root, path, func(p []string, cmd *cobra.Command) {
		if cmd.HasSubCommands() || (cmd.RunE == nil && cmd.Run == nil) {
			return
		}
		visit(p, cmd)
	})
}
