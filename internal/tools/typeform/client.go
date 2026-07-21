package typeform

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Typeform non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classification still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Typeform API request with Bearer auth and returns the raw
// response body. A 401 (or an AUTHENTICATION_FAILED code) marks the credential
// rejected; any other non-2xx is an apiError carrying Typeform's
// code/description and the HTTP status. A transport failure is an apiError with
// status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	base = strings.TrimRight(base, "/")

	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("typeform: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("typeform: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("typeform: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("typeform: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("typeform API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, body, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 (or an AUTHENTICATION_FAILED error code)
// as an explicit credential rejection so the engine can invalidate the token;
// every other failure (403 scope, 404, 429 rate limit, 5xx) leaves the
// credential untouched.
func classifyCredentialError(status int, body []byte, err error) error {
	if status == http.StatusUnauthorized || errorCode(body) == "AUTHENTICATION_FAILED" {
		return execution.RejectCredential(err)
	}
	return err
}

// errorCode extracts Typeform's machine-readable error code from an error body.
func errorCode(body []byte) string {
	var e struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return ""
	}
	return e.Code
}

// apiMessage extracts Typeform's error code + description from an error body,
// falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Code != "" || e.Description != "") {
		switch {
		case e.Code != "" && e.Description != "":
			return e.Code + ": " + e.Description
		case e.Code != "":
			return e.Code
		default:
			return e.Description
		}
	}
	return string(body)
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitOK writes a small client-side receipt for endpoints that return an empty
// body on success (204 No Content: form patch, form delete, webhook delete). It
// gives an agent reading stdout a definite success signal rather than nothing.
func (s *Service) emitOK(receipt map[string]any) error {
	b, err := json.Marshal(receipt)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("typeform: encode receipt: %v", err), err: err}
	}
	return s.emit(b)
}

// readJSONArg resolves a JSON body from an inline value or an @file reference
// (e.g. --definition @form.json), then validates it parses. An empty value is a
// fail-fast usage error (the flag is required where this is called). Invalid
// JSON — inline or from file — is a usage error.
func readJSONArg(flag, val string) (json.RawMessage, error) {
	v := strings.TrimSpace(val)
	if v == "" {
		return nil, &usageError{msg: fmt.Sprintf("--%s is required", flag)}
	}
	if strings.HasPrefix(v, "@") {
		b, err := os.ReadFile(v[1:])
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --%s %s: %v", flag, v[1:], err)}
		}
		v = string(b)
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(v), &raw); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s is not valid JSON: %v", flag, err)}
	}
	return raw, nil
}
