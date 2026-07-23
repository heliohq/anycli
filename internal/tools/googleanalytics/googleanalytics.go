// Package googleanalytics is the built-in Google Analytics (GA4) service: a
// non-interactive cobra tree over the two official reporting surfaces — the
// Analytics Data API v1beta (runReport / runRealtimeReport / metadata) and the
// Analytics Admin API v1beta (accountSummaries for property discovery). It is
// read-only by design: everything runs on the analytics.readonly scope; no
// admin writes and no Measurement Protocol ingestion. Date values pass native
// Data API forms verbatim (YYYY-MM-DD, NdaysAgo, yesterday, today). A 401/403
// very often means the token lacks a scope the user never granted — those
// errors carry an explicit reconnect hint.
package googleanalytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultDataBaseURL is the production Analytics Data API v1beta base.
const DefaultDataBaseURL = "https://analyticsdata.googleapis.com/v1beta"

// DefaultAdminBaseURL is the production Analytics Admin API v1beta base.
const DefaultAdminBaseURL = "https://analyticsadmin.googleapis.com/v1beta"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/google-analytics.json).
const EnvAccessToken = "GOOGLE_ANALYTICS_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect (analytics.readonly), or an
// API not enabled on the OAuth client's Google Cloud project.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for state-free reads, "true" for provider
// mutations. Group commands must not carry either. Google Analytics is
// read-only, so every leaf here is readOnly (report run/realtime are reads even
// though the Data API serves them over POST).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Google Analytics tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle). Two upstream hosts, so two base-URL overrides.
type Service struct {
	// DataBaseURL overrides the Analytics Data API base; empty =
	// DefaultDataBaseURL. Tests point it at an httptest server.
	DataBaseURL string
	// AdminBaseURL overrides the Analytics Admin API base; empty =
	// DefaultAdminBaseURL. Tests point it at an httptest server.
	AdminBaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// sleep overrides the retry backoff sleeper; nil = time.Sleep. Tests
	// inject a recorder to keep retries deterministic and fast.
	sleep func(time.Duration)
}

// Execute runs one google-analytics subcommand with the resolved credentials
// in env. Success is exit 0; usage/param errors (illegal flag combos, bad
// enums, invalid JSON, missing required flags, unknown subcommands) are exit
// 2; runtime/API errors (non-2xx, transport failure) are exit 1. Errors render
// to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAccessToken + " is not set"})
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

// hasJSONArg reports whether the raw args carry the --json global flag, used
// to pick the error format before cobra has parsed flags.
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

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// dataBase returns the Data API base URL without a trailing slash.
func (s *Service) dataBase() string {
	if s.DataBaseURL != "" {
		return strings.TrimRight(s.DataBaseURL, "/")
	}
	return DefaultDataBaseURL
}

// adminBase returns the Admin API base URL without a trailing slash.
func (s *Service) adminBase() string {
	if s.AdminBaseURL != "" {
		return strings.TrimRight(s.AdminBaseURL, "/")
	}
	return DefaultAdminBaseURL
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "google-analytics",
		Short:         "Google Analytics (GA4) built-in service (read-only reporting)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output the provider JSON response instead of the human-readable table")

	property := newGroupCmd("property", "GA4 properties (discovery: every report call needs a numeric property id)")
	property.AddCommand(s.newPropertyListCmd(token))

	report := newGroupCmd("report", "GA4 reports (run / realtime / metadata)")
	report.AddCommand(
		s.newReportRunCmd(token),
		s.newReportRealtimeCmd(token),
		s.newReportMetadataCmd(token),
	)

	root.AddCommand(property, report)
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
