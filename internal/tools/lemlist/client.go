package lemlist

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

// usageError is a parameter / usage error (missing required flag, bad input).
// It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Lemlist non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejection classification still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Lemlist API request with HTTP Basic auth (empty username,
// API key as password) and returns the raw response body. A 401 marks the
// credential rejected; any other non-2xx is an apiError carrying Lemlist's
// message plus the HTTP status.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("lemlist: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lemlist: build request: %v", err), err: err}
	}
	// Lemlist documents Basic auth with an EMPTY username and the API key as the
	// password ("THERE IS A COLON before your API key"): base64(":"+key).
	req.SetBasicAuth("", key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lemlist: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("lemlist: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("lemlist API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: execution.RejectCredential(raw)}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Lemlist's error message from an error body, falling back
// to the raw body. Lemlist errors variously carry "message" or "error".
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != "":
			return e.Error
		}
	}
	if len(body) == 0 {
		return "(empty response body)"
	}
	return string(body)
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. A malformed value is a usage error.
func decodeJSONFlag(name, raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("lemlist: --%s is not a valid JSON object: %v", name, err)}
	}
	return v, nil
}
