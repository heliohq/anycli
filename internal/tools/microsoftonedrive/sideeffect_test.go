package microsoftonedrive

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// sideEffectAnnotation mirrors the key Inspect reads (design 318 §anycli
// Inspect); the root anycli package is not importable from internal/tools.
const sideEffectAnnotation = "anycli.side_effect"

// TestSideEffectAnnotations pins every runnable leaf command's
// anycli.side_effect annotation to the may-mutate verdict (design 318): true
// iff the command can issue a mutating provider API call under any input.
// The walk also asserts exact coverage (no unannotated leaf, no stale table
// row) and that group commands carry no annotation.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"items list":   "false", // GET children
		"items get":    "false", // GET item metadata
		"items mkdir":  "true",  // POST children (folder)
		"items move":   "true",  // PATCH parentReference
		"items rename": "true",  // PATCH name
		"items share":  "true",  // POST createLink
		"items delete": "true",  // DELETE item
		"search":       "false", // GET drive search
		"download":     "false", // GET metadata + content
		"upload":       "true",  // PUT content / POST createUploadSession
	}

	svc := &Service{}
	root := svc.NewCommandTree()
	seen := map[string]bool{}
	walkCommands(root, func(cmd *cobra.Command) {
		path := strings.TrimPrefix(cmd.CommandPath(), root.Name())
		path = strings.TrimPrefix(path, " ")
		if cmd.HasSubCommands() {
			if _, ok := cmd.Annotations[sideEffectAnnotation]; ok {
				t.Errorf("group command %q must not carry %s", path, sideEffectAnnotation)
			}
			return
		}
		got, ok := cmd.Annotations[sideEffectAnnotation]
		if !ok {
			t.Errorf("leaf command %q is missing the %s annotation", path, sideEffectAnnotation)
			return
		}
		wantVal, ok := want[path]
		if !ok {
			t.Errorf("leaf command %q is not in the expectation table — classify it", path)
			return
		}
		seen[path] = true
		if got != wantVal {
			t.Errorf("leaf command %q: %s = %q, want %q", path, sideEffectAnnotation, got, wantVal)
		}
	})
	for path := range want {
		if !seen[path] {
			t.Errorf("expected leaf command %q not found in the tree (stale table row?)", path)
		}
	}
}

// walkCommands visits every command in the tree below (and excluding) root.
func walkCommands(root *cobra.Command, visit func(*cobra.Command)) {
	for _, c := range root.Commands() {
		visit(c)
		walkCommands(c, visit)
	}
}
