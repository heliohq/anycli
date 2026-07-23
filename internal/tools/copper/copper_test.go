package copper

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestBearerOnlyAuthHeaders asserts the OAuth path sends ONLY Authorization:
// Bearer + Content-Type: application/json, and never the legacy X-PW-* trio.
func TestBearerOnlyAuthHeaders(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":1}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "account", "get")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, stderr)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want %q", got.Auth, "Bearer tok-123")
	}
	if got.ContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.ContentType)
	}
	for _, h := range []string{"X-Pw-Accesstoken", "X-Pw-Application", "X-Pw-Useremail"} {
		if v := got.Header.Get(h); v != "" {
			t.Errorf("unexpected legacy API-key header %s = %q (OAuth path must not send X-PW-*)", h, v)
		}
	}
}

func TestAccountGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":42,"name":"Acme"}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "account", "get")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got.Method != http.MethodGet || got.Path != "/account" {
		t.Errorf("request = %s %s, want GET /account", got.Method, got.Path)
	}
	if strings.TrimSpace(stdout) != `{"id":42,"name":"Acme"}` {
		t.Errorf("stdout = %q, want provider JSON passthrough", stdout)
	}
}

func TestUserMeIsIdentitySource(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":159258,"email":"a@b.com"}`, &got)
	defer srv.Close()

	if code, _, se := run(t, srv, "user", "me"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	if got.Method != http.MethodGet || got.Path != "/users/me" {
		t.Errorf("request = %s %s, want GET /users/me", got.Method, got.Path)
	}
}

// TestListUsesPostSearch verifies Copper's POST /{res}/search list convention
// with the assembled JSON body and pagination.
func TestListUsesPostSearch(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	code, _, se := run(t, srv, "person", "list", "--name", "Jane", "--email", "jane@x.com", "--assignee-id", "7", "--page", "2", "--page-size", "25")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	if got.Method != http.MethodPost || got.Path != "/people/search" {
		t.Errorf("request = %s %s, want POST /people/search", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "Jane" {
		t.Errorf("body.name = %v, want Jane", body["name"])
	}
	if body["page_number"] != float64(2) || body["page_size"] != float64(25) {
		t.Errorf("pagination = %v/%v, want 2/25", body["page_number"], body["page_size"])
	}
	emails, ok := body["emails"].([]any)
	if !ok || len(emails) != 1 || emails[0] != "jane@x.com" {
		t.Errorf("body.emails = %v, want [jane@x.com]", body["emails"])
	}
	assignees, ok := body["assignee_ids"].([]any)
	if !ok || len(assignees) != 1 || assignees[0] != float64(7) {
		t.Errorf("body.assignee_ids = %v, want [7]", body["assignee_ids"])
	}
}

// TestListJSONBodyOverridesTypedFilters verifies --json-body merges over the
// typed search flags.
func TestListJSONBodyOverridesTypedFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `[]`, &got)
	defer srv.Close()

	code, _, se := run(t, srv, "opportunity", "list", "--name", "typed", "--json-body", `{"name":"override","tags":["hot"]}`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
	}
	if got.Path != "/opportunities/search" {
		t.Errorf("path = %s, want /opportunities/search", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["name"] != "override" {
		t.Errorf("body.name = %v, want override (json-body wins)", body["name"])
	}
	if tags, ok := body["tags"].([]any); !ok || len(tags) != 1 || tags[0] != "hot" {
		t.Errorf("body.tags = %v, want [hot]", body["tags"])
	}
}

// TestCRUDVerbMapping exercises get/create/update/delete method+path mapping
// across the uniform resources.
func TestCRUDVerbMapping(t *testing.T) {
	cases := []struct {
		args       []string
		wantMethod string
		wantPath   string
		wantBody   map[string]any
	}{
		{[]string{"company", "get", "--id", "5"}, http.MethodGet, "/companies/5", nil},
		{[]string{"company", "create", "--json-body", `{"name":"Acme"}`}, http.MethodPost, "/companies", map[string]any{"name": "Acme"}},
		{[]string{"lead", "update", "--id", "9", "--json-body", `{"status":"Open"}`}, http.MethodPut, "/leads/9", map[string]any{"status": "Open"}},
		{[]string{"task", "delete", "--id", "3"}, http.MethodDelete, "/tasks/3", nil},
		{[]string{"person", "find-email", "--email", "x@y.com"}, http.MethodPost, "/people/fetch_by_email", map[string]any{"email": "x@y.com"}},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{"ok":true}`, &got)
			defer srv.Close()
			if code, _, se := run(t, srv, tc.args...); code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
			}
			if got.Method != tc.wantMethod || got.Path != tc.wantPath {
				t.Errorf("request = %s %s, want %s %s", got.Method, got.Path, tc.wantMethod, tc.wantPath)
			}
			if tc.wantBody != nil {
				body := decodeBody(t, got.Body)
				for k, v := range tc.wantBody {
					if body[k] != v {
						t.Errorf("body[%q] = %v, want %v", k, body[k], v)
					}
				}
			}
		})
	}
}

func TestActivityHasNoUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	// activity update is not a registered subcommand → usage error (exit 2).
	if code, _, _ := run(t, srv, "activity", "update", "--id", "1"); code != 2 {
		t.Errorf("exit = %d, want 2 (activity has no update)", code)
	}
}

func TestActivityCreateAndDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"id":1}`, &got)
	defer srv.Close()
	if code, _, se := run(t, srv, "activity", "create", "--json-body", `{"type":{"category":"user","id":1}}`); code != 0 {
		t.Fatalf("create exit = %d, want 0 (stderr=%s)", code, se)
	}
	if got.Method != http.MethodPost || got.Path != "/activities" {
		t.Errorf("request = %s %s, want POST /activities", got.Method, got.Path)
	}
}

func TestLookupEndpoints(t *testing.T) {
	cases := map[string]string{
		"pipelines":        "/pipelines",
		"pipeline-stages":  "/pipeline_stages",
		"customer-sources": "/customer_sources",
		"loss-reasons":     "/loss_reasons",
		"activity-types":   "/activity_types",
		"contact-types":    "/contact_types",
	}
	for word, path := range cases {
		t.Run(word, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `[]`, &got)
			defer srv.Close()
			if code, _, se := run(t, srv, "lookup", word); code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr=%s)", code, se)
			}
			if got.Method != http.MethodGet || got.Path != path {
				t.Errorf("request = %s %s, want GET %s", got.Method, got.Path, path)
			}
		})
	}
}

