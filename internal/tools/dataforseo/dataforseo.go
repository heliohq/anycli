// Package dataforseo is the built-in DataForSEO service: a non-interactive
// cobra tree over the DataForSEO v3 REST surface (https://api.dataforseo.com/v3).
//
// Auth is HTTP Basic — the resolved credential is a login:password pair stored
// as one secret, base64-encoded into "Authorization: Basic <base64(pair)>".
//
// DataForSEO returns HTTP 200 for almost everything and rides real errors in
// the body: a top-level status_code and a per-task status_code, where 20000
// ("Ok.") is success. Every command therefore inspects the body status codes,
// not the HTTP status alone, unwraps tasks[0].result, and emits {cost, result}
// — cost is first-class because every non-appendix call spends real money.
package dataforseo

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

// readOnly / writeAction carry the design-318 side-effect annotation for runnable leaves.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}

// DefaultBaseURL is the production DataForSEO v3 API base.
const DefaultBaseURL = "https://api.dataforseo.com/v3"

// EnvCredentials is the env var the credential binding injects
// (definitions/tools/dataforseo.json). The value is the login:password pair.
const EnvCredentials = "DATAFORSEO_CREDENTIALS"

// Service implements the built-in DataForSEO tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API base; empty = DefaultBaseURL. Tests point it at
	// an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one dataforseo subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flags, bad enums,
// unknown subcommands) are exit 2; runtime/API errors (non-2xx HTTP, in-body
// error status codes, transport failure) are exit 1. Errors render to stderr —
// as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	credential := env[EnvCredentials]
	if credential == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvCredentials + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(credential)
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
// missing-credential check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api"}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(credential string) *cobra.Command {
	root := &cobra.Command{
		Use:           "dataforseo",
		Short:         "DataForSEO built-in service (SERP, keywords, backlinks, on-page)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (output is always JSON)")

	root.AddCommand(
		s.newSERPCmd(credential),
		s.newKeywordsCmd(credential),
		s.newDomainCmd(credential),
		s.newBacklinksCmd(credential),
		s.newOnpageCmd(credential),
		s.newMetaCmd(credential),
		s.newAccountCmd(credential),
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
