package snov

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
	result, err := svc.Execute(t.Context(), []string{"account", "balance"}, map[string]string{
		EnvClientID: "cid-1",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 1 || result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 without credential rejection", result)
	}
}

func TestAccountBalance_ExchangesTokenThenReads(t *testing.T) {
	fake := newFake().withToken().
		on("/v1/get-balance", cannedResponse{http.StatusOK, `{"success":true,"balance":"1000.00"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, stdout, stderr := runSnov(t, srv, "account", "balance")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", result.ExitCode, stderr)
	}
	// Token exchange came first with the client_credentials form body.
	tok := fake.first(t, tokenPath)
	if tok.Method != http.MethodPost {
		t.Errorf("token method = %s, want POST", tok.Method)
	}
	form := parseForm(t, tok.Body)
	if form.Get("grant_type") != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", form.Get("grant_type"))
	}
	if form.Get("client_id") != "cid-1" || form.Get("client_secret") != "csecret-1" {
		t.Errorf("token form did not carry both secrets: %v", form)
	}
	// Balance is a v1 GET carrying the access_token as a query parameter.
	bal := fake.first(t, "/v1/get-balance")
	if bal.Method != http.MethodGet {
		t.Errorf("balance method = %s, want GET", bal.Method)
	}
	if got := parseQuery(t, bal.Query).Get("access_token"); got != "tok-abc" {
		t.Errorf("balance access_token = %q, want tok-abc", got)
	}
	if !strings.Contains(stdout, `"balance":"1000.00"`) {
		t.Errorf("stdout missing balance payload: %q", stdout)
	}
}

func TestTokenExchange_BadCredentialsRejected(t *testing.T) {
	fake := newFake().on(tokenPath, cannedResponse{http.StatusUnauthorized, `{"message":"invalid client"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, stderr := runSnov(t, srv, "account", "balance")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 with credential rejection", result)
	}
	if !strings.Contains(stderr, "invalid client") {
		t.Errorf("stderr should carry the provider message: %q", stderr)
	}
	// A rejected token exchange must not attempt the downstream call.
	if hits := fake.hits("/v1/get-balance"); len(hits) != 0 {
		t.Errorf("balance should not be called after token rejection, got %d hits", len(hits))
	}
}

func TestTokenExchange_ServerErrorIsNotRejection(t *testing.T) {
	fake := newFake().on(tokenPath, cannedResponse{http.StatusInternalServerError, `{"message":"boom"}`})
	srv := fake.serve(t)
	defer srv.Close()

	result, _, _ := runSnov(t, srv, "account", "balance")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Errorf("a 5xx token exchange is transient, must not reject the credential")
	}
}
