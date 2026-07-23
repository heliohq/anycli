// Package sage is the built-in Sage Accounting service: a resource-grouped
// cobra tree over the Sage Accounting (Business Cloud) REST API v3.1
// (api.accounting.sage.com/v3.1). Every call carries a Bearer access token; a
// global --business flag selects the target business via the X-Business header
// (omitted → the user's lead business). Reads are modeled as list/get; the few
// writes (contact create, sales-invoice create, contact-payment create) take a
// verbatim --body JSON payload so the caller supplies the exact resource
// envelope. A generic top-level `fetch` reaches any other v3.1 resource.
//
// Sage fails with a non-2xx status and a JSON body carrying the error detail;
// every call surfaces both. OAuth (authorize, refresh, rotating refresh token)
// lives entirely on the Helio side — this service only receives a ready access
// token in SAGE_ACCESS_TOKEN.
package sage

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

// DefaultBaseURL is the production Sage Accounting API base (v3.1 is the single
// worldwide base URL — no country-specific routing since v3.1).
const DefaultBaseURL = "https://api.accounting.sage.com/v3.1"

// EnvToken is the env var the credential binding injects (definitions/tools/sage.json).
const EnvToken = "SAGE_ACCESS_TOKEN"

// Service implements the built-in Sage Accounting tool. It satisfies
// tools.Service by duck typing (this package never imports the registry — no
// import cycle).
type Service struct {
	// BaseURL overrides the Sage API base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one sage subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flags, bad JSON,
// unknown subcommands) are exit 2; runtime/API errors (Sage non-2xx, transport
// failure) are exit 1. Errors render to stderr — as JSON under --json, plain
// text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &usageError{msg: "SAGE_ACCESS_TOKEN is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token)
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
	// usageError plus every cobra-originated parse/arg/unknown-command error is
	// inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry the --json global flag, used to
// pick the error format before cobra has parsed flags (e.g. the pre-parse
// missing-token check).
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

// newRoot builds the resource-grouped cobra tree. `fetch` is the top-level
// cross-resource escape hatch; everything else hangs under a resource group.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "sage",
		Short:         "Sage Accounting built-in service (REST v3.1)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	// Global (persistent) flags — visible to every subcommand.
	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output")
	pf.String("business", "", "target business id (X-Business header); omit for the user's lead business")

	business := newGroupCmd("business", "Businesses (discover X-Business ids)")
	business.AddCommand(
		s.newListCmd(token, "list", "/businesses", "List businesses"),
		s.newGetCmd(token, "get", "/businesses", "Get a business by id"),
	)
	contact := newGroupCmd("contact", "Customers and suppliers")
	contact.AddCommand(
		s.newListCmd(token, "list", "/contacts", "List contacts"),
		s.newGetCmd(token, "get", "/contacts", "Get a contact by id"),
		s.newCreateCmd(token, "create", "/contacts", "Create a contact (--body wraps a contact object)"),
	)
	salesInvoice := newGroupCmd("sales-invoice", "Customer sales invoices")
	salesInvoice.AddCommand(
		s.newListCmd(token, "list", "/sales_invoices", "List sales invoices"),
		s.newGetCmd(token, "get", "/sales_invoices", "Get a sales invoice by id"),
		s.newCreateCmd(token, "create", "/sales_invoices", "Raise a sales invoice (--body wraps a sales_invoice object)"),
	)
	purchaseInvoice := newGroupCmd("purchase-invoice", "Supplier bills")
	purchaseInvoice.AddCommand(
		s.newListCmd(token, "list", "/purchase_invoices", "List purchase invoices"),
		s.newGetCmd(token, "get", "/purchase_invoices", "Get a purchase invoice by id"),
	)
	contactPayment := newGroupCmd("contact-payment", "Payments and receipts")
	contactPayment.AddCommand(
		s.newCreateCmd(token, "create", "/contact_payments", "Record a payment/receipt (--body wraps a contact_payment object)"),
	)
	ledgerAccount := newGroupCmd("ledger-account", "Chart of accounts")
	ledgerAccount.AddCommand(
		s.newListCmd(token, "list", "/ledger_accounts", "List ledger accounts"),
	)
	bankAccount := newGroupCmd("bank-account", "Bank accounts and balances")
	bankAccount.AddCommand(
		s.newListCmd(token, "list", "/bank_accounts", "List bank accounts"),
		s.newGetCmd(token, "get", "/bank_accounts", "Get a bank account by id"),
	)
	product := newGroupCmd("product", "Product catalog")
	product.AddCommand(
		s.newListCmd(token, "list", "/products", "List products"),
	)
	service := newGroupCmd("service", "Service catalog")
	service.AddCommand(
		s.newListCmd(token, "list", "/services", "List services"),
	)
	taxRate := newGroupCmd("tax-rate", "Tax rates")
	taxRate.AddCommand(
		s.newListCmd(token, "list", "/tax_rates", "List tax rates"),
	)

	root.AddCommand(
		s.newFetchCmd(token),
		business, contact, salesInvoice, purchaseInvoice, contactPayment,
		ledgerAccount, bankAccount, product, service, taxRate,
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

// NewCommandTree returns the full command tree built with an empty token for
// dry-run parsing and traversal (tools.Service seam, design 318). The token is
// only captured by RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }
