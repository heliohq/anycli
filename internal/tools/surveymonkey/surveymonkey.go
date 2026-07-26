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

// readOnly carries the design-318 anycli.side_effect annotation for runnable
// leaf commands. Every SurveyMonkey command is a GET read (surveys, responses,
// collectors, identity, and the GET-only fetch escape hatch), so all leaves
// carry it.
var readOnly = map[string]string{"anycli.side_effect": "false"}

// newRoot builds the grouped-by-resource cobra tree. me / fetch are top-level;
// surveys, responses, and collectors hang under resource groups.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "surveymonkey",
		Short: "SurveyMonkey built-in service (read-only surveys and responses)",
		Long: "Read-only over SurveyMonkey's v3 API: nothing here creates, edits or\n" +
			"deletes a survey, a collector or a response. Output is SurveyMonkey's own\n" +
			"JSON, with lists keeping the `{data, page, per_page, total, links}`\n" +
			"envelope; page with `--page` / `--per-page`, both 1-based and both left to\n" +
			"the provider default when unset.\n" +
			"\n" +
			"Analysis is a chain: find the survey, read its STRUCTURE, then read the\n" +
			"responses. Answers reference question ids and answer-option ids, never\n" +
			"human text, so `survey details` is what makes a response readable at all.\n" +
			"\n" +
			"Reading actual answers is PLAN-GATED. `response bulk` and `response get`\n" +
			"are the only two commands that return them and both need the paid\n" +
			"`responses_read_detail` permission; on a free plan they fail with an\n" +
			"explicit paid-plan message. That is a plan limit, not a transient error —\n" +
			"do not retry and do not look for another route to the same data. A great\n" +
			"deal still works without it: `survey get` carries `response_count`,\n" +
			"`response list` paginates response metadata and its envelope `total`\n" +
			"gives filtered counts, `survey details` gives the full structure, and\n" +
			"`collector list` shows where responses came from.\n" +
			"\n" +
			"Ids are passed as flags (`--id`, `--survey`), never positionally. An\n" +
			"account served from a non-default datacenter, such as EU data residency,\n" +
			"is not reachable and fails with a region message that has no workaround\n" +
			"here. A 429 is SurveyMonkey's per-minute or per-day app limit and clears\n" +
			"only after its reset window.",
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
