package dropboxsign

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendWithFileURLMultipart(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"signature_request":{"signature_request_id":"abc"}}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv,
		"signature-request", "send",
		"--file-url", "https://example.com/doc.pdf",
		"--signer", "Alice Example:alice@example.com",
		"--signer", "Bob:bob@example.com",
		"--title", "NDA",
		"--test-mode",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != "POST" || got.Path != "/signature_request/send" {
		t.Fatalf("route = %s %s", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Fatalf("auth = %q", got.Auth)
	}
	if !strings.HasPrefix(got.ContentType, "multipart/form-data") {
		t.Fatalf("content-type = %q, want multipart/form-data", got.ContentType)
	}
	values, files := parseMultipart(t, got.ContentType, got.Body)
	if len(files) != 0 {
		t.Fatalf("expected no file parts for --file-url, got %v", files)
	}
	assertField(t, values, "file_urls[0]", "https://example.com/doc.pdf")
	assertField(t, values, "signers[0][name]", "Alice Example")
	assertField(t, values, "signers[0][email_address]", "alice@example.com")
	assertField(t, values, "signers[0][order]", "0")
	assertField(t, values, "signers[1][email_address]", "bob@example.com")
	assertField(t, values, "signers[1][order]", "1")
	assertField(t, values, "title", "NDA")
	assertField(t, values, "test_mode", "true")
	contains(t, stdout, `"signature_request_id":"abc"`, "send stdout")
}

func TestSendWithFileUpload(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"signature_request":{"signature_request_id":"xyz"}}`, &got)
	defer srv.Close()

	dir := t.TempDir()
	doc := filepath.Join(dir, "contract.pdf")
	if err := os.WriteFile(doc, []byte("%PDF-1.7 fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	exit, _, stderr := run(t, srv,
		"signature-request", "send",
		"--file", doc,
		"--signer", "Carol:carol@example.com",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	values, files := parseMultipart(t, got.ContentType, got.Body)
	body, ok := files["files[0]"]
	if !ok {
		t.Fatalf("expected files[0] file part, got parts %v", files)
	}
	if body != "%PDF-1.7 fake" {
		t.Fatalf("file part body = %q", body)
	}
	assertField(t, values, "test_mode", "false")
	if _, present := values["file_urls[0]"]; present {
		t.Fatalf("did not expect file_urls[0] on an upload send")
	}
}

func TestSendRejectsBothFileAndURL(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()
	exit, _, stderr := run(t, srv,
		"signature-request", "send",
		"--file", "/tmp/x.pdf",
		"--file-url", "https://example.com/y.pdf",
		"--signer", "A:a@x.com",
	)
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	contains(t, stderr, "not both", "both-file stderr")
	if got.Method != "" {
		t.Fatalf("expected no API call, hit %s", got.Path)
	}
}

func TestSendRequiresADocument(t *testing.T) {
	srv := newServer(t, 200, `{}`, &capturedRequest{})
	defer srv.Close()
	exit, _, stderr := run(t, srv, "signature-request", "send", "--signer", "A:a@x.com")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	contains(t, stderr, "--file", "no-doc stderr")
}

func TestSendWithTemplate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"signature_request":{"signature_request_id":"t1"}}`, &got)
	defer srv.Close()
	exit, _, stderr := run(t, srv,
		"signature-request", "send-with-template",
		"--template", "tmpl-123",
		"--signer", "Client:Dana:dana@example.com",
		"--test-mode",
	)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Path != "/signature_request/send_with_template" {
		t.Fatalf("path = %s", got.Path)
	}
	values, _ := parseMultipart(t, got.ContentType, got.Body)
	assertField(t, values, "template_ids[0]", "tmpl-123")
	assertField(t, values, "signers[0][role]", "Client")
	assertField(t, values, "signers[0][name]", "Dana")
	assertField(t, values, "signers[0][email_address]", "dana@example.com")
}

func TestListPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"signature_requests":[],"list_info":{"page":2}}`, &got)
	defer srv.Close()
	exit, stdout, _ := run(t, srv, "signature-request", "list", "--page", "2", "--page-size", "5")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/signature_request/list" {
		t.Fatalf("path = %s", got.Path)
	}
	if !strings.Contains(got.Query, "page=2") || !strings.Contains(got.Query, "page_size=5") {
		t.Fatalf("query = %q, want page=2 & page_size=5", got.Query)
	}
	contains(t, stdout, `"list_info"`, "list stdout")
}

func TestGetSignatureRequest(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"signature_request":{"is_complete":true}}`, &got)
	defer srv.Close()
	exit, _, _ := run(t, srv, "signature-request", "get", "sig-9")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/signature_request/sig-9" {
		t.Fatalf("route = %s %s", got.Method, got.Path)
	}
}

