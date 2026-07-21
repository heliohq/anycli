package signnow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, invalid JSON, or a bad argument. It maps to exit code 2 and
// kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a SignNow non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejected error still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// call performs one SignNow API request with Bearer auth and a JSON body,
// returning the raw response body. path is joined to the resolved base; query
// is appended when non-empty.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var body io.Reader
	headers := map[string]string{}
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("signnow: encode request: %v", err), err: err}
		}
		body = bytes.NewReader(b)
		headers["Content-Type"] = "application/json"
	}
	return s.do(ctx, token, method, path, query, body, headers)
}

// callMultipart posts a single-file multipart/form-data request (document
// upload). extraFields are written as text form fields alongside the file part.
func (s *Service) callMultipart(ctx context.Context, token, path string, extraFields map[string]string, fileField, fileName string, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range extraFields {
		if err := mw.WriteField(k, v); err != nil {
			_ = mw.Close()
			return nil, &apiError{msg: fmt.Sprintf("signnow: build multipart field: %v", err), err: err}
		}
	}
	part, err := mw.CreateFormFile(fileField, fileName)
	if err != nil {
		_ = mw.Close()
		return nil, &apiError{msg: fmt.Sprintf("signnow: build multipart file: %v", err), err: err}
	}
	if _, err := part.Write(data); err != nil {
		_ = mw.Close()
		return nil, &apiError{msg: fmt.Sprintf("signnow: write multipart file: %v", err), err: err}
	}
	if err := mw.Close(); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("signnow: close multipart: %v", err), err: err}
	}
	return s.do(ctx, token, http.MethodPost, path, nil, &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
}

// callRaw performs a GET whose successful body is returned verbatim (binary
// download). A non-2xx is still parsed for SignNow's error dialects.
func (s *Service) callRaw(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	return s.do(ctx, token, http.MethodGet, path, query, nil, nil)
}

// do is the shared request path: Bearer auth, optional headers, SignNow error
// classification. A 401 (or an auth error code) rejects the credential; any
// other non-2xx surfaces SignNow's message as an apiError carrying the status.
func (s *Service) do(ctx context.Context, token, method, path string, query url.Values, body io.Reader, headers map[string]string) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("signnow: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("signnow: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("signnow: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		base := fmt.Errorf("signnow API error (HTTP %d): %s", resp.StatusCode, apiMessage(raw))
		classified := base
		if resp.StatusCode == http.StatusUnauthorized {
			classified = execution.RejectCredential(base)
		}
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return raw, nil
}

// apiMessage extracts a SignNow error message from a response body, handling
// both dialects — the current {"errors":[{"code","message"}]} and the legacy
// {"error":"..."} — and falling back to the raw body.
func apiMessage(body []byte) string {
	var current struct {
		Errors []struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &current); err == nil && len(current.Errors) > 0 {
		parts := make([]string, 0, len(current.Errors))
		for _, e := range current.Errors {
			switch {
			case e.Message != "":
				parts = append(parts, e.Message)
			case e.Code != nil:
				parts = append(parts, fmt.Sprintf("%v", e.Code))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	var legacy struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &legacy); err == nil && legacy.Error != "" {
		return legacy.Error
	}
	return strings.TrimSpace(string(body))
}

// emitRaw writes a provider JSON body to stdout verbatim (+ newline), used for
// responses whose useful shape (signing URLs, invite receipts) varies and is
// small enough to pass through unprojected. An empty body becomes {}.
func (s *Service) emitRaw(body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		trimmed = "{}"
	}
	if _, err := io.WriteString(s.stdout(), trimmed+"\n"); err != nil {
		return err
	}
	return nil
}

// emitJSON writes a client-side value as JSON to stdout (+ newline).
func (s *Service) emitJSON(value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("signnow: encode output: %v", err), err: err}
	}
	if _, err := s.stdout().Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}
