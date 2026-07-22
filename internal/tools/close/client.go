package close

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a missing required flag, an invalid
// JSON body, or a bad argument. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Close non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejection sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

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

// call performs one Close API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying Close's `error`/`field-errors` message and the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("close: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("close: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("close: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("close: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("close API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		cause := raw
		if resp.StatusCode == http.StatusUnauthorized {
			cause = execution.RejectCredential(raw)
		}
		return nil, &apiError{msg: cause.Error(), status: resp.StatusCode, err: cause}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline). An
// empty body (some deletes answer 200 with no content) emits a blank line.
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Close's error text from an error body, combining `error`
// with any `field-errors`, and falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error       string          `json:"error"`
		FieldErrors json.RawMessage `json:"field-errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Error != "" && len(e.FieldErrors) > 0:
			return e.Error + " (field-errors: " + string(e.FieldErrors) + ")"
		case e.Error != "":
			return e.Error
		case len(e.FieldErrors) > 0:
			return "field-errors: " + string(e.FieldErrors)
		}
	}
	return string(body)
}

// readData resolves a --data flag value into a decoded JSON body. The value is
// either a JSON literal or "@path" to read the JSON from a file. An invalid or
// missing value is a usage error (exit 2), never an API call.
func readData(flag, raw string) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, &usageError{msg: fmt.Sprintf("close: --%s is required (JSON object or @file.json)", flag)}
	}
	src := []byte(raw)
	if strings.HasPrefix(raw, "@") {
		b, err := os.ReadFile(raw[1:])
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("close: --%s: read file: %v", flag, err)}
		}
		src = b
	}
	var v any
	if err := json.Unmarshal(src, &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("close: --%s is not valid JSON: %v", flag, err)}
	}
	return v, nil
}
