// Package metaads implements the built-in Meta Ads service over the Meta
// Marketing API (Facebook Graph). It accepts an OAuth 2.0 user-context access
// token and exposes a non-interactive Cobra tree grouped by ad object.
package metaads

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
	// DefaultAPIBase is the production Facebook Graph API host.
	DefaultAPIBase = "https://graph.facebook.com"

	// GraphVersion pins the Graph API version. Meta deprecates versions on a
	// rolling ~2-year clock, so this is one maintained service constant rather
	// than a per-request default — matching the "no silent fallback" rule.
	GraphVersion = "v23.0"

	// EnvAccessToken is populated by the credential binding in
	// definitions/tools/meta-ads.json.
	EnvAccessToken = "META_ACCESS_TOKEN"
)

// readOnly / writeAction tag each leaf command for the design-318 approval
// gate. readOnly marks side-effect-free reads (GET list/get/insights);
// writeAction marks Graph writes that mutate provider state (create/update).
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Meta Ads tool. Empty fields select
// production defaults; tests inject an HTTP server and output buffers.
type Service struct {
	APIBase string
	HC      *http.Client
	Out     io.Writer
	Err     io.Writer
}

// Execute runs one Meta Ads subcommand with credentials resolved by the host.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "META_ACCESS_TOKEN is not set")
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

func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "meta-ads",
		Short:         "Meta Ads built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "single-result JSON; multi-result commands may emit JSONL")

	root.AddCommand(
		s.newAccountsCmd(token),
		s.newCampaignCmd(token),
		s.newAdSetCmd(token),
		s.newAdCmd(token),
		s.newCreativeCmd(token),
		s.newInsightsCmd(token),
	)
	return root
}

func (s *Service) apiBase() string {
	if s.APIBase != "" {
		return strings.TrimRight(s.APIBase, "/")
	}
	return DefaultAPIBase
}

// graphPath prefixes a node/edge path with the pinned Graph version.
func (s *Service) graphPath(path string) string {
	return "/" + GraphVersion + path
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
