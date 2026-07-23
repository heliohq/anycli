package tiktok

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
	result, err := svc.Execute(context.Background(), []string{"user", "info"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr.String(), "TIKTOK_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q, want missing-token message", stderr.String())
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

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
	}{
		{
			name:         "HTTP unauthorized",
			status:       http.StatusUnauthorized,
			body:         `{"error":{"code":"access_token_invalid","message":"invalid"}}`,
			wantRejected: true,
		},
		{
			name:         "envelope access_token_invalid on 200",
			status:       http.StatusOK,
			body:         `{"data":{},"error":{"code":"access_token_invalid","message":"invalid"}}`,
			wantRejected: true,
		},
		{
			name:         "scope error on 200 is not a credential rejection",
			status:       http.StatusOK,
			body:         `{"data":{},"error":{"code":"scope_not_authorized","message":"missing scope"}}`,
			wantRejected: false,
		},
		{
			name:         "rate limited",
			status:       http.StatusTooManyRequests,
			body:         `{"error":{"code":"rate_limit_exceeded","message":"slow down"}}`,
			wantRejected: false,
		},
		{
			name:         "server failure",
			status:       http.StatusInternalServerError,
			body:         `{"error":{"code":"internal_error","message":"boom"}}`,
			wantRejected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				jsonResponse(w, tc.status, tc.body)
			})
			defer server.Close()

			result, _, _ := runResult(t, server, fullEnv(), "user", "info")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
		})
	}
}

func TestErrorOutputRedactsToken(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		// Echo the token into the error body to prove redaction.
		jsonResponse(w, http.StatusForbidden, `{"error":{"code":"x","message":"token act.user-token leaked"}}`)
	})
	defer server.Close()

	_, _, stderr := run(t, server, fullEnv(), "user", "info")
	if strings.Contains(stderr, "act.user-token") {
		t.Fatalf("stderr leaked the access token: %q", stderr)
	}
}
