package billcom

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeBill is an httptest-backed BILL API fake covering both the v3 gateway
// (login + resource calls) and the v2 login endpoint used by the sync-token
// model. Handlers assert request shape and record what they saw.
type fakeBill struct {
	t *testing.T

	// v3LoginBody captures the decoded JSON body posted to /login.
	v3LoginBody map[string]any
	// v2LoginForm captures the form values posted to /api/v2/Login.json.
	v2LoginForm url.Values
	// lastReq records the last resource request's method+path+headers+query.
	lastMethod  string
	lastPath    string
	lastQuery   url.Values
	lastDevKey  string
	lastSession string
	lastBody    []byte

	// sessionID is returned by the v3 login handler.
	sessionID string
	// v2SessionID is returned by the v2 login handler.
	v2SessionID string

	// expireOnce makes the next resource GET return 401 exactly once,
	// forcing a re-login-and-retry.
	expireOnce bool

	// resourceStatus / resourceBody override the resource handler response.
	resourceStatus int
	resourceBody   string
}

func newFakeBill(t *testing.T) (*fakeBill, *httptest.Server) {
	f := &fakeBill{t: t, sessionID: "SESSION-V3", v2SessionID: "SESSION-V2"}
	srv := httptest.NewServer(http.HandlerFunc(f.route))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeBill) route(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/login":
		body, _ := io.ReadAll(r.Body)
		m := map[string]any{}
		if err := json.Unmarshal(body, &m); err != nil {
			f.t.Fatalf("v3 login body not JSON: %v", err)
		}
		f.v3LoginBody = m
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			f.t.Fatalf("v3 login content-type = %q, want application/json", ct)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"sessionId":"` + f.sessionID + `"}`))

	case r.Method == http.MethodPost && r.URL.Path == "/api/v2/Login.json":
		_ = r.ParseForm()
		f.v2LoginForm = r.PostForm
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-www-form-urlencoded") {
			f.t.Fatalf("v2 login content-type = %q, want form-urlencoded", ct)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"response_status":0,"response_message":"Success","response_data":{"sessionId":"` + f.v2SessionID + `"}}`))

	default:
		// Resource call.
		if f.expireOnce {
			f.expireOnce = false
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"errors":[{"code":"BDC_1105","message":"session expired"}]}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.lastMethod = r.Method
		f.lastPath = r.URL.Path
		f.lastQuery = r.URL.Query()
		f.lastDevKey = r.Header.Get("devKey")
		f.lastSession = r.Header.Get("sessionId")
		f.lastBody = body
		status := f.resourceStatus
		if status == 0 {
			status = 200
		}
		respBody := f.resourceBody
		if respBody == "" {
			respBody = `{"results":[{"id":"x1"}],"nextPage":"P2"}`
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}
}

// creds returns a full valid v3 credential env map pointed at the fake server.
func creds(srv *httptest.Server) map[string]string {
	return map[string]string{
		"BILLCOM_DEV_KEY":  "DK",
		"BILLCOM_USERNAME": "user@example.com",
		"BILLCOM_PASSWORD": "pw",
		"BILLCOM_ORG_ID":   "ORG1",
	}
}

// run executes the service with BaseURL/LoginV2BaseURL pointed at srv and
// returns stdout, stderr, and the exit code.
func run(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	s := &Service{
		BaseURL:        srv.URL,
		LoginV2BaseURL: srv.URL,
		HC:             srv.Client(),
		Out:            &out,
		Err:            &errb,
	}
	res, err := s.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	return out.String(), errb.String(), res.ExitCode
}

