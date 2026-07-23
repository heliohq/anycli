package reddit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxErrorBodyBytes = 8 << 10

// usageError is a parameter / usage error: a bad flag combination, missing
// required flag, bad enum value, or invalid argument. It maps to exit code 2
// and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Reddit non-2xx response, a transport
// failure, or the api_type=json HTTP-200 error dialect. It maps to exit code 1
// and kind "api". status is the HTTP status (0 for transport failures and the
// 200-with-errors dialect). It wraps the underlying cause so errors.As for
// *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// get performs an authenticated GET on the Reddit API, always adding raw_json=1
// so &/</> come back unescaped. A non-2xx surfaces the body (and rate-limit
// headers on 429) as an apiError carrying the HTTP status.
func (s *Service) get(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("raw_json", "1")
	return s.do(ctx, token, http.MethodGet, path, query, nil)
}

// postForm performs an authenticated POST with an application/x-www-form-urlencoded
// body. Callers targeting Reddit's action endpoints add api_type=json to the
// form and pass the response through checkJSONErrors to catch the 200-with-errors
// dialect.
func (s *Service) postForm(ctx context.Context, token, path string, form url.Values) ([]byte, error) {
	if form == nil {
		form = url.Values{}
	}
	form.Set("raw_json", "1")
	return s.do(ctx, token, http.MethodPost, path, nil, strings.NewReader(form.Encode()))
}

func (s *Service) do(ctx context.Context, token, method, path string, query url.Values, body io.Reader) ([]byte, error) {
	requestURL := s.apiBase() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("reddit: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: redact(fmt.Sprintf("reddit: %s %s: %v", method, path, err), token), err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("reddit: read response: %v", err), err: err}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, resp.Header, respBody, token)
	}
	return respBody, nil
}

// newAPIError builds an apiError from a non-2xx response, redacting the token,
// surfacing rate-limit headers on 429, and classifying 401 as a credential
// rejection so the engine can invalidate the stored token.
func newAPIError(status int, header http.Header, body []byte, token string) error {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		raw = "empty response body"
	}
	raw = redact(raw, token)
	if len(raw) > maxErrorBodyBytes {
		raw = raw[:maxErrorBodyBytes] + "…"
	}

	hint := ""
	switch status {
	case http.StatusUnauthorized:
		hint = "; access token is invalid or expired — reconnect Reddit"
	case http.StatusForbidden:
		hint = "; token may lack the required scope or account permission"
	case http.StatusTooManyRequests:
		hint = fmt.Sprintf("; rate limit exceeded (remaining=%s reset=%s) — do not retry in a loop",
			headerOr(header, "X-Ratelimit-Remaining", "?"),
			headerOr(header, "X-Ratelimit-Reset", "?"))
	}
	base := fmt.Errorf("reddit API error (HTTP %d %s): %s%s", status, http.StatusText(status), raw, hint)
	return &apiError{msg: base.Error(), status: status, err: classifyRedditCredentialError(status, base)}
}

func headerOr(header http.Header, key, fallback string) string {
	if v := header.Get(key); v != "" {
		return v
	}
	return fallback
}

// redact removes the bearer token from any string that reaches the user.
func redact(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[REDACTED]")
}
