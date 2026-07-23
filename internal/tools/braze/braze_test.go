package braze

import (
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// TestParseCredentialsReconstructsBaseURLPerCluster proves multi-cluster
// correctness with no live non-US credential: the base host is reconstructed
// from the DSN's host component, the key from its userinfo. A US and an EU DSN
// resolve to their respective clusters.
func TestParseCredentialsReconstructsBaseURLPerCluster(t *testing.T) {
	cases := []struct {
		name    string
		dsn     string
		wantKey string
		wantURL string
	}{
		{"US cluster", "https://us-key@rest.iad-05.braze.com", "us-key", "https://rest.iad-05.braze.com"},
		{"US-10 cluster", "https://k@rest.us-10.braze.com", "k", "https://rest.us-10.braze.com"},
		{"EU cluster", "https://eu-key@rest.fra-01.braze.eu", "eu-key", "https://rest.fra-01.braze.eu"},
		{"trailing slash tolerated", "https://k@rest.au-01.braze.com/", "k", "https://rest.au-01.braze.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, base, err := parseCredentials(tc.dsn)
			if err != nil {
				t.Fatalf("parseCredentials(%q) error: %v", tc.dsn, err)
			}
			if key != tc.wantKey {
				t.Fatalf("key = %q, want %q", key, tc.wantKey)
			}
			if base != tc.wantURL {
				t.Fatalf("baseURL = %q, want %q", base, tc.wantURL)
			}
		})
	}
}

// TestParseCredentialsRejectsMalformed covers every fatal-config shape, and
// asserts the error never echoes the secret.
func TestParseCredentialsRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
	}{
		{"empty", ""},
		{"no scheme/host", "definitely-not-a-dsn"},
		{"no key half", "https://rest.iad-05.braze.com"},
		{"hostless", "https://secret-key@"},
		{"unknown host suffix", "https://secret-key@rest.iad-05.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseCredentials(tc.dsn)
			if err == nil {
				t.Fatalf("parseCredentials(%q) = nil error, want failure", tc.dsn)
			}
			if strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("error echoes the secret: %q", err.Error())
			}
		})
	}
}

// TestExecuteMissingCredentialsExitsTwo — a missing/malformed BRAZE_CREDENTIALS
// is a fatal config error (exit 2), and the secret is never echoed.
func TestExecuteMissingCredentialsExitsTwo(t *testing.T) {
	result, _, stderr := runResultEnv(t, nil, map[string]string{}, "campaigns", "list")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode)
	}
	if result.CredentialRejected {
		t.Fatal("missing config must not set CredentialRejected")
	}

	result, _, stderr = runResultEnv(t, nil, map[string]string{EnvCredentials: "https://leaky-secret@bad"}, "--json", "campaigns", "list")
	if result.ExitCode != 2 {
		t.Fatalf("malformed exit = %d, want 2", result.ExitCode)
	}
	if strings.Contains(stderr, "leaky-secret") {
		t.Fatalf("stderr echoes the secret: %q", stderr)
	}
	if got := errorEnvelope(t, stderr)["kind"]; got != "usage" {
		t.Fatalf("kind = %v, want usage", got)
	}
}

// TestBearerHeaderFromDSNAndJSONPassthrough — every request carries
// Authorization: Bearer <key-from-userinfo>, the base is the overridden host,
// and Braze's JSON body is passed through on stdout verbatim + newline.
func TestBearerHeaderFromDSNAndJSONPassthrough(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{
		"GET /campaigns/list": {status: 200, body: `{"campaigns":[{"id":"c1","name":"Welcome"}],"message":"success"}`},
	})
	defer srv.Close()

	exit, stdout, stderr := run(t, srv, "campaigns", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%s)", exit, stderr)
	}
	req := findReq(reqs, "GET", "/campaigns/list")
	if req == nil {
		t.Fatal("no GET /campaigns/list request recorded")
	}
	if req.Auth != "Bearer "+testKey {
		t.Fatalf("Authorization = %q, want Bearer %s", req.Auth, testKey)
	}
	if stdout != `{"campaigns":[{"id":"c1","name":"Welcome"}],"message":"success"}`+"\n" {
		t.Fatalf("stdout = %q, want verbatim JSON + newline", stdout)
	}
	if strings.Contains(stderr, testKey) {
		t.Fatalf("stderr leaks the key: %q", stderr)
	}
}

