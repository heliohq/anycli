package snov

import (
	"net/http"
	"strings"
	"testing"
)

func TestEnrichByEmail_V1Form(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/get-profile-by-email", cannedResponse{http.StatusOK, `{"success":true,"name":"Jane Doe","industry":"Software"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runSnov(t, srv, "enrich", "by-email", "--email", "jane@example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	req := fake.first(t, "/v1/get-profile-by-email")
	if req.Method != http.MethodPost {
		t.Errorf("enrich method = %s, want POST", req.Method)
	}
	form := parseForm(t, req.Body)
	if form.Get("email") != "jane@example.com" {
		t.Errorf("email = %q, want jane@example.com", form.Get("email"))
	}
	if form.Get("access_token") != "tok-abc" {
		t.Errorf("enrich access_token = %q, want tok-abc", form.Get("access_token"))
	}
	if !strings.Contains(stdout, `"name":"Jane Doe"`) {
		t.Errorf("stdout missing enrichment payload: %q", stdout)
	}
}

func TestEnrichByEmail_RequiresEmail(t *testing.T) {
	fake := newFake().withToken()
	srv := fake.serve(t)
	defer srv.Close()

	result, _, stderr := runSnov(t, srv, "enrich", "by-email")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (missing required --email)", result.ExitCode)
	}
	if !strings.Contains(strings.ToLower(stderr), "email") {
		t.Errorf("stderr should name the missing flag: %q", stderr)
	}
	// A usage error must not reach the provider.
	if hits := fake.hits("/v1/get-profile-by-email"); len(hits) != 0 {
		t.Errorf("provider called despite missing flag: %d hits", len(hits))
	}
}
