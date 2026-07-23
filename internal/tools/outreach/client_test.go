package outreach

import (
	"net/http"
	"strings"
	"testing"
)

func TestGetFlattensResourceAndHoistsRelationships(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/prospects/123" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"type":"prospect","id":"123",
			"attributes":{"firstName":"Sally","emails":["s@x.com"]},
			"relationships":{"account":{"data":{"type":"account","id":"5"}},
			"owner":{"data":{"type":"user","id":"9"}},
			"tags":{"data":null}}}}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "prospect", "get", "123")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := `{"account_id":"5","emails":["s@x.com"],"firstName":"Sally","id":"123","owner_id":"9","type":"prospect"}` + "\n"
	if stdout != want {
		t.Fatalf("stdout = %q\nwant       %q", stdout, want)
	}
}

func TestListEmitsItemsCursorAndCount(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("count") != "true" {
			t.Fatalf("count param = %q, want true", r.URL.Query().Get("count"))
		}
		jsonResponse(w, http.StatusOK, `{"data":[
			{"type":"prospect","id":"1","attributes":{"firstName":"A"}},
			{"type":"prospect","id":"2","attributes":{"firstName":"B"}}
		],"links":{"next":"https://api.outreach.io/api/v2/prospects?page[size]=2&page[after]=CURSOR2"},
		"meta":{"count":42}}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "prospect", "list", "--count")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := `{"items":[{"firstName":"A","id":"1","type":"prospect"},{"firstName":"B","id":"2","type":"prospect"}],"next_cursor":"CURSOR2","count":42}` + "\n"
	if stdout != want {
		t.Fatalf("stdout = %q\nwant       %q", stdout, want)
	}
}

func TestListWithoutNextOrCountEmitsNullCursorNoCount(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, `{"data":[{"type":"stage","id":"7","attributes":{"name":"New"}}]}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "stage", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := `{"items":[{"id":"7","name":"New","type":"stage"}],"next_cursor":null}` + "\n"
	if stdout != want {
		t.Fatalf("stdout = %q\nwant       %q", stdout, want)
	}
}

func TestListFlagsMapToJSONAPIQuery(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"prospect", "list", "--limit", "25", "--cursor", "abc", "--sort", "-updatedAt", "--fields", "firstName,lastName")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	// Brackets are kept literal in the request URL (encodeQuery), matching the docs.
	for _, want := range []string{
		"page[size]=25",
		"page[after]=abc",
		"sort=-updatedAt",
		"fields[prospect]=firstName%2ClastName",
	} {
		if !strings.Contains(got.RawQuery, want) {
			t.Fatalf("query = %q, missing %q", got.RawQuery, want)
		}
	}
}

func TestProspectListFilters(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = captureRequest(t, r)
		jsonResponse(w, http.StatusOK, `{"data":[]}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(),
		"prospect", "list", "--q", "sally", "--email", "s@x.com", "--account-id", "5", "--owner-id", "9")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{
		"filter[q]=sally",
		"filter[emails]=s%40x.com",
		"filter[account][id]=5",
		"filter[owner][id]=9",
	} {
		if !strings.Contains(got.RawQuery, want) {
			t.Fatalf("query = %q, missing %q", got.RawQuery, want)
		}
	}
}

func TestNegativeLimitIsUsageError(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "prospect", "list", "--limit", "-1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--limit must not be negative") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestGetRejectsNonNumericID(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "prospect", "get", "abc")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "must be a numeric id") {
		t.Fatalf("stderr = %q", stderr)
	}
}
