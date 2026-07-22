package slides

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
// non-GET provider call under any input ⇒ "true" (own-deck mutations
// included); GET-only paths (including local thumbnail download) ⇒ "false".
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"presentations get":    "false", // GET /presentations/{id}
		"presentations create": "true",  // POST /presentations
		"pages get":            "false", // GET /presentations/{id}/pages/{pageId}
		"pages thumbnail":      "false", // GET .../thumbnail + GET contentUrl (local save only)
		"slides add":           "true",  // POST batchUpdate (createSlide + insertText)
		"slides duplicate":     "true",  // POST batchUpdate (duplicateObject)
		"slides move":          "true",  // POST batchUpdate (updateSlidesPosition)
		"slides delete":        "true",  // POST batchUpdate (deleteObject)
		"text insert":          "true",  // POST batchUpdate (insertText; GET only for --append index)
		"text replace":         "true",  // POST batchUpdate (replaceAllText)
		"text delete":          "true",  // POST batchUpdate (deleteText)
		"images insert":        "true",  // POST batchUpdate (createImage)
		"elements delete":      "true",  // POST batchUpdate (deleteObject)
		"batch-update":         "true",  // POST /presentations/{id}:batchUpdate (raw escape hatch)
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
// itself), e.g. "text replace".
func commandPath(root, cmd *cobra.Command) string {
	full := cmd.CommandPath() // e.g. "slides text replace"
	return strings.TrimPrefix(strings.TrimPrefix(full, root.Name()), " ")
}
