package adobesign

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capturedRequest records one request the fake shard host saw.
type capturedRequest struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Query       string
	Body        []byte
}

// route is a canned answer for a "METHOD /path" key.
type route struct {
	status int
	body   string
	// raw sends body bytes verbatim (used for binary downloads).
	raw bool
}

func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]route) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
			Query:       r.URL.RawQuery,
			Body:        body,
		})
		if rt, ok := routes[r.Method+" "+r.URL.Path]; ok {
			if !rt.raw {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(rt.status)
			_, _ = w.Write([]byte(rt.body))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"RESOURCE_NOT_FOUND","message":"not found"}`))
	}))
}

// run executes one adobe-sign invocation against srv, returning stdout, stderr
// and exit code. The fake shard host is passed via s.BaseURL, the token via env.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), args, map[string]string{EnvToken: "tok-123"})
	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	return out.String(), errBuf.String(), res.ExitCode
}

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

func TestAgreementListJSONShapeAndAuthHeader(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements": {status: 200, body: `{"userAgreementList":[{"id":"CBJ1","name":"NDA","status":"OUT_FOR_SIGNATURE","displayDate":"2026-07-20T10:00:00Z"}],"page":{"nextCursor":"NEXT"}}`},
	})
	defer srv.Close()

	out, errStr, code := run(t, srv, "agreement", "list", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	req := findReq(reqs, "GET", "/api/rest/v6/agreements")
	if req == nil {
		t.Fatal("did not call GET /api/rest/v6/agreements")
	}
	if req.Auth != "Bearer tok-123" {
		t.Errorf("auth header = %q, want Bearer tok-123", req.Auth)
	}
	var got struct {
		Agreements []agreementSummary `json:"agreements"`
		PageCursor string             `json:"page_cursor"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, out)
	}
	if len(got.Agreements) != 1 || got.Agreements[0].ID != "CBJ1" || got.Agreements[0].Status != "OUT_FOR_SIGNATURE" {
		t.Errorf("agreements = %+v", got.Agreements)
	}
	if got.Agreements[0].Created != "2026-07-20T10:00:00Z" {
		t.Errorf("created = %q", got.Agreements[0].Created)
	}
	if got.PageCursor != "NEXT" {
		t.Errorf("page_cursor = %q", got.PageCursor)
	}
}

func TestAgreementListPaginationQuery(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements": {status: 200, body: `{"userAgreementList":[]}`},
	})
	defer srv.Close()

	_, errStr, code := run(t, srv, "agreement", "list", "--cursor", "ABC", "--page-size", "5", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	req := findReq(reqs, "GET", "/api/rest/v6/agreements")
	if req == nil {
		t.Fatal("no list call")
	}
	if !strings.Contains(req.Query, "cursor=ABC") || !strings.Contains(req.Query, "pageSize=5") {
		t.Errorf("query = %q, want cursor=ABC & pageSize=5", req.Query)
	}
}

