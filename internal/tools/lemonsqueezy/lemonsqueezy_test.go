package lemonsqueezy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records one request the fake server saw.
type capturedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	Auth        string
	Accept      string
	ContentType string
	Body        []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newServer is a fake Lemon Squeezy API: it answers each request from routes
// keyed by "METHOD /path" (path only, query ignored) and records every request.
func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			RawQuery:    r.URL.RawQuery,
			Auth:        r.Header.Get("Authorization"),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		})
		w.Header().Set("Content-Type", mediaType)
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = io.WriteString(w, s.body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","title":"Not Found"}]}`)
	}))
}

// run executes one lemon-squeezy invocation against srv with a fixed token and
// returns exit code, stdout, stderr.
func run(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL + "/v1", Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvAPIKey: "lsk_test_key"})
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res.ExitCode, out.String(), errBuf.String()
}

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

// assertHeaders checks the three required headers on a captured request.
func assertHeaders(t *testing.T, req *capturedRequest) {
	t.Helper()
	if req.Auth != "Bearer lsk_test_key" {
		t.Errorf("Authorization = %q, want %q", req.Auth, "Bearer lsk_test_key")
	}
	if req.Accept != mediaType {
		t.Errorf("Accept = %q, want %q", req.Accept, mediaType)
	}
	if req.ContentType != mediaType {
		t.Errorf("Content-Type = %q, want %q", req.ContentType, mediaType)
	}
}

func TestWhoami(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /v1/users/me": {status: 200, body: `{"data":{"type":"users","id":"42","attributes":{"name":"Ada","email":"ada@example.com"}}}`},
	})
	defer srv.Close()

	code, out, errStr := run(t, srv, "whoami")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "GET", "/v1/users/me")
	if req == nil {
		t.Fatal("no GET /v1/users/me request")
	}
	assertHeaders(t, req)
	if !strings.Contains(out, `"id":"42"`) {
		t.Errorf("stdout did not pass through the response: %s", out)
	}
}

func TestStoreListQuery(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /v1/stores": {status: 200, body: `{"data":[],"meta":{}}`},
	})
	defer srv.Close()

	code, _, errStr := run(t, srv, "store", "list", "--page", "2", "--page-size", "50",
		"--filter", "store_id=1", "--include", "products,orders")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "GET", "/v1/stores")
	if req == nil {
		t.Fatal("no GET /v1/stores request")
	}
	q := req.RawQuery
	for _, want := range []string{"page%5Bnumber%5D=2", "page%5Bsize%5D=50", "filter%5Bstore_id%5D=1", "include=products%2Corders"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q missing %q", q, want)
		}
	}
}

func TestStoreGetWithInclude(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /v1/stores/7": {status: 200, body: `{"data":{"type":"stores","id":"7"}}`},
	})
	defer srv.Close()

	code, out, errStr := run(t, srv, "store", "get", "7", "--include", "products")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "GET", "/v1/stores/7")
	if req == nil {
		t.Fatal("no GET /v1/stores/7 request")
	}
	if req.RawQuery != "include=products" {
		t.Errorf("query = %q, want include=products", req.RawQuery)
	}
	if !strings.Contains(out, `"id":"7"`) {
		t.Errorf("passthrough failed: %s", out)
	}
}

func TestCheckoutCreateForwardsBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /v1/checkouts": {status: 201, body: `{"data":{"type":"checkouts","id":"c1","attributes":{"url":"https://x.lemonsqueezy.com/checkout/c1"}}}`},
	})
	defer srv.Close()

	payload := `{"data":{"type":"checkouts","relationships":{"store":{"data":{"type":"stores","id":"1"}},"variant":{"data":{"type":"variants","id":"2"}}}}}`
	code, out, errStr := run(t, srv, "checkout", "create", "--data", payload)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "POST", "/v1/checkouts")
	if req == nil {
		t.Fatal("no POST /v1/checkouts request")
	}
	assertHeaders(t, req)
	var got, want map[string]any
	_ = json.Unmarshal(req.Body, &got)
	_ = json.Unmarshal([]byte(payload), &want)
	if gotV, _ := json.Marshal(got); string(gotV) != mustMarshal(want) {
		t.Errorf("forwarded body = %s, want %s", req.Body, payload)
	}
	if !strings.Contains(out, "checkout/c1") {
		t.Errorf("passthrough failed: %s", out)
	}
}

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestCreateRequiresData(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, errStr := run(t, srv, "customer", "create")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage); stderr=%s", code, errStr)
	}
	if !strings.Contains(errStr, "--data is required") {
		t.Errorf("stderr = %q, want required-data message", errStr)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call should be made on a usage error, got %d", len(reqs))
	}
}

func TestSubscriptionCancelDelete(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"DELETE /v1/subscriptions/9": {status: 200, body: `{"data":{"type":"subscriptions","id":"9","attributes":{"status":"cancelled"}}}`},
	})
	defer srv.Close()

	code, out, errStr := run(t, srv, "subscription", "cancel", "9")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	if findReq(reqs, "DELETE", "/v1/subscriptions/9") == nil {
		t.Fatal("no DELETE /v1/subscriptions/9 request")
	}
	if !strings.Contains(out, "cancelled") {
		t.Errorf("passthrough failed: %s", out)
	}
}

func TestOrderRefundOptionalData(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /v1/orders/5/refund": {status: 200, body: `{"data":{"type":"orders","id":"5","attributes":{"refunded":true}}}`},
	})
	defer srv.Close()

	// No --data → full refund; the request must still be issued.
	code, out, errStr := run(t, srv, "order", "refund", "5")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "POST", "/v1/orders/5/refund")
	if req == nil {
		t.Fatal("no POST /v1/orders/5/refund request")
	}
	if len(bytes.TrimSpace(req.Body)) != 0 {
		t.Errorf("body should be empty for a full refund, got %s", req.Body)
	}
	if !strings.Contains(out, `"refunded":true`) {
		t.Errorf("passthrough failed: %s", out)
	}
}

func TestGenerateInvoiceParamsBecomeQuery(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST /v1/orders/5/generate-invoice": {status: 200, body: `{"meta":{"urls":{"download_invoice":"https://app.lemonsqueezy.com/i"}}}`},
	})
	defer srv.Close()

	code, _, errStr := run(t, srv, "order", "invoice", "5", "--param", "name=Ada Corp", "--param", "country=US")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	req := findReq(reqs, "POST", "/v1/orders/5/generate-invoice")
	if req == nil {
		t.Fatal("no POST generate-invoice request")
	}
	if !strings.Contains(req.RawQuery, "name=Ada+Corp") || !strings.Contains(req.RawQuery, "country=US") {
		t.Errorf("query = %q, want name and country params", req.RawQuery)
	}
}

func TestSubscriptionItemCurrentUsage(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /v1/subscription-items/3/current-usage": {status: 200, body: `{"meta":{"period_start":"x","quantity":10}}`},
	})
	defer srv.Close()

	code, out, errStr := run(t, srv, "subscription-item", "current-usage", "3")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errStr)
	}
	if findReq(reqs, "GET", "/v1/subscription-items/3/current-usage") == nil {
		t.Fatal("no current-usage request")
	}
	if !strings.Contains(out, `"quantity":10`) {
		t.Errorf("passthrough failed: %s", out)
	}
}

func TestBadFilterIsUsageError(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()

	code, _, errStr := run(t, srv, "order", "list", "--filter", "broken")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage); stderr=%s", code, errStr)
	}
	if !strings.Contains(errStr, "--filter") {
		t.Errorf("stderr = %q, want filter usage message", errStr)
	}
	if len(reqs) != 0 {
		t.Errorf("no HTTP call on a usage error, got %d", len(reqs))
	}
}

func TestAPIErrorPlainAndJSON(t *testing.T) {
	routes := map[string]stub{
		"GET /v1/stores": {status: 403, body: `{"errors":[{"status":"403","title":"Forbidden","detail":"You are not allowed"}]}`},
	}

	// Plain text.
	var reqs []capturedRequest
	srv := newServer(t, &reqs, routes)
	defer srv.Close()
	code, out, errStr := run(t, srv, "store", "list")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (api); stderr=%s", code, errStr)
	}
	if out != "" {
		t.Errorf("stdout should be empty on error, got %s", out)
	}
	if !strings.Contains(errStr, "HTTP 403") || !strings.Contains(errStr, "You are not allowed") {
		t.Errorf("plain error = %q, want status + detail", errStr)
	}

	// --json envelope.
	var reqs2 []capturedRequest
	srv2 := newServer(t, &reqs2, routes)
	defer srv2.Close()
	code, _, errStr = run(t, srv2, "store", "list", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", code, errStr)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errStr)), &env); err != nil {
		t.Fatalf("stderr is not the JSON error envelope: %v (%s)", err, errStr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 403 {
		t.Errorf("envelope = %+v, want kind=api status=403", env.Error)
	}
}

func TestMissingTokenExits1(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"whoami"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAPIKey+" is not set") {
		t.Errorf("stderr = %q, want not-set message", errBuf.String())
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /v1/users/me": {status: 401, body: `{"errors":[{"status":"401","title":"Unauthorized"}]}`},
	})
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL + "/v1", Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"whoami"}, map[string]string{EnvAPIKey: "bad"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Error("a 401 must be classified as a credential rejection")
	}
}
