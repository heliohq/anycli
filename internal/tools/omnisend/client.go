package omnisend

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

// usageError is a parameter / usage error: a missing required flag, a bad enum
// value, or invalid JSON. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Omnisend non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejected marker still resolves through it.
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

// call performs one Omnisend API request with Bearer auth and the pinned
// version header, returning the raw response body. A 401 marks the credential
// rejected; any other non-2xx is an apiError carrying Omnisend's message and
// the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("omnisend: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("omnisend: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Omnisend-Version", omnisendVersion)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("omnisend: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("omnisend: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("omnisend API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
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

// apiMessage extracts Omnisend's error message from an error body, falling back
// to the raw body. Omnisend's dated line returns either {"error":{"message":…}}
// or a flat {"message":…}; handle both.
func apiMessage(body []byte) string {
	var nested struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &nested); err == nil {
		if nested.Error.Message != "" {
			return nested.Error.Message
		}
		if nested.Message != "" {
			return nested.Message
		}
	}
	return string(body)
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. An invalid value is a usage error.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("omnisend: --%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// registerListFlags wires the shared --limit / --after pagination flags onto a
// list command. limit is sent only when > 0; after (the opaque cursor from
// paging.cursors.after) is sent only when non-empty.
func registerListFlags(cmd *cobra.Command, limit *int, after *string) {
	cmd.Flags().IntVar(limit, "limit", 0, "max items per page (0 = provider default)")
	cmd.Flags().StringVar(after, "after", "", "pagination cursor from paging.cursors.after")
}

// applyListQuery writes the pagination flags into a query value set.
func applyListQuery(q url.Values, limit int, after string) {
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if after != "" {
		q.Set("after", after)
	}
}
