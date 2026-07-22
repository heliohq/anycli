// Package later is the built-in Later Influence Reporting API service: a
// non-interactive cobra tree over the read-only analytics surface at
// https://reporting.api.later.com (v2). Later's public API is two-legged OAuth
// 2.0 client-credentials: the service exchanges a clientId/clientSecret pair
// for a short-lived JWT (POST /oauth/token) and passes that JWT as
// "Authorization: Bearer <jwt>" on every data request, re-minting once on a
// 401. The credential is injected as a single combined secret
// LATER_CREDENTIALS="<clientId>:<clientSecret>" (first-colon split) because the
// Helio manual-credential storage face is a single secret (design 317 D5).
//
// This wraps ONLY the reporting surface. Later's social-scheduling product has
// no public API, so this tool cannot schedule or publish — it reads campaign
// and instance performance for reporting.
package later

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

// DefaultBaseURL is the production Later Influence Reporting API base (v2).
const DefaultBaseURL = "https://reporting.api.later.com"

// EnvCredentials is the env var the credential binding injects
// (definitions/tools/later.json). It carries the client-credentials pair as a
// single "<clientId>:<clientSecret>" string; the service splits on the first
// colon (a clientId never contains one, the secret may).
const EnvCredentials = "LATER_CREDENTIALS"

// Service implements the built-in Later tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API base; empty = DefaultBaseURL. Tests point it at
	// an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one later subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	clientID, clientSecret, ok := splitCredentials(env[EnvCredentials])
	if !ok {
		fmt.Fprintln(s.stderr(), "LATER_CREDENTIALS is not set or is not in <clientId>:<clientSecret> form")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(clientID, clientSecret)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

// splitCredentials parses the combined "<clientId>:<clientSecret>" secret on
// the first colon. Both halves must be non-empty.
func splitCredentials(raw string) (clientID, clientSecret string, ok bool) {
	raw = strings.TrimSpace(raw)
	idx := strings.IndexByte(raw, ':')
	if idx <= 0 {
		return "", "", false
	}
	clientID = strings.TrimSpace(raw[:idx])
	clientSecret = strings.TrimSpace(raw[idx+1:])
	if clientID == "" || clientSecret == "" {
		return "", "", false
	}
	return clientID, clientSecret, true
}

func (s *Service) newRoot(clientID, clientSecret string) *cobra.Command {
	root := &cobra.Command{
		Use:           "later",
		Short:         "Later Influence Reporting built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	client := &reportingClient{svc: s, clientID: clientID, clientSecret: clientSecret}
	root.AddCommand(
		s.newInstancesCmd(client),
		s.newCampaignsCmd(client),
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
