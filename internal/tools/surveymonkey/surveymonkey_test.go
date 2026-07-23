package surveymonkey

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecuteRequiresAccessToken(t *testing.T) {
	var stderr bytes.Buffer
	svc := &Service{Err: &stderr}
	result, err := svc.Execute(context.Background(), []string{"me"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(stderr.String(), "SURVEYMONKEY_ACCESS_TOKEN is not set") {
		t.Fatalf("stderr = %q, want missing-token message", stderr.String())
	}
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	var stderr bytes.Buffer
	svc := &Service{Err: &stderr}
	result, err := svc.Execute(context.Background(), []string{"me", "--json"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("stderr is not a JSON error envelope: %v (%q)", err, stderr.String())
	}
	if env.Error.Kind != "usage" {
		t.Fatalf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "not-a-command")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q, want unknown-command error", stderr)
	}
}

func TestBareGroupShowsHelpExitZero(t *testing.T) {
	server := newTestServer(t, nil)
	defer server.Close()

	code, stdout, _ := run(t, server, fullEnv(), "survey")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Manage surveys") {
		t.Fatalf("stdout = %q, want group help", stdout)
	}
}

func TestCredentialRejectionClassification(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantRejected bool
	}{
		{name: "HTTP 401", status: http.StatusUnauthorized, body: `{"error":{"id":"1011","message":"The authorization token provided was invalid."}}`, wantRejected: true},
		{name: "expired token", status: http.StatusUnauthorized, body: `{"error":{"id":"1012","message":"expired"}}`, wantRejected: true},
		{name: "revoked token", status: http.StatusUnauthorized, body: `{"error":{"id":"1013","message":"revoked"}}`, wantRejected: true},
		{name: "permission not granted", status: http.StatusForbidden, body: `{"error":{"id":"1014","message":"Permission has not been granted."}}`, wantRejected: false},
		{name: "plan gate", status: http.StatusForbidden, body: `{"error":{"id":"1015","message":"required plan"}}`, wantRejected: false},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"error":{"id":"1040","message":"Too many requests"}}`, wantRejected: false},
		{name: "server failure", status: http.StatusInternalServerError, body: `{"error":{"id":"1050","message":"Internal Server Error"}}`, wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				jsonResponse(w, tc.status, tc.body)
			})
			defer server.Close()

			result, _, _ := runResult(t, server, fullEnv(), "me")
			if result.CredentialRejected != tc.wantRejected {
				t.Errorf("CredentialRejected = %t, want %t", result.CredentialRejected, tc.wantRejected)
			}
			if result.ExitCode != 1 {
				t.Errorf("ExitCode = %d, want 1 (API error)", result.ExitCode)
			}
		})
	}
}

// TestAnswerEndpointPaidGateMessage asserts that 1014 on the answer endpoints
// (response bulk, response get) surfaces the paid-plan-aware message, while 1014
// on a non-answer endpoint (collector list) surfaces the generic scope message.
func TestAnswerEndpointPaidGateMessage(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantPaid bool
	}{
		{name: "response bulk 1014", args: []string{"response", "bulk", "--survey", "42"}, wantPaid: true},
		{name: "response get 1014", args: []string{"response", "get", "--survey", "42", "--id", "7"}, wantPaid: true},
		{name: "collector list 1014", args: []string{"collector", "list", "--survey", "42"}, wantPaid: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				jsonResponse(w, http.StatusForbidden, `{"error":{"id":"1014","message":"Permission has not been granted by the user to make this request."}}`)
			})
			defer server.Close()

			code, _, stderr := run(t, server, fullEnv(), tc.args...)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			mentionsPaid := strings.Contains(stderr, "paid SurveyMonkey plan")
			if mentionsPaid != tc.wantPaid {
				t.Fatalf("stderr = %q; mentionsPaid=%t want %t", stderr, mentionsPaid, tc.wantPaid)
			}
		})
	}
}

func TestPlanGate1015Message(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusForbidden, `{"error":{"id":"1015","message":"The user does not have the required plan to make this request."}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "response", "bulk", "--survey", "42")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "paid") {
		t.Fatalf("stderr = %q, want plan-gate message", stderr)
	}
}

func TestRegionErrorMessage(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusForbidden, `{"error":{"id":"1018","message":"The user does not have permission to access the host in this region."}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "me")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "region") {
		t.Fatalf("stderr = %q, want region cap message", stderr)
	}
}

func TestJSONErrorEnvelopeCarriesStatus(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusForbidden, `{"error":{"id":"1014","message":"nope"}}`)
	})
	defer server.Close()

	_, _, stderr := run(t, server, fullEnv(), "me", "--json")
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
	if env.Error.Kind != "api" || env.Error.Status != http.StatusForbidden {
		t.Fatalf("envelope = %+v, want kind=api status=403", env.Error)
	}
}
