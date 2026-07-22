package copper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, invalid JSON, or an unknown subcommand. It maps to
// exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Copper non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for *credentialRejectedError still resolves through it.
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

// call performs one Copper API request with Bearer auth and returns the raw
// response body. A non-2xx surfaces Copper's error message as an apiError
// carrying the HTTP status; a 401/403 additionally marks the credential
// rejected for the token gateway. A transport failure is an apiError with
// status 0. Copper's OAuth path uses ONLY the Bearer header + JSON content type
// — never the legacy X-PW-* trio.
func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("copper: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL()+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("copper: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	// Copper requires Content-Type: application/json on every OAuth request,
	// including the bodyless GET/DELETE reads (per the OAuth quickstart).
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("copper: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("copper: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("copper API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401/403 as an explicit credential rejection
// so the token gateway can invalidate the stored token. Other non-2xx statuses
// (404, 422, 429, 5xx) stay ordinary runtime failures.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return execution.RejectCredential(err)
	}
	return err
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Copper's error message from an error body, falling back
// to the raw body. Copper returns errors as {"status":N,"error":"..."} or a
// {"message":"..."} shape depending on the failure; both are surfaced.
func apiMessage(body []byte) string {
	var e struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != nil:
			if str, ok := e.Error.(string); ok && str != "" {
				return str
			}
			if b, mErr := json.Marshal(e.Error); mErr == nil {
				return string(b)
			}
		}
	}
	return strings.TrimSpace(string(body))
}

// decodeJSONBody validates a raw --json-body flag value and returns the decoded
// value for use as (or merged into) a request body. An invalid value is a usage
// error (exit 2), not an API error.
func decodeJSONBody(raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--json-body is not a valid JSON object: %v", err)}
	}
	return v, nil
}
