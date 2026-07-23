package iterable

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
)

// usageError is a parameter / usage error: a malformed credential, missing
// required flag, bad enum, invalid JSON, or unknown subcommand. It maps to exit
// code 2 and error code "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Iterable non-2xx response, a 200 body
// carrying a non-Success code, or a transport failure. It maps to exit code 1
// and error code "api". status is the HTTP status (0 for transport failures or
// body-level code errors). It wraps the underlying cause so errors.As for the
// credential-rejected marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Iterable API request with Api-Key auth and returns the raw
// response body. Iterable reports failures two ways: a non-2xx HTTP status, and
// a 2xx body whose top-level "code" is not "Success" (its write endpoints).
// Both become an apiError; an auth failure (401, or a bad-key code) is wrapped
// as a credential rejection so the engine can invalidate the stored secret.
func (s *Service) call(ctx context.Context, cred credential, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: "iterable: encode request: " + err.Error(), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL(cred) + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: "iterable: build request: " + err.Error(), err: err}
	}
	req.Header.Set("Api-Key", cred.apiKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("iterable: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: "iterable: read response: " + err.Error(), err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		cause := fmt.Errorf("iterable API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		wrapped := &apiError{msg: cause.Error(), status: resp.StatusCode, err: cause}
		if resp.StatusCode == http.StatusUnauthorized || isBadKeyCode(responseCode(body)) {
			wrapped.err = execution.RejectCredential(cause)
		}
		return nil, wrapped
	}

	// 2xx: Iterable's write endpoints return {"code","msg","params"}; a code
	// other than "Success" is an application-level failure even at HTTP 200.
	if code := responseCode(body); code != "" && !strings.EqualFold(code, "Success") {
		cause := fmt.Errorf("iterable API error (code %s): %s", code, apiMessage(body))
		wrapped := &apiError{msg: cause.Error(), err: cause}
		if isBadKeyCode(code) {
			wrapped.err = execution.RejectCredential(cause)
		}
		return nil, wrapped
	}
	return body, nil
}

func (s *Service) baseURL(cred credential) string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return cred.baseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// responseCode reads the top-level "code" string from an Iterable response
// body, or "" when the body is not a code-carrying object (e.g. a GET resource
// array/object).
func responseCode(body []byte) string {
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Code
}

// isBadKeyCode reports whether an Iterable error code denotes an invalid or
// rejected API key (as opposed to a request/data error).
func isBadKeyCode(code string) bool {
	switch strings.ToLower(code) {
	case "badapikey", "invalidapikey", "badauthorizationheader", "unauthorized":
		return true
	default:
		return false
	}
}

// apiMessage extracts Iterable's error message from a response body, falling
// back to the raw body when no msg field is present.
func apiMessage(body []byte) string {
	var e struct {
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if e.Msg != "" {
			return e.Msg
		}
		if e.Message != "" {
			return e.Message
		}
	}
	return strings.TrimSpace(string(body))
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("iterable: --%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}
