// Package hotjar is the built-in Hotjar service: a non-interactive cobra tree
// over the Hotjar (Contentsquare) REST API (https://api.hotjar.io/v1).
//
// Auth is a two-legged OAuth2 client_credentials grant: the account owner's own
// client_id + client_secret (Hotjar Settings → API, Admin-only, auto-expiring
// after one year) are exchanged for a short-lived Bearer token at
// POST /v1/oauth/token. The service performs that exchange once per invocation
// and caches the token in memory for the life of the process (well within the
// ~3600s token life); it never sees a Helio concept — it receives only the two
// user secrets as env vars. This keeps the whole OAuth-ish dance inside anycli,
// so Helio stores a static secret pair and needs zero token-gateway machinery.
//
// The wrapped surface is read-first: survey enumeration + a survey's detail +
// survey-response export (the voice-of-customer data an analytics teammate
// pulls), plus a GDPR/ops user lookup. Deletion is deliberately unreachable:
// Hotjar's user-lookup endpoint doubles as its deletion endpoint via a
// delete_all_hits flag, so the `user lookup` command hardcodes
// delete_all_hits=false with no override (see user.go).
//
// Every command emits the provider JSON on stdout (passthrough + newline).
package hotjar

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

// DefaultBaseURL is the production Hotjar API host. Every request path carries
// the single global version prefix "/v1" itself (Hotjar versions globally, not
// per endpoint), so the base is the bare host.
const DefaultBaseURL = "https://api.hotjar.io"

// tokenPath is the two-legged client_credentials exchange endpoint.
const tokenPath = "/v1/oauth/token"

const (
	// EnvClientID and EnvClientSecret are the env vars the credential binding
	// injects (definitions/tools/hotjar.json). Both are the account owner's own
	// long-lived app credentials, not user tokens.
	EnvClientID     = "HOTJAR_CLIENT_ID"
	EnvClientSecret = "HOTJAR_CLIENT_SECRET"
)

// Service implements the built-in Hotjar tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Hotjar API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server (both the token exchange and API calls ride it).
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer

	// token caches the exchanged Bearer for the invocation (ensureToken).
	token string
}

// clientCreds carries the two account secrets down the cobra tree.
type clientCreds struct {
	clientID     string
	clientSecret string
}

// Execute runs one hotjar subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	clientID := strings.TrimSpace(env[EnvClientID])
	clientSecret := strings.TrimSpace(env[EnvClientSecret])
	if clientID == "" || clientSecret == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvClientID + " and " + EnvClientSecret + " must both be set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(clientCreds{clientID: clientID, clientSecret: clientSecret})
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
	// usageError plus every cobra-originated parse/arg/required-flag error is
	// inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

func (s *Service) newRoot(creds clientCreds) *cobra.Command {
	root := &cobra.Command{
		Use:           "hotjar",
		Short:         "Hotjar built-in service (survey responses export + user lookup)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newSurveyCmd(creds),
		s.newUserCmd(creds),
	)
	return root
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-credential check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
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
