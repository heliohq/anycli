package dropboxsign

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, or a bad value. It maps to exit code 2 and kind
// "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Dropbox Sign non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection classification still resolves through
// it.
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

// formPart is one ordered multipart/form-data field. Ordering is preserved so
// bracket-indexed arrays (files[0], signers[0][email_address]) emit
// deterministically; file parts carry a filePath to stream from disk.
type formPart struct {
	name     string
	value    string
	filePath string // non-empty => a file upload part read from this path
}

// callGET performs a GET with Bearer auth and returns the raw response body.
func (s *Service) callGET(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: build request: %v", err), err: err}
	}
	return s.do(req, token)
}

// callJSON performs a POST with a JSON body and Bearer auth.
func (s *Service) callJSON(ctx context.Context, token, path string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: encode request: %v", err), err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+path, bytes.NewReader(b))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: build request: %v", err), err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	return s.do(req, token)
}

// callPost performs a POST with no body (Bearer auth) — used by cancel.
func (s *Service) callPost(ctx context.Context, token, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+path, nil)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: build request: %v", err), err: err}
	}
	return s.do(req, token)
}

// callMultipart performs a POST with a multipart/form-data body assembled from
// ordered parts. String parts become form fields; parts with a filePath are
// streamed from disk as file uploads. This is the send / send-with-template
// path: Dropbox Sign's v3 send surface takes bracket-indexed array fields
// (files[0], file_urls[0], signers[0][email_address], …) plus binary file
// parts.
func (s *Service) callMultipart(ctx context.Context, token, path string, parts []formPart) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, p := range parts {
		if p.filePath != "" {
			if err := writeFilePart(mw, p.name, p.filePath); err != nil {
				return nil, &usageError{msg: err.Error()}
			}
			continue
		}
		if err := mw.WriteField(p.name, p.value); err != nil {
			return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: encode form field %q: %v", p.name, err), err: err}
		}
	}
	if err := mw.Close(); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: finalize form: %v", err), err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+path, &body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: build request: %v", err), err: err}
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.do(req, token)
}

// writeFilePart streams one on-disk file into the multipart writer as field
// name. A missing/unreadable file is surfaced as an error the caller maps to a
// usage error (a bad --file path is the operator's mistake, not the API's).
func writeFilePart(mw *multipart.Writer, field, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("dropbox-sign: open --file %q: %w", path, err)
	}
	defer f.Close()
	pw, err := mw.CreateFormFile(field, filepath.Base(path))
	if err != nil {
		return fmt.Errorf("dropbox-sign: create file part %q: %w", field, err)
	}
	if _, err := io.Copy(pw, f); err != nil {
		return fmt.Errorf("dropbox-sign: read --file %q: %w", path, err)
	}
	return nil
}

// do sends req with Bearer auth + Accept: application/json and classifies the
// response. A non-2xx surfaces Dropbox Sign's error_name/error_msg as an
// apiError carrying the HTTP status; a 401 is additionally marked as a
// credential rejection; a transport failure is an apiError with status 0.
func (s *Service) do(req *http.Request, token string) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: %s %s: %v", req.Method, req.URL.Path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("dropbox-sign: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("dropbox-sign API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: execution.RejectCredential(raw)}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (receipts / envelopes) to stdout.
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("dropbox-sign: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// apiMessage extracts Dropbox Sign's error_name/error_msg from an error body,
// falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error struct {
			ErrorMsg  string `json:"error_msg"`
			ErrorName string `json:"error_name"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error.ErrorMsg != "" || e.Error.ErrorName != "") {
		switch {
		case e.Error.ErrorName != "" && e.Error.ErrorMsg != "":
			return e.Error.ErrorName + ": " + e.Error.ErrorMsg
		case e.Error.ErrorName != "":
			return e.Error.ErrorName
		default:
			return e.Error.ErrorMsg
		}
	}
	return strings.TrimSpace(string(body))
}
