package expensify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// capturedRequest records what the fake Expensify Integration Server saw.
type capturedRequest struct {
	Method      string
	ContentType string
	// Job is the decoded requestJobDescription form field.
	Job map[string]any
	// RawJob is the undecoded requestJobDescription form value.
	RawJob string
}

// newServer returns a single-route server that records the request and replies
// with the given status/response. Expensify has exactly one endpoint, so a
// single route models the whole API.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		raw := values.Get("requestJobDescription")
		job := map[string]any{}
		_ = json.Unmarshal([]byte(raw), &job)
		*got = capturedRequest{
			Method:      r.Method,
			ContentType: r.Header.Get("Content-Type"),
			Job:         job,
			RawJob:      raw,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

const (
	testPartnerUserID     = "partner@example.com"
	testPartnerUserSecret = "S3CR3T_secret_value"
	testCredentials       = testPartnerUserID + ":" + testPartnerUserSecret
)

func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	result, out, errOut := runResult(t, srv, testCredentials, args...)
	return result.ExitCode, out, errOut
}

func runResult(t *testing.T, srv *httptest.Server, creds string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	env := map[string]string{}
	if creds != "" {
		env[EnvCredentials] = creds
	}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// credentialsOf extracts the credentials sub-object the service sent.
func credentialsOf(t *testing.T, got capturedRequest) map[string]any {
	t.Helper()
	creds, ok := got.Job["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("job has no credentials object: %v", got.Job)
	}
	return creds
}

// inputSettingsOf extracts the inputSettings sub-object the service sent.
func inputSettingsOf(t *testing.T, got capturedRequest) map[string]any {
	t.Helper()
	in, ok := got.Job["inputSettings"].(map[string]any)
	if !ok {
		t.Fatalf("job has no inputSettings object: %v", got.Job)
	}
	return in
}
