// Package calendly is the built-in Calendly service: a non-interactive cobra
// tree over the api.calendly.com REST surface plus the 2026 Scheduling API. It
// lets an AI teammate act as a scheduling aide — read availability and booked
// meetings, share single-use booking links, inspect who booked, cancel with a
// reason, mark no-shows, and (on paid plans) book a slot directly.
//
// Auth is "Authorization: Bearer <token>". Resources are identified by full
// URIs (e.g. https://api.calendly.com/users/XXXX), not bare ids; flags accept
// either a bare UUID (expanded to the canonical URI) or a full URI, and "me" is
// accepted wherever a user URI is expected (resolved via one cached
// GET /users/me per invocation). Calendly fails with a non-2xx status and a
// JSON body carrying title/message; 401 rejects the credential.
//
// Exit codes: 0 success; 2 usage/parameter errors (bad flags, missing required
// flags, unknown subcommands); 1 runtime/API errors (Calendly non-2xx,
// transport failure). Errors render to stderr — as a JSON envelope under
// --json, plain text otherwise.
package calendly

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

// DefaultBaseURL is the production Calendly API base. Unlike most providers it
// carries no version path segment; endpoints are rooted directly (e.g.
// /users/me, /scheduled_events).
const DefaultBaseURL = "https://api.calendly.com"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/calendly.json). Calendly OAuth access tokens are bearer
// tokens with a documented 2-hour lifetime; refresh is the host's job.
const EnvAccessToken = "CALENDLY_ACCESS_TOKEN"

// Service implements the built-in Calendly tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Calendly API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one calendly subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "CALENDLY_ACCESS_TOKEN is not set"})
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
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "calendly",
		Short:         "Calendly built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	eventType := newGroupCmd("event-type", "Discover bookable meeting kinds")
	eventType.AddCommand(
		s.newEventTypeListCmd(token),
		s.newEventTypeGetCmd(token),
	)
	availability := newGroupCmd("availability", "Open slots, busy times, and working-hours schedules")
	availability.AddCommand(
		s.newAvailabilitySlotsCmd(token),
		s.newAvailabilityBusyCmd(token),
		s.newAvailabilityScheduleCmd(token),
	)
	event := newGroupCmd("event", "List and inspect booked meetings")
	event.AddCommand(
		s.newEventListCmd(token),
		s.newEventGetCmd(token),
		s.newEventInviteesCmd(token),
		s.newEventCancelCmd(token),
	)
	invitee := newGroupCmd("invitee", "Mark or clear invitee no-shows")
	invitee.AddCommand(
		s.newInviteeNoShowCmd(token),
	)
	link := newGroupCmd("link", "Mint single-use scheduling links")
	link.AddCommand(
		s.newLinkCreateCmd(token),
	)
	book := newGroupCmd("book", "Book a slot on an invitee's behalf (Scheduling API)")
	book.AddCommand(
		s.newBookCreateCmd(token),
	)
	org := newGroupCmd("org", "Resolve organization memberships")
	org.AddCommand(
		s.newOrgMembersCmd(token),
	)

	root.AddCommand(
		s.newMeCmd(token),
		eventType, availability, event, invitee, link, book, org,
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
