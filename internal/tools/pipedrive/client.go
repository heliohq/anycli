package pipedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: a bad flag combination, missing
// required flag/arg, or invalid JSON. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Pipedrive non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport failures). It wraps the underlying cause so
// errors.As for the credential-rejected classification still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// caller carries the resolved base URL + token plus the Service (for the HTTP
// client and stdout). Every command closes over one caller.
type caller struct {
	s     *Service
	base  string
	token string
}

// do performs one Pipedrive API request: Bearer auth on every call, optional
// JSON body, verbatim response bytes on 2xx. A non-2xx surfaces Pipedrive's
// error/error_info as an apiError carrying the HTTP status; a transport failure
// is an apiError with status 0.
func (c *caller) do(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("pipedrive: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pipedrive: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	hc := c.s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pipedrive: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pipedrive: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("pipedrive API error (HTTP %d): %s%s",
			resp.StatusCode, apiMessage(body), accessHint(resp.StatusCode))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (design 003 §3:
// no re-shaping of provider payloads).
func (c *caller) emit(body []byte) error {
	_, err := c.s.stdout().Write(append(body, '\n'))
	return err
}

// run is the shared command tail: perform the request and emit the verbatim
// body. Command RunE funcs delegate here so every verb renders identically.
func (c *caller) run(ctx context.Context, method, path string, query url.Values, payload any) error {
	body, err := c.do(ctx, method, path, query, payload)
	if err != nil {
		return err
	}
	return c.emit(body)
}

// apiMessage extracts Pipedrive's error text (error + error_info) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error     string `json:"error"`
		ErrorInfo string `json:"error_info"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error != "" || e.ErrorInfo != "") {
		if e.ErrorInfo != "" && e.ErrorInfo != e.Error {
			return fmt.Sprintf("%s (%s)", e.Error, e.ErrorInfo)
		}
		return e.Error
	}
	return string(body)
}

// accessHint returns an actionable clause for the failures an agent most often
// hits on Pipedrive: an expired/insufficient token or an unknown id.
func accessHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return " (access token invalid or expired — reconnect Pipedrive)"
	case http.StatusForbidden:
		return " (token may lack the required scope for this action)"
	case http.StatusNotFound:
		return " (check the id and that this record exists in the connected company)"
	default:
		return ""
	}
}

// classifyCredentialError marks HTTP 401 as an explicit credential rejection so
// the engine can invalidate the token; every other status stays a plain
// runtime error (a valid token may still be rate-limited or hit a bad request).
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}
