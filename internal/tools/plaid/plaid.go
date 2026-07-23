// Package plaid is the built-in Plaid service: a non-interactive cobra tree over
// the Plaid REST API (https://sandbox.plaid.com and https://production.plaid.com).
//
// Plaid cleanly separates two credential classes, and this tool models that
// split deliberately:
//   - App credentials — client_id + secret — are the stored Helio connection,
//     injected as the PLAID-CLIENT-ID / PLAID-SECRET headers on every call. They
//     are long-lived, non-expiring, and environment-scoped.
//   - An Item access_token is per-linked-bank runtime data, NOT a stored
//     credential. It is supplied per invocation via --access-token (obtained
//     from the user's own Link integration, or minted by `sandbox
//     public-token-create` + `item exchange-public-token` in sandbox).
//
// PLAID_ENV selects the base host (sandbox|production); it defaults to sandbox
// and any other value is rejected with a usage error rather than silently
// guessing. Every call is an HTTPS POST returning JSON, emitted verbatim on
// stdout. Errors surface Plaid's own error_type / error_code / error_message /
// request_id so an agent can self-correct.
package plaid

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

// Host constants for the two live Plaid environments. The legacy `development`
// host is retired and intentionally unsupported.
const (
	sandboxBaseURL    = "https://sandbox.plaid.com"
	productionBaseURL = "https://production.plaid.com"
)

// Env var names injected by the credential bindings (definitions/tools/plaid.json).
const (
	EnvClientID    = "PLAID_CLIENT_ID"
	EnvSecret      = "PLAID_SECRET"
	EnvEnvironment = "PLAID_ENV"
)

// Environment names. sandbox is the default when PLAID_ENV is unset.
const (
	envSandbox    = "sandbox"
	envProduction = "production"
)

// Service implements the built-in Plaid tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the derived Plaid host; empty = derive from PLAID_ENV.
	// Tests point it at an httptest server while still passing PLAID_ENV=sandbox
	// so the environment-scoped behavior (e.g. the sandbox-only guard) is exercised.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// creds carries the resolved app credentials plus the selected environment into
// each command's RunE closure. access_token is never part of this — it is a
// per-invocation flag, not a stored credential.
type creds struct {
	clientID string
	secret   string
	baseURL  string
	env      string
}

// Execute runs one plaid subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad enums, invalid flags, unknown
// subcommands, an unsupported PLAID_ENV, a sandbox-only command run against
// production) are exit 2; runtime/API errors (Plaid non-2xx, transport failure)
// are exit 1. Errors render to stderr — as a structured envelope under --json,
// plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	jsonMode := hasJSONArg(args)

	clientID := strings.TrimSpace(env[EnvClientID])
	secret := strings.TrimSpace(env[EnvSecret])
	if clientID == "" || secret == "" {
		s.renderError(jsonMode, &usageError{msg: "PLAID_CLIENT_ID and PLAID_SECRET must both be set"})
		// A missing stored credential is a setup failure, not a parse error → exit 1.
		return execution.Result{ExitCode: 1}, nil
	}

	envName, err := resolveEnv(env[EnvEnvironment])
	if err != nil {
		s.renderError(jsonMode, err)
		return execution.Result{ExitCode: 2}, nil
	}

	c := creds{
		clientID: clientID,
		secret:   secret,
		baseURL:  s.resolveBaseURL(envName),
		env:      envName,
	}

	root := s.newRoot(c)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		jm, _ := root.PersistentFlags().GetBool("json")
		s.renderError(jm || jsonMode, err)
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return execution.Failure(err), nil
		}
		// usageError plus every cobra parse/arg/enum/unknown-command error → exit 2.
		return execution.Result{ExitCode: 2}, nil
	}
	return execution.Result{}, nil
}

// resolveEnv normalizes PLAID_ENV. Empty defaults to sandbox; sandbox and
// production pass through; anything else is a fail-fast usage error (no silent
// fallback to a guessed host).
func resolveEnv(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "":
		return envSandbox, nil
	case envSandbox, envProduction:
		return v, nil
	default:
		return "", &usageError{msg: fmt.Sprintf("PLAID_ENV must be %q or %q, got %q", envSandbox, envProduction, raw)}
	}
}

// resolveBaseURL picks the host. A non-empty Service.BaseURL (test override)
// always wins; otherwise the host is derived from the selected environment.
func (s *Service) resolveBaseURL(envName string) string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	if envName == envProduction {
		return productionBaseURL
	}
	return sandboxBaseURL
}

// newRoot builds the resource-grouped cobra tree. Every leaf is --json-capable
// and non-interactive.
func (s *Service) newRoot(c creds) *cobra.Command {
	root := &cobra.Command{
		Use:           "plaid",
		Short:         "Plaid built-in service (institution lookups, Item reads, sandbox loop)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	institutions := newGroupCmd("institutions", "Look up financial institutions (no access_token required)")
	institutions.AddCommand(
		s.newInstitutionsGetCmd(c),
		s.newInstitutionsGetByIDCmd(c),
	)
	accounts := newGroupCmd("accounts", "Read accounts and balances for a linked Item")
	accounts.AddCommand(
		s.newAccountsGetCmd(c),
		s.newAccountsBalanceCmd(c),
	)
	auth := newGroupCmd("auth", "Read account and routing numbers for a linked Item")
	auth.AddCommand(s.newAuthGetCmd(c))
	transactions := newGroupCmd("transactions", "Read transactions for a linked Item")
	transactions.AddCommand(
		s.newTransactionsSyncCmd(c),
		s.newTransactionsGetCmd(c),
	)
	identity := newGroupCmd("identity", "Read account-holder identity for a linked Item")
	identity.AddCommand(s.newIdentityGetCmd(c))
	item := newGroupCmd("item", "Inspect, exchange, and remove Items")
	item.AddCommand(
		s.newItemGetCmd(c),
		s.newItemRemoveCmd(c),
		s.newItemExchangePublicTokenCmd(c),
	)
	sandbox := newGroupCmd("sandbox", "Sandbox-only helpers (no Link widget required)")
	sandbox.AddCommand(s.newSandboxPublicTokenCreateCmd(c))

	root.AddCommand(institutions, accounts, auth, transactions, identity, item, sandbox)
	return root
}

// newGroupCmd is a runnable command group: cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a false
// success for an agent); making the group runnable restores it (a bare group
// shows help, an unknown subcommand fails).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-credential and bad-env checks).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","error_type":…,"error_code":…,
// "request_id":…,"status":<HTTP>}}, with the Plaid-specific fields present only
// for an apiError that decoded a Plaid error body.
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
		if apiErr.errorType != "" {
			payload["error_type"] = apiErr.errorType
		}
		if apiErr.errorCode != "" {
			payload["error_code"] = apiErr.errorCode
		}
		if apiErr.requestID != "" {
			payload["request_id"] = apiErr.requestID
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
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

// NewCommandTree returns the full command tree built with empty credentials for
// dry-run parsing and traversal (tools.Service seam, design 318). Credentials
// are only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot(creds{}) }
