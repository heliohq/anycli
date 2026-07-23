package crisp

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

// TestAuthHeaderAssembly proves the keypair is sent as HTTP Basic auth and the
// website tier constant header rides every request.
func TestAuthHeaderAssembly(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "conversation", "list", "--website", "wid-1")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(defaultToken))
	if got.Auth != wantAuth {
		t.Errorf("Authorization = %q, want %q", got.Auth, wantAuth)
	}
	if got.Tier != "website" {
		t.Errorf("X-Crisp-Tier = %q, want website", got.Tier)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
}

// TestMissingWebsiteIsUsageError proves an omitted --website exits 2 before any
// network call is attempted.
func TestMissingWebsiteIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "conversation", "list")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", exit, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if got.Method != "" {
		t.Errorf("a network call was made (%s %s); none expected", got.Method, got.Path)
	}
	if !strings.Contains(stderr, "website") {
		t.Errorf("stderr = %q, want it to mention --website", stderr)
	}
}

// TestMissingTokenExits1 proves an absent CRISP_TOKEN fails fast with exit 1.
func TestMissingTokenExits1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	result, _, stderr := runToken(t, srv, "", "conversation", "list", "--website", "wid-1")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, "CRISP_TOKEN") {
		t.Errorf("stderr = %q, want it to mention CRISP_TOKEN", stderr)
	}
}

// TestMalformedTokenExits1 proves a keypair with no colon separator is rejected
// before any request.
func TestMalformedTokenExits1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[]}`, &got)
	defer srv.Close()

	result, _, stderr := runToken(t, srv, "no-colon-here", "conversation", "list", "--website", "wid-1")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("a network call was made; none expected for a malformed token")
	}
	if !strings.Contains(stderr, "identifier:key") {
		t.Errorf("stderr = %q, want it to mention identifier:key form", stderr)
	}
}

// TestSuccessEnvelope proves success wraps the Crisp data with a meta block.
func TestSuccessEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":false,"data":[{"session_id":"session_1"}]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "conversation", "list", "--website", "wid-1", "--json")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	out := decodeOutput(t, stdout)
	if _, ok := out["data"]; !ok {
		t.Errorf("output missing data key: %s", stdout)
	}
	meta, ok := out["meta"].(map[string]any)
	if !ok {
		t.Fatalf("output missing meta object: %s", stdout)
	}
	if meta["website_id"] != "wid-1" {
		t.Errorf("meta.website_id = %v, want wid-1", meta["website_id"])
	}
}

// TestCrispErrorEnvelope proves a Crisp {error:true} body renders the structured
// error envelope on stderr with a non-zero exit.
func TestCrispErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"error":true,"reason":"conversation_not_found"}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "conversation", "get", "--session", "s1", "--website", "wid-1", "--json")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
	errOut := decodeOutput(t, stderr)
	inner, ok := errOut["error"].(map[string]any)
	if !ok {
		t.Fatalf("stderr not an error envelope: %s", stderr)
	}
	if !strings.Contains(inner["message"].(string), "conversation_not_found") {
		t.Errorf("error message = %v, want it to carry the Crisp reason", inner["message"])
	}
}

// TestUnauthorizedRejectsCredential proves a 401 marks the credential rejected.
func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"error":true,"reason":"invalid_session"}`, &got)
	defer srv.Close()

	result, _, stderr := runToken(t, srv, defaultToken, "conversation", "list", "--website", "wid-1")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Errorf("CredentialRejected = false, want true for a 401 (stderr: %s)", stderr)
	}
}
