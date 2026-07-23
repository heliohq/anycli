package salesforce

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

// client performs one Salesforce REST request per call, Bearer-authenticated
// against the connection's instance_url base.
type client struct {
	token string
	base  string // instance_url, trailing slash stripped
	hc    *http.Client
}

// usageError is a parameter / usage error: bad flag combination, missing
// required flag/arg, or invalid JSON payload. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Salesforce non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport failures). It wraps the underlying cause so
// errors.As for the execution credential-rejected classification resolves
// through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// dataPath builds a versioned Platform REST path, e.g.
// /services/data/v65.0/query.
func dataPath(version, suffix string) string {
	return "/services/data/" + version + suffix
}

// call performs one Salesforce request with Bearer auth. path is either an
// absolute path (starting with /) or is joined onto the instance base. A
// non-2xx surfaces the array error body's first errorCode/message as an
// apiError carrying the HTTP status; a 401 is additionally classified as a
// credential rejection. body is nil for GET/DELETE.
func (c *client) call(ctx context.Context, method, path string, payload []byte) ([]byte, int, error) {
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reqBody)
	if err != nil {
		return nil, 0, &apiError{msg: fmt.Sprintf("salesforce: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, &apiError{msg: fmt.Sprintf("salesforce: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, &apiError{msg: fmt.Sprintf("salesforce: read response: %v", err), status: resp.StatusCode, err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("salesforce API error (HTTP %d): %s", resp.StatusCode, apiMessage(respBody))
		classified := raw
		if resp.StatusCode == http.StatusUnauthorized {
			classified = execution.RejectCredential(raw)
		}
		return nil, resp.StatusCode, &apiError{msg: raw.Error(), status: resp.StatusCode, err: classified}
	}
	return respBody, resp.StatusCode, nil
}

// get is call for a GET with no body.
func (c *client) get(ctx context.Context, path string) ([]byte, int, error) {
	return c.call(ctx, http.MethodGet, path, nil)
}

// apiMessage extracts Salesforce's error message. Salesforce error bodies are
// JSON ARRAYS ([{"errorCode","message"}]); a few identity/limits endpoints use
// an object ({"error","error_description"}). Both are handled, falling back to
// the raw body.
func apiMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		var arr []struct {
			ErrorCode string `json:"errorCode"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
			e := arr[0]
			if e.ErrorCode != "" || e.Message != "" {
				return fmt.Sprintf("%s: %s", e.ErrorCode, e.Message)
			}
		}
	}
	var obj struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal(body, &obj); err == nil {
		switch {
		case obj.Error != "" && obj.Description != "":
			return fmt.Sprintf("%s: %s", obj.Error, obj.Description)
		case obj.Error != "":
			return obj.Error
		case obj.Message != "":
			return obj.Message
		}
	}
	return trimmed
}
