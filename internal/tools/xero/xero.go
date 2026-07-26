// Package xero is the built-in Xero service: a resource-grouped cobra tree over
// the Xero Accounting API (https://api.xero.com/api.xro/2.0). A Xero access
// token is scoped to a user, not one organisation — one authorization can act
// on many tenants — so which organisation a call targets is chosen per request
// via the Xero-Tenant-Id header, resolved from GET https://api.xero.com/connections.
// This service resolves the tenant itself (single-org is invisible; multi-org
// asks for --tenant) and emits Xero's JSON responses verbatim.
package xero

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

// DefaultBaseURL is the Xero API host. The Accounting API lives under
// /api.xro/2.0 and tenant discovery under /connections on the same host.
const DefaultBaseURL = "https://api.xero.com"

// EnvToken is the env var the credential binding injects (definitions/tools/xero.json).
const EnvToken = "XERO_ACCESS_TOKEN"

// EnvTenant is an optional default tenant selector. It is not a credential; when
// present in the injected env it seeds --tenant so a single-tenant deployment
// can skip the /connections round trip. --tenant always wins over it.
const EnvTenant = "XERO_TENANT_ID"

// Service implements the built-in Xero tool. It satisfies tools.Service by duck
// typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the Xero host; empty = DefaultBaseURL. Tests point it at
	// an httptest server serving both /connections and /api.xro/2.0/*.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one xero subcommand with the resolved credentials in env. Exit 0
// success; usage/param errors (bad flags, unknown subcommand, ambiguous tenant)
// exit 2; runtime/API errors (Xero non-2xx, transport failure) exit 1. Errors
// render to stderr — JSON envelope under --json, plain text otherwise.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	token := env[EnvToken]
	if token == "" {
		s.renderError(hasJSONArg(args), &usageError{msg: EnvToken + " is not set"})
		return execution.Result{ExitCode: 1}, nil
	}
	root := s.newRoot(token, env[EnvTenant])
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return execution.Failure(err), nil
	}
	return execution.Result{ExitCode: 2}, nil
}

