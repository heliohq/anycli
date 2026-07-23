// Package zohobooks is the built-in Zoho Books service: a resource-grouped
// cobra tree over the Zoho Books REST API v3
// (https://www.zohoapis.com/books/v3). It wraps organizations, contacts
// (customers + vendors), invoices, estimates, items, bills, customer payments,
// and expenses so an assistant can answer "did customer X pay?", look people
// up, and capture receivables.
//
// Two Books-specific rules shape the CLI:
//
//   - organization_id is mandatory on every data-plane call except
//     GET /organizations. It is a query parameter (a per-call org selector, not
//     part of the OAuth token), surfaced as the required --organization-id root
//     flag; `org list` is the one command that omits it and discovers the ids.
//   - Books signals success with an integer body `code` (0 = success), in
//     addition to the HTTP status. A non-2xx is an error; a 2xx carrying a
//     non-zero body `code` is defensively treated as an error too.
//
// V1 is scoped to Zoho's US datacenter (.com hosts): the access token is
// datacenter-specific, so a non-US Zoho account fails at the token layer with
// an explicit provider error rather than any silent fallback.
package zohobooks

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

// DefaultBaseURL is the production Zoho Books US-datacenter API host. Paths add
// the /books/v3 version prefix; tests point BaseURL at an httptest server.
const DefaultBaseURL = "https://www.zohoapis.com"

// apiPrefix is the versioned path prefix on every Books call.
const apiPrefix = "/books/v3"

// EnvToken is the env var the credential binding injects
// (definitions/tools/zoho-books.json).
const EnvToken = "ZOHO_BOOKS_ACCESS_TOKEN"

// orgFlag is the name of the persistent organization-selector flag.
const orgFlag = "organization-id"

// Service implements the built-in Zoho Books tool. It satisfies tools.Service
// by duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the API host; empty = DefaultBaseURL. Tests point it
	// at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one zoho-books subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (missing required flags, bad JSON,
// unknown subcommands) are exit 2; runtime/API errors (Books non-2xx, a 2xx
// with a non-zero body code, transport failure) are exit 1. Errors render to
// stderr — as JSON under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		// The token check runs before cobra parses flags, so detect --json in
		// the raw args to honor the structured error-envelope contract.
		s.renderError(hasJSONArg(args), &credentialError{msg: EnvToken + " is not set"})
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
	// usageError plus every cobra-originated parse/arg/enum/unknown-command
	// error is inherently a usage error → exit 2.
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
// {"error":{"message":…,"kind":"usage|credential|api","status":<HTTP or omitted>}}.
// kind mirrors the exit-code contract: usage=2, credential/api=1.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	var credErr *credentialError
	switch {
	case errors.As(err, &apiErr):
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	case errors.As(err, &credErr):
		payload["kind"] = "credential"
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

// NewCommandTree returns the full command tree built with an empty token, for
// lint/inspect/manifest traversal that needs the command structure without a
// live credential.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("") }

// newRoot builds the resource-grouped cobra tree. `org` discovers the ids that
// every other resource needs; contact/invoice/estimate/item/bill/payment/expense
// hang under resource groups and read the persistent --organization-id flag.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "zoho-books",
		Short:         "Zoho Books built-in service (invoices, contacts, estimates, items, bills, payments, expenses)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output on error")
	// organization-id is a persistent selector required on every command
	// except `org list`; the org-scoped commands read it at RunE time.
	orgID := new(string)
	pf.StringVar(orgID, orgFlag, "", "Zoho Books organization id (required except for `org list`; run `org list` to discover it)")

	contact := newGroupCmd("contact", "Look up and capture customers and vendors")
	contact.AddCommand(
		s.newListCmd(token, orgID, "contacts", []stringFlag{
			{"contact-type", "contact_type", "filter by contact_type: customer|vendor"},
			{"filter-by", "filter_by", "Books status view, e.g. Status.Active"},
			{"search-text", "search_text", "free-text search"},
		}),
		s.newGetCmd(token, orgID, "contacts"),
		s.newCreateCmd(token, orgID, "contacts"),
	)
	invoice := newGroupCmd("invoice", "Read and create invoices (receivables)")
	invoice.AddCommand(
		s.newListCmd(token, orgID, "invoices", []stringFlag{
			{"customer-id", "customer_id", "filter by customer id"},
			{"status", "status", "invoice status, e.g. sent|overdue|paid"},
			{"filter-by", "filter_by", "Books status view, e.g. Status.Overdue"},
			{"search-text", "search_text", "free-text search"},
		}),
		s.newGetCmd(token, orgID, "invoices"),
		s.newCreateCmd(token, orgID, "invoices"),
	)
	estimate := newGroupCmd("estimate", "Read and create estimates (quotes)")
	estimate.AddCommand(
		s.newListCmd(token, orgID, "estimates", []stringFlag{
			{"customer-id", "customer_id", "filter by customer id"},
			{"filter-by", "filter_by", "Books status view, e.g. Status.Sent"},
			{"search-text", "search_text", "free-text search"},
		}),
		s.newGetCmd(token, orgID, "estimates"),
		s.newCreateCmd(token, orgID, "estimates"),
	)
	item := newGroupCmd("item", "Look up items (rates, descriptions) for line items")
	item.AddCommand(
		s.newListCmd(token, orgID, "items", []stringFlag{
			{"search-text", "search_text", "free-text search"},
		}),
		s.newGetCmd(token, orgID, "items"),
	)
	bill := newGroupCmd("bill", "Read bills (payables)")
	bill.AddCommand(
		s.newListCmd(token, orgID, "bills", []stringFlag{
			{"vendor-id", "vendor_id", "filter by vendor id"},
			{"filter-by", "filter_by", "Books status view, e.g. Status.Overdue"},
			{"search-text", "search_text", "free-text search"},
		}),
		s.newGetCmd(token, orgID, "bills"),
	)
	payment := newGroupCmd("payment", "Read customer payments")
	payment.AddCommand(
		s.newListCmd(token, orgID, "customerpayments", []stringFlag{
			{"customer-id", "customer_id", "filter by customer id"},
		}),
		s.newGetCmd(token, orgID, "customerpayments"),
	)
	expense := newGroupCmd("expense", "Read and record expenses (payables)")
	expense.AddCommand(
		s.newListCmd(token, orgID, "expenses", []stringFlag{
			{"filter-by", "filter_by", "Books status view, e.g. Status.Unbilled"},
			{"search-text", "search_text", "free-text search"},
		}),
		s.newGetCmd(token, orgID, "expenses"),
		s.newCreateCmd(token, orgID, "expenses"),
	)

	root.AddCommand(
		s.newOrgCmd(token),
		contact, invoice, estimate, item, bill, payment, expense,
	)
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
