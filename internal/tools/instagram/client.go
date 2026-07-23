package instagram

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

// httpDoer is the minimal HTTP surface the service needs; *http.Client
// satisfies it and tests can point it at an httptest server.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, or bad enum value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Graph non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status
// (0 for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// graphError is the standard Meta Graph error envelope. Instagram uses the same
// shape as Facebook: {"error":{message,type,code,error_subcode,fbtrace_id}}.
type graphError struct {
	Error struct {
		Message      string `json:"message"`
		Type         string `json:"type"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
		FBTraceID    string `json:"fbtrace_id"`
	} `json:"error"`
}

// oauthExceptionCode is Graph's code for an expired/revoked/invalid access
// token (OAuthException). It maps to a distinct "reconnect needed" signal so
// the assistant prompts re-auth rather than retrying blindly.
const oauthExceptionCode = 190

// emitJSON writes the provider's JSON response to stdout verbatim (already
// structured — the assistant consumes it directly).
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(trimTrailingNewline(body), '\n'))
	return err
}

func trimTrailingNewline(b []byte) []byte {
	return []byte(strings.TrimRight(string(b), "\n"))
}

// get performs a GET against the Graph API with the given path and query.
func (s *Service) get(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	return s.call(ctx, token, http.MethodGet, path, query, nil)
}

// postForm performs a POST whose parameters are sent as an
// application/x-www-form-urlencoded body (so the token never lands in a URL).
func (s *Service) postForm(ctx context.Context, token, path string, form url.Values) ([]byte, error) {
	return s.call(ctx, token, http.MethodPost, path, nil, form)
}

// del performs a DELETE against the Graph API.
func (s *Service) del(ctx context.Context, token, path string) ([]byte, error) {
	return s.call(ctx, token, http.MethodDelete, path, nil, nil)
}

// call performs one Graph API request: Bearer auth on every call, an optional
// query string, and an optional form body. A non-2xx surfaces the Graph error
// envelope (with a reconnect hint for code 190) as an apiError carrying the
// HTTP status; a transport failure is an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query, form url.Values) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	full := base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, full, body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("instagram: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
		return nil, &apiError{msg: fmt.Sprintf("instagram: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("instagram: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyGraphError(resp.StatusCode, respBody)
	}
	return respBody, nil
}

// classifyGraphError turns a non-2xx Graph response into an apiError. A code
// 190 OAuthException (expired/revoked token) is wrapped as a rejected
// credential and surfaced with an explicit reconnect message so the assistant
// re-authorizes rather than retrying.
func classifyGraphError(status int, body []byte) error {
	var ge graphError
	msg := string(body)
	if err := json.Unmarshal(body, &ge); err == nil && ge.Error.Message != "" {
		msg = fmt.Sprintf("%s (type=%s, code=%d, subcode=%d)",
			ge.Error.Message, ge.Error.Type, ge.Error.Code, ge.Error.ErrorSubcode)
	}
	raw := fmt.Errorf("instagram API error (HTTP %d): %s", status, msg)
	if status == http.StatusUnauthorized || ge.Error.Code == oauthExceptionCode {
		reconnect := fmt.Errorf("instagram access token is expired or revoked; reconnect the Instagram account (%s)", msg)
		return &apiError{
			msg:    reconnect.Error(),
			status: status,
			err:    execution.RejectCredential(reconnect),
		}
	}
	return &apiError{msg: raw.Error(), status: status, err: raw}
}
