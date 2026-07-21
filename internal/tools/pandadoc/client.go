package pandadoc

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
// required flag, invalid JSON, or a malformed argument. It maps to exit code 2
// and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a PandaDoc non-2xx response, a transport
// failure, or a poll timeout. It maps to exit code 1 and kind "api". status is
// the HTTP status (0 for transport/network failures). It wraps the underlying
// cause so errors.As for the credential-rejection marker still resolves.
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

// call performs one PandaDoc JSON API request and returns the raw response
// body. authz is the full Authorization header value (scheme + credential). A
// non-2xx surfaces the body's type/detail as an apiError carrying the HTTP
// status; a 401 marks the credential rejected; a transport failure is an
// apiError with status 0.
func (s *Service) call(ctx context.Context, authz, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("pandadoc: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pandadoc: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", authz)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pandadoc: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pandadoc: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("pandadoc API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// download performs a binary GET (a signed-PDF download) and returns the raw
// bytes. Errors follow the same classification as call.
func (s *Service) download(ctx context.Context, authz, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL()+path, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pandadoc: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", authz)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pandadoc: GET %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("pandadoc: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("pandadoc API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so the
// host invalidates the cached credential; other statuses pass through.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts PandaDoc's error message from an error body. PandaDoc
// error bodies vary (type/detail, or a validation map under detail), so we
// surface type + detail when present and fall back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Type   string          `json:"type"`
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Type != "" || len(e.Detail) > 0) {
		detail := strings.TrimSpace(string(e.Detail))
		// A string detail is the common case; unquote it for readability.
		var asString string
		if json.Unmarshal(e.Detail, &asString) == nil {
			detail = asString
		}
		switch {
		case e.Type != "" && detail != "":
			return e.Type + ": " + detail
		case e.Type != "":
			return e.Type
		default:
			return detail
		}
	}
	return strings.TrimSpace(string(body))
}

// emitJSON writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emitJSON(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (download receipts) to stdout.
func (s *Service) emitValue(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("pandadoc: encode output: %v", err), err: err}
	}
	return s.emitJSON(b)
}
