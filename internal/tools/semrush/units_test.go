package semrush

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestUnits_ParsesPlainNumber(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "1000", &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "units")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != unitsPath {
		t.Errorf("path = %q, want %q", got.Path, unitsPath)
	}
	if parseQuery(t, got.Query).Get("key") != "key-abcd1234" {
		t.Errorf("units request must carry the key, query=%q", got.Query)
	}
	var out struct {
		Remaining int64 `json:"api_units_remaining"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, stdout)
	}
	if out.Remaining != 1000 {
		t.Errorf("api_units_remaining = %d, want 1000", out.Remaining)
	}
}

func TestUnits_HandlesThousandsSeparator(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "1,000,000\n", &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "units")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var out struct {
		Remaining int64 `json:"api_units_remaining"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, stdout)
	}
	if out.Remaining != 1000000 {
		t.Errorf("api_units_remaining = %d, want 1000000", out.Remaining)
	}
}

func TestUnits_ErrorBodyRejectsKey(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "ERROR 120 :: WRONG KEY - ID PAIR", &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "units")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("ERROR 120 on units should reject the credential")
	}
}
