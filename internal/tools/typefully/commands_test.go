package typefully

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocialSetList_Paging(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"id":1}]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "social-set", "list", "--limit", "3")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets" {
		t.Errorf("path = %q", got.Path)
	}
	if parseQuery(t, got.Query).Get("limit") != "3" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestSocialSetGet_TrailingSlash(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":"ss1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "social-set", "get", "--social-set", "ss1")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/" {
		t.Errorf("path = %q, want trailing slash", got.Path)
	}
}

func TestTagCreate_Body(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusCreated, `{"id":"tag1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "tag", "create", "--social-set", "ss1", "--name", "launch")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/social-sets/ss1/tags" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if decodeBody(t, got.Body)["name"] != "launch" {
		t.Errorf("body = %s", got.Body)
	}
}

func TestQueueView_DateWindow(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"slots":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "queue", "view", "--social-set", "ss1", "--start-date", "2026-01-01", "--end-date", "2026-01-31")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/queue" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("start_date") != "2026-01-01" || q.Get("end_date") != "2026-01-31" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestQueueScheduleSet_PUT(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "queue", "schedule-set", "--social-set", "ss1", "--data", `{"rules":[]}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodPut || got.Path != "/social-sets/ss1/queue/schedule" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
}

func TestAnalyticsPosts_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "analytics", "posts", "--social-set", "ss1", "--platform", "x", "--include-replies")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/analytics/x/posts" {
		t.Errorf("path = %q", got.Path)
	}
	if parseQuery(t, got.Query).Get("include_replies") != "true" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestAnalyticsFollowers_DefaultPlatform(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "analytics", "followers", "--social-set", "ss1")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/analytics/x/followers" {
		t.Errorf("path = %q, want default platform x", got.Path)
	}
}

func TestLinkedInResolveOrg_Query(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"mention":"@acme"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "linkedin", "resolve-org", "--social-set", "ss1", "--organization-url", "https://linkedin.com/company/acme")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/linkedin/organizations/resolve" {
		t.Errorf("path = %q", got.Path)
	}
	if parseQuery(t, got.Query).Get("organization_url") != "https://linkedin.com/company/acme" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestCommentThreads_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "comment", "threads", "--social-set", "ss1", "--id", "9", "--platform", "x", "--limit", "5")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Path != "/social-sets/ss1/drafts/9/comment-threads" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("platform") != "x" || q.Get("limit") != "5" {
		t.Errorf("query = %q", got.Query)
	}
}

// TestMediaUpload_TwoStep drives the presign POST then the PUT of the file
// bytes to the returned upload_url, and asserts the emitted media_id receipt.
func TestMediaUpload_TwoStep(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "pic.png")
	if err := os.WriteFile(file, []byte("PNGDATA"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	var putPath, postPath string
	var putBody []byte
	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/media/upload"):
			postPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Point the PUT back at this same test server.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"upload_url": serverURL + "/put-target",
				"media_id":   "mid-1",
			})
		case r.Method == http.MethodPut:
			putPath = r.URL.Path
			putBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	serverURL = srv.URL // set before Execute drives any request

	var out, errBuf bytes.Buffer
	svc := &Service{BaseURL: srv.URL, HC: srv.Client(), Out: &out, Err: &errBuf}
	result, err := svc.Execute(context.Background(), []string{"media", "upload", "--social-set", "ss1", "--file", file}, map[string]string{EnvAPIKey: "key-123"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr=%s", result.ExitCode, errBuf.String())
	}
	if postPath != "/social-sets/ss1/media/upload" {
		t.Errorf("post path = %q", postPath)
	}
	if putPath != "/put-target" {
		t.Errorf("put path = %q, want /put-target", putPath)
	}
	if string(putBody) != "PNGDATA" {
		t.Errorf("put body = %q, want PNGDATA", putBody)
	}
	if !strings.Contains(out.String(), `"media_id":"mid-1"`) {
		t.Errorf("stdout = %q, want media_id receipt", out.String())
	}
}
