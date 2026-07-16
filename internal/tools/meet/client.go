package meet

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

// retryBackoffs are the delays before each automatic GET retry on a transient
// 429/5xx. Length bounds the retry count. Non-GET requests are never retried:
// re-sending a POST/PATCH may double an applied side effect.
var retryBackoffs = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// call performs one Meet API v2 request against the standard base.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	return s.callBase(ctx, s.base(), token, method, path, query, payload)
}

// callBase performs one Meet API request against the given base with Bearer
// auth. Non-2xx surfaces the body's error message; 401/403 additionally carry
// the missing-scope hint and (for auth failures) mark the credential rejected.
// GET requests are retried on a transient 429/5xx.
func (s *Service) callBase(ctx context.Context, base, token, method, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payloadBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("meet: encode request: %w", err)
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
			apiErr := fmt.Errorf("meet API error (HTTP %d): %s%s", status, apiMessage(body), hint)
			return nil, classifyCredentialError(status, body, apiErr)
		}
		// Transcript entries carry free-form speech; escape any raw control
		// characters that would break --json pass-through before use.
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
		return 0, nil, fmt.Errorf("meet: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("meet: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("meet: read response: %w", err)
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
// inside JSON string literals, leaving everything outside strings untouched.
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
		return fmt.Errorf("meet: provider returned invalid JSON")
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("meet: encode output: %w", err)
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
