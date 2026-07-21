package helpscout

import (
	"net/http"
	"strings"
	"testing"
)

// TestCustomerUpdate_PartialPatch asserts `customer update` issues a partial
// PATCH (JSON-Patch array), NOT a destructive PUT/overwrite: only the fields
// passed appear as replace ops and omitted core fields are never sent, so the
// provider preserves them (Overwrite Customer nulls omitted fields — the
// footgun this guards against).
func TestCustomerUpdate_PartialPatch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "customer", "update", "101", "--job-title", "VP Sales")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	if got.Method != http.MethodPatch {
		t.Fatalf("method = %s, want PATCH (partial update, not PUT overwrite)", got.Method)
	}
	if got.Path != "/customers/101" {
		t.Errorf("path = %s", got.Path)
	}
	ops := decodeArray(t, got.Body)
	if len(ops) != 1 {
		t.Fatalf("ops = %v, want exactly one replace op (omitted fields not sent)", ops)
	}
	op, _ := ops[0].(map[string]any)
	if op["op"] != "replace" || op["path"] != "/jobTitle" || op["value"] != "VP Sales" {
		t.Errorf("op = %v, want replace /jobTitle=VP Sales", op)
	}
	// The destructive footgun: omitted core fields must NOT appear anywhere in
	// the patch, or the provider would null them.
	for _, o := range ops {
		m, _ := o.(map[string]any)
		switch m["path"] {
		case "/firstName", "/lastName", "/organization":
			t.Errorf("omitted field %v must not be in the patch (would null it)", m["path"])
		}
	}
	rec := decodeBody(t, []byte(stdout))
	if rec["id"] != "101" || rec["status"] != "updated" {
		t.Errorf("receipt = %s", strings.TrimSpace(stdout))
	}
}

// TestCustomerUpdate_MultipleFields sends every passed field as its own replace
// op inside a single PATCH array, and never sends an unset field.
func TestCustomerUpdate_MultipleFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "customer", "update", "5",
		"--first-name", "Ada", "--organization", "Analytical Engines")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr)
	}
	ops := decodeArray(t, got.Body)
	if len(ops) != 2 {
		t.Fatalf("ops = %v, want 2", ops)
	}
	values := map[string]any{}
	for _, o := range ops {
		m, _ := o.(map[string]any)
		if m["op"] != "replace" {
			t.Errorf("op = %v, want replace", m["op"])
		}
		path, _ := m["path"].(string)
		values[path] = m["value"]
	}
	if values["/firstName"] != "Ada" || values["/organization"] != "Analytical Engines" {
		t.Errorf("values = %v", values)
	}
	if _, ok := values["/lastName"]; ok {
		t.Error("unset --last-name must not appear in the patch")
	}
}

// TestCustomerUpdate_NoFieldsIsUsageError makes no HTTP call and exits 2.
func TestCustomerUpdate_NoFieldsIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "customer", "update", "5")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if got.Method != "" {
		t.Error("expected no HTTP call with nothing to update")
	}
}
