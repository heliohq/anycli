package attio

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

	"github.com/spf13/cobra"
)

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad value, invalid JSON, or a malformed identifier. It maps to
// exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: an Attio non-2xx response or a transport
// failure. It maps to exit code 1 and kind "api". status is the HTTP status (0
// for transport/network failures). It wraps the underlying cause so errors.As
// for the credential-rejection marker still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// call performs one Attio API request with Bearer auth. On a non-2xx it surfaces
// Attio's error body (type/code/message) as an apiError carrying the HTTP
// status; a transport failure becomes an apiError with status 0. A 401 (or an
// invalid_token body) is classified as a credential rejection so the host's
// token-refresh path can react.
func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("attio: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("attio: build request: %v", err), err: err}
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
		return nil, &apiError{msg: fmt.Sprintf("attio: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("attio: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("attio API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		classified := classifyAttioCredentialError(resp.StatusCode, body, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// apiMessage extracts Attio's error message (type + code + message) from an
// error body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Type != "" || e.Code != "" || e.Message != "") {
		parts := make([]string, 0, 3)
		if e.Type != "" {
			parts = append(parts, e.Type)
		}
		if e.Code != "" {
			parts = append(parts, e.Code)
		}
		if e.Message != "" {
			parts = append(parts, e.Message)
		}
		return strings.Join(parts, ": ")
	}
	return string(body)
}

// selfIdentity is the subset of GET /v2/self this tool consumes.
type selfIdentity struct {
	WorkspaceID                   string `json:"workspace_id"`
	WorkspaceName                 string `json:"workspace_name"`
	WorkspaceSlug                 string `json:"workspace_slug"`
	AuthorizedByWorkspaceMemberID string `json:"authorized_by_workspace_member_id"`
	Scope                         string `json:"scope"`
}

// self fetches and decodes GET /v2/self (the token/workspace identity).
func (s *Service) self(ctx context.Context, token string) (selfIdentity, []byte, error) {
	body, err := s.call(ctx, token, http.MethodGet, "/v2/self", nil)
	if err != nil {
		return selfIdentity{}, nil, err
	}
	var id selfIdentity
	if err := json.Unmarshal(body, &id); err != nil {
		return selfIdentity{}, body, &apiError{msg: fmt.Sprintf("attio: decode self: %v", err), err: err}
	}
	return id, body, nil
}

// --- output helpers -------------------------------------------------------

// emitJSON writes the provider's JSON response to stdout verbatim (already
// wrapped by Attio in {"data": …}) so agents can chain calls.
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emit renders a provider response: verbatim JSON when jsonMode, otherwise a
// compact human-readable summary of the {"data": …} envelope.
func (s *Service) emit(jsonMode bool, body []byte) error {
	if jsonMode {
		return s.emitJSON(body)
	}
	return s.emitSummary(body)
}

// emitSummary prints one line per item for a data array, or a single line for a
// data object, picking a stable id plus a human label. It falls back to raw
// JSON when the body is not the expected envelope, so nothing is ever silently
// dropped.
func (s *Service) emitSummary(body []byte) error {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Data) == 0 {
		return s.emitJSON(body)
	}
	var arr []json.RawMessage
	if json.Unmarshal(env.Data, &arr) == nil {
		if len(arr) == 0 {
			fmt.Fprintln(s.stdout(), "(no results)")
			return nil
		}
		for _, item := range arr {
			fmt.Fprintln(s.stdout(), summarizeItem(item))
		}
		return nil
	}
	fmt.Fprintln(s.stdout(), summarizeItem(env.Data))
	return nil
}

// summarizeItem builds a one-line "<id> <label>" summary of a single Attio
// resource object. It reads the nested id.* (record_id/entry_id/… or the flat
// id) and a label (record_text/title/content/name/… or workspace/member name).
func summarizeItem(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return string(raw)
	}
	id := extractID(m)
	label := extractLabel(m)
	switch {
	case id != "" && label != "":
		return id + "  " + label
	case id != "":
		return id
	case label != "":
		return label
	default:
		return string(raw)
	}
}

