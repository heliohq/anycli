package gumroad

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// apiError is a Gumroad runtime/API failure carrying the HTTP status (0 when
// the failure is pre-request, e.g. a missing token) and Gumroad's own message.
// Execute maps *apiError to exit 1; every other error is a usage error (exit 2).
type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("gumroad API error (HTTP %d): %s", e.status, e.message)
	}
	return e.message
}

// call performs one Gumroad v2 request with Bearer auth and returns the raw
// response body on success. GET requests carry params in the query string;
// mutating requests carry them as a form-urlencoded body (Gumroad's Rails API
// reads either into params, but form bodies match the documented curl style).
//
// Gumroad's success dialect: a request can fail with HTTP 200 and
// "success":false, so success is decided by (2xx AND success != false). A 401
// rejects the credential; any other failure is a plain runtime error carrying
// Gumroad's message.
func (s *Service) call(ctx context.Context, token, method, path string, params url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	var bodyReader io.Reader
	form := false
	if len(params) > 0 {
		if method == http.MethodGet {
			requestURL += "?" + params.Encode()
		} else {
			bodyReader = strings.NewReader(params.Encode())
			form = true
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return nil, &apiError{message: fmt.Sprintf("build request: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if form {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{message: fmt.Sprintf("%s %s: %v", method, path, err)}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{status: resp.StatusCode, message: fmt.Sprintf("read response: %v", err)}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 || bodySuccessIsFalse(body) {
		apiErr := &apiError{status: resp.StatusCode, message: apiMessage(body)}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}

// bodySuccessIsFalse reports whether a JSON body carries "success":false. A body
// with no success field (or a non-JSON body) is not treated as a failure — the
// HTTP status already gates those.
func bodySuccessIsFalse(body []byte) bool {
	var probe struct {
		Success *bool `json:"success"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return probe.Success != nil && !*probe.Success
}

// apiMessage extracts Gumroad's error message from a response body, falling
// back to the raw body when absent.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		return e.Message
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "request failed"
	}
	return trimmed
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
