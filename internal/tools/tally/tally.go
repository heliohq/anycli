// Package tally is the built-in Tally service: a non-interactive cobra tree
// over the api.tally.so REST surface (form builder). Auth is a personal API key
// sent as "Authorization: Bearer <token>". Tally fails with a non-2xx status;
// 401 rejects the credential. Every read command emits the provider JSON on
// stdout verbatim (passthrough + newline). Exit codes: 0 success, 1 runtime/API
// failure, 2 usage/param error. Errors render to stderr — as a JSON envelope
// under --json, plain text otherwise.
package tally

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Tally API base.
const DefaultBaseURL = "https://api.tally.so"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/tally.json). It carries the raw Tally personal API key
// (prefix tly-…); the service adds the "Bearer " scheme when building requests.
const EnvAPIKey = "TALLY_API_KEY"

// Service implements the built-in Tally tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Tally API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC httpDoer
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// In overrides stdin for --stdin body input; nil = os.Stdin.
	In io.Reader
}

// Execute runs one tally subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad enums, invalid JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Tally
// non-2xx, transport failure) are exit 1.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAPIKey]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAPIKey + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause.
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
// {"error":{"tool":"tally","code":"usage|api","message":…,"status":<HTTP>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"tool": "tally", "code": "usage", "message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["code"] = "api"
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "tally",
		Short:         "Tally built-in service (forms, submissions, analytics)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newFormCmd(token),
		s.newSubmissionCmd(token),
		s.newAnalyticsCmd(token),
		s.newWebhookCmd(token),
		s.newWorkspaceCmd(token),
		s.newMeCmd(token),
	)
	return root
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

func (s *Service) stdin() io.Reader {
	if s.In != nil {
		return s.In
	}
	return os.Stdin
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
