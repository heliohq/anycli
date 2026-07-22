package wise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a missing required flag, an illegal
// flag combination, or a bad value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Wise non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status
// (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejected sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Wise API request with Bearer auth and returns the raw
// response body. A 401/403 marks the credential rejected (personal tokens that
// are revoked or lack the required scope); any other non-2xx surfaces Wise's
// error message as an apiError carrying the HTTP status; a transport failure is
// an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("wise: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("wise: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("wise: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("wise: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("wise API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401/403 as a rejected credential so the host
// can prompt a reconnect; every other status stays a plain API error.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return execution.RejectCredential(err)
	}
	return err
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// apiMessage extracts Wise's error message from an error body. Wise returns
// either {"errors":[{"code":..,"message":..}]} or a top-level
// {"error":..,"message":..}; fall back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if len(e.Errors) > 0 && e.Errors[0].Message != "" {
			return e.Errors[0].Message
		}
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != "":
			return e.Error
		}
	}
	return string(body)
}
