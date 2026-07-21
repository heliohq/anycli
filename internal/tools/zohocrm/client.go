package zohocrm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Zoho non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status
// (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// emitJSON writes the provider's JSON response to stdout verbatim. A 204 No
// Content (e.g. an empty search result or a fresh-write search-index lag)
// carries no body — nothing is written and the caller still exits 0.
func (s *Service) emitJSON(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Zoho CRM API request. The path is joined onto BaseURL +
// /crm/v8. Auth uses the CRM-documented Zoho-oauthtoken scheme. A non-2xx
// surfaces the body's code/message as an apiError carrying the HTTP status; a
// transport failure surfaces as an apiError with status 0. A 2xx (including
// 204) returns the raw body.
func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("zoho-crm: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+apiPrefix+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoho-crm: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoho-crm: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("zoho-crm: read response: %v", err), err: err}
	}
	// 2xx is success. 207 (Multi-Status) on bulk writes is also success — the
	// per-record code/status live in the body and the agent inspects them.
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("zoho-crm API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, body, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// apiMessage extracts Zoho's error code + message from a top-level error body
// ({"code":"INVALID_TOKEN","message":"…","status":"error"}), falling back to
// the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Code != "" || e.Message != "") {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return string(body)
}

// classifyCredentialError marks a 401 INVALID_TOKEN / AUTHENTICATION_FAILURE
// as an explicit credential rejection so the engine invalidates the stored
// token. A scope mismatch (OAUTH_SCOPE_MISMATCH) is deliberately NOT a
// rejection — the token is valid, it just lacks a scope — so the credential
// survives and the agent gets an actionable scope error instead.
func classifyCredentialError(status int, body []byte, err error) error {
	if status != http.StatusUnauthorized {
		return err
	}
	switch topLevelCode(body) {
	case "INVALID_TOKEN", "AUTHENTICATION_FAILURE", "INVALID_OAUTHTOKEN":
		return execution.RejectCredential(err)
	}
	return err
}

// topLevelCode returns the top-level `code` field of a Zoho error body.
func topLevelCode(body []byte) string {
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Code
}
