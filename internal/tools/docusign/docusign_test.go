package docusign

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capturedRequest records one request the fake DocuSign server received.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Accept string
	Query  url.Values
	Body   []byte
}

// stub is one canned answer for a "METHOD /path" route.
type stub struct {
	status      int
	body        string
	contentType string
}

// newServer is a fake account-scoped eSignature server keyed by "METHOD /path"
// (path relative to the server root, including the /restapi/... prefix). It
// records every request. Unmatched routes return 404.
func newServer(t *testing.T, reqs *[]capturedRequest, routes map[string]stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Accept: r.Header.Get("Accept"),
			Query:  r.URL.Query(),
			Body:   body,
		})
		if s, ok := routes[r.Method+" "+r.URL.Path]; ok {
			ct := s.contentType
			if ct == "" {
				ct = "application/json"
			}
			w.Header().Set("Content-Type", ct)
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorCode":"ENVELOPE_DOES_NOT_EXIST","message":"not found"}`))
	}))
}

// run executes one docusign invocation against srv and returns stdout, stderr,
// and the exit code. account_id is fixed to "acc-1" so the base path is
// {srv}/restapi/v2.1/accounts/acc-1.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	env := map[string]string{
		EnvAccessToken: "tok-abc",
		EnvAccountID:   "acc-1",
	}
	res, err := svc.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("Execute returned a transport error (should be nil): %v", err)
	}
	return out.String(), errBuf.String(), res.ExitCode
}

const accountBase = "/restapi/v2.1/accounts/acc-1"

func findReq(reqs []capturedRequest, method, path string) *capturedRequest {
	for i := range reqs {
		if reqs[i].Method == method && reqs[i].Path == path {
			return &reqs[i]
		}
	}
	return nil
}

func bodyMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (%s)", err, b)
	}
	return m
}

func TestEnvelopeListBuildsAccountScopedRequest(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET " + accountBase + "/envelopes": {status: 200, body: `{"envelopes":[
			{"envelopeId":"e1","status":"sent","emailSubject":"NDA","sentDateTime":"2026-07-01T10:00:00Z"},
			{"envelopeId":"e2","status":"completed","emailSubject":"MSA","sentDateTime":"2026-07-02T10:00:00Z"}
		]}`},
	})
	defer srv.Close()

	stdout, stderr, code := run(t, srv, "envelope", "list", "--status", "sent", "--from-date", "2026-06-01", "--count", "5", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, http.MethodGet, accountBase+"/envelopes")
	if req == nil {
		t.Fatalf("no GET to %s/envelopes; got %+v", accountBase, reqs)
	}
	if req.Auth != "Bearer tok-abc" {
		t.Errorf("Authorization = %q, want Bearer tok-abc", req.Auth)
	}
	if req.Query.Get("status") != "sent" || req.Query.Get("from_date") != "2026-06-01" || req.Query.Get("count") != "5" {
		t.Errorf("query = %v, want status=sent from_date=2026-06-01 count=5", req.Query)
	}
	var payload struct {
		Envelopes []map[string]any `json:"envelopes"`
		Count     int              `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout is not the neutral list envelope: %v (%s)", err, stdout)
	}
	if payload.Count != 2 || payload.Envelopes[0]["id"] != "e1" || payload.Envelopes[0]["subject"] != "NDA" {
		t.Errorf("neutral list wrong: %s", stdout)
	}
}

func TestEnvelopeListDefaultsFromDate(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET " + accountBase + "/envelopes": {status: 200, body: `{"envelopes":[]}`},
	})
	defer srv.Close()
	_, stderr, code := run(t, srv, "envelope", "list")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, http.MethodGet, accountBase+"/envelopes")
	if req == nil || req.Query.Get("from_date") == "" {
		t.Fatalf("list without --from-date must default from_date; query=%v", reqOrNil(req))
	}
}

func reqOrNil(r *capturedRequest) any {
	if r == nil {
		return nil
	}
	return r.Query
}

func TestEnvelopeGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET " + accountBase + "/envelopes/e9": {status: 200, body: `{"envelopeId":"e9","status":"completed","emailSubject":"Deal","createdDateTime":"2026-07-01T00:00:00Z","completedDateTime":"2026-07-03T00:00:00Z"}`},
	})
	defer srv.Close()
	stdout, stderr, code := run(t, srv, "envelope", "get", "e9", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	m := bodyMap(t, []byte(stdout))
	if m["id"] != "e9" || m["status"] != "completed" || m["completed_at"] != "2026-07-03T00:00:00Z" {
		t.Errorf("neutral get wrong: %s", stdout)
	}
}