// TestCredentialErrorClassification — 401 (credential, CredentialRejected),
// 403 (permission, not rejected), 429 (rateLimit, reset hint) are each surfaced
// distinctly from the generic 4xx bucket, all exit 1.
func TestCredentialErrorClassification(t *testing.T) {
	t.Run("401 rejects the credential", func(t *testing.T) {
		var reqs []capturedRequest
		srv := newMux(t, &reqs, map[string]stub{
			"GET /campaigns/list": {status: 401, body: `{"message":"Invalid API key"}`},
		})
		defer srv.Close()

		result, _, stderr := runResult(t, srv, "--json", "campaigns", "list")
		if result.ExitCode != 1 || !result.CredentialRejected {
			t.Fatalf("result = %+v, want exit 1 + CredentialRejected", result)
		}
		env := errorEnvelope(t, stderr)
		if env["kind"] != "credential" {
			t.Fatalf("kind = %v, want credential", env["kind"])
		}
		if env["status"].(float64) != 401 {
			t.Fatalf("status = %v, want 401", env["status"])
		}
	})

	t.Run("403 is a permission signal, not a dead key", func(t *testing.T) {
		var reqs []capturedRequest
		srv := newMux(t, &reqs, map[string]stub{
			"POST /messages/send": {status: 403, body: `{"message":"This endpoint is not authorized for this key"}`},
		})
		defer srv.Close()

		result, _, stderr := runResult(t, srv, "--json", "messages", "send", "--body", `{"broadcast":true}`)
		if result.ExitCode != 1 {
			t.Fatalf("exit = %d, want 1", result.ExitCode)
		}
		if result.CredentialRejected {
			t.Fatal("403 must NOT set CredentialRejected — the key is live for its own scope")
		}
		if got := errorEnvelope(t, stderr)["kind"]; got != "permission" {
			t.Fatalf("kind = %v, want permission", got)
		}
	})

	t.Run("429 carries the reset hint and is transient", func(t *testing.T) {
		var reqs []capturedRequest
		srv := newMux(t, &reqs, map[string]stub{
			"GET /kpi/dau/data_series": {
				status:  429,
				body:    `{"message":"rate limit exceeded"}`,
				headers: map[string]string{"X-RateLimit-Reset": "1753200000", "X-RateLimit-Remaining": "0"},
			},
		})
		defer srv.Close()

		result, _, stderr := runResult(t, srv, "--json", "kpi", "dau", "--length", "7")
		if result.ExitCode != 1 {
			t.Fatalf("exit = %d, want 1", result.ExitCode)
		}
		if result.CredentialRejected {
			t.Fatal("429 must NOT set CredentialRejected")
		}
		env := errorEnvelope(t, stderr)
		if env["kind"] != "rateLimit" {
			t.Fatalf("kind = %v, want rateLimit", env["kind"])
		}
		if env["rate_limit_reset"] != "1753200000" {
			t.Fatalf("rate_limit_reset = %v, want 1753200000 epoch-seconds hint", env["rate_limit_reset"])
		}
	})

	t.Run("other 4xx is the generic api bucket", func(t *testing.T) {
		var reqs []capturedRequest
		srv := newMux(t, &reqs, map[string]stub{
			"GET /campaigns/details": {status: 400, body: `{"message":"invalid campaign_id"}`},
		})
		defer srv.Close()

		result, _, stderr := runResult(t, srv, "--json", "campaigns", "details", "--campaign-id", "nope")
		if result.ExitCode != 1 || result.CredentialRejected {
			t.Fatalf("result = %+v, want exit 1, not rejected", result)
		}
		if got := errorEnvelope(t, stderr)["kind"]; got != "api" {
			t.Fatalf("kind = %v, want api", got)
		}
	})
}

// TestUnknownSubcommandIsUsageExitTwo — a bad verb is a usage error (exit 2),
// never a false success.
func TestUnknownSubcommandIsUsageExitTwo(t *testing.T) {
	var reqs []capturedRequest
	srv := newMux(t, &reqs, map[string]stub{})
	defer srv.Close()

	result, _, _ := runResult(t, srv, "campaigns", "frobnicate")
	if result.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", result.ExitCode)
	}
	if len(reqs) != 0 {
		t.Fatalf("unknown subcommand hit the API %d times, want 0", len(reqs))
	}
}

var _ = execution.Result{}
