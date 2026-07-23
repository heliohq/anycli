package braze

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

// client carries the resolved Bearer key and cluster base URL for one
// invocation and performs the actual Braze HTTP requests.
type client struct {
	apiKey  string
	baseURL string
	hc      *http.Client
	out     io.Writer
}

// usageError is a parameter / usage error: a bad flag combination, missing
// required flag, invalid JSON, or a malformed credential. It maps to exit code
// 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Braze non-2xx response or a transport
// failure. It maps to exit code 1. kind distinguishes the three actionable
// failures — "credential" (401), "permission" (403), "rateLimit" (429) — from
// the generic "api" bucket so the host reacts differently to each. status is
// the HTTP status (0 for transport failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg                string
	kind               string
	status             int
	rateLimitReset     string
	rateLimitRemaining string
	err                error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// get performs a GET with optional query parameters.
func (c *client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, query, nil)
}

// post performs a POST with an optional JSON payload.
func (c *client) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, nil, payload)
}

// do performs one Braze API request with Bearer auth and returns the raw
// response body. A non-2xx is classified into a typed apiError.
func (c *client) do(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("braze: encode request: %v", err), kind: "api", err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braze: build request: %v", err), kind: "api", err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	hc := c.hc
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braze: %s %s: %v", method, path, err), kind: "api", err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("braze: read response: %v", err), kind: "api", err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyError(resp, body)
	}
	return body, nil
}

// classifyError maps a Braze non-2xx response to a typed apiError. 401 marks
// the credential rejected (bad/revoked key — the endpoint is stored with the
// key so a 401 is never a region mismatch); 403 is a distinct permission signal
// (valid key lacks the endpoint scope) that must NOT reject the credential; 429
// carries the X-RateLimit-Reset epoch-seconds back-off hint (Braze sends no
// Retry-After).
func classifyError(resp *http.Response, body []byte) error {
	status := resp.StatusCode
	msg := apiMessage(body)
	switch status {
	case http.StatusUnauthorized:
		raw := fmt.Errorf("braze API error (HTTP 401): %s (the REST API key is invalid or revoked — reconnect with a valid key)", msg)
		rejected := execution.RejectCredential(raw)
		return &apiError{msg: rejected.Error(), kind: "credential", status: status, err: rejected}
	case http.StatusForbidden:
		return &apiError{
			msg:    fmt.Sprintf("braze API error (HTTP 403): %s (the key is valid but lacks the endpoint permission for this call — reconnect with a broader-scoped REST API key)", msg),
			kind:   "permission",
			status: status,
		}
	case http.StatusTooManyRequests:
		reset := resp.Header.Get("X-RateLimit-Reset")
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		detail := ""
		if reset != "" {
			detail = fmt.Sprintf(" (rate limit window resets at %s UTC epoch seconds)", reset)
		}
		return &apiError{
			msg:                fmt.Sprintf("braze API error (HTTP 429): %s%s", msg, detail),
			kind:               "rateLimit",
			status:             status,
			rateLimitReset:     reset,
			rateLimitRemaining: remaining,
		}
	default:
		return &apiError{
			msg:    fmt.Sprintf("braze API error (HTTP %d): %s", status, msg),
			kind:   "api",
			status: status,
		}
	}
}

// apiMessage extracts Braze's error message from a response body, falling back
// to the raw body. Braze errors carry a top-level `message`, and some carry an
// `errors` array of finer-grained reasons.
func apiMessage(body []byte) string {
	var e struct {
		Message string            `json:"message"`
		Errors  []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if len(e.Errors) > 0 {
			return fmt.Sprintf("%s: %s", e.Message, joinRaw(e.Errors))
		}
		return e.Message
	}
	return string(body)
}

// joinRaw renders a slice of raw JSON error entries as a compact list.
func joinRaw(entries []json.RawMessage) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, string(entry))
	}
	return fmt.Sprint(parts)
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (c *client) emit(body []byte) error {
	if _, err := c.out.Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(c.out, "\n")
	return err
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// objectBodyFlag decodes an optional raw-JSON --<name> object flag (default an
// empty object) and overlays the given fixed fields on top of it. The AI passes
// the large, versioned Braze request body through --<name>; the tool only sets
// the id/envelope fields it owns. A non-object body is a usage error.
func objectBodyFlag(name, raw string, overlay map[string]any) (map[string]any, error) {
	body := map[string]any{}
	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &body); err != nil {
			return nil, &usageError{msg: fmt.Sprintf("--%s must be a JSON object: %v", name, err)}
		}
	}
	for k, v := range overlay {
		body[k] = v
	}
	return body, nil
}
