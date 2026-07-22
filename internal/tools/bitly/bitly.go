// Package bitly is the built-in Bitly service: a non-interactive cobra tree
// over the Bitly v4 REST surface (https://api-ssl.bitly.com/v4). Auth is
// "Authorization: Bearer <token>". Bitly errors are non-2xx with a JSON body
// carrying message/description; 401 rejects the credential. Every command emits
// the provider JSON on stdout (passthrough + newline) except `qr image`, which
// returns image bytes and therefore emits a JSON envelope/receipt instead.
package bitly

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

// DefaultBaseURL is the production Bitly v4 API base.
const DefaultBaseURL = "https://api-ssl.bitly.com/v4"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/bitly.json). Bitly tokens are non-expiring OAuth 2.0
// access tokens.
const EnvAccessToken = "BITLY_ACCESS_TOKEN"

// Service implements the built-in Bitly tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Bitly API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one bitly subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "BITLY_ACCESS_TOKEN is not set")
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
		Use:           "bitly",
		Short:         "Bitly built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newLinkCmd(token),
		s.newAnalyticsCmd(token),
		s.newGroupCmd(token),
		s.newQRCmd(token),
		s.newCampaignCmd(token),
		s.newUserCmd(token),
	)
	return root
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

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token
// is only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
