// Package paperform is the built-in Paperform service: a read-only,
// non-interactive cobra tree over the Paperform v1 REST surface
// (https://api.paperform.co/v1). Auth is "Authorization: Bearer <api_key>",
// where the key is the account-page API token. Paperform errors are non-2xx
// with a JSON body carrying a "message" field; 401 rejects the credential and
// 429 surfaces its Retry-After. Every command emits the provider JSON on stdout
// verbatim (passthrough + newline).
//
// Exit codes: 0 success, 1 runtime/API failure (non-2xx, transport, missing
// credential), 2 usage/parse error (bad flags, missing required flag, unknown
// subcommand). Errors render to stderr — a JSON envelope
// {"error":{"message":…,"status":<HTTP or omitted>}} under --json, plain text
// otherwise.
package paperform

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

// DefaultBaseURL is the production Paperform v1 API base.
const DefaultBaseURL = "https://api.paperform.co/v1"

// readOnly is the design-318 anycli.side_effect annotation for runnable leaf
// commands. The Paperform service is entirely read-only (every leaf issues a
// GET), so all leaves carry this and no writeAction var exists.
var readOnly = map[string]string{"anycli.side_effect": "false"}

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/paperform.json). The value is the raw account API key;
// the service prepends "Bearer " itself.
const EnvAPIKey = "PAPERFORM_API_KEY"

// Service implements the built-in Paperform tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Paperform API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one paperform subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAPIKey + " is not set"})
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
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
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
// {"error":{"message":…,"status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.status != 0 {
		payload["status"] = apiErr.status
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

func (s *Service) baseURL() string {
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

// newRoot builds the grouped-by-resource cobra tree. Every group is a runnable
// command so an unknown subcommand fails (exit 2) instead of printing help with
// a false success.
func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:   "paperform",
		Short: "Paperform built-in service (read-only)",
		Long: "Entirely read-only: there is no command that creates, edits, closes or\n" +
			"deletes a form, a submission or a coupon. Paperform's API is also gated on\n" +
			"a Standard or Business plan, so on a lower plan every command fails at\n" +
			"authentication rather than returning empty results.\n" +
			"\n" +
			"A form is addressed by its slug OR its id and `--form` accepts either.\n" +
			"Submissions carry their answers keyed by FIELD KEY, not by question text,\n" +
			"so `field list` is what turns a response into something readable.\n" +
			"\n" +
			"Paging flags exist on `form list`, `submission list` and\n" +
			"`partial-submission list` only: `--limit` (Paperform defaults to 20 and\n" +
			"caps it at 100), `--skip`, `--sort ASC|DESC` — newest-first by created_at\n" +
			"unless changed — plus the `--after-id` / `--before-id` and `--after-date` /\n" +
			"`--before-date` (UTC) windows. `field list`, `product list`, `coupon list`\n" +
			"and `space list` take none of them and return their whole set in one call.\n" +
			"\n" +
			"The provider's JSON is printed verbatim. A 429 surfaces Paperform's retry\n" +
			"delay in the error message — wait that long rather than retrying at once.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newFormCmd(key),
		s.newFieldCmd(key),
		s.newSubmissionCmd(key),
		s.newPartialSubmissionCmd(key),
		s.newSpaceCmd(key),
		s.newProductCmd(key),
		s.newCouponCmd(key),
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
