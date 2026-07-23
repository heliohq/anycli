package formstack

import (
	"net/http"
	"reflect"
	"testing"
)

func TestFieldGet_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"321"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "field", "get", "321"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/field/321.json" {
		t.Errorf("request = %s %s, want GET /field/321.json", got.Method, got.Path)
	}
}

func TestFieldCreate_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"400"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "field", "create", "555", "--type", "select", "--label", "Plan", "--options", "a,b,c", "--required", "--hidden")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/form/555/field.json" {
		t.Errorf("request = %s %s, want POST /form/555/field.json", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["field_type"] != "select" || body["label"] != "Plan" {
		t.Errorf("body = %v", body)
	}
	if body["required"] != true || body["hidden"] != true {
		t.Errorf("required/hidden = %v/%v", body["required"], body["hidden"])
	}
	opts, ok := body["options"].([]any)
	if !ok || !reflect.DeepEqual(opts, []any{"a", "b", "c"}) {
		t.Errorf("options = %v", body["options"])
	}
}

func TestFieldCreate_OmitsUnsetFlags(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"401"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "field", "create", "555", "--type", "text", "--label", "Name"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	body := decodeBody(t, got.Body)
	if _, ok := body["required"]; ok {
		t.Errorf("required should be omitted when unset, body = %v", body)
	}
	if _, ok := body["options"]; ok {
		t.Errorf("options should be omitted when unset, body = %v", body)
	}
}

func TestFolderList_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"folders":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "folder", "list"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/folder.json" {
		t.Errorf("request = %s %s, want GET /folder.json", got.Method, got.Path)
	}
}

func TestWebhookList_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "webhook", "list", "555"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/form/555/webhook.json" {
		t.Errorf("path = %q, want /form/555/webhook.json", got.Path)
	}
}

func TestWebhookGet_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"9"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "webhook", "get", "9"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/webhook/9.json" {
		t.Errorf("path = %q, want /webhook/9.json", got.Path)
	}
}

func TestWebhookCreate_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"9"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "webhook", "create", "555", "--url", "https://example.com/hook", "--content-type", "json")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/form/555/webhook.json" {
		t.Errorf("request = %s %s, want POST /form/555/webhook.json", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["url"] != "https://example.com/hook" || body["content_type"] != "json" {
		t.Errorf("body = %v", body)
	}
}

func TestWebhookDelete_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":"1"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "webhook", "delete", "9"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/webhook/9.json" {
		t.Errorf("request = %s %s, want DELETE /webhook/9.json", got.Method, got.Path)
	}
}
