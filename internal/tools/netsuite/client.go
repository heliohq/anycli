package netsuite

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// tbaCreds is the decoded NETSUITE_CREDENTIALS JSON payload — the five
// Token-Based Authentication values collected by the Helio connect form (or a
// pasted JSON blob) and handed to AnyCLI as one opaque secret.
type tbaCreds struct {
	AccountID      string `json:"account_id"`
	ConsumerKey    string `json:"consumer_key"`
	ConsumerSecret string `json:"consumer_secret"`
	TokenID        string `json:"token_id"`
	TokenSecret    string `json:"token_secret"`
}

// decodeCreds parses and validates the NETSUITE_CREDENTIALS payload. A missing,
// non-JSON, or any-empty-field payload is a usage error (exit 2) — the caller's
// connect form assembled a malformed secret, not a live API rejection.
func decodeCreds(raw string) (tbaCreds, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tbaCreds{}, &usageError{msg: "NETSUITE_CREDENTIALS is not set"}
	}
	var c tbaCreds
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return tbaCreds{}, &usageError{msg: fmt.Sprintf("NETSUITE_CREDENTIALS is not valid JSON: %v", err)}
	}
	missing := make([]string, 0, 5)
	for _, f := range []struct {
		name  string
		value string
	}{
		{"account_id", c.AccountID},
		{"consumer_key", c.ConsumerKey},
		{"consumer_secret", c.ConsumerSecret},
		{"token_id", c.TokenID},
		{"token_secret", c.TokenSecret},
	} {
		if strings.TrimSpace(f.value) == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return tbaCreds{}, &usageError{msg: fmt.Sprintf("NETSUITE_CREDENTIALS is missing required field(s): %s", strings.Join(missing, ", "))}
	}
	return c, nil
}

// usageError is a parameter / usage error (exit 2, kind "usage"): missing or
// malformed credentials, illegal flags, invalid --body JSON, unknown verb.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error (exit 1, kind "api"): a NetSuite non-2xx
// response, a transport failure, or a signing failure. status is the HTTP
// status (0 for transport/signing failures); retryAfter is populated only when
// NetSuite actually returns a Retry-After header on a 429.
type apiError struct {
	msg        string
	status     int
	retryAfter string
	err        error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// DefaultBaseURLFor builds the account-specific SuiteTalk REST base URL from the
// decoded account id (host component is lowercase/hyphen).
func DefaultBaseURLFor(accountID string) string {
	return "https://" + deriveHost(accountID) + ".suitetalk.api.netsuite.com/services/rest"
}

// baseURL returns the test override (Service.BaseURL) when set, otherwise the
// account-derived production base. The realm always comes from the account id
// regardless of the override, so the test seam never affects signing identity.
func (s *Service) baseURL(creds tbaCreds) string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURLFor(creds.AccountID)
}

func (s *Service) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

func (s *Service) nonce() string {
	if s.nonceFn != nil {
		return s.nonceFn()
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unrecoverable for signing; fall back to time.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// call signs and executes one SuiteTalk REST request. path is relative to the
// REST base (e.g. "/record/v1/customer/123" or "/query/v1/suiteql?limit=5").
// extraHeaders (e.g. Prefer: transient for SuiteQL) are set after signing —
// only the query string participates in the signature, never headers.
func (s *Service) call(ctx context.Context, creds tbaCreds, method, path string, body []byte, extraHeaders map[string]string) ([]byte, error) {
	fullURL := s.baseURL(creds) + path
	timestamp := strconv.FormatInt(s.now().Unix(), 10)
	authz, err := authorizationHeader(creds, method, fullURL, timestamp, s.nonce())
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("netsuite: sign request: %v", err), err: err}
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("netsuite: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", authz)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("netsuite: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("netsuite: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyError(resp.StatusCode, resp.Header.Get("Retry-After"), respBody, resp.Header.Get("Location"))
	}
	if location := resp.Header.Get("Location"); location != "" && len(bytes.TrimSpace(respBody)) == 0 {
		// Record create returns 204 + Location; surface the new internal id.
		return newLocationBody(location), nil
	}
	return respBody, nil
}

// classifyError maps a NetSuite non-2xx response to an apiError, marking 401 as
// a credential rejection (execution.RejectCredential) and echoing Retry-After
// best-effort on 429.
func classifyError(status int, retryAfter string, body []byte, location string) error {
	msg := fmt.Sprintf("netsuite API error (HTTP %d): %s", status, suiteMessage(body))
	e := &apiError{msg: msg, status: status, err: fmt.Errorf("%s", msg)}
	if status == http.StatusTooManyRequests && strings.TrimSpace(retryAfter) != "" {
		e.retryAfter = strings.TrimSpace(retryAfter)
	}
	if status == http.StatusUnauthorized {
		e.err = execution.RejectCredential(e.err)
	}
	_ = location
	return e
}

// suiteMessage extracts a human-readable message from a NetSuite REST error
// body. The REST error envelope is RFC 7807-style: {"title":..,"o:errorDetails":
// [{"detail":..}]}. Falls back to the raw (bounded) body when it does not parse.
func suiteMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response)"
	}
	var env struct {
		Title        string `json:"title"`
		Detail       string `json:"detail"`
		OErrorDetail []struct {
			Detail string `json:"detail"`
		} `json:"o:errorDetails"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		if len(env.OErrorDetail) > 0 && strings.TrimSpace(env.OErrorDetail[0].Detail) != "" {
			return env.OErrorDetail[0].Detail
		}
		if strings.TrimSpace(env.Detail) != "" {
			return env.Detail
		}
		if strings.TrimSpace(env.Title) != "" {
			return env.Title
		}
	}
	if len(trimmed) > 500 {
		return trimmed[:500]
	}
	return trimmed
}

// newLocationBody wraps a bare Location header (record create) into a small JSON
// envelope surfacing the new internal id (the trailing path segment).
func newLocationBody(location string) []byte {
	id := location
	if i := strings.LastIndex(strings.TrimRight(location, "/"), "/"); i >= 0 {
		id = strings.TrimRight(location, "/")[i+1:]
	}
	b, _ := json.Marshal(map[string]any{"id": id, "location": location})
	return b
}
