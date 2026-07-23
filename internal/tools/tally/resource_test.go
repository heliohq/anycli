package tally

import (
	"net/http"
	"strings"
	"testing"
)

func TestFormList_QueryParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[],"page":1,"limit":50,"hasMore":false}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "form", "list", "--workspace", "ws1", "--workspace", "ws2", "--page", "2", "--limit", "10")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/forms" {
		t.Errorf("path = %q, want /forms", got.Path)
	}
	q := parseQuery(t, got.Query)
	if got, want := q["workspaceIds"], []string{"ws1", "ws2"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("workspaceIds = %v, want %v", got, want)
	}
	if q.Get("page") != "2" || q.Get("limit") != "10" {
		t.Errorf("page/limit = %q/%q, want 2/10", q.Get("page"), q.Get("limit"))
	}
}

func TestFormList_NoPagingFlags_OmitsQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "list"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Query != "" {
		t.Errorf("query = %q, want empty when no flags set", got.Query)
	}
}

func TestSubmissionList_FilterAndCursor(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "submission", "list",
		"--form", "f1", "--filter", "completed", "--after-id", "s9",
		"--start-date", "2026-01-01", "--end-date", "2026-02-01", "--limit", "25")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/forms/f1/submissions" {
		t.Errorf("path = %q, want /forms/f1/submissions", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("filter") != "completed" || q.Get("afterId") != "s9" ||
		q.Get("startDate") != "2026-01-01" || q.Get("endDate") != "2026-02-01" || q.Get("limit") != "25" {
		t.Errorf("query = %q, missing expected params", got.Query)
	}
}

func TestSubmissionList_InvalidFilter_Exit2(t *testing.T) {
	result, _, stderr := runResult(t, nil, nil, "submission", "list", "--form", "f1", "--filter", "bogus")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for invalid filter enum", result.ExitCode)
	}
	if !strings.Contains(stderr, "filter") {
		t.Errorf("stderr = %q, want the enum message", stderr)
	}
}

func TestSubmissionGet_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"s1"}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "submission", "get", "--form", "f1", "--submission", "s1"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/forms/f1/submissions/s1" {
		t.Errorf("path = %q, want /forms/f1/submissions/s1", got.Path)
	}
}

func TestAnalytics_RequiresPeriod_Exit2(t *testing.T) {
	result, _, stderr := runResult(t, nil, nil, "analytics", "metrics", "--form", "f1")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 when --period missing", result.ExitCode)
	}
	if !strings.Contains(stderr, "period") {
		t.Errorf("stderr = %q, want the required-period message", stderr)
	}
}

func TestAnalytics_DropOffPathAndPeriod(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "analytics", "drop-off", "--form", "f1", "--period", "7d")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/forms/f1/analytics/drop-off" {
		t.Errorf("path = %q, want /forms/f1/analytics/drop-off", got.Path)
	}
	if parseQuery(t, got.Query).Get("period") != "7d" {
		t.Errorf("period = %q, want 7d", parseQuery(t, got.Query).Get("period"))
	}
}

func TestAnalytics_InvalidPeriod_Exit2(t *testing.T) {
	result, _, _ := runResult(t, nil, nil, "analytics", "visits", "--form", "f1", "--period", "decade")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for invalid period enum", result.ExitCode)
	}
}

func TestFormCreate_FromStdin(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"f_new"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, strings.NewReader(`{"status":"DRAFT"}`), "form", "create", "--stdin")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if got.Method != http.MethodPost || got.Path != "/forms" {
		t.Errorf("request = %s %s, want POST /forms", got.Method, got.Path)
	}
	if got.CType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.CType)
	}
	if strings.TrimSpace(string(got.Body)) != `{"status":"DRAFT"}` {
		t.Errorf("body = %q, want passthrough JSON", string(got.Body))
	}
}

func TestFormCreate_InvalidJSON_Exit2(t *testing.T) {
	result, _, stderr := runResult(t, nil, strings.NewReader(`{not json`), "form", "create", "--stdin")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for invalid JSON body", result.ExitCode)
	}
	if !strings.Contains(stderr, "valid JSON") {
		t.Errorf("stderr = %q, want the invalid-JSON message", stderr)
	}
}

func TestFormCreate_NoBodySource_Exit2(t *testing.T) {
	result, _, _ := runResult(t, nil, nil, "form", "create")
	if result.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 when neither --file nor --stdin given", result.ExitCode)
	}
}

func TestFormUpdate_PatchPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"f1"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, strings.NewReader(`{"name":"x"}`), "form", "update", "--form", "f1", "--stdin")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if got.Method != http.MethodPatch || got.Path != "/forms/f1" {
		t.Errorf("request = %s %s, want PATCH /forms/f1", got.Method, got.Path)
	}
}

func TestFormDelete_DeletePath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "delete", "--form", "f1"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Method != http.MethodDelete || got.Path != "/forms/f1" {
		t.Errorf("request = %s %s, want DELETE /forms/f1", got.Method, got.Path)
	}
}

func TestWebhookList_And_Workspace(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"items":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "webhook", "list", "--page", "1"); code != 0 {
		t.Fatalf("webhook list exit = %d, want 0", code)
	}
	if got.Path != "/webhooks" {
		t.Errorf("path = %q, want /webhooks", got.Path)
	}

	if code, _, _ := run(t, srv, "workspace", "list"); code != 0 {
		t.Fatalf("workspace list exit = %d, want 0", code)
	}
	if got.Path != "/workspaces" {
		t.Errorf("path = %q, want /workspaces", got.Path)
	}
}

func TestFormQuestions_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"questions":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "questions", "--form", "f1"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Path != "/forms/f1/questions" {
		t.Errorf("path = %q, want /forms/f1/questions", got.Path)
	}
}
