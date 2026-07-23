package freshbooks

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

// capturedRequest records one request the fake FreshBooks server received.
type capturedRequest struct {
	Method     string
	Path       string
	Auth       string
	APIVersion string
	Query      map[string][]string
	Body       []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status int
	body   string
}

// newServer builds a fake FreshBooks server that answers from routes keyed by
// "METHOD /path" and records every request. Unmatched routes return 404.
func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:     r.Method,
			Path:       r.URL.Path,
			Auth:       r.Header.Get("Authorization"),
			APIVersion: r.Header.Get("Api-Version"),
			Query:      r.URL.Query(),
			Body:       body,
		})
		w.Header().Set("Content-Type", "application/json")
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"response":{"errors":[{"message":"not found","errno":404}]}}`))
	}))
}

// run executes one freshbooks command against srv and returns the result plus
// captured stdout/stderr.
func run(t *testing.T, srv *httptest.Server, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "fb-token-123"})
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res.ExitCode, out.String(), errBuf.String()
}

const meSingleAccount = `{"response":{"id":42,"first_name":"Ada","email":"ada@x.com",
	"business_memberships":[{"business":{"id":7,"name":"Biz","account_id":"AcCt1"}}]}}`

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

func TestMeEmitsIdentityWithBearerAndVersion(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me": {200, meSingleAccount},
	})
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "me")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, "GET", "/auth/api/v1/users/me")
	if req == nil {
		t.Fatal("me endpoint was not called")
	}
	if req.Auth != "Bearer fb-token-123" {
		t.Errorf("Authorization = %q", req.Auth)
	}
	if req.APIVersion != "alpha" {
		t.Errorf("Api-Version = %q, want alpha", req.APIVersion)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, stdout)
	}
	if got["email"] != "ada@x.com" {
		t.Errorf("me output missing email: %s", stdout)
	}
}

func TestInvoiceListUnwrapsEnvelope(t *testing.T) {
	var reqs []capturedRequest
	list := `{"response":{"result":{"invoices":[{"id":1},{"id":2}],"page":1,"pages":3,"per_page":15,"total":42}}}`
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me":                       {200, meSingleAccount},
		"GET /accounting/account/AcCt1/invoices/invoices": {200, list},
	})
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "invoice", "list")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	var env struct {
		Items   []map[string]any `json:"items"`
		Page    int              `json:"page"`
		Pages   int              `json:"pages"`
		PerPage int              `json:"per_page"`
		Total   int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not the neutral envelope: %v (%s)", err, stdout)
	}
	if len(env.Items) != 2 || env.Page != 1 || env.Pages != 3 || env.PerPage != 15 || env.Total != 42 {
		t.Errorf("envelope mismatch: %+v", env)
	}
}

func TestAccountFlagShortCircuitsIdentityCall(t *testing.T) {
	var reqs []capturedRequest
	list := `{"response":{"result":{"invoices":[],"page":1,"pages":1,"per_page":15,"total":0}}}`
	srv := newServer(t, &reqs, map[string]stub{
		"GET /accounting/account/Explicit/invoices/invoices": {200, list},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "invoice", "list", "--account", "Explicit")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if findReq(reqs, "GET", "/auth/api/v1/users/me") != nil {
		t.Error("--account should skip the identity call")
	}
}

func TestMultiAccountFailsFastExit2(t *testing.T) {
	var reqs []capturedRequest
	multi := `{"response":{"id":1,"business_memberships":[
		{"business":{"account_id":"A1"}},{"business":{"account_id":"A2"}}]}}`
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me": {200, multi},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "invoice", "list")
	if code != 2 {
		t.Fatalf("multi-account exit=%d, want 2; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "A1") || !strings.Contains(stderr, "A2") || !strings.Contains(stderr, "--account") {
		t.Errorf("multi-account error should list ids and --account guidance: %s", stderr)
	}
}

func TestZeroAccountsFailsExit1(t *testing.T) {
	var reqs []capturedRequest
	none := `{"response":{"id":1,"business_memberships":[]}}`
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me": {200, none},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "invoice", "list")
	if code != 1 {
		t.Fatalf("zero-account exit=%d, want 1; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "no accounting account") {
		t.Errorf("zero-account error unclear: %s", stderr)
	}
}

func TestClientCreateWrapsPayloadAndUnwrapsResult(t *testing.T) {
	var reqs []capturedRequest
	created := `{"response":{"result":{"client":{"id":99,"organization":"Acme"}}}}`
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me":                    {200, meSingleAccount},
		"POST /accounting/account/AcCt1/users/clients": {200, created},
	})
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "client", "create", "--data", `{"organization":"Acme","email":"c@x.com"}`)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, "POST", "/accounting/account/AcCt1/users/clients")
	if req == nil {
		t.Fatal("create endpoint not called")
	}
	var sent map[string]any
	if err := json.Unmarshal(req.Body, &sent); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	client, ok := sent["client"].(map[string]any)
	if !ok {
		t.Fatalf("payload not wrapped in {client:...}: %s", req.Body)
	}
	if client["organization"] != "Acme" {
		t.Errorf("wrapped payload missing fields: %s", req.Body)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, stdout)
	}
	if out["id"] != float64(99) {
		t.Errorf("output should be the unwrapped client object: %s", stdout)
	}
}

func TestInvoiceSendActionEmail(t *testing.T) {
	var reqs []capturedRequest
	sent := `{"response":{"result":{"invoice":{"id":5,"v3_status":"sent"}}}}`
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me":                         {200, meSingleAccount},
		"PUT /accounting/account/AcCt1/invoices/invoices/5": {200, sent},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "invoice", "send", "5", "--to", "buyer@x.com")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, "PUT", "/accounting/account/AcCt1/invoices/invoices/5")
	if req == nil {
		t.Fatal("send endpoint not called")
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("send body not JSON: %v", err)
	}
	inv := body["invoice"]
	if inv["action_email"] != true {
		t.Errorf("send should set action_email=true: %s", req.Body)
	}
	if rcpts, _ := inv["email_recipients"].([]any); len(rcpts) != 1 || rcpts[0] != "buyer@x.com" {
		t.Errorf("send should carry the recipient: %s", req.Body)
	}
}

func TestInvoiceDeleteSoftDeletes(t *testing.T) {
	var reqs []capturedRequest
	del := `{"response":{"result":{"invoice":{"id":5,"vis_state":1}}}}`
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me":                         {200, meSingleAccount},
		"PUT /accounting/account/AcCt1/invoices/invoices/5": {200, del},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "invoice", "delete", "5")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, "PUT", "/accounting/account/AcCt1/invoices/invoices/5")
	var body map[string]map[string]any
	_ = json.Unmarshal(req.Body, &body)
	if body["invoice"]["vis_state"] != float64(1) {
		t.Errorf("delete should set vis_state=1: %s", req.Body)
	}
}

func TestJSONErrorRendersStatusAndCode(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me": {200, meSingleAccount},
		"GET /accounting/account/AcCt1/invoices/invoices/9": {
			422, `{"response":{"errors":[{"message":"bad id","errno":1012}]}}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "--json", "invoice", "get", "9")
	if code != 1 {
		t.Fatalf("API error exit=%d, want 1; stderr=%s", code, stderr)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Status  int    `json:"status"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("--json error not structured: %v (%s)", err, stderr)
	}
	if env.Error.Status != 422 || env.Error.Code != "1012" || !strings.Contains(env.Error.Message, "bad id") {
		t.Errorf("error envelope mismatch: %+v", env.Error)
	}
}

func TestBadDataIsUsageExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me": {200, meSingleAccount},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "client", "create", "--data", `{not-json`)
	if code != 2 {
		t.Fatalf("bad --data exit=%d, want 2; stderr=%s", code, stderr)
	}
	// The identity call must not fire before the payload is validated.
	if findReq(reqs, "GET", "/auth/api/v1/users/me") != nil {
		t.Error("payload should be validated before any network call")
	}
}

func TestUnauthorizedIsCredentialRejection(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET /auth/api/v1/users/me": {401, `{"error":"unauthenticated","error_description":"bad token"}`},
	})
	defer srv.Close()

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{EnvToken: "bad"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Errorf("401 should be exit 1 + credential rejection, got %+v; stderr=%s", res, errBuf.String())
	}
}

func TestMissingTokenExits1(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("missing token exit=%d, want 1", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "FRESHBOOKS_TOKEN") {
		t.Errorf("missing-token message unclear: %s", errBuf.String())
	}
}

func TestItemListReadOnlyHasNoCreate(t *testing.T) {
	tree := (&Service{}).NewCommandTree()
	for _, c := range tree.Commands() {
		if c.Name() != "item" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "create" || sub.Name() == "update" || sub.Name() == "delete" {
				t.Errorf("item must be read-only, found verb %q", sub.Name())
			}
		}
	}
}
