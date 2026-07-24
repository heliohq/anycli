package linkedin

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations pins the anycli.side_effect fact on every runnable
// leaf of the command tree (design 318 may-mutate criterion) and asserts group
// commands carry no annotation.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"post create":  "true",  // POST /rest/posts
		"me":           "false", // GET /v2/userinfo
		"video upload": "true",  // POST /rest/videos?action=initializeUpload|finalizeUpload
		"video get":    "false", // GET /rest/videos/{urn}
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
