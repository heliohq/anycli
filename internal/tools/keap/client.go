package keap

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

// usageError is a parameter / usage error: a missing required flag, a bad enum,
// invalid JSON, or an unresolvable id. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Keap non-2xx response or a transport
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

// call performs one Keap API request with Bearer auth and returns the raw
// response body. A 401 marks the credential rejected; any other non-2xx is an
// apiError carrying Keap's message and the HTTP status.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("keap: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("keap: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("keap: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("keap: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("keap API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		cause := raw
		if resp.StatusCode == http.StatusUnauthorized {
			cause = execution.RejectCredential(raw)
		}
		return nil, &apiError{msg: cause.Error(), status: resp.StatusCode, err: cause}
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

// apiMessage extracts Keap's error message from an error body, falling back to
// the raw body. Keap v2 errors carry a top-level "message"; some legacy shapes
// use "error"/"error_description".
func apiMessage(body []byte) string {
	var e struct {
		Message          string `json:"message"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		switch {
		case e.Message != "":
			return e.Message
		case e.Error != "" && e.ErrorDescription != "":
			return e.Error + ": " + e.ErrorDescription
		case e.Error != "":
			return e.Error
		}
	}
	return strings.TrimSpace(string(body))
}

// listFlags are the shared v2 list query params, mapped 1:1 to the API.
type listFlags struct {
	pageSize  int
	pageToken string
	filter    string
	orderBy   string
	fields    string
}

// registerListFlags attaches the standard list flags to a list command.
func registerListFlags(cmd *cobra.Command) *listFlags {
	lf := &listFlags{}
	cmd.Flags().IntVar(&lf.pageSize, "page-size", 0, "max items per page")
	cmd.Flags().StringVar(&lf.pageToken, "page-token", "", "resume from a prior response's next_page_token")
	cmd.Flags().StringVar(&lf.filter, "filter", "", "v2 filter expression (e.g. email==a@b.com)")
	cmd.Flags().StringVar(&lf.orderBy, "order-by", "", "sort field, optionally with a direction")
	cmd.Flags().StringVar(&lf.fields, "fields", "", "comma-separated fields to include")
	return lf
}

// values renders the list flags into a query value set, omitting empty ones.
func (lf *listFlags) values() url.Values {
	q := url.Values{}
	if lf.pageSize > 0 {
		q.Set("page_size", strconv.Itoa(lf.pageSize))
	}
	if lf.pageToken != "" {
		q.Set("page_token", lf.pageToken)
	}
	if lf.filter != "" {
		q.Set("filter", lf.filter)
	}
	if lf.orderBy != "" {
		q.Set("order_by", lf.orderBy)
	}
	if lf.fields != "" {
		q.Set("fields", lf.fields)
	}
	return q
}

// fieldsQuery builds a query carrying only --fields, used by get verbs.
func fieldsQuery(fields string) url.Values {
	q := url.Values{}
	if fields != "" {
		q.Set("fields", fields)
	}
	return q
}

// applyJSONBody overlays a raw --json-body payload onto a body map built from
// convenience flags. json-body keys win, so it can add custom_fields or
// override any convenience-mapped field. An empty raw value is a no-op.
func applyJSONBody(base map[string]any, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var overlay map[string]any
	if err := json.Unmarshal([]byte(raw), &overlay); err != nil {
		return &usageError{msg: fmt.Sprintf("--json-body is not a valid JSON object: %v", err)}
	}
	for k, v := range overlay {
		base[k] = v
	}
	return nil
}

// requireBody fails with a usage error when a create/update body carries no
// fields (neither convenience flags nor --json-body supplied anything).
func requireBody(body map[string]any) error {
	if len(body) == 0 {
		return &usageError{msg: "no fields to send; pass at least one field flag or --json-body"}
	}
	return nil
}
