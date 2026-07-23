package ahrefs

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

// usageError is a parameter / usage error: an illegal flag combination, a
// missing required flag, a bad enum, or an invalid value. It maps to exit
// code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Ahrefs non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// baseURL returns the configured base or the production default (no trailing
// slash).
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

// call performs one Ahrefs API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying the HTTP status and Ahrefs' error message. payload, when
// non-nil, is JSON-encoded as the request body (the batch-analysis POST).
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("ahrefs: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("ahrefs: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("ahrefs: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("ahrefs: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		base := fmt.Errorf("ahrefs API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := base
		if resp.StatusCode == http.StatusUnauthorized {
			classified = execution.RejectCredential(base)
		}
		return nil, &apiError{msg: base.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// apiMessage extracts Ahrefs' {"error": "<message>"} text from an error body,
// falling back to the raw body when the shape does not match.
func apiMessage(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error != "" {
		return e.Error
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(no response body)"
	}
	return trimmed
}

// rawJSON wraps a provider response body so it re-marshals verbatim inside a
// tool-fabricated object (the merged domain overview), instead of being
// re-encoded field by field.
func rawJSON(b []byte) json.RawMessage { return json.RawMessage(b) }

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a tool-fabricated value (the merged domain overview) and
// writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("ahrefs: encode output: %v", err), err: err}
	}
	return s.emit(body)
}

// rowFlags holds the shared filter grammar every rows-returning endpoint
// accepts. select is required by the API; each command supplies a curated
// default so an agent never accidentally requests the full (expensive) field
// set. where/order_by are passed through verbatim (Ahrefs' documented filter
// syntax); the CLI invents no DSL of its own.
type rowFlags struct {
	selectFields string
	where        string
	orderBy      string
	limit        int
	offset       int
	mode         string
	protocol     string
}

// registerRowFlags wires the shared rows flags onto cmd with the command's
// curated default select and a unit-safe default limit of 10. withTargetMode
// adds --mode/--protocol for the site-explorer target endpoints (keywords
// explorer rows have no target, so they omit them).
func registerRowFlags(cmd *cobra.Command, rf *rowFlags, defaultSelect string, withTargetMode bool) {
	cmd.Flags().StringVar(&rf.selectFields, "select", defaultSelect, "comma-separated fields to return (unit cost scales with fields)")
	cmd.Flags().StringVar(&rf.where, "where", "", "Ahrefs filter expression, passed through verbatim")
	cmd.Flags().StringVar(&rf.orderBy, "order-by", "", "order_by expression, e.g. \"traffic:desc\"")
	cmd.Flags().IntVar(&rf.limit, "limit", 10, "max rows to return (default 10 for unit safety)")
	cmd.Flags().IntVar(&rf.offset, "offset", 0, "row offset for pagination")
	if withTargetMode {
		cmd.Flags().StringVar(&rf.mode, "mode", "", "target mode: exact|prefix|domain|subdomains")
		cmd.Flags().StringVar(&rf.protocol, "protocol", "", "protocol: both|http|https")
	}
}

// apply writes the rows flags into q. select is always sent; the rest are
// omitted when empty/zero so the request stays minimal.
func (rf rowFlags) apply(q url.Values) {
	q.Set("select", rf.selectFields)
	if rf.where != "" {
		q.Set("where", rf.where)
	}
	if rf.orderBy != "" {
		q.Set("order_by", rf.orderBy)
	}
	if rf.limit > 0 {
		q.Set("limit", strconv.Itoa(rf.limit))
	}
	if rf.offset > 0 {
		q.Set("offset", strconv.Itoa(rf.offset))
	}
	if rf.mode != "" {
		q.Set("mode", rf.mode)
	}
	if rf.protocol != "" {
		q.Set("protocol", rf.protocol)
	}
}
