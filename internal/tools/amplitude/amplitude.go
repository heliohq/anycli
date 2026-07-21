// Package amplitude is the built-in Amplitude service: a read/query cobra tree
// over Amplitude's Dashboard REST, Export, and Behavioral Cohorts APIs
// (https://amplitude.com/docs/apis). It is analysis-only — event ingestion
// (HTTP V2 / Batch) is deliberately out of scope.
//
// Auth is HTTP Basic: the injected AMPLITUDE_API_CREDENTIALS value is the raw
// `apiKey:secretKey` pre-image (the literal Basic user:password string), so it
// is base64-encoded directly into the Authorization header. Amplitude projects
// live in exactly one data-residency silo; --region us|eu selects the host
// (default us). A 401 from the default (unasserted) US host is region-ambiguous
// — it may be a live EU-project key hitting the wrong silo — so it is NOT
// treated as a dead credential (see auth_error.go).
package amplitude

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// Host bases for the two Amplitude data-residency silos. analytics.amplitude.com
// is the web UI, not a REST host, so it is intentionally absent.
const (
	usBaseURL = "https://amplitude.com"
	euBaseURL = "https://analytics.eu.amplitude.com"
)

// EnvCredentials is the env var the credential binding injects
// (definitions/tools/amplitude.json). Its value is the raw Basic-auth pre-image
// `apiKey:secretKey`.
const EnvCredentials = "AMPLITUDE_API_CREDENTIALS"

// Service implements the built-in Amplitude tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the resolved region host; empty = region-selected host.
	// Tests point it at an httptest server while still exercising region flags.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one amplitude subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, malformed credentials) are
// exit 2; runtime/API errors (Amplitude non-2xx, transport failure) are exit 1.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	authHeader, err := parseCredentials(env[EnvCredentials])
	if err != nil {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), err)
		return execution.Result{ExitCode: 2}, nil
	}

	root := s.newRoot(authHeader)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		jsonMode, _ := root.PersistentFlags().GetBool("json")
		s.renderError(jsonMode, err)

		var apiErr *apiError
		if errors.As(err, &apiErr) {
			// Runtime/API failure: exit 1, preserving credential-rejection
			// classification carried through the wrapped cause.
			return execution.Failure(err), nil
		}
		// usageError plus every cobra parse/arg/enum/unknown-command error is a
		// usage error → exit 2.
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
}

// parseCredentials validates the injected AMPLITUDE_API_CREDENTIALS value and
// returns the "Basic <base64>" header. The value must split into exactly two
// non-empty halves on the FIRST colon (Amplitude secret keys never contain a
// colon, so an errant one fails loudly). The raw value is already the Basic
// user:password pre-image, so it is base64-encoded directly. The error is
// static and never echoes the secret.
func parseCredentials(raw string) (string, error) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", &usageError{msg: "AMPLITUDE_API_CREDENTIALS must be set to your Amplitude project credentials in the form apiKey:secretKey"}
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw)), nil
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

// newRoot builds the cobra tree. events / cohorts are runnable groups; the
// query commands (segmentation, funnels, retention, user-search, user-activity,
// chart, export) are top-level.
func (s *Service) newRoot(authHeader string) *cobra.Command {
	root := &cobra.Command{
		Use:           "amplitude",
		Short:         "Amplitude product analytics (read/query built-in service)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output (default for all data commands)")
	pf.String("region", "us", "Amplitude data-residency region: us|eu (must match the project's silo)")
	// --base-url is a hidden test/override seam; region selects the host in
	// normal use.
	pf.String("base-url", "", "override the API base URL (advanced/testing)")
	_ = pf.MarkHidden("base-url")

	events := newGroupCmd("events", "Event catalog")
	events.AddCommand(s.newEventsListCmd(authHeader))
	cohorts := newGroupCmd("cohorts", "Behavioral cohorts")
	cohorts.AddCommand(s.newCohortsListCmd(authHeader))

	root.AddCommand(
		s.newSegmentationCmd(authHeader),
		s.newFunnelsCmd(authHeader),
		s.newRetentionCmd(authHeader),
		s.newUserSearchCmd(authHeader),
		s.newUserActivityCmd(authHeader),
		s.newChartCmd(authHeader),
		s.newExportCmd(authHeader),
		events,
		cohorts,
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it.
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
