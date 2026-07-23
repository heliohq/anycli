// Package hubspot is the built-in HubSpot service: a non-interactive cobra
// tree over the HubSpot CRM v3 object surface (https://api.hubapi.com). Auth is
// "Authorization: Bearer <token>" from an OAuth access token. Records
// (contacts, companies, deals, tickets), engagements (notes, tasks),
// associations, owners, pipelines, and properties are read and written as the
// provider's own JSON, which every command emits on stdout verbatim (+ newline).
//
// HubSpot fails with a non-2xx status and a JSON body carrying
// status/message/category/correlationId; every call surfaces the message. A 401
// (typically category EXPIRED_AUTHENTICATION) rejects the credential so the host
// prompts a reconnect.
package hubspot

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

// DefaultBaseURL is the production HubSpot API base. Paths carry their own
// version segment (/crm/v3, /crm/v4, /account-info/v3), so the base is bare.
const DefaultBaseURL = "https://api.hubapi.com"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/hubspot.json). HubSpot OAuth access tokens are short-lived
// (~30 min); the Helio token gateway refreshes them, so the tool only ever sees
// a currently-valid bearer.
const EnvAccessToken = "HUBSPOT_ACCESS_TOKEN"

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for state-free reads, "true" for provider
// mutations. Group commands must not carry either.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in HubSpot tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the HubSpot API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one hubspot subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing required flags,
// malformed --prop/--filter, unknown subcommands) are exit 2; runtime/API
// errors (HubSpot non-2xx, transport failure) are exit 1. Errors render to
// stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// A missing bound token is a credential problem, not a flag-usage error:
		// classify it as a credential rejection so the exit code (1) and the
		// --json envelope kind ("credential") agree and the host prompts a
		// reconnect. The check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		credErr := execution.RejectCredential(errors.New(EnvAccessToken + " is not set"))
		s.renderError(hasJSONArg(args), credErr)
		return execution.Failure(credErr), nil
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
// {"error":{"message":…,"kind":"usage|api|credential","status":<HTTP or
// omitted>}}. kind mirrors the exit code: "usage" → exit 2, "api"/"credential"
// → exit 1. A 401 keeps kind "api" (it carries an HTTP status); a missing bound
// token renders as the bare "credential" rejection.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	switch {
	case errors.As(err, &apiErr):
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	case execution.IsCredentialRejected(err):
		payload["kind"] = "credential"
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

// newRoot builds the grouped-by-resource cobra tree. Each CRM object type
// (contact/company/deal/ticket) is a group with identical verbs; engagements
// (note/task), associations, owners, pipelines, and properties get their own
// groups; account is a top-level whoami/smoke command.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "hubspot",
		Short:         "HubSpot CRM built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output (always on; accepted for uniformity)")

	root.AddCommand(
		s.newAccountCmd(token),
		s.newObjectGroup(token, "contact", "contacts"),
		s.newObjectGroup(token, "company", "companies"),
		s.newObjectGroup(token, "deal", "deals"),
		s.newObjectGroup(token, "ticket", "tickets"),
		s.newNoteGroup(token),
		s.newTaskGroup(token),
		s.newAssocGroup(token),
		s.newOwnerGroup(token),
		s.newPipelineGroup(token),
		s.newPropertyGroup(token),
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
