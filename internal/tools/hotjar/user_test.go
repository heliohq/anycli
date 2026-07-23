package hotjar

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUserLookup_PostsReadOnlyBody(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/organizations/99/user-lookup", cannedResponse{http.StatusOK,
			`{"results":[{"site_id":"42"}]}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runHotjar(t, srv, "user", "lookup", "--org", "99", "--email", "jane@example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	req := fake.first(t, "/v1/organizations/99/user-lookup")
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if !strings.HasPrefix(req.ContentType, "application/json") {
		t.Errorf("content-type = %q, want application/json", req.ContentType)
	}
	if req.Auth != "Bearer tok-abc" {
		t.Errorf("auth = %q, want Bearer tok-abc", req.Auth)
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, req.Body)
	}
	if body["data_subject_email"] != "jane@example.com" {
		t.Errorf("data_subject_email = %v, want jane@example.com", body["data_subject_email"])
	}
	if !strings.Contains(stdout, `"site_id":"42"`) {
		t.Errorf("stdout missing payload: %q", stdout)
	}
}

// TestUserLookup_NeverDeletes is the safety regression for Divergence 4:
// deletion is the delete_all_hits:true mode of this same endpoint, so the
// lookup body must ALWAYS carry delete_all_hits:false and there must be no flag
// that flips it.
func TestUserLookup_NeverDeletes(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/organizations/99/user-lookup", cannedResponse{http.StatusOK, `{"results":[]}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, stderr := runHotjar(t, srv, "user", "lookup", "--org", "99", "--email", "jane@example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	var body map[string]any
	if err := json.Unmarshal(fake.first(t, "/v1/organizations/99/user-lookup").Body, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	del, ok := body["delete_all_hits"]
	if !ok {
		t.Fatal("body must explicitly carry delete_all_hits to pin it false")
	}
	if del != false {
		t.Fatalf("delete_all_hits = %v, want false — lookup must never delete", del)
	}
}

func TestUserLookup_MissingEmailIsUsageError(t *testing.T) {
	srv := newFake().withToken().serve(t)
	defer srv.Close()

	result, _, _ := runHotjar(t, srv, "user", "lookup", "--org", "99")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage)", result.ExitCode)
	}
}
