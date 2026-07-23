//go:build e2e

// Real-API e2e for the attio service (design 008 D4): a read smoke plus a
// closed-loop record chain. The chain is self-cleaning — the delete step IS
// the cleanup; the anycli-e2e-<runid>- name prefix makes any interrupted-run
// leftovers identifiable.
package attio_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

func TestE2EWhoami(t *testing.T) {
	out, exit := e2e.RunTool(t, "attio", "", "whoami", "--json")
	if exit != 0 {
		t.Fatalf("whoami exit = %d, output:\n%s", exit, out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("whoami output is not JSON: %v\n%s", err, out)
	}
	if len(v) == 0 {
		t.Fatal("whoami returned empty JSON")
	}
}

func TestE2ERecordClosedLoop(t *testing.T) {
	name := e2e.Prefix() + "company"

	// Create. --values is a plain JSON object of attribute slug -> value
	// (records.go newRecordCreateCmd wraps it as {"data":{"values": ...}}
	// before sending); the server response is Attio's standard {"data": ...}
	// envelope (client.go emitJSON prints the provider body verbatim).
	out, exit := e2e.RunTool(t, "attio", "", "record", "create", "companies",
		"--values", fmt.Sprintf(`{"name":%q}`, name), "--json")
	if exit != 0 {
		t.Fatalf("create exit = %d, output:\n%s", exit, out)
	}
	recordID := extractRecordID(t, out)

	// Read back: the created name must be visible.
	out, exit = e2e.RunTool(t, "attio", "", "record", "get", "companies", recordID, "--json")
	if exit != 0 {
		t.Fatalf("get exit = %d, output:\n%s", exit, out)
	}
	if !strings.Contains(out, name) {
		t.Fatalf("get output does not contain created name %q:\n%s", name, out)
	}

	// Delete (this IS the cleanup).
	out, exit = e2e.RunTool(t, "attio", "", "record", "delete", "companies", recordID, "--json")
	if exit != 0 {
		t.Fatalf("delete exit = %d, output:\n%s", exit, out)
	}

	// Verify gone: get after delete must fail.
	out, exit = e2e.RunTool(t, "attio", "", "record", "get", "companies", recordID, "--json")
	if exit == 0 {
		t.Fatalf("get after delete succeeded, record %s still exists:\n%s", recordID, out)
	}
}

// extractRecordID pulls the record id out of a create/get JSON response.
// Attio wraps every response in a {"data": ...} envelope (see
// client.go emitJSON / records_test.go okData), and within that envelope
// nests the id as id.record_id. Both wrapped and unwrapped shapes are
// accepted here so this stays robust if the envelope ever changes.
func extractRecordID(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		Data struct {
			ID struct {
				RecordID string `json:"record_id"`
			} `json:"id"`
			RecordID string `json:"record_id"`
		} `json:"data"`
		ID struct {
			RecordID string `json:"record_id"`
		} `json:"id"`
		RecordID string `json:"record_id"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse create output: %v\n%s", err, out)
	}
	if v.Data.ID.RecordID != "" {
		return v.Data.ID.RecordID
	}
	if v.Data.RecordID != "" {
		return v.Data.RecordID
	}
	if v.ID.RecordID != "" {
		return v.ID.RecordID
	}
	if v.RecordID != "" {
		return v.RecordID
	}
	t.Fatalf("no record id in output:\n%s", out)
	return ""
}
