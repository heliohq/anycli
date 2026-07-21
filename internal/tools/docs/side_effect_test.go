package docs

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// sideEffectAnnotation mirrors the key read by the root-package Inspect API
// (design 318). "true" ⇔ the command may issue a mutating provider API call
// under some input; "false" ⇔ read-only under all inputs.
const sideEffectAnnotation = "anycli.side_effect"

// TestSideEffectAnnotations pins the side-effect fact of every runnable leaf
// command (leaf ⇔ no subcommands) and asserts group commands carry no
// annotation. The expected values follow the may-mutate criterion: any
// non-GET provider call under any input ⇒ "true" (own-document mutations
// included); GET-only paths ⇒ "false".
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"documents get":          "false", // GET /documents/{id}
		"documents create":       "true",  // POST /documents (+ optional batchUpdate for --body-file)
		"documents append":       "true",  // POST /documents/{id}:batchUpdate (GET only to compute the index)
		"documents replace-all":  "true",  // POST /documents/{id}:batchUpdate (replaceAllText)
		"documents batch-update": "true",  // POST /documents/{id}:batchUpdate (raw escape hatch)
	}

	s := &Service{}
	root := s.NewCommandTree()
	seen := map[string]bool{}
	walkCommands(root, func(cmd *cobra.Command) {
		path := commandPath(root, cmd)
		if cmd.HasSubCommands() {
			// Group command (root included): must not carry the annotation.
			if _, ok := cmd.Annotations[sideEffectAnnotation]; ok {
				t.Errorf("group command %q must not carry %s", path, sideEffectAnnotation)
			}
			return
		}
		seen[path] = true
		got, ok := cmd.Annotations[sideEffectAnnotation]
		if !ok {
			t.Errorf("runnable leaf %q is missing the %s annotation", path, sideEffectAnnotation)
			return
		}
		expected, known := want[path]
		if !known {
			t.Errorf("unexpected leaf %q in command tree — add it to the expectation table", path)
			return
		}
		if got != expected {
			t.Errorf("leaf %q: %s = %q, want %q", path, sideEffectAnnotation, got, expected)
		}
	})
	for path := range want {
		if !seen[path] {
			t.Errorf("expected leaf %q not found in command tree", path)
		}
	}
}

// walkCommands visits cmd and every descendant.
func walkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, sub := range cmd.Commands() {
		walkCommands(sub, visit)
	}
}

// commandPath returns cmd's space-joined path below root ("" for root
// itself), e.g. "documents get".
func commandPath(root, cmd *cobra.Command) string {
	full := cmd.CommandPath() // e.g. "docs documents get"
	return strings.TrimPrefix(strings.TrimPrefix(full, root.Name()), " ")
}
