package notion

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations pins the anycli.side_effect fact on every runnable
// leaf of the command tree (design 318 may-mutate criterion) and asserts group
// commands carry no annotation. Notable calls: `search` (POST /search) and
// `data-source query` (POST /data_sources/{id}/query) are POST-shaped documented
// read-only lookups → false; every page/db/view/comment write is POST/PATCH →
// true.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		// top-level (read-only)
		"search": "false", // POST /search — documented lookup
		"fetch":  "false", // GET page markdown / database / data source
		// page (all mutate)
		"page create":    "true", // POST /pages
		"page update":    "true", // PATCH /pages/{id} + /pages/{id}/markdown
		"page replace":   "true", // PATCH /pages/{id}/markdown
		"page edit":      "true", // PATCH /pages/{id}/markdown
		"page insert":    "true", // PATCH /pages/{id}/markdown
		"page append":    "true", // PATCH /pages/{id}/markdown
		"page move":      "true", // POST /pages/{id}/move, PATCH /databases/{id}
		"page duplicate": "true", // GET source + POST /pages
		// db / data-source
		"db create":          "true",  // POST /databases
		"data-source query":  "false", // POST query — documented read-only
		"data-source update": "true",  // PATCH /data_sources/{id}
		// view
		"view create": "true", // POST /views
		"view update": "true", // PATCH /views/{id}
		// comment
		"comment create": "true",  // POST /comments
		"comment list":   "false", // GET /comments
		// user / task (all GET)
		"user get": "false",
		"task get": "false",
		// file (both create provider-side objects)
		"file upload": "true", // POST /file_uploads (+ send parts, complete)
		"file attach": "true", // PATCH page/block with the uploaded file
		// raw API escape hatch: method is runtime input — may-mutate
		"api": "true",
	}

	seen := map[string]string{}
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		if cmd.HasSubCommands() {
			if _, ok := cmd.Annotations["anycli.side_effect"]; ok {
				t.Errorf("group command %q must not carry anycli.side_effect", strings.Join(path, " "))
			}
			for _, sub := range cmd.Commands() {
				walk(sub, append(path, sub.Name()))
			}
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		key := strings.Join(path, " ")
		got, ok := cmd.Annotations["anycli.side_effect"]
		if !ok {
			t.Errorf("leaf %q missing explicit anycli.side_effect annotation", key)
			return
		}
		seen[key] = got
	}
	root := (&Service{}).NewCommandTree()
	walk(root, nil)

	for key, wantVal := range want {
		got, ok := seen[key]
		if !ok {
			t.Errorf("expected leaf %q not found in command tree", key)
			continue
		}
		if got != wantVal {
			t.Errorf("leaf %q: anycli.side_effect = %q, want %q", key, got, wantVal)
		}
	}
	for key := range seen {
		if _, ok := want[key]; !ok {
			t.Errorf("leaf %q present in tree but missing from expectation table — classify it", key)
		}
	}
}
