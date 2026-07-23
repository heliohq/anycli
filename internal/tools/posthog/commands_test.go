package posthog

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestQueryRunWrapsHogQL(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusOK, `{"results":[["pageview",5]]}`, &got)
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "query", "run", "--project", "1", "--hogql", "select event, count() from events group by event")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/api/projects/1/query/" {
		t.Fatalf("request = %s %s, want POST /api/projects/1/query/", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	query, ok := body["query"].(map[string]any)
	if !ok {
		t.Fatalf("body.query is not an object: %v", body)
	}
	if query["kind"] != "HogQLQuery" || query["query"] != "select event, count() from events group by event" {
		t.Fatalf("query node = %v, want HogQLQuery wrapper", query)
	}
	if stdout == "" {
		t.Fatalf("stdout empty, want provider passthrough")
	}
}

func TestQueryRunPassesThroughRawQueryNode(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "q.json")
	if err := os.WriteFile(file, []byte(`{"kind":"TrendsQuery","series":[{"event":"$pageview"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	exit, _, stderr := run(t, srv, "query", "run", "--project", "7", "--query-json", file)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	body := decodeBody(t, got.Body)
	query, ok := body["query"].(map[string]any)
	if !ok || query["kind"] != "TrendsQuery" {
		t.Fatalf("query node = %v, want raw TrendsQuery passthrough", body["query"])
	}
}

func TestQueryRunRejectsBothQuerySources(t *testing.T) {
	exit, _, _ := runService(t, &Service{BaseURL: "https://unused.example"},
		map[string]string{EnvAccessToken: testToken},
		"query", "run", "--project", "1", "--hogql", "select 1", "--query-json", "-")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for mutually exclusive flags", exit)
	}
}

func TestProjectScopedCommandRequiresProject(t *testing.T) {
	exit, _, stderr := runService(t, &Service{BaseURL: "https://unused.example"},
		map[string]string{EnvAccessToken: testToken}, "insight", "list")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 when --project missing", exit)
	}
	if stderr == "" {
		t.Fatalf("want a usage message on stderr")
	}
}

func TestListCommandMapsPagingAndSearch(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusOK, `{"count":0,"next":null,"previous":null,"results":[]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "insight", "list", "--project", "1", "--limit", "10", "--offset", "20", "--search", "funnel")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if got.Path != "/api/projects/1/insights/" {
		t.Fatalf("path = %q, want /api/projects/1/insights/", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "10" || q.Get("offset") != "20" || q.Get("search") != "funnel" {
		t.Fatalf("query = %v, want limit=10 offset=20 search=funnel", q)
	}
}

func TestGetCommandBuildsIDPath(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusOK, `{"id":42}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "insight", "get", "--project", "1", "--id", "42")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if got.Path != "/api/projects/1/insights/42/" {
		t.Fatalf("path = %q, want /api/projects/1/insights/42/", got.Path)
	}
}

func TestFlagToggleSendsActivePatch(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusOK, `{"id":55,"active":false}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "flag", "toggle", "--project", "1", "--id", "55", "--active=false")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if got.Method != http.MethodPatch || got.Path != "/api/projects/1/feature_flags/55/" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["active"] != false {
		t.Fatalf("body = %v, want active=false", body)
	}
}

func TestFlagToggleRequiresActive(t *testing.T) {
	exit, _, _ := runService(t, &Service{BaseURL: "https://unused.example"},
		map[string]string{EnvAccessToken: testToken},
		"flag", "toggle", "--project", "1", "--id", "55")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 when --active omitted", exit)
	}
}

func TestFlagCreatePostsRawBody(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusCreated, `{"id":99}`, &got)
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "flag.json")
	if err := os.WriteFile(file, []byte(`{"key":"new-flag","active":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	exit, _, stderr := run(t, srv, "flag", "create", "--project", "1", "--data", file)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/api/projects/1/feature_flags/" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["key"] != "new-flag" || body["active"] != true {
		t.Fatalf("body = %v, want raw flag passthrough", body)
	}
}

func TestAnnotationCreateBuildsPayload(t *testing.T) {
	got := capturedRequest{}
	srv := singleRouteServer(t, http.StatusCreated, `{"id":1}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "annotation", "create", "--project", "1",
		"--content", "deploy v2", "--date-marker", "2026-07-22T00:00:00Z", "--scope", "project")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", exit, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/api/projects/1/annotations/" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["content"] != "deploy v2" || body["date_marker"] != "2026-07-22T00:00:00Z" || body["scope"] != "project" {
		t.Fatalf("body = %v", body)
	}
}

func TestAnnotationCreateRequiresContent(t *testing.T) {
	exit, _, _ := runService(t, &Service{BaseURL: "https://unused.example"},
		map[string]string{EnvAccessToken: testToken},
		"annotation", "create", "--project", "1")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 when --content omitted", exit)
	}
}
