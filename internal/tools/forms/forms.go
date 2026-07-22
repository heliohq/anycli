// Package forms is the built-in Google Forms service: a non-interactive cobra
// tree projecting the Forms API v1 resource namespaces (forms / forms.responses)
// plus a small set of safe synthetic verbs. The editing surface is a faithful
// pass-through of the batchUpdate Request[] JSON — no second question-type DSL
// is invented (design 303 §Google Forms).
//
// create always builds an unpublished form; publish and responder sharing are
// the highest destructive gradient and are gated by a human-in-the-loop soft
// guardrail carried in the skill doc, not an approval gate in the tool.
//
// Responder sharing (responders add/remove/list) is a cross-API synthesis onto
// the Drive v3 permissions endpoint using the published view. It is bounded by
// the drive.file scope: it only reaches forms this client created (design 303
// §张力 2).
//
// A 401/403 very often means the token lacks a scope the user never granted —
// those errors carry an explicit reconnect hint.
package forms

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

// DefaultBaseURL is the production Forms API base.
const DefaultBaseURL = "https://forms.googleapis.com/v1"

// DefaultDriveBaseURL is the production Drive API v3 base — responder sharing
// (responders *) targets Drive permissions on the form's published view.
const DefaultDriveBaseURL = "https://www.googleapis.com/drive/v3"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/forms.json).
const EnvAccessToken = "FORMS_ACCESS_TOKEN"

// scopeHint is appended to 401/403 errors: the usual cause is a token that
// lacks a scope the user never granted on connect.
const scopeHint = " (possibly missing scope — reconnect and grant access)"

// Service implements the built-in Google Forms tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Forms API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// DriveBaseURL overrides the Drive API base; empty = DefaultDriveBaseURL.
	DriveBaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// sleep overrides the retry backoff sleeper; nil = time.Sleep. Tests
	// inject a recorder to keep retries deterministic and fast.
	sleep func(time.Duration)
}

// Execute runs one forms subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "FORMS_ACCESS_TOKEN is not set")
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

func (s *Service) driveBase() string {
	if s.DriveBaseURL != "" {
		return strings.TrimRight(s.DriveBaseURL, "/")
	}
	return DefaultDriveBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "forms",
		Short:         "Google Forms built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON instead of the human-readable summary")

	responses := newGroupCmd("responses", "Form responses (read-only)")
	responses.AddCommand(s.newResponsesListCmd(token), s.newResponsesGetCmd(token))

	responders := newGroupCmd("responders", "Who can answer the form (Drive permissions on the published view)")
	responders.AddCommand(
		s.newRespondersListCmd(token),
		s.newRespondersAddCmd(token),
		s.newRespondersRemoveCmd(token),
	)

	root.AddCommand(
		s.newGetCmd(token),
		s.newCreateCmd(token),
		s.newBatchUpdateCmd(token),
		s.newPublishCmd(token, publishOp),
		s.newPublishCmd(token, unpublishOp),
		s.newPublishCmd(token, closeOp),
		s.newPublishCmd(token, reopenOp),
		responses,
		responders,
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
