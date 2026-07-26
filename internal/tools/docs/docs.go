// Package docs is the built-in Google Docs service: a non-interactive cobra
// tree projecting the Docs API v1 `documents` resource (the only resource, with
// three methods — get / create / batchUpdate) plus two safe synthetic verbs
// (append / replace-all). Markdown is the read/write lingua franca: reads render
// the deeply nested document JSON to markdown, writes translate a markdown
// subset into batchUpdate requests so the caller never touches the UTF-16 index
// arithmetic (design 303 §Google Docs). A 401 or a scope-insufficient 403
// carries a reconnect hint; a 404 or a permission 403 carries a sharing hint.
package docs

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

// DefaultBaseURL is the production Docs API base.
const DefaultBaseURL = "https://docs.googleapis.com/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/docs.json).
const EnvAccessToken = "DOCS_ACCESS_TOKEN"

// Service implements the built-in Google Docs tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Docs API base; empty = DefaultBaseURL. Tests
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

// Execute runs one docs subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "DOCS_ACCESS_TOKEN is not set")
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
		Use:   "docs",
		Short: "Google Docs built-in service",
		Long: "Documents are addressed by a pasted link or a bare id — every command\n" +
			"accepts a full docs.google.com document URL and extracts the id from it.\n" +
			"There is no search and no list command, because the Docs API has no list\n" +
			"method: when the user means \"that doc\", their link is the only way in.\n" +
			"\n" +
			"Indices are never the caller's problem. Reads render the structured\n" +
			"document to markdown, and `documents create --body-file` and `documents\n" +
			"append --body-file` translate a markdown subset into edit requests.\n" +
			"`documents batch-update` is the one place where raw Docs API requests, and\n" +
			"their index arithmetic, become the caller's responsibility.\n" +
			"\n" +
			"The WRITE subset covers headings, paragraphs, bold, italic, strikethrough,\n" +
			"inline code, links and ordered/unordered lists. Tables and images are not\n" +
			"written: their markup is inserted as literal text and a warning goes to\n" +
			"stderr. List nesting is flattened, so every bullet lands at the top level.\n" +
			"Reads render tables and nested lists correctly — it is only the write\n" +
			"direction that is a subset.\n" +
			"\n" +
			"`create` and `append` only add content. `replace-all` and `batch-update`\n" +
			"overwrite, and neither can be undone through the API, so read the document\n" +
			"first and keep the count `replace-all` reports.\n" +
			"\n" +
			"A 404, or a 403 with no scope hint, nearly always means the document is\n" +
			"not shared with the connected account rather than that it does not exist —\n" +
			"usually because the link came from a different Google account.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output the raw API JSON response instead of the human-readable rendering")

	documents := newGroupCmd("documents", "Documents (the only Docs API resource: get / create / batch-update)")
	documents.AddCommand(
		s.newDocumentsGetCmd(token),
		s.newDocumentsCreateCmd(token),
		s.newDocumentsAppendCmd(token),
		s.newDocumentsReplaceAllCmd(token),
		s.newDocumentsBatchUpdateCmd(token),
	)

	root.AddCommand(documents)
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
