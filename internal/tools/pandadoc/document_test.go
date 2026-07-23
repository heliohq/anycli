package pandadoc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"results":[{"id":"d-1","name":"NDA","status":"document.sent"}]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv,
		"document", "list",
		"--q", "acme", "--status", "document.sent", "--template", "t-9",
		"--folder", "f-2", "--count", "10", "--page", "2", "--order", "date_created")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Path != "/public/v1/documents" {
		t.Errorf("path = %q, want /public/v1/documents", got.Path)
	}
	q := got.Query
	checks := map[string]string{
		"q": "acme", "status": "document.sent", "template_id": "t-9",
		"folder_uuid": "f-2", "count": "10", "page": "2", "ordering": "date_created",
	}
	for k, want := range checks {
		if q.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, q.Get(k), want)
		}
	}
	// Default output is concise text carrying id/status/name.
	if !strings.Contains(stdout, "d-1") || !strings.Contains(stdout, "NDA") {
		t.Errorf("stdout = %q, want id and name", stdout)
	}
}

func TestDocumentCreate_BodyShapeAndPolling(t *testing.T) {
	var reqs []capturedRequest
	// create returns uploaded; the first status poll flips to draft.
	routes := map[string]stub{
		"POST /public/v1/documents":     {status: 201, body: `{"id":"d-77","name":"NDA","status":"document.uploaded"}`},
		"GET /public/v1/documents/d-77": {status: 200, body: `{"id":"d-77","name":"NDA","status":"document.draft"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv,
		"document", "create",
		"--template", "tmpl-1", "--name", "NDA",
		"--recipient", "alice@acme.com:Client:Alice:Smith",
		"--recipient", "bob@acme.com",
		"--token", "Sender.Company=Acme",
		"--field", "CustomerName=Alice",
		"--metadata", "customerId=42")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	create := findReq(reqs, "POST", "/public/v1/documents")
	if create == nil {
		t.Fatal("no POST /public/v1/documents recorded")
	}
	b := bodyMap(t, create.Body)
	if b["template_uuid"] != "tmpl-1" || b["name"] != "NDA" {
		t.Errorf("body template_uuid/name = %v/%v", b["template_uuid"], b["name"])
	}
	recips, ok := b["recipients"].([]any)
	if !ok || len(recips) != 2 {
		t.Fatalf("recipients = %v, want 2", b["recipients"])
	}
	r0 := recips[0].(map[string]any)
	if r0["email"] != "alice@acme.com" || r0["role"] != "Client" || r0["first_name"] != "Alice" || r0["last_name"] != "Smith" {
		t.Errorf("recipient[0] = %v", r0)
	}
	toks, ok := b["tokens"].([]any)
	if !ok || len(toks) != 1 {
		t.Fatalf("tokens = %v, want 1", b["tokens"])
	}
	tk := toks[0].(map[string]any)
	if tk["name"] != "Sender.Company" || tk["value"] != "Acme" {
		t.Errorf("token[0] = %v", tk)
	}
	fields, ok := b["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields = %v, want object", b["fields"])
	}
	cn, ok := fields["CustomerName"].(map[string]any)
	if !ok || cn["value"] != "Alice" {
		t.Errorf("fields.CustomerName = %v, want {value: Alice}", fields["CustomerName"])
	}
	meta, ok := b["metadata"].(map[string]any)
	if !ok || meta["customerId"] != "42" {
		t.Errorf("metadata = %v", b["metadata"])
	}
	// Polled the status endpoint at least once to reach draft.
	if countReq(reqs, "GET", "/public/v1/documents/d-77") < 1 {
		t.Errorf("expected a status poll to draft, got %d", countReq(reqs, "GET", "/public/v1/documents/d-77"))
	}
	if !strings.Contains(stdout, "d-77") {
		t.Errorf("stdout = %q, want created doc id", stdout)
	}
}

func TestDocumentCreate_NoWaitSkipsPolling(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"POST /public/v1/documents": {status: 201, body: `{"id":"d-88","status":"document.uploaded"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "document", "create", "--template", "t", "--recipient", "a@b.com", "--no-wait")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if countReq(reqs, "GET", "/public/v1/documents/d-88") != 0 {
		t.Errorf("expected no status poll with --no-wait, got %d", countReq(reqs, "GET", "/public/v1/documents/d-88"))
	}
}

func TestDocumentCreate_BodyEscapeHatch(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"POST /public/v1/documents": {status: 201, body: `{"id":"d-9","status":"document.draft"}`},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "document", "create",
		"--body", `{"name":"Raw","template_uuid":"tt","recipients":[{"email":"z@z.com"}]}`, "--no-wait")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	create := findReq(reqs, "POST", "/public/v1/documents")
	b := bodyMap(t, create.Body)
	if b["name"] != "Raw" || b["template_uuid"] != "tt" {
		t.Errorf("raw body not forwarded: %v", b)
	}
}

func TestDocumentCreate_BodyAndFlagsMutuallyExclusive(t *testing.T) {
	result, _, stderr := runEnv(t, nil, map[string]string{EnvAccessToken: "tok-abc"},
		"document", "create", "--body", "{}", "--template", "t")
	if result.ExitCode != 2 {
		t.Errorf("exit = %d, want 2", result.ExitCode)
	}
	if !strings.Contains(stderr, "mutually exclusive") && !strings.Contains(stderr, "cannot be combined") {
		t.Errorf("stderr = %q, want exclusivity error", stderr)
	}
}

func TestDocumentStatus(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"d-1","name":"NDA","status":"document.completed"}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "document", "status", "d-1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/public/v1/documents/d-1" {
		t.Errorf("path = %q", got.Path)
	}
	if !strings.Contains(stdout, "document.completed") {
		t.Errorf("stdout = %q, want status", stdout)
	}
}

