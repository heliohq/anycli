package hootsuite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// apiError is a Hootsuite non-2xx response. status is the HTTP status; code is
// Hootsuite's numeric error code (as a string, e.g. "1005") when present;
// message is the provider's human-readable text. Runtime/API errors → exit 1.
type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) Error() string {
	if e.code != "" {
		return fmt.Sprintf("hootsuite API error (HTTP %d, code %s): %s", e.status, e.code, e.message)
	}
	return fmt.Sprintf("hootsuite API error (HTTP %d): %s", e.status, e.message)
}

// call performs one Hootsuite API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying Hootsuite's numeric code and message.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("hootsuite: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("hootsuite: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("hootsuite: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("hootsuite: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := parseAPIError(resp.StatusCode, body)
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}

// parseAPIError extracts Hootsuite's first errors[] code/message, falling back
// to a bare {"message":…} body and finally the raw body / status text.
func parseAPIError(status int, body []byte) *apiError {
	var env struct {
		Errors []struct {
			Code    json.Number `json:"code"`
			Message string      `json:"message"`
		} `json:"errors"`
		Message string `json:"message"`
	}
	ae := &apiError{status: status, message: strings.TrimSpace(string(body))}
	if err := json.Unmarshal(body, &env); err == nil {
		switch {
		case len(env.Errors) > 0:
			ae.code = env.Errors[0].Code.String()
			if env.Errors[0].Message != "" {
				ae.message = env.Errors[0].Message
			}
		case env.Message != "":
			ae.message = env.Message
		}
	}
	if ae.message == "" {
		ae.message = http.StatusText(status)
	}
	return ae
}

// emit unwraps Hootsuite's {"data": …} envelope and writes the inner value to
// stdout (+ newline). Bodies without a data key, or empty bodies (e.g. a 204
// DELETE), are handled gracefully.
func (s *Service) emit(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err == nil && len(env.Data) > 0 {
		return s.write(env.Data)
	}
	return s.write(body)
}

func (s *Service) write(b []byte) error {
	if _, err := s.stdout().Write(b); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// validateUTC returns a usageError unless value is a valid RFC3339 timestamp
// ending in "Z" (Hootsuite rejects offset/ambiguous timestamps; fail fast
// before the request).
func validateUTC(flag, value string) error {
	if !strings.HasSuffix(value, "Z") {
		return &usageError{msg: fmt.Sprintf("--%s must be a UTC ISO-8601 timestamp ending in Z (got %q)", flag, value)}
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return &usageError{msg: fmt.Sprintf("--%s is not a valid RFC3339 timestamp: %v", flag, err)}
	}
	return nil
}
