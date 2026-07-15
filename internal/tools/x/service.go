// Package x implements the built-in X service over the X API v2. It accepts
// OAuth 2.0 user-context credentials and exposes a non-interactive Cobra tree.
package x

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

const (
	// DefaultAPIBase is the production X API base.
	DefaultAPIBase = "https://api.x.com"

	// EnvAccessToken and EnvUserID are populated by the credential bindings in
	// definitions/tools/x.json.
	EnvAccessToken = "X_ACCESS_TOKEN"
	EnvUserID      = "X_USER_ID"
)

// Service implements the built-in X tool. Empty fields select production
// defaults; tests inject an HTTP server and output buffers.
type Service struct {
	APIBase string
	HC      *http.Client
	Out     io.Writer
	Err     io.Writer
}

// Execute runs one X subcommand with credentials resolved by the host.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "X_ACCESS_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(token, env[EnvUserID])
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

func (s *Service) newRoot(token, userID string) *cobra.Command {
	root := &cobra.Command{
		Use:           "x",
		Short:         "X built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "single-result JSON; multi-result commands may emit JSONL")

	root.AddCommand(
		s.newMeCmd(token),
		s.newUserCmd(token),
		s.newPostCmd(token),
		s.newTimelineCmd(token, userID),
		s.newRepostCmd(token, userID),
		s.newMediaCmd(token),
		s.newDMCmd(token),
	)
	return root
}

func (s *Service) apiBase() string {
	if s.APIBase != "" {
		return strings.TrimRight(s.APIBase, "/")
	}
	return DefaultAPIBase
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
