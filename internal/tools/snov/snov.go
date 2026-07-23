// Package snov is the built-in Snov.io service: a non-interactive cobra tree
// over the Snov.io sales-intelligence REST surface (https://api.snov.io).
//
// Auth is a two-legged OAuth2 client_credentials grant: the account owner's
// own client_id + client_secret (Snov Settings → API) are exchanged for a
// short-lived Bearer token at POST /v1/oauth/access_token. The service performs
// that exchange once per invocation and caches the token in memory for the life
// of the process (well within the ~3600s token life); it never sees a Helio
// concept — it receives only the two user secrets as env vars.
//
// Snov mixes two request styles: v1 methods (get-balance, get-profile-by-email,
// get-domain-emails-count) take the access_token as a request parameter and
// return synchronously; v2 methods (domain-search, email-verification,
// emails-by-domain-by-name) authenticate with the Bearer header and are
// asynchronous — a `start` call returns a task_hash and the service polls the
// matching `result` endpoint until the task completes, so the agent issues one
// blocking command and receives the finished payload (never a raw task_hash).
//
// Every command emits the provider JSON on stdout (passthrough + newline). Note
// that email-finder, verification, and enrichment calls consume Snov credits.
package snov

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

// DefaultBaseURL is the production Snov.io API base.
const DefaultBaseURL = "https://api.snov.io"

const (
	// EnvClientID and EnvClientSecret are the env vars the credential binding
	// injects (definitions/tools/snov.json). Both are long-lived account-level
	// app credentials, not user tokens.
	EnvClientID     = "SNOV_CLIENT_ID"
	EnvClientSecret = "SNOV_CLIENT_SECRET"
)

const (
	// defaultPollInterval spaces async result polls; Snov caps traffic at 60
	// req/min, so 2s keeps a single command well under the ceiling.
	defaultPollInterval = 2 * time.Second
	// defaultPollTimeout bounds an async start→result loop overall. Exposed as
	// --timeout on the async commands.
	defaultPollTimeout = 60 * time.Second
)

// Service implements the built-in Snov.io tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Snov API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server (both the token exchange and API calls ride it).
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
	// PollInterval / PollTimeout override the async poll cadence; 0 = the
	// defaults above. Tests shrink them so the poll loop runs instantly.
	PollInterval time.Duration
	PollTimeout  time.Duration

	// token caches the exchanged Bearer for the invocation (ensureToken).
	token string
}

// Execute runs one snov subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	clientID := strings.TrimSpace(env[EnvClientID])
	clientSecret := strings.TrimSpace(env[EnvClientSecret])
	if clientID == "" || clientSecret == "" {
		fmt.Fprintf(s.stderr(), "%s and %s must both be set\n", EnvClientID, EnvClientSecret)
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

// readOnly carries the design-318 anycli.side_effect annotation for runnable
// leaf commands. Every Snov command is a lookup/search/enrich read (balance,
// email finder/verifier, profile enrichment) — no provider-state mutation — so
// all leaves carry it.
var readOnly = map[string]string{"anycli.side_effect": "false"}

func (s *Service) newRoot(clientID, clientSecret string) *cobra.Command {
	root := &cobra.Command{
		Use:           "snov",
		Short:         "Snov.io built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	creds := clientCreds{clientID: clientID, clientSecret: clientSecret}
	root.AddCommand(
		s.newEmailCmd(creds),
		s.newEnrichCmd(creds),
		s.newAccountCmd(creds),
	)
	return root
}

// clientCreds carries the two account secrets down the cobra tree.
type clientCreds struct {
	clientID     string
	clientSecret string
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

func (s *Service) pollInterval() time.Duration {
	if s.PollInterval > 0 {
		return s.PollInterval
	}
	return defaultPollInterval
}

func (s *Service) pollTimeout() time.Duration {
	if s.PollTimeout > 0 {
		return s.PollTimeout
	}
	return defaultPollTimeout
}
