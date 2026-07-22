package resend

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingAPIKey(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"email", "get", "e1"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "RESEND_API_KEY is not set") {
		t.Errorf("stderr = %q, want missing-key message", errBuf.String())
	}
}

func TestExecute_UnknownSubcommandIsUsageError(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{}`, &capturedRequest{})
	defer srv.Close()
	code, _, _ := run(t, srv, "email", "frobnicate")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
}

// TestCall_SetsAuthAndUserAgent proves every request carries the Bearer key and
// an explicit User-Agent (Resend 403s a missing User-Agent — see DESIGN §2.3).
func TestCall_SetsAuthAndUserAgent(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"e1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "email", "get", "e1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Auth != "Bearer re_test-key" {
		t.Errorf("Authorization = %q, want Bearer re_test-key", got.Auth)
	}
	if got.UserAgent == "" {
		t.Error("User-Agent header is empty; Resend rejects missing User-Agent with 403")
	}
}

// credentialRejectCase pins the name-based reject contract (DESIGN §2.3): the
// decision keys on the parsed error `name`, never the raw HTTP status, because
// both 401 and 403 are overloaded between credential and non-credential errors.
type credentialRejectCase struct {
	name       string
	status     int
	body       string
	wantReject bool
}

func TestCall_CredentialRejectMatrix(t *testing.T) {
	cases := []credentialRejectCase{
		{"invalid_api_key 403 -> reject", http.StatusForbidden, `{"statusCode":403,"name":"invalid_api_key","message":"API key is invalid."}`, true},
		{"missing_api_key 401 -> reject", http.StatusUnauthorized, `{"statusCode":401,"name":"missing_api_key","message":"Missing API key."}`, true},
		{"validation_error 403 unverified domain -> NO reject", http.StatusForbidden, `{"statusCode":403,"name":"validation_error","message":"The example.com domain is not verified."}`, false},
		{"restricted_api_key 401 live sending-only key -> NO reject", http.StatusUnauthorized, `{"statusCode":401,"name":"restricted_api_key","message":"This API key is restricted to only send emails."}`, false},
		{"validation_error 400 -> NO reject", http.StatusBadRequest, `{"statusCode":400,"name":"validation_error","message":"bad field"}`, false},
		{"invalid_attachment 422 -> NO reject", http.StatusUnprocessableEntity, `{"statusCode":422,"name":"invalid_attachment","message":"attachments not supported"}`, false},
		{"rate_limit_exceeded 429 -> NO reject", http.StatusTooManyRequests, `{"statusCode":429,"name":"rate_limit_exceeded","message":"Too many requests."}`, false},
		{"unparseable 401 body -> NO reject", http.StatusUnauthorized, `not json`, false},
		{"unparseable 403 body -> NO reject", http.StatusForbidden, `<html>gateway</html>`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, tc.body, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "email", "get", "e1")
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1 (API error)", result.ExitCode)
			}
			if result.CredentialRejected != tc.wantReject {
				t.Errorf("CredentialRejected = %v, want %v (stderr=%q)", result.CredentialRejected, tc.wantReject, stderr)
			}
		})
	}
}

func TestCall_JSONErrorEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"statusCode":422,"name":"validation_error","message":"missing subject"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "email", "get", "e1", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr, `"error"`) || !strings.Contains(stderr, `"status":422`) {
		t.Errorf("stderr = %q, want JSON error envelope with status 422", stderr)
	}
	if !strings.Contains(stderr, "missing subject") {
		t.Errorf("stderr = %q, want provider message surfaced", stderr)
	}
}

func TestCall_PlainTextErrorSurfacesMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"statusCode":422,"name":"validation_error","message":"missing subject"}`, &got)
	defer srv.Close()

	_, _, stderr := runResult(t, srv, "email", "get", "e1")
	if !strings.Contains(stderr, "missing subject") {
		t.Errorf("stderr = %q, want provider message surfaced in plain text", stderr)
	}
	if strings.Contains(stderr, `"error"`) {
		t.Errorf("stderr = %q, want plain text (no JSON envelope) without --json", stderr)
	}
}
