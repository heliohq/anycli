package razorpay

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const testToken = "rzp-oauth-access-token"

// capture holds the fields a fake server records for one request, so tests can
// assert request shape.
type capture struct {
	method string
	path   string
	query  string
	auth   string
	accept string
}

// newFakeServer returns an httptest server that records the request into rec
// and replies with status + body.
func newFakeServer(t *testing.T, rec *capture, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.query = r.URL.RawQuery
		rec.auth = r.Header.Get("Authorization")
		rec.accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// run executes the service against a fake server and returns stdout, stderr,
// and the result.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, execution.Result) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvAccessToken: testToken})
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return out.String(), errBuf.String(), res
}

func TestListEveryResourceHitsCollectionPathWithBearer(t *testing.T) {
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"payment", "list"}, "/payments"},
		{[]string{"order", "list"}, "/orders"},
		{[]string{"refund", "list"}, "/refunds"},
		{[]string{"customer", "list"}, "/customers"},
		{[]string{"payment-link", "list"}, "/payment_links"},
		{[]string{"settlement", "list"}, "/settlements"},
		{[]string{"subscription", "list"}, "/subscriptions"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			var rec capture
			srv := newFakeServer(t, &rec, http.StatusOK, `{"entity":"collection","count":0,"items":[]}`)
			out, errBuf, res := run(t, srv, tc.args...)
			if res.ExitCode != 0 {
				t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errBuf)
			}
			if rec.method != http.MethodGet {
				t.Errorf("method = %q, want GET", rec.method)
			}
			if rec.path != tc.path {
				t.Errorf("path = %q, want %q", rec.path, tc.path)
			}
			if rec.auth != "Bearer "+testToken {
				t.Errorf("Authorization = %q, want Bearer %s", rec.auth, testToken)
			}
			if rec.accept != "application/json" {
				t.Errorf("Accept = %q, want application/json", rec.accept)
			}
			if !strings.Contains(out, `"entity":"collection"`) {
				t.Errorf("stdout did not pass the collection envelope through verbatim: %q", out)
			}
		})
	}
}

func TestGetHitsResourcePathWithID(t *testing.T) {
	var rec capture
	srv := newFakeServer(t, &rec, http.StatusOK, `{"id":"pay_123","entity":"payment"}`)
	_, errBuf, res := run(t, srv, "payment", "get", "pay_123")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errBuf)
	}
	if rec.path != "/payments/pay_123" {
		t.Errorf("path = %q, want /payments/pay_123", rec.path)
	}
	if rec.method != http.MethodGet {
		t.Errorf("method = %q, want GET", rec.method)
	}
}

func TestListForwardsPaginationAndWindowParams(t *testing.T) {
	var rec capture
	srv := newFakeServer(t, &rec, http.StatusOK, `{"entity":"collection","count":2,"items":[]}`)
	_, errBuf, res := run(t, srv, "payment", "list", "--count", "2", "--skip", "10", "--from", "1600000000", "--to", "1700000000")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, errBuf)
	}
	q := rec.query
	for _, want := range []string{"count=2", "skip=10", "from=1600000000", "to=1700000000"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q missing %q", q, want)
		}
	}
}

func TestListOmitsUnsetPaginationParams(t *testing.T) {
	var rec capture
	srv := newFakeServer(t, &rec, http.StatusOK, `{"entity":"collection","count":0,"items":[]}`)
	_, _, res := run(t, srv, "order", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	if rec.query != "" {
		t.Errorf("query = %q, want empty when no pagination flags are set", rec.query)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var rec capture
	body := `{"error":{"code":"BAD_REQUEST_ERROR","description":"The api key/secret provided is invalid"}}`
	srv := newFakeServer(t, &rec, http.StatusUnauthorized, body)
	_, errBuf, res := run(t, srv, "payment", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("401 did not classify the credential as rejected")
	}
	if !strings.Contains(errBuf, "The api key/secret provided is invalid") {
		t.Errorf("stderr = %q, want provider description", errBuf)
	}
}

func TestAPIErrorExitOneWithoutCredentialRejection(t *testing.T) {
	var rec capture
	body := `{"error":{"code":"SERVER_ERROR","description":"We are facing a technical issue"}}`
	srv := newFakeServer(t, &rec, http.StatusInternalServerError, body)
	_, errBuf, res := run(t, srv, "payment", "list")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if res.CredentialRejected {
		t.Error("a 500 must not reject the credential")
	}
	if !strings.Contains(errBuf, "SERVER_ERROR") {
		t.Errorf("stderr = %q, want error code", errBuf)
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	var rec capture
	body := `{"error":{"code":"BAD_REQUEST_ERROR","description":"id is not a valid id"}}`
	srv := newFakeServer(t, &rec, http.StatusBadRequest, body)
	_, errBuf, res := run(t, srv, "payment", "get", "nope", "--json")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf)), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %q (%v)", errBuf, err)
	}
	if envelope.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", envelope.Error.Kind)
	}
	if envelope.Error.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", envelope.Error.Status)
	}
}

func TestMissingTokenIsRuntimeFailure(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"payment", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "RAZORPAY_ACCESS_TOKEN is not set") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	var rec capture
	srv := newFakeServer(t, &rec, http.StatusOK, `{}`)
	_, _, res := run(t, srv, "payment", "list", "--nope")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for a usage error", res.ExitCode)
	}
	if rec.method != "" {
		t.Error("a usage error must not reach the provider")
	}
}

func TestGetRequiresID(t *testing.T) {
	var rec capture
	srv := newFakeServer(t, &rec, http.StatusOK, `{}`)
	_, _, res := run(t, srv, "refund", "get")
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 when id is missing", res.ExitCode)
	}
	if rec.method != "" {
		t.Error("missing id must not reach the provider")
	}
}
