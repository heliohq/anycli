// Package reddit implements the built-in Reddit service over the Reddit Data
// API (https://oauth.reddit.com). It accepts an OAuth 2.0 user access token and
// exposes a non-interactive Cobra tree. Every request carries a descriptive,
// unique User-Agent (Reddit blocks/throttles generic agents) and raw_json=1 so
// entity text comes back unescaped. Read listings are stripped of Reddit's
// kind/data envelopes into flat, provider-neutral items; form-post endpoints
// use api_type=json and are checked for the HTTP-200-with-errors dialect.
package reddit

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

const (
	// DefaultAPIBase is the production Reddit OAuth API base. Token endpoints
	// live on www.reddit.com; all authenticated calls go to oauth.reddit.com.
	DefaultAPIBase = "https://oauth.reddit.com"

	// EnvAccessToken is populated by the credential binding in
	// definitions/tools/reddit.json.
	EnvAccessToken = "REDDIT_ACCESS_TOKEN"

	// userAgent is the mandatory Reddit User-Agent. Format is
	// <platform>:<app-id>:<version> (by /u/<username>). The account handle is
	// filled from lane 1's registered Reddit app at rollout; the constant shape
	// is what Reddit requires (a generic/default UA is throttled or blocked).
	userAgent = "helio:im.helio.heliox-reddit:v1.0 (by /u/helio-assistant)"
)

// Service implements the built-in Reddit tool. Empty fields select production
// defaults; tests inject an HTTP server and output buffers. It satisfies
// tools.Service by duck typing (this package never imports the registry).
type Service struct {
	// APIBase overrides the Reddit API base; empty = DefaultAPIBase.
	APIBase string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one reddit subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, bad enums, missing required
// flags, unknown subcommands) are exit 2; runtime/API errors (Reddit non-2xx,
// transport failure, the api_type=json error dialect) are exit 1. Errors render
// to stderr — as a JSON envelope under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "REDDIT_ACCESS_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "reddit",
		Short:         "Reddit built-in service (OAuth 2.0 user token)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "single-result JSON; listings emit JSONL plus a final {\"after\":…} cursor")

	root.AddCommand(
		s.newMeCmd(token),
		s.newSubredditCmd(token),
		s.newSearchCmd(token),
		s.newPostCmd(token),
		s.newCommentCmd(token),
		s.newUserCmd(token),
		s.newInboxCmd(token),
		s.newMessageCmd(token),
		s.newSubsCmd(token),
	)
	return root
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags.
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

// jsonMode reads the root --json persistent flag from within a subcommand.
func jsonMode(cmd *cobra.Command) bool {
	v, _ := cmd.Root().PersistentFlags().GetBool("json")
	return v
}

// newGroup is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
func newGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

func (s *Service) apiBase() string {
	if s.APIBase != "" {
		return strings.TrimRight(s.APIBase, "/")
	}
	return DefaultAPIBase
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
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
