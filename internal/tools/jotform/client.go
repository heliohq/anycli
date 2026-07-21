package jotform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: a missing required arg, bad --field
// syntax, or an unknown subcommand. It maps to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Jotform non-2xx response, a responseCode
// other than 200, or a transport failure. It maps to exit code 1 and kind
// "api". status is the HTTP status (0 for transport/network failures).
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// jotformEnvelope is the shape shared by every Jotform response. content is
// left raw — callers emit the whole body verbatim; only responseCode/message
// are decoded here, to detect an error carried inside an HTTP-200 envelope.
type jotformEnvelope struct {
	ResponseCode int             `json:"responseCode"`
	Message      json.RawMessage `json:"message"`
}

// call performs one Jotform API request with the raw APIKEY header and returns
// the response body verbatim. A non-2xx status — or a responseCode other than
// 200 inside a 2xx body — becomes an apiError carrying Jotform's message. The
// credential is never marked rejected: Jotform overloads 401 for both an
// invalid key and a valid read-only key attempting a write, so auto-
// invalidating on 401 would brick a still-valid read-only connection (the
// execution contract keeps scope/permission failures non-rejecting).
func (s *Service) call(ctx context.Context, key, method, path string, query url.Values, body io.Reader, contentType string) ([]byte, error) {
	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("jotform: build request: %v", err), err: err}
	}
	req.Header.Set(authHeader, key)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("jotform: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("jotform: read response: %v", err), err: err}
	}

	env := decodeEnvelope(respBody)
	httpOK := resp.StatusCode >= 200 && resp.StatusCode <= 299
	envOK := env == nil || env.ResponseCode == 0 || env.ResponseCode == 200
	if !httpOK || !envOK {
		status := resp.StatusCode
		if httpOK && env != nil && env.ResponseCode != 0 {
			status = env.ResponseCode
		}
		return nil, &apiError{
			msg:    fmt.Sprintf("jotform API error (HTTP %d): %s", status, apiMessage(respBody)),
			status: status,
		}
	}
	return respBody, nil
}

// get is the read-verb shorthand: GET path with optional query, no body.
func (s *Service) get(ctx context.Context, key, path string, query url.Values) ([]byte, error) {
	return s.call(ctx, key, http.MethodGet, path, query, nil, "")
}

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

// emit writes the provider's JSON response to stdout verbatim (+ newline).
func (s *Service) emit(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// decodeEnvelope best-effort parses the Jotform envelope; a body that is not a
// JSON object (or lacks the fields) yields nil, treated as "no envelope signal".
func decodeEnvelope(body []byte) *jotformEnvelope {
	var env jotformEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil
	}
	return &env
}

// apiMessage extracts Jotform's error message from a response body, falling
// back to the raw body. Jotform's message field is usually a string but can be
// a structured object on validation errors, so it is rendered as-is.
func apiMessage(body []byte) string {
	env := decodeEnvelope(body)
	if env == nil || len(env.Message) == 0 {
		return strings.TrimSpace(string(body))
	}
	var text string
	if err := json.Unmarshal(env.Message, &text); err == nil && strings.TrimSpace(text) != "" {
		return text
	}
	return strings.TrimSpace(string(env.Message))
}

// listParams holds the shared pagination/ordering flags Jotform's list
// endpoints accept. Empty values are omitted so Jotform applies its defaults.
type listParams struct {
	offset  int
	limit   int
	filter  string
	orderby string
}

// registerListFlags wires --offset / --limit / --filter / --orderby onto a list
// command. Defaults are -1 (sentinel = "unset, let Jotform decide") for the
// numeric flags so we never send offset=0/limit=0 the user did not ask for.
func registerListFlags(cmd *cobra.Command, p *listParams) {
	cmd.Flags().IntVar(&p.offset, "offset", -1, "number of results to skip")
	cmd.Flags().IntVar(&p.limit, "limit", -1, "max number of results (Jotform caps at 1000)")
	cmd.Flags().StringVar(&p.filter, "filter", "", "JSON filter object, e.g. '{\"status\":\"ENABLED\"}'")
	cmd.Flags().StringVar(&p.orderby, "orderby", "", "field to order by, e.g. created_at")
}

// apply writes the set list params into a query value set.
func (p listParams) apply(q url.Values) {
	if p.offset >= 0 {
		q.Set("offset", strconv.Itoa(p.offset))
	}
	if p.limit >= 0 {
		q.Set("limit", strconv.Itoa(p.limit))
	}
	if p.filter != "" {
		q.Set("filter", p.filter)
	}
	if p.orderby != "" {
		q.Set("orderby", p.orderby)
	}
}
