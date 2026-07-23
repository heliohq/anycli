// Package mastodon is the built-in Mastodon service: a provider-neutral cobra
// tree over the Mastodon REST API (docs.joinmastodon.org). Mastodon is
// federated — every server hosts its own API and its own OAuth authorization
// server — so there is no single API host: the target instance is per-account
// and travels with the credential.
//
// Because Mastodon OAuth apps register per-instance (proprietary
// POST /api/v1/apps, no shared client possible), the connection credential is
// the account owner's self-serve Application access token (Settings →
// Development), a long-lived bearer token that does not expire. The token is
// instance-scoped, so the injected credential carries BOTH the instance base
// URL and the token, joined by a single space:
//
//	MASTODON_ACCESS_TOKEN = "https://mastodon.social <access-token>"
//
// The service splits on the first space: the left half is the instance base
// URL (every request is <instance>/api/v1/...), the right half is the bearer
// token (Authorization: Bearer). A URL never contains a space and Mastodon
// tokens are URL-safe base64, so the split is unambiguous.
package mastodon

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

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/mastodon.json). Its value is the instance base URL and
// the access token joined by a single space (see the package doc).
const EnvAccessToken = "MASTODON_ACCESS_TOKEN"

// Service implements the built-in Mastodon tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the instance base URL derived from the credential;
	// empty = derive from the credential. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one mastodon subcommand with the resolved credential in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (Mastodon non-2xx, transport failure) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	instanceURL, token, err := splitCredential(env[EnvAccessToken])
	if err != nil {
		s.renderError(hasJSONArg(args), &usageError{msg: err.Error()})
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(instanceURL, token)
	root.SetArgs(args)
	runErr := root.ExecuteContext(ctx)
	if runErr == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, runErr)

	var apiErr *apiError
	if errors.As(runErr, &apiErr) {
		return execution.Failure(runErr), nil
	}
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// splitCredential splits the injected credential into the instance base URL
// and the bearer token. The two are joined by a single space; the URL is
// normalized to scheme+host with no trailing slash. Both halves are required.
func splitCredential(raw string) (instanceURL, token string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("%s is not set", EnvAccessToken)
	}
	left, right, ok := strings.Cut(raw, " ")
	if !ok {
		return "", "", fmt.Errorf(
			"%s must be the instance URL and access token joined by a space (for example \"https://mastodon.social <token>\")",
			EnvAccessToken)
	}
	instanceURL = normalizeInstanceURL(left)
	token = strings.TrimSpace(right)
	if instanceURL == "" {
		return "", "", fmt.Errorf("%s: instance URL is empty or invalid", EnvAccessToken)
	}
	if token == "" {
		return "", "", fmt.Errorf("%s: access token is empty", EnvAccessToken)
	}
	return instanceURL, token, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (the pre-parse
// credential check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"code":…,"message":…,"status":<HTTP, omitted when 0>,"provider_error":…}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	inner := map[string]any{"code": "usage_error", "message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		inner["code"] = "api_error"
		if apiErr.status != 0 {
			inner["status"] = apiErr.status
		}
		if apiErr.providerError != "" {
			inner["provider_error"] = apiErr.providerError
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": inner})
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

// newRoot builds the grouped-by-resource cobra tree. whoami / search / api are
// top-level; posting, timelines, accounts, and notifications hang under groups
// or read directly.
func (s *Service) newRoot(instanceURL, token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "mastodon",
		Short:         "Mastodon built-in service (post, read timelines, search, engage)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	rt := &runContext{svc: s, instanceURL: instanceURL, token: token}

	post := newGroupCmd("post", "Create, delete, and read statuses")
	post.AddCommand(
		rt.newPostCreateCmd(),
		rt.newPostDeleteCmd(),
		rt.newPostGetCmd(),
	)
	timeline := newGroupCmd("timeline", "Read timelines")
	timeline.AddCommand(
		rt.newTimelineHomeCmd(),
		rt.newTimelinePublicCmd(),
		rt.newTimelineTagCmd(),
	)
	account := newGroupCmd("account", "Look up accounts and their posts")
	account.AddCommand(
		rt.newAccountGetCmd(),
		rt.newAccountPostsCmd(),
	)
	notifications := newGroupCmd("notifications", "Read notifications")
	notifications.AddCommand(
		rt.newNotificationsListCmd(),
	)

	root.AddCommand(
		rt.newWhoamiCmd(),
		rt.newSearchCmd(),
		rt.newFavouriteCmd(),
		rt.newBoostCmd(),
		rt.newFollowCmd(),
		rt.newUnfollowCmd(),
		rt.newAPICmd(),
		post, timeline, account, notifications,
	)
	return root
}

// runContext carries the resolved credential and the service pointer into
// every command's RunE closure.
type runContext struct {
	svc         *Service
	instanceURL string
	token       string
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// NewCommandTree returns the full command tree built with an empty credential
// for dry-run parsing and traversal (tools.Service seam, design 318). The
// credential is only captured by RunE closures, which are never run on this
// tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("", "") }
