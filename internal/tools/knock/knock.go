// Package knock is the built-in Knock service: a non-interactive cobra tree
// over the Knock v1 REST surface (https://api.knock.app/v1). Knock is
// notification infrastructure — you model recipients, then trigger workflows
// that fan one event across channels (email, SMS, push, in-app, Slack, …). An
// AI teammate uses it the way an on-call or growth engineer would: send a
// notification, make sure the right person gets it, and check whether it
// landed.
//
// Auth is a single environment-scoped secret key injected as KNOCK_API_KEY and
// sent as "Authorization: Bearer <key>" on every request. Knock returns clean,
// envelope-consistent JSON, which the service passes through to stdout
// verbatim. Errors follow the shared exit-code contract: 0 success, 2 for
// usage/parameter errors (bad flags, invalid JSON, missing required id), 1 for
// runtime/API errors (Knock non-2xx, transport failure); a 401 marks the
// credential rejected.
package knock

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

// design-318 side_effect annotation maps shared by every runnable leaf.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}

// DefaultBaseURL is the production Knock v1 API base.
const DefaultBaseURL = "https://api.knock.app/v1"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/knock.json). Knock secret keys are long-lived,
// non-expiring, environment-scoped secrets (pk_ public / sk_ secret; the
// service uses the secret key).
const EnvAPIKey = "KNOCK_API_KEY"

// Service implements the built-in Knock tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Knock API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one knock subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, invalid JSON,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Knock non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "KNOCK_API_KEY is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. Every command hangs under
// a resource group matching the Knock API nouns (workflow, user, message,
// object, tenant, schedule).
func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "knock",
		Short:         "Knock built-in service (notification infrastructure)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newWorkflowCmd(key),
		s.newUserCmd(key),
		s.newMessageCmd(key),
		s.newObjectCmd(key),
		s.newTenantCmd(key),
		s.newScheduleCmd(key),
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
