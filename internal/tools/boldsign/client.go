package boldsign

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or malformed input. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a BoldSign non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// baseURL returns the configured base with any trailing slash trimmed.
func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// call performs one BoldSign API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx becomes
// an apiError carrying BoldSign's message and the HTTP status. A JSON payload
// is sent as application/json.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("boldsign: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("boldsign: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("boldsign: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("boldsign: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, s.apiErrorFromResponse(resp.StatusCode, body)
	}
	return body, nil
}

// download performs a GET that returns raw file bytes (PDF). Unlike call it does
// not parse the body as JSON on success; on a non-2xx it still extracts
// BoldSign's JSON error message.
func (s *Service) download(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("boldsign: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("boldsign: GET %s: %v", path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("boldsign: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, s.apiErrorFromResponse(resp.StatusCode, body)
	}
	return body, nil
}

// apiErrorFromResponse builds the apiError for a non-2xx response, marking a 401
// as an explicit credential rejection so the engine can invalidate the token.
func (s *Service) apiErrorFromResponse(status int, body []byte) error {
	raw := fmt.Errorf("boldsign API error (HTTP %d): %s", status, apiMessage(body))
	if status == http.StatusUnauthorized {
		return &apiError{msg: raw.Error(), status: status, err: execution.RejectCredential(raw)}
	}
	return &apiError{msg: raw.Error(), status: status, err: raw}
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (receipts) and writes it to stdout (+
// newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("boldsign: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// apiMessage extracts BoldSign's error text from an error body, falling back to
// the raw body. BoldSign returns errors under a few different shapes across its
// surface (message/error/title, or a validation errors map), so this probes the
// common ones.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Title   string `json:"title"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != "":
			return e.Error
		case e.Detail != "":
			return e.Detail
		case e.Title != "":
			return e.Title
		}
	}
	return strings.TrimSpace(string(body))
}
