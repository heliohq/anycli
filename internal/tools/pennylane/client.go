package pennylane

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, invalid JSON, or a missing argument. It maps to exit code 2
// and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Pennylane non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classification still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Pennylane API request. Bearer auth on every call; a JSON
// payload sets Content-Type; a non-2xx surfaces the body's message (and, for
// 401/403/404, an actionable hint) as an apiError carrying the HTTP status, and
// a transport failure as an apiError with status 0. The 2xx body is returned
// verbatim for pass-through.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload []byte) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	requestURL := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pennylane: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("pennylane: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pennylane: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("pennylane API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), accessHint(resp.StatusCode))
		classified := classifyPennylaneCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// emitJSON writes the provider's JSON response to stdout verbatim.
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// accessHint returns an actionable clause for the failures an agent most often
// hits: an expired token, a missing scope, or a wrong id.
func accessHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return " (access token is invalid or expired — reconnect Pennylane)"
	case http.StatusForbidden:
		return " (token may lack the required scope for this resource)"
	case http.StatusNotFound:
		return " (check the id)"
	default:
		return ""
	}
}

// apiMessage extracts Pennylane's error message from an error body, falling
// back to the raw body. Pennylane returns { "message": "…" } or, for validation
// failures, { "errors": {…} }.
func apiMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response body"
	}
	var e struct {
		Message string          `json:"message"`
		Error   string          `json:"error"`
		Errors  json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != "":
			return e.Error
		case len(e.Errors) > 0:
			return "validation failed: " + string(e.Errors)
		}
	}
	return trimmed
}
