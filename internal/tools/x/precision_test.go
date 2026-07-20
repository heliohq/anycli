package x

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPostGetMultipleIDsUsesBatchLookup(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/2/tweets" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		want := url.Values{
			"ids":          {"1,2,3"},
			"tweet.fields": {defaultPostFields},
		}
		if r.URL.Query().Encode() != want.Encode() {
			t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
		}
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "get", "1", "2", "3")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestPostGetSingleIDKeepsSingleLookupPath(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2/tweets/123" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"id":"123"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "get", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestPostGetRejectsTooManyIDs(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "1"
	}
	code, _, stderr := run(t, server, fullEnv(), append([]string{"post", "get"}, ids...)...)
	if code == 0 || !strings.Contains(stderr, "at most 100") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestPostSearchSinceIDAndSortOrder(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		want := url.Values{
			"query":        {"from:helio"},
			"max_results":  {"10"},
			"since_id":     {"42"},
			"sort_order":   {"relevancy"},
			"tweet.fields": {defaultPostFields},
		}
		if r.URL.Query().Encode() != want.Encode() {
			t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
		}
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "post", "search", "--query", "from:helio", "--since-id", "42", "--sort-order", "relevancy")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestPostSearchRejectsUnknownSortOrder(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "post", "search", "--query", "q", "--sort-order", "newest")
	if code == 0 || !strings.Contains(stderr, "sort order") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestTimelineSinceID(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{name: "user", args: []string{"timeline", "user", "--since-id", "42"}, path: "/2/users/2244994945/tweets"},
		{name: "mentions", args: []string{"timeline", "mentions", "--since-id", "42"}, path: "/2/users/2244994945/mentions"},
		{name: "home", args: []string{"timeline", "home", "--since-id", "42"}, path: "/2/users/2244994945/timelines/reverse_chronological"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, tt.path)
				}
				if r.URL.Query().Get("since_id") != "42" {
					t.Fatalf("since_id = %q", r.URL.Query().Get("since_id"))
				}
				jsonResponse(w, http.StatusOK, `{"data":[]}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}
