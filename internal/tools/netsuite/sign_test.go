package netsuite

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestRFC3986EscapeUnreservedAndReserved(t *testing.T) {
	cases := map[string]string{
		"AZaz09-._~": "AZaz09-._~",        // unreserved pass through
		"a b":        "a%20b",             // space is %20, not +
		"a+b":        "a%2Bb",             // plus is escaped
		"a/b?c=d&e":  "a%2Fb%3Fc%3Dd%26e", // reserved all escaped
		"SELECT *":   "SELECT%20%2A",      // '*' is escaped (not unreserved)
		"n=1,2":      "n%3D1%2C2",         // comma escaped
	}
	for in, want := range cases {
		if got := rfc3986Escape(in); got != want {
			t.Errorf("rfc3986Escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeriveHostAndRealmTransforms(t *testing.T) {
	cases := []struct {
		in        string
		wantHost  string
		wantRealm string
	}{
		{"1234567", "1234567", "1234567"},
		{"9876543_SB1", "9876543-sb1", "9876543_SB1"},
		// Lowercased/hyphen input still normalizes to the canonical realm form.
		{"1234567_sb1", "1234567-sb1", "1234567_SB1"},
		{"9876543-sb1", "9876543-sb1", "9876543_SB1"},
		{"  9876543_SB1  ", "9876543-sb1", "9876543_SB1"},
	}
	for _, c := range cases {
		if got := deriveHost(c.in); got != c.wantHost {
			t.Errorf("deriveHost(%q) = %q, want %q", c.in, got, c.wantHost)
		}
		if got := deriveRealm(c.in); got != c.wantRealm {
			t.Errorf("deriveRealm(%q) = %q, want %q", c.in, got, c.wantRealm)
		}
	}
}

func TestSignatureBaseStringFoldsQueryParams(t *testing.T) {
	oauth := map[string]string{
		"oauth_consumer_key":     "ck",
		"oauth_token":            "tk",
		"oauth_signature_method": "HMAC-SHA256",
		"oauth_timestamp":        "1700000000",
		"oauth_nonce":            "nonce123",
		"oauth_version":          "1.0",
	}
	base, err := signatureBaseString("post", "https://1234567.suitetalk.api.netsuite.com/services/rest/query/v1/suiteql?limit=5&offset=10", oauth)
	if err != nil {
		t.Fatalf("signatureBaseString: %v", err)
	}
	// Method uppercased; base URL has no query; params percent-encoded and sorted.
	if !strings.HasPrefix(base, "POST&") {
		t.Errorf("base does not start with POST&: %q", base)
	}
	if strings.Contains(base, "?limit") {
		t.Errorf("base URL must not carry the query string: %q", base)
	}
	// The SuiteQL limit/offset query params MUST fold into the signed params.
	if !strings.Contains(base, "limit%3D5") || !strings.Contains(base, "offset%3D10") {
		t.Errorf("query params not folded into signature base: %q", base)
	}
	if !strings.Contains(base, "oauth_nonce%3Dnonce123") {
		t.Errorf("oauth params not in signature base: %q", base)
	}
}

func TestAuthorizationHeaderShapeAndSignatureVerifies(t *testing.T) {
	creds := tbaCreds{
		AccountID:      "9876543_sb1",
		ConsumerKey:    "consumer-key",
		ConsumerSecret: "consumer-secret",
		TokenID:        "token-id",
		TokenSecret:    "token-secret",
	}
	url := "https://9876543-sb1.suitetalk.api.netsuite.com/services/rest/record/v1/customer/42"
	header, err := authorizationHeader(creds, "GET", url, "1700000000", "fixed-nonce")
	if err != nil {
		t.Fatalf("authorizationHeader: %v", err)
	}
	// Realm must be the canonical uppercase/underscore form, not the input casing.
	if !strings.Contains(header, `realm="9876543_SB1"`) {
		t.Errorf("header realm not canonicalized: %q", header)
	}
	for _, want := range []string{
		`oauth_consumer_key="consumer-key"`,
		`oauth_token="token-id"`,
		`oauth_signature_method="HMAC-SHA256"`,
		`oauth_timestamp="1700000000"`,
		`oauth_nonce="fixed-nonce"`,
		`oauth_version="1.0"`,
		`oauth_signature="`,
	} {
		if !strings.Contains(header, want) {
			t.Errorf("header missing %q: %q", want, header)
		}
	}
	if !strings.HasPrefix(header, "OAuth ") {
		t.Errorf("header must start with OAuth: %q", header)
	}

	// Recompute the signature independently and confirm the header carries it.
	oauth := map[string]string{
		"oauth_consumer_key":     creds.ConsumerKey,
		"oauth_token":            creds.TokenID,
		"oauth_signature_method": "HMAC-SHA256",
		"oauth_timestamp":        "1700000000",
		"oauth_nonce":            "fixed-nonce",
		"oauth_version":          "1.0",
	}
	base, err := signatureBaseString("GET", url, oauth)
	if err != nil {
		t.Fatalf("signatureBaseString: %v", err)
	}
	key := rfc3986Escape(creds.ConsumerSecret) + "&" + rfc3986Escape(creds.TokenSecret)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(base))
	wantSig := rfc3986Escape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	if !strings.Contains(header, `oauth_signature="`+wantSig+`"`) {
		t.Errorf("header signature does not match independent HMAC-SHA256 recompute\nheader: %q\nwant sig: %q", header, wantSig)
	}
}
