package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// pollInterval / pollMaxAttempts bound the --allow-async task poll loop: a
// fixed interval, no backoff, and a hard attempt cap after which the caller
// gets the task id back plus an explicit timeout error (design 304).
const (
	pollInterval    = 2 * time.Second
	pollMaxAttempts = 30
)

// taskEndpoint is the async-task status path (base URL already carries /v1):
// GET /v1/async_tasks/{id}, i.e. Notion's "Retrieve an async task". The async
// response's status_url is the absolute form of this path; we poll it through
// the base-URL-aware client (by task id) so BaseURL overrides and the httptest
// harness keep working, rather than dialing the absolute URL directly.
const taskEndpoint = "/async_tasks/"

// usageError is a parameter / usage error: illegal flag combination, missing
// required flag, bad enum value, invalid JSON, or an unresolvable id. It maps
// to exit code 2 and kind "usage".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// apiError is a runtime / API error: a Notion non-2xx response, a transport
// failure, or a poll timeout. It maps to exit code 1 and kind "api". status is
// the HTTP status (0 for transport/network failures). It wraps the underlying
// cause so errors.As for *credentialRejectedError still resolves through it.
type apiError struct {
	msg    string
	status int
	err    error
}

func (e *apiError) Error() string { return e.msg }
func (e *apiError) Unwrap() error { return e.err }