func TestAgreementSendTwoStepFromFile(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "contract.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4 fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"POST /api/rest/v6/transientDocuments": {status: 201, body: `{"transientDocumentId":"TRANS1"}`},
		"POST /api/rest/v6/agreements":         {status: 201, body: `{"id":"AGR1"}`},
	})
	defer srv.Close()

	out, errStr, code := run(t, srv, "agreement", "send",
		"--document", pdf, "--recipient-email", "signer@example.com",
		"--recipient-name", "Signer One", "--name", "Q3 Contract", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	// Step 1: multipart transient upload.
	up := findReq(reqs, "POST", "/api/rest/v6/transientDocuments")
	if up == nil {
		t.Fatal("did not upload transient document first")
	}
	if !strings.HasPrefix(up.ContentType, "multipart/form-data") {
		t.Errorf("transient upload content-type = %q, want multipart/form-data", up.ContentType)
	}
	// Step 2: create agreement referencing the transient id.
	cr := findReq(reqs, "POST", "/api/rest/v6/agreements")
	if cr == nil {
		t.Fatal("did not create agreement")
	}
	var body map[string]any
	if err := json.Unmarshal(cr.Body, &body); err != nil {
		t.Fatalf("agreement body not JSON: %v", err)
	}
	if body["state"] != "IN_PROCESS" {
		t.Errorf("state = %v, want IN_PROCESS", body["state"])
	}
	fileInfos, _ := body["fileInfos"].([]any)
	if len(fileInfos) != 1 {
		t.Fatalf("fileInfos = %v", body["fileInfos"])
	}
	fi, _ := fileInfos[0].(map[string]any)
	if fi["transientDocumentId"] != "TRANS1" {
		t.Errorf("fileInfos[0] = %v, want transientDocumentId TRANS1", fi)
	}
	var res struct {
		AgreementID string `json:"agreement_id"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, out)
	}
	if res.AgreementID != "AGR1" || res.Status != "IN_PROCESS" {
		t.Errorf("result = %+v", res)
	}
}

func TestAgreementSendFromLibrarySkipsTransientUpload(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"POST /api/rest/v6/agreements": {status: 201, body: `{"id":"AGR2"}`},
	})
	defer srv.Close()

	_, errStr, code := run(t, srv, "agreement", "send",
		"--library-id", "LIB9", "--recipient-email", "s@example.com", "--name", "From Template", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	if findReq(reqs, "POST", "/api/rest/v6/transientDocuments") != nil {
		t.Error("library send must NOT upload a transient document")
	}
	cr := findReq(reqs, "POST", "/api/rest/v6/agreements")
	if cr == nil {
		t.Fatal("no agreement create")
	}
	var body map[string]any
	_ = json.Unmarshal(cr.Body, &body)
	fi, _ := body["fileInfos"].([]any)[0].(map[string]any)
	if fi["libraryDocumentId"] != "LIB9" {
		t.Errorf("fileInfos[0] = %v, want libraryDocumentId LIB9", fi)
	}
}

func TestAgreementSendRejectsBothSources(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, nil)
	defer srv.Close()
	_, _, code := run(t, srv, "agreement", "send",
		"--document", "/tmp/x.pdf", "--library-id", "L1", "--recipient-email", "a@b.co", "--name", "X")
	if code != 2 {
		t.Errorf("exit=%d, want 2 (usage error for both sources)", code)
	}
}

func TestAgreementSendRequiresRecipientAndName(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, nil)
	defer srv.Close()
	if _, _, code := run(t, srv, "agreement", "send", "--library-id", "L1", "--name", "X"); code != 2 {
		t.Errorf("missing recipient exit=%d, want 2", code)
	}
	if _, _, code := run(t, srv, "agreement", "send", "--library-id", "L1", "--recipient-email", "a@b.co"); code != 2 {
		t.Errorf("missing name exit=%d, want 2", code)
	}
}

func TestAgreementGetJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements/AGR1": {status: 200, body: `{"id":"AGR1","name":"NDA","status":"SIGNED","displayDate":"2026-07-19"}`},
	})
	defer srv.Close()
	out, errStr, code := run(t, srv, "agreement", "get", "AGR1", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	var got agreementSummary
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, out)
	}
	if got.ID != "AGR1" || got.Status != "SIGNED" || got.Name != "NDA" {
		t.Errorf("summary = %+v", got)
	}
}

func TestAgreementMembersFlattensParticipants(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements/AGR1/members": {status: 200, body: `{"participantSets":[{"order":1,"role":"SIGNER","status":"WAITING_FOR_MY_SIGNATURE","memberInfos":[{"email":"a@ex.com"}]}]}`},
	})
	defer srv.Close()
	out, errStr, code := run(t, srv, "agreement", "members", "AGR1", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	var got struct {
		Participants []struct {
			Email  string `json:"email"`
			Status string `json:"status"`
			Order  int    `json:"order"`
			Role   string `json:"role"`
		} `json:"participants"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, out)
	}
	if len(got.Participants) != 1 {
		t.Fatalf("participants = %+v", got.Participants)
	}
	p := got.Participants[0]
	if p.Email != "a@ex.com" || p.Status != "WAITING_FOR_MY_SIGNATURE" || p.Order != 1 || p.Role != "SIGNER" {
		t.Errorf("participant = %+v", p)
	}
}

