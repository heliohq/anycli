package boldsign

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentSend_JSONBodyWithBase64File(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "contract.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.7 fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"documentId":"doc-1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "document", "send",
		"--file", pdf, "--title", "MSA",
		"--signer", "Alice <alice@example.com>",
		"--message", "please sign")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/document/send" {
		t.Errorf("request = %s %s, want POST /v1/document/send", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q", got.Auth)
	}
	body := decodeBody(t, got.Body)
	if body["Title"] != "MSA" || body["Message"] != "please sign" {
		t.Errorf("body Title/Message = %v / %v", body["Title"], body["Message"])
	}
	files, ok := body["Files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("Files = %v, want one entry", body["Files"])
	}
	entry := files[0].(map[string]any)
	if entry["fileName"] != "contract.pdf" {
		t.Errorf("fileName = %v, want contract.pdf", entry["fileName"])
	}
	b64, _ := entry["base64"].(string)
	if !strings.HasPrefix(b64, "data:application/pdf;base64,") {
		t.Errorf("base64 = %q, want data-URI with application/pdf prefix", b64)
	}
	signers, ok := body["Signers"].([]any)
	if !ok || len(signers) != 1 {
		t.Fatalf("Signers = %v, want one entry", body["Signers"])
	}
	signer := signers[0].(map[string]any)
	if signer["Name"] != "Alice" || signer["EmailAddress"] != "alice@example.com" || signer["SignerType"] != "Signer" {
		t.Errorf("signer = %v", signer)
	}
	if _, ok := signer["SignerOrder"]; ok {
		t.Errorf("SignerOrder should be absent without --signing-order, got %v", signer["SignerOrder"])
	}
	if !strings.Contains(stdout, `"documentId"`) {
		t.Errorf("stdout = %q, want provider passthrough", stdout)
	}
}

func TestDocumentSend_SigningOrderAndFileURL(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"documentId":"doc-2"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "document", "send",
		"--file-url", "https://example.com/a.pdf", "--title", "T",
		"--signer", "Alice <alice@example.com>", "--signer", "Bob <bob@example.com>",
		"--signing-order", "--expiry-days", "14", "--auto-detect-fields",
		"--disable-emails", "--on-behalf-of", "boss@example.com")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body := decodeBody(t, got.Body)
	urls, ok := body["FileUrls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://example.com/a.pdf" {
		t.Errorf("FileUrls = %v", body["FileUrls"])
	}
	if body["EnableSigningOrder"] != true {
		t.Errorf("EnableSigningOrder = %v, want true", body["EnableSigningOrder"])
	}
	if body["ExpiryDays"].(float64) != 14 {
		t.Errorf("ExpiryDays = %v, want 14", body["ExpiryDays"])
	}
	if body["AutoDetectFields"] != true || body["DisableEmails"] != true {
		t.Errorf("AutoDetectFields/DisableEmails = %v / %v", body["AutoDetectFields"], body["DisableEmails"])
	}
	if body["OnBehalfOf"] != "boss@example.com" {
		t.Errorf("OnBehalfOf = %v", body["OnBehalfOf"])
	}
	signers := body["Signers"].([]any)
	if len(signers) != 2 {
		t.Fatalf("want 2 signers, got %d", len(signers))
	}
	if signers[0].(map[string]any)["SignerOrder"].(float64) != 1 {
		t.Errorf("first SignerOrder = %v, want 1", signers[0].(map[string]any)["SignerOrder"])
	}
	if signers[1].(map[string]any)["SignerOrder"].(float64) != 2 {
		t.Errorf("second SignerOrder = %v, want 2", signers[1].(map[string]any)["SignerOrder"])
	}
}

func TestDocumentSend_MissingFileIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "document", "send", "--title", "T", "--signer", "Alice <a@e.com>")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "at least one --file") {
		t.Errorf("stderr = %q, want file requirement message", stderr)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected, but server saw %s", got.Method)
	}
}

func TestDocumentSend_BadSignerSpecIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "document", "send",
		"--file-url", "https://e.com/a.pdf", "--title", "T", "--signer", "no-brackets@example.com")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "Name <email>") {
		t.Errorf("stderr = %q, want format hint", stderr)
	}
}

func TestDocumentSend_BadSignerTypeIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "document", "send",
		"--file-url", "https://e.com/a.pdf", "--title", "T",
		"--signer", "Alice <a@e.com>", "--signer-type", "Bogus")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "signer-type") {
		t.Errorf("stderr = %q, want signer-type message", stderr)
	}
}