// TestAPIErrorExitOne verifies a Copper non-2xx maps to exit 1 with an api-kind
// JSON error envelope under --json.
func TestAPIErrorExitOne(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnprocessableEntity, `{"status":422,"error":"Name is required"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "company", "create", "--json-body", `{}`, "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("422 should not reject the credential")
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 422 {
		t.Errorf("envelope = %+v, want kind=api status=422", env.Error)
	}
	if !strings.Contains(env.Error.Message, "Name is required") {
		t.Errorf("message = %q, want Copper error surfaced", env.Error.Message)
	}
}

// TestCredentialRejectionOn401 verifies a 401 is classified as a credential
// rejection (exit 1, CredentialRejected true) so the token gateway invalidates.
func TestCredentialRejectionOn401(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"status":401,"error":"Invalid token"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "account", "get")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("result = %+v, want exit 1 with CredentialRejected", result)
	}
}

func TestInvalidJSONBodyIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "person", "create", "--json-body", `{not json`); code != 2 {
		t.Errorf("exit = %d, want 2 (invalid --json-body is a usage error)", code)
	}
	if got.Method != "" {
		t.Errorf("no HTTP request should be made on a parse error; saw %s %s", got.Method, got.Path)
	}
}

func TestMissingIDIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "person", "get"); code != 2 {
		t.Errorf("exit = %d, want 2 (missing --id)", code)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "person", "bogus"); code != 2 {
		t.Errorf("exit = %d, want 2 (unknown subcommand)", code)
	}
}

func TestMissingTokenExitOne(t *testing.T) {
	result, _, stderr := runNoToken(t, "account", "get")
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1 (missing token)", result.ExitCode)
	}
	if !strings.Contains(stderr, "COPPER_ACCESS_TOKEN") {
		t.Errorf("stderr = %q, want COPPER_ACCESS_TOKEN mention", stderr)
	}
}
