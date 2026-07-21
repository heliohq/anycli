package dataforseo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// creds is the login:password pair the tests inject as DATAFORSEO_CREDENTIALS.
const creds = "user@example.com:api-pass"

// capturedRequest records what the fake DataForSEO server saw.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   []byte
}

// newServer returns a single-route server that records the request and replies
// with the given status/response for every path. DataForSEO Live endpoints hit
// one path per invocation, so a single route suffices.
func newServer(t *testing.T, status int, response string, got *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

// okEnvelope wraps a single-task result array into a success DataForSEO
// envelope with the given top-level cost.
func okEnvelope(cost string, result string) string {
	return `{"version":"0.1","status_code":20000,"status_message":"Ok.","cost":` + cost +
		`,"tasks_count":1,"tasks_error":0,"tasks":[{"id":"t1","status_code":20000,"status_message":"Ok.","cost":` +
		cost + `,"result":` + result + `}]}`
}

// taskErrorEnvelope is HTTP 200 with a top-level success but a failing task.
func taskErrorEnvelope(taskCode int, taskMsg string) string {
	return `{"version":"0.1","status_code":20000,"status_message":"Ok.","cost":0,"tasks_count":1,"tasks_error":1,` +
		`"tasks":[{"id":"t1","status_code":` + itoa(taskCode) + `,"status_message":"` + taskMsg + `","cost":0,"result":null}]}`
}

// topErrorEnvelope is HTTP 200 with a top-level error status code.
func topErrorEnvelope(code int, msg string) string {
	return `{"version":"0.1","status_code":` + itoa(code) + `,"status_message":"` + msg + `","cost":0,"tasks_count":0,"tasks_error":1,"tasks":[]}`
}

func itoa(n int) string {
	return jsonNumber(n)
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// run executes one dataforseo command against srv with valid credentials.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	result, stdout, stderr := runCreds(t, srv, creds, args...)
	return result.ExitCode, stdout, stderr
}

// runCreds executes one command with a caller-chosen credential value.
func runCreds(t *testing.T, srv *httptest.Server, credential string, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), args, map[string]string{EnvCredentials: credential})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

// runResult is run but returns the full execution.Result (for rejection checks).
func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	return runCreds(t, srv, creds, args...)
}

// decodeBody unmarshals a captured request body into a slice of generic maps
// (DataForSEO POST bodies are always a JSON array of task objects).
func decodeBody(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		t.Fatalf("request body is not a JSON array of task objects: %v (%s)", err, raw)
	}
	return arr
}

// decodeOutput unmarshals stdout into the {cost, result} envelope.
func decodeOutput(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, stdout)
	}
	return m
}
