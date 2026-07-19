package notion

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPI_JSONBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"ok":true}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "api", "POST", "/file_uploads", "--body", `{"mode":"single_part"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/file_uploads" {
		t.Errorf("request = %s %s, want POST /file_uploads", got.Method, got.Path)
	}
	assertAuth(t, got, notionVersion)
	if !strings.Contains(string(got.Body), "single_part") || !strings.Contains(stdout, "ok") {
		t.Errorf("body/stdout = %s / %q", got.Body, stdout)
	}
}

func TestAPI_MultipartFormFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invoice.pdf")
	if err := os.WriteFile(path, []byte("pdf-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"file_upload","id":"fu1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "api", "POST", "/file_uploads/fu1/send", "--form-file", "file="+path, "--form", "part_number=1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/file_uploads/fu1/send" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	assertAuth(t, got, notionVersion)
	if !strings.Contains(string(got.Body), "pdf-bytes") || !strings.Contains(string(got.Body), "part_number") {
		t.Errorf("multipart body = %s", got.Body)
	}
	if !strings.Contains(string(got.Body), "Content-Type: application/pdf") {
		t.Errorf("multipart body = %s, want file part Content-Type: application/pdf", got.Body)
	}
}

func TestAPI_RejectsAuthHeaderOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "api", "GET", "/users/me", "--header", "Authorization: nope")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "cannot be overridden") {
		t.Errorf("stderr = %q", stderr)
	}
	if got.Path != "" {
		t.Errorf("no request must be sent, got %s", got.Path)
	}
}

func TestFileUpload_Happy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invoice.pdf")
	if err := os.WriteFile(path, []byte("pdf-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /file_uploads":          {http.StatusOK, `{"object":"file_upload","id":"fu1"}`},
		"POST /file_uploads/fu1/send": {http.StatusOK, `{"object":"file_upload","id":"fu1","status":"uploaded"}`},
	})
	defer srv.Close()

	code, stdout, _ := run(t, srv, "file", "upload", path, "--name", "invoice.pdf", "--content-type", "application/pdf")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) != "fu1" {
		t.Fatalf("stdout = %q, want fu1", stdout)
	}
	create := findReq(reqs, http.MethodPost, "/file_uploads")
	if create == nil {
		t.Fatalf("create request missing; reqs=%v", reqs)
	}
	body := bodyMap(t, create.Body)
	if body["mode"] != "single_part" || body["filename"] != "invoice.pdf" || body["content_type"] != "application/pdf" {
		t.Errorf("create body = %v", body)
	}
	send := findReq(reqs, http.MethodPost, "/file_uploads/fu1/send")
	if send == nil || !strings.Contains(string(send.Body), "pdf-bytes") {
		t.Fatalf("send request = %#v", send)
	}
	if !strings.HasPrefix(send.ContentType, "multipart/form-data;") {
		t.Errorf("send Content-Type = %q, want multipart/form-data", send.ContentType)
	}
	if !strings.Contains(string(send.Body), "Content-Type: application/pdf") {
		t.Errorf("send multipart body = %s, want file part Content-Type: application/pdf", send.Body)
	}
}

func TestFileUpload_SendFailurePreservesJSONStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invoice.pdf")
	if err := os.WriteFile(path, []byte("pdf-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /file_uploads":          {http.StatusOK, `{"object":"file_upload","id":"fu1"}`},
		"POST /file_uploads/fu1/send": {http.StatusUnsupportedMediaType, `{"object":"error","code":"validation_error","message":"content type mismatch"}`},
	})
	defer srv.Close()

	code, _, stderr := run(t, srv, "file", "upload", path, "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var envelope struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not JSON: %v (%q)", err, stderr)
	}
	if envelope.Error.Kind != "api" || envelope.Error.Status != http.StatusUnsupportedMediaType {
		t.Fatalf("error envelope = %#v, want api status %d", envelope.Error, http.StatusUnsupportedMediaType)
	}
}

func TestFileAttach_UploadID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"object":"page","id":"p1"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "file", "attach", "p1", "--property", "Invoice", "--upload-id", "43833259-72ae-404e-8441-b6577f3159b4", "--name", "invoice.pdf")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) != "p1" {
		t.Errorf("stdout = %q", stdout)
	}
	body := bodyMap(t, got.Body)
	props := body["properties"].(map[string]any)
	invoice := props["Invoice"].(map[string]any)
	files := invoice["files"].([]any)
	file := files[0].(map[string]any)
	if file["type"] != "file_upload" || file["name"] != "invoice.pdf" {
		t.Errorf("file object = %v", file)
	}
}
