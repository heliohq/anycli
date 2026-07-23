// Package outreach implements the built-in Outreach service over the Outreach
// API v2 (https://api.outreach.io/api/v2), a JSON:API 1.0 surface. It accepts an
// OAuth 2.0 user access token and exposes a non-interactive cobra tree grouped
// by resource (prospect, account, sequence, enrollment, task, …). Every JSON:API
// resource is flattened to a provider-neutral object ({id, type, ...attributes,
// <rel>_id}) and list commands emit {items, next_cursor, count}. Outreach fails
// with a non-2xx status and a JSON:API errors[] body — every call surfaces it.
package outreach

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

// DefaultBaseURL is the production Outreach API base (already carries /api/v2).
const DefaultBaseURL = "https://api.outreach.io/api/v2"

// EnvToken is the env var the credential binding injects (definitions/tools/outreach.json).
const EnvToken = "OUTREACH_ACCESS_TOKEN"

// readOnly and writeAction are the design-318 side-effect annotations applied to
// every runnable leaf command: "false" for read-only reads, "true" for calls
// that mutate provider state.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Outreach tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
// Empty fields select production defaults; tests inject an httptest server and
// output buffers.
type Service struct {
	// BaseURL overrides the Outreach API base; empty = DefaultBaseURL.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one outreach subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Outreach non-2xx, transport failure) are exit 1. Errors render to stderr —
// as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvToken + " is not set"})
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
// pick the error format before cobra has parsed flags.
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "outreach",
		Short:         "Outreach sales-engagement built-in service (JSON:API v2)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (default output is already JSON)")

	root.AddCommand(
		s.newProspectCmd(token),
		s.newAccountCmd(token),
		s.newSequenceCmd(token),
		s.newEnrollmentCmd(token),
		s.newMailboxCmd(token),
		s.newMailingCmd(token),
		s.newCallCmd(token),
		s.newTaskCmd(token),
		s.newOpportunityCmd(token),
		s.newUserCmd(token),
		s.newTemplateCmd(token),
		s.newStageCmd(token),
		s.newPersonaCmd(token),
	)
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
