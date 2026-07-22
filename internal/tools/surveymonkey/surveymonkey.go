// Package surveymonkey implements the built-in SurveyMonkey service over the
// SurveyMonkey REST API v3. It accepts a Bearer OAuth access token and exposes a
// non-interactive, read-only Cobra tree over surveys, responses, and collectors.
//
// The tool is read-only by design: it wraps the discover -> structure ->
// responses -> identity path an AI teammate uses to analyze survey results, plus
// a generic GET escape hatch. Reading survey answers (response bulk / response
// get) requires the connected account to hold the paid responses_read_detail
// scope; the service maps SurveyMonkey's 1014/1015 permission codes to an
// explicit "reading answers requires a paid SurveyMonkey plan" message rather
// than letting an opaque 403 fall through.
package surveymonkey

import (
	"context"
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

// DefaultBaseURL is the production SurveyMonkey API base for default-region (US)
// accounts. Non-default-region accounts (served from a different datacenter per
// the token-exchange access_url) are an explicit known cap in v1: those calls
// fail with error 1018, which the service maps to a clear region message.
const DefaultBaseURL = "https://api.surveymonkey.com/v3"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/surveymonkey.json).
const EnvAccessToken = "SURVEYMONKEY_ACCESS_TOKEN"

// Service implements the built-in SurveyMonkey tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API base; empty = DefaultBaseURL. Tests point it at
	// an httptest server (with the /v3 segment included).
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one surveymonkey subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flags, unknown
// subcommands, bad flag values) are exit 2; runtime/API errors (SurveyMonkey
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
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

// newRoot builds the grouped-by-resource cobra tree. me / fetch are top-level;
// surveys, responses, and collectors hang under resource groups.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "surveymonkey",
		Short:         "SurveyMonkey built-in service (read-only surveys and responses)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	survey := newGroupCmd("survey", "Manage surveys")
	survey.AddCommand(
		s.newSurveyListCmd(token),
		s.newSurveyGetCmd(token),
		s.newSurveyDetailsCmd(token),
	)
	response := newGroupCmd("response", "Read survey responses")
	response.AddCommand(
		s.newResponseListCmd(token),
		s.newResponseBulkCmd(token),
		s.newResponseGetCmd(token),
	)
	collector := newGroupCmd("collector", "List survey collectors")
	collector.AddCommand(
		s.newCollectorListCmd(token),
	)

	root.AddCommand(
		s.newMeCmd(token),
		s.newFetchCmd(token),
		survey, response, collector,
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

// requireFlag returns a usageError when a required string flag is empty.
func requireFlag(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return &usageError{msg: fmt.Sprintf("--%s is required", name)}
	}
	return nil
}
