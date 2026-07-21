package figma

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations pins the anycli.side_effect fact on every runnable
// leaf of the command tree (design 318 may-mutate criterion) and asserts group
// commands carry no annotation. Catalog-backed commands derive the fact from
// the pinned catalog's HTTP method; this table pins the resulting value per
// command so any catalog or wiring drift shows up in review. Notable calls:
// `api call` invokes any catalogued operation by id, including non-GET
// mutations → true; `assets download` / `assets download-fills` fetch via GET
// and only write local files → false; `api list` / `api describe` /
// `capabilities` never leave the process → false.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		// analytics (all GET)
		"analytics component-actions": "false",
		"analytics component-usages":  "false",
		"analytics style-actions":     "false",
		"analytics style-usages":      "false",
		"analytics variable-actions":  "false",
		"analytics variable-usages":   "false",
		// api
		"api call":     "true", // arbitrary catalogued operation, incl. POST/PUT/DELETE
		"api describe": "false",
		"api list":     "false",
		"api request":  "true", // arbitrary raw --method/--path request, incl. POST/PUT/PATCH/DELETE
		// assets (GET + local file writes)
		"assets download":       "false",
		"assets download-fills": "false",
		// capabilities (local only)
		"capabilities": "false",
		// comments
		"comments delete":           "true", // DELETE comment
		"comments list":             "false",
		"comments post":             "true", // POST comment
		"comments reactions add":    "true", // POST reaction
		"comments reactions delete": "true", // DELETE reaction
		"comments reactions list":   "false",
		// context (all GET reads + local shaping)
		"context design":     "false",
		"context figjam":     "false",
		"context metadata":   "false",
		"context screenshot": "false",
		"context variables":  "false",
		// dev-resources
		"dev-resources create": "true", // POST dev resources
		"dev-resources delete": "true", // DELETE dev resource
		"dev-resources list":   "false",
		"dev-resources update": "true", // PUT dev resources
		// files / images (all GET)
		"files get":      "false",
		"files meta":     "false",
		"files nodes":    "false",
		"files versions": "false",
		"images fills":   "false",
		"images render":  "false",
		// libraries (all GET)
		"libraries component-sets file": "false",
		"libraries component-sets get":  "false",
		"libraries component-sets team": "false",
		"libraries components file":     "false",
		"libraries components get":      "false",
		"libraries components team":     "false",
		"libraries styles file":         "false",
		"libraries styles get":          "false",
		"libraries styles team":         "false",
		// misc reads
		"me":            "false",
		"oembed get":    "false",
		"payments list": "false",
		// projects / teams (all GET)
		"projects files": "false",
		"projects meta":  "false",
		"teams projects": "false",
		// variables
		"variables local":     "false",
		"variables published": "false",
		"variables update":    "true", // POST bulk variable create/update/delete
		// webhooks
		"webhooks create":   "true", // POST webhook
		"webhooks delete":   "true", // DELETE webhook
		"webhooks get":      "false",
		"webhooks list":     "false",
		"webhooks requests": "false",
		"webhooks team":     "false",
		"webhooks update":   "true", // PUT webhook
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

// TestOperationSideEffectDerivation pins the method → side-effect mapping used
// for catalog-backed commands and the safe-side default on a catalog miss.
func TestOperationSideEffectDerivation(t *testing.T) {
	cases := []struct {
		method string
		want   string
	}{
		{"GET", "false"},
		{"POST", "true"},
		{"PUT", "true"},
		{"PATCH", "true"},
		{"DELETE", "true"},
	}
	for _, tc := range cases {
		if got := operationSideEffect(tc.method); got != tc.want {
			t.Errorf("operationSideEffect(%q) = %q, want %q", tc.method, got, tc.want)
		}
	}

	stub := (&Service{}).newOperationCommand("", operationCommandSpec{Use: "broken", Short: "broken", OperationID: "noSuchOperation"})
	if got := stub.Annotations["anycli.side_effect"]; got != "true" {
		t.Errorf("catalog-miss stub anycli.side_effect = %q, want safe-side %q", got, "true")
	}
}
