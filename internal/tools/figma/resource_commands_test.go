package figma

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestFirstClassResourceCommands(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantQuery  map[string]string
		wantBody   string
	}{
		{name: "file versions", args: []string{"files", "versions", "--file-key", "abc", "--page-size", "20", "--after", "10"}, wantMethod: "GET", wantPath: "/v1/files/abc/versions", wantQuery: map[string]string{"page_size": "20", "after": "10"}},
		{name: "image fills", args: []string{"images", "fills", "--file-key", "abc"}, wantMethod: "GET", wantPath: "/v1/files/abc/images"},
		{name: "reaction add", args: []string{"comments", "reactions", "add", "--file-key", "abc", "--comment-id", "c1", "--emoji", "+1"}, wantMethod: "POST", wantPath: "/v1/files/abc/comments/c1/reactions", wantBody: `{"emoji":"+1"}`},
		{name: "team components", args: []string{"libraries", "components", "team", "--team-id", "100", "--page-size", "30"}, wantMethod: "GET", wantPath: "/v1/teams/100/components", wantQuery: map[string]string{"page_size": "30"}},
		{name: "variables update", args: []string{"variables", "update", "--file-key", "abc", "--body-json", `{"variables":[]}`}, wantMethod: "POST", wantPath: "/v1/files/abc/variables", wantBody: `{"variables":[]}`},
		{name: "dev resource delete", args: []string{"dev-resources", "delete", "--file-key", "abc", "--dev-resource-id", "r1"}, wantMethod: "DELETE", wantPath: "/v1/files/abc/dev_resources/r1"},
		{name: "webhook create", args: []string{"webhooks", "create", "--body-json", `{"event_type":"FILE_UPDATE"}`}, wantMethod: "POST", wantPath: "/v2/webhooks", wantBody: `{"event_type":"FILE_UPDATE"}`},
		{name: "variable usage analytics", args: []string{"analytics", "variable-usages", "--file-key", "abc", "--group-by", "variable"}, wantMethod: "GET", wantPath: "/v1/analytics/libraries/abc/variable/usages", wantQuery: map[string]string{"group_by": "variable"}},
		{name: "oembed", args: []string{"oembed", "get", "--url", "https://www.figma.com/file/abc/Test", "--max-width", "800"}, wantMethod: "GET", wantPath: "/v1/oembed", wantQuery: map[string]string{"url": "https://www.figma.com/file/abc/Test", "maxwidth": "800"}},
		{name: "payments", args: []string{"payments", "list", "--user-id", "42"}, wantMethod: "GET", wantPath: "/v1/payments", wantQuery: map[string]string{"user_id": "42"}},
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
			if got.Method != tc.wantMethod || got.Path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", got.Method, got.Path, tc.wantMethod, tc.wantPath)
			}
			query, err := url.ParseQuery(got.Query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			for key, value := range tc.wantQuery {
				if query.Get(key) != value {
					t.Errorf("query[%s] = %q, want %q", key, query.Get(key), value)
				}
			}
			if tc.wantBody != "" && string(got.Body) != tc.wantBody {
				t.Errorf("body = %s, want %s", got.Body, tc.wantBody)
			}
		})
	}
}

func TestFirstClassCommandsCoverEveryPATOperation(t *testing.T) {
	service := &Service{}
	root := service.newRoot("token")
	covered := map[string]struct{}{}
	collectOperationAnnotations(root, covered)

	catalog, err := loadOperationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range catalog.Operations {
		if !operation.PAT {
			continue
		}
		if _, ok := covered[operation.ID]; !ok {
			t.Errorf("PAT operation %s has no first-class command", operation.ID)
		}
	}
}

func collectOperationAnnotations(command *cobra.Command, covered map[string]struct{}) {
	if operationID := command.Annotations[operationIDAnnotation]; operationID != "" {
		covered[operationID] = struct{}{}
	}
	for _, child := range command.Commands() {
		collectOperationAnnotations(child, covered)
	}
}

func TestResourceCommandsRejectInvalidBodiesBeforeRequest(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
	defer server.Close()
	code, _, stderr := runService(t, server,
		"variables", "update", "--file-key", "abc", "--body-json", `{`,
	)
	if code != 1 || !strings.Contains(stderr, "must be valid JSON") {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got.Path != "" {
		t.Errorf("request unexpectedly sent to %s", got.Path)
	}
}

func TestRequiredQueryParametersFailBeforeRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "named reaction delete", args: []string{"comments", "reactions", "delete", "--file-key", "abc", "--comment-id", "c1"}, want: `required flag(s) "emoji"`},
		{name: "named oembed", args: []string{"oembed", "get"}, want: `required flag(s) "url"`},
		{name: "named analytics", args: []string{"analytics", "component-actions", "--file-key", "abc"}, want: `required flag(s) "group-by"`},
		{name: "catalog call", args: []string{"api", "call", "getOEmbed"}, want: "missing required query parameter url"},
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
