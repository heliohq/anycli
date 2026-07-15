// Package notion is the built-in Notion service: a markdown-native,
// MCP-aligned cobra tree over the api.notion.com REST surface (design 304).
// Content is read and written as markdown via the official page-markdown
// endpoints; parameters mirror the official Notion MCP so pretrained models
// transfer their tool-call intuition. Notion fails with a non-2xx status and a
// JSON body carrying code/message — every call surfaces both.
package notion

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

// DefaultBaseURL is the production Notion API base.
const DefaultBaseURL = "https://api.notion.com/v1"

// notionVersion is the default Notion-Version header; commands override it per
// call via callWithVersion (markdown + 2026-03-11 data-model endpoints).
const notionVersion = "2022-06-28"

// markdownVersion is the Notion-Version required by the markdown endpoints and
// the 2026-03-11 data model (page markdown, search object filter, views).
const markdownVersion = "2026-03-11"

// EnvToken is the env var the credential binding injects (definitions/tools/notion.json).
const EnvToken = "NOTION_TOKEN"

// Service implements the built-in Notion tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Notion API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one notion subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad enums,
// invalid JSON, missing required flags, unknown subcommands) are exit 2;
// runtime/API errors (Notion non-2xx, transport failure, poll timeout) are
// exit 1. Errors render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract (§error).
		s.renderError(hasJSONArg(args), &usageError{msg: "NOTION_TOKEN is not set"})
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

// newRoot builds the grouped-by-resource cobra tree. search / fetch are
// top-level (cross-resource); everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "notion",
		Short:         "Notion built-in service (markdown-native, MCP-aligned)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	// Global (persistent) flags — visible to every subcommand. Pagination
	// flags are NOT global; they register locally on search/db query/comment
	// list only (see registerPaginationFlags).
	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output")
	pf.String("file", "", "read single-segment markdown content from a file")
	pf.Bool("allow-async", false, "allow async processing with automatic task polling")
	pf.Bool("allow-deleting-content", false, "allow content updates that delete existing child blocks")

	// Resource groups. Slice 1 registers them as empty runnable groups plus the
	// top-level fetch/search; later slices attach each group's subcommands
	// (page create/update/…, db create/query, view create/update, …).
	page := newGroupCmd("page", "Manage pages")
	page.AddCommand(
		s.newPageCreateCmd(token),
		s.newPageUpdateCmd(token),
		s.newPageReplaceCmd(token),
		s.newPageEditCmd(token),
		s.newPageInsertCmd(token),
		s.newPageAppendCmd(token),
		s.newPageMoveCmd(token),
		s.newPageDuplicateCmd(token),
	)
	db := newGroupCmd("db", "Manage databases")
	db.AddCommand(
		s.newDBCreateCmd(token),
		s.newDBQueryCmd(token),
	)
	dataSource := newGroupCmd("data-source", "Manage data sources")
	dataSource.AddCommand(
		s.newDataSourceUpdateCmd(token),
	)
	view := newGroupCmd("view", "Manage views")
	view.AddCommand(
		s.newViewCreateCmd(token),
		s.newViewUpdateCmd(token),
	)
	comment := newGroupCmd("comment", "Manage comments")
	comment.AddCommand(
		s.newCommentCreateCmd(token),
		s.newCommentListCmd(token),
	)
	user := newGroupCmd("user", "Look up users")
	user.AddCommand(s.newUserGetCmd(token))
	task := newGroupCmd("task", "Async task status")
	task.AddCommand(s.newTaskGetCmd(token))

	root.AddCommand(
		s.newFetchCmd(token),
		s.newSearchCmd(token),
		page, db, dataSource, view, comment, user, task,
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
