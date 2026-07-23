package activecampaign

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

// apiPath is the fixed v3 path segment every account base carries.
const apiPath = "/api/3"

// usageError is a parameter / usage error (bad flag combo, missing required
// flag, invalid JSON). It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a non-2xx response or a transport failure.
// It maps to exit code 1 and kind "api". status is the HTTP status (0 for
// transport failures). It wraps its cause so errors.As for a credential
// rejection still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// normalizeBaseURL turns any accepted paste of the account URL into the
// canonical request base `<scheme>://<host>[:port]/api/3` (design 317: the
// single normalization site is here in AnyCLI; the bundle stores the value
// verbatim). Accepts: full URL, trailing slash, a stray /api/3[/], or a bare
// host (scheme defaults to https). The path/query/fragment the user pasted are
// discarded — only scheme + host survive, then /api/3 is appended.
func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("account URL is empty")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("not a valid URL: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("URL has no host")
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Host + apiPath, nil
}

// client performs authenticated ActiveCampaign v3 requests. base ends in
// /api/3 (no trailing slash); relative paths are joined with a single slash.
type client struct {
	base  string
	token string
	hc    *http.Client
	out   io.Writer
}

// get performs a GET with optional query params and emits the body verbatim.
func (c *client) get(ctx context.Context, relPath string, query url.Values) error {
	body, err := c.call(ctx, http.MethodGet, relPath, query, nil)
	if err != nil {
		return err
	}
	return c.emit(body)
}

// send performs a write (POST/PUT/DELETE) with an optional JSON payload and
// emits the body verbatim.
func (c *client) send(ctx context.Context, method, relPath string, payload any) error {
	body, err := c.call(ctx, method, relPath, nil, payload)
	if err != nil {
		return err
	}
	return c.emit(body)
}

// emit writes the provider's JSON response to stdout verbatim.
func (c *client) emit(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	_, err := c.out.Write(append(body, '\n'))
	return err
}

// call performs one request with the Api-Token header. A non-2xx surfaces the
// body's message as an apiError carrying the status (401/403 also classified as
// a credential rejection); a transport failure is an apiError with status 0.
func (c *client) call(ctx context.Context, method, relPath string, query url.Values, payload any) ([]byte, error) {
	target := c.base + "/" + strings.TrimPrefix(relPath, "/")
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("activecampaign: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("activecampaign: build request: %v", err), err: err}
	}
	req.Header.Set("Api-Token", c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.hc
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("activecampaign: %s %s: %v", method, relPath, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("activecampaign: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("ActiveCampaign API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError flags 401/403 as an explicit credential rejection so
// the host invalidates a bad key; other non-2xx (429, 422, 5xx) stay ordinary
// runtime failures that keep the credential.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts ActiveCampaign's error text. v3 errors come either as
// {"message":"…"} (single) or {"errors":[{"title":"…"}]} (validation);
// fall back to the raw body.
func apiMessage(body []byte) string {
	var single struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &single); err == nil && single.Message != "" {
		return single.Message
	}
	var multi struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &multi); err == nil && len(multi.Errors) > 0 {
		parts := make([]string, 0, len(multi.Errors))
		for _, e := range multi.Errors {
			if e.Detail != "" {
				parts = append(parts, e.Title+": "+e.Detail)
			} else {
				parts = append(parts, e.Title)
			}
		}
		return strings.Join(parts, "; ")
	}
	return strings.TrimSpace(string(body))
}
