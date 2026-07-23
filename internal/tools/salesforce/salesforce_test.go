package salesforce

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

	"github.com/heliohq/anycli/internal/tools/execution"
)

const (
	testToken    = "00Dxx-access-token"
	testInstance = "https://acme.my.salesforce.com"
)

type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	Accept      string
	ContentType string
	Query       url.Values
	Body        []byte
}

// fakeServer routes by path so pagination (a second GET to nextRecordsUrl) and
// per-endpoint bodies can be exercised. handler returns (status, body).
func fakeServer(t *testing.T, got *capturedRequest, handler func(r *http.Request, body []byte) (int, string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			Accept:      r.Header.Get("Accept"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.Query(),
			Body:        body,
		}
		status, resp := handler(r, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
}

// staticServer answers every request with one status + body.
func staticServer(t *testing.T, got *capturedRequest, status int, resp string) *httptest.Server {
	return fakeServer(t, got, func(_ *http.Request, _ []byte) (int, string) { return status, resp })
}

func run(t *testing.T, srv *httptest.Server, args ...string) (int, string, string) {
	t.Helper()
	result, out, errOut := runResult(t, srv, args...)
	return result.ExitCode, out, errOut
}

func runResult(t *testing.T, srv *httptest.Server, args ...string) (execution.Result, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	env := map[string]string{EnvAccessToken: testToken, EnvInstanceURL: testInstance}
	result, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	return result, out.String(), errBuf.String()
}

func assertAuth(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.Auth != "Bearer "+testToken {
		t.Errorf("Authorization = %q, want Bearer %s", got.Auth, testToken)
	}
}

func TestQuerySinglePage(t *testing.T) {
	var got capturedRequest
	resp := `{"totalSize":1,"done":true,"records":[{"Id":"001","Name":"Acme"}]}`
	srv := staticServer(t, &got, 200, resp)
	defer srv.Close()

	code, out, errOut := run(t, srv, "query", "SELECT Id, Name FROM Account LIMIT 1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	assertAuth(t, got)
	if got.Path != "/services/data/v65.0/query/" {
		t.Errorf("path = %q, want /services/data/v65.0/query/", got.Path)
	}
	if q := got.Query.Get("q"); q != "SELECT Id, Name FROM Account LIMIT 1" {
		t.Errorf("q = %q", q)
	}
	var parsed queryPage
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, out)
	}
	if parsed.TotalSize != 1 || len(parsed.Records) != 1 || !parsed.Done {
		t.Errorf("merged result = %+v", parsed)
	}
}

func TestQueryFollowsPagination(t *testing.T) {
	var got capturedRequest
	nextPath := "/services/data/v65.0/query/01g0-2000"
	srv := fakeServer(t, &got, func(r *http.Request, _ []byte) (int, string) {
		if r.URL.Path == nextPath {
			return 200, `{"totalSize":2,"done":true,"records":[{"Id":"002"}]}`
		}
		return 200, `{"totalSize":2,"done":false,"nextRecordsUrl":"` + nextPath + `","records":[{"Id":"001"}]}`
	})
	defer srv.Close()

	code, out, errOut := run(t, srv, "query", "SELECT Id FROM Account")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	var parsed queryPage
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if len(parsed.Records) != 2 || !parsed.Done {
		t.Errorf("expected 2 accumulated records done=true, got %+v", parsed)
	}
}

func TestQueryAllUsesQueryAllResource(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{"totalSize":0,"done":true,"records":[]}`)
	defer srv.Close()
	if code, _, e := run(t, srv, "query", "--all", "SELECT Id FROM Account"); code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if got.Path != "/services/data/v65.0/queryAll/" {
		t.Errorf("path = %q, want queryAll", got.Path)
	}
}

func TestSearchPostsParameterizedBody(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{"searchRecords":[]}`)
	defer srv.Close()

	code, _, errOut := run(t, srv, "search", "Acme", "--objects", "Account,Contact", "--fields", "Id,Name", "--limit", "10")
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, errOut)
	}
	if got.Method != http.MethodPost || got.Path != "/services/data/v65.0/parameterizedSearch" {
		t.Errorf("method/path = %s %s", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q", got.ContentType)
	}
	var body searchRequest
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body.Q != "Acme" || len(body.SObjects) != 2 || body.SObjects[0].Name != "Account" || body.OverallLimit != 10 {
		t.Errorf("search body = %+v", body)
	}
}

