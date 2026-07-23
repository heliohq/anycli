// Package formstack is the built-in Formstack service: a non-interactive cobra
// tree over the Formstack v2 (classic) REST surface
// (https://www.formstack.com/api/v2). Auth is "Authorization: Bearer <token>"
// using an OAuth 2.0 access token. Formstack errors are non-2xx with a JSON
// body carrying an "error" message; 401 rejects the credential. Every command
// emits the provider JSON on stdout (passthrough + trailing newline).
package formstack

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

// DefaultBaseURL is the production Formstack v2 (classic) API base.
const DefaultBaseURL = "https://www.formstack.com/api/v2"

// EnvAccessToken is the env var the credential binding injects
// (definitions/tools/formstack.json). Formstack v2 uses an OAuth 2.0 access
// token carrying the authorizing user's own in-app form permissions.
const EnvAccessToken = "FORMSTACK_ACCESS_TOKEN"

// Service implements the built-in Formstack tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Formstack API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one formstack subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvAccessToken]
	if token == "" {
		fmt.Fprintln(s.stderr(), "FORMSTACK_ACCESS_TOKEN is not set")
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
		Use:           "formstack",
		Short:         "Formstack built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newFormCmd(token),
		s.newFieldCmd(token),
		s.newFolderCmd(token),
		s.newSubmissionCmd(token),
		s.newWebhookCmd(token),
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
