// Package close is the built-in Close CRM service: a non-interactive cobra
// tree over the Close REST API v1 (https://api.close.com/api/v1). Auth is an
// OAuth access token sent as "Authorization: Bearer <token>". Close errors are
// non-2xx with a JSON body carrying `error` and optional `field-errors`; a 401
// rejects the credential. Every command emits the provider JSON on stdout
// verbatim (+ newline) — Close's own list endpoints already return the
// agent-friendly {"data":[...],"has_more":bool} envelope, so no wrapping.
package close

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

// DefaultBaseURL is the production Close API base (already carries /api/v1).
const DefaultBaseURL = "https://api.close.com/api/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/close.json). Helio drives Close through OAuth, so the
// injected value is a short-lived OAuth access token sent as a Bearer token.
const EnvAccessToken = "CLOSE_ACCESS_TOKEN"

// Service implements the built-in Close tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Close API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one close subcommand with the resolved credentials in env.
// Success is exit 0; runtime/API failures (Close non-2xx, transport failure)
// are exit 1; usage/parse errors (bad flags, invalid JSON, unknown
// subcommands, missing required flags) are exit 2. Errors render to stderr —
// as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags; detect --json in the
		// raw args so the missing-token error honors the JSON envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "CLOSE_ACCESS_TOKEN is not set"})
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
		// Runtime/API failure: exit 1, preserving any credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(err), nil
	}
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
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

// newRoot builds the grouped-by-resource cobra tree. search / me are top-level
// (cross-resource); lead/contact/opportunity/task/activity are resource groups.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "close",
		Short:         "Close CRM built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newResourceCmd(token, "lead", "/lead/", "Manage leads (companies/accounts)"),
		s.newResourceCmd(token, "contact", "/contact/", "Manage contacts (people on a lead)"),
		s.newResourceCmd(token, "opportunity", "/opportunity/", "Manage opportunities (deals)"),
		s.newTaskCmd(token),
		s.newActivityCmd(token),
		s.newSearchCmd(token),
		s.newMeCmd(token),
	)
	return root
}

// newGroupCmd is a runnable command group: a bare group prints help, an unknown
// subcommand fails (cobra skips Args validation on non-runnable commands, which
// would let an unknown subcommand exit 0 — a false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
