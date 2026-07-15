package figma

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	requestTimeout             = 30 * time.Second
	maxErrorResponseBodyBytes  = 64 << 10
	maxParsedResponseBodyBytes = 32 << 20
)

var defaultHTTPClient = &http.Client{Timeout: requestTimeout}

func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	resp, err := s.doRequest(ctx, token, method, path, query, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readBoundedResponse(resp.Body, maxParsedResponseBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("figma: read response: %w", err)
	}
	return body, nil
}

func (s *Service) callAndEmit(ctx context.Context, token, method, path string, query url.Values, payload any) error {
	resp, err := s.doRequest(ctx, token, method, path, query, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return s.emitStream(resp.Body)
}

func (s *Service) doRequest(ctx context.Context, token, method, path string, query url.Values, payload any) (*http.Response, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	endpoint := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var requestBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("figma: encode request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return nil, fmt.Errorf("figma: build request: %w", err)
	}
	req.Header.Set("X-Figma-Token", token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	hc := s.HC
	if hc == nil {
		hc = defaultHTTPClient
	}
	hc = withRedirectPolicy(hc, sameOriginRedirect)
	resp, err := hc.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("figma: %s %s: %w", method, path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBodyBytes))
		if readErr != nil {
			return nil, fmt.Errorf("figma: read error response: %w", readErr)
		}
		return nil, figmaAPIError(resp.StatusCode, resp.Header.Get("Retry-After"), body, token)
	}
	return resp, nil
}

func readBoundedResponse(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
}

func withRedirectPolicy(client *http.Client, policy func(*http.Request, []*http.Request) error) *http.Client {
	clone := *client
	previous := clone.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := policy(req, via); err != nil {
			return err
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &clone
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	original := via[0].URL
	if !strings.EqualFold(req.URL.Scheme, original.Scheme) || !strings.EqualFold(req.URL.Host, original.Host) {
		return fmt.Errorf("figma API redirect changed origin")
	}
	return nil
}

type apiError struct {
	Status     int
	Message    string
	RetryAfter string
}

func figmaAPIError(status int, retryAfter string, body []byte, token string) error {
	var envelope struct {
		Err     string `json:"err"`
		Message string `json:"message"`
	}
	message := ""
	if err := json.Unmarshal(body, &envelope); err == nil {
		message = envelope.Err
		if message == "" {
			message = envelope.Message
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	message = redactSecret(message, token)
	return &apiError{Status: status, Message: message, RetryAfter: retryAfter}
}

func redactSecret(value, secret string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}

func (e *apiError) Error() string {
	if e.Status == http.StatusTooManyRequests && e.RetryAfter != "" {
		return fmt.Sprintf("figma API error: %s (HTTP %d, retry after %s seconds)", e.Message, e.Status, e.RetryAfter)
	}
	return fmt.Sprintf("figma API error: %s (HTTP %d)", e.Message, e.Status)
}

func isCredentialRejected(err error) bool {
	var providerError *apiError
	if !errors.As(err, &providerError) {
		return false
	}
	if providerError.Status == http.StatusUnauthorized {
		return true
	}
	if providerError.Status != http.StatusForbidden {
		return false
	}
	message := strings.ToLower(providerError.Message)
	authorizationMessages := []string{
		"scope",
		"permission",
		"entitlement",
		"plan",
		"seat",
		"not authorized for",
		"not authorised for",
	}
	for _, candidate := range authorizationMessages {
		if strings.Contains(message, candidate) {
			return false
		}
	}
	credentialMessages := []string{
		"invalid token",
		"invalid access token",
		"access token is invalid",
		"invalid personal access token",
		"personal access token is invalid",
		"token expired",
		"expired token",
		"token has expired",
		"personal access token has expired",
	}
	for _, candidate := range credentialMessages {
		if strings.Contains(message, candidate) {
			return true
		}
	}
	return false
}
