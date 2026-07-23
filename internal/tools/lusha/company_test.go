package lusha

import (
	"net/http"
	"testing"
)

func TestCompanyEnrichNoRevealKey(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"id":"16303253"}],"billing":{"creditsCharged":1,"resultsReturned":1}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "enrich", "--domain", "lusha.com")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/companies/search-and-enrich" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	companies := body["companies"].([]any)
	if companies[0].(map[string]any)["domain"] != "lusha.com" {
		t.Errorf("companies = %v", companies)
	}
	// company enrich must NOT emit a reveal key (schema has no reveal field).
	if _, ok := body["reveal"]; ok {
		t.Errorf("company enrich must not send a reveal key: %v", body)
	}
}

func TestCompanyEnrichHasNoRevealFlag(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// --reveal is not a registered flag on company enrich; cobra rejects it
	// as a usage error (exit 2) without hitting the API.
	result, _, _ := runResult(t, srv, "company", "enrich", "--domain", "lusha.com", "--reveal", "emails")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 (unknown flag)", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("should not reach API")
	}
}

func TestCompanyRevealFirmographicExpansion(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"id":"1"}],"billing":{"creditsCharged":2,"resultsReturned":1}}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "company", "reveal", "--id", "16303253", "--reveal", "competitors,intent")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/companies/enrich" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	reveal := body["reveal"].([]any)
	if len(reveal) != 2 || reveal[0] != "competitors" || reveal[1] != "intent" {
		t.Errorf("reveal = %v", reveal)
	}
}

func TestCompanyRevealRejectsContactRevealValue(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// emails/phones are contact-only; company reveal must reject them.
	result, _, _ := runResult(t, srv, "company", "reveal", "--id", "1", "--reveal", "emails")
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("should not reach API on invalid reveal")
	}
}

func TestCompanySearchBody(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"results":[{"id":"1"}],"pagination":{"page":1,"size":50,"total":60}}`, &got)
	defer srv.Close()

	code, out, _ := run(t, srv, "company", "search", "--filters", `{"companies":{"include":{"sizes":[{"min":50}]}}}`, "--page", "1", "--size", "50")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/companies/prospecting" {
		t.Errorf("path = %q", got.Path)
	}
	pg := decodeBody(t, got.Body)["pagination"].(map[string]any)
	if pg["page"].(float64) != 1 || pg["size"].(float64) != 50 {
		t.Errorf("pagination = %v", pg)
	}
	// page 1, size 50, total 60 → (1+1)*50=100 >= 60 → has_more false.
	meta := decodeStdout(t, out)["meta"].(map[string]any)
	if meta["has_more"] != false {
		t.Errorf("has_more = %v, want false", meta["has_more"])
	}
}

func TestRevealTooManyIDs(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	args := []string{"contact", "reveal"}
	for i := 0; i < 101; i++ {
		args = append(args, "--id", "x")
	}
	result, _, _ := runResult(t, srv, args...)
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2 for >100 ids", result.ExitCode)
	}
	if got.Method != "" {
		t.Errorf("should not reach API")
	}
}
