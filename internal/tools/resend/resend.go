// Package resend is the built-in Resend service: a non-interactive cobra tree
// over the Resend REST API (https://api.resend.com). Auth is a single
// team-scoped API key delivered as "Authorization: Bearer <re_...>"; there is
// no OAuth flow. Every command emits the provider JSON on stdout (passthrough +
// newline).
//
// Credential rejection keys on the parsed error `name`, never the raw HTTP
// status: Resend overloads both 401 (missing_api_key vs the valid
// restricted_api_key) and 403 (invalid_api_key vs validation_error, including
// the default unverified-domain state), so a raw-status rule would tear down a
// valid key. Only name ∈ {invalid_api_key, missing_api_key} rejects the
// credential; everything else (restricted_api_key, validation_error, rate
// limits, unparseable bodies) is a plain passthrough error the agent acts on.
package resend

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Resend API base.
const DefaultBaseURL = "https://api.resend.com"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/resend.json). Resend keys are non-expiring, team-scoped
// bearer keys of the form re_...
const EnvAPIKey = "RESEND_API_KEY"

// userAgent is set explicitly on every request: Resend rejects requests with a
// missing User-Agent with 403, so we never rely on Go's default.
const userAgent = "helio-anycli/resend"

// readOnly / writeAction annotate leaf commands for the design-318 approval
// gate: "false" for side-effect-free reads (GET list/get), "true" for
// provider-state mutations (send, create, update, verify, delete, cancel).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Resend tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Resend API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one resend subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flag combos, invalid JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Resend
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "RESEND_API_KEY is not set"})
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
	if asAPIError(err, &apiErr) {
		// Runtime/API failure: exit 1, preserving the credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(err), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
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

func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "resend",
		Short:         "Resend built-in service (transactional & broadcast email)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newEmailCmd(key),
		s.newDomainCmd(key),
		s.newAudienceCmd(key),
		s.newContactCmd(key),
		s.newBroadcastCmd(key),
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
