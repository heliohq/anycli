package msgraph

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// testClient wires a Client at srv with sleeps recorded (never slept) so retry
// paths run fast and deterministically.
func testClient(srv *httptest.Server, out *bytes.Buffer, sleeps *[]time.Duration) *Client {
	return &Client{
		Provider:    "microsoft-test",
		APILabel:    "microsoft-test API error",
		ScopeHint:   " (scope hint)",
		ResolveBase: func() string { return srv.URL },
		ResolveHTTP: srv.Client,
		ResolveOut:  func() io.Writer { return out },
		Sleep:       func(d time.Duration) { *sleeps = append(*sleeps, d) },
	}
}

func TestDoRetriesEmptyGetThenErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK) // 2xx with an empty body
	}))
	defer srv.Close()

	var out bytes.Buffer
	var sleeps []time.Duration
	_, err := testClient(srv, &out, &sleeps).Do(context.Background(), "tok", Request{Method: http.MethodGet, Path: "/me"})
	if err == nil || !strings.Contains(err.Error(), "empty response from API") {
		t.Fatalf("err = %v, want empty-response error", err)
	}
	if calls != len(retryBackoffs)+1 {
		t.Errorf("calls = %d, want %d (initial + retries)", calls, len(retryBackoffs)+1)
	}
	if len(sleeps) != len(retryBackoffs) {
		t.Errorf("sleeps = %d, want %d", len(sleeps), len(retryBackoffs))
	}
}

func TestDoRawGetAllowsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // empty body = legitimate empty file
	}))
	defer srv.Close()

	var out bytes.Buffer
	var sleeps []time.Duration
	body, err := testClient(srv, &out, &sleeps).Do(context.Background(), "tok",
		Request{Method: http.MethodGet, Path: "/content", Raw: true})
	if err != nil {
		t.Fatalf("Do(raw) err = %v, want nil", err)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
	if len(sleeps) != 0 {
		t.Errorf("sleeps = %d, want 0 (no empty-body retry for raw)", len(sleeps))
	}
}

func TestDoRetriesOn5xxThenSucceeds(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	var sleeps []time.Duration
	body, err := testClient(srv, &out, &sleeps).Do(context.Background(), "tok",
		Request{Method: http.MethodGet, Path: "/me"})
	if err != nil {
		t.Fatalf("Do err = %v, want nil", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
	if calls != 2 || len(sleeps) != 1 {
		t.Errorf("calls = %d, sleeps = %d, want 2 and 1", calls, len(sleeps))
	}
}

func TestEmitCoercesEmptyBodyToObject(t *testing.T) {
	var out bytes.Buffer
	c := &Client{Provider: "microsoft-test", ResolveOut: func() io.Writer { return &out }}
	if err := c.Emit([]byte("  ")); err != nil {
		t.Fatalf("Emit err = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "{}" {
		t.Errorf("emit = %q, want {}", got)
	}
}

func TestAPIErrorClassifiesCredentialRejection(t *testing.T) {
	c := &Client{Provider: "microsoft-test", APILabel: "microsoft-test API error", ScopeHint: " (scope hint)"}
	// 401 → credential rejection carrying the scope hint.
	err := c.APIError(http.StatusUnauthorized, "/me", []byte(`{"error":{"code":"InvalidAuthenticationToken"}}`))
	if !execution.IsCredentialRejected(err) {
		t.Errorf("401 err = %v, want a rejected-credential error", err)
	}
	if !strings.Contains(err.Error(), "scope hint") {
		t.Errorf("401 err = %v, want the scope hint", err)
	}
	// 403 (valid token, missing scope) → NOT a credential rejection.
	err = c.APIError(http.StatusForbidden, "/me", []byte(`{"error":{"code":"ErrorAccessDenied"}}`))
	if execution.IsCredentialRejected(err) {
		t.Errorf("403 err = %v, should not be a rejected-credential error", err)
	}
}
