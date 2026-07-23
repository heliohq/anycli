package netsuite

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// TBA (Token-Based Authentication) is NetSuite's OAuth 1.0a-style per-request
// signing: HMAC-SHA256 over four secrets (consumer key/secret + token
// id/secret) plus the account id. This file owns the signing math with the Go
// standard library only — no third-party OAuth1 dependency.

// rfc3986Escape percent-encodes s per RFC 3986, leaving only the unreserved set
// (A-Z a-z 0-9 - . _ ~) unescaped. net/url's escapers do not match OAuth's
// required set (QueryEscape encodes space as '+', PathEscape leaves sub-delims),
// so OAuth 1.0a signing needs this exact helper.
func rfc3986Escape(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// deriveHost returns the account-specific SuiteTalk subdomain host component:
// lowercase, underscores to hyphens (account 9876543_SB1 -> 9876543-sb1). The
// user's pasted casing/separators are normalized, never trusted verbatim.
func deriveHost(accountID string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(accountID), "_", "-"))
}

// deriveRealm returns the canonical OAuth realm form of the account id:
// UPPERCASE, hyphens to underscores (9876543-sb1 -> 9876543_SB1). NetSuite's
// realm is case-sensitive and expects this canonical form, so a user who pastes
// the lowercase/hyphen host form still signs correctly.
func deriveRealm(accountID string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(accountID), "-", "_"))
}

// signatureBaseString builds the OAuth 1.0a signature base string:
//
//	METHOD & rfc3986(scheme://host/path) & rfc3986(sorted-merged-params)
//
// oauthParams (the oauth_* set) are merged with rawURL's query params — SuiteQL
// folds limit/offset into the query, and every query param MUST enter the base
// string or the signature will not match. Params are sorted by encoded key then
// encoded value, joined key=value with '&', then the whole joined string is
// itself percent-encoded into the base string.
func signatureBaseString(method, rawURL string, oauthParams map[string]string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url for signing: %w", err)
	}
	pairs := make([]string, 0, len(oauthParams)+4)
	add := func(k, v string) {
		pairs = append(pairs, rfc3986Escape(k)+"="+rfc3986Escape(v))
	}
	for k, v := range oauthParams {
		add(k, v)
	}
	for k, vs := range u.Query() {
		for _, v := range vs {
			add(k, v)
		}
	}
	sort.Strings(pairs)
	baseURL := u.Scheme + "://" + u.Host + u.EscapedPath()
	return strings.ToUpper(method) + "&" + rfc3986Escape(baseURL) + "&" + rfc3986Escape(strings.Join(pairs, "&")), nil
}

// sign computes the base64 HMAC-SHA256 signature. The signing key is
// rfc3986(consumer_secret) & rfc3986(token_secret).
func sign(baseString, consumerSecret, tokenSecret string) string {
	key := rfc3986Escape(consumerSecret) + "&" + rfc3986Escape(tokenSecret)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(baseString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// authorizationHeader builds the full OAuth 1.0a Authorization header value for
// one request. Every value is rfc3986-encoded and double-quoted; the realm is
// the canonical account id (deriveRealm).
func authorizationHeader(creds tbaCreds, method, rawURL, timestamp, nonce string) (string, error) {
	oauthParams := map[string]string{
		"oauth_consumer_key":     creds.ConsumerKey,
		"oauth_token":            creds.TokenID,
		"oauth_signature_method": "HMAC-SHA256",
		"oauth_timestamp":        timestamp,
		"oauth_nonce":            nonce,
		"oauth_version":          "1.0",
	}
	base, err := signatureBaseString(method, rawURL, oauthParams)
	if err != nil {
		return "", err
	}
	oauthParams["oauth_signature"] = sign(base, creds.ConsumerSecret, creds.TokenSecret)

	// Header field order is not significant to NetSuite, but a stable order
	// keeps the output deterministic and testable. realm is first per convention.
	fields := []string{fmt.Sprintf("realm=%q", deriveRealm(creds.AccountID))}
	keys := make([]string, 0, len(oauthParams))
	for k := range oauthParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fields = append(fields, fmt.Sprintf("%s=%q", k, rfc3986Escape(oauthParams[k])))
	}
	return "OAuth " + strings.Join(fields, ", "), nil
}
