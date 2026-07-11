package x

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTimelineCommandsUseOneExplicitPage(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		path string
	}{
		{name: "user", cmd: "user", path: "/2/users/77/tweets"},
		{name: "mentions", cmd: "mentions", path: "/2/users/77/mentions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, tt.path)
				}
				want := url.Values{
					"max_results":      {"25"},
					"pagination_token": {"page-2"},
					"tweet.fields":     {defaultPostFields},
				}
				if r.URL.Query().Encode() != want.Encode() {
					t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
				}
				jsonResponse(w, http.StatusOK, `{"data":[],"meta":{"next_token":"page-3"}}`)
			})
			defer server.Close()

			code, _, stderr := run(t, server, fullEnv(), "timeline", tt.cmd, "--user-id", "77", "--limit", "25", "--next-token", "page-2")
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

func TestHomeTimelineIsFixedToConnectedUserAndAllowsLimitOne(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2/users/2244994945/timelines/reverse_chronological" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		want := url.Values{
			"max_results":      {"1"},
			"pagination_token": {"page-2"},
			"tweet.fields":     {defaultPostFields},
		}
		if r.URL.Query().Encode() != want.Encode() {
			t.Fatalf("query = %q, want %q", r.URL.Query().Encode(), want.Encode())
		}
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "timeline", "home", "--limit", "1", "--next-token", "page-2")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestHomeTimelineRejectsUserOverride(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "timeline", "home", "--user-id", "77")
	if code == 0 || !strings.Contains(stderr, "unknown flag: --user-id") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestUserTimelineKeepsFivePostMinimum(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "timeline", "user", "--limit", "1")
	if code == 0 || !strings.Contains(stderr, "limit must be between 5 and 100") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestTimelineDefaultsToConnectedUserID(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2/users/2244994945/tweets" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "timeline", "user")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestTimelineRequiresUserIDWhenCredentialMissing(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, map[string]string{EnvAccessToken: "token"}, "timeline", "home")
	if code == 0 || !strings.Contains(stderr, "user id is required") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
