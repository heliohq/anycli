package figma

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFileGetBuildsDocumentQuery(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"name":"Design System","document":{}}`, nil, &got)
	defer server.Close()

	code, _, stderr := runService(t, server,
		"files", "get",
		"--file-key", "abc/branch",
		"--version", "v1",
		"--ids", "1:2,3:4",
		"--depth", "2",
		"--geometry", "paths",
		"--plugin-data", "shared",
		"--branch-data",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/files/abc/branch" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if !strings.HasPrefix(got.RequestURI, "/v1/files/abc%2Fbranch?") {
		t.Errorf("RequestURI = %q, want escaped file-key path segment", got.RequestURI)
	}
	query, err := url.ParseQuery(got.Query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	want := map[string]string{
		"version": "v1", "ids": "1:2,3:4", "depth": "2",
		"geometry": "paths", "plugin_data": "shared", "branch_data": "true",
	}
	for key, value := range want {
		if query.Get(key) != value {
			t.Errorf("query[%s] = %q, want %q", key, query.Get(key), value)
		}
	}
}

func TestFileMetaAndNodesPaths(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantPath string
		wantIDs  string
	}{
		{name: "metadata", args: []string{"files", "meta", "--file-key", "abc"}, wantPath: "/v1/files/abc/meta"},
		{name: "nodes", args: []string{"files", "nodes", "--file-key", "abc", "--ids", "1:2,3:4"}, wantPath: "/v1/files/abc/nodes", wantIDs: "1:2,3:4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
			defer server.Close()
			code, _, stderr := runService(t, server, tc.args...)
			if code != 0 || stderr != "" {
				t.Fatalf("code = %d, stderr = %q", code, stderr)
			}
			if got.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tc.wantPath)
			}
			query, _ := url.ParseQuery(got.Query)
			if query.Get("ids") != tc.wantIDs {
				t.Errorf("ids = %q, want %q", query.Get("ids"), tc.wantIDs)
			}
		})
	}
}

func TestFileCommandsRejectInvalidInputBeforeRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative depth", args: []string{"files", "get", "--file-key", "abc", "--depth", "-1"}, want: "--depth must be positive"},
		{name: "invalid geometry", args: []string{"files", "get", "--file-key", "abc", "--geometry", "vectors"}, want: "--geometry must be paths"},
		{name: "missing node ids", args: []string{"files", "nodes", "--file-key", "abc"}, want: "required flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
			defer server.Close()
			code, _, stderr := runService(t, server, tc.args...)
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("code = %d, stderr = %q, want %q", code, stderr, tc.want)
			}
			if got.Path != "" {
				t.Errorf("request unexpectedly sent to %s", got.Path)
			}
		})
	}
}

func TestImageRender(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"images":{"1:2":"https://example.com/image.png"}}`, nil, &got)
	defer server.Close()

	code, stdout, stderr := runService(t, server,
		"images", "render", "--file-key", "abc", "--ids", "1:2", "--format", "png", "--scale", "2",
		"--svg-outline-text=false", "--svg-include-id", "--svg-include-node-id",
		"--svg-simplify-stroke", "--contents-only=false", "--use-absolute-bounds",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v1/images/abc" {
		t.Errorf("path = %q", got.Path)
	}
	query, _ := url.ParseQuery(got.Query)
	if query.Get("ids") != "1:2" || query.Get("format") != "png" || query.Get("scale") != "2" {
		t.Errorf("query = %v", query)
	}
	booleanOptions := map[string]string{
		"svg_outline_text": "false", "svg_include_id": "true", "svg_include_node_id": "true",
		"svg_simplify_stroke": "true", "contents_only": "false", "use_absolute_bounds": "true",
	}
	for key, value := range booleanOptions {
		if query.Get(key) != value {
			t.Errorf("query[%s] = %q, want %q", key, query.Get(key), value)
		}
	}
	if !strings.Contains(stdout, `"images"`) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestImageRenderRejectsInvalidOptions(t *testing.T) {
	cases := [][]string{
		{"images", "render", "--file-key", "abc", "--ids", "1:2", "--scale", "5"},
		{"images", "render", "--file-key", "abc", "--ids", "1:2", "--format", "gif"},
	}
	for _, args := range cases {
		var got capturedRequest
		server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
		code, _, _ := runService(t, server, args...)
		server.Close()
		if code != 1 {
			t.Errorf("args %v: code = %d, want 1", args, code)
		}
		if got.Path != "" {
			t.Errorf("args %v sent request to %s", args, got.Path)
		}
	}
}
