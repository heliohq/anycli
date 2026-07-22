package kit

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
// required flag, or a bad value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Kit non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejection sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Kit API request with Bearer auth and returns the raw body.
// A non-2xx surfaces the body's error message as an apiError carrying the HTTP
// status (401 additionally marks the credential rejected); a transport failure
// is an apiError with status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	full := base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("kit: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("kit: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("kit: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("kit: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("kit API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so
// the engine can invalidate the stored token; other statuses pass through.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts Kit's error text. V4 returns {"errors":[...]}; some
// endpoints use {"error":"..."}. Falls back to the raw body.
func apiMessage(body []byte) string {
	var envelope struct {
		Errors []string `json:"errors"`
		Error  string   `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		if len(envelope.Errors) > 0 {
			return strings.Join(envelope.Errors, "; ")
		}
		if envelope.Error != "" {
			return envelope.Error
		}
	}
	return strings.TrimSpace(string(body))
}

// emitData writes the provider-neutral success envelope to stdout. It lifts the
// resource out of Kit's per-endpoint wrapper into a stable "data" key, and
// carries any cursor "pagination" object alongside it, so the agent sees the
// same shape regardless of the endpoint's wrapper name. When wrapKey is empty
// or absent, the whole decoded object becomes data (e.g. GET /account).
func (s *Service) emitData(body []byte, wrapKey string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		// Not a JSON object — emit the raw body under data unchanged.
		return s.writeJSON(map[string]any{"data": json.RawMessage(body)})
	}
	out := map[string]any{}
	if lifted, ok := raw[wrapKey]; wrapKey != "" && ok {
		out["data"] = lifted
	} else {
		out["data"] = raw
	}
	if pag, ok := raw["pagination"]; ok {
		out["pagination"] = pag
	}
	return s.writeJSON(out)
}

// writeJSON marshals v and writes it to stdout with a trailing newline.
func (s *Service) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("kit: encode output: %v", err), err: err}
	}
	_, err = s.stdout().Write(append(b, '\n'))
	return err
}
