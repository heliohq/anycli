// Package microsoftcalendar is the built-in Microsoft (Outlook) Calendar
// service: a non-interactive cobra tree projecting the Microsoft Graph v1.0
// calendar resources (/me/calendars, /me/events, /me/calendarView) plus the
// synthetic freebusy / cancel / respond verbs (design 308 §microsoft_calendar).
//
// v1 is locked to a single delegated Graph scope, Calendars.ReadWrite: read
// events, create/update/cancel events, and reply to invites. Reading OTHER
// attendees' free/busy (findMeetingTimes / getSchedule) needs
// Calendars.Read.Shared and is out of v1 — the freebusy verb here computes the
// signed-in user's own busy windows from /me/calendarView only.
//
// A 401/403 very often means the token lacks a scope the user never granted —
// those errors carry an explicit reconnect hint.
package microsoftcalendar

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

// DefaultBaseURL is the production Microsoft Graph v1.0 base.
const DefaultBaseURL = "https://graph.microsoft.com/v1.0"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/microsoft-calendar.json).
const EnvAccessToken = "MICROSOFT_CALENDAR_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Microsoft Calendar tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the Graph API base; empty = DefaultBaseURL. Tests
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
}

// Execute runs one microsoft-calendar subcommand with the resolved credentials
// in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), EnvAccessToken+" is not set")
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
		Use:           "microsoft-calendar",
		Short:         "Microsoft (Outlook) Calendar built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	calendars := newGroupCmd("calendars", "Calendars")
	calendars.AddCommand(s.newCalendarsListCmd(token))

	events := newGroupCmd("events", "Events")
	events.AddCommand(
		s.newEventsListCmd(token),
		s.newEventsGetCmd(token),
		s.newEventsCreateCmd(token),
		s.newEventsUpdateCmd(token),
		s.newEventsCancelCmd(token),
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
