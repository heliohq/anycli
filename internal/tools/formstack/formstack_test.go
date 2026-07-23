package formstack

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestUnauthorized_RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"error":"Invalid access token"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "form", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("401 should mark the credential rejected")
	}
	if !strings.Contains(stderr, "Invalid access token") {
		t.Errorf("stderr = %q, want the API message", stderr)
	}
}

func TestForbidden_IsPlainFailure(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, `{"error":"You do not have access"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "form", "get", "1")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("403 must not reject the credential")
	}
	if !strings.Contains(stderr, "You do not have access") || !strings.Contains(stderr, "403") {
		t.Errorf("stderr = %q, want the API message and status", stderr)
	}
}

func TestRateLimited_IsPlainFailure(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusTooManyRequests, `{"error":"Rate limit reached"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "form", "list")
	if result.ExitCode != 1 || result.CredentialRejected {
		t.Fatalf("result = %+v, want plain failure", result)
	}
	if !strings.Contains(stderr, "Rate limit reached") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestMissingToken(t *testing.T) {
	svc := &Service{}
	var out, errBuf strings.Builder
	svc.Out = &out
	svc.Err = &errBuf
	result, err := svc.Execute(context.Background(), []string{"form", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "FORMSTACK_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}
