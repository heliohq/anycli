package googleads

import (
	"net/http"
	"strings"
	"testing"
)

func TestExecute_MissingAccessToken(t *testing.T) {
	svc := &Service{}
	var out, errBuf strings.Builder
	svc.Out = &out
	svc.Err = &errBuf
	result, err := svc.Execute(t.Context(), []string{"accounts", "list"}, map[string]string{EnvDeveloperToken: "dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvAccessToken+" is not set") {
		t.Errorf("stderr = %q, want the missing access-token message", errBuf.String())
	}
}

func TestExecute_MissingDeveloperToken(t *testing.T) {
	svc := &Service{}
	var out, errBuf strings.Builder
	svc.Out = &out
	svc.Err = &errBuf
	result, err := svc.Execute(t.Context(), []string{"accounts", "list"}, map[string]string{EnvAccessToken: "tok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), EnvDeveloperToken+" is not set") {
		t.Errorf("stderr = %q, want the missing developer-token message", errBuf.String())
	}
}

func TestAccountsList_HeadersAndPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"resourceNames":["customers/1234567890","customers/9999"]}`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "accounts", "list")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/customers:listAccessibleCustomers" {
		t.Errorf("request = %s %s, want GET /customers:listAccessibleCustomers", got.Method, got.Path)
	}
	if got.Auth != "Bearer user-tok" {
		t.Errorf("Authorization = %q, want Bearer user-tok", got.Auth)
	}
	if got.DeveloperToken != "dev-tok" {
		t.Errorf("developer-token = %q, want dev-tok", got.DeveloperToken)
	}
	// listAccessibleCustomers ignores login-customer-id; it must not be sent
	// even when the env sets one.
	if got.LoginCustomerID != "" {
		t.Errorf("login-customer-id = %q, want empty on accounts list", got.LoginCustomerID)
	}
	if !strings.Contains(stdout, `"resourceNames"`) {
		t.Errorf("stdout = %q, want provider passthrough", stdout)
	}
}

func TestAccountsList_OmitsLoginCustomerIDEvenWhenSet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"resourceNames":[]}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, map[string]string{EnvLoginCustomerID: "111"}, "accounts", "list")
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if got.LoginCustomerID != "" {
		t.Errorf("login-customer-id = %q, want empty (endpoint ignores it)", got.LoginCustomerID)
	}
}
