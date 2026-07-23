// Package buffer is the built-in Buffer service: a non-interactive cobra tree
// over the Buffer GraphQL API (https://api.buffer.com). Every operation is a
// single POST of a GraphQL document + typed variables with an
// "Authorization: Bearer <token>" header; the account/organization/channel/
// post/idea data model maps to agent-facing resource verbs. Buffer signals
// request-level failures with an HTTP non-2xx or a top-level GraphQL `errors`
// array, and mutation-level failures with a `MutationError` arm on the payload
// union — both are surfaced. Verified against developers.buffer.com
// (2026-07-22); see the tool/buffer DESIGN.md §8.
package buffer

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

// DefaultBaseURL is the production Buffer GraphQL endpoint. The whole API is a
// single POST to this base URL (no `/graphql` path segment).
const DefaultBaseURL = "https://api.buffer.com"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/buffer.json). It carries either an OAuth 2.0 access token
// or a personal/static API key — both are sent as "Authorization: Bearer".
const EnvAccessToken = "BUFFER_ACCESS_TOKEN"

// Service implements the built-in Buffer tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Buffer GraphQL base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one buffer subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (Buffer non-2xx, GraphQL/mutation error, transport
// failure) are exit 1. Errors render to stderr — as JSON under --json, plain
// text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "BUFFER_ACCESS_TOKEN is not set"})
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

// newRoot builds the resource-grouped cobra tree: account / org / channel /
// post / idea. Groups are runnable so an unknown subcommand fails instead of
// silently printing help with exit 0 (a false success for an agent).
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "buffer",
		Short:         "Buffer built-in service (social publishing via the Buffer GraphQL API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	account := newGroupCmd("account", "Read the authenticated account")
	account.AddCommand(s.newAccountGetCmd(token))

	org := newGroupCmd("org", "List organizations (workspaces)")
	org.AddCommand(s.newOrgListCmd(token))

	channel := newGroupCmd("channel", "List connected channels")
	channel.AddCommand(s.newChannelListCmd(token))

	post := newGroupCmd("post", "Manage posts")
	post.AddCommand(
		s.newPostListCmd(token),
		s.newPostCreateCmd(token),
		s.newPostEditCmd(token),
		s.newPostDeleteCmd(token),
	)

	idea := newGroupCmd("idea", "Manage ideas")
	idea.AddCommand(s.newIdeaCreateCmd(token))

	root.AddCommand(account, org, channel, post, idea)
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
