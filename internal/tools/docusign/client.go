package docusign

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
// required flag, bad value, or invalid JSON. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a DocuSign non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejected sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// apiClient carries the resolved credentials and issues account-scoped
// eSignature REST calls. The account base path is
// {baseURI}/restapi/{apiVersion}/accounts/{accountID}.
type apiClient struct {
	baseURI   string
	accountID string
	token     string
	hc        *http.Client
}

func (c *apiClient) client() *http.Client {
	if c.hc != nil {
		return c.hc
	}
	return http.DefaultClient
}

// accountPath composes an account-scoped URL for a relative resource path
// (which must start with "/"), e.g. "/envelopes" →
// https://na3.docusign.net/restapi/v2.1/accounts/<acc>/envelopes.
func (c *apiClient) accountPath(rel string) string {
	return strings.TrimRight(c.baseURI, "/") + "/restapi/" + apiVersion + "/accounts/" + c.accountID + rel
}

// callJSON performs one account-scoped JSON request and returns the raw
// response body. A non-2xx surfaces DocuSign's errorCode/message as an
// apiError carrying the HTTP status; a 401 additionally rejects the credential.
func (c *apiClient) callJSON(ctx context.Context, method, rel string, query, payload any) ([]byte, error) {
	return c.call(ctx, method, rel, query, payload, "application/json")
}

// call issues the request and returns the raw body, asserting the response
// Content-Type is acceptable for JSON callers. accept is the requested Accept
// header (application/json for data calls; the binary download uses callRaw).
func (c *apiClient) call(ctx context.Context, method, rel string, query, payload any, accept string) ([]byte, error) {
	resp, body, err := c.do(ctx, method, rel, query, payload, accept)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyError(resp.StatusCode, body)
	}
	return body, nil
}

// callRaw is call for a binary response (the combined-documents PDF): it
// returns the raw bytes without assuming a JSON body, but still classifies a
// non-2xx JSON error envelope.
func (c *apiClient) callRaw(ctx context.Context, rel string) ([]byte, error) {
	resp, body, err := c.do(ctx, http.MethodGet, rel, nil, nil, "application/pdf")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, classifyError(resp.StatusCode, body)
	}
	return body, nil
}

// do builds and sends the request, reading the full body. It centralizes URL
// composition, Bearer auth, query encoding, and JSON payload marshaling.
func (c *apiClient) do(ctx context.Context, method, rel string, query, payload any, accept string) (*http.Response, []byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, &apiError{msg: fmt.Sprintf("docusign: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.accountPath(rel), reqBody)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("docusign: build request: %v", err), err: err}
	}
	if q := encodeQuery(query); q != "" {
		req.URL.RawQuery = q
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", accept)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("docusign: %s %s: %v", method, rel, err), err: err}
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		resp.Body.Close()
		return nil, nil, &apiError{msg: fmt.Sprintf("docusign: read response: %v", readErr), err: readErr}
	}
	return resp, body, nil
}

// classifyError turns a non-2xx DocuSign response into an apiError, extracting
// errorCode/message from the JSON body and rejecting the credential on 401.
func classifyError(status int, body []byte) error {
	msg := fmt.Sprintf("docusign API error (HTTP %d): %s", status, apiMessage(body))
	err := &apiError{msg: msg, status: status, err: fmt.Errorf("%s", msg)}
	if status == http.StatusUnauthorized {
		return &apiError{msg: msg, status: status, err: execution.RejectCredential(err.err)}
	}
	return err
}

// apiMessage extracts DocuSign's errorCode + message from an error body,
// falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.ErrorCode != "" || e.Message != "") {
		if e.ErrorCode != "" && e.Message != "" {
			return e.ErrorCode + ": " + e.Message
		}
		return e.ErrorCode + e.Message
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// decodeInto unmarshals a JSON body into v, wrapping a decode failure as an
// apiError so a malformed 2xx response is a runtime failure, not a usage error.
func decodeInto(body []byte, v any) error {
	if err := json.Unmarshal(body, v); err != nil {
		return &apiError{msg: fmt.Sprintf("docusign: decode response: %v", err), err: err}
	}
	return nil
}
