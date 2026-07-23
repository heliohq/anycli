package loops

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingKey(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"whoami"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "LOOPS_API_KEY is not set") {
		t.Errorf("stderr = %q, want the missing-key message", errBuf.String())
	}
}

func TestWhoami_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":true,"teamName":"Acme"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "whoami")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/api-key" {
		t.Errorf("request = %s %s, want GET /v1/api-key", got.Method, got.Path)
	}
	if got.Auth != "Bearer key-123" {
		t.Errorf("Authorization = %q, want Bearer key-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"teamName":"Acme"`) {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"success":false,"message":"Invalid API key"}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "whoami")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "Invalid API key") {
				t.Errorf("stderr = %q, want the provider message", stderr)
			}
		})
	}
}

// TestErrorMessage_PrefersMessageThenError proves the message extractor reads
// {message} first, then the deprecated {error}.
func TestErrorMessage_PrefersMessageThenError(t *testing.T) {
	if got := apiMessage([]byte(`{"message":"m","error":"e"}`)); got != "m" {
		t.Errorf("apiMessage = %q, want m", got)
	}
	if got := apiMessage([]byte(`{"error":"e"}`)); got != "e" {
		t.Errorf("apiMessage = %q, want e (deprecated fallback)", got)
	}
	if got := apiMessage([]byte(`not json`)); got != "not json" {
		t.Errorf("apiMessage = %q, want raw-body fallback", got)
	}
}

// TestJSONErrorEnvelope_API renders an API error as the structured envelope
// under --json, carrying kind:api and the HTTP status.
func TestJSONErrorEnvelope_API(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"message":"bad email"}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "--json", "whoami")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusUnprocessableEntity {
		t.Errorf("envelope = %+v, want kind=api status=422", env.Error)
	}
	if !strings.Contains(env.Error.Message, "bad email") {
		t.Errorf("message = %q, want the provider text", env.Error.Message)
	}
}

// TestUsageError_UnknownSubcommand is a cobra usage error → exit 2.
func TestUsageError_UnknownSubcommand(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "contact", "frobnicate")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage error)", code)
	}
	if got.Method != "" {
		t.Errorf("expected no HTTP call for an unknown subcommand, saw %s %s", got.Method, got.Path)
	}
}

func TestListLs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[{"id":"l1"}]`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "list", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/lists" {
		t.Errorf("request = %s %s, want GET /v1/lists", got.Method, got.Path)
	}
	if !strings.Contains(stdout, `"id":"l1"`) {
		t.Errorf("stdout = %q, want passthrough", stdout)
	}
}
