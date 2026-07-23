// Package tiktok implements the built-in TikTok service over the TikTok API
// v2 (Display API + Content Posting API). It accepts an OAuth 2.0 user access
// token and exposes a non-interactive Cobra tree for a creator account:
// profile, video read, and posting.
package tiktok

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
	// DefaultAPIBase is the production TikTok open-API base.
	DefaultAPIBase = "https://open.tiktokapis.com"

	// EnvAccessToken and EnvOpenID are populated by the credential bindings in
	// definitions/tools/tiktok.json.
	EnvAccessToken = "TIKTOK_ACCESS_TOKEN"
	EnvOpenID      = "TIKTOK_OPEN_ID"
)

// Service implements the built-in TikTok tool. Empty fields select production
// defaults; tests inject an HTTP server and output buffers.
type Service struct {
	APIBase string
	HC      *http.Client
	Out     io.Writer
	Err     io.Writer
}

// Execute runs one TikTok subcommand with credentials resolved by the host.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "TIKTOK_ACCESS_TOKEN is not set")
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(token, env[EnvOpenID])
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for reads (creator info, user info, video
// list/query, post status), "true" for publishing a video (mutates state).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

func (s *Service) newRoot(token, openID string) *cobra.Command {
	root := &cobra.Command{
		Use:           "tiktok",
		Short:         "TikTok built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "single-result JSON; multi-result commands may emit JSONL")

	root.AddCommand(
		s.newUserCmd(token),
		s.newVideoCmd(token),
		s.newCreatorCmd(token),
		s.newPostCmd(token),
	)
	_ = openID // reserved: the acting account is identified by the bearer token
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
