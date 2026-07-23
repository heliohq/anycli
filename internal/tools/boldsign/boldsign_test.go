package boldsign

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingTokenFailsExit1(t *testing.T) {
	svc := &Service{}
	result, err := svc.Execute(context.Background(), []string{"document", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
}

func TestExecute_UnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "document", "bogus")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for unknown subcommand", code)
	}
}

func TestExecute_APIErrorIsExit1(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"message":"Title is required"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "document", "get", "--id", "doc-1")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Title is required") {
		t.Errorf("stderr = %q, want provider message", stderr)
	}
}

func TestExecute_401RejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"unauthorized"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "document", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("a 401 must mark the credential rejected")
	}
}

func TestExecute_JSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"message":"document not found"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "document", "get", "--id", "missing", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &envelope); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != http.StatusNotFound {
		t.Errorf("envelope = %+v, want kind=api status=404", envelope.Error)
	}
	if !strings.Contains(envelope.Error.Message, "document not found") {
		t.Errorf("envelope message = %q", envelope.Error.Message)
	}
}

func TestExecute_MissingTokenJSONEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"document", "list", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envelope struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &envelope); err != nil {
		t.Fatalf("stderr not JSON: %v (%s)", err, errBuf.String())
	}
	if envelope.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", envelope.Error.Kind)
	}
}
