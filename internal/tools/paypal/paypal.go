// Package paypal is the built-in PayPal service: a cobra tree over the PayPal
// REST API (developer.paypal.com/api/rest) scoped to a finance/ops teammate's
// read-first workflow — invoicing, transaction reporting, account balances, and
// subscription lookup.
//
// PayPal has no authorization-code consent flow a shared app could register for
// arbitrary accounts; every REST call is authorized by a bearer token minted
// from an app-owned client_credentials grant. This service therefore performs
// the client-credentials exchange itself (POST {host}/v1/oauth2/token, HTTP
// Basic auth) from the user-supplied client_id + secret, caches the bearer for
// the process lifetime, and calls the data endpoints with it. The
// Sandbox/Live host is selected from PAYPAL_ENV, because a credential pair is
// bound to the environment its REST app was created in.
package paypal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// Env vars the credential binding injects (definitions/tools/paypal.json). The
// secret is stored under the field name `api_secret` (integration-service
// denylists `client_secret`) but its inject env var is the semantically
// correct PAYPAL_CLIENT_SECRET.
const (
	EnvClientID     = "PAYPAL_CLIENT_ID"
	EnvClientSecret = "PAYPAL_CLIENT_SECRET"
	EnvEnvironment  = "PAYPAL_ENV"
)

// PayPal environment-bound API hosts. A Sandbox app's credential pair only
// authorizes against the sandbox host; a Live app's only against the live host.
const (
	liveBaseURL    = "https://api-m.paypal.com"
	sandboxBaseURL = "https://api-m.sandbox.paypal.com"
)

// Service implements the built-in PayPal tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the environment-selected host; empty = select from
	// PAYPAL_ENV. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one paypal subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, invalid dates, unknown
// subcommands) are exit 2; runtime/API errors (PayPal non-2xx, transport
// failure) are exit 1. Errors render to stderr — as JSON under --json, plain
// text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	clientID := env[EnvClientID]
	secret := env[EnvClientSecret]
	if clientID == "" || secret == "" {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET must be set"})
		return execution.Result{ExitCode: 1}, nil
	}
	cl := &client{
		baseURL:  s.resolveBaseURL(env[EnvEnvironment]),
		clientID: clientID,
		secret:   secret,
		hc:       s.httpClient(),
	}
	root := s.newRoot(cl)
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

// resolveBaseURL picks the API host. An explicit BaseURL (test override) wins;
// otherwise PAYPAL_ENV selects sandbox vs live, defaulting to live when unset
// or unrecognized — the production posture for a real PayPal business account.
func (s *Service) resolveBaseURL(environment string) string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	if environment == "sandbox" {
		return sandboxBaseURL
	}
	return liveBaseURL
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

func (s *Service) httpClient() *http.Client {
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

// newRoot builds the grouped-by-resource cobra tree. Every command hangs under
// a resource group (invoice / transaction / balance / subscription).
func (s *Service) newRoot(cl *client) *cobra.Command {
	root := &cobra.Command{
		Use:           "paypal",
		Short:         "PayPal built-in service (invoicing, reporting, subscriptions)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	invoice := newGroupCmd("invoice", "List, read, search, draft, and send invoices")
	invoice.AddCommand(
		s.newInvoiceListCmd(cl),
		s.newInvoiceGetCmd(cl),
		s.newInvoiceSearchCmd(cl),
		s.newInvoiceCreateDraftCmd(cl),
		s.newInvoiceSendCmd(cl),
	)
	transaction := newGroupCmd("transaction", "Read transaction history")
	transaction.AddCommand(s.newTransactionListCmd(cl))
	balance := newGroupCmd("balance", "Read account balances")
	balance.AddCommand(s.newBalanceListCmd(cl))
	subscription := newGroupCmd("subscription", "Look up subscriptions")
	subscription.AddCommand(s.newSubscriptionGetCmd(cl))

	root.AddCommand(invoice, transaction, balance, subscription)
	return root
}

// newGroupCmd is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it: a bare
// group shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// NewCommandTree returns the full command tree built with empty credentials for
// dry-run parsing and traversal (tools.Service seam, design 318). Credentials
// are only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command {
	return s.newRoot(&client{})
}
