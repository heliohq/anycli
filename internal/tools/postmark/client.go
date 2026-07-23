package postmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// call performs one Postmark API request with Server-token auth and returns the
// raw response body on success. Success is HTTP 2xx AND (ErrorCode absent OR
// == 0); any non-2xx, or a 2xx body carrying a non-zero ErrorCode, is an
// apiError surfacing Postmark's Message. A 401 (or a 2xx ErrorCode-10 body)
// marks the credential rejected.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("postmark: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("postmark: build request: %v", err), err: err}
	}
	req.Header.Set(serverTokenHeader, token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("postmark: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("postmark: read response: %v", err), status: resp.StatusCode, err: err}
	}

	code, message := postmarkEnvelope(body)
	statusOK := resp.StatusCode >= 200 && resp.StatusCode <= 299
	if statusOK && code == 0 {
		return body, nil
	}
	return nil, classify(resp.StatusCode, code, message, body)
}

// postmarkEnvelope extracts Postmark's {"ErrorCode","Message"} pair. A body with
// no ErrorCode field (a successful read) decodes to the (0, "") zero values,
// which the caller treats as success. A body that is not a JSON object at all
// (e.g. a bare array) likewise yields (0, "").
func postmarkEnvelope(body []byte) (int, string) {
	var e struct {
		ErrorCode int    `json:"ErrorCode"`
		Message   string `json:"Message"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return 0, ""
	}
	return e.ErrorCode, e.Message
}

// classify builds the apiError for a failed call and marks credential rejection
// for an auth failure. Postmark returns HTTP 401 for a missing/incorrect token
// (application ErrorCode 10); either signal rejects the credential.
func classify(status, errorCode int, message string, body []byte) error {
	detail := message
	if detail == "" {
		detail = string(body)
	}
	err := &apiError{
		msg:       fmt.Sprintf("postmark API error (HTTP %d, ErrorCode %d): %s", status, errorCode, detail),
		status:    status,
		errorCode: errorCode,
	}
	if status == http.StatusUnauthorized || errorCode == 10 {
		err.err = execution.RejectCredential(fmt.Errorf("%s", err.msg))
		return err
	}
	return err
}

// getAndEmit runs a GET and emits the provider JSON verbatim on success.
func (s *Service) getAndEmit(ctx context.Context, token, path string, query url.Values) error {
	raw, err := s.call(ctx, token, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	return s.emit(raw)
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (e.g. the redacted server view) and
// writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("postmark: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// decodeJSONObject validates a raw-JSON flag value that must be a JSON object
// (Postmark TemplateModel / Metadata), returning it for passthrough. A non-object
// or malformed value is a usageError (exit 2).
func decodeJSONObject(name, raw string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, usagef("postmark: --%s is not a valid JSON object: %v", name, err)
	}
	return v, nil
}
