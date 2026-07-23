package jotform

import (
	"net/http"
	"testing"
)

func TestSubmissionCreate_FormEncodesFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":{"submissionID":"1"}}`, &got)
	defer srv.Close()

	code, _, se := run(t, srv, "submission", "create", "form-7",
		"--field", "3=hello world",
		"--field", "5:first=Ada",
		"--field", "5:last=Lovelace",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, se)
	}
	if got.Method != http.MethodPost || got.Path != "/form/form-7/submissions" {
		t.Errorf("request = %s %s, want POST /form/form-7/submissions", got.Method, got.Path)
	}
	if got.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q", got.ContentType)
	}
	form := parseForm(t, got.Body)
	if form.Get("submission[3]") != "hello world" {
		t.Errorf("submission[3] = %q, want 'hello world'", form.Get("submission[3]"))
	}
	if form.Get("submission[5][first]") != "Ada" {
		t.Errorf("submission[5][first] = %q, want Ada", form.Get("submission[5][first]"))
	}
	if form.Get("submission[5][last]") != "Lovelace" {
		t.Errorf("submission[5][last] = %q, want Lovelace", form.Get("submission[5][last]"))
	}
}

func TestSubmissionEdit_PostsToSubmissionPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":{}}`, &got)
	defer srv.Close()

	code, _, se := run(t, srv, "submission", "edit", "sub-3", "--field", "3=updated")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, se)
	}
	if got.Method != http.MethodPost || got.Path != "/submission/sub-3" {
		t.Errorf("request = %s %s, want POST /submission/sub-3", got.Method, got.Path)
	}
	if form := parseForm(t, got.Body); form.Get("submission[3]") != "updated" {
		t.Errorf("submission[3] = %q", form.Get("submission[3]"))
	}
}

func TestSubmissionCreate_ValueWithEquals(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":{}}`, &got)
	defer srv.Close()

	// The split is on the first '=' only, so a value may itself contain '='.
	if code, _, _ := run(t, srv, "submission", "create", "f1", "--field", "9=a=b=c"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if v := parseForm(t, got.Body).Get("submission[9]"); v != "a=b=c" {
		t.Errorf("submission[9] = %q, want a=b=c", v)
	}
}

func TestSubmissionCreate_BadFieldSyntax_Exit2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "submission", "create", "f1", "--field", "noequalshere")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP request should be made on a usage error, saw %s %s", got.Method, got.Path)
	}
	if stderr == "" {
		t.Error("expected a usage error on stderr")
	}
}

func TestSubmissionDelete_Method(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":{}}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "submission", "delete", "sub-9"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/submission/sub-9" {
		t.Errorf("request = %s %s, want DELETE /submission/sub-9", got.Method, got.Path)
	}
}
