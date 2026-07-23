package expensify

import (
	"strings"
	"testing"
)

func TestRequestInjectsCredentialsIntoRawJob(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, "myFile.csv", &got)
	defer srv.Close()

	input := `{"type":"file","inputSettings":{"type":"combinedReportData","filters":{"reportIDList":"R1,R2"}},"outputSettings":{"fileExtension":"csv"}}`
	exit, stdout, stderr := run(t, srv, "request", "--input", input)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", exit, stderr)
	}
	if got.Job["type"] != "file" {
		t.Fatalf("job type = %v, want file", got.Job["type"])
	}
	creds := credentialsOf(t, got)
	if creds["partnerUserID"] != testPartnerUserID || creds["partnerUserSecret"] != testPartnerUserSecret {
		t.Fatalf("credentials not injected: %v", creds)
	}
	in := inputSettingsOf(t, got)
	if in["type"] != "combinedReportData" {
		t.Fatalf("inputSettings.type = %v, want combinedReportData", in["type"])
	}
	// Non-JSON provider body (a filename) is emitted verbatim.
	if !strings.Contains(stdout, "myFile.csv") {
		t.Fatalf("stdout should passthrough non-JSON body, got %q", stdout)
	}
}

func TestRequestRejectsCallerSuppliedCredentials(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":200}`, &capturedRequest{})
	defer srv.Close()

	// A caller must never pass credentials in the body; they come from env only.
	input := `{"type":"get","credentials":{"partnerUserID":"x","partnerUserSecret":"y"},"inputSettings":{"type":"policyList"}}`
	result, _, stderr := runResult(t, srv, testCredentials, "request", "--input", input)
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", result.ExitCode)
	}
	if !strings.Contains(stderr, "credentials") {
		t.Fatalf("stderr should explain credentials are injected, got %q", stderr)
	}
}

func TestRequestRejectsNonObjectInput(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":200}`, &capturedRequest{})
	defer srv.Close()

	result, _, _ := runResult(t, srv, testCredentials, "request", "--input", `["not","an","object"]`)
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", result.ExitCode)
	}
}

func TestRequestRequiresInput(t *testing.T) {
	srv := newServer(t, 200, `{"responseCode":200}`, &capturedRequest{})
	defer srv.Close()

	result, _, _ := runResult(t, srv, testCredentials, "request")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (usage error)", result.ExitCode)
	}
}
