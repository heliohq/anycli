package x

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxErrorBodyBytes = 8 << 10

func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var requestBody io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("x: encode request: %w", err)
		}
		requestBody = bytes.NewReader(body)
	}

	requestURL := s.apiBase() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("x: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("x: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("x: read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, body, token)
	}
	return body, nil
}

func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("x: encode output: %w", err)
	}
	return s.emit(body)
}

func (s *Service) download(ctx context.Context, token, path, output string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBase()+path, nil)
	if err != nil {
		return 0, fmt.Errorf("x: build download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("x: download %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes+1))
		if readErr != nil {
			return 0, fmt.Errorf("x: read download error: %w", readErr)
		}
		return 0, newAPIError(resp.StatusCode, body, token)
	}

	dir := filepath.Dir(output)
	temp, err := os.CreateTemp(dir, ".x-download-*")
	if err != nil {
		return 0, fmt.Errorf("x: create download file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	written, copyErr := io.Copy(temp, resp.Body)
	closeErr := temp.Close()
	if copyErr != nil {
		return 0, fmt.Errorf("x: write download: %w", copyErr)
	}
	if closeErr != nil {
		return 0, fmt.Errorf("x: close download: %w", closeErr)
	}
	if err := os.Rename(tempName, output); err != nil {
		return 0, fmt.Errorf("x: finalize download: %w", err)
	}
	return written, nil
}

func newAPIError(status int, body []byte, token string) error {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		raw = "empty response body"
	}
	raw = strings.ReplaceAll(raw, token, "[REDACTED]")
	if len(raw) > maxErrorBodyBytes {
		raw = raw[:maxErrorBodyBytes] + "…"
	}

	hint := ""
	switch status {
	case http.StatusUnauthorized:
		hint = "; access token is invalid or expired — reconnect X"
	case http.StatusForbidden:
		hint = "; token may lack the required scope or account permission"
	case http.StatusTooManyRequests:
		hint = "; rate limit exceeded — retry after the provider reset window"
	}
	return fmt.Errorf("x API error (HTTP %d %s): %s%s", status, http.StatusText(status), raw, hint)
}
