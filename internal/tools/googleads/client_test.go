package googleads

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// googleErrorBody is a realistic Google Ads REST error envelope: a top-level
// error with a GoogleAdsFailure in details[].errors[] carrying the actionable
// errorCode + message.
const googleErrorBody = `{
  "error": {
    "code": 400,
    "message": "Request contains an invalid argument.",
    "status": "INVALID_ARGUMENT",
    "details": [
      {
        "@type": "type.googleapis.com/google.ads.googleads.v24.errors.GoogleAdsFailure",
        "errors": [
          {"errorCode": {"queryError": "UNRECOGNIZED_FIELD"}, "message": "Error in query: unrecognized field."}
        ]
      }
    ]
  }
}`

func TestAPIError_SurfacesNestedGoogleAdsFailure(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, googleErrorBody, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "query", "--customer-id", "1234567890", "--gaql", "SELECT bad FROM campaign")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	for _, want := range []string{"INVALID_ARGUMENT", "queryError=UNRECOGNIZED_FIELD", "unrecognized field"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", stderr, want)
		}
	}
}

func TestAPIError_JSONEnvelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, googleErrorBody, &got)
	defer srv.Close()

	code, _, stderr := run(t, srv, "query", "--customer-id", "1234567890",
		"--gaql", "SELECT bad FROM campaign", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
			Status  int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr)
	}
	if env.Error.Kind != "api" {
		t.Errorf("kind = %q, want api", env.Error.Kind)
	}
	if env.Error.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", env.Error.Status)
	}
	if !strings.Contains(env.Error.Message, "queryError=UNRECOGNIZED_FIELD") {
		t.Errorf("message = %q, want the nested error code", env.Error.Message)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantRejected: true},
		{name: "forbidden", status: http.StatusForbidden, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, wantRejected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			srv := newServer(t, tc.status, `{"error":{"message":"nope","status":"UNAUTHENTICATED"}}`, &got)
			defer srv.Close()

			result, _, _ := runResult(t, srv, nil, "accounts", "list")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("exit code = %d, want 1", result.ExitCode)
			}
		})
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "campaign", "delete", "--id", "1")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for unknown subcommand", code)
	}
	if got.Method != "" {
		t.Errorf("a request was sent for an unknown subcommand: %s %s", got.Method, got.Path)
	}
}
