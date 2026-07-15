// Package gmail is the built-in Gmail service: a non-interactive cobra tree
// projecting the Gmail API v1 users.* resource namespaces (profile / messages
// / threads / drafts / labels) plus the synthetic reply / forward verbs
// (design 303). Search flags pass the native Gmail query syntax through
// verbatim. A 401/403 very often means the token lacks a scope the user never
// granted — those errors carry an explicit reconnect hint.
package gmail

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

// DefaultBaseURL is the production Gmail API base.
const DefaultBaseURL = "https://gmail.googleapis.com/gmail/v1"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/gmail.json).
const EnvAccessToken = "GMAIL_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Gmail tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Gmail API base; empty = DefaultBaseURL. Tests
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

// Execute runs one gmail subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "GMAIL_ACCESS_TOKEN is not set")
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
		Use:           "gmail",
		Short:         "Gmail built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	messages := newGroupCmd("messages", "Messages")
	messages.AddCommand(
		s.newMessagesListCmd(token),
		s.newMessagesGetCmd(token),
		s.newMessagesAttachmentsCmd(token),
		s.newMessagesSendCmd(token),
		s.newMessagesReplyCmd(token),
		s.newMessagesForwardCmd(token),
		s.newMessagesModifyCmd(token),
		s.newMessagesTrashCmd(token, false),
		s.newMessagesTrashCmd(token, true),
	)

	threads := newGroupCmd("threads", "Threads")
	threads.AddCommand(s.newThreadsListCmd(token), s.newThreadsGetCmd(token))

	drafts := newGroupCmd("drafts", "Drafts")
	drafts.AddCommand(
		s.newDraftsCreateCmd(token),
		s.newDraftsListCmd(token),
		s.newDraftsGetCmd(token),
		s.newDraftsUpdateCmd(token),
		s.newDraftsSendCmd(token),
		s.newDraftsDeleteCmd(token),
	)

	labels := newGroupCmd("labels", "Labels (`labels get INBOX` returns unread/total counters in one call)")
	labels.AddCommand(s.newLabelsListCmd(token), s.newLabelsGetCmd(token), s.newLabelsCreateCmd(token))

	root.AddCommand(s.newProfileCmd(token), messages, threads, drafts, labels)
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
