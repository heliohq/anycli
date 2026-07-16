// Package microsoftoutlook is the built-in Microsoft Outlook service: a
// non-interactive cobra tree projecting the Microsoft Graph v1.0 mail resource
// namespaces (/me, /me/messages, /me/mailFolders) plus the synthetic verbs
// reply / forward (createReply/createForward + send) and batched move / mark
// (design 308 §Outlook). Search flags pass Graph $search / $filter (OData)
// through verbatim. A 401/403 very often means the token lacks a Mail scope the
// user never granted — those errors carry an explicit reconnect hint.
//
// The anycli tool id is microsoft-outlook (dash form); the Go package name
// microsoftoutlook has no dash. The RegisterService key must equal the id.
package microsoftoutlook

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
// (definitions/tools/microsoft-outlook.json).
const EnvAccessToken = "MICROSOFT_OUTLOOK_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a Mail.ReadWrite / Mail.Send scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Microsoft Outlook tool. It satisfies
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

// Execute runs one microsoft-outlook subcommand with the resolved credentials
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
		Use:           "microsoft-outlook",
		Short:         "Microsoft Outlook built-in service",
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
		s.newMessagesMoveCmd(token),
		s.newMessagesMarkCmd(token),
		s.newMessagesSendCmd(token),
		s.newMessagesReplyCmd(token),
		s.newMessagesForwardCmd(token),
	)

	folders := newGroupCmd("folders", "Mail folders")
	folders.AddCommand(s.newFoldersListCmd(token))

	drafts := newGroupCmd("drafts", "Drafts (human-in-the-loop soft guardrail before sending)")
	drafts.AddCommand(
		s.newDraftsCreateCmd(token),
		s.newDraftsListCmd(token),
		s.newDraftsGetCmd(token),
		s.newDraftsUpdateCmd(token),
		s.newDraftsSendCmd(token),
		s.newDraftsDeleteCmd(token),
	)

	root.AddCommand(s.newProfileCmd(token), messages, folders, drafts)
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
