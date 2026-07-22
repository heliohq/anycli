// Package calcom is the built-in Cal.com service: a cobra tree over the
// api.cal.com/v2 scheduling surface (event types, availability slots, bookings,
// schedules, profile). An AI teammate uses it as a scheduling actuator — read
// the meeting types a user offers, find open time, and book / cancel /
// reschedule on the user's behalf.
//
// The one correctness trap this package encodes: Cal.com v2 pins its API version
// PER ENDPOINT FAMILY via the required cal-api-version header (see client.go).
// A single global version would send the wrong date to /slots and /event-types
// and silently downgrade their response semantics, so every command sends its
// own route version.
package calcom

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

// DefaultBaseURL is the production Cal.com v2 API base.
const DefaultBaseURL = "https://api.cal.com/v2"

// EnvToken is the env var the credential binding injects (definitions/tools/calcom.json).
const EnvToken = "CALCOM_TOKEN"

// Service implements the built-in Cal.com tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Cal.com API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one calcom subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (Cal.com non-2xx, transport failure) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "CALCOM_TOKEN is not set"})
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
// pick the error format before cobra has parsed flags (e.g. the pre-parse
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

// newRoot builds the grouped-by-resource cobra tree: event-type / slot /
// booking / schedule groups plus the top-level `me` profile command.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "calcom",
		Short:         "Cal.com scheduling built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	eventType := newGroupCmd("event-type", "Inspect bookable meeting types")
	eventType.AddCommand(
		s.newEventTypeListCmd(token),
		s.newEventTypeGetCmd(token),
	)
	slot := newGroupCmd("slot", "Find available time")
	slot.AddCommand(s.newSlotListCmd(token))
	booking := newGroupCmd("booking", "Read and manage bookings")
	booking.AddCommand(
		s.newBookingListCmd(token),
		s.newBookingGetCmd(token),
		s.newBookingCreateCmd(token),
		s.newBookingCancelCmd(token),
		s.newBookingRescheduleCmd(token),
	)
	schedule := newGroupCmd("schedule", "Read availability schedules")
	schedule.AddCommand(s.newScheduleListCmd(token))

	root.AddCommand(eventType, slot, booking, schedule, s.newMeCmd(token))
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: a bare
// group shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