func TestRecordGetWithFields(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{"Id":"001","Name":"Acme"}`)
	defer srv.Close()
	if code, _, e := run(t, srv, "record", "get", "Account", "001", "--fields", "Id,Name"); code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if got.Path != "/services/data/v65.0/sobjects/Account/001" {
		t.Errorf("path = %q", got.Path)
	}
	if got.Query.Get("fields") != "Id,Name" {
		t.Errorf("fields = %q", got.Query.Get("fields"))
	}
}

func TestRecordCreate(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 201, `{"id":"001","success":true,"errors":[]}`)
	defer srv.Close()
	code, out, errOut := run(t, srv, "record", "create", "Account", "--data", `{"Name":"Acme"}`)
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, errOut)
	}
	if got.Method != http.MethodPost || got.Path != "/services/data/v65.0/sobjects/Account" {
		t.Errorf("method/path = %s %s", got.Method, got.Path)
	}
	if !strings.Contains(out, `"success":true`) {
		t.Errorf("stdout = %s", out)
	}
}

func TestRecordUpdateSynthesizesResultOn204(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 204, "")
	defer srv.Close()
	code, out, errOut := run(t, srv, "record", "update", "Account", "001", "--data", `{"Name":"New"}`)
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, errOut)
	}
	if got.Method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", got.Method)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if res["success"] != true || res["id"] != "001" {
		t.Errorf("result = %v", res)
	}
}

func TestRecordDelete(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 204, "")
	defer srv.Close()
	code, out, e := run(t, srv, "record", "delete", "Account", "001")
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if got.Method != http.MethodDelete {
		t.Errorf("method = %s", got.Method)
	}
	if !strings.Contains(out, `"success":true`) {
		t.Errorf("stdout = %s", out)
	}
}

func TestRecordUpsertPassesBodyThrough(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 201, `{"id":"001","success":true,"created":true}`)
	defer srv.Close()
	code, out, e := run(t, srv, "record", "upsert", "Account", "ExtId__c", "A-1", "--data", `{"Name":"Acme"}`)
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if got.Method != http.MethodPatch || got.Path != "/services/data/v65.0/sobjects/Account/ExtId__c/A-1" {
		t.Errorf("method/path = %s %s", got.Method, got.Path)
	}
	if !strings.Contains(out, `"created":true`) {
		t.Errorf("stdout = %s", out)
	}
}

func TestSObjectListTrimsAndFilters(t *testing.T) {
	var got capturedRequest
	resp := `{"sobjects":[
		{"name":"Account","label":"Account","custom":false,"queryable":true,"extra":"drop"},
		{"name":"My__c","label":"My","custom":true,"queryable":true}
	]}`
	srv := staticServer(t, &got, 200, resp)
	defer srv.Close()

	code, out, e := run(t, srv, "sobject", "list", "--custom-only")
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if strings.Contains(out, "Account\"") && !strings.Contains(out, "My__c") {
		t.Errorf("custom-only filter failed: %s", out)
	}
	if strings.Contains(out, "extra") || strings.Contains(out, "drop") {
		t.Errorf("trimming leaked extra fields: %s", out)
	}
	var parsed struct {
		SObjects []trimmedSObject `json:"sobjects"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if len(parsed.SObjects) != 1 || parsed.SObjects[0].Name != "My__c" {
		t.Errorf("filtered list = %+v", parsed.SObjects)
	}
}

