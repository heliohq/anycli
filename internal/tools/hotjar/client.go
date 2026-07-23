package hotjar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error (missing required flag surfaces via
// cobra directly, but pre-flight argument checks use this). It maps to exit
// code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Hotjar non-2xx response, a transport
// failure, or a decode failure. It maps to exit code 1 and kind "api". status
// is the HTTP status (0 for transport/network failures). It wraps the
// underlying cause so errors.As for *credentialRejectedError still resolves
// through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// ensureToken exchanges the account's client_id/client_secret for a Bearer
// access token at POST /v1/oauth/token, caching it for the invocation. The two
// secrets are the only inputs, so a 4xx here is a bad or revoked key pair, not
// a transient fault — reject the credential so the gateway surfaces a reconnect;
// a 5xx stays a plain (retryable) error. The exchange runs at most once per
// process — every API call funnels through here.
func (s *Service) ensureToken(ctx context.Context, creds clientCreds) (string, error) {
	if s.token != "" {
		return s.token, nil
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", creds.clientID)
	form.Set("client_secret", creds.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+tokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("hotjar: build token request: %v", err), err: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("hotjar: token exchange: %v", err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("hotjar: read token response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		base := fmt.Errorf("hotjar token exchange failed (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		cause := base
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			cause = execution.RejectCredential(base)
		}
		return "", &apiError{msg: base.Error(), status: resp.StatusCode, err: cause}
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", &apiError{msg: fmt.Sprintf("hotjar: decode token response: %v", err), err: err}
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		rejected := execution.RejectCredential(fmt.Errorf("hotjar token exchange returned no access_token"))
		return "", &apiError{msg: rejected.Error(), err: rejected}
	}
	s.token = out.AccessToken
	return s.token, nil
}

// get performs a Bearer-authenticated GET and returns the raw response body.
func (s *Service) get(ctx context.Context, creds clientCreds, path string, query url.Values) ([]byte, error) {
	token, err := s.ensureToken(ctx, creds)
	if err != nil {
		return nil, err
	}
	requestURL := s.baseURL() + path
	if enc := query.Encode(); enc != "" {
		requestURL += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hotjar: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	return s.do(req, http.MethodGet, path)
}

// post performs a Bearer-authenticated JSON POST and returns the raw response
// body.
func (s *Service) post(ctx context.Context, creds clientCreds, path string, payload any) ([]byte, error) {
	token, err := s.ensureToken(ctx, creds)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hotjar: encode request: %v", err), err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+path, bytes.NewReader(b))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hotjar: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	return s.do(req, http.MethodPost, path)
}

// do executes a prepared request and returns the response body. A 401
// (missing/expired token) or 403 (valid token, insufficient permission or plan
// tier) marks the credential rejected so the gateway surfaces a reconnect; any
// other non-2xx (including 429 rate-limit) is a plain, retryable error carrying
// Hotjar's message.
func (s *Service) do(req *http.Request, method, path string) ([]byte, error) {
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hotjar: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("hotjar: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		base := fmt.Errorf("hotjar API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		cause := error(base)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			cause = execution.RejectCredential(base)
		}
		return nil, &apiError{msg: base.Error(), status: resp.StatusCode, err: cause}
	}
	return body, nil
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

// apiMessage extracts a human-readable error from a Hotjar error body, falling
// back to the raw body. Hotjar surfaces failures under `error`,
// `error_description`, or `message`.
func apiMessage(body []byte) string {
	var e struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case strings.TrimSpace(e.ErrorDescription) != "":
			return e.ErrorDescription
		case strings.TrimSpace(e.Message) != "":
			return e.Message
		case strings.TrimSpace(e.Error) != "":
			return e.Error
		}
	}
	return strings.TrimSpace(string(body))
}
