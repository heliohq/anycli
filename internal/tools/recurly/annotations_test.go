package recurly

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations pins the anycli.side_effect fact on every runnable
// leaf (design 318): reads are "false", the curated lifecycle writes are "true".
// Group commands must not carry the annotation.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"recurly account list":           "false",
		"recurly account get":            "false",
		"recurly account balance":        "false",
		"recurly account billing-info":   "false",
		"recurly subscription list":      "false",
		"recurly subscription get":       "false",
		"recurly subscription create":    "true",
		"recurly subscription change":    "true",
		"recurly subscription cancel":    "true",
		"recurly subscription pause":     "true",
		"recurly subscription resume":    "true",
		"recurly subscription terminate": "true",
		"recurly invoice list":           "false",
		"recurly invoice get":            "false",
		"recurly invoice line-items":     "false",
		"recurly invoice collect":        "true",
		"recurly transaction list":       "false",
		"recurly transaction get":        "false",
		"recurly plan list":              "false",
		"recurly plan get":               "false",
		"recurly coupon list":            "false",
		"recurly coupon get":             "false",
		"recurly line-item list":         "false",
		"recurly site list":              "false",
		"recurly site get":               "false",
	}

	got := map[string]string{}
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		key := strings.Join(path, " ")
		leaf := !cmd.HasSubCommands()
		ann, ok := cmd.Annotations["anycli.side_effect"]
		if leaf && cmd.RunE != nil {
			if !ok {
				t.Errorf("leaf %q is missing anycli.side_effect", key)
				return
			}
			got[key] = ann
			return
		}
		// Non-leaf group commands must not carry the annotation.
		if ok && len(path) > 1 {
			t.Errorf("group %q must not carry anycli.side_effect (got %q)", key, ann)
		}
		for _, c := range cmd.Commands() {
			walk(c, append(path, c.Name()))
		}
	}
	root := (&Service{}).NewCommandTree()
	walk(root, []string{"recurly"})

	for key, wantVal := range want {
		if got[key] != wantVal {
			t.Errorf("leaf %q side_effect = %q, want %q", key, got[key], wantVal)
		}
	}
	if len(got) != len(want) {
		t.Errorf("leaf count = %d, want %d (leaves: %v)", len(got), len(want), keys(got))
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
