// Package twitch is the built-in Twitch service: a non-interactive cobra tree
// over the Twitch Helix REST surface (https://api.twitch.tv/helix). Auth is the
// pair "Authorization: Bearer <user-token>" AND "Client-Id: <client_id>" — every
// Helix request carries both, so this tool injects two credentials
// (definitions/tools/twitch.json). Helix errors are non-2xx with a JSON body
// carrying {error,status,message}; a 401 rejects the credential. List verbs emit
// {"data":[...],"cursor":"<next or empty>"}; single-object verbs emit the Helix
// object unwrapped from its data[0] array.
package twitch

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

// DefaultBaseURL is the production Twitch Helix API base.
const DefaultBaseURL = "https://api.twitch.tv/helix"

// Env vars the credential bindings inject (definitions/tools/twitch.json). The
// user token expires (~4h) and is refreshed by the Helio token gateway; the
// client id is the OAuth app's public Client-Id, required on every Helix call.
const (
	EnvToken    = "TWITCH_TOKEN"
	EnvClientID = "TWITCH_CLIENT_ID"
)

// Service implements the built-in Twitch tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Helix API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer

	// selfID caches the authenticated user's id for the process (resolved lazily
	// via Get Users), so channel-scoped verbs work without the AI first looking
	// up its own broadcaster id.
	selfID string
}

// Execute runs one twitch subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flags, bad enums, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Helix
// non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	clientID := env[EnvClientID]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	if clientID == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvClientID + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token, clientID)
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
// missing-credential checks).
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

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for reads (Helix GET list/get/search),
// "true" for provider-state mutations (chat send, clip create, channel update).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// newRoot builds the grouped-by-resource cobra tree.
func (s *Service) newRoot(token, clientID string) *cobra.Command {
	root := &cobra.Command{
		Use:           "twitch",
		Short:         "Twitch built-in service (Helix API)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	rc := &reqCtx{token: token, clientID: clientID}

	user := newGroupCmd("user", "Look up users")
	user.AddCommand(s.newUserGetCmd(rc))

	channel := newGroupCmd("channel", "Read and update channel information")
	channel.AddCommand(s.newChannelGetCmd(rc), s.newChannelUpdateCmd(rc))

	stream := newGroupCmd("stream", "Live streams")
	stream.AddCommand(s.newStreamListCmd(rc), s.newStreamFollowedCmd(rc))

	search := newGroupCmd("search", "Search Twitch")
	search.AddCommand(s.newSearchChannelsCmd(rc))

	clip := newGroupCmd("clip", "Clips")
	clip.AddCommand(s.newClipListCmd(rc), s.newClipCreateCmd(rc))

	video := newGroupCmd("video", "Videos / VODs")
	video.AddCommand(s.newVideoListCmd(rc))

	follower := newGroupCmd("follower", "Channel followers")
	follower.AddCommand(s.newFollowerListCmd(rc))

	subscriber := newGroupCmd("subscriber", "Channel subscribers")
	subscriber.AddCommand(s.newSubscriberListCmd(rc))

	chat := newGroupCmd("chat", "Chat")
	chat.AddCommand(s.newChatSendCmd(rc), s.newChattersCmd(rc))

	root.AddCommand(user, channel, stream, search, clip, video, follower, subscriber, chat)
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
