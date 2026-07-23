package facebookpages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxErrorBodyBytes = 8 << 10

// usageError is a parameter / usage error (illegal flag combination, missing
// required flag, bad enum). It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Graph non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport failures). It wraps the underlying cause so errors.As for a
// credential-rejection classification still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// graphError is Facebook's standard error envelope, returned as
// {"error":{"message","type","code","error_subcode","fbtrace_id"}}.
type graphError struct {
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	FBTraceID    string `json:"fbtrace_id"`
}

// call performs one Graph API request with Bearer auth using the supplied
// token (a user token for discovery, a Page token for Page-scoped ops). A
// non-2xx surfaces the Graph error envelope as an apiError carrying the HTTP
// status; a transport failure as an apiError with status 0. The active token is
// redacted from any error text so it never leaks into output or logs.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload url.Values) ([]byte, error) {
	var requestBody io.Reader
	if payload != nil {
		requestBody = strings.NewReader(payload.Encode())
	}

	requestURL := s.apiBase() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, requestBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("facebook-pages: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("facebook-pages: %s %s: %v", method, path, redact(err.Error(), token)), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("facebook-pages: read response: %v", err), err: err}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newAPIError(resp.StatusCode, body, token)
	}
	return body, nil
}

// pageToken resolves the Page access token for pageID using the stored USER
// token — the first leg of the two-hop. It is reported distinctly on failure so
// the assistant can tell "wrong Page id / no access to that Page" from "the
// actual operation failed". The returned token is internal and must never be
// printed.
func (s *Service) pageToken(ctx context.Context, userToken, pageID string) (string, error) {
	query := url.Values{"fields": {"access_token"}}
	body, err := s.call(ctx, userToken, http.MethodGet, "/"+url.PathEscape(pageID), query, nil)
	if err != nil {
		// Prepend the resolve-leg context in place. We mutate the underlying
		// *apiError's message but return the ORIGINAL err so any
		// credential-rejection wrapper (code 190) is preserved.
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			apiErr.msg = fmt.Sprintf("facebook-pages: resolve Page access token for %s: %s", pageID, apiErr.msg)
		}
		return "", err
	}
	var envelope struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", &apiError{msg: fmt.Sprintf("facebook-pages: decode Page token for %s: %v", pageID, err), err: err}
	}
	if strings.TrimSpace(envelope.AccessToken) == "" {
		return "", &apiError{msg: fmt.Sprintf(
			"facebook-pages: no Page access token returned for %s (the user may lack a role on this Page)", pageID)}
	}
	return envelope.AccessToken, nil
}

// callAsPage performs a Page-scoped request: it resolves the Page token via the
// two-hop and issues the actual call with that token. Both legs redact their
// respective token from error output, so neither the user nor the Page token
// leaks.
func (s *Service) callAsPage(ctx context.Context, userToken, pageID, method, path string, query, payload url.Values) ([]byte, error) {
	pt, err := s.pageToken(ctx, userToken, pageID)
	if err != nil {
		return nil, err
	}
	return s.call(ctx, pt, method, path, query, payload)
}

// emit writes a provider JSON response to stdout verbatim (plus a newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitCreated projects a write response to a compact {"id":...} result so the
// assistant gets the new post/comment id without any Page-token bleed from a
// read-after-write echo.
func (s *Service) emitCreated(body []byte) error {
	var envelope struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &apiError{msg: fmt.Sprintf("facebook-pages: decode write response: %v", err), err: err}
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("facebook-pages: encode result: %v", err), err: err}
	}
	return s.emit(out)
}

// newAPIError builds an apiError from a Graph non-2xx response, mapping the
// well-known auth/permission codes to distinct, actionable messages and
// classifying an expired/invalid token (code 190) as a credential rejection so
// the engine prompts re-auth rather than retrying blindly.
func newAPIError(status int, body []byte, token string) error {
	var envelope struct {
		Error graphError `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	ge := envelope.Error

	detail := strings.TrimSpace(ge.Message)
	if detail == "" {
		detail = redact(strings.TrimSpace(string(body)), token)
		if detail == "" {
			detail = "empty response body"
		}
		if len(detail) > maxErrorBodyBytes {
			detail = detail[:maxErrorBodyBytes] + "…"
		}
	}

	hint := graphHint(ge)
	msg := fmt.Sprintf("facebook-pages API error (HTTP %d, code %d): %s%s", status, ge.Code, detail, hint)
	return classifyCredentialError(status, ge, &apiError{msg: msg, status: status})
}

// graphHint returns an actionable clause for the failures an agent most often
// hits: an expired/revoked token or an insufficient Page permission.
func graphHint(ge graphError) string {
	switch ge.Code {
	case codeOAuthException:
		return "; access token is invalid or expired — reconnect Facebook Pages"
	case codePermission:
		return "; the connection lacks the required Page permission or task (e.g. CREATE_CONTENT), or the app has not been granted the needed scope"
	}
	return ""
}

// redact removes the active token from a string so it never lands in an error
// message or log line.
func redact(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[REDACTED]")
}

// fieldsOrDefault resolves a comma-separated --fields flag into a Graph
// ?fields= value, falling back to a caller default when the flag is empty.
func fieldsOrDefault(fields, fallback string) string {
	if strings.TrimSpace(fields) == "" {
		return fallback
	}
	return fields
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
