// Package zuora is the built-in Zuora Billing service: a cobra tree over the
// Zuora Billing REST v1 surface (developer.zuora.com) scoped to a
// finance/ops teammate's read-first workflow — account lookups and rolled-up
// summaries, subscriptions, invoices, payments, the product catalog, and a
// read-only ZOQL query escape hatch.
//
// Zuora has no authorization-code consent flow a shared app could register for
// arbitrary tenants; every REST call is authorized by a bearer token minted
// from a per-tenant OAuth client the customer creates inside their own Zuora
// tenant (client_credentials grant). This service therefore performs the
// client-credentials exchange itself (POST {base_url}/oauth/token, form body,
// no auth headers), caches the bearer for the process lifetime, and calls the
// data endpoints with it. The REST base URL is user-supplied because each Zuora
// data center / environment (US/EU, production/sandbox) has a distinct host and
// cannot be inferred from the id/secret pair.
package zuora

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

// Env vars the credential binding injects (definitions/tools/zuora.json). The
// secret is stored under the field name `api_secret` (integration-service
// denylists `client_secret`) but its inject env var is the semantically correct
// ZUORA_CLIENT_SECRET.
const (
	EnvBaseURL      = "ZUORA_BASE_URL"
	EnvClientID     = "ZUORA_CLIENT_ID"
	EnvClientSecret = "ZUORA_CLIENT_SECRET"
)

// Service implements the built-in Zuora tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the user-supplied ZUORA_BASE_URL; empty = use the env
	// value. Tests point it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one zuora subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad flags, missing required flags,
// invalid JSON, unknown subcommands) are exit 2; runtime/API errors (Zuora
// non-2xx, an error envelope in a 2xx body, transport failure) are exit 1.
// Errors render to stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	baseURL := s.resolveBaseURL(env[EnvBaseURL])
	clientID := env[EnvClientID]
	secret := env[EnvClientSecret]
	if baseURL == "" || clientID == "" || secret == "" {
		// The credential check runs before cobra parses flags, so detect --json
		// in the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "ZUORA_BASE_URL, ZUORA_CLIENT_ID, and ZUORA_CLIENT_SECRET must be set"})
		return execution.Result{ExitCode: 1}, nil
	}
	cl := &client{
		baseURL:  strings.TrimRight(baseURL, "/"),
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

// resolveBaseURL picks the REST host. An explicit BaseURL (test override) wins;
// otherwise the user-supplied ZUORA_BASE_URL is used verbatim — unlike an
// environment-selected host, Zuora's data center is not derivable and must be
// entered by the tenant admin.
func (s *Service) resolveBaseURL(envBaseURL string) string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	return strings.TrimSpace(envBaseURL)
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
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>,"code":<Zuora code or omitted>}}.
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
		if apiErr.code != "" {
			payload["code"] = apiErr.code
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
// a resource group (account / subscription / invoice / payment / catalog) plus
// the top-level `query` ZOQL escape hatch.
func (s *Service) newRoot(cl *client) *cobra.Command {
	root := &cobra.Command{
		Use:           "zuora",
		Short:         "Zuora Billing built-in service (accounts, subscriptions, invoices, payments, catalog, ZOQL)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured JSON output")

	account := newGroupCmd("account", "Read accounts and rolled-up account summaries")
	account.AddCommand(
		s.newAccountGetCmd(cl),
		s.newAccountSummaryCmd(cl),
	)
	subscription := newGroupCmd("subscription", "List and read subscriptions")
	subscription.AddCommand(
		s.newSubscriptionListCmd(cl),
		s.newSubscriptionGetCmd(cl),
	)
	invoice := newGroupCmd("invoice", "Read invoices and list an account's invoices")
	invoice.AddCommand(
		s.newInvoiceGetCmd(cl),
		s.newInvoiceListCmd(cl),
	)
	payment := newGroupCmd("payment", "Read payments and list an account's payments (needs Invoice Settlement)")
	payment.AddCommand(
		s.newPaymentGetCmd(cl),
		s.newPaymentListCmd(cl),
	)
	catalog := newGroupCmd("catalog", "Read the product catalog and rate plans")
	catalog.AddCommand(s.newCatalogProductsCmd(cl))

	root.AddCommand(account, subscription, invoice, payment, catalog, s.newQueryCmd(cl))
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