func TestDocumentDetails(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"d-1","name":"NDA","status":"document.sent"}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "document", "details", "d-1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/public/v1/documents/d-1/details" {
		t.Errorf("path = %q, want .../details", got.Path)
	}
}

func TestDocumentSend(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"d-1","status":"document.sent"}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "document", "send", "d-1", "--subject", "Please sign", "--message", "Hi", "--silent")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "POST" || got.Path != "/public/v1/documents/d-1/send" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	b := bodyMap(t, got.Body)
	if b["subject"] != "Please sign" || b["message"] != "Hi" || b["silent"] != true {
		t.Errorf("send body = %v", b)
	}
}

func TestDocumentLink(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 201, `{"id":"sess-123","expires_at":"2026-08-01T00:00:00Z"}`, &got)
	defer srv.Close()

	exit, stdout, _ := run(t, srv, "document", "link", "d-1", "--recipient", "a@b.com", "--lifetime", "900")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "POST" || got.Path != "/public/v1/documents/d-1/session" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	b := bodyMap(t, got.Body)
	if b["recipient"] != "a@b.com" || b["lifetime"] != float64(900) {
		t.Errorf("session body = %v", b)
	}
	if !strings.Contains(stdout, "sess-123") {
		t.Errorf("stdout = %q, want session id / link", stdout)
	}
}

func TestDocumentDownload_WritesFile(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /public/v1/documents/d-1/download": {status: 200, rawBody: []byte("%PDF-1.7 fake")},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "signed.pdf")
	exit, stdout, stderr := run(t, srv, "document", "download", "d-1", "--out", out)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if string(data) != "%PDF-1.7 fake" {
		t.Errorf("file contents = %q", data)
	}
	if !strings.Contains(stdout, out) {
		t.Errorf("stdout = %q, want saved path", stdout)
	}
}

func TestDocumentDownload_Protected(t *testing.T) {
	var reqs []capturedRequest
	routes := map[string]stub{
		"GET /public/v1/documents/d-1/download-protected": {status: 200, rawBody: []byte("%PDF")},
	}
	srv := newMux(t, &reqs, routes)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "p.pdf")
	exit, stdout, stderr := run(t, srv, "document", "download", "d-1", "--out", out, "--protected", "--json")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if findReq(reqs, "GET", "/public/v1/documents/d-1/download-protected") == nil {
		t.Errorf("did not hit download-protected endpoint")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatalf("stdout not JSON: %v (%s)", err, stdout)
	}
	if m["path"] != out || m["bytes"] != float64(4) {
		t.Errorf("json receipt = %v, want path=%s bytes=4", m, out)
	}
}

func TestDocumentDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 204, ``, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "document", "delete", "d-1")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "DELETE" || got.Path != "/public/v1/documents/d-1" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if !strings.Contains(stdout, "d-1") {
		t.Errorf("stdout = %q, want deleted confirmation", stdout)
	}
}
