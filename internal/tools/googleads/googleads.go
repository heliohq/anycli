// Package googleads is the built-in Google Ads service: a non-interactive
// cobra tree over the Google Ads API REST surface
// (https://googleads.googleapis.com/v24), driven by GAQL (Google Ads Query
// Language). Auth is two headers on every call — "Authorization: Bearer
// <user access token>" and "developer-token: <app developer token>" — plus an
// optional "login-customer-id" header for manager (MCC) operation. The service
// is reporting/analysis-first: enumerate accounts, run GAQL, and a small set of
// guarded status/budget mutations. Google Ads fails with a non-2xx status and a
// JSON body carrying error.details[].errors[] (errorCode + message); every call
// surfaces those verbatim.
package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production Google Ads API base. apiVersion is pinned as
// a constant: Google ships a new version roughly quarterly and supports about
// the latest three, so the version is bumped deliberately on the deprecation
// cadence, never tracked implicitly.
const (
	apiVersion     = "v24"
	DefaultBaseURL = "https://googleads.googleapis.com/" + apiVersion
)

// Env vars the credential bindings inject (definitions/tools/google-ads.json).
// The login-customer-id is optional operator config, not a resolved credential;
// it may arrive via env or the --login-customer-id flag.
const (
	EnvAccessToken     = "GOOGLE_ADS_ACCESS_TOKEN"
	EnvDeveloperToken  = "GOOGLE_ADS_DEVELOPER_TOKEN"
	EnvLoginCustomerID = "GOOGLE_ADS_LOGIN_CUSTOMER_ID"
)

// Service implements the built-in Google Ads tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Google Ads API base; empty = DefaultBaseURL. Tests
	// point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// creds bundles the two resolved headers plus the optional manager id, threaded
// to every request builder.
type creds struct {
	accessToken     string
	developerToken  string
	loginCustomerID string
}

// Execute runs one google-ads subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing required flags,
// unknown subcommands, invalid enums) are exit 2; runtime/API errors (Google
// Ads non-2xx, transport failure) are exit 1. Errors render to stderr — as JSON
// under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	accessToken := env[EnvAccessToken]
	developerToken := env[EnvDeveloperToken]
	// Both credentials are mandatory: every Google Ads call needs the user
	// bearer AND the app developer token. A missing developer token is not a
	// usage error the caller can fix by re-typing — it is a host-side wiring
	// gap — but it is fatal here all the same, so fail fast before cobra.
	if accessToken == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvAccessToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	if developerToken == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvDeveloperToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	c := creds{
		accessToken:     accessToken,
		developerToken:  developerToken,
		loginCustomerID: env[EnvLoginCustomerID],
	}
	root := s.newRoot(c)
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
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// newRoot builds the resource-grouped cobra tree. accounts / query / report are
// top-level (cross-account or GAQL-centric); campaign / budget writes hang off
// their own groups.
func (s *Service) newRoot(c creds) *cobra.Command {
	root := &cobra.Command{
		Use:           "google-ads",
		Short:         "Google Ads built-in service (GAQL-driven reporting + light management)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output")
	// --login-customer-id overrides the GOOGLE_ADS_LOGIN_CUSTOMER_ID env for
	// manager (MCC) operation; empty on both means no login-customer-id header.
	pf.String("login-customer-id", c.loginCustomerID, "manager (MCC) customer id, digits only (optional)")

	accounts := newGroupCmd("accounts", "Enumerate reachable Google Ads accounts")
	accounts.AddCommand(s.newAccountsListCmd(c))

	campaign := newGroupCmd("campaign", "Manage campaigns")
	campaign.AddCommand(s.newCampaignSetStatusCmd(c))

	campaigns := newGroupCmd("campaigns", "List campaigns")
	campaigns.AddCommand(s.newCampaignsListCmd(c))

	budget := newGroupCmd("budget", "Manage campaign budgets")
	budget.AddCommand(s.newBudgetSetCmd(c))

	root.AddCommand(
		accounts,
		s.newQueryCmd(c),
		s.newReportCmd(c),
		campaigns,
		campaign,
		budget,
	)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it: a bare group
// shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// loginCustomerID reads the effective manager id from the persistent flag
// (which is seeded from the env default), trimming surrounding whitespace.
func loginCustomerID(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("login-customer-id")
	return strings.TrimSpace(v)
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-credential checks).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
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
