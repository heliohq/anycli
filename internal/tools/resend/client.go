package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad value, or invalid JSON. It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Resend non-2xx response or a transport
// failure. It maps to exit code 1. status is the HTTP status (0 for
// transport/network failures). It wraps the underlying cause so errors.As for
// *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

func asAPIError(err error, target **apiError) bool { return errors.As(err, target) }

// credentialRejectNames is the closed set of Resend error `name` values that
// mean the resolved credential is genuinely bad or absent. Everything else —
// restricted_api_key (a live sending-only key), validation_error (including the
// default unverified-domain 403), rate limits, unparseable bodies — is a plain
// passthrough error, never a credential reject. See package doc.
var credentialRejectNames = map[string]bool{
	"invalid_api_key": true,
	"missing_api_key": true,
}

// resendError is the Resend error body shape:
// {"statusCode":<n>,"name":"<slug>","message":"<text>"}.
type resendError struct {
	StatusCode int    `json:"statusCode"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

// call performs one Resend API request with Bearer auth and an explicit
// User-Agent, returning the raw response body. Credential rejection is decided
// by the parsed error `name`, never the raw status (see package doc).
func (s *Service) call(ctx context.Context, key, method, path string, payload any, headers map[string]string) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("resend: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.baseURL()+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("resend: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("resend: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("resend: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, s.apiError(resp.StatusCode, body)
	}
	return body, nil
}

// apiError builds a typed API error, promoting it to a credential rejection
// only when the parsed error `name` is in credentialRejectNames.
func (s *Service) apiError(status int, body []byte) error {
	name, message := parseResendError(body)
	text := message
	if text == "" {
		text = string(body)
	}
	apiErr := &apiError{
		msg:    fmt.Sprintf("resend API error (HTTP %d): %s", status, text),
		status: status,
	}
	if credentialRejectNames[name] {
		return execution.RejectCredential(apiErr)
	}
	return apiErr
}

// parseResendError extracts the error name and message from a Resend error
// body. A body that does not parse as the documented shape returns empty
// strings, so the caller never rejects a credential on an unrecognizable body.
func parseResendError(body []byte) (name, message string) {
	var e resendError
	if err := json.Unmarshal(body, &e); err != nil {
		return "", ""
	}
	return e.Name, e.Message
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. Invalid JSON is a usage error (exit 2).
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}
