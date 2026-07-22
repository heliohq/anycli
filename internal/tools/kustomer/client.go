package kustomer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required argument, or invalid JSON. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Kustomer non-2xx response or a transport
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

// emitJSON writes the provider's JSON response to stdout verbatim.
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Kustomer API request with Bearer auth. A non-2xx surfaces
// the body's error message as an apiError carrying the HTTP status (a 401 is
// additionally classified as a credential rejection); a transport failure is an
// apiError with status 0.
func (s *Service) call(ctx context.Context, base, token, method, path string, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("kustomer: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("kustomer: build request: %v", err), err: err}
	}
	// A space after "Bearer" is required by Kustomer.
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
		return nil, &apiError{msg: fmt.Sprintf("kustomer: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("kustomer: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("kustomer API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so
// the engine can invalidate the resolved token; other failures are left
// unclassified (the token may still be valid).
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts Kustomer's error text from an error body, falling back to
// the raw body. Kustomer error bodies carry an `errors` array of
// {status,title,detail} objects (JSON:API), or a flat {message} shape.
func apiMessage(body []byte) string {
	var japi struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &japi); err == nil && len(japi.Errors) > 0 {
		parts := make([]string, 0, len(japi.Errors))
		for _, e := range japi.Errors {
			switch {
			case e.Detail != "" && e.Title != "":
				parts = append(parts, e.Title+": "+e.Detail)
			case e.Detail != "":
				parts = append(parts, e.Detail)
			case e.Title != "":
				parts = append(parts, e.Title)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	var flat struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && flat.Message != "" {
		return flat.Message
	}
	return strings.TrimSpace(string(body))
}

// readBody resolves a raw JSON request body from either --data or --file. The
// two are mutually exclusive; a missing/empty body is a usage error for a write
// command; invalid JSON is a usage error. It returns the parsed value ready to
// re-marshal so the request carries canonical JSON.
func readBody(data, file string) (any, error) {
	if data != "" && file != "" {
		return nil, &usageError{msg: "--data and --file are mutually exclusive"}
	}
	raw := data
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --file %s: %v", file, err)}
		}
		raw = string(b)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, &usageError{msg: "a request body is required (use --data '<json>' or --file <path>)"}
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("request body is not valid JSON: %v", err)}
	}
	return v, nil
}

// buildQuery composes the query string for a list request from the pagination
// convenience flags and any repeated --query key=value pairs. Explicit --query
// pairs win over the convenience flags on key collision (they append after).
func buildQuery(page, pageSize int, extra []string) (string, error) {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(pageSize))
	}
	for _, kv := range extra {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return "", &usageError{msg: fmt.Sprintf("--query must be key=value, got %q", kv)}
		}
		q.Add(k, v)
	}
	if len(q) == 0 {
		return "", nil
	}
	return "?" + q.Encode(), nil
}
