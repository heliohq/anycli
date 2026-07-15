package figma

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteRejectsMissingToken(t *testing.T) {
	var stderr bytes.Buffer
	service := &Service{Err: &stderr}
	result, err := service.Execute(context.Background(), []string{"me"}, nil)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("missing local credential must not be reported as provider rejection")
	}
	if !strings.Contains(stderr.String(), "FIGMA_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q, want missing-token error", stderr.String())
	}
}

func TestExecuteClassifiesOnlyExplicitCredentialRejection(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		args     []string
		rejected bool
	}{
		{name: "invalid PAT", status: http.StatusForbidden, body: `{"err":"Invalid token"}`, args: []string{"me"}, rejected: true},
		{name: "invalid personal access token", status: http.StatusForbidden, body: `{"err":"Personal access token is invalid"}`, args: []string{"me"}, rejected: true},
		{name: "expired PAT", status: http.StatusUnauthorized, body: `{"message":"Authentication required"}`, args: []string{"me"}, rejected: true},
		{name: "missing scope", status: http.StatusForbidden, body: `{"err":"Insufficient scope"}`, args: []string{"me"}},
		{name: "invalid token scope", status: http.StatusForbidden, body: `{"err":"Invalid token scope"}`, args: []string{"me"}},
		{name: "rate limit", status: http.StatusTooManyRequests, body: `{"err":"Rate limited"}`, args: []string{"me"}},
		{name: "bad flags", status: http.StatusOK, body: `{}`, args: []string{"files", "nodes", "--file-key", "abc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			server := newTestServer(t, tc.status, tc.body, nil, &got)
			defer server.Close()
			result, _, _ := runServiceResult(t, server, tc.args...)
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want 1", result.ExitCode)
			}
			if result.CredentialRejected != tc.rejected {
				t.Errorf("CredentialRejected = %v, want %v", result.CredentialRejected, tc.rejected)
			}
		})
	}
}

func TestExecuteRedactsEchoedPATFromProviderError(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusBadRequest, `{"err":"Rejected figd_test_token"}`, nil, &got)
	defer server.Close()
	code, _, stderr := runService(t, server, "me")
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if strings.Contains(stderr, "figd_test_token") || !strings.Contains(stderr, "[REDACTED]") {
		t.Fatalf("stderr did not redact the PAT: %q", stderr)
	}
}

func TestAPIRedirectCannotForwardPATAcrossOrigins(t *testing.T) {
	forwardedToken := ""
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		forwardedToken = "request reached target"
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/capture", http.StatusFound)
	}))
	defer source.Close()

	code, _, stderr := runService(t, source, "me")
	if code != 1 || !strings.Contains(stderr, "redirect changed origin") {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if forwardedToken != "" {
		t.Fatal("cross-origin redirect reached the target")
	}
}

func TestAPIRedirectPreservesPATOnSameOrigin(t *testing.T) {
	redirectedToken := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me" {
			http.Redirect(w, r, "/redirected", http.StatusFound)
			return
		}
		redirectedToken = r.Header.Get("X-Figma-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"42"}`)
	}))
	defer server.Close()

	code, _, stderr := runService(t, server, "me")
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if redirectedToken != "figd_test_token" {
		t.Fatalf("redirected X-Figma-Token = %q", redirectedToken)
	}
}

func TestReadBoundedResponseRejectsOversizeBody(t *testing.T) {
	_, err := readBoundedResponse(strings.NewReader("12345"), 4)
	if err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") {
		t.Fatalf("error = %v, want size bound", err)
	}
}

func TestValidateJSONStream(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		hasJSON bool
		wantErr string
	}{
		{name: "object", body: `{"ok":true}`, hasJSON: true},
		{name: "scalar", body: `true`, hasJSON: true},
		{name: "empty", body: " \n ", hasJSON: false},
		{name: "multiple", body: `{} {}`, wantErr: "multiple top-level"},
		{name: "invalid", body: `{`, wantErr: "EOF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hasJSON, err := validateJSONStream(strings.NewReader(tc.body))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || hasJSON != tc.hasJSON {
				t.Fatalf("hasJSON = %v, err = %v", hasJSON, err)
			}
		})
	}
}

func TestExecuteRejectsNonJSONSuccessWithoutEmittingIt(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `not-json`, nil, &got)
	defer server.Close()
	code, stdout, stderr := runService(t, server, "me")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "response is not valid JSON") {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestMeUsesFigmaPATHeader(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"id":"42","email":"designer@example.com"}`, nil, &got)
	defer server.Close()

	code, stdout, stderr := runService(t, server, "me")
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/me" {
		t.Errorf("request = %s %s, want GET /v1/me", got.Method, got.Path)
	}
	if got.FigmaToken != "figd_test_token" {
		t.Errorf("X-Figma-Token = %q", got.FigmaToken)
	}
	if got.Authorization != "" {
		t.Errorf("Authorization = %q, want empty", got.Authorization)
	}
	if !strings.Contains(stdout, `"id":"42"`) {
		t.Errorf("stdout = %q, want provider JSON", stdout)
	}
}

func TestFigmaAPIErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		want    string
	}{
		{name: "err dialect", status: http.StatusForbidden, body: `{"status":403,"err":"Invalid token"}`, want: "Invalid token (HTTP 403)"},
		{name: "message dialect", status: http.StatusBadRequest, body: `{"error":true,"status":400,"message":"Bad file key"}`, want: "Bad file key (HTTP 400)"},
		{name: "rate limit", status: http.StatusTooManyRequests, body: `{"status":429,"err":"Rate limited"}`, headers: map[string]string{"Retry-After": "30"}, want: "Rate limited (HTTP 429, retry after 30 seconds)"},
		{name: "invalid body", status: http.StatusInternalServerError, body: `not-json`, want: "Internal Server Error (HTTP 500)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			server := newTestServer(t, tc.status, tc.body, tc.headers, &got)
			defer server.Close()
			code, _, stderr := runService(t, server, "me")
			if code != 1 {
				t.Fatalf("code = %d, want 1", code)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want %q", stderr, tc.want)
			}
		})
	}
}
