//go:build e2e

// Real-API e2e for the notion service (design 008 D4). Confirmation round:
// a read-only smoke against the connected workspace. Write closed loops
// follow once the gateway path is confirmed.
package notion_test

import (
	"encoding/json"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

func TestE2EUserSelf(t *testing.T) {
	out, exit := e2e.RunTool(t, "notion", "", "user", "get", "self", "--json")
	if exit != 0 {
		t.Fatalf("user get self exit = %d, output:\n%s", exit, out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(v) == 0 {
		t.Fatal("user get self returned empty JSON")
	}
}
