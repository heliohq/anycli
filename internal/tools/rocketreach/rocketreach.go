// Package rocketreach is the built-in RocketReach service: a non-interactive
// cobra tree over the RocketReach v2 REST surface
// (https://api.rocketreach.co/api/v2). Auth is the "Api-Key: <key>" request
// header on every call. RocketReach is a contact-enrichment / prospecting
// database; this tool wraps the jobs an AI teammate actually needs — enrich a
// known person into verified emails/phones, find people or companies, and check
// the remaining credit balance before spending. Person lookups are
// asynchronous: `person lookup` returns a status the agent polls with
// `person status`. Every command emits the provider JSON verbatim on stdout;
// errors render to stderr (plain text, or a structured envelope under --json).
package rocketreach

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

// DefaultBaseURL is the production RocketReach API host. The /api/v2 path
// prefix lives on each request path, not here, so the full API path is visible
// at every call site (and asserted verbatim in tests).
const DefaultBaseURL = "https://api.rocketreach.co"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/rocketreach.json). RocketReach keys are long-lived,
// non-expiring per-user API keys sent as the Api-Key header.
const EnvAPIKey = "ROCKETREACH_API_KEY"

// Service implements the built-in RocketReach tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the RocketReach API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one rocketreach subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flags, bad JSON,
// unknown subcommands) are exit 2; runtime/API errors (RocketReach non-2xx,
// transport failure) are exit 1. Errors render to stderr — as JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "ROCKETREACH_API_KEY is not set"})
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

// newRoot builds the grouped-by-resource cobra tree: `account` is top-level;
// person and company each hang under a runnable resource group.
func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "rocketreach",
		Short:         "RocketReach built-in service (contact enrichment + prospecting)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (output is always JSON; controls error format)")

	person := newGroupCmd("person", "Enrich, poll, and search people")
	person.AddCommand(
		s.newPersonLookupCmd(key),
		s.newPersonStatusCmd(key),
		s.newPersonSearchCmd(key),
	)
	company := newGroupCmd("company", "Look up and search companies")
	company.AddCommand(
		s.newCompanyLookupCmd(key),
		s.newCompanySearchCmd(key),
	)

	root.AddCommand(s.newAccountCmd(key), person, company)
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
