// Package calendar is the built-in Google Calendar service: a non-interactive
// cobra tree projecting the Calendar API v3 resource namespaces (calendars /
// events / freebusy) plus a few safe synthetic verbs (respond, --meet). Search
// and time-window flags pass native API values (q, timeMin/timeMax RFC3339,
// RFC 5545 RRULE) through verbatim. A 401/403 very often means the token lacks
// a scope the user never granted — those errors carry an explicit reconnect
// hint. Destructive gradient (design 303): reaching an attendee is gated by a
// skill-level confirmation soft-guardrail, not by the tool; the tool only
// exposes safe verbs and sane defaults.
package calendar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Calendar API v3 base.
const DefaultBaseURL = "https://www.googleapis.com/calendar/v3"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/calendar.json).
const EnvAccessToken = "CALENDAR_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// defaultCalendar is the calendar id used when --calendar is omitted.
const defaultCalendar = "primary"

// Service implements the built-in Calendar tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Calendar API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// sleep overrides the retry backoff sleeper; nil = time.Sleep. Tests
	// inject a recorder to keep retries deterministic and fast.
	sleep func(time.Duration)
	// newRequestID overrides the Meet createRequest id generator; nil = a
	// crypto-random hex id. Tests inject a deterministic generator.
	newRequestID func() string
}

// Execute runs one calendar subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "CALENDAR_ACCESS_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
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

func (s *Service) base() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "calendar",
		Short: "Google Calendar built-in service",
		Long: "Every timed argument is RFC3339 WITH an explicit offset\n" +
			"(2026-07-16T14:00:00-07:00) and anything else is rejected before the\n" +
			"call; --all-day boundaries are bare YYYY-MM-DD instead. A calendar's own\n" +
			"IANA zone comes from `calendars list`, and a time reported back to a\n" +
			"person should carry its offset rather than a naked clock reading.\n" +
			"\n" +
			"--calendar defaults to `primary` wherever it appears, and an event id is\n" +
			"only resolvable against the calendar that holds it. Ids for other\n" +
			"calendars, and the caller's access role on each, come from\n" +
			"`calendars list`.\n" +
			"\n" +
			"Finding a free slot is `freebusy`, not `events list` pointed at someone\n" +
			"else's calendar: freebusy needs only free/busy sharing and returns no\n" +
			"event content.\n" +
			"\n" +
			"A recurring event has two editable levels. The base event's id governs\n" +
			"the whole series; one occurrence has its own id, which only\n" +
			"`events instances` produces. Editing the base when a single occurrence\n" +
			"was meant is the most common way to damage a calendar.\n" +
			"\n" +
			"Output defaults to a one-line-per-item human summary. --json prints the\n" +
			"full API resource and is the only way to see attendees, response\n" +
			"statuses, descriptions or recurrence rules.\n" +
			"\n" +
			"A 401 or 403 here usually means the token was never granted the scope\n" +
			"the command needs, rather than that it expired.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	calendars := newGroupCmd("calendars", "Calendars (calendarList: ids, time zones, access roles)")
	calendars.AddCommand(s.newCalendarsListCmd(token), s.newCalendarsGetCmd(token))

	events := newGroupCmd("events", "Events")
	events.AddCommand(
		s.newEventsListCmd(token),
		s.newEventsGetCmd(token),
		s.newEventsInstancesCmd(token),
		s.newEventsCreateCmd(token),
		s.newEventsUpdateCmd(token),
		s.newEventsDeleteCmd(token),
		s.newEventsRespondCmd(token),
	)

	root.AddCommand(calendars, events, s.newFreebusyCmd(token))
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: bare
// group shows help, unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// jsonOut reports whether the persistent --json flag was set for cmd.
func jsonOut(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token
// is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
