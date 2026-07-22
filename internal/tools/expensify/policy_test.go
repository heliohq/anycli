package expensify

import (
	"reflect"
	"testing"
)

func TestPolicyGetBuildsInputSettings(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"responseCode":200,"policyInfo":{}}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv,
		"policy", "get",
		"--policy-id", "0123456789ABCDEF",
		"--policy-id", "DEADBEEF01234567",
		"--field", "tax",
		"--field", "categories",
		"--user-email", "boss@example.com",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	in := inputSettingsOf(t, got)
	if in["type"] != "policy" {
		t.Fatalf("inputSettings.type = %v, want policy", in["type"])
	}
	wantIDs := []any{"0123456789ABCDEF", "DEADBEEF01234567"}
	if !reflect.DeepEqual(in["policyIDList"], wantIDs) {
		t.Fatalf("policyIDList = %v, want %v", in["policyIDList"], wantIDs)
	}
	wantFields := []any{"tax", "categories"}
	if !reflect.DeepEqual(in["fields"], wantFields) {
		t.Fatalf("fields = %v, want %v", in["fields"], wantFields)
	}
	if in["userEmail"] != "boss@example.com" {
		t.Fatalf("userEmail = %v", in["userEmail"])
	}
}

func TestPolicyGetRequiresPolicyID(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":200}`, &capturedRequest{})
	defer srv.Close()

	// Missing --policy-id must fail as a usage error (exit 2), before any call.
	result, _, _ := runResult(t, srv, testCredentials, "policy", "get")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", result.ExitCode)
	}
}

func TestPolicyGetOmitsFieldsWhenUnset(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"responseCode":200}`, &got)
	defer srv.Close()

	if exit, _, stderr := run(t, srv, "policy", "get", "--policy-id", "ABC"); exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	in := inputSettingsOf(t, got)
	if _, ok := in["fields"]; ok {
		t.Fatalf("fields must be omitted when unset, got %v", in["fields"])
	}
	if _, ok := in["userEmail"]; ok {
		t.Fatalf("userEmail must be omitted when empty, got %v", in["userEmail"])
	}
}
