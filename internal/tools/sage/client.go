package sage

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

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: a missing required flag, invalid
// JSON, or a bad flag value. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Sage non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejection classification still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// emitJSON writes the provider's JSON response to stdout verbatim (output is
// provider-neutral: the caller receives Sage's own resource / list envelope).
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// businessFlag returns the resolved --business persistent flag value (empty →
// Sage falls back to the user's lead business via an omitted X-Business header).
func businessFlag(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("business")
	return strings.TrimSpace(v)
}

// withPaging appends Sage's page / items_per_page query params to a path when
// set. Sage list responses carry the $items / $total / $next envelope, so the
// caller continues by re-requesting with the next page number.
func withPaging(path string, page, itemsPerPage int) string {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if itemsPerPage > 0 {
		q.Set("items_per_page", strconv.Itoa(itemsPerPage))
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// call performs one Sage API request: Bearer auth on every call, the X-Business
// header when business is non-empty, and a JSON Content-Type when a payload is
// sent. A non-2xx surfaces the body's error detail as an apiError carrying the
// HTTP status (401 is additionally classified as a credential rejection); a
// transport failure surfaces as an apiError with status 0.
func (s *Service) call(ctx context.Context, token, business, method, path string, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("sage: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("sage: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if business != "" {
		req.Header.Set("X-Business", business)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("sage: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("sage: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("sage API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyCredentialError(resp.StatusCode, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// classifyCredentialError marks a 401 as an explicit credential rejection so
// the Helio token gateway can distinguish a stale/invalid access token (which
// warrants a refresh / re-consent) from an ordinary permission or input error.
func classifyCredentialError(status int, err error) error {
	if status == http.StatusUnauthorized {
		return execution.RejectCredential(err)
	}
	return err
}

// apiMessage extracts a human-readable message from a Sage error body. Sage
// v3.1 returns errors as a JSON array of objects carrying $message / $dataCode;
// some responses use a single object. Both are handled, falling back to the raw
// body.
func apiMessage(body []byte) string {
	type sageErr struct {
		Message  string `json:"$message"`
		DataCode string `json:"$dataCode"`
	}
	var arr []sageErr
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
		var parts []string
		for _, e := range arr {
			if m := errText(e.Message, e.DataCode); m != "" {
				parts = append(parts, m)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	var one sageErr
	if err := json.Unmarshal(body, &one); err == nil {
		if m := errText(one.Message, one.DataCode); m != "" {
			return m
		}
	}
	return strings.TrimSpace(string(body))
}

// errText joins a Sage error's message and data code into one clause.
func errText(message, dataCode string) string {
	switch {
	case message != "" && dataCode != "":
		return fmt.Sprintf("%s (%s)", message, dataCode)
	case message != "":
		return message
	case dataCode != "":
		return dataCode
	default:
		return ""
	}
}
