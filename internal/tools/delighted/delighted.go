// Package delighted is the built-in Delighted service: a non-interactive cobra
// tree over the Delighted v1 REST surface (https://api.delighted.com/v1). Auth
// is HTTP Basic with the project API key as the username and a blank password
// (req.SetBasicAuth(key, "")) — the key-as-username scheme is kept entirely
// inside AnyCLI; the host stores and serves one opaque secret. Every path ends
// in ".json". Delighted errors are non-2xx with a JSON body; 401 rejects the
// credential. Every command emits the provider JSON on stdout verbatim.
//
// NOTE (provider status): the Delighted product was fully sunset on
// 2026-06-30 and the production REST API returns HTTP 410 Gone as of
// 2026-07-22. This service is faithful to the pre-sunset v1 API and is tested
// entirely against httptest fakes (L1); it ships hidden on the Helio side and
// can never be exercised against the live API (L2/L4/L5 are permanently
// unexecutable). See the branch DESIGN.md.
package delighted

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

// readOnly / writeAction carry the design-318 side-effect annotation for runnable leaves.
var readOnly = map[string]string{"anycli.side_effect": "false"}
var writeAction = map[string]string{"anycli.side_effect": "true"}

// DefaultBaseURL is the production Delighted v1 API base.
const DefaultBaseURL = "https://api.delighted.com/v1"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/delighted.json). The key is a long-lived per-CX-project
// API key used as the HTTP Basic username.
const EnvAPIKey = "DELIGHTED_API_KEY"

// Service implements the built-in Delighted tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Delighted API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one delighted subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		fmt.Fprintln(s.stderr(), "DELIGHTED_API_KEY is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:           "delighted",
		Short:         "Delighted built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newMetricsCmd(key),
		s.newResponseCmd(key),
		s.newPeopleCmd(key),
		s.newBouncesCmd(key),
		s.newUnsubscribesCmd(key),
		s.newAutopilotCmd(key),
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
