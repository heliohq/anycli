package semrush

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNothingFound_IsEmptySuccess(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "ERROR 50 :: NOTHING FOUND", &got)
	defer srv.Close()

	result, stdout, stderr := runResult(t, srv, "domain", "organic", "example.com")
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (nothing-found is a valid empty answer)", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("nothing-found must not reject the credential")
	}
	if stderr != "" {
		t.Errorf("nothing-found should not write to stderr, got %q", stderr)
	}
	env := decodeEnvelope(t, stdout)
	if env.RowCount != 0 || len(env.Rows) != 0 {
		t.Errorf("expected empty rows, got %+v", env)
	}
	if env.Note == "" {
		t.Error("expected a note explaining the empty result")
	}
}

func TestRejectedKey_ClassifiesCredentialRejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"wrong key", "ERROR 120 :: WRONG KEY - ID PAIR"},
		{"empty key", "ERROR 122 :: WRONG FORMAT OR EMPTY KEY"},
		{"api disabled", "ERROR 130 :: API DISABLED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, http.StatusOK, tc.body, &got)
			defer srv.Close()

			result, _, stderr := runResult(t, srv, "domain", "organic", "example.com")
			if result.ExitCode != 1 {
				t.Fatalf("exit = %d, want 1", result.ExitCode)
			}
			if !result.CredentialRejected {
				t.Errorf("%q should mark the credential rejected", tc.body)
			}
			if stderr == "" {
				t.Error("expected an error on stderr")
			}
		})
	}
}

func TestLimitError_IsPlainAPIFailureNotRejection(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "ERROR 132 :: API UNITS BALANCE IS ZERO", &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "keyword", "overview", "seo")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("a units/limit error must NOT reject the credential")
	}
}

func TestErrorBody_WithNon200Status(t *testing.T) {
	// Some ERROR responses arrive with a non-2xx status; the ERROR body still
	// drives classification, not the HTTP status.
	var got capturedRequest
	srv := newServer(t, http.StatusForbidden, "ERROR 120 :: WRONG KEY - ID PAIR", &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "domain", "organic", "example.com")
	if result.ExitCode != 1 || !result.CredentialRejected {
		t.Fatalf("result = %+v, want exit 1 + credential rejected", result)
	}
}

func TestNonERRORNon2xx_IsGenericAPIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusInternalServerError, "<html>gateway timeout</html>", &got)
	defer srv.Close()

	result, _, stderr := runResult(t, srv, "domain", "organic", "example.com")
	if result.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Error("an infrastructure 500 must not reject the credential")
	}
	if !strings.Contains(stderr, "500") {
		t.Errorf("expected HTTP status in error, got %q", stderr)
	}
}

func TestErrorEnvelope_JSONShape(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "ERROR 132 :: API UNITS BALANCE IS ZERO", &got)
	defer srv.Close()

	_, _, stderr := run(t, srv, "keyword", "overview", "seo", "--json")
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%s)", err, stderr)
	}
	if envelope.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", envelope.Error.Kind)
	}
	if envelope.Error.Code != 132 {
		t.Errorf("code = %d, want 132", envelope.Error.Code)
	}
}

func TestParseSemrushError_ToleratesSpacing(t *testing.T) {
	cases := []struct {
		body     string
		wantCode int
		wantOK   bool
	}{
		{"ERROR 50 :: NOTHING FOUND", 50, true},
		{"ERROR 120 :: WRONG KEY - ID PAIR", 120, true},
		{"Keyword;Position\nfoo;1", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		code, _, ok := parseSemrushError(tc.body)
		if ok != tc.wantOK || code != tc.wantCode {
			t.Errorf("parseSemrushError(%q) = (%d,%v), want (%d,%v)", tc.body, code, ok, tc.wantCode, tc.wantOK)
		}
	}
}

func TestMissingKey_Exit1(t *testing.T) {
	svc := &Service{}
	result, err := svc.Execute(context.Background(), []string{"units"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit = %d, want 1 for missing key", result.ExitCode)
	}
}
