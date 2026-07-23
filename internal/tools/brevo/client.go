package brevo

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Brevo non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". display is the full
// human-readable line (returned by Error()); code and message carry Brevo's own
// error body fields for the structured --json envelope; status is the HTTP
// status (0 for transport failures). It wraps the underlying cause so errors.As
// for *credentialRejectedError still resolves through it.
type apiError struct {
	display string
	code    string
	message string
	status  int
	err     error
}

func (e *apiError) Error() string { return e.display }
func (e *apiError) Unwrap() error { return e.err }

// transportError builds an apiError for a local/transport failure (no Brevo
// error body): display and message are the same string, code/status are empty.
func transportError(msg string, cause error) *apiError {
	return &apiError{display: msg, message: msg, err: cause}
}

// call performs one Brevo API request with api-key auth and returns the raw
// response body. A 401 (or an "unauthorized" error code) marks the credential
// rejected; any other non-2xx surfaces Brevo's code/message as an apiError
// carrying the HTTP status.
func (s *Service) call(ctx context.Context, apiKey, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, transportError(fmt.Sprintf("brevo: encode request: %v", err), err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, transportError(fmt.Sprintf("brevo: build request: %v", err), err)
	}
	req.Header.Set("api-key", apiKey)
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
		return nil, transportError(fmt.Sprintf("brevo: %s %s: %v", method, path, err), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, transportError(fmt.Sprintf("brevo: read response: %v", err), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code, msg := parseAPIError(body)
		raw := fmt.Errorf("brevo API error (HTTP %d): %s", resp.StatusCode, apiMessage(code, msg, body))
		classified := classifyCredentialError(resp.StatusCode, code, raw)
		return nil, &apiError{display: classified.Error(), code: code, message: msg, status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks 401 responses (or an "unauthorized" error code)
// as an explicit credential rejection so the engine can invalidate the stored
// key; every other failure leaves the credential untouched.
func classifyCredentialError(status int, code string, err error) error {
	if status == http.StatusUnauthorized || code == "unauthorized" {
		return execution.RejectCredential(err)
	}
	return err
}

// parseAPIError extracts Brevo's error code/message from a non-2xx body. Brevo
// error bodies are {"code":"...","message":"..."}.
func parseAPIError(body []byte) (code, message string) {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		return e.Code, e.Message
	}
	return "", ""
}

// apiMessage renders Brevo's code/message for a human-readable error line,
// falling back to the raw body when neither is present.
func apiMessage(code, message string, body []byte) string {
	switch {
	case code != "" && message != "":
		return code + ": " + message
	case message != "":
		return message
	case code != "":
		return code
	default:
		return string(body)
	}
}

// emit writes the provider's JSON response to stdout verbatim (+ newline). An
// empty body (e.g. a 204 No Content from update/delete) prints nothing.
func (s *Service) emit(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. A parse failure is a usageError (exit 2).
func decodeJSONObjectFlag(name, raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("brevo: --%s is not a valid JSON object: %v", name, err)}
	}
	return v, nil
}

// decodeJSONArrayFlag validates a raw-JSON array flag and returns it for
// passthrough. A parse failure is a usageError (exit 2).
func decodeJSONArrayFlag(name, raw string) ([]any, error) {
	var v []any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("brevo: --%s is not a valid JSON array: %v", name, err)}
	}
	return v, nil
}

// emailEntries builds Brevo recipient objects ([{email}]) from plain email
// addresses.
func emailEntries(emails []string) []map[string]any {
	out := make([]map[string]any, 0, len(emails))
	for _, e := range emails {
		out = append(out, map[string]any{"email": e})
	}
	return out
}
