package servicenow

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

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or an unresolvable number. It
// maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a ServiceNow non-2xx response or a
// transport failure. It maps to exit code 1. status is the HTTP status (0 for
// transport/network failures); detail carries ServiceNow's error.detail field.
// It wraps the underlying cause so errors.As for *credentialRejectedError still
// resolves through it.
type apiError struct {
	msg    string
	detail string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// client performs Table API requests against one instance base URL with the
// x-sn-apikey header.
type client struct {
	base   string // scheme://host, no trailing slash, no path
	apiKey string
	hc     *http.Client
}

func (c *client) httpClient() *http.Client {
	if c.hc != nil {
		return c.hc
	}
	return http.DefaultClient
}

// normalizeInstanceURL reduces a user-supplied instance URL to scheme://host
// (host includes any port), dropping path/query/fragment and any trailing
// slash. A bare host (no scheme) defaults to https. This value is both the
// request base and the account_key the Helio bundle derives.
func normalizeInstanceURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", &usageError{msg: "instance URL is empty"}
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", &usageError{msg: fmt.Sprintf("invalid instance URL %q: %v", raw, err)}
	}
	if u.Host == "" {
		return "", &usageError{msg: fmt.Sprintf("invalid instance URL %q: missing host", raw)}
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Host, nil
}

// callTable issues a Table API request: <base>/api/now/table/<table>[/<sysID>]
// with the given query. payload nil for GET/DELETE. It returns the raw response
// body on 2xx and an *apiError otherwise.
func (c *client) callTable(ctx context.Context, method, table, sysID string, query url.Values, payload any) ([]byte, error) {
	path := tableAPIPrefix + "/" + url.PathEscape(table)
	if sysID != "" {
		path += "/" + url.PathEscape(sysID)
	}
	return c.do(ctx, method, path, query, payload)
}

// do performs one request against <base><path> with the x-sn-apikey header and,
// for a JSON payload, Accept/Content-Type application/json.
func (c *client) do(ctx context.Context, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("servicenow: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	full := c.base + path
	if enc := query.Encode(); enc != "" {
		full += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, method, full, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("servicenow: build request: %v", err), err: err}
	}
	req.Header.Set(apiKeyHeader, c.apiKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("servicenow: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("servicenow: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp.StatusCode, body)
	}
	return body, nil
}

// newAPIError builds an *apiError from ServiceNow's error body
// ({"error":{"message","detail"},"status":"failure"}), classifying 401/403 as a
// credential rejection so the engine can invalidate the stored key.
func newAPIError(status int, body []byte) error {
	message, detail := parseSNError(body)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	msg := fmt.Sprintf("servicenow API error (HTTP %d): %s", status, message)
	if detail != "" {
		msg += " — " + detail
	}
	e := &apiError{msg: msg, detail: detail, status: status, err: fmt.Errorf("%s", msg)}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		e.err = execution.RejectCredential(e.err)
	}
	return e
}

// parseSNError extracts message + detail from a ServiceNow error envelope.
func parseSNError(body []byte) (message, detail string) {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Detail  string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", ""
	}
	return env.Error.Message, env.Error.Detail
}

// resultObject unwraps a Table API response envelope {"result": …} to the bare
// result value. Table API always wraps single-record reads/writes in an object.
func unwrapResult(body []byte) (json.RawMessage, error) {
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("servicenow: decode response: %v", err), err: err}
	}
	if env.Result == nil {
		// No result field — hand back the raw body so nothing is silently lost.
		return json.RawMessage(body), nil
	}
	return env.Result, nil
}

// emitResult unwraps {"result": …} and writes the bare result JSON to stdout.
func (s *Service) emitResult(body []byte) error {
	result, err := unwrapResult(body)
	if err != nil {
		return err
	}
	return s.emitJSON(result)
}

// emitJSON writes a JSON value to stdout with a trailing newline.
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(bytes.TrimRight(body, "\n"), '\n'))
	return err
}
