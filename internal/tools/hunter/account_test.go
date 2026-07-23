package hunter

import (
	"net/http"
	"strings"
	"testing"
)

func TestAccount_Passthrough(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK,
		`{"data":{"email":"me@acme.com","plan_name":"Free","reset_date":"2026-08-01","requests":{"searches":{"used":1,"available":25}}}}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "account")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/account" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	if got.Query != "" {
		t.Errorf("account takes no query params; got %q", got.Query)
	}
	if !strings.Contains(stdout, `"reset_date"`) || !strings.Contains(stdout, `"plan_name"`) {
		t.Errorf("stdout = %q, want verbatim account body", stdout)
	}
}
