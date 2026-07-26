package pennylane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// sideEffect builds the cobra Annotations map carrying the "anycli.side_effect"
// fact for a runnable leaf (design 318): true ⇔ the command can issue a
// mutating (non-GET) provider call. Group commands carry no annotation.
func sideEffect(mayMutate bool) map[string]string {
	return map[string]string{"anycli.side_effect": strconv.FormatBool(mayMutate)}
}

// newGroupCmd is a runnable, help-only command group. cobra skips Args
// validation on non-runnable commands (help + exit 0 even for an unknown
// subcommand — a false success for an agent); making the group runnable
// restores it: a bare group shows help, an unknown subcommand fails.
func newGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// newListCmd builds a read-only list command over a collection path. It exposes
// the four v2 list query parameters (cursor / limit / filter / sort) and passes
// only the ones the caller set — a bare list stays a clean GET. The provider's
// JSON list envelope is emitted verbatim; the tool never auto-loops pages (the
// agent follows the response cursor).
func (s *Service) newListCmd(token, use, short, long, path string) *cobra.Command {
	var cursor, filter, sort string
	var limit int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if cursor != "" {
				q.Set("cursor", cursor)
			}
			if filter != "" {
				q.Set("filter", filter)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "opaque pagination cursor from a prior response")
	cmd.Flags().IntVar(&limit, "limit", 0, "items per page (1-100)")
	cmd.Flags().StringVar(&filter, "filter", "", "provider filter expression")
	cmd.Flags().StringVar(&sort, "sort", "", "sort field, e.g. -id")
	return cmd
}

// newGetCmd builds a read-only retrieve-by-id command over a collection path
// (the id is appended as prefix/{id}).
func (s *Service) newGetCmd(token, use, short, long, prefix string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := requireID(args[0])
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, prefix+"/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
}

// newBodyCmd builds a mutating command that posts a caller-supplied JSON body
// to a fixed collection path (e.g. create). --body is required and must be
// valid JSON (object or array).
func (s *Service) newBodyCmd(token, use, short, long, method, path string) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := requireJSONBody(body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, method, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "request body as JSON (required)")
	return cmd
}

// newIDBodyCmd builds a mutating command that sends a caller-supplied JSON body
// to an id-scoped sub-path (prefix/{id}suffix), e.g. transaction categorize →
// PUT /transactions/{id}/categories.
func (s *Service) newIDBodyCmd(token, use, short, long, method, prefix, suffix string) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := requireID(args[0])
			if err != nil {
				return err
			}
			payload, err := requireJSONBody(body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, method, prefix+url.PathEscape(id)+suffix, nil, payload)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "request body as JSON (required)")
	return cmd
}

// requireID trims and validates a positional id argument.
func requireID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", &usageError{msg: "empty id"}
	}
	return id, nil
}

// requireJSONBody validates the --body value: non-empty and valid JSON.
func requireJSONBody(body string) ([]byte, error) {
	if strings.TrimSpace(body) == "" {
		return nil, &usageError{msg: "--body is required and must be valid JSON"}
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--body is not valid JSON: %v", err)}
	}
	return []byte(body), nil
}

