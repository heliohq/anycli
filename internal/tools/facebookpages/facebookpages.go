// Package facebookpages implements the built-in Facebook Pages service over
// the Facebook Graph API. It accepts a long-lived Facebook *user* OAuth token
// and exposes a non-interactive Cobra tree grouped by resource (pages / page /
// post / comment / insights).
//
// The defining shape is the Page access-token two-hop, absorbed entirely inside
// this service: the connection stores the user token, but Page-scoped actions —
// above all publishing — require a per-Page access token. For every command
// that targets a Page (all except `pages list`) the service first resolves that
// Page's token with the stored user token, then performs the actual call with
// the Page token. The Page token is an internal, single-request value: it is
// never printed to stdout, never emitted under --json, and never logged.
package facebookpages

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

const (
	// graphVersion is the single pinned Graph API version. Meta deprecates
	// versions on a ~2-year clock, so this is one maintained constant, never
	// defaulted per request. Keep it aligned with the sibling meta-ads /
	// instagram services.
	graphVersion = "v23.0"

	// DefaultBaseURL is the production Graph API base for the pinned version.
	DefaultBaseURL = "https://graph.facebook.com/" + graphVersion

	// EnvAccessToken is the env var the credential binding injects
	// (definitions/tools/facebook-pages.json): the long-lived Facebook USER
	// access token. Page tokens are derived from it at runtime.
	EnvAccessToken = "FACEBOOK_ACCESS_TOKEN"
)

// Service implements the built-in Facebook Pages tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle). Empty fields select production defaults; tests inject an HTTP
// server and output buffers.
type Service struct {
	// BaseURL overrides the Graph API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one facebook-pages subcommand with the resolved credentials in
// env. Success is exit 0; usage/param errors (missing required flags, unknown
// subcommands, bad enums) are exit 2; runtime/API errors (Graph non-2xx,
// transport failure) are exit 1. Errors render to stderr — as a JSON envelope
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAccessToken + " is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. `pages` is the only group
// that operates on the user token directly (discovery); every other group takes
// a required --page flag and rides the Page-token two-hop.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "facebook-pages",
		Short:         "Facebook Pages built-in service (Graph API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	pages := newGroupCmd("pages", "Discover the Pages this user manages")
	pages.AddCommand(s.newPagesListCmd(token))

	page := newGroupCmd("page", "Read a Page's profile")
	page.AddCommand(s.newPageGetCmd(token))

	post := newGroupCmd("post", "Manage Page posts")
	post.AddCommand(
		s.newPostListCmd(token),
		s.newPostGetCmd(token),
		s.newPostCreateCmd(token),
		s.newPostUpdateCmd(token),
		s.newPostDeleteCmd(token),
	)

	comment := newGroupCmd("comment", "Read and moderate comments")
	comment.AddCommand(
		s.newCommentListCmd(token),
		s.newCommentReplyCmd(token),
		s.newCommentHideCmd(token),
		s.newCommentDeleteCmd(token),
	)

	root.AddCommand(pages, page, post, comment, s.newInsightsCmd(token))
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

func (s *Service) apiBase() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// pageFlag registers the required --page flag shared by every Page-scoped
// command and returns a pointer to its bound value.
func pageFlag(cmd *cobra.Command) *string {
	pageID := new(string)
	cmd.Flags().StringVar(pageID, "page", "", "Page id to operate on (run `pages list` to discover)")
	_ = cmd.MarkFlagRequired("page")
	return pageID
}