func TestFilesStreamsToStdout(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, "PDFBYTES", &got)
	defer srv.Close()
	exit, stdout, _ := run(t, srv, "signature-request", "files", "sig-1", "--file-type", "pdf")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Path != "/signature_request/files/sig-1" {
		t.Fatalf("path = %s", got.Path)
	}
	if !strings.Contains(got.Query, "file_type=pdf") {
		t.Fatalf("query = %q", got.Query)
	}
	if stdout != "PDFBYTES" {
		t.Fatalf("stdout = %q, want raw bytes", stdout)
	}
}

func TestFilesWritesToOutAndReceipts(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, "ZIPBYTES", &got)
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "signed.zip")
	exit, stdout, _ := run(t, srv, "signature-request", "files", "sig-2", "--out", dst)
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(data) != "ZIPBYTES" {
		t.Fatalf("out file = %q", data)
	}
	var receipt struct {
		Bytes int    `json:"bytes"`
		Path  string `json:"path"`
	}
	if jerr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &receipt); jerr != nil {
		t.Fatalf("receipt not JSON: %v (%q)", jerr, stdout)
	}
	if receipt.Bytes != len("ZIPBYTES") || receipt.Path != dst {
		t.Fatalf("receipt = %+v", receipt)
	}
}

func TestRemindPostsJSON(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"signature_request":{}}`, &got)
	defer srv.Close()
	exit, _, stderr := run(t, srv, "signature-request", "remind", "sig-3", "--email", "signer@example.com")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr=%s", exit, stderr)
	}
	if got.Method != "POST" || got.Path != "/signature_request/remind/sig-3" {
		t.Fatalf("route = %s %s", got.Method, got.Path)
	}
	if !strings.HasPrefix(got.ContentType, "application/json") {
		t.Fatalf("content-type = %q", got.ContentType)
	}
	var payload map[string]string
	if jerr := json.Unmarshal(got.Body, &payload); jerr != nil {
		t.Fatalf("body not JSON: %v", jerr)
	}
	if payload["email_address"] != "signer@example.com" {
		t.Fatalf("email_address = %q", payload["email_address"])
	}
}

func TestRemindRequiresEmail(t *testing.T) {
	srv := newServer(t, 200, `{}`, &capturedRequest{})
	defer srv.Close()
	exit, _, stderr := run(t, srv, "signature-request", "remind", "sig-3")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	contains(t, stderr, "--email is required", "remind stderr")
}

func TestCancelEmitsReceipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, ``, &got)
	defer srv.Close()
	exit, stdout, _ := run(t, srv, "signature-request", "cancel", "sig-4")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "POST" || got.Path != "/signature_request/cancel/sig-4" {
		t.Fatalf("route = %s %s", got.Method, got.Path)
	}
	contains(t, stdout, `"cancelled":true`, "cancel receipt")
}

func TestAPIErrorTextAndJSON(t *testing.T) {
	srv := newServer(t, 400, `{"error":{"error_name":"bad_request","error_msg":"missing signer"}}`, &capturedRequest{})
	defer srv.Close()

	// plain text
	exit, _, stderr := run(t, srv, "signature-request", "get", "sig-x")
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	contains(t, stderr, "bad_request: missing signer", "api text error")

	// json envelope
	exit, _, stderrJSON := run(t, srv, "--json", "signature-request", "get", "sig-x")
	if exit != 1 {
		t.Fatalf("json exit = %d, want 1", exit)
	}
	var env struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(strings.TrimSpace(stderrJSON)), &env); jerr != nil {
		t.Fatalf("json stderr: %v (%q)", jerr, stderrJSON)
	}
	if env.Error.Kind != "api" || env.Error.Status != 400 {
		t.Fatalf("envelope = %+v, want kind=api status=400", env.Error)
	}
}

func TestUnauthorizedRejectsCredential(t *testing.T) {
	srv := newServer(t, 401, `{"error":{"error_name":"unauthorized","error_msg":"bad token"}}`, &capturedRequest{})
	defer srv.Close()
	res, _, _ := runResult(t, srv, "account", "get")
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !res.CredentialRejected {
		t.Fatal("expected CredentialRejected on 401")
	}
}

// assertField fails unless values[name] == want.
func assertField(t *testing.T, values map[string]string, name, want string) {
	t.Helper()
	got, ok := values[name]
	if !ok {
		t.Fatalf("missing form field %q (have %v)", name, keys(values))
	}
	if got != want {
		t.Fatalf("field %q = %q, want %q", name, got, want)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
