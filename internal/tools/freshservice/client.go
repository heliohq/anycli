package freshservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// client performs Freshservice v2 API calls with HTTP Basic auth. baseURL
// already carries /api/v2; apiKey is the per-account key sent as the Basic
// username with a dummy password.
type client struct {
	baseURL string
	apiKey  string
	hc      *http.Client
}

// apiError is a runtime / API error: a Freshservice non-2xx response or a
// transport failure. It maps to exit code 1. status is the HTTP status (0 for
// transport failures); providerCode carries Freshservice's error code when
// present; retryAfter carries the Retry-After header on a 429.
type apiError struct {
	msg          string
	status       int
	providerCode string
	retryAfter   string
	err          error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// authHeader returns the Basic credential. Freshservice ignores the password,
// so a fixed dummy "X" is used, per the official docs (-u apikey:X).
func (c *client) authHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.apiKey+":X"))
}

// call performs one API request and returns the raw body plus the response
// headers (needed for the pagination link header). A 401 marks the credential
// rejected; any other non-2xx becomes an apiError carrying Freshservice's
// message and, on 429, its Retry-After.
func (c *client) call(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, http.Header, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, &apiError{msg: fmt.Sprintf("freshservice: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("freshservice: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("freshservice: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("freshservice: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, nil, newAPIError(resp, body)
	}
	return body, resp.Header, nil
}

// newAPIError builds a typed error from a non-2xx response. On 401 it is marked
// as a credential rejection so AnyCLI can invalidate the stored secret.
func newAPIError(resp *http.Response, body []byte) error {
	code := providerErrorCode(body)
	msg := fmt.Sprintf("freshservice API error (HTTP %d): %s", resp.StatusCode, providerMessage(body))
	e := &apiError{
		msg:          msg,
		status:       resp.StatusCode,
		providerCode: code,
		err:          fmt.Errorf("%s", msg),
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		e.retryAfter = resp.Header.Get("Retry-After")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		e.err = execution.RejectCredential(e.err)
	}
	return e
}

// providerMessage extracts Freshservice's human-readable error from a response
// body. Freshservice returns {"description": "...", "errors": [{...}]}; both are
// surfaced verbatim so the agent sees exactly which field an account requires.
func providerMessage(body []byte) string {
	var env struct {
		Description string            `json:"description"`
		Message     string            `json:"message"`
		Errors      []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		parts := make([]string, 0, 2)
		switch {
		case env.Description != "":
			parts = append(parts, env.Description)
		case env.Message != "":
			parts = append(parts, env.Message)
		}
		if len(env.Errors) > 0 {
			errsRaw, _ := json.Marshal(env.Errors)
			parts = append(parts, string(errsRaw))
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	return string(body)
}

// providerErrorCode returns the first field-level error code Freshservice
// reports, else empty.
func providerErrorCode(body []byte) string {
	var env struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err == nil && len(env.Errors) > 0 {
		return env.Errors[0].Code
	}
	return ""
}

// emit writes raw JSON to stdout (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value and writes it (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("freshservice: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// emitResource unwraps Freshservice's single-resource envelope
// ({"ticket": {...}} → {...}) and emits the bare object. If the envelope key
// is absent, the body is emitted verbatim.
func (s *Service) emitResource(body []byte, resourceKey string) error {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(body, &env); err == nil {
		if inner, ok := env[resourceKey]; ok {
			return s.emit(inner)
		}
	}
	return s.emit(body)
}

// emitList projects a Freshservice list envelope ({"tickets": [...]}) into the
// provider-neutral shape {"items":[...],"page":N,"per_page":N,"next_page":N|null}.
// next_page is derived from the link header: present → page+1, absent → null.
func (s *Service) emitList(body []byte, hdr http.Header, resourceKey string, page, perPage int) error {
	var env map[string]json.RawMessage
	items := json.RawMessage("[]")
	if err := json.Unmarshal(body, &env); err == nil {
		if inner, ok := env[resourceKey]; ok {
			items = inner
		}
	}
	out := map[string]any{
		"items":    items,
		"page":     page,
		"per_page": perPage,
	}
	if hasNextLink(hdr) {
		out["next_page"] = page + 1
	} else {
		out["next_page"] = nil
	}
	return s.emitValue(out)
}

// hasNextLink reports whether the response link header advertises a next page.
// Freshservice sets `link: <...?page=N>; rel="next"` only when more pages exist.
func hasNextLink(hdr http.Header) bool {
	link := hdr.Get("Link")
	return strings.Contains(link, `rel="next"`)
}

// emitListResult runs a GET list request and projects it. Shared by every list
// subcommand.
func (s *Service) emitListResult(cmd *cobra.Command, c *client, path, resourceKey string, query url.Values, page, perPage int) error {
	body, hdr, err := c.call(cmd.Context(), http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	return s.emitList(body, hdr, resourceKey, page, perPage)
}
