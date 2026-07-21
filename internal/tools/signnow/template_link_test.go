package signnow

import (
	"net/http"
	"strings"
	"testing"
)

func TestTemplateCreate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /template": {http.StatusOK, `{"id":"tpl1"}`},
	})
	defer srv.Close()

	res, stdout, stderr := runSN(t, srv, "template", "create", "doc1", "--name", "NDA Template")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/template")
	if got == nil {
		t.Fatalf("no POST /template recorded")
	}
	body := bodyMap(t, got.Body)
	if body["document_id"] != "doc1" || body["document_name"] != "NDA Template" {
		t.Errorf("body = %v, want document_id/document_name", body)
	}
	if decodeStdout(t, stdout)["id"] != "tpl1" {
		t.Errorf("stdout id = %q, want tpl1", stdout)
	}
}

func TestTemplateCreate_MissingName_Exit2(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()
	res, _, _ := runSN(t, srv, "template", "create", "doc1")
	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for missing --name", res.exitCode)
	}
}

func TestTemplateCopy(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /template/tpl1/copy": {http.StatusOK, `{"id":"doc9"}`},
	})
	defer srv.Close()

	res, stdout, stderr := runSN(t, srv, "template", "copy", "tpl1", "--name", "Q3 NDA")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/template/tpl1/copy")
	if got == nil {
		t.Fatalf("no POST /template/tpl1/copy recorded")
	}
	if bodyMap(t, got.Body)["document_name"] != "Q3 NDA" {
		t.Errorf("body = %s, want document_name Q3 NDA", got.Body)
	}
	if decodeStdout(t, stdout)["id"] != "doc9" {
		t.Errorf("stdout id = %q, want doc9", stdout)
	}
}

func TestLinkCreate(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"POST /link": {http.StatusOK, `{"url":"https://app.signnow.com/l/abc","url_no_signup":"https://app.signnow.com/s/abc"}`},
	})
	defer srv.Close()

	res, stdout, stderr := runSN(t, srv, "link", "create", "doc1")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", res.exitCode, stderr)
	}
	got := findReq(reqs, http.MethodPost, "/link")
	if got == nil {
		t.Fatalf("no POST /link recorded")
	}
	if bodyMap(t, got.Body)["document_id"] != "doc1" {
		t.Errorf("body = %s, want document_id doc1", got.Body)
	}
	if !strings.Contains(stdout, "app.signnow.com") {
		t.Errorf("stdout = %q, want the signing link passthrough", stdout)
	}
}
