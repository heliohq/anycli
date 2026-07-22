package wise

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestProfileList pins the path, Bearer header, Accept header, and verbatim
// passthrough of the response body.
func TestProfileList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"id":1000,"type":"PERSONAL"}]`, &got)
	defer srv.Close()

	code, stdout, stderr := run(t, srv, "profile", "list")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got.Method != "GET" || got.Path != "/v1/profiles" {
		t.Errorf("request = %s %s, want GET /v1/profiles", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want %q", got.Auth, "Bearer tok-123")
	}
	if got.Accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Accept)
	}
	if strings.TrimSpace(stdout) != `[{"id":1000,"type":"PERSONAL"}]` {
		t.Errorf("stdout = %q, want passthrough JSON", stdout)
	}
}

// TestBalanceListDefaultTypes pins the path, the required types query defaulting
// to STANDARD,SAVINGS, and profile path-escaping.
func TestBalanceListDefaultTypes(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "balance", "list", "--profile", "42")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got.Path != "/v4/profiles/42/balances" {
		t.Errorf("path = %q, want /v4/profiles/42/balances", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("types") != "STANDARD,SAVINGS" {
		t.Errorf("types = %q, want STANDARD,SAVINGS", q.Get("types"))
	}
}

// TestBalanceListTypesOverride pins the --types override.
func TestBalanceListTypesOverride(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "balance", "list", "--profile", "42", "--types", "STANDARD")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if q := parseQuery(t, got.Query); q.Get("types") != "STANDARD" {
		t.Errorf("types = %q, want STANDARD", q.Get("types"))
	}
}

// TestBalanceListRequiresProfile pins the usage error (exit 2) when --profile is
// omitted, and that no HTTP call is made.
func TestBalanceListRequiresProfile(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "balance", "list")
	if code != 2 {
		t.Fatalf("exit=%d, want 2 (usage)", code)
	}
	if got.Path != "" {
		t.Errorf("unexpected HTTP call to %q on a usage error", got.Path)
	}
	if !strings.Contains(stderr, "--profile is required") {
		t.Errorf("stderr = %q, want a --profile required message", stderr)
	}
}

// TestBalanceGet pins the single-balance path.
func TestBalanceGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":7}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "balance", "get", "7", "--profile", "42")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got.Path != "/v4/profiles/42/balances/7" {
		t.Errorf("path = %q, want /v4/profiles/42/balances/7", got.Path)
	}
}

// TestTransferListFilters pins the transfer list query passthrough.
func TestTransferListFilters(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "transfer", "list",
		"--profile", "42", "--status", "outgoing_payment_sent",
		"--offset", "10", "--limit", "50",
		"--created-date-start", "2026-01-01", "--created-date-end", "2026-02-01")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got.Path != "/v1/transfers" {
		t.Errorf("path = %q, want /v1/transfers", got.Path)
	}
	q := parseQuery(t, got.Query)
	for k, want := range map[string]string{
		"profile":          "42",
		"status":           "outgoing_payment_sent",
		"offset":           "10",
		"limit":            "50",
		"createdDateStart": "2026-01-01",
		"createdDateEnd":   "2026-02-01",
	} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
}

// TestTransferGet pins the single-transfer path.
func TestTransferGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"id":"abc"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "transfer", "get", "abc")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got.Path != "/v1/transfers/abc" {
		t.Errorf("path = %q, want /v1/transfers/abc", got.Path)
	}
}

// TestActivityListCursorPassthrough pins the activities path, size + cursor
// params, and that the response (cursor + activities) is passed through so the
// AI can page.
func TestActivityListCursorPassthrough(t *testing.T) {
	var got capturedRequest
	body := `{"cursor":"NEXT","activities":[{"id":"a1"}]}`
	srv := newServer(t, 200, body, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "activity", "list",
		"--profile", "42", "--size", "50", "--next-cursor", "CUR")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got.Path != "/v1/profiles/42/activities" {
		t.Errorf("path = %q, want /v1/profiles/42/activities", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("size") != "50" {
		t.Errorf("size = %q, want 50", q.Get("size"))
	}
	if q.Get("nextCursor") != "CUR" {
		t.Errorf("nextCursor = %q, want CUR", q.Get("nextCursor"))
	}
	if strings.TrimSpace(stdout) != body {
		t.Errorf("stdout = %q, want the activity envelope verbatim", stdout)
	}
}

// TestActivityListRequiresProfile pins the usage error for the profile-scoped
// activity verb.
func TestActivityListRequiresProfile(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "activity", "list")
	if code != 2 {
		t.Fatalf("exit=%d, want 2 (usage)", code)
	}
	if got.Path != "" {
		t.Errorf("unexpected HTTP call to %q on a usage error", got.Path)
	}
}

// TestRecipientListV2 pins that recipient list hits v2 /accounts with the
// profile + currency filters.
func TestRecipientListV2(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"content":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "recipient", "list", "--profile", "42", "--currency", "EUR")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got.Path != "/v2/accounts" {
		t.Errorf("path = %q, want /v2/accounts", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("profile") != "42" || q.Get("currency") != "EUR" {
		t.Errorf("query = %q, want profile=42&currency=EUR", got.Query)
	}
}

// TestQuoteCreateUnauthenticated pins that quote create POSTs to the
// unauthenticated /v3/quotes endpoint (no profile in path) with the amount in
// the body, and still injects the Bearer token.
func TestQuoteCreateSourceAmount(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"rate":1.1}`, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "quote", "create",
		"--source-currency", "USD", "--target-currency", "EUR", "--source-amount", "5000")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got.Method != "POST" || got.Path != "/v3/quotes" {
		t.Errorf("request = %s %s, want POST /v3/quotes", got.Method, got.Path)
	}
	if got.Auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want the token injected", got.Auth)
	}
	body := decodeBody(t, got.Body)
	if body["sourceCurrency"] != "USD" || body["targetCurrency"] != "EUR" {
		t.Errorf("body currencies = %v", body)
	}
	if body["sourceAmount"] != float64(5000) {
		t.Errorf("sourceAmount = %v, want 5000", body["sourceAmount"])
	}
	if _, hasTarget := body["targetAmount"]; hasTarget {
		t.Errorf("targetAmount must be absent when --source-amount is given: %v", body)
	}
}

