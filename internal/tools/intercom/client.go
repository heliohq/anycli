package intercom

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
	"github.com/spf13/cobra"
)

// intToString renders an int as a base-10 query value.
func intToString(n int) string { return strconv.Itoa(n) }

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, or invalid JSON. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Intercom non-2xx response or a
// transport failure. It maps to exit code 1 and kind "api". status is the HTTP
// status (0 for transport/network failures). It wraps the underlying cause so
// errors.As for the credential-rejection sentinel still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Intercom API request on the pinned Intercom-Version with
// Bearer auth and returns the raw response body. A 401 marks the credential
// rejected; any other non-2xx surfaces Intercom's error.list message(s) as an
// apiError carrying the HTTP status; a transport failure is an apiError with
// status 0.
func (s *Service) call(ctx context.Context, token, method, path string, query url.Values, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("intercom: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("intercom: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Intercom-Version", intercomVersion)
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
		return nil, &apiError{msg: fmt.Sprintf("intercom: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("intercom: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("intercom API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
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

// apiMessage extracts Intercom's error.list message(s) from an error body,
// falling back to the raw body. Intercom returns
// {"type":"error.list","errors":[{"code":"...","message":"..."}]}.
func apiMessage(body []byte) string {
	var e struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && len(e.Errors) > 0 {
		out := ""
		for i, er := range e.Errors {
			if i > 0 {
				out += "; "
			}
			switch {
			case er.Code != "" && er.Message != "":
				out += er.Code + ": " + er.Message
			case er.Code != "":
				out += er.Code
			default:
				out += er.Message
			}
		}
		if out != "" {
			return out
		}
	}
	return string(body)
}

// resolveAdminID returns explicit when non-empty, otherwise reads the
// authenticating admin's id from GET /me (the acting teammate). The lookup runs
// at most once per command invocation (callers resolve lazily, only when the
// --admin-id flag is absent).
func (s *Service) resolveAdminID(ctx context.Context, token, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	body, err := s.call(ctx, token, http.MethodGet, "/me", nil, nil)
	if err != nil {
		return "", err
	}
	var me struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return "", &apiError{msg: fmt.Sprintf("intercom: decode /me: %v", err), err: err}
	}
	if me.ID == "" {
		return "", &usageError{msg: "intercom: could not resolve acting admin id from /me; pass --admin-id explicitly"}
	}
	return me.ID, nil
}

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. A parse failure is a usage error.
func decodeJSONFlag(name, raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("intercom: --%s is not valid JSON: %v", name, err)}
	}
	return v, nil
}

// searchFlags holds the shared search inputs: a raw Intercom query object
// (--query), and cursor pagination (--per-page / --starting-after).
type searchFlags struct {
	query         string
	perPage       int
	startingAfter string
}

// registerSearchFlags wires --query / --per-page / --starting-after onto a
// search command. Per-resource convenience filter flags are registered by the
// caller.
func registerSearchFlags(cmd *cobra.Command, sf *searchFlags) {
	cmd.Flags().StringVar(&sf.query, "query", "", "raw Intercom query object as JSON (mutually exclusive with convenience filters)")
	cmd.Flags().IntVar(&sf.perPage, "per-page", 0, "results per page (Intercom default 20, max 150)")
	cmd.Flags().StringVar(&sf.startingAfter, "starting-after", "", "pagination cursor from pages.next.starting_after")
}

// buildSearchBody assembles a POST /<resource>/search request body. Exactly one
// query source is allowed: a raw --query JSON object, OR the compiled
// convenience filters — supplying both is a usage error. Compiled filters are
// wrapped in an AND group. Pagination is attached when either field is set.
func buildSearchBody(sf searchFlags, filters []map[string]any) (map[string]any, error) {
	if sf.query != "" && len(filters) > 0 {
		return nil, &usageError{msg: "intercom: --query and convenience filter flags are mutually exclusive"}
	}
	body := map[string]any{}
	switch {
	case sf.query != "":
		v, err := decodeJSONFlag("query", sf.query)
		if err != nil {
			return nil, err
		}
		body["query"] = v
	case len(filters) == 1:
		body["query"] = filters[0]
	case len(filters) > 1:
		body["query"] = map[string]any{"operator": "AND", "value": filters}
	default:
		return nil, &usageError{msg: "intercom: a search needs --query or at least one convenience filter flag"}
	}
	if sf.perPage > 0 || sf.startingAfter != "" {
		pagination := map[string]any{}
		if sf.perPage > 0 {
			pagination["per_page"] = sf.perPage
		}
		if sf.startingAfter != "" {
			pagination["starting_after"] = sf.startingAfter
		}
		body["pagination"] = pagination
	}
	return body, nil
}

// filterEq builds a single equality filter {field, operator, value}.
func filterEq(field string, value any) map[string]any {
	return map[string]any{"field": field, "operator": "=", "value": value}
}

// filterGT builds a single greater-than filter (used for --updated-since etc.).
func filterGT(field string, value any) map[string]any {
	return map[string]any{"field": field, "operator": ">", "value": value}
}
