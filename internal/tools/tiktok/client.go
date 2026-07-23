package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxErrorBodyBytes = 8 << 10

// envelope is TikTok's uniform response wrapper: every v2 endpoint returns a
// `data` object and an `error` object. A successful call carries
// error.code == "ok"; any other code is a business error even on HTTP 200,
// so it must be inspected alongside the status code (the Slack `ok:false`
// dialect).
type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error apiErrorBody    `json:"error"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	LogID   string `json:"log_id"`
}

// call performs a TikTok v2 request and returns the unwrapped `data` payload.
// query is appended to the URL; payload, when non-nil, is JSON-encoded as the
// request body. Both HTTP-level failures and in-envelope error codes are
// surfaced as an error.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) (json.RawMessage, error) {
	var requestBody io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("tiktok: encode request: %w", err)
		}
		requestBody = bytes.NewReader(body)
	}

	requestURL := s.apiBase() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("tiktok: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("tiktok: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tiktok: read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newHTTPError(resp.StatusCode, body, token)
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("tiktok: decode response: %w", err)
	}
	if code := env.Error.Code; code != "" && code != "ok" {
		return nil, newEnvelopeError(resp.StatusCode, env.Error, token)
	}
	return env.Data, nil
}

// emit writes a raw JSON payload followed by a newline.
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitField unwraps a named field from the data object and emits it, falling
// back to the whole data object when the field is absent.
func (s *Service) emitField(data json.RawMessage, field string) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err == nil {
		if v, ok := m[field]; ok {
			return s.emit(v)
		}
	}
	return s.emit(data)
}

func redact(raw, token string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "empty response body"
	}
	if token != "" {
		raw = strings.ReplaceAll(raw, token, "[REDACTED]")
	}
	if len(raw) > maxErrorBodyBytes {
		raw = raw[:maxErrorBodyBytes] + "…"
	}
	return raw
}