// idKeys are the terminal id fields Attio nests under the "id" object, in the
// order we prefer them when several are present.
var idKeys = []string{
	"record_id", "entry_id", "note_id", "task_id", "comment_id", "thread_id",
	"attribute_id", "object_id", "list_id", "workspace_member_id", "option_id",
	"status_id", "workspace_id",
}

// extractID pulls a stable identifier from a resource, preferring the nested
// id.* terminal fields, then a flat string id.
func extractID(m map[string]any) string {
	if idObj, ok := m["id"].(map[string]any); ok {
		for _, k := range idKeys {
			if v, ok := idObj[k].(string); ok && v != "" {
				return v
			}
		}
	}
	if v, ok := m["id"].(string); ok {
		return v
	}
	return ""
}

// labelKeys are the human-readable summary fields, in preference order.
var labelKeys = []string{
	"record_text", "title", "content", "content_plaintext",
	"workspace_name", "name", "api_slug", "slug",
}

// extractLabel pulls a short human label from a resource, checking the common
// scalar fields first, then a couple of nested shapes Attio uses.
func extractLabel(m map[string]any) string {
	for _, k := range labelKeys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return oneLine(v)
		}
	}
	// Workspace-member name: {first_name,last_name} or {name:{full_name}}.
	if first, ok := m["first_name"].(string); ok {
		last, _ := m["last_name"].(string)
		if n := strings.TrimSpace(first + " " + last); n != "" {
			return n
		}
	}
	return ""
}

// oneLine collapses a possibly multi-line label into a single trimmed line.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}

// --- shared flag / parsing utilities --------------------------------------

// parseJSONFlag parses a JSON flag value into a generic value. An empty value
// yields nil (flag unset). A parse failure is a usage error.
func parseJSONFlag(name, raw string) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--%s must be valid JSON: %v", name, err)}
	}
	return v, nil
}

// parseValuesFlag parses a --values JSON object flag. It must be a JSON object
// (Attio values / entry_values are keyed by attribute slug/id).
func parseValuesFlag(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, &usageError{msg: "--values is required and must be a JSON object of attribute slug/id → value"}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--values must be a JSON object: %v", err)}
	}
	return m, nil
}

// parseRecordRef splits a "<object>:<record_id>" reference into its parts. Both
// halves are required; a missing colon or empty half is a usage error.
func parseRecordRef(flag, raw string) (object, recordID string, err error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", &usageError{msg: fmt.Sprintf("--%s must be <object>:<record_id>", flag)}
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

// limitOffset holds the standard pagination flags. changedLimit / changedOffset
// let callers place them as query params or body fields only when set, so a
// command never overrides the provider's own defaults with a zero.
type limitOffset struct {
	limit  int
	offset int
	cmd    *cobra.Command
}

// registerLimitOffset attaches --limit / --offset to cmd.
func registerLimitOffset(cmd *cobra.Command) *limitOffset {
	lo := &limitOffset{cmd: cmd}
	cmd.Flags().IntVar(&lo.limit, "limit", 0, "maximum number of results to return")
	cmd.Flags().IntVar(&lo.offset, "offset", 0, "number of results to skip")
	return lo
}

func (lo *limitOffset) limitChanged() bool  { return lo.cmd.Flags().Changed("limit") }
func (lo *limitOffset) offsetChanged() bool { return lo.cmd.Flags().Changed("offset") }

// applyToPayload sets limit/offset on a POST-query JSON body when the flags were
// provided.
func (lo *limitOffset) applyToPayload(payload map[string]any) {
	if lo.limitChanged() {
		payload["limit"] = lo.limit
	}
	if lo.offsetChanged() {
		payload["offset"] = lo.offset
	}
}

// applyToQuery sets limit/offset as URL query params when the flags were
// provided.
func (lo *limitOffset) applyToQuery(q url.Values) {
	if lo.limitChanged() {
		q.Set("limit", strconv.Itoa(lo.limit))
	}
	if lo.offsetChanged() {
		q.Set("offset", strconv.Itoa(lo.offset))
	}
}
