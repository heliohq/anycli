package jotform

import (
	"net/http"
	"strings"
	"testing"
)

const okEnvelope = `{"responseCode":200,"message":"success","content":{"username":"acme"}}`

func TestUser_SendsRawAPIKeyHeader(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "user")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got.Method != http.MethodGet || got.Path != "/user" {
		t.Errorf("request = %s %s, want GET /user", got.Method, got.Path)
	}
	// Jotform's scheme is the raw key in the APIKEY header — no "Bearer" prefix.
	if got.APIKey != testKey {
		t.Errorf("APIKEY header = %q, want %q", got.APIKey, testKey)
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if strings.TrimSpace(stdout) != okEnvelope {
		t.Errorf("stdout = %q, want the envelope verbatim", stdout)
	}
}

func TestUsage_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, okEnvelope, &got)
	defer srv.Close()

	if code, _, se := run(t, srv, "usage"); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, se)
	}
	if got.Path != "/user/usage" {
		t.Errorf("path = %q, want /user/usage", got.Path)
	}
}

func TestFormList_ListParams(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":[]}`, &got)
	defer srv.Close()

	code, _, se := run(t, srv, "form", "list", "--limit", "5", "--offset", "10", "--orderby", "created_at", "--filter", `{"status":"ENABLED"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, se)
	}
	if got.Path != "/user/forms" {
		t.Errorf("path = %q, want /user/forms", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("limit") != "5" || q.Get("offset") != "10" {
		t.Errorf("pagination = %q", got.Query)
	}
	if q.Get("orderby") != "created_at" {
		t.Errorf("orderby = %q", q.Get("orderby"))
	}
	if q.Get("filter") != `{"status":"ENABLED"}` {
		t.Errorf("filter = %q", q.Get("filter"))
	}
}

func TestFormList_OmitsUnsetPagination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":[]}`, &got)
	defer srv.Close()

	if code, _, _ := run(t, srv, "form", "list"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	// Default sentinels must not leak offset=0/limit=0 the user never asked for.
	if got.Query != "" {
		t.Errorf("query = %q, want empty when no list flags set", got.Query)
	}
}

func TestFormGet_And_Questions_PathEscape(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"get", []string{"form", "get", "2500001"}, "/form/2500001"},
		{"questions", []string{"form", "questions", "2500001"}, "/form/2500001/questions"},
		{"submissions", []string{"form", "submissions", "2500001"}, "/form/2500001/submissions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":{}}`, &got)
			defer srv.Close()
			if code, _, se := run(t, srv, tc.args...); code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, se)
			}
			if got.Path != tc.want {
				t.Errorf("path = %q, want %q", got.Path, tc.want)
			}
		})
	}
}

func TestSubmissionGet_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":{}}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "submission", "get", "sub-9"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/submission/sub-9" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestSubmissionList_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":[]}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "submission", "list"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/user/submissions" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestReportList_AccountAndForm(t *testing.T) {
	t.Run("account", func(t *testing.T) {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":[]}`, &got)
		defer srv.Close()
		if code, _, _ := run(t, srv, "report", "list"); code != 0 {
			t.Fatalf("exit code = %d", code)
		}
		if got.Path != "/user/reports" {
			t.Errorf("path = %q, want /user/reports", got.Path)
		}
	})
	t.Run("form-scoped", func(t *testing.T) {
		var got capturedRequest
		srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":[]}`, &got)
		defer srv.Close()
		if code, _, _ := run(t, srv, "report", "list", "--form", "42"); code != 0 {
			t.Fatalf("exit code = %d", code)
		}
		if got.Path != "/form/42/reports" {
			t.Errorf("path = %q, want /form/42/reports", got.Path)
		}
	})
}

func TestFolderList_Path(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"responseCode":200,"content":[]}`, &got)
	defer srv.Close()
	if code, _, _ := run(t, srv, "folder", "list"); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/user/folders" {
		t.Errorf("path = %q", got.Path)
	}
}

func TestMissingKey_Exit1(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"user"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "JOTFORM_API_KEY is not set") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}
