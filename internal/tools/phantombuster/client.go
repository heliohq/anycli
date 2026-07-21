package phantombuster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// apiError is a runtime / API error: a PhantomBuster non-2xx response or a
// transport failure. It maps to exit code 1 and error code "api". status is the
// HTTP status (0 for transport/network failures and pre-request checks). It
// wraps the underlying cause so errors.As for *credentialRejectedError still
// resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one PhantomBuster API request with the X-Phantombuster-Key
// header and returns the raw response body. A 401/403 marks the credential
// rejected; any other non-2xx is an apiError carrying PhantomBuster's status +
// message.
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("phantombuster: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("phantombuster: build request: %v", err), err: err}
	}
	req.Header.Set(authHeader, key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("phantombuster: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("phantombuster: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{
			msg:    fmt.Sprintf("phantombuster API error (HTTP %d): %s", resp.StatusCode, apiMessage(body)),
			status: resp.StatusCode,
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return body, nil
}

// apiMessage extracts PhantomBuster's error text from a non-2xx body. v2 errors
// carry either a top-level "error" string or a {status,error} envelope; fall
// back to the raw body when neither is present.
func apiMessage(body []byte) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Error != "":
			return e.Error
		case e.Message != "":
			return e.Message
		}
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// emitObject decodes a raw provider object, merges the caller's normalized
// fields, adds ISO-8601 mirrors for the ms timestamps, and writes it under the
// success envelope's data key. augment values overwrite any same-named provider
// key so the normalized poll fields are authoritative.
func (s *Service) emitObject(raw []byte, augment map[string]any) error {
	obj := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &obj); err != nil {
			return &apiError{msg: fmt.Sprintf("phantombuster: decode response: %v", err), err: err}
		}
	}
	addISOMirrors(obj)
	addNormalizedFields(obj)
	for k, v := range augment {
		obj[k] = v
	}
	return s.emitData(obj)
}

// emitItems decodes a raw provider array and writes it under data.items. A null
// or empty body yields an empty slice so data.items is always an array.
func (s *Service) emitItems(raw []byte) error {
	items := []any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &items); err != nil {
			return &apiError{msg: fmt.Sprintf("phantombuster: decode response: %v", err), err: err}
		}
	}
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			addISOMirrors(m)
		}
	}
	return s.emitData(map[string]any{"items": items})
}

// emitData writes {"ok":true,"data":<v>} to stdout (+ newline).
func (s *Service) emitData(v any) error {
	b, err := json.Marshal(map[string]any{"ok": true, "data": v})
	if err != nil {
		return &apiError{msg: fmt.Sprintf("phantombuster: encode output: %v", err), err: err}
	}
	_, err = s.stdout().Write(append(b, '\n'))
	return err
}

// addISOMirrors adds a "<key>_iso" RFC-3339 mirror for every key ending in "At"
// whose value is a JSON number of Unix milliseconds (v2 timestamps are ms).
// This is forward-compatible: new timestamp fields get a mirror automatically.
func addISOMirrors(obj map[string]any) {
	for k, v := range obj {
		if !strings.HasSuffix(k, "At") {
			continue
		}
		num, ok := v.(float64)
		if !ok {
			continue
		}
		mirror := k + "_iso"
		if _, exists := obj[mirror]; exists {
			continue
		}
		obj[mirror] = time.UnixMilli(int64(num)).UTC().Format(time.RFC3339)
	}
}

// addNormalizedFields adds stable snake_case poll-loop fields so the AI's
// incremental loop reads a fixed contract rather than provider-versioned camel
// keys: output_pos (the fetch-output cursor to pass back as --from-pos) and
// is_running (from the authoritative isAgentRunning boolean, else derived from a
// "running" status). Explicit augment values still override these.
func addNormalizedFields(obj map[string]any) {
	if v, ok := obj["outputPos"].(float64); ok {
		obj["output_pos"] = int64(v)
	}
	if v, ok := obj["isAgentRunning"].(bool); ok {
		obj["is_running"] = v
	} else if st, ok := obj["status"].(string); ok {
		obj["is_running"] = st == "running"
	}
}

// itoa renders an int as a base-10 query value.
func itoa(n int) string {
	return strconv.Itoa(n)
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		// A bad --argument is a usage error (exit 2), not an apiError.
		return nil, fmt.Errorf("--%s is not valid JSON: %w", name, err)
	}
	return v, nil
}