func TestAgreementCancelPutsCancelledState(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"PUT /api/rest/v6/agreements/AGR1/state": {status: 200, body: `{}`},
	})
	defer srv.Close()
	_, errStr, code := run(t, srv, "agreement", "cancel", "AGR1", "--comment", "sent in error", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	req := findReq(reqs, "PUT", "/api/rest/v6/agreements/AGR1/state")
	if req == nil {
		t.Fatal("no cancel PUT")
	}
	var body map[string]any
	_ = json.Unmarshal(req.Body, &body)
	if body["state"] != "CANCELLED" {
		t.Errorf("state = %v, want CANCELLED", body["state"])
	}
	info, _ := body["agreementCancellationInfo"].(map[string]any)
	if info["comment"] != "sent in error" {
		t.Errorf("comment = %v", info["comment"])
	}
}

func TestAgreementDownloadWritesFile(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements/AGR1/combinedDocument": {status: 200, body: "%PDF-signed-bytes", raw: true},
	})
	defer srv.Close()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "signed.pdf")
	_, errStr, code := run(t, srv, "agreement", "download", "AGR1", "--out", outPath)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "%PDF-signed-bytes" {
		t.Errorf("downloaded content = %q", data)
	}
}

func TestLibraryListJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/libraryDocuments": {status: 200, body: `{"libraryDocumentList":[{"id":"LIB1","name":"MSA"}]}`},
	})
	defer srv.Close()
	out, errStr, code := run(t, srv, "library", "list", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	var got struct {
		LibraryDocuments []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"library_documents"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, out)
	}
	if len(got.LibraryDocuments) != 1 || got.LibraryDocuments[0].ID != "LIB1" {
		t.Errorf("library_documents = %+v", got.LibraryDocuments)
	}
}

func TestDocumentUploadPlainReturnsID(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "f.pdf")
	_ = os.WriteFile(pdf, []byte("data"), 0o600)
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"POST /api/rest/v6/transientDocuments": {status: 201, body: `{"transientDocumentId":"TID42"}`},
	})
	defer srv.Close()
	out, errStr, code := run(t, srv, "document", "upload", pdf)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errStr)
	}
	if strings.TrimSpace(out) != "TID42" {
		t.Errorf("stdout = %q, want TID42", out)
	}
}

func TestAPIErrorJSONEnvelopeAndExit1(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements/BAD": {status: 404, body: `{"code":"INVALID_AGREEMENT_ID","message":"no such agreement"}`},
	})
	defer srv.Close()
	_, errStr, code := run(t, srv, "agreement", "get", "BAD", "--json")
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errStr), &env); err != nil {
		t.Fatalf("stderr not JSON: %v (%s)", err, errStr)
	}
	if env.Error.Code != "api_error" || env.Error.Status != 404 {
		t.Errorf("error envelope = %+v", env.Error)
	}
	if !strings.Contains(env.Error.Message, "INVALID_AGREEMENT_ID") {
		t.Errorf("message = %q, want provider code", env.Error.Message)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements": {status: 401, body: `{"code":"UNAUTHORIZED","message":"bad token"}`},
	})
	defer srv.Close()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"agreement", "list"}, map[string]string{EnvToken: "tok"})
	if err != nil {
		t.Fatalf("Execute Go error: %v", err)
	}
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 with credential rejection", res)
	}
}

func TestMissingBaseURIFailsFast(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, _ := svc.Execute(context.Background(), []string{"agreement", "list"}, map[string]string{EnvToken: "tok"})
	if res.ExitCode != 1 {
		t.Errorf("exit=%d, want 1 when %s unset", res.ExitCode, EnvBaseURI)
	}
	if !strings.Contains(errBuf.String(), EnvBaseURI) {
		t.Errorf("stderr = %q, want mention of %s", errBuf.String(), EnvBaseURI)
	}
}

func TestBaseURITrailingSlashTolerated(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]route{
		"GET /api/rest/v6/agreements": {status: 200, body: `{"userAgreementList":[]}`},
	})
	defer srv.Close()
	var out, errBuf bytes.Buffer
	// Service.BaseURL already has no trailing slash; assert base() composes the
	// v6 path correctly whether or not a trailing slash is present.
	svc := &Service{BaseURL: srv.URL + "/", HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"agreement", "list"}, map[string]string{EnvToken: "tok"})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("exit=%d err=%v stderr=%s", res.ExitCode, err, errBuf.String())
	}
	if findReq(reqs, "GET", "/api/rest/v6/agreements") == nil {
		t.Errorf("expected /api/rest/v6/agreements; got reqs=%+v", reqs)
	}
}
