package chargebee

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

// usageError is a parameter / usage error: missing required flag, bad value, or
// unknown subcommand. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Chargebee non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport failures); code is Chargebee's api_error_code when present. It
// wraps the underlying cause so errors.As for the credential-rejected marker
// still resolves through it.
type apiError struct {
	msg    string
	status int
	code   string
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Chargebee v2 request. query is appended for reads; form is
// sent as application/x-www-form-urlencoded for writes (Chargebee v2 writes are
// form-encoded, not JSON). Basic auth uses the API key as the username with an
// empty password. A 401/403 marks the credential rejected; any other non-2xx
// surfaces Chargebee's api_error_code/message.
func (s *Service) call(ctx context.Context, cfg reqConfig, method, path string, query, form url.Values) ([]byte, error) {
	requestURL := cfg.base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("chargebee: build request: %v", err), err: err}
	}
	req.SetBasicAuth(cfg.apiKey, "")
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("chargebee: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("chargebee: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code, message := apiErrorFields(respBody)
		raw := fmt.Errorf("chargebee API error (HTTP %d): %s", resp.StatusCode, message)
		cause := error(raw)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			cause = execution.RejectCredential(raw)
		}
		return nil, &apiError{msg: cause.Error(), status: resp.StatusCode, code: code, err: cause}
	}
	return respBody, nil
}

// apiErrorFields extracts Chargebee's api_error_code and message from an error
// body, falling back to the raw body for the message.
func apiErrorFields(body []byte) (code, message string) {
	var e struct {
		APIErrorCode string `json:"api_error_code"`
		ErrorCode    string `json:"error_code"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		code = e.APIErrorCode
		if code == "" {
			code = e.ErrorCode
		}
		message = e.Message
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if code != "" {
		message = code + ": " + message
	}
	return code, message
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
