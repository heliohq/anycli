package surveymonkey

import (
	"net/http"
	"strings"
	"testing"
)

func TestCollectorList(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[{"id":"5","name":"Web Link"}],"total":1}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "collector", "list", "--survey", "42", "--page", "1")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/surveys/42/collectors" {
		t.Fatalf("path = %q, want /v3/surveys/42/collectors", got.Path)
	}
	if !strings.Contains(stdout, `"name":"Web Link"`) {
		t.Fatalf("stdout = %q, want collector JSON", stdout)
	}
}

func TestMe(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"id":"777","username":"alice","email":"a@example.com"}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "me")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/users/me" {
		t.Fatalf("path = %q, want /v3/users/me", got.Path)
	}
	if !strings.Contains(stdout, `"username":"alice"`) {
		t.Fatalf("stdout = %q, want identity JSON", stdout)
	}
}

func TestFetch(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "fetch", "--path", "surveys/42/rollups")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v3/surveys/42/rollups" {
		t.Fatalf("path = %q, want /v3/surveys/42/rollups", got.Path)
	}
}

func TestFetchStripsLeadingSlashAndV3(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{}`)
	})
	defer server.Close()

	// A caller may pass a leading slash or an explicit v3/ prefix; normalize both.
	code, _, _ := run(t, server, fullEnv(), "fetch", "--path", "/v3/users/me")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/v3/users/me" {
		t.Fatalf("path = %q, want /v3/users/me", got.Path)
	}
}

func TestFetchRequiresPath(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, _ := run(t, server, fullEnv(), "fetch")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
}
