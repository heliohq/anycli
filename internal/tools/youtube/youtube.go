// Package youtube is the built-in YouTube service: a non-interactive cobra
// tree over the YouTube Data API v3 (https://www.googleapis.com/youtube/v3).
// It projects the resource namespaces an AI teammate uses — channels, search,
// videos, playlists, playlist-items, comments, subscriptions — with a single
// broad OAuth scope (youtube.force-ssl) behind the connection.
//
// The Data API takes a mandatory `part` parameter on every read (which
// resource sections to hydrate: snippet / statistics / contentDetails /
// status / replies …); the service sends a sensible default per verb and
// passes `--part` through verbatim. A 401/403 usually means the token lacks a
// scope the user never granted — those errors carry an explicit reconnect
// hint. Exit codes: 0 success, 1 runtime/API error (typed apiError), 2
// usage/parse error.
package youtube

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

// DefaultBaseURL is the production YouTube Data API v3 base.
const DefaultBaseURL = "https://www.googleapis.com/youtube/v3"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/youtube.json).
const EnvAccessToken = "YOUTUBE_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaves: "false" for side-effect-free reads (GET/list/get/search),
// "true" for provider-state mutations (create/update/delete/rate/add/reply/
// moderate).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in YouTube tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API base; empty = DefaultBaseURL. Tests point it
	// at an httptest server.
	BaseURL string
	// UploadBaseURL overrides the resumable-upload base; empty =
	// DefaultUploadBaseURL. Tests point it at an httptest server.
	UploadBaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one youtube subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// missing required flags, unknown subcommands) are exit 2; runtime/API errors
// (YouTube non-2xx, transport failure) are exit 1. Errors render to stderr —
// as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "YOUTUBE_ACCESS_TOKEN is not set"})
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

func (s *Service) base() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) uploadBase() string {
	if s.UploadBaseURL != "" {
		return strings.TrimRight(s.UploadBaseURL, "/")
	}
	return DefaultUploadBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

// newRoot builds the grouped-by-resource cobra tree. Every resource group is a
// runnable command (bare group shows help, unknown subcommand fails).
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "youtube",
		Short:         "YouTube built-in service (Data API v3)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	channels := newGroupCmd("channels", "Channel info and statistics")
	channels.AddCommand(s.newChannelsGetCmd(token))

	videos := newGroupCmd("videos", "Video metadata, stats, ratings and your uploads")
	videos.AddCommand(
		s.newVideosGetCmd(token),
		s.newVideosMineCmd(token),
		s.newVideosUploadCmd(token),
		s.newVideosUpdateCmd(token),
		s.newVideosRateCmd(token),
	)

	playlists := newGroupCmd("playlists", "Playlists (list / create / update / delete)")
	playlists.AddCommand(
		s.newPlaylistsListCmd(token),
		s.newPlaylistsCreateCmd(token),
		s.newPlaylistsUpdateCmd(token),
		s.newPlaylistsDeleteCmd(token),
	)

	playlistItems := newGroupCmd("playlist-items", "Playlist membership (list / add / remove)")
	playlistItems.AddCommand(
		s.newPlaylistItemsListCmd(token),
		s.newPlaylistItemsAddCmd(token),
		s.newPlaylistItemsRemoveCmd(token),
	)

	comments := newGroupCmd("comments", "Comment threads and moderation")
	comments.AddCommand(
		s.newCommentsListCmd(token),
		s.newCommentsRepliesCmd(token),
		s.newCommentsReplyCmd(token),
		s.newCommentsUpdateCmd(token),
		s.newCommentsDeleteCmd(token),
		s.newCommentsModerateCmd(token),
	)

	subscriptions := newGroupCmd("subscriptions", "Channels the account subscribes to")
	subscriptions.AddCommand(s.newSubscriptionsListCmd(token))

	root.AddCommand(
		s.newSearchCmd(token),
		channels, videos, playlists, playlistItems, comments, subscriptions,
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

// jsonOut reports whether the persistent --json flag was set for cmd.
func jsonOut(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}
