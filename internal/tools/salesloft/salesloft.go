// Package salesloft is the built-in Salesloft service: a non-interactive cobra
// tree over the Salesloft REST API v2 (https://api.salesloft.com/v2). Auth is
// "Authorization: Bearer <access_token>". Requests and responses are JSON with
// a `data` payload key and, on list endpoints, a `metadata` block carrying
// paging. Salesloft errors are a non-2xx status with either an `error` string
// (403/404) or an `errors` field→messages map (422); every command surfaces
// them. Responses are passed through verbatim so the agent keeps Salesloft's
// own envelope (data + metadata.paging) without a second vocabulary.
package salesloft

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

// DefaultBaseURL is the production Salesloft API host; the /v2 version prefix is
// added per request in call, so paths stay version-relative (e.g. "/me").
const DefaultBaseURL = "https://api.salesloft.com"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/salesloft.json). Salesloft OAuth access tokens expire
// after 2 hours; the token gateway refreshes them out of band.
const EnvAccessToken = "SALESLOFT_ACCESS_TOKEN"

// Service implements the built-in Salesloft tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Salesloft API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one salesloft subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flag combos, invalid JSON, missing
// required flags, unknown subcommands, per-page over the max) are exit 2;
// runtime/API errors (Salesloft non-2xx, transport failure) are exit 1. Errors
// render to stderr — JSON envelope under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "SALESLOFT_ACCESS_TOKEN is not set"})
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

// newRoot builds the resource-grouped cobra tree. me is top-level (no resource);
// everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "salesloft",
		Short:         "Salesloft built-in service (people, cadences, tasks, activity)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (responses are already JSON)")

	root.AddCommand(
		s.newMeCmd(token),
		s.newUserCmd(token),
		s.newPersonCmd(token),
		s.newAccountCmd(token),
		s.newCadenceCmd(token),
		s.newTaskCmd(token),
		s.newNoteCmd(token),
		s.newActivityCmd(token),
		s.newEmailCmd(token),
		s.newCallCmd(token),
	)
	return root
}

// newGroupCmd is a runnable command group: a bare group prints help and a bad
// subcommand fails (cobra skips Args validation on non-runnable commands, which
// would exit 0 on an unknown subcommand — a false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
