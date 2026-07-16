package drive

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
// 429/5xx. Length bounds the retry count. Non-GET requests are never retried: a
// mutating call may have applied even on a 5xx, and re-sending would double the
// side effect.
var retryBackoffs = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// driveParams returns the base query every files call carries. supportsAllDrives
// keeps operations working after a user moves an app-created file into a shared
// drive (design 303 §shared drive 兼容透传).
func driveParams() url.Values {
	q := url.Values{}
	q.Set("supportsAllDrives", "true")
	return q
}

// call performs one Drive API JSON request with Bearer auth and returns the
// (validated) response body. Non-2xx surfaces the body's error message; 401/403
// additionally carry the missing-scope hint. GET is retried on 429/5xx.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := s.base() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payloadBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("drive: encode request: %w", err)
		}
		payloadBytes = b
	}
	for attempt := 0; ; attempt++ {
		status, _, body, err := s.doRequest(ctx, token, method, endpoint, path, "application/json", payloadBytes)
		if err != nil {
			return nil, err
		}
		if method == http.MethodGet && attempt < len(retryBackoffs) && retryableGET(status) {
			s.pause(retryBackoffs[attempt])
			continue
		}
		if status < 200 || status > 299 {
			return nil, s.apiError(status, path, body)
		}
		return body, nil
	}
}

// callRaw performs a request whose success body is opaque bytes (blob download,
// Workspace export) rather than JSON. Error bodies are still parsed as Drive
// JSON errors. A 404 carries the drive.file visibility hint.
func (s *Service) callRaw(ctx context.Context, token, method, endpoint, path string, query url.Values) ([]byte, error) {
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	status, _, body, err := s.doRequest(ctx, token, method, endpoint, path, "", nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, s.apiError(status, path, body)
	}
	return body, nil
}

// doRequest performs a single HTTP round trip and returns status, response
// headers, and body. contentType is set on the request only when a payload is
// present.
func (s *Service) doRequest(ctx context.Context, token, method, endpoint, path, contentType string, payload []byte) (int, http.Header, []byte, error) {
	var reqBody io.Reader
	if len(payload) > 0 {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("drive: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(payload) > 0 && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("drive: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("drive: read response: %w", err)
	}
	return resp.StatusCode, resp.Header, body, nil
}

// apiError builds the surfaced error for a non-2xx status, attaching the
// scope hint on 401/403 and the drive.file visibility hint on 404, and
// classifying credential rejection for the execution engine.
func (s *Service) apiError(status int, path string, body []byte) error {
	hint := ""
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		hint = scopeHint
	case http.StatusNotFound:
		hint = visibilityHint
	}
	apiErr := fmt.Errorf("drive API error (HTTP %d): %s%s", status, apiMessage(body), hint)
	return classifyCredentialError(status, body, apiErr)
}

// retryableGET reports whether a GET response warrants an automatic retry.
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

// emit writes a provider JSON response to stdout. It refuses to write bytes that
// are not strictly valid JSON so --json output is always parseable.
func (s *Service) emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return fmt.Errorf("drive: provider returned invalid JSON")
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitJSON marshals a synthesized value to stdout.
func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("drive: encode output: %w", err)
	}
	return s.emit(body)
}

// apiMessage extracts Google's error message from an error body, falling back to
// the raw body.
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
