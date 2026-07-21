package helpscout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Help Scout non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// apiResponse is one Help Scout API response: reads carry a JSON body, writes
// answer 201/204 with an empty body and a Resource-Id header.
type apiResponse struct {
	status int
	body   []byte
	header http.Header
}

// resourceID returns the Resource-Id header (the id of the resource a write
// created), empty when absent. Go canonicalizes Help Scout's "Resource-ID"
// header key to "Resource-Id", so either lookup resolves it.
func (r *apiResponse) resourceID() string {
	return r.header.Get("Resource-Id")
}

// call performs one Help Scout API request with Bearer auth and returns the
// full response. A 401 marks the credential rejected; a 429 surfaces the
// documented X-RateLimit-Retry-After hint; any other non-2xx is an apiError
// carrying Help Scout's error message and the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) (*apiResponse, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("help-scout: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("help-scout: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("help-scout: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("help-scout: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := fmt.Sprintf("help-scout API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			if retry := resp.Header.Get("X-RateLimit-Retry-After"); retry != "" {
				msg = fmt.Sprintf("%s (retry after %ss)", msg, retry)
			}
		}
		raw := &apiError{msg: msg, status: resp.StatusCode}
		if resp.StatusCode == http.StatusUnauthorized {
			raw.err = execution.RejectCredential(errors.New(msg))
			return nil, raw
		}
		raw.err = errors.New(msg)
		return nil, raw
	}
	return &apiResponse{status: resp.StatusCode, body: body, header: resp.Header}, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (receipts) and writes it to stdout.
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("help-scout: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// emitReceipt writes a small {"id":…, "status":…} receipt for write endpoints
// that answer with an empty body. id is the affected resource id (the
// Resource-Id header for 201 creates, the target id for 204 mutations).
func (s *Service) emitReceipt(id, status string) error {
	return s.emitValue(map[string]any{"id": id, "status": status})
}

// apiMessage extracts Help Scout's error message from an error body, falling
// back to the raw body. Help Scout validation errors carry a top-level
// "message" plus an "_embedded.errors" array of field-level detail.
func apiMessage(body []byte) string {
	var e struct {
		Message  string `json:"message"`
		Embedded struct {
			Errors []struct {
				Path    string `json:"path"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if len(e.Embedded.Errors) > 0 {
			parts := make([]string, 0, len(e.Embedded.Errors))
			for _, fe := range e.Embedded.Errors {
				parts = append(parts, strings.TrimSpace(fe.Path+" "+fe.Message))
			}
			return e.Message + ": " + strings.Join(parts, "; ")
		}
		return e.Message
	}
	return string(body)
}
