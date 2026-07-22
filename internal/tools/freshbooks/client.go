package freshbooks

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, invalid JSON, or an unresolvable account. It maps to exit code
// 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a FreshBooks non-2xx response, a transport
// failure, or a decode failure. It maps to exit code 1 and kind "api". status is
// the HTTP status (0 for transport/network failures); code carries FreshBooks'
// own error code when present. It wraps the underlying cause so errors.As for a
// credential rejection still resolves through it.
type apiError struct {
	msg    string
	status int
	code   string
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one FreshBooks API request with Bearer auth and the
// Api-Version header, returning the raw response body. A non-2xx surfaces the
// FreshBooks error message + code as an apiError carrying the HTTP status; a
// transport failure surfaces as an apiError with status 0. A 401 (or an
// explicit unauthenticated error code) is classified as a credential rejection.
func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("freshbooks: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("freshbooks: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Api-Version", apiVersion)
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
		return nil, &apiError{msg: fmt.Sprintf("freshbooks: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("freshbooks: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, code := apiMessage(body)
		raw := fmt.Errorf("freshbooks API error (HTTP %d): %s", resp.StatusCode, msg)
		classified := classifyCredentialError(resp.StatusCode, code, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, code: code, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 or an unauthenticated error code as an
// explicit credential rejection so the engine can invalidate the token.
func classifyCredentialError(status int, code string, err error) error {
	if status == http.StatusUnauthorized || code == "unauthenticated" || code == "unauthorized" {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts FreshBooks' error text and code from an error body.
// FreshBooks reports errors in two shapes: the accounting shape
// {"response":{"errors":[{"message":"...","errno":...}]}} and the auth/identity
// shape {"error":"...","error_description":"..."} / {"message":"..."}. It falls
// back to the raw body when neither matches.
func apiMessage(body []byte) (msg string, code string) {
	var acct struct {
		Response struct {
			Errors []struct {
				Message string `json:"message"`
				Errno   int    `json:"errno"`
			} `json:"errors"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &acct); err == nil && len(acct.Response.Errors) > 0 {
		e := acct.Response.Errors[0]
		if e.Errno != 0 {
			code = fmt.Sprintf("%d", e.Errno)
		}
		if strings.TrimSpace(e.Message) != "" {
			return e.Message, code
		}
	}
	var auth struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
	}
	if err := json.Unmarshal(body, &auth); err == nil {
		if strings.TrimSpace(auth.ErrorDescription) != "" {
			return auth.ErrorDescription, strings.TrimSpace(auth.Error)
		}
		if strings.TrimSpace(auth.Error) != "" {
			return auth.Error, strings.TrimSpace(auth.Error)
		}
		if strings.TrimSpace(auth.Message) != "" {
			return auth.Message, ""
		}
	}
	return strings.TrimSpace(string(body)), ""
}

// emitJSON writes a value to stdout as compact JSON followed by a newline.
func (s *Service) emitJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("freshbooks: encode output: %v", err), err: err}
	}
	_, err = s.stdout().Write(append(b, '\n'))
	return err
}
