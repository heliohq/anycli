package figma

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestContextMetadataAcceptsFigmaURLAndEmitsSparseTree(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"nodes":{"1:2":{"document":{"id":"1:2","name":"Hero","type":"FRAME","children":[{"id":"2:3","name":"Title","type":"TEXT","characters":"Hello"}]}}}}`, nil, &got)
	defer server.Close()

	code, stdout, stderr := runService(t, server,
		"context", "metadata", "--url", "https://www.figma.com/design/abc/Product?node-id=1-2", "--depth", "3", "--max-nodes", "20",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "/v1/files/abc/nodes" {
		t.Errorf("path = %q", got.Path)
	}
	query, _ := url.ParseQuery(got.Query)
	if query.Get("ids") != "1:2" || query.Get("depth") != "3" {
		t.Errorf("query = %v", query)
	}
	if !strings.Contains(stdout, `"name":"Hero"`) || !strings.Contains(stdout, `"node_count":2`) {
		t.Errorf("stdout = %s", stdout)
	}
}

func TestContextDesignCombinesIndependentFigmaReads(t *testing.T) {
	var mu sync.Mutex
	paths := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/files/abc/nodes":
			_, _ = io.WriteString(w, `{"nodes":{"1:2":{"document":{"id":"1:2","type":"FRAME"}}}}`)
		case "/v1/images/abc":
			_, _ = io.WriteString(w, `{"images":{"1:2":"https://cdn.example/render.png"}}`)
		case "/v1/files/abc/variables/local":
			_, _ = io.WriteString(w, `{"meta":{"variables":{"v1":{"name":"color/brand"}}}}`)
		default:
			http.Error(w, `{"err":"unexpected path"}`, http.StatusNotFound)
		}
	}))
	defer server.Close()

	code, stdout, stderr := runService(t, server,
		"context", "design", "--file-key", "abc", "--ids", "1:2", "--include-variables", "--depth", "4",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	var output map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	for _, key := range []string{"source", "nodes", "renders", "variables"} {
		if len(output[key]) == 0 {
			t.Errorf("output missing %q: %s", key, stdout)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{"/v1/files/abc/nodes", "/v1/images/abc", "/v1/files/abc/variables/local"} {
		if paths[path] != 1 {
			t.Errorf("request count for %s = %d", path, paths[path])
		}
	}
}

func TestContextCommandsRequireNodeIDsWhereNeeded(t *testing.T) {
	cases := []string{"design", "figjam", "screenshot"}
	for _, command := range cases {
		t.Run(command, func(t *testing.T) {
			var got capturedRequest
			server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
			defer server.Close()
			code, _, stderr := runService(t, server, "context", command, "--file-key", "abc")
			if code != 1 || !strings.Contains(stderr, "requires a node-id in --url or --ids") {
				t.Fatalf("code = %d, stderr = %q", code, stderr)
			}
			if got.Path != "" {
				t.Errorf("request unexpectedly sent to %s", got.Path)
			}
		})
	}
}

func TestContextMetadataRejectsInvalidLimitBeforeRequest(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
	defer server.Close()
	code, _, stderr := runService(t, server,
		"context", "metadata", "--file-key", "abc", "--max-nodes", "0",
	)
	if code != 1 || !strings.Contains(stderr, "--max-nodes must be positive") {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "" {
		t.Errorf("request unexpectedly sent to %s", got.Path)
	}
}

func TestContextCommandsRejectUnsupportedFileTypeCombinations(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "design with FigJam URL", args: []string{"context", "design", "--url", "https://figma.com/board/abc/Jam?node-id=1-2"}, want: "does not accept a FigJam"},
		{name: "FigJam with Design URL", args: []string{"context", "figjam", "--url", "https://figma.com/design/abc/App?node-id=1-2"}, want: "requires a FigJam board URL"},
		{name: "FigJam variables", args: []string{"context", "figjam", "--url", "https://figma.com/board/abc/Jam?node-id=1-2", "--include-variables"}, want: "only for Figma Design"},
		{name: "variables with FigJam URL", args: []string{"context", "variables", "--url", "https://figma.com/board/abc/Jam"}, want: "only for Figma Design"},
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
