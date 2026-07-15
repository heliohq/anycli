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
)

// call performs one Gmail API request with Bearer auth. Non-2xx surfaces the
// body's error message; 401/403 additionally carry the missing-scope hint.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	endpoint := s.base() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("gmail: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("gmail: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gmail: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		hint := ""
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			hint = scopeHint
		}
		apiErr := fmt.Errorf("gmail API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), hint)
		return nil, classifyCredentialError(resp.StatusCode, body, apiErr)
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(bytes.TrimSpace(body), '\n'))
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
