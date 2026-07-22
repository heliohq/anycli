// Package apollo is the built-in Apollo.io service: a non-interactive cobra
// tree over the Apollo REST surface (https://api.apollo.io/api/v1). It serves
// an AI sales teammate: find and enrich people/companies, persist contacts,
// enroll them into outbound sequences, create follow-up tasks, and read/write
// deals. Auth is an OAuth 2.0 access token sent as "Authorization: Bearer
// <token>"; anycli injects it via the APOLLO_ACCESS_TOKEN env var. Apollo
// errors are non-2xx with a JSON body carrying an "error"/"errors" message;
// 401 rejects the credential. Every command emits the provider JSON on stdout
// verbatim (passthrough + newline) — matching the notion/bitly convention so
// an agent sees the same output shape across every heliox tool.
//
// A subset of Apollo endpoints (people search, sequence add/remove, deal
// list/update) are documented as master-API-key-only and return 403 to an
// OAuth token. They are shipped as subcommands regardless; the L2 harness pass
// determines which are OAuth-reachable and drops any that provably are not
// (fail-fast, no silent API-key fallback).
package apollo

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

// DefaultBaseURL is the production Apollo REST API base.
const DefaultBaseURL = "https://api.apollo.io/api/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/apollo.json). Apollo OAuth access tokens are Bearer
// tokens with a 30-day lifetime and a rotating refresh cycle (the token
// gateway owns refresh; this service only presents the current access token).
const EnvAccessToken = "APOLLO_ACCESS_TOKEN"

// Service implements the built-in Apollo tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Apollo API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one apollo subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flag combos, invalid JSON,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Apollo non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "APOLLO_ACCESS_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. Every leaf hangs under a
// resource group an agent reasons about (people, org, contacts, accounts,
// sequences, tasks, deals, users, email-accounts).
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "apollo",
		Short:         "Apollo.io built-in service (sales intelligence + engagement)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newPeopleCmd(token),
		s.newOrgCmd(token),
		s.newContactsCmd(token),
		s.newAccountsCmd(token),
		s.newSequencesCmd(token),
		s.newTasksCmd(token),
		s.newDealsCmd(token),
		s.newUsersCmd(token),
		s.newEmailAccountsCmd(token),
	)
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
