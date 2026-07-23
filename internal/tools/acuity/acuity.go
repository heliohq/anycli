// Package acuity is the built-in Acuity Scheduling service: a non-interactive
// cobra tree over the Acuity API v1 surface (https://acuityscheduling.com/api/v1).
// Auth is "Authorization: Bearer <token>" using a non-expiring OAuth 2.0 access
// token (scope api-v1). Every command emits the provider JSON on stdout
// (passthrough + newline). Acuity errors are non-2xx with a JSON body carrying
// status_code/message/error; a 401 rejects the credential.
package acuity

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

// DefaultBaseURL is the production Acuity API v1 base.
const DefaultBaseURL = "https://acuityscheduling.com/api/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/acuity.json). Acuity OAuth access tokens do not expire and
// have no refresh grant.
const EnvAccessToken = "ACUITY_ACCESS_TOKEN"

// readOnly / writeAction carry the design-318 side-effect annotation on runnable
// leaf commands: "false" for side-effect-free reads (GET), "true" for writes
// that mutate provider state (POST/PUT/DELETE and semantic writes).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Acuity tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Acuity API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one acuity subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags/enums, malformed --field,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Acuity non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &apiError{msg: EnvAccessToken + " is not set"})
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
		return execution.Failure(err), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command error
	// is inherently a usage error → exit 2.
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

// newRoot builds the resource-grouped cobra tree. me is top-level (account
// identity); every other command hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "acuity",
		Short:         "Acuity Scheduling built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newAppointmentCmd(token),
		s.newAvailabilityCmd(token),
		s.newTypeCmd(token),
		s.newCalendarCmd(token),
		s.newClientCmd(token),
		s.newFormCmd(token),
		s.newLabelCmd(token),
		s.newBlockCmd(token),
		s.newMeCmd(token),
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
