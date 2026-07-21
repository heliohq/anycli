package formstack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// itoa renders an int as a base-10 query/param value.
func itoa(n int) string { return strconv.Itoa(n) }

// EncryptionPasswordHeader carries the per-request password for encrypted forms
// (submission read). It is form data, not a stored credential.
const EncryptionPasswordHeader = "X-FS-ENCRYPTION-PASSWORD"

// call performs one Formstack API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is a
// plain error carrying Formstack's "error" message. Extra per-request headers
// (e.g. the encryption password) are applied when non-empty.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any, headers map[string]string) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("formstack: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("formstack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("formstack: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("formstack: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := fmt.Errorf("formstack API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
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

// apiMessage extracts Formstack's error text from an error body, falling back
// to the raw body. Formstack v2 encodes failures as {"error": "..."}; some
// endpoints use {"status":"error","error":"..."}.
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
	return string(body)
}
