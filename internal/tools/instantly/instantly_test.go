package instantly

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMissingAPIKeyFailsFast(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"campaign", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAPIKey) {
		t.Fatalf("stderr = %q, want mention of %s", errBuf.String(), EnvAPIKey)
	}
	if result.CredentialRejected {
		t.Fatal("missing key must not be reported as a credential rejection")
	}
}

func TestMissingAPIKeyJSONEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"--json", "campaign", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, EnvAPIKey) {
		t.Fatalf("envelope = %+v, want usage kind mentioning %s", env.Error, EnvAPIKey)
	}
}

func TestBearerAuthHeaderInjected(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "campaign", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr)
	}
	if got.Auth != "Bearer key-123" {
		t.Fatalf("Authorization = %q, want %q", got.Auth, "Bearer key-123")
	}
	if got.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got.Accept)
	}
}

func TestPassthroughJSONOnStdout(t *testing.T) {
	var got capturedRequest
	body := `{"items":[{"id":"c1"}],"next_starting_after":"cursor2"}`
	srv := newServer(t, http.StatusOK, body, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "campaign", "list")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if strings.TrimSpace(stdout) != body {
		t.Fatalf("stdout = %q, want verbatim %q", stdout, body)
	}
}

func TestAPIErrorExit1AndCredentialRejectionOn401(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"statusCode":401,"error":"Unauthorized","message":"Invalid API key"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "campaign", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Fatal("401 must be classified as credential rejection")
	}
	if !strings.Contains(stderr, "Invalid API key") {
		t.Fatalf("stderr = %q, want provider message", stderr)
	}
}

func TestAPIError402NotCredentialRejection(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusPaymentRequired, `{"statusCode":402,"error":"Payment Required","message":"Workspace does not have an active paid plan"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "campaign", "list")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatal("402 (plan gate) must not invalidate the credential")
	}
	if !strings.Contains(stderr, "active paid plan") {
		t.Fatalf("stderr = %q, want plan message", stderr)
	}
}

func TestAPIErrorJSONEnvelopeCarriesStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"statusCode":404,"error":"Not Found","message":"Resource not found"}`, &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "--json", "campaign", "get", "--id", "missing")
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 404 {
		t.Fatalf("envelope = %+v, want api kind with status 404", env.Error)
	}
}

func TestUnknownSubcommandIsUsageExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "campaign", "frobnicate")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", exit)
	}
}
