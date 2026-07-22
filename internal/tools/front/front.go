// Package front is the built-in Front service: a cobra tree over the Front
// Core API (https://api2.frontapp.com) that lets an AI teammate work a shared
// inbox the way a human agent does — read the conversation queue, reply, leave
// an internal comment, tag/assign/archive, and look up contacts.
//
// Auth is a bearer token injected as FRONT_TOKEN (definitions/tools/front.json).
// The same bearer works whether it is an OAuth access token (production, minted
// by the Helio token gateway) or a Settings→Developers API token (handy for L2
// harness runs) — both are company-scoped.
//
// Output contract: every command emits a provider-neutral JSON envelope on
// stdout and never the raw Front body. List commands emit
// {"data":[…],"next_page_token":"<cursor|empty>"}; single-object commands emit
// {"data":{…}}; no-content mutations emit {"data":{"ok":true}}. Front paginates
// with an absolute _pagination.next URL; the service extracts the opaque
// page_token from it and re-exposes it as next_page_token, accepted back via
// --page-token, so agents never handle Front URLs.
package front

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

// DefaultBaseURL is the production Front Core API base.
const DefaultBaseURL = "https://api2.frontapp.com"

// EnvToken is the env var the credential binding injects (definitions/tools/front.json).
const EnvToken = "FRONT_TOKEN"

// Service implements the built-in Front tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Front API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one front subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (Front non-2xx, transport failure) are exit 1. Errors render to stderr — as
// JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "FRONT_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. me is top-level (a debug /
// identity read); everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "front",
		Short:         "Front shared-inbox built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	conversation := newGroupCmd("conversation", "Read and triage conversations")
	conversation.AddCommand(
		s.newConversationListCmd(token),
		s.newConversationGetCmd(token),
		s.newConversationUpdateCmd(token),
		s.newConversationMessagesCmd(token),
		s.newConversationCommentsCmd(token),
	)
	message := newGroupCmd("message", "Send messages")
	message.AddCommand(s.newMessageSendCmd(token))
	draft := newGroupCmd("draft", "Create drafts")
	draft.AddCommand(s.newDraftCreateCmd(token))
	comment := newGroupCmd("comment", "Add internal comments")
	comment.AddCommand(s.newCommentAddCmd(token))
	contact := newGroupCmd("contact", "Look up and create contacts")
	contact.AddCommand(
		s.newContactListCmd(token),
		s.newContactGetCmd(token),
		s.newContactCreateCmd(token),
	)
	inbox := newGroupCmd("inbox", "Discover inboxes")
	inbox.AddCommand(s.newInboxListCmd(token))
	teammate := newGroupCmd("teammate", "List teammates")
	teammate.AddCommand(s.newTeammateListCmd(token))
	tag := newGroupCmd("tag", "List tags")
	tag.AddCommand(s.newTagListCmd(token))

	root.AddCommand(
		conversation, message, draft, comment, contact, inbox, teammate, tag,
		s.newMeCmd(token),
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
