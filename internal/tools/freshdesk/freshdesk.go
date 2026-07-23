// Package freshdesk is the built-in Freshdesk service: a non-interactive cobra
// tree over the Freshdesk v2 REST surface
// (https://<domain>.freshdesk.com/api/v2). Auth is HTTP Basic with the API key
// as username and any dummy password ("Authorization: Basic base64(<key>:X)").
// Freshdesk is per-account, so the base URL is built from a required domain
// credential in addition to the key. Errors are non-2xx with a JSON body
// carrying description/errors; 401/403 rejects the credential, 429 surfaces
// Retry-After. Every command emits the provider JSON on stdout (passthrough +
// newline).
package freshdesk

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

// EnvAPIKey is the env var carrying the Freshdesk API key (the Basic-auth
// username). Injected per definitions/tools/freshdesk.json.
const EnvAPIKey = "FRESHDESK_API_KEY"

// EnvDomain is the env var carrying the Freshdesk account domain. Accepts a
// bare subdomain ("acme"), a full host ("acme.freshdesk.com"), or a URL
// ("https://acme.freshdesk.com"); normalizeDomain reduces all three to the
// canonical host.
const EnvDomain = "FRESHDESK_DOMAIN"

// dummyPassword is the literal placeholder Freshdesk documents for the Basic
// password slot ("-u apikey:X"): the key authenticates, the password is
// ignored.
const dummyPassword = "X"

// Service implements the built-in Freshdesk tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the derived per-account API base; empty = built from
	// FRESHDESK_DOMAIN. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one freshdesk subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	apiKey := env[EnvAPIKey]
	if apiKey == "" {
		fmt.Fprintln(s.stderr(), EnvAPIKey+" is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	base, err := s.resolveBaseURL(env[EnvDomain])
	if err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Result{ExitCode: 1}, nil
	}

	root := s.newRoot(apiKey, base)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(s.stderr(), err)
		return execution.Failure(err), nil
	}
	return execution.Result{}, nil
}

func (s *Service) newRoot(apiKey, base string) *cobra.Command {
	root := &cobra.Command{
		Use:           "freshdesk",
		Short:         "Freshdesk built-in service",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	c := &client{apiKey: apiKey, base: base, svc: s}
	root.AddCommand(
		s.newTicketCmd(c),
		s.newContactCmd(c),
		s.newCompanyCmd(c),
		s.newAgentCmd(c),
	)
	return root
}

// resolveBaseURL returns the configured BaseURL override when set (tests),
// otherwise derives https://<host>/api/v2 from the domain credential.
func (s *Service) resolveBaseURL(domain string) (string, error) {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/"), nil
	}
	host, err := normalizeDomain(domain)
	if err != nil {
		return "", err
	}
	return "https://" + host + "/api/v2", nil
}

// normalizeDomain canonicalizes the domain credential to a Freshdesk host.
// It accepts a bare subdomain ("acme"), a full host ("acme.freshdesk.com"),
// or a URL with scheme and/or path; a bare subdomain gets ".freshdesk.com"
// appended. An empty value is an error (the domain is a required credential).
func normalizeDomain(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if d == "" {
		return "", fmt.Errorf(EnvDomain + " is not set")
	}
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	d = strings.ToLower(strings.TrimSpace(d))
	if d == "" {
		return "", fmt.Errorf("%s %q has no host", EnvDomain, raw)
	}
	if !strings.Contains(d, ".") {
		d += ".freshdesk.com"
	}
	return d, nil
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