func TestEnvelopeRecipientsFlattensRoles(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET " + accountBase + "/envelopes/e1/recipients": {status: 200, body: `{
			"signers":[{"name":"Ada","email":"ada@x.com","status":"completed","routingOrder":"1","recipientId":"1","signedDateTime":"2026-07-02T00:00:00Z"}],
			"carbonCopies":[{"name":"Cc Bob","email":"bob@x.com","status":"created","routingOrder":"2","recipientId":"2"}]
		}`},
	})
	defer srv.Close()
	stdout, stderr, code := run(t, srv, "envelope", "recipients", "e1", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	var payload struct {
		Recipients []map[string]any `json:"recipients"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("bad recipients json: %v (%s)", err, stdout)
	}
	if len(payload.Recipients) != 2 {
		t.Fatalf("want 2 recipients, got %d: %s", len(payload.Recipients), stdout)
	}
	if payload.Recipients[0]["type"] != "signer" || payload.Recipients[0]["signed_at"] != "2026-07-02T00:00:00Z" {
		t.Errorf("signer view wrong: %v", payload.Recipients[0])
	}
	if payload.Recipients[1]["type"] != "carbon_copy" {
		t.Errorf("cc view wrong: %v", payload.Recipients[1])
	}
}

func TestEnvelopeSendTemplate(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST " + accountBase + "/envelopes": {status: 201, body: `{"envelopeId":"e-new","status":"sent","uri":"/envelopes/e-new"}`},
	})
	defer srv.Close()
	stdout, stderr, code := run(t, srv, "envelope", "send",
		"--template-id", "tpl-1", "--signer-email", "ada@x.com", "--signer-name", "Ada", "--subject", "Sign this", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, http.MethodPost, accountBase+"/envelopes")
	if req == nil {
		t.Fatalf("no POST /envelopes")
	}
	m := bodyMap(t, req.Body)
	if m["templateId"] != "tpl-1" || m["status"] != "sent" || m["emailSubject"] != "Sign this" {
		t.Errorf("send body wrong: %s", req.Body)
	}
	roles, ok := m["templateRoles"].([]any)
	if !ok || len(roles) != 1 {
		t.Fatalf("templateRoles missing: %s", req.Body)
	}
	role := roles[0].(map[string]any)
	if role["email"] != "ada@x.com" || role["name"] != "Ada" || role["roleName"] != "Signer" {
		t.Errorf("template role wrong: %v", role)
	}
	out := bodyMap(t, []byte(stdout))
	if out["envelope_id"] != "e-new" || out["status"] != "sent" {
		t.Errorf("send neutral output wrong: %s", stdout)
	}
}

func TestEnvelopeSendDocumentBase64AndAnchorTab(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "contract.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4 fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"POST " + accountBase + "/envelopes": {status: 201, body: `{"envelopeId":"e-doc","status":"sent"}`},
	})
	defer srv.Close()
	_, stderr, code := run(t, srv, "envelope", "send",
		"--document", pdf, "--signer-email", "ada@x.com", "--signer-name", "Ada")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, http.MethodPost, accountBase+"/envelopes")
	m := bodyMap(t, req.Body)
	docs, ok := m["documents"].([]any)
	if !ok || len(docs) != 1 {
		t.Fatalf("documents missing: %s", req.Body)
	}
	doc := docs[0].(map[string]any)
	if doc["fileExtension"] != "pdf" || doc["name"] != "contract.pdf" || doc["documentBase64"] == "" {
		t.Errorf("document descriptor wrong: %v", doc)
	}
	recips := m["recipients"].(map[string]any)
	signer := recips["signers"].([]any)[0].(map[string]any)
	tabs := signer["tabs"].(map[string]any)["signHereTabs"].([]any)[0].(map[string]any)
	if tabs["anchorString"] != defaultAnchor {
		t.Errorf("anchor tab wrong: %v", tabs)
	}
}

func TestEnvelopeSendRequiresExactlyOneSource(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()
	_, stderr, code := run(t, srv, "envelope", "send", "--signer-email", "a@x.com", "--signer-name", "A")
	if code != 2 {
		t.Fatalf("missing source must be usage error exit 2, got %d", code)
	}
	if !strings.Contains(stderr, "template-id") {
		t.Errorf("stderr should mention the exclusive-source rule: %s", stderr)
	}
	if len(reqs) != 0 {
		t.Errorf("no request should be made on a usage error; got %d", len(reqs))
	}
}

func TestEnvelopeVoidPutsVoidedStatus(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"PUT " + accountBase + "/envelopes/e1": {status: 200, body: `{"envelopeId":"e1"}`},
	})
	defer srv.Close()
	stdout, stderr, code := run(t, srv, "envelope", "void", "e1", "--reason", "sent by mistake", "--json")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, http.MethodPut, accountBase+"/envelopes/e1")
	m := bodyMap(t, req.Body)
	if m["status"] != "voided" || m["voidedReason"] != "sent by mistake" {
		t.Errorf("void body wrong: %s", req.Body)
	}
	out := bodyMap(t, []byte(stdout))
	if out["envelope_id"] != "e1" || out["status"] != "voided" {
		t.Errorf("void output wrong: %s", stdout)
	}
}