func TestDocumentList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"pageDetails":{},"result":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "document", "list", "--page", "2", "--page-size", "25",
		"--status", "Completed", "--status", "WaitingForOthers",
		"--search", "invoice", "--transmit-type", "Sent")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/document/list" {
		t.Errorf("request = %s %s, want GET /v1/document/list", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("page") != "2" || q.Get("pageSize") != "25" {
		t.Errorf("page/pageSize = %q / %q", q.Get("page"), q.Get("pageSize"))
	}
	if q.Get("searchKey") != "invoice" || q.Get("transmitType") != "Sent" {
		t.Errorf("searchKey/transmitType = %q / %q", q.Get("searchKey"), q.Get("transmitType"))
	}
	if statuses := q["status"]; len(statuses) != 2 {
		t.Errorf("status = %v, want two values", statuses)
	}
}

func TestDocumentList_DefaultPage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"result":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "document", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if q := parseQuery(t, got.Query); q.Get("page") != "1" {
		t.Errorf("page = %q, want default 1 (BoldSign requires page)", q.Get("page"))
	}
}

func TestDocumentGet_DocumentIDQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"documentId":"doc-1","status":"InProgress"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "document", "get", "--id", "doc-1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v1/document/properties" {
		t.Errorf("path = %q, want /v1/document/properties", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("documentId") != "doc-1" {
		t.Errorf("documentId = %q", q.Get("documentId"))
	}
}

func TestDocumentDownload_WritesBytesAndReceipt(t *testing.T) {
	var got capturedRequest
	pdf := []byte("%PDF-1.7 signed bytes")
	srv := newBinaryServer(t, http.StatusOK, "application/pdf", pdf, &got)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "signed.pdf")
	code, stdout, _ := run(t, srv, "document", "download", "--id", "doc-1", "--out", out)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v1/document/download" {
		t.Errorf("path = %q, want /v1/document/download", got.Path)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(written) != string(pdf) {
		t.Errorf("written bytes = %q, want the PDF payload", written)
	}
	var receipt downloadReceipt
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &receipt); err != nil {
		t.Fatalf("receipt not JSON: %v (%s)", err, stdout)
	}
	if !receipt.OK || receipt.Path != out || receipt.Bytes != len(pdf) {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestDocumentAuditLog_Endpoint(t *testing.T) {
	var got capturedRequest
	srv := newBinaryServer(t, http.StatusOK, "application/pdf", []byte("audit"), &got)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "audit.pdf")
	code, _, _ := run(t, srv, "document", "audit-log", "--id", "doc-1", "--out", out)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v1/document/downloadAuditLog" {
		t.Errorf("path = %q, want /v1/document/downloadAuditLog", got.Path)
	}
}

func TestDocumentRemind_QueryEmailsBodyMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "document", "remind", "--id", "doc-1",
		"--email", "a@e.com", "--email", "b@e.com", "--message", "reminder")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/document/remind" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("documentId") != "doc-1" {
		t.Errorf("documentId = %q", q.Get("documentId"))
	}
	if emails := q["receiverEmails"]; len(emails) != 2 {
		t.Errorf("receiverEmails = %v, want two query values", emails)
	}
	if body := decodeBody(t, got.Body); body["Message"] != "reminder" {
		t.Errorf("Message = %v", body["Message"])
	}
	var receipt actionReceipt
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &receipt); err != nil {
		t.Fatalf("receipt not JSON: %v (%s)", err, stdout)
	}
	if !receipt.OK || receipt.Action != "remind" || receipt.DocumentID != "doc-1" {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestDocumentRevoke_RequiresMessage(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "document", "revoke", "--id", "doc-1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (missing required --message)", code)
	}
	if !strings.Contains(stderr, "message") {
		t.Errorf("stderr = %q, want required-flag message", stderr)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected on usage error, server saw %s", got.Method)
	}
}

func TestDocumentRevoke_SendsReasonAndReceipt(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusNoContent, ``, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "document", "revoke", "--id", "doc-1", "--message", "duplicate")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v1/document/revoke" {
		t.Errorf("path = %q", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("documentId") != "doc-1" {
		t.Errorf("documentId = %q", q.Get("documentId"))
	}
	if body := decodeBody(t, got.Body); body["Message"] != "duplicate" {
		t.Errorf("Message = %v", body["Message"])
	}
	var receipt actionReceipt
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &receipt); err != nil {
		t.Fatalf("receipt not JSON: %v (%s)", err, stdout)
	}
	if receipt.Action != "revoke" {
		t.Errorf("action = %q, want revoke", receipt.Action)
	}
}
