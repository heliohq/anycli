package stripe

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad --param syntax, or an unresolvable argument. It maps to
// exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Stripe non-2xx response or a transport
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

// callOpts carries the optional pieces of a Stripe request beyond method/path.
type callOpts struct {
	// query is appended as the URL query string (pagination + list filters).
	query url.Values
	// form, when non-nil, is sent as an application/x-www-form-urlencoded body
	// (Stripe's wire format for create/update/delete params).
	form url.Values
	// idempotencyKey, when non-empty, is forwarded as the Idempotency-Key
	// header (Stripe's documented safe-retry mechanism).
	idempotencyKey string
}

// call performs one Stripe API request with Bearer auth and the pinned
// Stripe-Version header, returning the raw response body. A 401 marks the
// credential rejected; any other non-2xx is an apiError carrying Stripe's
// error.{type,code,message,param} and the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, opts callOpts) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(opts.query) > 0 {
		requestURL += "?" + opts.query.Encode()
	}

	var body io.Reader
	if opts.form != nil {
		body = strings.NewReader(opts.form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("stripe: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Stripe-Version", stripeVersion)
	req.Header.Set("Accept", "application/json")
	if opts.form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if opts.idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", opts.idempotencyKey)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("stripe: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("stripe: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("stripe API error (HTTP %d): %s", resp.StatusCode, apiMessage(respBody))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return respBody, nil
}

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

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// classifyCredentialError marks a 401 as an explicit credential rejection so
// the host can invalidate the token; every other status stays an ordinary
// failure (a scope/permission/rate-limit error must not invalidate a token
// that may still be valid).
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts Stripe's error.{type,code,message,param} from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
			Param   string `json:"param"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return string(body)
	}
	er := e.Error
	if er.Type == "" && er.Code == "" && er.Message == "" && er.Param == "" {
		return string(body)
	}
	parts := make([]string, 0, 4)
	if er.Type != "" {
		parts = append(parts, er.Type)
	}
	if er.Code != "" {
		parts = append(parts, er.Code)
	}
	msg := strings.Join(parts, "/")
	if er.Message != "" {
		if msg != "" {
			msg += ": " + er.Message
		} else {
			msg = er.Message
		}
	}
	if er.Param != "" {
		msg += fmt.Sprintf(" (param: %s)", er.Param)
	}
	if msg == "" {
		return string(body)
	}
	return msg
}
