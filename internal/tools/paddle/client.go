package paddle

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

// Base URLs for Paddle Billing. Live and sandbox are wholly separate
// environments with non-interchangeable keys and datasets; the service selects
// the base from the injected key's prefix so the caller never supplies a URL.
const (
	liveBaseURL    = "https://api.paddle.com"
	sandboxBaseURL = "https://sandbox-api.paddle.com"
)

// paddleVersion pins the dated Paddle-Version so the response envelope shape
// this package decodes stays stable across Paddle's dated API releases.
const paddleVersion = "1"

// baseURLForKey routes to the live or sandbox base from the key prefix. Keys
// minted since 2025-05-06 encode the environment (pdl_live_… / pdl_sdbx_…).
// Legacy unstructured keys carry no environment, so they default to live and
// can be overridden to sandbox with PADDLE_ENV=sandbox.
func baseURLForKey(token, env string) string {
	switch {
	case strings.HasPrefix(token, "pdl_live_"):
		return liveBaseURL
	case strings.HasPrefix(token, "pdl_sdbx_"):
		return sandboxBaseURL
	case strings.EqualFold(strings.TrimSpace(env), "sandbox"):
		return sandboxBaseURL
	default:
		return liveBaseURL
	}
}

// usageError is a parameter / usage error (bad flag combo, missing required
// flag, invalid JSON). It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Paddle non-2xx response or a transport
// failure. It maps to exit code 1. It carries the parsed Paddle error object so
// the --json envelope can surface code/detail/documentation_url and per-field
// validation errors. It wraps the cause so errors.As for credentialRejected
// still resolves through it.
type apiError struct {
	status     int
	requestID  string
	retryAfter string
	detail     paddleError
	err        error
}

func (e *apiError) Error() string {
	if e.detail.Code != "" || e.detail.Detail != "" {
		return fmt.Sprintf("paddle API error (HTTP %d): %s: %s", e.status, e.detail.Code, e.detail.Detail)
	}
	if e.err != nil {
		return fmt.Sprintf("paddle request failed: %v", e.err)
	}
	return fmt.Sprintf("paddle API error (HTTP %d)", e.status)
}

func (e *apiError) Unwrap() error { return e.err }

// paddleError mirrors Paddle's error object. Validation (400) errors add a
// per-field errors[] array surfaced verbatim in the --json envelope.
type paddleError struct {
	Type             string            `json:"type"`
	Code             string            `json:"code"`
	Detail           string            `json:"detail"`
	DocumentationURL string            `json:"documentation_url"`
	Errors           []json.RawMessage `json:"errors,omitempty"`
}

// errorEnvelope is the Paddle error response shape.
type errorEnvelope struct {
	Error paddleError `json:"error"`
	Meta  struct {
		RequestID string `json:"request_id"`
	} `json:"meta"`
}

// successEnvelope is the Paddle success response shape. data is passed through
// verbatim; meta carries the request id and cursor pagination.
type successEnvelope struct {
	Data json.RawMessage `json:"data"`
	Meta json.RawMessage `json:"meta,omitempty"`
}

// call performs one Paddle API request: Bearer auth, pinned Paddle-Version, and
// JSON content-type on write bodies. A non-2xx surfaces the parsed error object
// as an apiError (credential rejection on 401/403); a transport failure surfaces
// as an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload []byte) (*successEnvelope, error) {
	base := s.baseURL
	if base == "" {
		base = baseURLForKey(token, s.env)
	}
	endpoint := base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, &apiError{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Paddle-Version", paddleVersion)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{err: fmt.Errorf("%s %s: %w", method, path, err)}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{status: resp.StatusCode, err: fmt.Errorf("read response: %w", err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp, raw)
	}
	var env successEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &apiError{status: resp.StatusCode, err: fmt.Errorf("decode response: %w", err)}
	}
	return &env, nil
}

// newAPIError builds an apiError from a non-2xx Paddle response, classifying
// 401/403 as credential rejection so the engine can invalidate the stored key.
func newAPIError(resp *http.Response, raw []byte) error {
	var env errorEnvelope
	_ = json.Unmarshal(raw, &env)
	apiErr := &apiError{
		status:     resp.StatusCode,
		requestID:  env.Meta.RequestID,
		retryAfter: resp.Header.Get("Retry-After"),
		detail:     env.Error,
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &apiError{
			status:     apiErr.status,
			requestID:  apiErr.requestID,
			retryAfter: apiErr.retryAfter,
			detail:     apiErr.detail,
			err:        execution.RejectCredential(fmt.Errorf("%s", apiErr.Error())),
		}
	}
	return apiErr
}

// The usageError message is set via the struct literal below; expose a helper
// so paddle.go can build one before cobra parses flags.
func newUsageError(format string, args ...any) *usageError {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}
