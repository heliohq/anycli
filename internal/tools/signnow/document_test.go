package signnow

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDocumentList_MergesBothEndpoints proves the list covers both the modified
// (documentsv2) and freshly-uploaded (documents) legs and dedupes by id.
func TestDocumentList_MergesBothEndpoints(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /user/documentsv2": {http.StatusOK, `[{"id":"m1","document_name":"Modified","field_invites":[{"id":"fi1","email":"s@x.com","status":"pending"}]},{"id":"dup","document_name":"Both"}]`},
		"GET /user/documents":   {http.StatusOK, `[{"id":"f1","document_name":"Fresh"},{"id":"dup","document_name":"Both"}]`},
	})
	defer srv.Close()

	res, stdout, stderr := runSN(t, srv, "document", "list")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	if findReq(reqs, http.MethodGet, "/user/documentsv2") == nil {
		t.Errorf("did not query the modified leg /user/documentsv2")
	}
	if findReq(reqs, http.MethodGet, "/user/documents") == nil {
		t.Errorf("did not query the fresh leg /user/documents")
	}
	out := decodeStdout(t, stdout)
	docs, ok := out["documents"].([]any)
	if !ok {
		t.Fatalf("output has no documents array: %v", out)
	}
	if len(docs) != 3 {
		t.Fatalf("got %d docs, want 3 (m1, dup once, f1)", len(docs))
	}
	ids := map[string]bool{}
	for _, d := range docs {
		ids[d.(map[string]any)["id"].(string)] = true
	}
	for _, want := range []string{"m1", "f1", "dup"} {
		if !ids[want] {
			t.Errorf("missing id %q in %v", want, ids)
		}
	}
}

func TestDocumentList_Limit(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /user/documentsv2": {http.StatusOK, `[{"id":"a"},{"id":"b"},{"id":"c"}]`},
		"GET /user/documents":   {http.StatusOK, `[]`},
	})
	defer srv.Close()

	_, stdout, _ := runSN(t, srv, "document", "list", "--limit", "2")
	out := decodeStdout(t, stdout)
	if got := len(out["documents"].([]any)); got != 2 {
		t.Fatalf("got %d docs, want 2 after --limit", got)
	}
}

func TestDocumentGet_Projection(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /document/doc1": {http.StatusOK, `{"id":"doc1","document_name":"NDA","created":"1699999999","roles":[{"name":"Signer 1"}],"field_invites":[{"id":"fi1","email":"s@x.com","role":"Signer 1","status":"pending"}],"signatures":[{"id":"sig1"}]}`},
	})
	defer srv.Close()

	res, stdout, _ := runSN(t, srv, "document", "get", "doc1")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.exitCode)
	}
	out := decodeStdout(t, stdout)
	if out["document_name"] != "NDA" || out["created"] != "1699999999" {
		t.Errorf("projection = %v, want NDA / created string", out)
	}
	if got := out["signatures_count"].(float64); got != 1 {
		t.Errorf("signatures_count = %v, want 1", got)
	}
	if len(out["field_invites"].([]any)) != 1 {
		t.Errorf("field_invites not projected: %v", out["field_invites"])
	}
}

func TestDocumentUpload_Multipart(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /document": {http.StatusOK, `{"id":"newdoc"}`},
	})
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "contract.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, stdout, stderr := runSN(t, srv, "document", "upload", "--file", path)
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/document")
	if got == nil {
		t.Fatalf("no POST /document recorded: %+v", reqs)
	}
	if !strings.HasPrefix(got.ContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data", got.ContentType)
	}
	if !strings.Contains(string(got.Body), "contract.pdf") {
		t.Errorf("multipart body missing the filename: %s", got.Body)
	}
	if decodeStdout(t, stdout)["id"] != "newdoc" {
		t.Errorf("stdout id = %v, want newdoc", stdout)
	}
}

func TestDocumentUpload_ExtractFields_Path(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /document/fieldextract": {http.StatusOK, `{"id":"tagged"}`},
	})
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "tagged.pdf")
	if err := os.WriteFile(path, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _, stderr := runSN(t, srv, "document", "upload", "--file", path, "--extract-fields", "--name", "Renamed")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/document/fieldextract")
	if got == nil {
		t.Fatalf("--extract-fields must route to /document/fieldextract: %+v", reqs)
	}
	if !strings.Contains(string(got.Body), "Renamed") {
		t.Errorf("--name did not set the multipart filename: %s", got.Body)
	}
}

func TestDocumentUpload_MissingFile_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	res, _, _ := runSN(t, srv, "document", "upload")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for missing --file", res.exitCode)
	}
}

func TestDocumentAddFields_Body(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"PUT /document/doc1": {http.StatusOK, `{"id":"doc1"}`},
	})
	defer srv.Close()

	fields := `[{"x":10,"y":20,"page_number":0,"type":"signature","role":"Signer 1"}]`
	res, _, stderr := runSN(t, srv, "document", "add-fields", "doc1", "--fields", fields)
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPut, "/document/doc1")
	if got == nil {
		t.Fatalf("no PUT /document/doc1 recorded")
	}
	body := bodyMap(t, got.Body)
	arr, ok := body["fields"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("body.fields = %v, want a 1-element array", body["fields"])
	}
}

func TestDocumentAddFields_InvalidJSON_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	res, _, _ := runSN(t, srv, "document", "add-fields", "doc1", "--fields", "{not an array}")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for invalid --fields JSON", res.exitCode)
	}
}

func TestDocumentDownload_WritesFile(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /document/doc1/download": {http.StatusOK, "PDFBYTES"},
	})
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "signed.pdf")
	res, stdout, stderr := runSN(t, srv, "document", "download", "doc1", "--out", out, "--with-history")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodGet, "/document/doc1/download")
	if got == nil {
		t.Fatalf("no download request recorded")
	}
	if got.Query.Get("type") != "collapsed" {
		t.Errorf("type query = %q, want collapsed", got.Query.Get("type"))
	}
	if got.Query.Get("with_history") != "1" {
		t.Errorf("with_history query = %q, want 1", got.Query.Get("with_history"))
	}
	data, err := os.ReadFile(out)
	if err != nil || string(data) != "PDFBYTES" {
		t.Fatalf("downloaded file = %q err=%v, want PDFBYTES", data, err)
	}
	if decodeStdout(t, stdout)["bytes"].(float64) != float64(len("PDFBYTES")) {
		t.Errorf("bytes count wrong: %s", stdout)
	}
}

func TestDocumentDownload_MissingOut_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	res, _, _ := runSN(t, srv, "document", "download", "doc1")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for missing --out", res.exitCode)
	}
}

func TestDocumentDelete(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"DELETE /document/doc1": {http.StatusOK, `{"status":"success"}`},
	})
	defer srv.Close()

	res, stdout, _ := runSN(t, srv, "document", "delete", "doc1")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.exitCode)
	}
	if findReq(reqs, http.MethodDelete, "/document/doc1") == nil {
		t.Fatalf("no DELETE recorded")
	}
	if decodeStdout(t, stdout)["status"] != "deleted" {
		t.Errorf("stdout = %q, want status deleted", stdout)
	}
}
