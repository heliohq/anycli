package searchconsole

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a missing required flag, an illegal
// flag combination, or bad filter syntax. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Search Console non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// escapePathSegment fully percent-encodes a single path segment, including the
// ':' and '/' that appear in Search Console property ids. A URL-prefix property
// (https://example.com/) becomes https%3A%2F%2Fexample.com%2F and a Domain
// property (sc-domain:example.com) becomes sc-domain%3Aexample.com — the exact
// forms the API requires for the {siteUrl} and {feedpath} path parameters.
func escapePathSegment(s string) string {
	// url.QueryEscape encodes ':' and '/' but renders a space as '+'; path
	// segments need %20, so fix that up (siteUrls rarely contain spaces, but a
	// feedpath conceivably could).
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// call performs one Search Console API request with Bearer auth against the
// caller-provided fully-qualified endpoint. A non-2xx surfaces the body's error
// message as an apiError carrying the HTTP status; 401/403 additionally carry
// the missing-scope hint. No auto-retry: Search Console's per-property
// inspection quota returns 429 that must reach the caller verbatim, and a
// mutation (sitemaps submit/delete) must never be silently re-sent.
func (s *Service) call(ctx context.Context, token, method, endpoint string, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("search-console: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("search-console: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("search-console: %s: %v", method, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("search-console: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		hint := ""
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			hint = scopeHint
		}
		raw := fmt.Errorf("search-console API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), hint)
		classified := classifyCredentialError(resp.StatusCode, body, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks only genuine credential rejections (a real 401,
// or an explicit UNAUTHENTICATED status body) so the engine does not invalidate
// a credential that is merely missing a scope (403 PERMISSION_DENIED) or
// rate-limited (429 RESOURCE_EXHAUSTED).
func classifyCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || errorStatus(body) == "UNAUTHENTICATED" {
		return execution.RejectCredential(err)
	}
	return err
}

// errorStatus extracts the Google canonical error status (PERMISSION_DENIED,
// UNAUTHENTICATED, RESOURCE_EXHAUSTED, …) from an error body.
func errorStatus(body []byte) string {
	var e struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	return e.Error.Status
}

// apiMessage extracts Google's error message (status + message) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error.Status != "" || e.Error.Message != "") {
		return strings.TrimSpace(strings.TrimPrefix(e.Error.Status+": "+e.Error.Message, ": "))
	}
	return string(body)
}

// emit writes a provider JSON response to stdout. It refuses to write bytes that
// are not strictly valid JSON so --json output is always parseable.
func (s *Service) emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		return &apiError{msg: "search-console: provider returned invalid JSON"}
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("search-console: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// rawArrayOrEmpty returns the slice as-is, or an empty (non-nil) slice so JSON
// renders [] instead of null.
func rawArrayOrEmpty(rows []json.RawMessage) []json.RawMessage {
	if rows == nil {
		return []json.RawMessage{}
	}
	return rows
}
