// Package instagram is the built-in Instagram service: a cobra tree over the
// "Instagram API with Instagram Login" Graph surface (graph.instagram.com).
// One connection == one Instagram professional account, so /me is that account
// and no account selector is needed. Content publishing is exposed as the
// real async 3-step container flow (publish create -> status -> finish) rather
// than a single magic verb, so the assistant polls and decides.
//
// Host discipline: this service pins graph.instagram.com. Sending an
// Instagram-Login token to graph.facebook.com yields the classic
// "Cannot parse access token" — the single most common failure mode — so the
// API host is one service constant, never per-request-defaulted.
package instagram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Instagram Graph API base. graphVersion is
// pinned as a single service constant carrying Meta's ~2-year deprecation
// clock; it is never per-request-defaulted (no silent fallback).
const (
	graphVersion   = "v23.0"
	DefaultBaseURL = "https://graph.instagram.com/" + graphVersion
)

// EnvToken is the env var the credential binding injects (definitions/tools/instagram.json).
const EnvToken = "INSTAGRAM_ACCESS_TOKEN"

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for state-free reads, "true" for provider
// mutations. Group commands must not carry either.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Instagram tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Graph API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC httpDoer
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one instagram subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad enums, missing required flags,
// unknown subcommands) are exit 2; runtime/API errors (Graph non-2xx,
// transport failure) are exit 1. Errors render to stderr — as JSON under
// --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvToken + " is not set"})
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
	// error is inherently a usage error -> exit 2.
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

// newRoot builds the grouped-by-resource cobra tree. insights is top-level
// (account-level); everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "instagram",
		Short:         "Instagram professional account built-in service (Instagram API with Instagram Login)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON error output")

	account := newGroupCmd("account", "Read the connected Instagram account")
	account.AddCommand(s.newAccountGetCmd(token))

	media := newGroupCmd("media", "List and inspect media")
	media.AddCommand(
		s.newMediaListCmd(token),
		s.newMediaGetCmd(token),
		s.newMediaInsightsCmd(token),
	)

	publish := newGroupCmd("publish", "Publish media via the async container flow")
	publish.AddCommand(
		s.newPublishCreateCmd(token),
		s.newPublishStatusCmd(token),
		s.newPublishFinishCmd(token),
	)

	comment := newGroupCmd("comment", "Community management on media comments")
	comment.AddCommand(
		s.newCommentListCmd(token),
		s.newCommentReplyCmd(token),
		s.newCommentHideCmd(token),
		s.newCommentDeleteCmd(token),
	)

	root.AddCommand(account, media, publish, comment, s.newInsightsCmd(token))
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
