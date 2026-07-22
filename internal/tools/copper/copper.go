// Package copper is the built-in Copper CRM service: a non-interactive cobra
// tree over the Copper Developer API v1 (https://api.copper.com/developer_api/v1).
//
// Auth is an OAuth 2.0 access token sent as "Authorization: Bearer <token>"
// plus "Content-Type: application/json". Copper's legacy X-PW-AccessToken /
// X-PW-Application / X-PW-UserEmail header trio is the API-KEY path only and is
// NOT sent on the OAuth path (verified against Copper's OAuth quickstart).
//
// Copper models list/read as POST /{resource}/search (a JSON filter body with
// page_number / page_size), not GET-with-query. Every command emits the
// provider JSON on stdout verbatim (+ newline). Errors render to stderr — as a
// structured JSON envelope under --json, plain text otherwise. Exit codes: 0
// success, 1 runtime/API failure (Copper non-2xx, transport), 2 usage/parse
// (bad flags, invalid --json-body, unknown subcommand).
package copper

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

// DefaultBaseURL is the production Copper Developer API v1 base. The former
// api.prosperworks.com host is retired.
const DefaultBaseURL = "https://api.copper.com/developer_api/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/copper.json). Copper OAuth access tokens do not expire and
// have no refresh token.
const EnvAccessToken = "COPPER_ACCESS_TOKEN"

// Service implements the built-in Copper tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Copper API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one copper subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flags, invalid JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Copper
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "COPPER_ACCESS_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. Each CRM record type is a
// resource group with a uniform verb set; account / user / lookup are
// read-only helper groups.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "copper",
		Short:         "Copper CRM built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output on the error channel")

	root.AddCommand(
		s.newAccountCmd(token),
		s.newUserCmd(token),
		s.newActivityCmd(token),
		s.newLookupCmd(token),
	)
	// Uniform CRUD record resources. people additionally carries find-email.
	for _, r := range crudResources {
		root.AddCommand(s.newResourceCmd(token, r))
	}
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: a bare
// group shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}
