// Package hunter is the built-in Hunter.io service: a non-interactive cobra
// tree over the Hunter v2 REST surface (https://api.hunter.io/v2). Auth is the
// per-account API key, sent as the "X-API-KEY" header on every request (never
// the api_key query parameter, so the key never leaks into logs or URLs).
//
// Hunter errors are non-2xx with a JSON body carrying an
// {"errors":[{"id","code","details"}]} envelope; 401 rejects the credential.
// Two Hunter quirks are inverted from common conventions and documented for
// callers: 403 means the per-second rate limit was hit, 429 means the monthly
// quota is exhausted — both are plain errors, not credential rejections.
//
// Every command emits the provider JSON on stdout verbatim (+ newline). The
// Email Verifier may reply 202 while verification is still running; a 202 is a
// success passthrough (the body's data.status tells the agent to re-poll).
package hunter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Hunter v2 API base.
const DefaultBaseURL = "https://api.hunter.io/v2"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/hunter.json). A Hunter API key is a long-lived, scopeless
// bearer secret with no expiry.
const EnvAPIKey = "HUNTER_API_KEY"

// readOnly / writeAction carry the design-318 anycli.side_effect annotation for
// runnable leaf commands: "false" for state-free reads, "true" for provider
// mutations. Group commands must not carry either.
var (
	readOnly    = map[string]string{"anycli.side_effect": "false"}
	writeAction = map[string]string{"anycli.side_effect": "true"}
)

// Service implements the built-in Hunter tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Hunter API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one hunter subcommand with the resolved credentials in env.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	key := env[EnvAPIKey]
	if key == "" {
		fmt.Fprintln(s.stderr(), "HUNTER_API_KEY is not set")
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(key)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}
	fmt.Fprintln(s.stderr(), err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		// Runtime/API failure: exit 1, preserving the credential-rejection
		// classification carried through the wrapped cause (401).
		return execution.Failure(err), nil
	}
	// Every cobra parse/arg/unknown-command error and every bad --json/--filters
	// flag surfaces here as a plain error → usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

func (s *Service) newRoot(key string) *cobra.Command {
	root := &cobra.Command{
		Use:   "hunter",
		Short: "Hunter.io built-in service",
		Long: "Speaks the Hunter v2 API with the account's API key in a header. The key\n" +
			"carries no scopes, so anything the plan allows is available, and quota is\n" +
			"per Hunter ACCOUNT rather than per key — a second key cut from the same\n" +
			"account draws on the same pool. Responses are Hunter's JSON verbatim,\n" +
			"payload under `data`.\n" +
			"\n" +
			"Three commands cost credits: `domain-search` (one search credit per 1-10\n" +
			"addresses returned), `email-finder` (one search credit per call, found or\n" +
			"not) and `email-verifier` (one verification credit). `account`,\n" +
			"`email-count`, `domain-finder` and every `lead` and `lead-list` command\n" +
			"are free. Plans are small — a free one is around 25 searches a month — so\n" +
			"do the exploring with the free commands and spend the paid ones once each\n" +
			"where possible.\n" +
			"\n" +
			"Hunter's status codes read backwards from most APIs. 403 is the\n" +
			"per-second rate limit and is worth retrying shortly; 429 is the monthly\n" +
			"quota being spent, and nothing will succeed until the `reset_date` that\n" +
			"`account` reports. Only 401 means the key itself is bad. A 404 from the\n" +
			"`enrich` commands is Hunter having no record for that person or company,\n" +
			"which is an ordinary outcome rather than a failure.\n" +
			"\n" +
			"Nearly everything keys off a DOMAIN, not a company name. Where\n" +
			"`--company` is accepted it is a fuzzier match; `domain-finder` turns a\n" +
			"name into a domain for free and is the reliable first step.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newDomainSearchCmd(key),
		s.newEmailCountCmd(key),
		s.newDomainFinderCmd(key),
		s.newEmailFinderCmd(key),
		s.newEmailVerifierCmd(key),
		s.newDiscoverCmd(key),
		s.newEnrichCmd(key),
		s.newLeadCmd(key),
		s.newLeadListCmd(key),
		s.newAccountCmd(key),
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
