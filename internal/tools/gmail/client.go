package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// retryBackoffs are the delays before each automatic GET retry (issue: the
// Gmail API intermittently returns empty 2xx bodies or transient 429/5xx
// under rapid sequential calls). Length bounds the retry count.
var retryBackoffs = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// call performs one Gmail API request with Bearer auth. Non-2xx surfaces the
// body's error message; 401/403 additionally carry the missing-scope hint.
//
// GET requests (idempotent) are retried up to len(retryBackoffs) times on a
// 429/5xx status or on a 2xx response with an empty body (a GET here always
// expects a body). Non-GET requests are never auto-retried: a POST may have
// been applied even when the response is a 5xx, and re-sending would double
// the side effect.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := s.base() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payloadBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("gmail: encode request: %w", err)
		}
		payloadBytes = b
	}
	for attempt := 0; ; attempt++ {
		status, body, err := s.doRequest(ctx, token, method, endpoint, path, payloadBytes)
		if err != nil {
			return nil, err
		}
		if method == http.MethodGet && attempt < len(retryBackoffs) && retryableGET(status, body) {
			s.pause(retryBackoffs[attempt])
			continue
		}
		if status < 200 || status > 299 {
			hint := ""
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				hint = scopeHint
			}
			apiErr := fmt.Errorf("gmail API error (HTTP %d): %s%s", status, apiMessage(body), hint)
			return nil, classifyCredentialError(status, body, apiErr)
		}
		if method == http.MethodGet && len(bytes.TrimSpace(body)) == 0 {
			return nil, fmt.Errorf("gmail: GET %s: empty response from API (HTTP %d) after %d attempts", path, status, attempt+1)
		}
		// Gmail has been observed returning bodies with raw control
		// characters inside string values (e.g. Subject headers of GitHub
		// notification mail) — invalid JSON that breaks both --json
		// pass-through and view parsing. Escape them before use.
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
		return 0, nil, fmt.Errorf("gmail: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gmail: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("gmail: read response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// retryableGET reports whether a GET response warrants an automatic retry:
// rate limit, server failure, or a 2xx with an empty body where one is
// expected.
func retryableGET(status int, body []byte) bool {
	if status == http.StatusTooManyRequests || status >= 500 {
		return true
	}
	return status >= 200 && status <= 299 && len(bytes.TrimSpace(body)) == 0
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
// The result of escaping an otherwise-well-formed body is strictly valid
// JSON; a body that is broken in other ways stays invalid and is rejected by
// the caller's decode or by emit.
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
	if !json.Valid(body) {
		return fmt.Errorf("gmail: provider returned invalid JSON")
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("gmail: encode output: %w", err)
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

// decodeBase64URL decodes Gmail body/attachment data, which is URL-safe
// base64 with or without padding depending on the field.
func decodeBase64URL(data string) ([]byte, error) {
	if b, err := base64.URLEncoding.DecodeString(data); err == nil {
		return b, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("gmail: decode base64url data: %w", err)
	}
	return b, nil
}
