package adobesign

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// httpClient is the minimal client seam so tests can inject an httptest client.
type httpClient = http.Client

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, invalid input, or an unreadable file. It maps to exit code 2
// and code "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Adobe non-2xx response or a transport
// failure. It maps to exit code 1 and code "api_error". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classification still resolves.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// base composes the v6 REST base path from the shard host, tolerating a
// trailing slash on base_uri (Adobe delivers api_access_point with one).
func base(baseURI string) string {
	return strings.TrimRight(baseURI, "/") + apiPath
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// call performs one JSON Acrobat Sign request and returns the raw response
// body. A non-2xx surfaces the body's message as an apiError carrying the HTTP
// status; a transport failure as an apiError with status 0.
func (s *Service) call(ctx context.Context, token, baseURI, method, path string, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("adobe-sign: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base(baseURI)+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, s.apiFailure(resp.StatusCode, body)
	}
	return body, nil
}

// callMultipart POSTs a single file part (Adobe transientDocuments upload). The
// file field is "File" per the v6 contract; extra text fields are optional.
func (s *Service) callMultipart(ctx context.Context, token, baseURI, path, fileName, contentType string, data []byte, fields map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			_ = mw.Close()
			return nil, &apiError{msg: fmt.Sprintf("adobe-sign: build multipart field: %v", err), err: err}
		}
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="File"; filename=%q`, fileName))
	if strings.TrimSpace(contentType) != "" {
		partHeader.Set("Content-Type", contentType)
	}
	part, err := mw.CreatePart(partHeader)
	if err != nil {
		_ = mw.Close()
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: build multipart file: %v", err), err: err}
	}
	if _, err := part.Write(data); err != nil {
		_ = mw.Close()
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: write multipart file: %v", err), err: err}
	}
	if err := mw.Close(); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: close multipart: %v", err), err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base(baseURI)+path, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: POST %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("adobe-sign: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, s.apiFailure(resp.StatusCode, body)
	}
	return body, nil
}

// download streams a binary GET (combinedDocument) to outPath, or to stdout
// when outPath is empty.
func (s *Service) download(ctx context.Context, token, baseURI, path, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base(baseURI)+path, nil)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("adobe-sign: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client().Do(req)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("adobe-sign: GET %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return s.apiFailure(resp.StatusCode, body)
	}
	var out io.Writer = s.stdout()
	if strings.TrimSpace(outPath) != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return &usageError{msg: fmt.Sprintf("adobe-sign: create %s: %v", outPath, err)}
		}
		defer f.Close()
		out = f
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return &apiError{msg: fmt.Sprintf("adobe-sign: write document: %v", err), err: err}
	}
	return nil
}

// apiFailure builds a classified apiError; a 401 is a credential rejection so
// the engine can invalidate the token, everything else is an ordinary failure.
func (s *Service) apiFailure(status int, body []byte) *apiError {
	raw := fmt.Errorf("adobe-sign API error (HTTP %d): %s", status, apiMessage(body))
	if status == http.StatusUnauthorized {
		return &apiError{msg: raw.Error(), status: status, err: execution.RejectCredential(raw)}
	}
	return &apiError{msg: raw.Error(), status: status, err: raw}
}

// apiMessage extracts Adobe's error message (code + message) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Code != "" || e.Message != "") {
		if e.Code != "" && e.Message != "" {
			return fmt.Sprintf("%s: %s", e.Code, e.Message)
		}
		return e.Code + e.Message
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// emitJSON writes a value as compact JSON to stdout.
func (s *Service) emitJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("adobe-sign: encode output: %v", err), err: err}
	}
	_, err = s.stdout().Write(append(b, '\n'))
	return err
}

// sourceMime infers a MIME type from a file extension, defaulting to PDF (the
// dominant Acrobat Sign document type).
func sourceMime(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/pdf"
}
