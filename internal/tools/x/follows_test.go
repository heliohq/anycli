package x

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFollowCreateAndDeleteUseConnectedUser(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		method     string
		path       string
		wantBody   string
		statusCode int
	}{
		{name: "create", args: []string{"follow", "create", "555"}, method: http.MethodPost, path: "/2/users/2244994945/following", wantBody: `{"target_user_id":"555"}`, statusCode: http.StatusOK},
		{name: "delete", args: []string{"follow", "delete", "555"}, method: http.MethodDelete, path: "/2/users/2244994945/following/555", statusCode: http.StatusOK},
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
				jsonResponse(w, tt.statusCode, `{"data":{"following":true}}`)
			})
			defer server.Close()
			code, _, stderr := run(t, server, fullEnv(), tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestFollowRequiresConnectedUserID(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, map[string]string{EnvAccessToken: "token"}, "follow", "create", "555")
	if code == 0 || !strings.Contains(stderr, "X_USER_ID is not set") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestUserFollowersAndFollowing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{name: "followers default connected user", args: []string{"user", "followers", "--limit", "200", "--next-token", "next"}, path: "/2/users/2244994945/followers"},
		{name: "following explicit user", args: []string{"user", "following", "--user-id", "555", "--limit", "200", "--next-token", "next"}, path: "/2/users/555/following"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tt.path {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				want := url.Values{
					"max_results":      {"200"},
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

func TestUserFollowersRequiresUserID(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, map[string]string{EnvAccessToken: "token"}, "user", "followers")
	if code == 0 || !strings.Contains(stderr, "user id is required") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
