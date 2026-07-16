// Package tasks is the built-in Google Tasks service: a non-interactive cobra
// tree projecting the Tasks API v1 resource namespaces — tasklists (`lists
// ...`) and tasks (top-level verbs) — plus the synthetic complete / reopen
// verbs (design 303 §Google Tasks). The API has no query language, so there is
// no text-search flag: the filter surface is exactly the date-window and
// boolean toggles the API exposes. A 401/403 very often means the token lacks a
// scope the user never granted — those errors carry an explicit reconnect hint.
package tasks

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

// DefaultBaseURL is the production Tasks API base.
const DefaultBaseURL = "https://tasks.googleapis.com/tasks/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/tasks.json).
const EnvAccessToken = "GOOGLE_TASKS_ACCESS_TOKEN"

// defaultList is the Tasks API alias for the user's primary task list. "Jot me
// a todo" lands here with zero follow-up questions.
const defaultList = "@default"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Google Tasks tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Tasks API base; empty = DefaultBaseURL. Tests
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

// Execute runs one tasks subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "GOOGLE_TASKS_ACCESS_TOKEN is not set")
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
		Use:           "tasks",
		Short:         "Google Tasks built-in service (personal task lists)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	lists := newGroupCmd("lists", "Task lists (tasklists resource)")
	lists.AddCommand(
		s.newListsListCmd(token),
		s.newListsGetCmd(token),
		s.newListsCreateCmd(token),
		s.newListsUpdateCmd(token),
		s.newListsDeleteCmd(token),
	)

	root.AddCommand(
		lists,
		s.newTasksListCmd(token),
		s.newTasksGetCmd(token),
		s.newTasksCreateCmd(token),
		s.newTasksUpdateCmd(token),
		s.newTasksStatusCmd(token, statusCompleted),
		s.newTasksStatusCmd(token, statusNeedsAction),
		s.newTasksMoveCmd(token),
		s.newTasksClearCmd(token),
		s.newTasksDeleteCmd(token),
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
