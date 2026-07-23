//go:build e2e

// Real-API e2e for the bitly service (design 008 D4). Confirmation round:
// a read-only smoke against the connected account. Write closed loops
// follow once the gateway path is confirmed.
package bitly_test

import (
	"encoding/json"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

func TestE2EUserGet(t *testing.T) {
	out, exit := e2e.RunTool(t, "bitly", "", "user", "get", "--json")
	if exit != 0 {
		t.Fatalf("user get exit = %d, output:\n%s", exit, out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(v) == 0 {
		t.Fatal("user get returned empty JSON")
	}
}
