package x

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestLikeCreateAndDeleteUseConnectedUser(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		method     string
		path       string
		wantBody   string
		statusCode int
	}{
		{name: "create", args: []string{"like", "create", "123"}, method: http.MethodPost, path: "/2/users/2244994945/likes", wantBody: `{"tweet_id":"123"}`, statusCode: http.StatusOK},
		{name: "delete", args: []string{"like", "delete", "123"}, method: http.MethodDelete, path: "/2/users/2244994945/likes/123", statusCode: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				got := captureRequest(t, r)
				if got.Method != tt.method || got.Path != tt.path {
					t.Fatalf("request = %s %s", got.Method, got.Path)
				}
				if tt.wantBody != "" && string(got.Body) != tt.wantBody {
					t.Fatalf("body = %s, want %s", got.Body, tt.wantBody)
				}
				jsonResponse(w, tt.statusCode, `{"data":{"liked":true}}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestLikeRequiresConnectedUserID(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, map[string]string{EnvAccessToken: "token"}, "like", "create", "123")
	if code == 0 || !strings.Contains(stderr, "X_USER_ID is not set") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestPostLikingUsersAndReposters(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{name: "liking-users", args: []string{"post", "liking-users", "123", "--limit", "30", "--next-token", "next"}, path: "/2/tweets/123/liking_users"},
		{name: "reposters", args: []string{"post", "reposters", "123", "--limit", "30", "--next-token", "next"}, path: "/2/tweets/123/retweeted_by"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tt.path {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				want := url.Values{
					"max_results":      {"30"},
					"pagination_token": {"next"},
				}
				if r.URL.Query().Encode() != want.Encode() {
					t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
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
