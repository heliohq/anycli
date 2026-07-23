// Package searchconsole is the built-in Google Search Console service: a
// non-interactive cobra tree over the Search Console API. It fronts two
// production bases that are a fact of this API — the Webmasters v3 surface
// (sites / sitemaps / searchAnalytics) and the URL Inspection v1 surface — so
// an agent can answer "how did our site do in search", "is this page indexed",
// and "submit the new sitemap" from one tool. Native API enum values (type,
// dimensions, filter operators, dataState, aggregationType) pass through
// verbatim; the service owns only credential injection, siteUrl/feedpath path
// escaping, and error shaping. A 401/403 usually means the token lacks the
// webmasters scope the user never granted — those errors carry a reconnect hint.
package searchconsole

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

// DefaultBaseURL is the production Webmasters v3 base (sites, sitemaps,
// searchAnalytics).
const DefaultBaseURL = "https://www.googleapis.com/webmasters/v3"

// DefaultInspectBaseURL is the production URL Inspection v1 base — a distinct
// host from the Webmasters surface, hence a separate overridable field.
const DefaultInspectBaseURL = "https://searchconsole.googleapis.com/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/search-console.json).
const EnvAccessToken = "SEARCH_CONSOLE_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks the webmasters scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// readOnly / writeAction tag leaf commands for the design-318 approval gate.
// Search Console's query (searchAnalytics) and inspect are analytics reads
// even though they POST; sites/sitemaps list+get are reads; only sitemap
// submit (PUT) and delete mutate provider state.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Search Console tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the Webmasters v3 base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// InspectBaseURL overrides the URL Inspection v1 base; empty =
	// DefaultInspectBaseURL. Tests point it at the same httptest server under a
	// distinct path prefix.
	InspectBaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// now overrides the clock used by --days window math; nil = time.Now. Tests
	// inject a fixed America/Los_Angeles instant.
	now func() time.Time
}

// Execute runs one search-console subcommand with the resolved credentials in
// env. Success is exit 0; usage/param errors (illegal flag combos, missing
// required flags, bad filter syntax, unknown subcommands) are exit 2;
// runtime/API errors (Search Console non-2xx, transport failure) are exit 1.
// Errors render to stderr — as JSON under --json, plain text otherwise.
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

func (s *Service) inspectBase() string {
	if s.InspectBaseURL != "" {
		return strings.TrimRight(s.InspectBaseURL, "/")
	}
	return DefaultInspectBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "search-console",
		Short:         "Google Search Console built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	root.AddCommand(
		s.newSitesCmd(token),
		s.newSitemapsCmd(token),
		s.newQueryCmd(token),
		s.newInspectCmd(token),
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

// jsonOut reports whether the persistent --json flag was set for cmd.
func jsonOut(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}
