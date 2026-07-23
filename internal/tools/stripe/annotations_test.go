package stripe

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSideEffectAnnotations pins the anycli.side_effect fact on every runnable
// leaf: mutations (create/update/cancel/finalize/send) are "true", reads are
// "false". The approval gate (design 318) reads this fact, so a drift here
// silently mis-gates a Stripe mutation — this test fails the build instead.
func TestSideEffectAnnotations(t *testing.T) {
	want := map[string]string{
		"stripe balance get":          "false",
		"stripe balance transactions": "false",
		"stripe charge list":          "false",
		"stripe charge get":           "false",
		"stripe payment-intent list":  "false",
		"stripe payment-intent get":   "false",
		"stripe customer list":        "false",
		"stripe customer get":         "false",
		"stripe customer search":      "false",
		"stripe customer create":      "true",
		"stripe customer update":      "true",
		"stripe invoice list":         "false",
		"stripe invoice get":          "false",
		"stripe invoice create":       "true",
		"stripe invoice finalize":     "true",
		"stripe invoice send":         "true",
		"stripe subscription list":    "false",
		"stripe subscription get":     "false",
		"stripe subscription cancel":  "true",
		"stripe refund list":          "false",
		"stripe refund get":           "false",
		"stripe refund create":        "true",
		"stripe payout list":          "false",
		"stripe payout get":           "false",
		"stripe product list":         "false",
		"stripe product get":          "false",
		"stripe price list":           "false",
		"stripe price get":            "false",
		"stripe dispute list":         "false",
		"stripe dispute get":          "false",
		"stripe event list":           "false",
		"stripe event get":            "false",
		"stripe search":               "false",
		"stripe get":                  "false",
	}

	got := map[string]string{}
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		p := append(path, firstWord(cmd.Use))
		if cmd.HasSubCommands() {
			for _, c := range cmd.Commands() {
				walk(c, p)
			}
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			return
		}
		key := strings.Join(p, " ")
		got[key] = cmd.Annotations[sideEffectAnnotation]
	}
	walk((&Service{}).NewCommandTree(), nil)

	for key, wantVal := range want {
		gotVal, ok := got[key]
		if !ok {
			t.Errorf("leaf %q not found in command tree", key)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("leaf %q side_effect = %q, want %q", key, gotVal, wantVal)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("unexpected leaf %q — add it to the want map with its side-effect fact", key)
		}
	}
}

// firstWord returns the command word from a cobra Use string (e.g. "get <id>"
// -> "get").
func firstWord(use string) string {
	if i := strings.IndexByte(use, ' '); i >= 0 {
		return use[:i]
	}
	return use
}
