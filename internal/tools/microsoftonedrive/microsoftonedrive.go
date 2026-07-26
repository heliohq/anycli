// Package microsoftonedrive is the built-in Microsoft OneDrive service: a
// non-interactive cobra tree projecting the Microsoft Graph v1.0 /me/drive
// resources (items browse/get, search, download, upload, mkdir, move, rename,
// create-link, delete) plus a small set of safe synthetic verbs (design 308
// §OneDrive). Path addressing uses Graph's /me/drive/root:/path form; the
// search query passes through verbatim. A 401/403 very often means the token
// lacks a scope the user never granted — those errors carry an explicit
// reconnect hint.
//
// The anycli tool id is microsoft-onedrive (dash form); the Go package name
// cannot carry a dash, so the two intentionally differ.
package microsoftonedrive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Microsoft Graph v1.0 base.
const DefaultBaseURL = "https://graph.microsoft.com/v1.0"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/microsoft-onedrive.json).
const EnvAccessToken = "MICROSOFT_ONEDRIVE_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Microsoft OneDrive tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the Graph API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// sleep overrides the retry backoff sleeper; nil = time.Sleep. Tests
	// inject a recorder to keep retries deterministic and fast.
	sleep func(time.Duration)
}

// Execute runs one microsoft-onedrive subcommand with the resolved credentials
// in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), EnvAccessToken+" is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
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

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:   "microsoft-onedrive",
		Short: "Microsoft OneDrive built-in service",
		Long: "Projects the connected account's OWN OneDrive. Shared libraries,\n" +
			"SharePoint sites and other people's drives are out of reach — a team\n" +
			"library path will not resolve, and no command switches drive. Say that\n" +
			"rather than guessing a path when the user points at a shared location.\n" +
			"\n" +
			"An item is addressed either by its opaque Graph item id (the positional\n" +
			"argument) or by a root-relative `--path` such as\n" +
			"/Documents/report.docx; the two are mutually exclusive, and ids come from\n" +
			"`items list` or `search`. Watch which form each flag wants: `items move\n" +
			"--to` and `items mkdir --parent` take a folder ID, while `upload --to`\n" +
			"takes a folder PATH.\n" +
			"\n" +
			"Paging is a whole URL, not a number: a list prints the Graph\n" +
			"`@odata.nextLink` and `--page` takes that entire URL back. `search` has no\n" +
			"page flag at all — it returns a single page, so raise its `--max`.\n" +
			"\n" +
			"`--json` returns the raw Graph DriveItem (webUrl, parentReference, hashes);\n" +
			"the default output is a trimmed one-line-per-item summary.\n" +
			"\n" +
			"Writes land immediately. `items delete` moves items to the recycle bin and\n" +
			"nothing here restores them or purges it — recovery is a manual step in the\n" +
			"OneDrive web UI, and there is no permanent-delete verb.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	items := newGroupCmd("items", "Drive items (browse / metadata / organize / share / delete)")
	items.AddCommand(
		s.newItemsListCmd(token),
		s.newItemsGetCmd(token),
		s.newItemsMkdirCmd(token),
		s.newItemsMoveCmd(token),
		s.newItemsRenameCmd(token),
		s.newItemsShareCmd(token),
		s.newItemsDeleteCmd(token),
	)

	root.AddCommand(
		items,
		s.newSearchCmd(token),
		s.newDownloadCmd(token),
		s.newUploadCmd(token),
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: bare
// group shows help, unknown subcommand fails.
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

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token
// is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
