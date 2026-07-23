package posthog

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a PostHog non-2xx response, a transport
// failure, or a region-resolution failure. It maps to exit code 1 and kind
// "api". status is the HTTP status (0 for transport/network failures). It wraps
// the underlying cause so errors.As for the credential-rejection sentinel still
// resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one PostHog API request against the resolved region host and
// returns the raw response body.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	host, err := s.resolveHost(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.do(ctx, token, host, method, path, query, payload)
}

// do issues one request against an explicit host and maps the outcome to a raw
// body or a typed apiError.
func (s *Service) do(ctx context.Context, token, host, method, path string, query url.Values, payload any) ([]byte, error) {
	body, status, err := s.roundtrip(ctx, token, host, method, path, query, payload)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("posthog: %s %s: %v", method, path, err), err: err}
	}
	if status < 200 || status > 299 {
		return nil, s.httpError(status, body)
	}
	return body, nil
}

// roundtrip builds and sends one request, returning the raw body and status. It
// never inspects the status — callers classify it — so the region probe can
// treat a 401 as "wrong region" rather than a fatal credential rejection.
func (s *Service) roundtrip(ctx context.Context, token, host, method, path string, query url.Values, payload any) ([]byte, int, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := strings.TrimRight(host, "/") + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return body, resp.StatusCode, nil
}

// httpError builds a typed apiError from a non-2xx response. A 401 marks the
// credential rejected (feeds heliox's stale-credential feedback); every other
// status surfaces PostHog's {"type","code","detail"} body verbatim.
func (s *Service) httpError(status int, body []byte) error {
	err := &apiError{
		msg:    fmt.Sprintf("posthog API error (HTTP %d): %s", status, apiMessage(body)),
		status: status,
	}
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// resolveHost returns the region host for this invocation. A fixed host
// (BaseURL test seam, cached region, or POSTHOG_API_HOST) skips the probe;
// otherwise it probes US then EU via self().
func (s *Service) resolveHost(ctx context.Context, token string) (string, error) {
	if fixed := s.fixedHost(); fixed != "" {
		return fixed, nil
	}
	_, host, err := s.self(ctx, token)
	return host, err
}

// self fetches the authenticated user (GET /api/users/@me) and the resolved
// region host. When the region is not yet fixed it folds the region probe into
// this fetch — trying US then EU — so `whoami` costs a single request and the
// winning region is cached for the rest of the process.
func (s *Service) self(ctx context.Context, token string) ([]byte, string, error) {
	const path = "/api/users/@me"
	if fixed := s.fixedHost(); fixed != "" {
		body, err := s.do(ctx, token, fixed, http.MethodGet, path, nil, nil)
		return body, fixed, err
	}
	for _, host := range s.regions() {
		body, status, err := s.roundtrip(ctx, token, host, http.MethodGet, path, nil, nil)
		if err != nil {
			return nil, "", &apiError{msg: fmt.Sprintf("posthog: probe %s: %v", host, err), err: err}
		}
		if status == http.StatusUnauthorized {
			continue // token is not known to this region — try the next one
		}
		if status < 200 || status > 299 {
			return nil, "", s.httpError(status, body)
		}
		s.region = host
		return body, host, nil
	}
	return nil, "", execution.RejectCredential(&apiError{
		msg:    "posthog: access token not recognized in the US or EU region; set POSTHOG_API_HOST for self-hosted instances",
		status: http.StatusUnauthorized,
	})
}

// fixedHost returns a host that needs no probe: the BaseURL test seam, the
// cached resolved region, or the POSTHOG_API_HOST override. Empty means the
// region must be discovered.
func (s *Service) fixedHost() string {
	if s.BaseURL != "" {
		return s.baseURL()
	}
	if s.region != "" {
		return s.region
	}
	if s.apiHost != "" {
		return strings.TrimRight(s.apiHost, "/")
	}
	return ""
}

// regions returns the probe order (US then EU), honoring test overrides.
func (s *Service) regions() []string {
	us, eu := USHost, EUHost
	if s.usHost != "" {
		us = s.usHost
	}
	if s.euHost != "" {
		eu = s.euHost
	}
	return []string{us, eu}
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts PostHog's error detail from an error body, falling back
// to the raw body. PostHog errors are {"type","code","detail","attr"}.
func apiMessage(body []byte) string {
	var e struct {
		Detail string `json:"detail"`
		Code   string `json:"code"`
		Type   string `json:"type"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Detail != "" {
		return e.Detail
	}
	return strings.TrimSpace(string(body))
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
