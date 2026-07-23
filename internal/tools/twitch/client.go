package twitch

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

// reqCtx carries the two injected credentials through the command tree so every
// Helix request can set both required headers.
type reqCtx struct {
	token    string
	clientID string
}

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, or invalid JSON. It maps to exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Helix non-2xx response or a transport
// failure. It maps to exit code 1. status is the HTTP status (0 for
// transport/network failures). It wraps the underlying cause so errors.As for a
// credential rejection still resolves through it.
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

// call performs one Helix API request with both required headers
// (Authorization: Bearer + Client-Id) and returns the raw response body. A 401
// marks the credential rejected; any other non-2xx is an apiError carrying
// Helix's message and status.
func (s *Service) call(ctx context.Context, rc *reqCtx, method, path string, query url.Values, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("twitch: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}

	requestURL := s.baseURL() + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("twitch: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+rc.token)
	req.Header.Set("Client-Id", rc.clientID)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("twitch: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("twitch: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("twitch API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: execution.RejectCredential(raw)}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// apiMessage extracts Helix's error message (error + message) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Error != "" || e.Message != "") {
		switch {
		case e.Error != "" && e.Message != "":
			return e.Error + ": " + e.Message
		case e.Message != "":
			return e.Message
		default:
			return e.Error
		}
	}
	return string(body)
}

// helixEnvelope is the shape every Helix response shares: a data array and, for
// paginated endpoints, a cursor under pagination.
type helixEnvelope struct {
	Data       []json.RawMessage `json:"data"`
	Pagination struct {
		Cursor string `json:"cursor"`
	} `json:"pagination"`
}

// emitList re-emits a Helix list response as the provider-neutral
// {"data":[...],"cursor":"<next or empty>"} shape so the AI can page with
// --after.
func (s *Service) emitList(body []byte) error {
	var env helixEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return &apiError{msg: fmt.Sprintf("twitch: decode response: %v", err), err: err}
	}
	if env.Data == nil {
		env.Data = []json.RawMessage{}
	}
	out := map[string]any{"data": env.Data, "cursor": env.Pagination.Cursor}
	return s.emitValue(out)
}

// emitOne unwraps a Helix single-object response (data[0]) and emits the object
// verbatim. An empty data array (e.g. a lookup that matched nothing) emits an
// explicit null so the AI can distinguish "found nothing" from a malformed
// reply.
func (s *Service) emitOne(body []byte) error {
	var env helixEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return &apiError{msg: fmt.Sprintf("twitch: decode response: %v", err), err: err}
	}
	if len(env.Data) == 0 {
		return s.emitRaw([]byte("null"))
	}
	return s.emitRaw(env.Data[0])
}

// emitRaw writes raw JSON bytes to stdout (+ newline).
func (s *Service) emitRaw(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}

// emitValue marshals a client-side value (receipts / re-shaped envelopes) and
// writes it to stdout (+ newline).
func (s *Service) emitValue(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("twitch: encode output: %v", err), err: err}
	}
	return s.emitRaw(body)
}

// resolveSelfID returns the authenticated user's id, calling Get Users with no
// id/login (which returns the token's own user) at most once per invocation and
// caching the result. Channel-scoped verbs use it so the AI need not first look
// up its own broadcaster id.
func (s *Service) resolveSelfID(ctx context.Context, rc *reqCtx) (string, error) {
	if s.selfID != "" {
		return s.selfID, nil
	}
	body, err := s.call(ctx, rc, http.MethodGet, "/users", nil, nil)
	if err != nil {
		return "", err
	}
	var env helixEnvelope
	if err := json.Unmarshal(body, &env); err != nil || len(env.Data) == 0 {
		return "", &apiError{msg: "twitch: could not resolve the authenticated user's id"}
	}
	var u struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Data[0], &u); err != nil || u.ID == "" {
		return "", &apiError{msg: "twitch: could not resolve the authenticated user's id"}
	}
	s.selfID = u.ID
	return s.selfID, nil
}

// paginationFlags holds the shared cursor-pagination flags Helix list endpoints
// accept.
type paginationFlags struct {
	first int
	after string
}

// registerPaginationFlags wires --first / --after onto a list command.
func registerPaginationFlags(cmd *cobra.Command, p *paginationFlags) {
	cmd.Flags().IntVar(&p.first, "first", 0, "max items per page (Helix default 20, max 100)")
	cmd.Flags().StringVar(&p.after, "after", "", "pagination cursor from a previous response")
}

// apply writes the pagination flags into a query value set (omitting unset).
func (p paginationFlags) apply(q url.Values) {
	if p.first > 0 {
		q.Set("first", strconv.Itoa(p.first))
	}
	if p.after != "" {
		q.Set("after", p.after)
	}
}

// addRepeated adds every value of a repeatable flag under the given query key.
func addRepeated(q url.Values, key string, values []string) {
	for _, v := range values {
		if v != "" {
			q.Add(key, v)
		}
	}
}
