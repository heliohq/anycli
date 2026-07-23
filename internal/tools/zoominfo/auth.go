package zoominfo

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ZoomInfo authenticates with a proprietary two-step JWT exchange (verified
// against the official github.com/Zoominfo/api-auth-python-client PKI client):
// the caller signs a short-lived RS256 client-assertion JWT with the account's
// RSA private key, POSTs it to /authenticate, and receives a ~60-minute access
// JWT used as a bearer token on every data call. There is no refresh token —
// the exchange is repeated per invocation. anycli runs one-shot, so a single
// exchange per process stays well within ZoomInfo's 1 req/sec authenticate cap.
const (
	authEndpoint = "/authenticate"

	// jwtAudience / jwtIssuer are the exact client-assertion claims the official
	// ZoomInfo auth client sends; ZoomInfo validates them server-side.
	jwtAudience = "enterprise_api"
	jwtIssuer   = "api-client@zoominfo.com"

	// clientAssertionTTL is the 5-minute assertion lifetime the official client
	// uses (expiry_time_in_seconds = 300). Only used for the immediate exchange.
	clientAssertionTTL = 5 * time.Minute
)

// credentials is the long-lived PKI credential anycli injects as the
// ZOOMINFO_CREDENTIALS JSON blob (Helio projects the three connect-form fields
// as one packed secret). All three fields are required; the private key is a
// multi-line PEM RSA key. No human login password is ever stored.
type credentials struct {
	Username   string `json:"username"`
	ClientID   string `json:"client_id"`
	PrivateKey string `json:"private_key"`
}

// parseCredentials decodes and validates the injected credential blob. A
// missing field is a usage error (exit 2): the connection is misconfigured,
// not a runtime/API failure.
func parseCredentials(raw string) (credentials, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return credentials{}, &usageError{msg: "ZOOMINFO_CREDENTIALS is not set"}
	}
	var c credentials
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return credentials{}, &usageError{msg: fmt.Sprintf("ZOOMINFO_CREDENTIALS is not valid JSON: %v", err)}
	}
	c.Username = strings.TrimSpace(c.Username)
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.PrivateKey = strings.TrimSpace(c.PrivateKey)
	switch {
	case c.Username == "":
		return credentials{}, &usageError{msg: "ZOOMINFO_CREDENTIALS is missing username"}
	case c.ClientID == "":
		return credentials{}, &usageError{msg: "ZOOMINFO_CREDENTIALS is missing client_id"}
	case c.PrivateKey == "":
		return credentials{}, &usageError{msg: "ZOOMINFO_CREDENTIALS is missing private_key"}
	}
	return c, nil
}

// buildClientAssertion mints the RS256-signed client-assertion JWT with the
// exact claim set ZoomInfo expects. Signing uses the stdlib crypto/rsa so
// anycli takes on no new module dependency for its first JWT-signing tool.
func buildClientAssertion(c credentials, now time.Time) (string, error) {
	key, err := parseRSAPrivateKey(c.PrivateKey)
	if err != nil {
		return "", &usageError{msg: fmt.Sprintf("ZOOMINFO_CREDENTIALS private_key is not a valid RSA private key: %v", err)}
	}
	header := map[string]any{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"aud":       jwtAudience,
		"iss":       jwtIssuer,
		"iat":       now.Unix(),
		"exp":       now.Add(clientAssertionTTL).Unix(),
		"client_id": c.ClientID,
		"username":  c.Username,
	}
	signingInput, err := jwtSigningInput(header, claims)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zoominfo: sign client assertion: %v", err), err: err}
	}
	return signingInput + "." + base64URL(sig), nil
}

// parseRSAPrivateKey accepts both PKCS#1 ("RSA PRIVATE KEY") and PKCS#8
// ("PRIVATE KEY") PEM encodings — ZoomInfo's Admin Portal and common tooling
// emit either.
func parseRSAPrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("PEM key is not an RSA private key")
	}
	return key, nil
}

// jwtSigningInput renders base64url(header) + "." + base64url(claims).
func jwtSigningInput(header, claims map[string]any) (string, error) {
	h, err := json.Marshal(header)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zoominfo: encode jwt header: %v", err), err: err}
	}
	c, err := json.Marshal(claims)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zoominfo: encode jwt claims: %v", err), err: err}
	}
	return base64URL(h) + "." + base64URL(c), nil
}

func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// authenticate exchanges the client-assertion JWT for a short-lived access JWT.
// A 401/403 means the PKI credential is bad or expired → credential rejection
// (exit 1, so the runtime can surface a reconnect). The response body is
// {"jwt": "<access JWT>"}.
func (s *Service) authenticate(ctx context.Context, c credentials) (string, error) {
	assertion, err := buildClientAssertion(c, time.Now())
	if err != nil {
		return "", err
	}
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+authEndpoint, nil)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zoominfo: build authenticate request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+assertion)
	req.Header.Set("Accept", "application/json")
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zoominfo: authenticate: %v", err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("zoominfo: read authenticate response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("zoominfo authenticate failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return "", &apiError{msg: classifyCredentialError(resp.StatusCode, raw).Error(), status: resp.StatusCode, err: classifyCredentialError(resp.StatusCode, raw)}
	}
	var envelope struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || strings.TrimSpace(envelope.JWT) == "" {
		return "", &apiError{msg: "zoominfo: authenticate response did not contain a jwt", err: err}
	}
	return envelope.JWT, nil
}
