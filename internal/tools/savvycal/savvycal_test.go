package savvycal

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
	if !strings.Contains(errBuf.String(), "SAVVYCAL_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}

func TestExecute_MissingToken_JSON(t *testing.T) {
	var errBuf bytes.Buffer
	svc := &Service{Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errBuf.String()), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, errBuf.String())
	}
	if env.Error.Kind != "usage" || !strings.Contains(env.Error.Message, "SAVVYCAL_ACCESS_TOKEN") {
		t.Errorf("error envelope = %+v, want usage kind with token message", env.Error)
	}
}

func TestMe_Happy(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"user_01ED74","email":"a@b.co","display_name":"Alice"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/me" {
		t.Errorf("request = %s %s, want GET /me", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", got.Auth)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if !strings.Contains(stdout, `"id":"user_01ED74"`) || !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout = %q, want provider JSON passthrough + newline", stdout)
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
			srv := newServer(t, tc.status, `{"error":"nope"}`, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "me")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
			if !strings.Contains(stderr, "savvycal API error") {
				t.Errorf("stderr = %q, want the provider error", stderr)
			}
		})
	}
}

func TestAPIError_ValidationBodyPassthrough_JSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"errors":{"email":["is invalid"]}}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "me", "--json")
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
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("error envelope = %+v, want api kind with status 422", env.Error)
	}
	if !strings.Contains(env.Error.Message, "is invalid") {
		t.Errorf("message = %q, want validation body passed through", env.Error.Message)
	}
}

func TestUnknownSubcommand_UsageExit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "bogus")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 for unknown subcommand", code)
	}
}