// hasJSONArg reports whether the raw args carry --json, used to pick the error
// format before cobra has parsed flags (the pre-parse missing-token check).
func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"tool":"xero","code":"usage|api_error","status":<HTTP>,"message":…,"details":<xero body>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"tool": "xero", "code": "usage", "message": err.Error()}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["code"] = "api_error"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
		if len(apiErr.details) > 0 {
			payload["details"] = apiErr.details
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

// newGroupCmd is a runnable command group: a bare group prints help, an unknown
// subcommand fails (cobra skips Args validation on non-runnable commands, which
// would let an unknown subcommand exit 0 — a false success for an agent).
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// newRoot builds the resource-grouped cobra tree. defaultTenant seeds --tenant.
func (s *Service) newRoot(token, defaultTenant string) *cobra.Command {
	root := &cobra.Command{
		Use:   "xero",
		Short: "Xero accounting (invoices, bills, contacts, payments, accounts, reports)",
		Long: "Calls the Xero Accounting API and emits Xero's JSON verbatim. Every\n" +
			"response is the PascalCase envelope, single reads included: `contact get`\n" +
			"returns `{\"Contacts\":[{…}]}`, a one-element array, never a bare object.\n" +
			"\n" +
			"One Xero login can act on MANY organisations (Xero calls them tenants), and\n" +
			"a token is scoped to the login rather than to one organisation, so every\n" +
			"accounting call needs a target. With one organisation connected it is\n" +
			"chosen automatically and invisibly. With more than one the command exits 2\n" +
			"and prints each organisation's name and id — retry with `--tenant`, which\n" +
			"takes either the tenantId GUID or an organisation name matched\n" +
			"case-insensitively (an unknown or ambiguous name also exits 2). A GUID is\n" +
			"used directly; a name costs an extra lookup call first. `connections`\n" +
			"lists the candidates at any time.\n" +
			"\n" +
			"Reads take repeatable `--query key=value`, passed straight through as Xero\n" +
			"query parameters (`where`, `order`, `page`, `Statuses`, `summaryOnly`, …).\n" +
			"Xero's `where` is its own filter syntax, not SQL: string tests read\n" +
			"`Name.Contains(\"Acme\")` or `Status==\"AUTHORISED\"`, dates read\n" +
			"`Date>=DateTime(2026,01,01)`. Paging is per-endpoint — the transactional\n" +
			"collections page at 100 rows, while the reference lists (`account list`,\n" +
			"`item list`, `tax-rate list`) return everything at once.\n" +
			"\n" +
			"Writes invert the usual verbs: `create` is PUT and `update` is POST, and\n" +
			"both take a complete Xero envelope through `--data` or `--file`, never a\n" +
			"bare object. Because the envelope holds an array, one call can write\n" +
			"several records. An update is matched by the record's own id — a POST that\n" +
			"omits it CREATES a new record instead of failing, which is the most\n" +
			"expensive mistake available here.\n" +
			"\n" +
			"Exit codes are 0 success, 1 for a Xero or transport failure, 2 for usage\n" +
			"(bad flags, unknown subcommand, unresolved organisation). Under `--json`\n" +
			"failures render as\n" +
			"`{\"error\":{\"tool\":\"xero\",\"code\":…,\"status\":…,\"details\":<xero body>}}`, so a\n" +
			"ValidationException's per-field messages survive.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())

	pf := root.PersistentFlags()
	pf.Bool("json", false, "force structured JSON output/errors")
	pf.String("tenant", "", "target Xero organisation by tenantId (GUID) or name; omit when only one org is connected")

	rc := &resourceCtx{svc: s, token: token, defaultTenant: defaultTenant}

	contact := newGroupCmd("contact", "Manage contacts (customers & suppliers)")
	contact.AddCommand(
		rc.listCmd("list", "List contacts", longContactList, "/Contacts"),
		rc.getCmd("get", "Get a contact by id", longContactGet, "/Contacts"),
		rc.writeCmd("create", "Create contacts", longContactCreate, http.MethodPut, "/Contacts"),
		rc.writeCmd("update", "Update contacts", longContactUpdate, http.MethodPost, "/Contacts"),
	)

	invoice := newGroupCmd("invoice", "Manage sales invoices & bills")
	invoice.AddCommand(
		rc.listCmd("list", "List invoices", longInvoiceList, "/Invoices"),
		rc.getCmd("get", "Get an invoice by id or number", longInvoiceGet, "/Invoices"),
		rc.writeCmd("create", "Create invoices", longInvoiceCreate, http.MethodPut, "/Invoices"),
		rc.writeCmd("update", "Update invoices", longInvoiceUpdate, http.MethodPost, "/Invoices"),
		rc.emailCmd(),
	)

	payment := newGroupCmd("payment", "Manage payments")
	payment.AddCommand(
		rc.listCmd("list", "List payments", longPaymentList, "/Payments"),
		rc.getCmd("get", "Get a payment by id", longPaymentGet, "/Payments"),
		rc.writeCmd("create", "Create payments", longPaymentCreate, http.MethodPut, "/Payments"),
	)

	bankTxn := newGroupCmd("bank-transaction", "Manage bank transactions")
	bankTxn.AddCommand(
		rc.listCmd("list", "List bank transactions", longBankTxnList, "/BankTransactions"),
		rc.getCmd("get", "Get a bank transaction by id", longBankTxnGet, "/BankTransactions"),
		rc.writeCmd("create", "Create bank transactions", longBankTxnCreate, http.MethodPut, "/BankTransactions"),
	)

	account := newGroupCmd("account", "Chart of accounts")
	account.AddCommand(rc.listCmd("list", "List accounts", longAccountList, "/Accounts"))

	item := newGroupCmd("item", "Manage items (products & services)")
	item.AddCommand(
		rc.listCmd("list", "List items", longItemList, "/Items"),
		rc.getCmd("get", "Get an item by id", longItemGet, "/Items"),
		rc.writeCmd("create", "Create items", longItemCreate, http.MethodPut, "/Items"),
		rc.writeCmd("update", "Update items", longItemUpdate, http.MethodPost, "/Items"),
	)

	taxRate := newGroupCmd("tax-rate", "Tax rates")
	taxRate.AddCommand(rc.listCmd("list", "List tax rates", longTaxRateList, "/TaxRates"))

	organisation := newGroupCmd("organisation", "Organisation details")
	organisation.AddCommand(rc.orgGetCmd())

	root.AddCommand(
		rc.connectionsCmd(),
		organisation,
		contact,
		invoice,
		payment,
		bankTxn,
		account,
		item,
		taxRate,
		rc.reportCmd(),
		rc.fetchCmd(),
	)
	return root
}

// NewCommandTree returns the tree built with an empty token for dry-run parsing
// and traversal (tools.Service seam, design 318). The token is only captured by
// RunE closures, which are never run on this tree.
func (s *Service) NewCommandTree() *cobra.Command { return s.newRoot("", "") }
