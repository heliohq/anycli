package sendgrid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// apiResponse carries the parts of a successful SendGrid response callers need.
// mail send reads Header for X-Message-Id; every other command reads Body.
type apiResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

// do performs one SendGrid API request with Bearer auth. A 401 marks the
// credential rejected (dead/revoked key); any other non-2xx is a plain error
// carrying SendGrid's error message(s). A 2xx (including the empty-body 202
// from mail send) returns the response for the caller to interpret.
func (s *Service) do(ctx context.Context, token, region, method, path string, query url.Values, payload any) (*apiResponse, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("sendgrid: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL(region) + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("sendgrid: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("sendgrid: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sendgrid: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("sendgrid API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		// Only a genuinely dead/revoked key (401) rejects the credential. A 403
		// is a normal scope / verified-sender error — the key is valid, it just
		// lacks permission for this operation (mirrors the connect-time
		// 401-vs-403 split, DESIGN §4). Everything else is a plain runtime error.
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return &apiResponse{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// call performs a request and returns the raw response body (the passthrough
// path shared by every command except mail send).
func (s *Service) call(ctx context.Context, token, region, method, path string, query url.Values, payload any) ([]byte, error) {
	resp, err := s.do(ctx, token, region, method, path, query, payload)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (mail-send acceptance receipt) and
// writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("sendgrid: encode output: %w", err)
	}
	return s.emit(body)
}

// apiMessage extracts SendGrid's error message(s) from an error body. v3 error
// bodies are {"errors":[{"field","message"}]}; the messages are joined. Falls
// back to the raw body when the shape does not match.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && len(e.Errors) > 0 {
		messages := make([]string, 0, len(e.Errors))
		for _, item := range e.Errors {
			if item.Message != "" {
				messages = append(messages, item.Message)
			}
		}
		if len(messages) > 0 {
			return strings.Join(messages, "; ")
		}
	}
	return string(body)
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("sendgrid: --%s is not valid JSON: %w", name, err)
	}
	return v, nil
}

// intToString renders an int as a base-10 query value.
func intToString(n int) string {
	return strconv.Itoa(n)
}
