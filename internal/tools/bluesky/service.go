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

// readOnly and writeAction carry the design-318 side-effect annotation for
// runnable leaf commands: reads (XRPC *.get*/*.search*/*.list* queries) are
// side-effect-free; writes create/delete records or mutate state.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
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
		Use:   "bluesky",
		Short: "Bluesky built-in service",
		Long: "Three identifiers flow through this tool and none of them can be\n" +
			"invented. A handle (alice.bsky.social) is human-readable and can change;\n" +
			"a DID (did:plc:...) is the stable account key, and --actor takes either.\n" +
			"A specific post or record is an at:// URI —\n" +
			"at://<did>/<collection>/<rkey> — paired with a `cid` content hash.\n" +
			"Writes echo the uri and cid they produced; keep them, because replying\n" +
			"to, quoting, liking or deleting that item later needs them and nothing\n" +
			"reconstructs them from the post's text.\n" +
			"\n" +
			"Reads return ONE page. Feed the returned `cursor` back as --cursor for\n" +
			"the next one; --limit is 1-100 with a default of 25 wherever it appears.\n" +
			"\n" +
			"For \"what engagement did the connected account get\",\n" +
			"`notifications list` is the standing answer — mentions, replies, likes,\n" +
			"reposts and follows arrive in one call — rather than repeated searches.\n" +
			"\n" +
			"Reads are reshaped, not passed through: a post comes back as uri, cid,\n" +
			"author, text, created_at and the three engagement counts, so embeds,\n" +
			"facets, labels and viewer state are simply not visible through this\n" +
			"tool.\n" +
			"\n" +
			"Writes are public the instant they succeed. There is no draft or\n" +
			"scheduling state, and the only undo is deleting the record that was\n" +
			"created.",
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
