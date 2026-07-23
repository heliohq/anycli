package klaviyo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// call performs one Klaviyo API request and returns the raw response body.
//
// Auth header is selected from the injected credential's shape: an OAuth bearer
// access token uses "Authorization: Bearer <tok>", while a Klaviyo private API
// key (documented "pk_" prefix) uses "Authorization: Klaviyo-API-Key <tok>".
// This is keyed on Klaviyo's own documented key format, not a silent fallback,
// and lets the L2 dev harness run against a self-serve private key before the
// OAuth app exists.
//
// Every request carries the pinned `revision` header and Accept:
// application/json; writes add Content-Type: application/json. A non-2xx
// surfaces the JSON:API errors array as an apiError carrying the HTTP status; a
// 401 additionally marks the credential rejected. A transport failure is an
// apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("klaviyo: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("klaviyo: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("revision", apiRevision)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("klaviyo: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("klaviyo: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("klaviyo API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			raw = execution.RejectCredential(raw)
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// authHeader picks the Authorization value from the credential shape. A Klaviyo
// private API key carries the documented "pk_" prefix and uses the
// Klaviyo-API-Key scheme; anything else is treated as an OAuth bearer token.
func authHeader(token string) string {
	if strings.HasPrefix(token, "pk_") {
		return "Klaviyo-API-Key " + token
	}
	return "Bearer " + token
}

// apiMessage extracts the first error's code/title/detail from a JSON:API
// `errors` array, falling back to the raw body.
func apiMessage(body []byte) string {
	var env struct {
		Errors []struct {
			Code   string `json:"code"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err == nil && len(env.Errors) > 0 {
		e := env.Errors[0]
		parts := make([]string, 0, 3)
		for _, p := range []string{e.Code, e.Title, e.Detail} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ": ")
		}
	}
	return string(body)
}

// emit writes the provider's JSON:API response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
