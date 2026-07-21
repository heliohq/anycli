package boldsign

import (
	"net/http"
	"strings"
	"testing"
)

func TestTemplateList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"result":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "template", "list", "--page", "3", "--page-size", "5", "--search", "nda")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v1/template/list" {
		t.Errorf("path = %q, want /v1/template/list", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("page") != "3" || q.Get("pageSize") != "5" || q.Get("searchKey") != "nda" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestTemplateGet_TemplateIDQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"templateId":"tpl-1","roles":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "template", "get", "--id", "tpl-1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/v1/template/properties" {
		t.Errorf("path = %q, want /v1/template/properties", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("templateId") != "tpl-1" {
		t.Errorf("templateId = %q", q.Get("templateId"))
	}
}

func TestTemplateSend_RolesAndFields(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"documentId":"doc-9"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "template", "send", "--id", "tpl-1",
		"--title", "Signed NDA", "--message", "hello",
		"--role", "1:Alice <alice@example.com>", "--role", "2:Bob <bob@example.com>",
		"--field", "company=Acme", "--signing-order")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/template/send" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("templateId") != "tpl-1" {
		t.Errorf("templateId query = %q", q.Get("templateId"))
	}
	body := decodeBody(t, got.Body)
	if body["Title"] != "Signed NDA" || body["Message"] != "hello" {
		t.Errorf("Title/Message = %v / %v", body["Title"], body["Message"])
	}
	if body["EnableSigningOrder"] != true {
		t.Errorf("EnableSigningOrder = %v", body["EnableSigningOrder"])
	}
	roles, ok := body["Roles"].([]any)
	if !ok || len(roles) != 2 {
		t.Fatalf("Roles = %v, want two", body["Roles"])
	}
	r0 := roles[0].(map[string]any)
	if r0["RoleIndex"].(float64) != 1 || r0["SignerName"] != "Alice" || r0["SignerEmail"] != "alice@example.com" {
		t.Errorf("role0 = %v", r0)
	}
	if r0["SignerOrder"].(float64) != 1 {
		t.Errorf("role0 SignerOrder = %v, want 1", r0["SignerOrder"])
	}
	fields, ok := body["ExistingFormFields"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("ExistingFormFields = %v, want one", body["ExistingFormFields"])
	}
	f0 := fields[0].(map[string]any)
	if f0["Id"] != "company" || f0["Value"] != "Acme" {
		t.Errorf("field0 = %v", f0)
	}
}

func TestTemplateSend_MissingRoleIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "template", "send", "--id", "tpl-1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "at least one --role") {
		t.Errorf("stderr = %q", stderr)
	}
	if got.Method != "" {
		t.Errorf("no HTTP call expected, server saw %s", got.Method)
	}
}

func TestTemplateSend_BadRoleIndexIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "template", "send", "--id", "tpl-1",
		"--role", "99:Alice <a@e.com>")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "between 1 and 50") {
		t.Errorf("stderr = %q, want range message", stderr)
	}
}
