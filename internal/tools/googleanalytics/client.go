package googleanalytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status
// (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// retryBackoffs are the delays before each automatic GET retry on a transient
// 429/5xx. Length bounds the retry count.
var retryBackoffs = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// call performs one GA API request with Bearer auth against the given base
// (Data or Admin host). Non-2xx surfaces the body's error message; 401/403
// additionally carry the missing-scope hint.
//
// GET requests (idempotent) are retried up to len(retryBackoffs) times on a
// 429/5xx status. POST requests are never auto-retried: report runs are
// read-only but quota-metered, and blind retries would double-spend the
// property's report quota on a provider-side failure.
func (s *Service) call(ctx context.Context, token, method, base, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payloadBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("google-analytics: encode request: %v", err), err: err}
		}
		payloadBytes = b
	}
	for attempt := 0; ; attempt++ {
		status, body, err := s.doRequest(ctx, token, method, endpoint, path, payloadBytes)
		if err != nil {
			return nil, err
		}
		if method == http.MethodGet && attempt < len(retryBackoffs) && (status == http.StatusTooManyRequests || status >= 500) {
			s.pause(retryBackoffs[attempt])
			continue
		}
		if status < 200 || status > 299 {
			hint := ""
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				hint = scopeHint
			}
			raw := fmt.Errorf("google-analytics API error (HTTP %d): %s%s", status, apiMessage(body), hint)
			classified := classifyCredentialError(status, body, raw)
			return nil, &apiError{msg: classified.Error(), status: status, err: classified}
		}
		if len(bytes.TrimSpace(body)) > 0 && !json.Valid(body) {
			body = sanitizeJSON(body)
		}
		return body, nil
	}
}

// doRequest performs a single HTTP round trip and returns status + body.
func (s *Service) doRequest(ctx context.Context, token, method, endpoint, path string, payload []byte) (int, []byte, error) {
	var reqBody io.Reader
	if len(payload) > 0 {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return 0, nil, &apiError{msg: fmt.Sprintf("google-analytics: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, &apiError{msg: fmt.Sprintf("google-analytics: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, &apiError{msg: fmt.Sprintf("google-analytics: read response: %v", err), err: err}
	}
	return resp.StatusCode, body, nil
}

// pause sleeps for the retry backoff; tests inject a recorder via s.sleep.
func (s *Service) pause(d time.Duration) {
	if s.sleep != nil {
		s.sleep(d)
		return
	}
	time.Sleep(d)
}

// sanitizeJSON escapes raw control characters (bytes < 0x20) that appear
// inside JSON string literals — display names occasionally carry them —
// leaving everything outside strings untouched.
func sanitizeJSON(body []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(body))
	inString, escaped := false, false
	for _, c := range body {
		switch {
		case escaped:
			escaped = false
			out.WriteByte(c)
		case inString && c == '\\':
			escaped = true
			out.WriteByte(c)
		case c == '"':
			inString = !inString
			out.WriteByte(c)
		case inString && c < 0x20:
			fmt.Fprintf(&out, `\u%04x`, c)
		default:
			out.WriteByte(c)
		}
	}
	return out.Bytes()
}

// emit writes a provider JSON response to stdout. It refuses to write bytes
// that are not strictly valid JSON so --json output is always parseable.
func (s *Service) emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		return &apiError{msg: "google-analytics: provider returned invalid JSON"}
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("google-analytics: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// apiMessage extracts Google's error message from an error body, falling back
// to the raw body.
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
