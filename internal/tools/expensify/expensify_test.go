package expensify

import (
	"strings"
	"testing"
)

func TestExecuteMissingCredentials(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":200}`, &capturedRequest{})
	defer srv.Close()
	result, _, stderr := runResult(t, srv, "", "policy", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatal("missing env must not be reported as credential rejection")
	}
	if !strings.Contains(stderr, EnvCredentials) {
		t.Fatalf("stderr should name %s, got %q", EnvCredentials, stderr)
	}
}

func TestExecuteMalformedCredentials(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":200}`, &capturedRequest{})
	defer srv.Close()
	// No colon separator.
	result, _, stderr := runResult(t, srv, "no-colon-here", "policy", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "partnerUserID:partnerUserSecret") {
		t.Fatalf("stderr should show the expected format, got %q", stderr)
	}
}

func TestPolicyListInjectsCredentialsAndJob(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"responseCode":200,"policyList":[]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "policy", "list", "--admin-only", "--user-email", "boss@example.com")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Method != "POST" {
		t.Fatalf("method = %s, want POST", got.Method)
	}
	if !strings.HasPrefix(got.ContentType, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type = %q, want form-urlencoded", got.ContentType)
	}
	if got.Job["type"] != "get" {
		t.Fatalf("job type = %v, want get", got.Job["type"])
	}
	creds := credentialsOf(t, got)
	if creds["partnerUserID"] != testPartnerUserID {
		t.Fatalf("partnerUserID = %v, want %s", creds["partnerUserID"], testPartnerUserID)
	}
	if creds["partnerUserSecret"] != testPartnerUserSecret {
		t.Fatalf("partnerUserSecret = %v, want %s", creds["partnerUserSecret"], testPartnerUserSecret)
	}
	in := inputSettingsOf(t, got)
	if in["type"] != "policyList" {
		t.Fatalf("inputSettings.type = %v, want policyList", in["type"])
	}
	if in["adminOnly"] != true {
		t.Fatalf("adminOnly = %v, want true", in["adminOnly"])
	}
	if in["userEmail"] != "boss@example.com" {
		t.Fatalf("userEmail = %v, want boss@example.com", in["userEmail"])
	}
	if !strings.Contains(stdout, "policyList") {
		t.Fatalf("stdout should passthrough provider body, got %q", stdout)
	}
}

func TestPolicyListOmitsUnsetOptionals(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"responseCode":200}`, &got)
	defer srv.Close()

	if exit, _, stderr := run(t, srv, "policy", "list"); exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	in := inputSettingsOf(t, got)
	if _, ok := in["adminOnly"]; ok {
		t.Fatalf("adminOnly must be omitted when unset, got %v", in["adminOnly"])
	}
	if _, ok := in["userEmail"]; ok {
		t.Fatalf("userEmail must be omitted when empty, got %v", in["userEmail"])
	}
}

// TestCredentialRejectedOnResponseCode401 verifies the 200-with-responseCode
// dialect maps a 401 body to an explicit credential rejection.
func TestCredentialRejectedOnResponseCode401(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":401,"responseMessage":"Authentication error"}`, &capturedRequest{})
	defer srv.Close()

	result, _, stderr := runResult(t, srv, testCredentials, "policy", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatal("responseCode 401 must mark the credential rejected")
	}
	if !strings.Contains(stderr, "Authentication error") {
		t.Fatalf("stderr should carry provider message, got %q", stderr)
	}
}

func TestNonAuthResponseCodeIsPlainError(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":410,"responseMessage":"Policy not found"}`, &capturedRequest{})
	defer srv.Close()

	result, _, stderr := runResult(t, srv, testCredentials, "policy", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatal("a non-auth responseCode must not invalidate the credential")
	}
	if !strings.Contains(stderr, "Policy not found") {
		t.Fatalf("stderr should carry provider message, got %q", stderr)
	}
}
