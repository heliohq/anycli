package freshservice

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseCredential(t *testing.T) {
	cases := []struct {
		name    string
		blob    string
		key     string
		host    string
		wantErr bool
	}{
		{name: "well-formed", blob: "https://key-abc123@acme.freshservice.com", key: "key-abc123", host: "acme.freshservice.com"},
		{name: "trailing slash", blob: "https://k@acme.freshservice.com/", key: "k", host: "acme.freshservice.com"},
		{name: "empty", blob: "", wantErr: true},
		{name: "no key", blob: "https://acme.freshservice.com", wantErr: true},
		{name: "no host", blob: "https://key@", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cred, err := parseCredential(tc.blob)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got none", tc.blob)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cred.apiKey != tc.key || cred.host != tc.host {
				t.Fatalf("got key=%q host=%q, want key=%q host=%q", cred.apiKey, cred.host, tc.key, tc.host)
			}
		})
	}
}

func TestMissingCredentialExit2(t *testing.T) {
	code, _, errStr, _ := runBlob(t, nil, "", "ticket", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errStr, EnvURL) {
		t.Fatalf("stderr should mention %s: %q", EnvURL, errStr)
	}
}

func TestMalformedCredentialExit2(t *testing.T) {
	// A blob with no API key in userinfo is a malformed credential.
	code, _, errStr, _ := runBlob(t, nil, "https://acme.freshservice.com", "ticket", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errStr, "API key") {
		t.Fatalf("stderr should explain the missing key: %q", errStr)
	}
}

func TestMalformedCredentialJSONEnvelope(t *testing.T) {
	code, _, errStr, _ := runBlob(t, nil, "", "--json", "ticket", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	m := decodeJSON(t, strings.TrimSpace(errStr))
	if _, ok := m["error"]; !ok {
		t.Fatalf("expected error envelope, got %q", errStr)
	}
}

// The credential blob must never be echoed in an error message.
func TestCredentialNeverEchoed(t *testing.T) {
	code, _, errStr, _ := runBlob(t, nil, "not a url with spaces %zz", "ticket", "list")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if strings.Contains(errStr, "%zz") {
		t.Fatalf("stderr leaked the raw blob: %q", errStr)
	}
}

func TestAuthHeaderIsBasicAPIKey(t *testing.T) {
	captured := map[string]capturedRequest{}
	srv := newFakeServer(t, map[string]routeReply{
		"/tickets": {body: `{"tickets":[]}`},
	}, captured)
	defer srv.Close()

	code, _, errStr := run(t, srv, "ticket", "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errStr)
	}
	got := captured["/tickets"]
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(testAPIKey+":X"))
	if got.Auth != want {
		t.Fatalf("Authorization = %q, want %q", got.Auth, want)
	}
	if got.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got.Accept)
	}
}
