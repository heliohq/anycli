package hotjar

import (
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingCredentials(t *testing.T) {
	srv := newFake().serve(t)
	defer srv.Close()

	var svc Service
	svc.BaseURL = srv.URL
	// Only one secret set → fail fast without any HTTP call.
	result, err := svc.Execute(t.Context(), []string{"survey", "list", "--site", "1"}, map[string]string{
		EnvClientID: "cid-1",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 || result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 without credential rejection", result)
	}
}

func TestSurveyList_ExchangesTokenThenReadsWithBearer(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/sites/42/surveys", cannedResponse{http.StatusOK,
			`{"results":[{"id":7,"name":"NPS"}],"next_cursor":"c2"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runHotjar(t, srv, "survey", "list", "--site", "42")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	// Token exchange came first with the client_credentials form body.
	tok := fake.first(t, tokenPath)
	if tok.Method != http.MethodPost {
		t.Errorf("token method = %s, want POST", tok.Method)
	}
	if !strings.HasPrefix(tok.ContentType, "application/x-www-form-urlencoded") {
		t.Errorf("token content-type = %q, want form-urlencoded", tok.ContentType)
	}
	form := parseForm(t, tok.Body)
	if form.Get("grant_type") != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", form.Get("grant_type"))
	}
	if form.Get("client_id") != "cid-1" || form.Get("client_secret") != "csecret-1" {
		t.Errorf("token form did not carry both secrets: %v", form)
	}
	// The survey list is a Bearer-authenticated GET.
	list := fake.first(t, "/v1/sites/42/surveys")
	if list.Method != http.MethodGet {
		t.Errorf("survey list method = %s, want GET", list.Method)
	}
	if list.Auth != "Bearer tok-abc" {
		t.Errorf("survey list auth = %q, want Bearer tok-abc", list.Auth)
	}
	if !strings.Contains(stdout, `"next_cursor":"c2"`) {
		t.Errorf("stdout missing passthrough payload: %q", stdout)
	}
}

func TestSurveyList_PaginationFlags(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/sites/42/surveys", cannedResponse{http.StatusOK, `{"results":[],"next_cursor":null}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, stderr := runHotjar(t, srv, "survey", "list", "--site", "42", "--cursor", "abc", "--limit", "50")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	q := parseQuery(t, fake.first(t, "/v1/sites/42/surveys").Query)
	if q.Get("cursor") != "abc" {
		t.Errorf("cursor = %q, want abc", q.Get("cursor"))
	}
	if q.Get("limit") != "50" {
		t.Errorf("limit = %q, want 50", q.Get("limit"))
	}
}

func TestTokenExchange_BadCredentialsRejected(t *testing.T) {
	fake := newFake().on(tokenPath, cannedResponse{http.StatusUnauthorized, `{"error":"invalid_client"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, stderr := runHotjar(t, srv, "survey", "list", "--site", "1")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 with credential rejection", result)
	}
	if !strings.Contains(stderr, "invalid_client") {
		t.Errorf("stderr should carry the provider message: %q", stderr)
	}
	// A rejected token exchange must not attempt the downstream call.
	if hits := fake.hits("/v1/sites/1/surveys"); len(hits) != 0 {
		t.Errorf("survey list should not be called after token rejection, got %d hits", len(hits))
	}
}

func TestTokenExchange_ServerErrorIsNotRejection(t *testing.T) {
	fake := newFake().on(tokenPath, cannedResponse{http.StatusInternalServerError, `{"error":"boom"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, _ := runHotjar(t, srv, "survey", "list", "--site", "1")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("a 5xx token exchange is transient, must not reject the credential")
	}
}

func TestDataCall_ForbiddenRejectsCredential(t *testing.T) {
	// 403 = valid token but the key lacks permission / plan tier — treat as a
	// credential the gateway should surface for reconnect.
	fake := newFake().withToken().
		on("/v1/sites/1/surveys", cannedResponse{http.StatusForbidden, `{"error":"forbidden"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, stderr := runHotjar(t, srv, "survey", "list", "--site", "1")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 with credential rejection", result)
	}
	if !strings.Contains(stderr, "403") {
		t.Errorf("stderr should carry the status: %q", stderr)
	}
}

func TestDataCall_RateLimitedIsNotRejection(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/sites/1/surveys", cannedResponse{http.StatusTooManyRequests, `{"error":"rate limited"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, stderr := runHotjar(t, srv, "survey", "list", "--site", "1")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("429 is transient, must not reject the credential")
	}
	if !strings.Contains(stderr, "429") {
		t.Errorf("stderr should carry the status: %q", stderr)
	}
}
