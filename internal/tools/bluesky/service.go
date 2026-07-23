// Package bluesky implements the built-in Bluesky service over the AT Protocol
// XRPC surface. It accepts app-password credentials (identifier + app
// password), opens a session with com.atproto.server.createSession, and
// exposes a non-interactive Cobra tree that posts, reads, searches, and
// engages on the user's behalf. Every request runs against the entryway/PDS
// host (bsky.social by default, or an account-specific pds_host override) with
// a plain Bearer access token — the app-password path does not use DPoP.
package bluesky

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

const (
	// DefaultHost is the Bluesky entryway/PDS. The PDS proxies app.bsky.*
	// AppView reads, so one host and one bearer token serves both the repo
	// plane and the AppView. v1 targets bsky.social only; self-hosted-PDS
	// override is a follow-up.
	DefaultHost = "https://bsky.social"

	// EnvCredentials is populated by the credential binding in
	// definitions/tools/bluesky.json. It carries the combined app-password
	// credential as "<identifier>:<app-password>" — Helio stores a single
	// secret through the manual-credentials plane (see DESIGN.md §0a), and the
	// service splits it on the first colon. Neither a handle/email identifier
	// nor an xxxx-xxxx-xxxx-xxxx app password contains a colon, so the split is
	// unambiguous.
	EnvCredentials = "BLUESKY_CREDENTIALS"
)

// Service implements the built-in Bluesky tool. Empty fields select production
// defaults; tests inject an HTTP server (via APIBase) and output buffers.
type Service struct {
	// APIBase, when set, overrides both the default host and any pds_host env
	// value — tests point it at an httptest server.
	APIBase string
	HC      *http.Client
	Out     io.Writer
	Err     io.Writer
}

// Execute runs one Bluesky subcommand. Credentials are resolved by the host and
// delivered as a single combined environment variable; the service splits it
// and opens its own session.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	identifier, appPassword, ok := splitCredentials(env[EnvCredentials])
	if !ok {
		fmt.Fprintln(s.stderr(), "BLUESKY_CREDENTIALS must be set as \"<identifier>:<app-password>\"")
		return execution.Result{ExitCode: 1}, nil
	}

	sess := &session{
		svc:        s,
		host:       s.host(),
		identifier: identifier,
		password:   appPassword,
	}

	root := s.newRoot(sess)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

func (s *Service) newRoot(sess *session) *cobra.Command {
	root := &cobra.Command{
		Use:           "bluesky",
		Short:         "Bluesky built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "single-result JSON; multi-result commands may emit JSONL")

	root.AddCommand(
		s.newWhoamiCmd(sess),
		s.newPostCmd(sess),
		s.newTimelineCmd(sess),
		s.newFeedCmd(sess),
		s.newSearchCmd(sess),
		s.newProfileCmd(sess),
		s.newFollowCmd(sess),
		s.newUnfollowCmd(sess),
		s.newLikeCmd(sess),
		s.newRepostCmd(sess),
		s.newNotificationsCmd(sess),
	)
	return root
}

// splitCredentials splits the combined "<identifier>:<app-password>" secret on
// the first colon. Both parts must be non-empty.
func splitCredentials(combined string) (identifier, appPassword string, ok bool) {
	identifier, appPassword, found := strings.Cut(strings.TrimSpace(combined), ":")
	identifier = strings.TrimSpace(identifier)
	appPassword = strings.TrimSpace(appPassword)
	if !found || identifier == "" || appPassword == "" {
		return "", "", false
	}
	return identifier, appPassword, true
}

// host resolves the request host: an injected APIBase (tests) overrides the
// default entryway.
func (s *Service) host() string {
	if s.APIBase != "" {
		return strings.TrimRight(s.APIBase, "/")
	}
	return DefaultHost
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