// The Long texts for every pennylane leaf. They live here, next to the shared
// list/get/body builders that consume them, because it is the call site in each
// resource constructor below that fixes the endpoint a text describes — the
// builders themselves are generic over all of them.
const (
	longCustomerList = "Returns BOTH company and individual customers — there is no type flag,\n" +
		"so narrow with `--filter` using Pennylane's own filter grammar. The id\n" +
		"returned here is what a `customer-invoice create` body references, which\n" +
		"makes this the usual first step in issuing an invoice."
	longCustomerGet = "Takes a Pennylane customer id and resolves both company and individual\n" +
		"customers, unlike `customer create`, which only makes company ones."
	longCustomerCreate = "Pennylane has no single create endpoint covering every customer type:\n" +
		"this posts to the company-customer collection, so it creates a B2B\n" +
		"COMPANY customer. Reads through `customer list` and `customer get` still\n" +
		"return individuals as well. --body is required raw JSON, minimally a\n" +
		"`name`; `emails` is where invoices for this customer are delivered."
	longSupplierList = "Suppliers are the counterparty on `supplier-invoice` (accounts payable)\n" +
		"records. They are read-only through this tool — there is no supplier\n" +
		"create or update. Page with `--cursor` and narrow with `--filter`."
	longSupplierGet = "Takes a Pennylane supplier id from `supplier list`. Read-only, like the\n" +
		"whole supplier surface here."
	longCustomerInvoiceList = "Accounts receivable: the invoices the company has ISSUED. Payment\n" +
		"status, date range and per-customer narrowing all live in `--filter` —\n" +
		"there are no typed shortcuts for them. `--sort -id` returns the most\n" +
		"recently created first."
	longCustomerInvoiceGet = "Takes a customer-invoice id from `customer-invoice list` and returns the\n" +
		"full issued document, including its line items and payment state."
	longCustomerInvoiceCreate = "Issues a REAL accounts-receivable document in the company's books. It is\n" +
		"not a draft, and this tool has no delete or void command to undo it.\n" +
		"--body is required raw JSON carrying the customer reference, dates and\n" +
		"line items: resolve the customer id with `customer list` and the\n" +
		"line-item products with `product list` before composing it."
	longSupplierInvoiceList = "Accounts payable: the bills the company has RECEIVED, which is the read\n" +
		"behind \"what is unpaid or unvalidated\". Status and date narrowing go\n" +
		"through `--filter`. Supplier invoices are created by Pennylane's own\n" +
		"ingestion, not from here, so this surface is read-only."
	longSupplierInvoiceGet = "Takes a supplier-invoice id from `supplier-invoice list`. Read-only,\n" +
		"like the rest of the accounts-payable surface."
	longProductList = "Products are the catalogue that invoice line items reference, so this is\n" +
		"the lookup to run before composing a `customer-invoice create` body.\n" +
		"Read-only through this tool — products are maintained in Pennylane."
	longProductGet = "Takes a product id from `product list` and returns the single catalogue\n" +
		"entry an invoice line item points at."
	longTransactionList = "Bank transactions imported into Pennylane, and the input to `transaction\n" +
		"categorize`. Date range, bank account and \"still uncategorized\"\n" +
		"narrowing all live in `--filter`; `--sort -id` puts the most recently\n" +
		"imported first."
	longTransactionGet = "Takes a bank-transaction id from `transaction list` and returns its\n" +
		"current category assignments. Read it before `transaction categorize`,\n" +
		"which replaces those assignments rather than adding to them."
	longTransactionCategorize = "--body is a JSON ARRAY, not an object: entries of\n" +
		"{\"id\":<category-id>,\"weight\":\"<w>\"}, and the weights within one group\n" +
		"must sum to 1 — a 50/50 split is two entries at \"0.5\". A single category\n" +
		"is still an array of one. This is a PUT, so the array supplied becomes\n" +
		"the transaction's complete set of assignments and any existing split is\n" +
		"overwritten. It mutates the company's books."
	longLedgerTrialBalance = "Every ledger account with its debit and credit totals. It shares the\n" +
		"list flags, so `--cursor` pages and `--filter` narrows using Pennylane's\n" +
		"grammar — a period-scoped balance is expressed as a filter, not a\n" +
		"dedicated date flag."
	longLedgerEntries = "The line-level detail behind the trial balance, and by far the largest\n" +
		"of the ledger reads. Narrow with `--filter` before pulling: `--limit`\n" +
		"tops out at 100, so an unfiltered year of entries is a long cursor loop."
	longLedgerJournals = "The books entries are posted into — sales, purchases, bank and so on.\n" +
		"Small and slow-changing, which makes it the cheap lookup for turning a\n" +
		"journal code seen on a ledger entry into a name."
	longLedgerAccounts = "The chart of accounts: every ledger account number and label the company\n" +
		"posts to. Use it to resolve an account number appearing on a ledger\n" +
		"entry or a trial-balance row into something human-readable."
)

