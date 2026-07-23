package formstack

import (
	"net/http"
	"testing"
)

func TestSubmissionList_SearchPairingAndTimeWindow(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"submissions":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv,
		"submission", "list", "555",
		"--since", "2026-07-01",
		"--until", "2026-07-21 23:59:59",
		"--search", "email=a@b.com",
		"--search", "status=paid",
		"--sort", "DESC",
		"--page", "3", "--per-page", "10",
	)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/form/555/submission.json" {
		t.Errorf("request = %s %s, want GET /form/555/submission.json", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("data") != "true" {
		t.Errorf("data = %q, want true (default)", q.Get("data"))
	}
	if q.Get("min_time") != "2026-07-01" || q.Get("max_time") != "2026-07-21 23:59:59" {
		t.Errorf("time window = min %q max %q", q.Get("min_time"), q.Get("max_time"))
	}
	if q.Get("search_field_0") != "email" || q.Get("search_value_0") != "a@b.com" {
		t.Errorf("search pair 0 = %q/%q", q.Get("search_field_0"), q.Get("search_value_0"))
	}
	if q.Get("search_field_1") != "status" || q.Get("search_value_1") != "paid" {
		t.Errorf("search pair 1 = %q/%q", q.Get("search_field_1"), q.Get("search_value_1"))
	}
	if q.Get("sort") != "DESC" || q.Get("page") != "3" || q.Get("per_page") != "10" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestSubmissionList_NoDataAndExpand(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"submissions":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "submission", "list", "555", "--no-data", "--expand-data"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	q := parseQuery(t, got.Query)
	if q.Get("data") != "false" {
		t.Errorf("data = %q, want false with --no-data", q.Get("data"))
	}
	if q.Get("expand_data") != "true" {
		t.Errorf("expand_data = %q, want true", q.Get("expand_data"))
	}
}

func TestSubmissionList_EncryptionHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"submissions":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "submission", "list", "555", "--encryption-password", "s3cret"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Encryption != "s3cret" {
		t.Errorf("%s = %q, want s3cret", EncryptionPasswordHeader, got.Encryption)
	}
}

func TestSubmissionList_BadSearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "submission", "list", "555", "--search", "novalue")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if got.Method != "" {
		t.Errorf("no request should be sent on a bad --search, got %s %s", got.Method, got.Path)
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}

func TestSubmissionGet_PathAndEncryption(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"7"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "submission", "get", "7", "--encryption-password", "pw"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/submission/7.json" {
		t.Errorf("path = %q, want /submission/7.json", got.Path)
	}
	if got.Encryption != "pw" {
		t.Errorf("encryption header = %q", got.Encryption)
	}
}

func TestSubmissionCreate_FieldParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"1001"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "submission", "create", "555", "--field", "123=Alice", "--field", "124=alice@x.com", "--read")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/form/555/submission.json" {
		t.Errorf("request = %s %s, want POST /form/555/submission.json", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["field_123"] != "Alice" || body["field_124"] != "alice@x.com" {
		t.Errorf("body field params = %v", body)
	}
	if body["read"] != true {
		t.Errorf("read = %v, want true", body["read"])
	}
}

func TestSubmissionDelete_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"success":"1"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "submission", "delete", "1001"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/submission/1001.json" {
		t.Errorf("request = %s %s, want DELETE /submission/1001.json", got.Method, got.Path)
	}
}
