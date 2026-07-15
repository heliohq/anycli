package figma

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIGetSupportsVersionedPathsAndRepeatedQueries(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"ok":true}`, nil, &got)
	defer server.Close()

	code, stdout, stderr := runService(t, server,
		"api", "--method", "GET", "--path", "/v1/files/abc/nodes",
		"--query", "ids=1:2,3:4", "--query", "plugin_data=shared",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/files/abc/nodes" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	query, err := url.ParseQuery(got.Query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if query.Get("ids") != "1:2,3:4" || query.Get("plugin_data") != "shared" {
		t.Errorf("query = %v", query)
	}
	if stdout != "{\"ok\":true}\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestAPICatalogListAndDescribeAreJSON(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "list", args: []string{"api", "list"}, want: []string{`"source":"figma/rest-api-spec@`, `"id":"getFile"`, `"pat":false`}},
		{name: "describe", args: []string{"api", "describe", "postVariables"}, want: []string{`"method":"POST"`, `"file_variables:write"`, `"body_required":true`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
			defer server.Close()
			code, stdout, stderr := runService(t, server, tc.args...)
			if code != 0 || stderr != "" {
				t.Fatalf("code = %d, stderr = %q", code, stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout = %q, want %q", stdout, want)
				}
			}
			if got.Path != "" {
				t.Errorf("catalog command sent request to %s", got.Path)
			}
		})
	}
}

func TestAPICallUsesCataloguedOperation(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"nodes":{}}`, nil, &got)
	defer server.Close()

	code, _, stderr := runService(t, server,
		"api", "call", "getFileNodes",
		"--param", "file_key=abc/branch", "--param", "ids=1:2,3:4", "--param", "depth=2",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/files/abc/branch/nodes" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if !strings.HasPrefix(got.RequestURI, "/v1/files/abc%2Fbranch/nodes?") {
		t.Errorf("RequestURI = %q", got.RequestURI)
	}
	query, _ := url.ParseQuery(got.Query)
	if query.Get("ids") != "1:2,3:4" || query.Get("depth") != "2" {
		t.Errorf("query = %v", query)
	}
}

func TestAPICallValidatesCatalogContractBeforeRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown operation", args: []string{"api", "call", "notReal"}, want: `unknown Figma operation "notReal"`},
		{name: "non PAT operation", args: []string{"api", "call", "getActivityLogs"}, want: "does not accept a personal access token"},
		{name: "required body", args: []string{"api", "call", "postVariables", "--param", "file_key=abc"}, want: "requires --body-json or --body-file"},
		{name: "unexpected body", args: []string{"api", "call", "getMe", "--body-json", `{}`}, want: "does not accept a request body"},
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

func TestAPIPostSupportsV2AndInlineJSON(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{"id":"webhook-1"}`, nil, &got)
	defer server.Close()

	code, _, stderr := runService(t, server,
		"api", "--method", "POST", "--path", "/v2/webhooks",
		"--body-json", `{"event_type":"FILE_UPDATE","context":"team"}`,
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodPost || got.Path != "/v2/webhooks" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q", got.ContentType)
	}
	if string(got.Body) != `{"event_type":"FILE_UPDATE","context":"team"}` {
		t.Errorf("body = %s", got.Body)
	}
}

func TestAPIBodyFile(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
	defer server.Close()

	bodyPath := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(bodyPath, []byte(`{"name":"Design source"}`), 0o600); err != nil {
		t.Fatalf("write request body: %v", err)
	}
	code, _, stderr := runService(t, server,
		"api", "--method", "PUT", "--path", "/v1/files/abc/dev_resources/resource-1", "--body-file", bodyPath,
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if string(got.Body) != `{"name":"Design source"}` {
		t.Errorf("body = %s", got.Body)
	}
}

func TestAPIRejectsUnsafeOrMalformedInputBeforeRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "arbitrary host", args: []string{"api", "--path", "https://attacker.example/v1/me"}, want: "must start with /v1/ or /v2/"},
		{name: "unsupported version", args: []string{"api", "--path", "/v3/me"}, want: "must start with /v1/ or /v2/"},
		{name: "path traversal", args: []string{"api", "--path", "/v1/../secrets"}, want: "must not contain path traversal"},
		{name: "inline query", args: []string{"api", "--path", "/v1/me?token=bad"}, want: "must not contain a query"},
		{name: "unsupported method", args: []string{"api", "--method", "TRACE", "--path", "/v1/me"}, want: "--method must be one of"},
		{name: "malformed query", args: []string{"api", "--path", "/v1/me", "--query", "missing-value"}, want: "--query must use key=value"},
		{name: "invalid JSON", args: []string{"api", "--method", "POST", "--path", "/v1/me", "--body-json", "{"}, want: "--body-json must be valid JSON"},
		{name: "two bodies", args: []string{"api", "--method", "POST", "--path", "/v1/me", "--body-json", `{}`, "--body-file", "body.json"}, want: "mutually exclusive"},
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