func TestEnvelopeVoidRequiresReason(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{})
	defer srv.Close()
	_, _, code := run(t, srv, "envelope", "void", "e1")
	if code != 2 {
		t.Fatalf("missing --reason must be usage exit 2, got %d", code)
	}
	if len(reqs) != 0 {
		t.Errorf("no request on usage error")
	}
}

func TestEnvelopeDownloadWritesFile(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET " + accountBase + "/envelopes/e1/documents/combined": {status: 200, body: "%PDF-1.4 signed", contentType: "application/pdf"},
	})
	defer srv.Close()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "signed.pdf")
	_, stderr, code := run(t, srv, "envelope", "download", "e1", "--out", outPath)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	req := findReq(reqs, http.MethodGet, accountBase+"/envelopes/e1/documents/combined")
	if req == nil || req.Accept != "application/pdf" {
		t.Errorf("download must request application/pdf; got %v", reqOrNilAccept(req))
	}
	data, err := os.ReadFile(outPath)
	if err != nil || string(data) != "%PDF-1.4 signed" {
		t.Errorf("downloaded file wrong: %v (%s)", err, data)
	}
}

func reqOrNilAccept(r *capturedRequest) any {
	if r == nil {
		return nil
	}
	return r.Accept
}

func TestTemplateListAndGet(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET " + accountBase + "/templates":    {status: 200, body: `{"envelopeTemplates":[{"templateId":"t1","name":"NDA"}]}`},
		"GET " + accountBase + "/templates/t1": {status: 200, body: `{"templateId":"t1","name":"NDA","description":"Mutual"}`},
	})
	defer srv.Close()
	stdout, _, code := run(t, srv, "template", "list", "--json")
	if code != 0 {
		t.Fatalf("list exit=%d", code)
	}
	var lp struct {
		Templates []map[string]any `json:"templates"`
		Count     int              `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &lp); err != nil || lp.Count != 1 || lp.Templates[0]["id"] != "t1" {
		t.Fatalf("template list wrong: %s (%v)", stdout, err)
	}
	gout, _, gcode := run(t, srv, "template", "get", "t1", "--json")
	if gcode != 0 {
		t.Fatalf("get exit=%d", gcode)
	}
	gm := bodyMap(t, []byte(gout))
	if gm["id"] != "t1" || gm["description"] != "Mutual" {
		t.Errorf("template get wrong: %s", gout)
	}
}

func TestAPIErrorPlainAndJSON(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"PUT " + accountBase + "/envelopes/e1": {status: 409, body: `{"errorCode":"ENVELOPE_CANNOT_VOID","message":"already completed"}`},
	})
	defer srv.Close()

	_, stderr, code := run(t, srv, "envelope", "void", "e1", "--reason", "x")
	if code != 1 {
		t.Fatalf("API error must exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "409") || !strings.Contains(stderr, "already completed") {
		t.Errorf("plain error should carry status + message: %s", stderr)
	}

	_, jstderr, jcode := run(t, srv, "envelope", "void", "e1", "--reason", "x", "--json")
	if jcode != 1 {
		t.Fatalf("json API error must exit 1, got %d", jcode)
	}
	var env struct {
		Error struct {
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(jstderr), &env); err != nil {
		t.Fatalf("json error not an envelope: %v (%s)", err, jstderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 409 {
		t.Errorf("json error envelope wrong: %s", jstderr)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	var reqs []capturedRequest
	srv := newServer(t, &reqs, map[string]stub{
		"GET " + accountBase + "/envelopes/e1": {status: 401, body: `{"errorCode":"AUTHORIZATION_INVALID_TOKEN","message":"expired"}`},
	})
	defer srv.Close()
	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"envelope", "get", "e1"},
		map[string]string{EnvAccessToken: "bad", EnvAccountID: "acc-1"})
	if err != nil {
		t.Fatalf("Execute transport err: %v", err)
	}
	if res.ExitCode != 1 || !res.CredentialRejected {
		t.Errorf("401 must exit 1 with CredentialRejected; got exit=%d rejected=%v", res.ExitCode, res.CredentialRejected)
	}
}

func TestMissingCredentialFailsFast(t *testing.T) {
	var out, errBuf bytes.Buffer
	svc := &Service{Out: &out, Err: &errBuf}
	res, _ := svc.Execute(context.Background(), []string{"envelope", "list"},
		map[string]string{EnvAccessToken: "tok"}) // no base_uri, no account_id
	if res.ExitCode != 1 {
		t.Fatalf("missing credential must exit 1, got %d", res.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvBaseURI) {
		t.Errorf("stderr should name the first missing credential: %s", errBuf.String())
	}
}