// TestQuoteCreateAmountXor pins the exactly-one-amount usage rule.
func TestQuoteCreateAmountXor(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{}`, &got)
	defer srv.Close()

	// Neither amount given.
	if code, _, _ := run(t, srv, "quote", "create",
		"--source-currency", "USD", "--target-currency", "EUR"); code != 2 {
		t.Errorf("no-amount exit=%d, want 2", code)
	}
	// Both amounts given.
	if code, _, _ := run(t, srv, "quote", "create",
		"--source-currency", "USD", "--target-currency", "EUR",
		"--source-amount", "1", "--target-amount", "2"); code != 2 {
		t.Errorf("both-amounts exit=%d, want 2", code)
	}
	if got.Path != "" {
		t.Errorf("unexpected HTTP call on a usage error: %q", got.Path)
	}
}

// TestCurrencyList pins the reference endpoint.
func TestCurrencyList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `[{"code":"USD"}]`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "currency", "list")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if got.Path != "/v1/currencies" {
		t.Errorf("path = %q, want /v1/currencies", got.Path)
	}
}

// TestBaseURLOverride pins that --base-url redirects the request host: the
// override server is hit and the default-BaseURL server is not.
func TestBaseURLOverride(t *testing.T) {
	var override capturedRequest
	overrideSrv := newServer(t, 200, `[]`, &override)
	defer overrideSrv.Close()

	// A Service whose BaseURL points at a server that must NOT be hit; the flag
	// wins over it.
	var unused capturedRequest
	unusedSrv := newServer(t, 500, `boom`, &unused)
	defer unusedSrv.Close()

	code, _, stderr := run(t, unusedSrv, "profile", "list", "--base-url", overrideSrv.URL)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if override.Path != "/v1/profiles" {
		t.Errorf("override server path = %q, want the request routed there", override.Path)
	}
	if unused.Path != "" {
		t.Errorf("default BaseURL server was hit (%q); --base-url must win", unused.Path)
	}
}

// TestUnauthorizedRejectsCredential pins 401 → exit 1, credential rejected, and
// api-kind JSON error rendering.
func TestUnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 401, `{"errors":[{"code":"NOT_AUTHORISED","message":"bad token"}]}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "profile", "list", "--json")
	if result.ExitCode != 1 {
		t.Fatalf("exit=%d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("CredentialRejected = false, want true on 401")
	}
	var env struct {
		Error struct {
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr not a JSON envelope: %v (%s)", err, stderr)
	}
	if env.Error.Kind != "api" || env.Error.Status != 401 {
		t.Errorf("envelope = %+v, want kind=api status=401", env.Error)
	}
	if !strings.Contains(env.Error.Message, "bad token") {
		t.Errorf("message = %q, want the Wise error surfaced", env.Error.Message)
	}
}

// TestForbiddenRejectsCredential pins 403 (PSD2/scope denial) → exit 1 +
// credential rejected.
func TestForbiddenRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 403, `{"message":"forbidden"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "profile", "list")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Errorf("exit=%d rejected=%v, want 1/true on 403", result.ExitCode, result.CredentialRejected)
	}
}

// TestServerErrorPlainRendering pins a 500 → exit 1 (api, not credential) with a
// plain-text stderr message when --json is absent.
func TestServerErrorPlainRendering(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 500, `{"message":"boom"}`, &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "transfer", "get", "abc")
	if result.ExitCode != 1 {
		t.Fatalf("exit=%d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("500 must not reject the credential")
	}
	if strings.HasPrefix(strings.TrimSpace(stderr), "{") {
		t.Errorf("stderr = %q, want plain text (no --json)", stderr)
	}
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the Wise message surfaced", stderr)
	}
}

// TestMissingToken pins the pre-parse missing-token path (exit 1).
func TestMissingToken(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	result, err := svc.Execute(t.Context(), []string{"profile", "list"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit=%d, want 1", result.ExitCode)
	}
	if !strings.Contains(errBuf.String(), "WISE_API_TOKEN is not set") {
		t.Errorf("stderr = %q, want the missing-token message", errBuf.String())
	}
}
