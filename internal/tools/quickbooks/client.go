package quickbooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// QuickBooks Online API bases (design: §1). Every Accounting/Reports call is
// company-scoped under /v3/company/<realmId>/. The sandbox base is selected
// only for the L2 dev harness via QUICKBOOKS_ENVIRONMENT=sandbox.
const (
	prodBaseURL    = "https://quickbooks.api.intuit.com"
	sandboxBaseURL = "https://sandbox-quickbooks.api.intuit.com"
	// minorVersion pins the response schema so output is stable across Intuit's
	// rolling schema bumps (§1). Sent on every request as ?minorversion=.
	minorVersion = "75"
)

// baseURLFor resolves the API base from the QUICKBOOKS_ENVIRONMENT value.
// Empty or "production" → prod; "sandbox" → the sandbox host. Any other value
// is treated as production (fail-open to the real API rather than a silent
// wrong host); the harness is the only caller that sets it.
func baseURLFor(environment string) string {
	if strings.EqualFold(strings.TrimSpace(environment), "sandbox") {
		return sandboxBaseURL
	}
	return prodBaseURL
}

// usageError is a parameter / usage error (illegal flag combo, missing required
// flag, invalid JSON). It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// faultDetail is one entry of QuickBooks' error envelope
// ({"Fault":{"Error":[{"Message","Detail","code"}]}}). Passed through so the
// teammate sees the code and detail, not just the HTTP status (§2).
type faultDetail struct {
	Message string `json:"message,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Code    string `json:"code,omitempty"`
}

// apiError is a runtime / API error: a QBO non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport failures); faults carries the parsed QBO Fault array.
type apiError struct {
	msg    string
	status int
	faults []faultDetail
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client performs company-scoped QuickBooks Online API calls. It is built per
// invocation from the resolved credentials; tests point base at an httptest
// server.
type client struct {
	base  string
	realm string
	token string
	hc    *http.Client
	out   io.Writer
	err   io.Writer
}

func (c *client) httpClient() *http.Client {
	if c.hc != nil {
		return c.hc
	}
	return http.DefaultClient
}

// companyPath builds the company-scoped path for an Accounting resource, e.g.
// resource "invoice/42" → /v3/company/<realm>/invoice/42.
func (c *client) companyPath(resource string) string {
	return "/v3/company/" + url.PathEscape(c.realm) + "/" + resource
}

// call performs one QuickBooks Online API request. query is merged with the
// pinned minorversion. payload (when non-nil) is JSON-encoded. Bearer auth and
// Accept: application/json are sent on every call; a non-2xx surfaces the QBO
// Fault body as an apiError carrying the status and parsed faults.
func (c *client) call(ctx context.Context, method, resource string, query url.Values, payload any) ([]byte, error) {
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	q.Set("minorversion", minorVersion)

	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("quickbooks: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	endpoint := c.base + c.companyPath(resource) + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("quickbooks: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("quickbooks: %s %s: %v", method, resource, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("quickbooks: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		faults := parseFaults(body)
		return nil, &apiError{
			msg:    fmt.Sprintf("quickbooks API error (HTTP %d): %s", resp.StatusCode, faultSummary(resp.StatusCode, faults, body)),
			status: resp.StatusCode,
			faults: faults,
		}
	}
	return body, nil
}

// emitJSON writes the provider's JSON response to stdout verbatim.
func (c *client) emitJSON(body []byte) error {
	_, err := c.out.Write(append(body, '\n'))
	return err
}

// faultEnvelope models QuickBooks' error body. Intuit returns Fault with a
// capitalized key on the data API; the OAuth/token surface uses lowercase
// fault, so both are decoded.
type faultEnvelope struct {
	Fault struct {
		Type  string `json:"type"`
		Error []struct {
			Message string `json:"Message"`
			Detail  string `json:"Detail"`
			Code    string `json:"code"`
		} `json:"Error"`
	} `json:"Fault"`
	FaultLower struct {
		Error []struct {
			Message string `json:"message"`
			Detail  string `json:"detail"`
			Code    string `json:"code"`
		} `json:"error"`
	} `json:"fault"`
}

// parseFaults extracts QBO's Fault/Error array from an error body, tolerating
// either casing. An unparseable body yields nil (the raw body is surfaced by
// faultSummary instead).
func parseFaults(body []byte) []faultDetail {
	var env faultEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil
	}
	var out []faultDetail
	for _, e := range env.Fault.Error {
		out = append(out, faultDetail{Message: e.Message, Detail: e.Detail, Code: e.Code})
	}
	for _, e := range env.FaultLower.Error {
		out = append(out, faultDetail{Message: e.Message, Detail: e.Detail, Code: e.Code})
	}
	return out
}

// faultSummary renders a compact one-line reason from the parsed faults,
// falling back to the raw body when nothing parsed.
func faultSummary(status int, faults []faultDetail, body []byte) string {
	if len(faults) == 0 {
		return strings.TrimSpace(string(body))
	}
	parts := make([]string, 0, len(faults))
	for _, f := range faults {
		seg := f.Message
		if f.Code != "" {
			seg = f.Code + ": " + seg
		}
		if f.Detail != "" {
			seg += " (" + f.Detail + ")"
		}
		parts = append(parts, seg)
	}
	return strings.Join(parts, "; ")
}