func TestSObjectListRejectsConflictingFilters(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{"sobjects":[]}`)
	defer srv.Close()
	code, _, _ := run(t, srv, "sobject", "list", "--custom-only", "--standard-only")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
}

func TestSObjectDescribeTrims(t *testing.T) {
	var got capturedRequest
	resp := `{"name":"Account","label":"Account","fields":[
		{"name":"Name","label":"Name","type":"string","nillable":false,"defaultedOnCreate":false,"updateable":true,"huge":"drop"},
		{"name":"Type","label":"Type","type":"picklist","nillable":true,"updateable":true,"picklistValues":[{"value":"A"}]}
	]}`
	srv := staticServer(t, &got, 200, resp)
	defer srv.Close()

	code, out, e := run(t, srv, "sobject", "describe", "Account")
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if got.Path != "/services/data/v65.0/sobjects/Account/describe" {
		t.Errorf("path = %q", got.Path)
	}
	if strings.Contains(out, "huge") || strings.Contains(out, "drop") {
		t.Errorf("describe trimming leaked: %s", out)
	}
	var parsed describeResult
	_ = parsed
	if !strings.Contains(out, `"required":true`) {
		t.Errorf("expected Name required=true: %s", out)
	}
}

func TestSObjectDescribeFieldNamesOnly(t *testing.T) {
	var got capturedRequest
	resp := `{"name":"Account","label":"Account","fields":[{"name":"Name"},{"name":"Type"}]}`
	srv := staticServer(t, &got, 200, resp)
	defer srv.Close()
	code, out, e := run(t, srv, "sobject", "describe", "Account", "--field-names-only")
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	var parsed struct {
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if len(parsed.Fields) != 2 || parsed.Fields[0] != "Name" {
		t.Errorf("field names = %v", parsed.Fields)
	}
}

func TestSObjectDescribeRawPassesThrough(t *testing.T) {
	var got capturedRequest
	resp := `{"name":"Account","fields":[{"name":"Name","huge":"kept"}]}`
	srv := staticServer(t, &got, 200, resp)
	defer srv.Close()
	code, out, e := run(t, srv, "sobject", "describe", "Account", "--raw")
	if code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("--raw should preserve full body: %s", out)
	}
}

func TestWhoami(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{"user_id":"005","organization_id":"00D","preferred_username":"a@b.com"}`)
	defer srv.Close()
	if code, _, e := run(t, srv, "whoami"); code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if got.Path != "/services/oauth2/userinfo" {
		t.Errorf("path = %q", got.Path)
	}
	assertAuth(t, got)
}

func TestLimits(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{"DailyApiRequests":{"Max":15000,"Remaining":14999}}`)
	defer srv.Close()
	if code, _, e := run(t, srv, "limits"); code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if got.Path != "/services/data/v65.0/limits" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestAPIVersionOverride(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{"totalSize":0,"done":true,"records":[]}`)
	defer srv.Close()
	if code, _, e := run(t, srv, "query", "--api-version", "v60.0", "SELECT Id FROM Account"); code != 0 {
		t.Fatalf("exit = %d, %s", code, e)
	}
	if !strings.HasPrefix(got.Path, "/services/data/v60.0/") {
		t.Errorf("path = %q, want v60.0", got.Path)
	}
}

func TestArrayErrorBodyRendered(t *testing.T) {
	var got capturedRequest
	resp := `[{"errorCode":"MALFORMED_QUERY","message":"unexpected token"}]`
	srv := staticServer(t, &got, 400, resp)
	defer srv.Close()

	code, out, errOut := run(t, srv, "query", "SELECT bad")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out != "" {
		t.Errorf("stdout should be empty on error, got %s", out)
	}
	if !strings.Contains(errOut, "MALFORMED_QUERY") || !strings.Contains(errOut, "unexpected token") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestErrorJSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 404, `[{"errorCode":"NOT_FOUND","message":"missing"}]`)
	defer srv.Close()
	code, _, errOut := run(t, srv, "record", "get", "Account", "001", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(errOut)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, errOut)
	}
	if env.Error.Kind != "api" || env.Error.Status != 404 {
		t.Errorf("envelope = %+v", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 401, `[{"errorCode":"INVALID_SESSION_ID","message":"Session expired"}]`)
	defer srv.Close()
	result, _, _ := runResult(t, srv, "limits")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 credential-rejected", result)
	}
}

func TestMissingAccessTokenExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"limits"}, map[string]string{EnvInstanceURL: testInstance})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAccessToken) {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestMissingInstanceURLExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"limits"}, map[string]string{EnvAccessToken: testToken})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvInstanceURL) {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 200, `{}`)
	defer srv.Close()
	if code, _, _ := run(t, srv, "record", "frobnicate"); code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
}

func TestInvalidDataJSONIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := staticServer(t, &got, 201, `{}`)
	defer srv.Close()
	if code, _, _ := run(t, srv, "record", "create", "Account", "--data", `{not json`); code != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", code)
	}
}
