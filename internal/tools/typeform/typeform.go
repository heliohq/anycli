// Package typeform is the built-in Typeform service: a non-interactive cobra
// tree over the api.typeform.com REST surface (Create, Responses, and Webhooks
// APIs). Auth is "Authorization: Bearer <token>" — a personal access token or
// an OAuth access token, which are interchangeable. Content is passed through
// as the provider's own JSON so an agent joins responses against a form's field
// dictionary without any lossy transformation.
//
// Typeform errors are non-2xx with a JSON body carrying code/description
// (+ optional details/help); every call surfaces that message. Exit codes: 0
// success, 1 runtime/API failure (typed apiError), 2 usage/parse errors. Under
// --json, errors render as a structured envelope on stderr.
package typeform

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

// DefaultBaseURL is the production Typeform API base (global data center).
// Accounts homed in Typeform's EU data center use https://api.eu.typeform.com
// (or https://api.typeform.eu for newer EU accounts); v1 targets the global
// base only and keeps BaseURL as the seam for a later EU option.
const DefaultBaseURL = "https://api.typeform.com"

// EnvToken is the env var the credential binding injects
// (definitions/tools/typeform.json).
const EnvToken = "TYPEFORM_TOKEN"

// Service implements the built-in Typeform tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Typeform API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server; a later EU option threads the EU base
	// through here.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one typeform subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad enums, invalid JSON, missing
// required args, unknown subcommands) are exit 2; runtime/API errors (Typeform
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		//
		// Deliberate taxonomy: a missing credential is exit 1 (a runtime
		// classification — the tool cannot run without the injected token),
		// distinct from parse/arg usage errors which are exit 2. Under --json it
		// still renders kind "usage" (there is no separate "credential" kind in
		// the envelope), matching the notion precedent across the tool batch.
		s.renderError(hasJSONArg(args), &usageError{msg: "TYPEFORM_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. me is top-level (identity);
// everything else hangs under a resource group (form, response, workspace,
// webhook).
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "typeform",
		Short:         "Typeform built-in service (forms, responses, workspaces, webhooks)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	form := newGroupCmd("form", "Manage forms")
	form.AddCommand(
		s.newFormListCmd(token),
		s.newFormGetCmd(token),
		s.newFormCreateCmd(token),
		s.newFormUpdateCmd(token),
		s.newFormPatchCmd(token),
		s.newFormDeleteCmd(token),
	)
	response := newGroupCmd("response", "Read form responses")
	response.AddCommand(
		s.newResponseListCmd(token),
	)
	workspace := newGroupCmd("workspace", "Manage workspaces")
	workspace.AddCommand(
		s.newWorkspaceListCmd(token),
		s.newWorkspaceGetCmd(token),
		s.newWorkspaceCreateCmd(token),
	)
	webhook := newGroupCmd("webhook", "Manage a form's webhooks")
	webhook.AddCommand(
		s.newWebhookListCmd(token),
		s.newWebhookGetCmd(token),
		s.newWebhookSetCmd(token),
		s.newWebhookDeleteCmd(token),
	)

	root.AddCommand(s.newMeCmd(token), form, response, workspace, webhook)
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
