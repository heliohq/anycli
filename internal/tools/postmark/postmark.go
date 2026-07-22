// Package postmark is the built-in Postmark service: a non-interactive cobra
// tree over the Postmark REST surface (https://api.postmarkapp.com). Auth is a
// Server API Token delivered in the "X-Postmark-Server-Token" request header.
//
// Postmark's error dialect is uniform: every error response — and every send
// response — carries {"ErrorCode":<int>,"Message":"…"}. A successful send has
// ErrorCode 0; a successful read (GET /server, /templates, /messages/…, …)
// returns its data object with NO ErrorCode field at all. The service therefore
// keys success on "HTTP 2xx AND (ErrorCode absent OR == 0)": the absent arm
// covers reads (Go decodes the missing field to its 0 zero value), the == 0 arm
// covers a successful send, and the same rule stays correct once the v1.1 batch
// endpoints (which may return HTTP 200 with a per-message non-zero ErrorCode)
// land. A 401 rejects the credential.
//
// Every command emits the provider JSON on stdout (passthrough + newline)
// except `server get`, which prints a REDACTED subset and never the raw body:
// GET /server echoes the caller's own Server API Token back in its ApiTokens
// array, so that field must never reach stdout.
package postmark

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Postmark API base. Postmark endpoints hang
// directly off the host (/email, /server, /messages/outbound, …) with no
// version path segment.
const DefaultBaseURL = "https://api.postmarkapp.com"

// EnvServerToken is the env var the credential binding injects
// (definitions/tools/postmark.json). A Postmark Server API Token is a static,
// non-expiring bearer of one server's privileges until the user rotates it.
const EnvServerToken = "POSTMARK_SERVER_TOKEN"

// serverTokenHeader is the Postmark authentication header for Server-scoped
// requests. Every built-in Postmark call sets it.
const serverTokenHeader = "X-Postmark-Server-Token"

// Service implements the built-in Postmark tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Postmark API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one postmark subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (illegal flag combos, bad JSON, missing
// required flags, unknown subcommands) are exit 2; runtime/API errors (Postmark
// non-2xx or a non-zero ErrorCode, transport failure) are exit 1. Errors render
// to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvServerToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "POSTMARK_SERVER_TOKEN is not set"})
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "postmark",
		Short:         "Postmark built-in service (transactional email)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newEmailCmd(token),
		s.newMessageCmd(token),
		s.newTemplateCmd(token),
		s.newBounceCmd(token),
		s.newStatsCmd(token),
		s.newServerCmd(token),
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

func (s *Service) baseURL() string {
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
