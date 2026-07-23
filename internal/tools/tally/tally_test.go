package tally

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingToken(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "TALLY_API_KEY is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestExecute_MissingToken_JSONEnvelope(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Error struct {
			Tool    string `json:"tool"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if jErr := json.Unmarshal([]byte(errBuf.String()), &env); jErr != nil {
		t.Fatalf("stderr is not a JSON envelope: %v (%s)", jErr, errBuf.String())
	}
	if env.Error.Tool != "tally" || env.Error.Code != "usage" {
		t.Errorf("envelope = %+v, want tool=tally code=usage", env.Error)
	}
}

func TestMe_Happy_BearerHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"user_1","email":"a@b.co"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/users/me" {
		t.Errorf("request = %s %s, want GET /users/me", got.Method, got.Path)
	}
	if got.Auth != "Bearer tly-abc" {
		t.Errorf("Authorization = %q, want Bearer tly-abc", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"id":"user_1"`) {
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
			srv := newServer(t, tc.status, `{"message":"nope"}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, nil, "me")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1 (API failure)", result.ExitCode)
			}
			if !strings.Contains(stderr, "nope") {
				t.Errorf("stderr = %q, want the provider message", stderr)
			}
		})
	}
}

func TestAPIError_JSONEnvelopeCarriesStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNotFound, `{"message":"form not found"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, nil, "form", "get", "--form", "missing", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Tool    string `json:"tool"`
			Code    string `json:"code"`
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Code != "api" || env.Error.Status != http.StatusNotFound || env.Error.Tool != "tally" {
		t.Errorf("envelope = %+v, want code=api status=404 tool=tally", env.Error)
	}
	if !strings.Contains(env.Error.Message, "form not found") {
		t.Errorf("message = %q, want provider text", env.Error.Message)
	}
}

func TestUsageError_UnknownSubcommand_Exit2(t *testing.T) {
	result, _, _ := runResult(t, nil, nil, "form", "bogus")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for unknown subcommand", result.ExitCode)
	}
}

func TestUsageError_MissingRequiredFlag_Exit2(t *testing.T) {
	// form get requires --form; omitting it is a usage error (exit 2), and no
	// HTTP request must be attempted.
	result, _, stderr := runResult(t, nil, nil, "form", "get")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for missing required flag", result.ExitCode)
	}
	if !strings.Contains(stderr, "form") {
		t.Errorf("stderr = %q, want the required-flag message", stderr)
	}
}

func TestBareGroup_ShowsHelp_Exit0(t *testing.T) {
	// A runnable group with no subcommand prints help and exits 0 (not a false
	// success against an unknown subcommand).
	result, _, _ := runResult(t, nil, nil, "form")
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0 for bare group help", result.ExitCode)
	}
}
