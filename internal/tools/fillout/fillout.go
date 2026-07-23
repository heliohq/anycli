// Package fillout is the built-in Fillout service: a non-interactive cobra tree
// over the Fillout REST API (https://www.fillout.com/help/fillout-rest-api). It
// reads forms and their submissions and manages submission webhooks. The API
// base is per-connection (US api.fillout.com, EU eu-api.fillout.com, or a
// self-host), injected as FILLOUT_API_BASE and defaulting to the US host; the
// bearer token is injected as FILLOUT_ACCESS_TOKEN. Fillout answers a non-2xx
// status with a JSON body carrying "message" — every call surfaces it.
package fillout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// readOnly / writeAction carry the design-318 side-effect annotation for runnable leaves.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}

// DefaultAPIBase is the documented US Fillout API host used when no
// per-connection base is injected.
const DefaultAPIBase = "https://api.fillout.com"

// apiPrefix is the versioned path segment every Fillout REST call carries.
const apiPrefix = "/v1/api"

// Env vars the credential bindings inject (definitions/tools/fillout.json).
// EnvAPIBase can legitimately be absent — the service then falls back to the
// documented US host (DefaultAPIBase), never a silent wrong-host default: the
// production OAuth path always injects the connection's real base_url.
const (
	EnvAccessToken = "FILLOUT_ACCESS_TOKEN"
	EnvAPIBase     = "FILLOUT_API_BASE"
)

// Service implements the built-in Fillout tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// APIBase overrides the injected/default Fillout host; empty = the
	// FILLOUT_API_BASE env value, else DefaultAPIBase. Tests point it at an
	// httptest server.
	APIBase string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// apiError is a Fillout API/runtime failure (non-2xx). It carries the HTTP
// status so the --json error envelope can report it; a nil-status apiError is
// still an API-class (exit 1) error.
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

// usageError is an input/usage failure (bad flag combo, invalid enum, invalid
// JSON body). It renders as a usage-kind error and exits 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// Execute runs one fillout subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags/args, unknown subcommands) are exit 2;
// runtime/API errors (Fillout non-2xx, transport failure) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &apiError{msg: "FILLOUT_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	base := s.APIBase
	if base == "" {
		base = env[EnvAPIBase]
	}
	root := s.newRoot(token, base)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return execution.Failure(err), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// missing-token check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

func (s *Service) stdout() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s *Service) stderr() io.Writer {
	if s.Err != nil {
		return s.Err
	}
	return os.Stderr
}

// resolveBase returns the host to use: the injected base, or the documented US
// default when none was supplied.
func resolveBase(apiBase string) string {
	if apiBase == "" {
		return DefaultAPIBase
	}
	return apiBase
}

// newRoot builds the grouped-by-resource cobra tree for one invocation.
func (s *Service) newRoot(token, apiBase string) *cobra.Command {
	root := &cobra.Command{
		Use:           "fillout",
		Short:         "Fillout built-in service (forms + submissions)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	// --json is accepted for a uniform agent-facing surface; provider JSON is
	// already the only success output. It also selects the error envelope shape.
	root.PersistentFlags().Bool("json", false, "output JSON (always on for success; selects the error envelope shape)")

	form := newGroupCmd("form", "Read forms")
	form.AddCommand(s.newFormListCmd(token, apiBase), s.newFormGetCmd(token, apiBase))

	submission := newGroupCmd("submission", "Read and write submissions")
	submission.AddCommand(
		s.newSubmissionListCmd(token, apiBase),
		s.newSubmissionGetCmd(token, apiBase),
		s.newSubmissionCreateCmd(token, apiBase),
		s.newSubmissionDeleteCmd(token, apiBase),
	)

	webhook := newGroupCmd("webhook", "Manage submission webhooks")
	webhook.AddCommand(s.newWebhookCreateCmd(token, apiBase), s.newWebhookDeleteCmd(token, apiBase))

	root.AddCommand(form, submission, webhook)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// emit writes the provider's JSON response to stdout verbatim. Fillout answers
// some mutations (e.g. delete) with an empty body — emit a bare newline then.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// call performs one Fillout API request with Bearer auth against apiBase. path
// is the resource path after the /v1/api prefix (e.g. "/forms"). A non-2xx
// surfaces the body's "message"; 401 additionally classifies as a rejected
// credential.
func (s *Service) call(ctx context.Context, token, apiBase, method, path string, query map[string]string, body io.Reader) ([]byte, error) {
	u := resolveBase(apiBase) + apiPrefix + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("fillout: build request: %v", err)}
	}
	if len(query) > 0 {
		q := req.URL.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("fillout: %s %s: %v", method, path, err)}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("fillout: read response: %v", err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{status: resp.StatusCode, msg: fmt.Sprintf("fillout API error (HTTP %d): %s", resp.StatusCode, apiMessage(respBody))}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, execution.RejectCredential(apiErr)
		}
		return nil, apiErr
	}
	return respBody, nil
}

// apiMessage extracts Fillout's error message from a response body, falling
// back to the raw body (trimmed of nothing — callers keep it short).
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if e.Message != "" {
			return e.Message
		}
		if e.Error != "" {
			return e.Error
		}
	}
	if len(body) == 0 {
		return "(no response body)"
	}
	return string(body)
}
