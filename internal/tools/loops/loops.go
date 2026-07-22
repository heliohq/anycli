// Package loops is the built-in Loops service: a non-interactive cobra tree
// over the Loops v1 REST surface (https://app.loops.so/api). Loops is a
// transactional-email + audience platform; this tool wraps the CRM + messaging
// core an AI teammate actually drives — contacts, custom properties, events
// (workflow triggers), transactional email, and mailing lists. Auth is
// "Authorization: Bearer <API_KEY>". Errors are non-2xx with a JSON body
// carrying {message} (and a deprecated {error}); a 401 rejects the credential.
// Every command emits the provider's JSON response on stdout (passthrough +
// newline). Exit codes: 0 success, 1 runtime/API failure, 2 usage/param error.
package loops

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

// DefaultBaseURL is the production Loops API base. Endpoint paths carry the
// /v1/ prefix (e.g. POST /v1/contacts/create).
const DefaultBaseURL = "https://app.loops.so/api"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/loops.json). Loops keys are long-lived, team-scoped bearer
// tokens with no expiry or refresh.
const EnvAPIKey = "LOOPS_API_KEY"

// Service implements the built-in Loops tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Loops API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one loops subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, missing required
// flags, bad key=value pairs, unknown subcommands) are exit 2; runtime/API
// errors (Loops non-2xx, transport failure) are exit 1. Errors render to
// stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "LOOPS_API_KEY is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key)
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
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-key check).
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

func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "loops",
		Short:         "Loops built-in service (transactional email, contacts, events)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (output is always JSON; accepted for uniformity)")

	root.AddCommand(
		s.newWhoamiCmd(key),
		s.newContactCmd(key),
		s.newContactPropertyCmd(key),
		s.newEventCmd(key),
		s.newEmailCmd(key),
		s.newListCmd(key),
	)
	return root
}

// newGroup is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: a bare
// group shows help, an unknown subcommand fails with a usage error (exit 2).
func newGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
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
