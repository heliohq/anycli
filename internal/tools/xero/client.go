package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// accountingPrefix is the Accounting API path prefix appended to BaseURL; the
// tenant is a per-call header, not part of the path (design: one token, N orgs).
const accountingPrefix = "/api.xro/2.0"

// connectionsPath is the tenant-discovery endpoint on the same host as the
// Accounting API but outside the /api.xro/2.0 prefix. It needs no tenant header.
const connectionsPath = "/connections"

// usageError is a parameter / usage error (bad flag combo, unknown subcommand,
// ambiguous tenant, invalid JSON). It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Xero non-2xx response or a transport
// failure. It maps to exit code 1. status is the HTTP status (0 for transport
// failures); details carries Xero's own error body verbatim so it is surfaced
// rather than swallowed.
type apiError struct {
	msg     string
	status  int
	details json.RawMessage
	err     error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// emitJSON writes the provider's JSON response to stdout verbatim (agent-neutral
// — Xero's PascalCase envelopes pass through unreshaped).
func (s *Service) emitJSON(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one request against the Xero host. path is the full path after
// BaseURL (e.g. accountingPrefix+"/Invoices" or connectionsPath). tenant, when
// non-empty, is sent as the Xero-Tenant-Id header. query is optional. Every call
// carries Bearer auth and Accept: application/json.
func (s *Service) call(ctx context.Context, token, method, path, tenant string, query url.Values, body any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	full := base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	var reqBody io.Reader
	if body != nil {
		b, err := toJSON(body)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("xero: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("xero: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if tenant != "" {
		req.Header.Set("Xero-Tenant-Id", tenant)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("xero: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("xero: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &apiError{
			msg:     fmt.Sprintf("xero API error (HTTP %d): %s", resp.StatusCode, apiMessage(respBody)),
			status:  resp.StatusCode,
			details: rawOrNil(respBody),
		}
	}
	return respBody, nil
}

// toJSON marshals a payload: a json.RawMessage / []byte is passed through so a
// caller-supplied raw body is forwarded verbatim.
func toJSON(body any) ([]byte, error) {
	switch v := body.(type) {
	case json.RawMessage:
		return v, nil
	case []byte:
		return v, nil
	default:
		return json.Marshal(v)
	}
}

// rawOrNil returns b as a json.RawMessage when it is valid JSON, else nil (so
// the error envelope's details field stays well-formed).
func rawOrNil(b []byte) json.RawMessage {
	if len(b) == 0 || !json.Valid(b) {
		return nil
	}
	return json.RawMessage(b)
}

// apiMessage extracts a human message from a Xero error body. Xero uses two
// shapes: {"Type","Message","Elements":[…]} on 400 validation, and
// {"Detail":"…"} / {"Title":"…"} on 401/403/404. Falls back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Type    string `json:"Type"`
		Message string `json:"Message"`
		Detail  string `json:"Detail"`
		Title   string `json:"Title"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			if e.Type != "" {
				return fmt.Sprintf("%s: %s", e.Type, e.Message)
			}
			return e.Message
		case e.Detail != "":
			return e.Detail
		case e.Title != "":
			return e.Title
		}
	}
	if len(body) == 0 {
		return "no response body"
	}
	return string(body)
}
