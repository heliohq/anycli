package netsuite

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const validCreds = `{"account_id":"9876543_SB1","consumer_key":"ck","consumer_secret":"cs","token_id":"ti","token_secret":"ts"}`

// run executes the service against a fake server with fixed signing inputs and
// captures exit code, stdout, and stderr.
func run(t *testing.T, server *httptest.Server, credsJSON string, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{
		Out:     &out,
		Err:     &errBuf,
		nowFn:   func() time.Time { return time.Unix(1700000000, 0) },
		nonceFn: func() string { return "fixed-nonce" },
	}
	if server != nil {
		svc.BaseURL = server.URL
		svc.HC = server.Client()
	}
	env := map[string]string{}
	if credsJSON != "" {
		env[EnvCredentials] = credsJSON
	}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a Go error (should be nil, exit code carries failure): %v", err)
	}
	return res.ExitCode, out.String(), errBuf.String()
}

func newServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestQuerySetsPreferTransientAndFoldsPagination(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/query/v1/suiteql") {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Prefer"); got != "transient" {
			t.Errorf("Prefer header = %q, want transient", got)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit = %q, want 5", got)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "OAuth ") {
			t.Errorf("missing OAuth Authorization header: %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		if !strings.Contains(payload["q"], "SELECT") {
			t.Errorf("body q = %q", payload["q"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[{"id":"1"}],"hasMore":false}`))
	})
	defer server.Close()

	code, out, errOut := run(t, server, validCreds, "query", "--q", "SELECT id FROM customer", "--limit", "5")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, `"items"`) {
		t.Errorf("stdout = %q", out)
	}
}

func TestQueryRequiresStatement(t *testing.T) {
	code, _, errOut := run(t, nil, validCreds, "query")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage); stderr = %q", code, errOut)
	}
	if !strings.Contains(errOut, "--q") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRecordGetPathAndAuth(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/record/v1/customer/42" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"42","companyName":"ACME"}`))
	})
	defer server.Close()

	code, out, errOut := run(t, server, validCreds, "record", "get", "--type", "customer", "--id", "42")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "ACME") {
		t.Errorf("stdout = %q", out)
	}
}

func TestRecordCreateSurfacesLocationID(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/record/v1/customer" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Location", "https://x.suitetalk.api.netsuite.com/services/rest/record/v1/customer/999")
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	code, out, errOut := run(t, server, validCreds, "record", "create", "--type", "customer", "--body", `{"companyName":"New"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, `"id":"999"`) {
		t.Errorf("stdout should surface new internal id: %q", out)
	}
}

func TestRecordCreateRejectsInvalidBody(t *testing.T) {
	code, _, errOut := run(t, nil, validCreds, "record", "create", "--type", "customer", "--body", "{not json}")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %q", code, errOut)
	}
	if !strings.Contains(errOut, "not valid JSON") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestUnauthorizedIsCredentialRejectionExit1(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"title":"Invalid login attempt."}`))
	})
	defer server.Close()

	code, _, errOut := run(t, server, validCreds, "record", "get", "--type", "customer", "--id", "1")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (runtime/API); stderr = %q", code, errOut)
	}
	if !strings.Contains(errOut, "Invalid login") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestTooManyRequestsEchoesRetryAfterOnlyWhenPresent(t *testing.T) {
	// With Retry-After present.
	withHeader := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"title":"concurrency limit"}`))
	})
	defer withHeader.Close()
	code, _, errOut := run(t, withHeader, validCreds, "--json", "query", "--q", "SELECT 1")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr = %q", code, errOut)
	}
	if !strings.Contains(errOut, `"retry_after":"30"`) {
		t.Errorf("stderr should carry retry_after: %q", errOut)
	}

	// Without Retry-After: field omitted.
	noHeader := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"title":"concurrency limit"}`))
	})
	defer noHeader.Close()
	code, _, errOut = run(t, noHeader, validCreds, "--json", "query", "--q", "SELECT 1")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr = %q", code, errOut)
	}
	if strings.Contains(errOut, "retry_after") {
		t.Errorf("retry_after should be absent when NetSuite omits it: %q", errOut)
	}
}

func TestMissingCredentialsIsUsageExit2(t *testing.T) {
	code, _, errOut := run(t, nil, "", "query", "--q", "SELECT 1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %q", code, errOut)
	}
	if !strings.Contains(errOut, "NETSUITE_CREDENTIALS is not set") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestMalformedCredentialsJSONIsUsageExit2(t *testing.T) {
	code, _, errOut := run(t, nil, `{"account_id":`, "query", "--q", "SELECT 1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %q", code, errOut)
	}
	if !strings.Contains(errOut, "not valid JSON") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestEmptySubFieldIsUsageExit2(t *testing.T) {
	code, _, errOut := run(t, nil, `{"account_id":"9876543_SB1","consumer_key":"","consumer_secret":"cs","token_id":"ti","token_secret":"ts"}`, "query", "--q", "SELECT 1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %q", code, errOut)
	}
	if !strings.Contains(errOut, "consumer_key") {
		t.Errorf("stderr should name the missing field: %q", errOut)
	}
}

func TestDecodeCredsJSONErrorEnvelope(t *testing.T) {
	code, _, errOut := run(t, nil, "", "--json", "query", "--q", "SELECT 1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %q", code, errOut)
	}
	var env struct {
		Error struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %q (%v)", errOut, err)
	}
	if env.Error.Kind != "usage" {
		t.Errorf("kind = %q, want usage", env.Error.Kind)
	}
}
