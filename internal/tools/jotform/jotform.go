// Package jotform is the built-in Jotform service: a non-interactive cobra
// tree over the Jotform v1 REST surface (https://api.jotform.com). Auth is the
// provider's custom scheme — the API key is sent as the raw "APIKEY" request
// header, with no "Bearer" prefix. Jotform wraps every response in a
// {"responseCode":200,"message":"success","content":{...}} envelope; the AI
// reads content. A non-2xx status (or a responseCode other than 200) becomes a
// typed apiError. Read verbs (user/usage/form/submission list/get) work with
// any key; write verbs (submission create/edit/delete) need a Full Access key,
// and a read-only key's Jotform 401 is surfaced verbatim.
package jotform

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

// DefaultBaseURL is the production Jotform (US) API base. EU
// (eu-api.jotform.com) and HIPAA (hipaa-api.jotform.com) accounts are out of
// scope for v1: a key only authenticates against its own account's region.
const DefaultBaseURL = "https://api.jotform.com"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/jotform.json). Jotform keys are long-lived, per-account,
// and non-expiring.
const EnvAPIKey = "JOTFORM_API_KEY"

// authHeader is Jotform's custom API-key header. The key is sent raw (no
// "Bearer" prefix), matching the provider bundle's auth.api_key.header.
const authHeader = "APIKEY"

// Service implements the built-in Jotform tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Jotform API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one jotform subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required args, bad --field
// syntax, unknown subcommands) are exit 2; runtime/API errors (Jotform non-2xx,
// transport failure) are exit 1. Errors render to stderr — as JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		// The key check runs before cobra parses flags, so detect --json in the
		// raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "JOTFORM_API_KEY is not set"})
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
		// Runtime/API failure: exit 1.
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

func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "jotform",
		Short:         "Jotform built-in service (API key)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	root.AddCommand(
		s.newUserCmd(key),
		s.newUsageCmd(key),
		s.newFormCmd(key),
		s.newSubmissionCmd(key),
		s.newReportCmd(key),
		s.newFolderCmd(key),
	)
	return root
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