func TestBillListLoginAndEnvelope(t *testing.T) {
	f, srv := newFakeBill(t)
	out, _, code := run(t, srv, creds(srv), "bill", "list", "--max", "5", "--page", "TOK", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, out)
	}
	// v3 login body carries all four fields.
	for _, k := range []string{"devKey", "username", "password", "organizationId"} {
		if _, ok := f.v3LoginBody[k]; !ok {
			t.Errorf("v3 login body missing %q: %v", k, f.v3LoginBody)
		}
	}
	if f.v3LoginBody["organizationId"] != "ORG1" {
		t.Errorf("organizationId = %v, want ORG1", f.v3LoginBody["organizationId"])
	}
	// Resource call headers + path + query.
	if f.lastPath != "/bills" {
		t.Errorf("path = %q, want /bills", f.lastPath)
	}
	if f.lastDevKey != "DK" || f.lastSession != "SESSION-V3" {
		t.Errorf("headers devKey=%q sessionId=%q", f.lastDevKey, f.lastSession)
	}
	if f.lastQuery.Get("max") != "5" || f.lastQuery.Get("page") != "TOK" {
		t.Errorf("query = %v", f.lastQuery)
	}
	// Normalized envelope on stdout.
	var env struct {
		Items    []map[string]any `json:"items"`
		NextPage string           `json:"next_page"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout not envelope JSON: %v; out=%s", err, out)
	}
	if len(env.Items) != 1 || env.Items[0]["id"] != "x1" {
		t.Errorf("items = %v", env.Items)
	}
	if env.NextPage != "P2" {
		t.Errorf("next_page = %q, want P2", env.NextPage)
	}
}

func TestBillGetRawBody(t *testing.T) {
	f, srv := newFakeBill(t)
	f.resourceBody = `{"id":"00n01","amount":42}`
	out, _, code := run(t, srv, creds(srv), "bill", "get", "00n01")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if f.lastMethod != http.MethodGet || f.lastPath != "/bills/00n01" {
		t.Errorf("got %s %s", f.lastMethod, f.lastPath)
	}
	if !strings.Contains(out, `"amount":42`) {
		t.Errorf("stdout = %q", out)
	}
}

func TestVendorCreatePostsBody(t *testing.T) {
	f, srv := newFakeBill(t)
	f.resourceBody = `{"id":"v99"}`
	out, _, code := run(t, srv, creds(srv), "vendor", "create", "--data", `{"name":"Acme"}`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, out)
	}
	if f.lastMethod != http.MethodPost || f.lastPath != "/vendors" {
		t.Errorf("got %s %s", f.lastMethod, f.lastPath)
	}
	var sent map[string]any
	if err := json.Unmarshal(f.lastBody, &sent); err != nil {
		t.Fatalf("posted body not JSON: %v", err)
	}
	if sent["name"] != "Acme" {
		t.Errorf("posted body = %v", sent)
	}
}

func TestPaymentHasNoCreate(t *testing.T) {
	_, srv := newFakeBill(t)
	_, _, code := run(t, srv, creds(srv), "payment", "create", "--data", "{}")
	if code != 2 {
		t.Fatalf("payment create exit = %d, want 2 (money-movement carve-out)", code)
	}
}

func TestPaymentListWorks(t *testing.T) {
	f, srv := newFakeBill(t)
	_, _, code := run(t, srv, creds(srv), "payment", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if f.lastPath != "/payments" {
		t.Errorf("path = %q, want /payments", f.lastPath)
	}
}

func TestWhoami(t *testing.T) {
	f, srv := newFakeBill(t)
	f.resourceBody = `{"organizationId":"ORG1","userId":"U1"}`
	out, _, code := run(t, srv, creds(srv), "whoami")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if f.lastPath != "/login/session" {
		t.Errorf("path = %q, want /login/session", f.lastPath)
	}
	if !strings.Contains(out, "ORG1") {
		t.Errorf("stdout = %q", out)
	}
}

func TestOrgList(t *testing.T) {
	f, srv := newFakeBill(t)
	f.resourceBody = `[{"orgId":"ORG1"}]`
	_, _, code := run(t, srv, creds(srv), "org", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if f.lastPath != "/organizations" {
		t.Errorf("path = %q, want /organizations", f.lastPath)
	}
}

func TestSyncTokenUsesV2Login(t *testing.T) {
	f, srv := newFakeBill(t)
	env := creds(srv)
	env["BILLCOM_AUTH_MODE"] = "sync_token"
	_, _, code := run(t, srv, env, "vendor", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if f.v2LoginForm == nil {
		t.Fatalf("v2 login was not called for sync_token mode")
	}
	for _, k := range []string{"userName", "password", "orgId", "devKey"} {
		if f.v2LoginForm.Get(k) == "" {
			t.Errorf("v2 login form missing %q: %v", k, f.v2LoginForm)
		}
	}
	// Resource call must ride the v2-minted session.
	if f.lastSession != "SESSION-V2" {
		t.Errorf("sessionId = %q, want SESSION-V2", f.lastSession)
	}
}

func TestSessionExpiryReloginRetry(t *testing.T) {
	f, srv := newFakeBill(t)
	f.expireOnce = true
	_, _, code := run(t, srv, creds(srv), "bill", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 after re-login-and-retry", code)
	}
	if f.lastPath != "/bills" {
		t.Errorf("retry path = %q, want /bills", f.lastPath)
	}
}

func TestAPIErrorPlainAndJSON(t *testing.T) {
	f, srv := newFakeBill(t)
	f.resourceStatus = 400
	f.resourceBody = `{"errors":[{"code":"BDC_1","message":"bad request"}]}`

	// Plain.
	_, errb, code := run(t, srv, creds(srv), "bill", "list")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb, "400") {
		t.Errorf("plain stderr = %q", errb)
	}

	// JSON envelope.
	f.resourceStatus = 400
	f.resourceBody = `{"errors":[{"code":"BDC_1","message":"bad request"}]}`
	_, errb2, code2 := run(t, srv, creds(srv), "bill", "list", "--json")
	if code2 != 1 {
		t.Fatalf("exit = %d, want 1", code2)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errb2), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v; stderr=%s", err, errb2)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Errorf("error envelope = %+v", env.Error)
	}
}

func TestMissingDevKeyExit1(t *testing.T) {
	_, srv := newFakeBill(t)
	env := creds(srv)
	delete(env, "BILLCOM_DEV_KEY")
	_, errb, code := run(t, srv, env, "bill", "list")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for missing dev key", code)
	}
	if !strings.Contains(strings.ToLower(errb), "dev") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestUnknownSubcommandExit2(t *testing.T) {
	_, srv := newFakeBill(t)
	_, _, code := run(t, srv, creds(srv), "bill", "frobnicate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", code)
	}
}

func TestCreateWithoutDataExit2(t *testing.T) {
	_, srv := newFakeBill(t)
	_, _, code := run(t, srv, creds(srv), "vendor", "create")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for missing --data", code)
	}
}

func TestCredentialsBlobIsParsed(t *testing.T) {
	f, srv := newFakeBill(t)
	// Helio's single-secret store projects the whole credential set as one
	// JSON blob in BILLCOM_CREDENTIALS (no individual env vars).
	env := map[string]string{
		"BILLCOM_CREDENTIALS": `{"dev_key":"DK","username":"u@e.com","password":"pw","organization_id":"ORG1","auth_mode":"sync_token"}`,
	}
	_, _, code := run(t, srv, env, "vendor", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for blob credentials", code)
	}
	// sync_token in the blob must route through the v2 login.
	if f.v2LoginForm == nil {
		t.Fatalf("blob auth_mode=sync_token did not use v2 login")
	}
	if f.v2LoginForm.Get("devKey") != "DK" || f.v2LoginForm.Get("orgId") != "ORG1" {
		t.Errorf("v2 login form = %v", f.v2LoginForm)
	}
	if f.lastSession != "SESSION-V2" {
		t.Errorf("sessionId = %q, want SESSION-V2", f.lastSession)
	}
}

func TestNewCommandTreeTraversable(t *testing.T) {
	s := &Service{}
	root := s.NewCommandTree()
	if root == nil {
		t.Fatal("NewCommandTree returned nil")
	}
	if root.Use != "bill-com" {
		t.Errorf("root.Use = %q, want bill-com", root.Use)
	}
	// Ensure the resource groups exist.
	want := map[string]bool{"bill": false, "vendor": false, "invoice": false, "customer": false, "payment": false, "org": false, "whoami": false}
	for _, c := range root.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("command tree missing %q", name)
		}
	}
}

var _ = io.Discard
