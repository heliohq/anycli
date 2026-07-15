package figma

import (
	"net/http"
	"net/url"
	"testing"
)

func TestProjectDiscoveryCommands(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantPath   string
		wantBranch string
	}{
		{name: "team projects", args: []string{"teams", "projects", "--team-id", "100"}, wantPath: "/v1/teams/100/projects"},
		{name: "project metadata", args: []string{"projects", "meta", "--project-id", "200"}, wantPath: "/v1/projects/200/meta"},
		{name: "project files", args: []string{"projects", "files", "--project-id", "200", "--branch-data"}, wantPath: "/v1/projects/200/files", wantBranch: "true"},
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
			if query.Get("branch_data") != tc.wantBranch {
				t.Errorf("branch_data = %q, want %q", query.Get("branch_data"), tc.wantBranch)
			}
		})
	}
}
