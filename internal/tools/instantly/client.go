package instantly

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
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, or invalid JSON. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Instantly non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
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

// call performs one Instantly API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying Instantly's message and HTTP status. payload, when non-nil,
// is JSON-encoded as the request body.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("instantly: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("instantly: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("instantly: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("instantly: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("instantly API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: classifyCredentialError(resp.StatusCode, raw)}
	}
	return body, nil
}

// classifyCredentialError marks only a 401 as an explicit credential rejection.
// 402 (plan gate), 403 (scope), 404, 429, and 5xx leave the credential valid so
// the token gateway does not invalidate a working key on a transient or
// authorization-shaped failure.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// get performs a GET and emits the raw response. query may be nil.
func (s *Service) get(cmd *cobra.Command, token, path string, query url.Values) error {
	body, err := s.call(cmd.Context(), token, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// send performs a write request (POST/PATCH/DELETE) and emits the raw response.
// payload may be nil for a no-body action.
func (s *Service) send(cmd *cobra.Command, token, method, path string, payload any) error {
	body, err := s.call(cmd.Context(), token, method, path, nil, payload)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// apiMessage extracts Instantly's error message from an error body
// ({statusCode, error, message}), falling back to the error field, then the
// raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "" && e.Error != "":
			return e.Error + ": " + e.Message
		case e.Message != "":
			return e.Message
		case e.Error != "":
			return e.Error
		}
	}
	return strings.TrimSpace(string(body))
}

// decodeDataFlag parses a raw-JSON --data flag into a body map used as the base
// for a request body; explicit flags override its keys. An empty value yields
// an empty map.
func decodeDataFlag(raw string) (map[string]any, error) {
	m := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("instantly: --data is not a valid JSON object: %v", err)}
	}
	return m, nil
}
