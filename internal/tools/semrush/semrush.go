// Package semrush is the built-in Semrush service: a non-interactive cobra tree
// over the Semrush v3 SEO API (https://api.semrush.com). Auth is a v3 API key
// carried in the `key=` query parameter (no header, no OAuth). Reports return
// semicolon-delimited CSV with human-readable headers; this service parses that
// into provider-neutral JSON rows keyed by snake_cased header names.
//
// Semrush signals failure not by HTTP status (report errors typically arrive
// with HTTP 200) but by a plain-text body of the form "ERROR NN :: MESSAGE".
// The service sniffs that dialect (client.go) and maps it to the exit-code
// contract below.
//
// Exit-code contract (mirrors the notion/bitly built-ins):
//   - 0 success (including ERROR 50 :: NOTHING FOUND — "no data" is a valid
//     answer for an agent, emitted as an empty rows array with a note);
//   - 1 runtime/API failure (any other ERROR NN, transport failure, non-2xx
//     without an ERROR body) via a typed apiError; a rejected/disabled key
//     (ERROR 120/121/122/130) additionally marks the credential rejected;
//   - 2 usage/parse errors (missing args, bad enums, unknown subcommands).
//
// Unit-cost safety: every returned line debits the account's API-unit balance
// and Semrush's server default is 10,000 lines/request, so the service always
// sends an explicit display_limit (default 10); larger pulls require --limit.
package semrush

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

// DefaultReportsBaseURL is the production Semrush v3 SEO API base. Domain,
// keyword, and url reports are GET requests directly against it; backlinks
// reports live under /analytics/v1/ (see analyticsPath).
const DefaultReportsBaseURL = "https://api.semrush.com"

// DefaultUnitsBaseURL hosts the free API-units balance endpoint, which lives on
// the www host rather than the api host.
const DefaultUnitsBaseURL = "https://www.semrush.com"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/semrush.json). Semrush v3 keys are static, non-expiring
// account secrets with no refresh cycle.
const EnvAPIKey = "SEMRUSH_API_KEY"

// Service implements the built-in Semrush tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// ReportsBaseURL overrides the reports API base; empty = DefaultReportsBaseURL.
	// Tests point it at an httptest server.
	ReportsBaseURL string
	// UnitsBaseURL overrides the balance API base; empty = DefaultUnitsBaseURL.
	UnitsBaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one semrush subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "SEMRUSH_API_KEY is not set"})
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
// {"error":{"message":…,"kind":"usage|api","code":<semrush ERROR code>,"status":<HTTP>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.code != 0 {
			payload["code"] = apiErr.code
		}
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

// newRoot builds the grouped-by-resource cobra tree. Every report group hangs
// under a resource group (domain / keyword / url / backlinks); units is a
// top-level balance check.
func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "semrush",
		Short:         "Semrush built-in service (v3 SEO API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newDomainCmd(key),
		s.newKeywordCmd(key),
		s.newURLCmd(key),
		s.newBacklinksCmd(key),
		s.newUnitsCmd(key),
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
