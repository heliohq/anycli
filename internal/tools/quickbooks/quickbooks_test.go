package quickbooks

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

const testRealm = "9130347597"

// capturedRequest records one request the fake QBO server received.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	Accept      string
	ContentType string
	Query       map[string][]string
	Body        []byte
}

// newFake is a fake QuickBooks Online server: it records every request and
// answers matched "METHOD /path" routes, defaulting to 200 {} otherwise.
func newFake(t *testing.T, reqs *[]capturedRequest, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        raw,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// run executes one quickbooks command against a fake server and returns stdout,
// stderr, and the exit code.
func run(t *testing.T, base string, args []string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	svc := &Service{BaseURL: base, Out: &out, Err: &errb}
	env := map[string]string{EnvAccessToken: "tok-abc", EnvRealmID: testRealm}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return out.String(), errb.String(), res.ExitCode
}

func TestCompanyGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{"CompanyInfo":{"CompanyName":"Acme"}}`)
	defer srv.Close()

	out, _, code := run(t, srv.URL, []string{"company", "get"})
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	r := reqs[0]
	wantPath := "/v3/company/" + testRealm + "/companyinfo/" + testRealm
	if r.Method != http.MethodGet || r.Path != wantPath {
		t.Fatalf("got %s %s, want GET %s", r.Method, r.Path, wantPath)
	}
	if got := r.Query["minorversion"]; len(got) != 1 || got[0] != minorVersion {
		t.Fatalf("minorversion=%v want [%s]", got, minorVersion)
	}
	if r.Auth != "Bearer tok-abc" {
		t.Fatalf("Authorization=%q want Bearer tok-abc", r.Auth)
	}
	if r.Accept != "application/json" {
		t.Fatalf("Accept=%q want application/json", r.Accept)
	}
	if !strings.Contains(out, "Acme") {
		t.Fatalf("stdout %q missing company payload", out)
	}
}

func TestQueryPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{"QueryResponse":{}}`)
	defer srv.Close()

	sql := "select * from Invoice where Balance > '0'"
	_, _, code := run(t, srv.URL, []string{"query", "--sql", sql})
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	r := reqs[0]
	if r.Path != "/v3/company/"+testRealm+"/query" {
		t.Fatalf("path=%s want query endpoint", r.Path)
	}
	if got := r.Query["query"]; len(got) != 1 || got[0] != sql {
		t.Fatalf("query param=%v want [%s]", got, sql)
	}
}

func TestEntityListBuildsSelect(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{"QueryResponse":{}}`)
	defer srv.Close()

	_, _, code := run(t, srv.URL, []string{"customer", "list", "--where", "Active = true", "--max", "10", "--start-position", "1"})
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	q := reqs[0].Query["query"]
	if len(q) != 1 {
		t.Fatalf("query param=%v", q)
	}
	want := "select * from Customer where Active = true startposition 1 maxresults 10"
	if q[0] != want {
		t.Fatalf("query=%q want %q", q[0], want)
	}
}

func TestEntityGetByID(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{"Customer":{"Id":"42"}}`)
	defer srv.Close()

	_, _, code := run(t, srv.URL, []string{"customer", "get", "--id", "42"})
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if reqs[0].Path != "/v3/company/"+testRealm+"/customer/42" {
		t.Fatalf("path=%s want customer/42", reqs[0].Path)
	}
}

func TestEntityCreatePostsBody(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{"Invoice":{"Id":"7"}}`)
	defer srv.Close()

	body := `{"Line":[{"Amount":100.0,"DetailType":"SalesItemLineDetail"}],"CustomerRef":{"value":"1"}}`
	_, _, code := run(t, srv.URL, []string{"invoice", "create", "--json-body", body})
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/v3/company/"+testRealm+"/invoice" {
		t.Fatalf("got %s %s want POST invoice", r.Method, r.Path)
	}
	if r.ContentType != "application/json" {
		t.Fatalf("Content-Type=%q want application/json", r.ContentType)
	}
	var sent map[string]any
	if err := json.Unmarshal(r.Body, &sent); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, r.Body)
	}
	if _, ok := sent["Line"]; !ok {
		t.Fatalf("body missing Line: %s", r.Body)
	}
}

func TestInvoiceSend(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{"Invoice":{"Id":"7"}}`)
	defer srv.Close()

	_, _, code := run(t, srv.URL, []string{"invoice", "send", "--id", "7", "--to", "cfo@acme.com"})
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/v3/company/"+testRealm+"/invoice/7/send" {
		t.Fatalf("got %s %s want POST invoice/7/send", r.Method, r.Path)
	}
	if got := r.Query["sendTo"]; len(got) != 1 || got[0] != "cfo@acme.com" {
		t.Fatalf("sendTo=%v want [cfo@acme.com]", got)
	}
}

func TestReportGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{"Header":{"ReportName":"ProfitAndLoss"}}`)
	defer srv.Close()

	_, _, code := run(t, srv.URL, []string{"report", "get", "--name", "ProfitAndLoss", "--start-date", "2024-01-01", "--end-date", "2024-12-31", "--date-macro", "This Fiscal Year"})
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	r := reqs[0]
	if r.Path != "/v3/company/"+testRealm+"/reports/ProfitAndLoss" {
		t.Fatalf("path=%s want reports/ProfitAndLoss", r.Path)
	}
	if r.Query["start_date"][0] != "2024-01-01" || r.Query["end_date"][0] != "2024-12-31" {
		t.Fatalf("date params=%v", r.Query)
	}
	if r.Query["date_macro"][0] != "This Fiscal Year" {
		t.Fatalf("date_macro=%v", r.Query["date_macro"])
	}
}

func TestFaultErrorPlain(t *testing.T) {
	var reqs []capturedRequest
	faultBody := `{"Fault":{"Error":[{"Message":"Object Not Found","Detail":"Object Not Found : Something","code":"610"}],"type":"ValidationFault"},"time":"2024-01-01T00:00:00.000-08:00"}`
	srv := newFake(t, &reqs, http.StatusBadRequest, faultBody)
	defer srv.Close()

	_, errOut, code := run(t, srv.URL, []string{"customer", "get", "--id", "999"})
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	if !strings.Contains(errOut, "610") || !strings.Contains(errOut, "Object Not Found") {
		t.Fatalf("stderr %q missing fault code/message", errOut)
	}
}

func TestFaultErrorJSON(t *testing.T) {
	var reqs []capturedRequest
	faultBody := `{"Fault":{"Error":[{"Message":"Invalid Reference","Detail":"bad ref","code":"2500"}],"type":"ValidationFault"}}`
	srv := newFake(t, &reqs, http.StatusBadRequest, faultBody)
	defer srv.Close()

	_, errOut, code := run(t, srv.URL, []string{"--json", "invoice", "create", "--json-body", "{}"})
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
			Fault  []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Detail  string `json:"detail"`
			} `json:"fault"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, errOut)
	}
	if env.Error.Kind != "api" || env.Error.Status != http.StatusBadRequest {
		t.Fatalf("envelope kind/status = %q/%d", env.Error.Kind, env.Error.Status)
	}
	if len(env.Error.Fault) != 1 || env.Error.Fault[0].Code != "2500" {
		t.Fatalf("fault array = %+v", env.Error.Fault)
	}
}

func TestUsageErrorExit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newFake(t, &reqs, http.StatusOK, `{}`)
	defer srv.Close()

	// Missing --id on get is a usage error → exit 2, no request made.
	_, _, code := run(t, srv.URL, []string{"customer", "get"})
	if code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if len(reqs) != 0 {
		t.Fatalf("made %d requests on usage error, want 0", len(reqs))
	}
}

func TestMissingRealmExit1(t *testing.T) {
	var out, errb bytes.Buffer
	svc := &Service{Out: &out, Err: &errb}
	env := map[string]string{EnvAccessToken: "tok"}
	res, err := svc.Execute(context.Background(), []string{"company", "get"}, env)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit=%d want 1", res.ExitCode)
	}
	if !strings.Contains(errb.String(), "QUICKBOOKS_REALM_ID") {
		t.Fatalf("stderr %q missing realm hint", errb.String())
	}
}

// TestMissingCredentialJSONKindMatchesExit pins the emitted error kind to the
// exit code for the missing-credential case. Absent credentials are a runtime/
// environment failure (the connection was never injected), so it is exit 1 —
// and under --json the kind must agree ("api" = the API/runtime category), not
// "usage" (which would imply exit 2 and a caller-fixable flag mistake).
func TestMissingCredentialJSONKindMatchesExit(t *testing.T) {
	var out, errb bytes.Buffer
	svc := &Service{Out: &out, Err: &errb}
	// Missing access token, --json requested.
	res, err := svc.Execute(context.Background(), []string{"--json", "company", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit=%d want 1", res.ExitCode)
	}
	var env struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errb.String())), &env); err != nil {
		t.Fatalf("stderr not JSON envelope: %v (%s)", err, errb.String())
	}
	if env.Error.Kind != "api" {
		t.Fatalf("kind=%q want %q (must agree with exit 1)", env.Error.Kind, "api")
	}
}

func TestBaseURLForSandbox(t *testing.T) {
	if got := baseURLFor("sandbox"); got != sandboxBaseURL {
		t.Fatalf("sandbox base=%q want %q", got, sandboxBaseURL)
	}
	if got := baseURLFor(""); got != prodBaseURL {
		t.Fatalf("default base=%q want %q", got, prodBaseURL)
	}
	if got := baseURLFor("production"); got != prodBaseURL {
		t.Fatalf("production base=%q want %q", got, prodBaseURL)
	}
}
