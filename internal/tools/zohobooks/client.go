package zohobooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a missing required flag, invalid
// JSON. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// credentialError is a missing/failed credential (e.g. the access-token env var
// is unset). It maps to exit code 1 — a runtime credential failure, not a
// caller usage mistake — and kind "credential", so the JSON error kind stays
// aligned with the exit-code contract (usage=2, runtime=1).
type credentialError struct{ msg string }

func (e *credentialError) Error() string { return e.msg }

// apiError is a runtime / API error: a Books non-2xx response, a 2xx carrying a
// non-zero body `code`, or a transport failure. It maps to exit code 1 and
// kind "api". status is the HTTP status (0 for transport/network failures). It
// wraps the underlying cause so errors.As for the credential-rejection
// sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// emitJSON writes the provider's JSON response (envelope included, so the agent
// sees page_context) to stdout verbatim. An empty body writes nothing and the
// caller still exits 0.
func (s *Service) emitJSON(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Zoho Books API request. The path is joined onto BaseURL +
// /books/v3. Auth uses the Books-documented Zoho-oauthtoken scheme (NOT
// Bearer). A non-2xx surfaces the body's integer code/message as an apiError
// carrying the HTTP status; a transport failure surfaces as an apiError with
// status 0. On a 2xx, the body is checked defensively: Books' integer `code` is
// the authoritative success signal, so a non-zero `code` on a 2xx is still
// surfaced as an error. Otherwise the raw body is returned.
func (s *Service) call(ctx context.Context, token, method, path string, body []byte) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+apiPrefix+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoho-books: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoho-books: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoho-books: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("zoho-books API error (HTTP %d): %s", resp.StatusCode, apiMessage(respBody))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	// Defensive 2xx check: Books' integer `code` is authoritative, so a 2xx
	// carrying a non-zero `code` is still an error (not a credential rejection —
	// the token was accepted; the request itself was rejected).
	if code, ok := bodyCode(respBody); ok && code != 0 {
		return nil, &apiError{
			msg:    fmt.Sprintf("zoho-books API error (HTTP %d): %s", resp.StatusCode, apiMessage(respBody)),
			status: resp.StatusCode,
			err:    fmt.Errorf("zoho-books API error: %s", apiMessage(respBody)),
		}
	}
	return respBody, nil
}

// apiMessage extracts Books' integer code + message from an error body
// ({"code":57,"message":"…"}), falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		return fmt.Sprintf("code %d: %s", e.Code, e.Message)
	}
	return string(body)
}

// bodyCode returns the top-level integer `code` of a Books response body and
// whether it was present. Books uses `code:0` for success on every envelope.
func bodyCode(body []byte) (int, bool) {
	var envelope struct {
		Code *int `json:"code"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Code == nil {
		return 0, false
	}
	return *envelope.Code, true
}

// classifyCredentialError marks an HTTP 401 as an explicit credential rejection
// so the engine invalidates the stored token. Per the official Books error
// table, 401 means "Unauthorized (Invalid AuthToken)". Unlike Zoho CRM (whose
// string error codes distinguish INVALID_TOKEN from OAUTH_SCOPE_MISMATCH),
// Books does not expose a body-code split between an invalid token and a
// missing scope — both surface as HTTP 401 with body code 57 — so V1 keys the
// rejection on the HTTP 401 status alone. A scope-miss reaching this path
// forces a reconnect, which is the correct remedy (the missing scope can only
// be granted by re-consenting). Every other status (400 permission, 404, 429,
// 500) leaves the credential intact.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}
