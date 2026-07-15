package x

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecuteRequiresAccessToken(t *testing.T) {
	var stderr bytes.Buffer
	svc := &Service{Err: &stderr}
	result, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr.String(), "X_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q, want missing-token message", stderr.String())
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
	}{
		{name: "HTTP unauthorized", status: http.StatusUnauthorized, body: `{"title":"Unauthorized"}`, wantRejected: true},
		{name: "legacy invalid token", status: http.StatusForbidden, body: `{"errors":[{"code":89,"message":"Invalid or expired token."}]}`, wantRejected: true},
		{name: "suspended account", status: http.StatusForbidden, body: `{"errors":[{"code":64,"message":"Your account is suspended."}]}`, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"errors":[{"code":88,"message":"Rate limit exceeded."}]}`, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, body: `{"title":"Internal Error"}`, wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				jsonResponse(w, tc.status, tc.body)
			})
			defer server.Close()

			result, _, _ := runResult(t, server, fullEnv(), "me")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
		})
	}
}

func TestUnknownSubcommandFails(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "not-a-command")
	if code == 0 {
		t.Fatal("unknown subcommand returned exit code 0")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q, want unknown-command error", stderr)
	}
}

func TestJSONFlagHelpDescribesJSONL(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, stdout, stderr := run(t, server, fullEnv(), "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "single-result JSON; multi-result commands may emit JSONL") {
		t.Fatalf("help = %q", stdout)
	}
}
