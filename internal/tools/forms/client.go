package forms

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
// 429/5xx status. Length bounds the retry count.
var retryBackoffs = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// call performs one Forms API request against the Forms base URL.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	return s.callBase(ctx, s.base(), "forms", token, method, path, query, payload)
}

// callDrive performs one Drive API request against the Drive base URL. Used by
// the responders * verbs (permissions on the published view).
func (s *Service) callDrive(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	return s.callBase(ctx, s.driveBase(), "drive", token, method, path, query, payload)
}

// callBase performs one API request with Bearer auth. Non-2xx surfaces the
// body's error message; 401/403 additionally carry the missing-scope hint.
//
// GET requests (idempotent) are retried up to len(retryBackoffs) times on a
// 429/5xx status. Non-GET requests are never auto-retried: a POST may have been
// applied even when the response is a 5xx, and re-sending would double the side
// effect.
func (s *Service) callBase(ctx context.Context, base, label, token, method, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payloadBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("%s: encode request: %w", label, err)
		}
		payloadBytes = b
	}
	for attempt := 0; ; attempt++ {
		status, body, err := s.doRequest(ctx, label, token, method, endpoint, path, payloadBytes)
		if err != nil {
			return nil, err
		}
		if method == http.MethodGet && attempt < len(retryBackoffs) && retryableGET(status) {
			s.pause(retryBackoffs[attempt])
			continue
		}
		if status < 200 || status > 299 {
			hint := ""
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				hint = scopeHint
			}
			apiErr := fmt.Errorf("%s API error (HTTP %d): %s%s", label, status, apiMessage(body), hint)
			return nil, classifyCredentialError(status, body, apiErr)
		}
		return body, nil
	}
}

// doRequest performs a single HTTP round trip and returns status + body.
func (s *Service) doRequest(ctx context.Context, label, token, method, endpoint, path string, payload []byte) (int, []byte, error) {
	var reqBody io.Reader
	if len(payload) > 0 {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: build request: %w", label, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %s %s: %w", label, method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: read response: %w", label, err)
	}
	return resp.StatusCode, body, nil
}

// retryableGET reports whether a GET response warrants an automatic retry:
// rate limit or server failure.
func retryableGET(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// pause sleeps for the retry backoff; tests inject a recorder via s.sleep.
func (s *Service) pause(d time.Duration) {
	if s.sleep != nil {
		s.sleep(d)
		return
	}
	time.Sleep(d)
}

// emit writes a provider JSON response to stdout. It refuses to write bytes
// that are not strictly valid JSON so --json output is always parseable.
func (s *Service) emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return fmt.Errorf("forms: provider returned invalid JSON")
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("forms: encode output: %w", err)
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