// emitJSON writes the provider's JSON response to stdout verbatim.
func (s *Service) emitJSON(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// pageMarkdown is the page-markdown envelope. truncated / unknown_block_ids are
// the agent's re-fetch signal; they are surfaced on stderr, never mixed into
// the markdown on stdout.
type pageMarkdown struct {
	Markdown        string   `json:"markdown"`
	Truncated       bool     `json:"truncated"`
	UnknownBlockIDs []string `json:"unknown_block_ids"`
}

// emitMarkdown extracts the markdown field from a page-markdown envelope and
// writes the raw markdown to stdout. A partial read (truncated or unknown
// blocks) prints a re-fetch note to stderr so stdout stays clean markdown.
func (s *Service) emitMarkdown(body []byte) error {
	var pm pageMarkdown
	if err := json.Unmarshal(body, &pm); err != nil {
		return &apiError{msg: fmt.Sprintf("notion: decode page markdown: %v", err), err: err}
	}
	out := pm.Markdown
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if _, err := io.WriteString(s.stdout(), out); err != nil {
		return err
	}
	if pm.Truncated || len(pm.UnknownBlockIDs) > 0 {
		fmt.Fprintf(s.stderr(),
			"note: page markdown is partial (truncated=%t, unknown_block_ids=%d); re-fetch to get the full content\n",
			pm.Truncated, len(pm.UnknownBlockIDs))
	}
	return nil
}

// readPageMarkdown GETs a page body as the markdown envelope.
func (s *Service) readPageMarkdown(ctx context.Context, token, id string) ([]byte, error) {
	return s.call(ctx, token, http.MethodGet, "/pages/"+url.PathEscape(id)+"/markdown", nil)
}

// hintIfEmptyDatabaseRow guards against a silent-empty fetch. fetch reads a
// page's body markdown, but a database row's data lives in its *properties*,
// not its body — so fetching a row returns an empty body and an agent can
// wrongly conclude the row has no data. When the body is empty, retrieve the
// page and, if its parent is a data source / database (i.e. it is a row), print
// a non-fatal hint on stderr pointing at `data-source query` / properties. Best-effort:
// stdout and the exit code are never changed, and a failed probe is silent (the
// primary read already succeeded).
func (s *Service) hintIfEmptyDatabaseRow(ctx context.Context, token, id string, body []byte) {
	var pm pageMarkdown
	if json.Unmarshal(body, &pm) != nil || strings.TrimSpace(pm.Markdown) != "" {
		return // decode failed or body has content — nothing to warn about
	}
	meta, err := s.call(ctx, token, http.MethodGet, "/pages/"+url.PathEscape(id), nil)
	if err != nil {
		return // best-effort; don't turn a successful read into a failure
	}
	var page struct {
		Parent struct {
			Type string `json:"type"`
		} `json:"parent"`
	}
	if json.Unmarshal(meta, &page) != nil {
		return
	}
	switch page.Parent.Type {
	case "data_source_id", "database_id":
		fmt.Fprintln(s.stderr(),
			"note: this looks like a database row; its fields may not be in the page body — use `data-source query` (or read the row's properties) to double-check")
	}
}

// writePageMarkdown PATCHes a page body from a markdown-endpoint payload.
func (s *Service) writePageMarkdown(ctx context.Context, token, id string, payload any) ([]byte, error) {
	return s.call(ctx, token, http.MethodPatch, "/pages/"+url.PathEscape(id)+"/markdown", payload)
}

// taskGet fetches one async task's status. Shared by task get and pollTask.
func (s *Service) taskGet(ctx context.Context, token, id string) ([]byte, error) {
	return s.call(ctx, token, http.MethodGet, taskEndpoint+url.PathEscape(id), nil)
}

// asyncPoll is the "retrieve an async task" response. In-progress states
// (queued/running/retrying) carry poll_after_seconds; the terminal states are
// succeeded (with result) and failed (with error).
type asyncPoll struct {
	Status           string          `json:"status"`
	PollAfterSeconds int             `json:"poll_after_seconds"`
	Result           json.RawMessage `json:"result"`
	Error            *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// pollTask polls an async task to a terminal state, honoring the endpoint's
// poll_after_seconds between polls (falling back to pollInterval, no backoff)
// and capping at pollMaxAttempts. On success it returns the task's result body
// (the created/updated resource); on a failed task it returns an apiError. On
// the attempt cap it writes the task id to stdout — so a caller reading stdout
// can recover it for a manual `task get` (design 304 §error-convention) — and
// returns an apiError timeout. pollAfter seeds the first interval from the
// original async_task handle.
func (s *Service) pollTask(ctx context.Context, token, taskID string, pollAfter int) ([]byte, error) {
	delay := pollDelay(pollAfter)
	for attempt := 0; attempt < pollMaxAttempts; attempt++ {
		body, err := s.taskGet(ctx, token, taskID)
		if err != nil {
			return nil, err
		}
		var p asyncPoll
		_ = json.Unmarshal(body, &p)
		switch strings.ToLower(strings.TrimSpace(p.Status)) {
		case "succeeded":
			if len(p.Result) > 0 {
				return p.Result, nil
			}
			return body, nil
		case "failed":
			msg := fmt.Sprintf("async task %s failed", taskID)
			if p.Error != nil && (p.Error.Code != "" || p.Error.Message != "") {
				msg = fmt.Sprintf("async task %s failed: %s: %s", taskID, p.Error.Code, p.Error.Message)
			}
			return nil, &apiError{msg: msg}
		}
		if p.PollAfterSeconds > 0 {
			delay = pollDelay(p.PollAfterSeconds)
		}
		select {
		case <-ctx.Done():
			return nil, &apiError{msg: fmt.Sprintf("notion: poll task %s: %v", taskID, ctx.Err()), err: ctx.Err()}
		case <-time.After(delay):
		}
	}
	// Timeout: surface the task id on stdout so an agent can resume with
	// `task get`, then fail with an explicit non-zero (exit 1) error.
	fmt.Fprintln(s.stdout(), taskID)
	return nil, &apiError{msg: fmt.Sprintf(
		"async task %s did not complete after %d polls; run `task get %s` to keep checking",
		taskID, pollMaxAttempts, taskID)}
}

// pollDelay honors the endpoint's poll_after_seconds guidance, falling back to
// the fixed pollInterval when the field is absent or non-positive.
func pollDelay(pollAfterSeconds int) time.Duration {
	if pollAfterSeconds > 0 {
		return time.Duration(pollAfterSeconds) * time.Second
	}
	return pollInterval
}

// call performs one Notion API request on the globally pinned Notion-Version.
func (s *Service) call(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	return s.callWithVersion(ctx, token, method, path, payload, notionVersion)
}

// callWithVersion is call with a caller-chosen Notion-Version header. Bearer
// auth + the version on every call; a non-2xx surfaces the body's message
// (and, for 403/404, an actionable access hint) as an apiError carrying the
// HTTP status, and a transport failure as an apiError with status 0.
func (s *Service) callWithVersion(ctx context.Context, token, method, path string, payload any, version string) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, &apiError{msg: fmt.Sprintf("notion: encode request: %v", err), err: err}
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("notion: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", version)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("notion: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("notion: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("notion API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), accessHint(resp.StatusCode))
		classified := classifyNotionCredentialError(resp.StatusCode, body, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}

// accessHint returns an actionable clause for the failures an agent most often
// hits: a wrong ID or a resource never shared with the integration.
func accessHint(status int) string {
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return " (check the ID and that the integration has been granted access to this resource)"
	}
	return ""
}

// apiMessage extracts Notion's error message (code + message) from an error
// body, falling back to the raw body.
func apiMessage(body []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && (e.Code != "" || e.Message != "") {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return string(body)
}