// newCustomerCmd: customer reads use GET /customers[/id] (company + individual),
// but create has no POST /customers — creation is split by type, and we wrap the
// B2B-invoicing default POST /company_customers.
func (s *Service) newCustomerCmd(token string) *cobra.Command {
	g := newGroupCmd("customer", "List, retrieve, and create customers")
	g.AddCommand(
		s.newListCmd(token, "list", "List customers (company + individual)", longCustomerList, "/customers"),
		s.newGetCmd(token, "get <id>", "Retrieve a customer by id (company + individual)", longCustomerGet, "/customers"),
		s.newBodyCmd(token, "create", "Create a company customer (POST /company_customers)", longCustomerCreate, http.MethodPost, "/company_customers"),
	)
	return g
}

func (s *Service) newSupplierCmd(token string) *cobra.Command {
	g := newGroupCmd("supplier", "List and retrieve suppliers")
	g.AddCommand(
		s.newListCmd(token, "list", "List suppliers", longSupplierList, "/suppliers"),
		s.newGetCmd(token, "get <id>", "Retrieve a supplier by id", longSupplierGet, "/suppliers"),
	)
	return g
}

func (s *Service) newCustomerInvoiceCmd(token string) *cobra.Command {
	g := newGroupCmd("customer-invoice", "List, retrieve, and issue customer (AR) invoices")
	g.AddCommand(
		s.newListCmd(token, "list", "List customer invoices", longCustomerInvoiceList, "/customer_invoices"),
		s.newGetCmd(token, "get <id>", "Retrieve a customer invoice by id", longCustomerInvoiceGet, "/customer_invoices"),
		s.newBodyCmd(token, "create", "Create a customer invoice", longCustomerInvoiceCreate, http.MethodPost, "/customer_invoices"),
	)
	return g
}

func (s *Service) newSupplierInvoiceCmd(token string) *cobra.Command {
	g := newGroupCmd("supplier-invoice", "List and retrieve supplier (AP) invoices")
	g.AddCommand(
		s.newListCmd(token, "list", "List supplier invoices", longSupplierInvoiceList, "/supplier_invoices"),
		s.newGetCmd(token, "get <id>", "Retrieve a supplier invoice by id", longSupplierInvoiceGet, "/supplier_invoices"),
	)
	return g
}

func (s *Service) newProductCmd(token string) *cobra.Command {
	g := newGroupCmd("product", "List and retrieve products")
	g.AddCommand(
		s.newListCmd(token, "list", "List products", longProductList, "/products"),
		s.newGetCmd(token, "get <id>", "Retrieve a product by id", longProductGet, "/products"),
	)
	return g
}

// newTransactionCmd: categorize maps to PUT /transactions/{id}/categories, whose
// body is a JSON array of {id, weight} category assignments.
func (s *Service) newTransactionCmd(token string) *cobra.Command {
	g := newGroupCmd("transaction", "List, retrieve, and categorize bank transactions")
	g.AddCommand(
		s.newListCmd(token, "list", "List bank transactions", longTransactionList, "/transactions"),
		s.newGetCmd(token, "get <id>", "Retrieve a bank transaction by id", longTransactionGet, "/transactions"),
		s.newIDBodyCmd(token, "categorize <id>", "Categorize a bank transaction (PUT /transactions/{id}/categories)", longTransactionCategorize, http.MethodPut, "/transactions/", "/categories"),
	)
	return g
}

// newLedgerCmd wraps the four read-only accounting-report endpoints, each behind
// its own granular readonly scope.
func (s *Service) newLedgerCmd(token string) *cobra.Command {
	g := newGroupCmd("ledger", "Read accounting reports (trial balance, ledger entries, journals, accounts)")
	g.AddCommand(
		s.newListCmd(token, "trial-balance", "Get the trial balance", longLedgerTrialBalance, "/trial_balance"),
		s.newListCmd(token, "entries", "List ledger entries", longLedgerEntries, "/ledger_entries"),
		s.newListCmd(token, "journals", "List journals", longLedgerJournals, "/journals"),
		s.newListCmd(token, "accounts", "List ledger accounts", longLedgerAccounts, "/ledger_accounts"),
	)
	return g
}
