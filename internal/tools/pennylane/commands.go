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
func (s *Service) newListCmd(token, use, short, path string) *cobra.Command {
	var cursor, filter, sort string
	var limit int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
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
func (s *Service) newGetCmd(token, use, short, prefix string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
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
func (s *Service) newBodyCmd(token, use, short, method, path string) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
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
func (s *Service) newIDBodyCmd(token, use, short, method, prefix, suffix string) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
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

// newCustomerCmd: customer reads use GET /customers[/id] (company + individual),
// but create has no POST /customers — creation is split by type, and we wrap the
// B2B-invoicing default POST /company_customers.
func (s *Service) newCustomerCmd(token string) *cobra.Command {
	g := newGroupCmd("customer", "List, retrieve, and create customers")
	g.AddCommand(
		s.newListCmd(token, "list", "List customers (company + individual)", "/customers"),
		s.newGetCmd(token, "get <id>", "Retrieve a customer by id (company + individual)", "/customers"),
		s.newBodyCmd(token, "create", "Create a company customer (POST /company_customers)", http.MethodPost, "/company_customers"),
	)
	return g
}

func (s *Service) newSupplierCmd(token string) *cobra.Command {
	g := newGroupCmd("supplier", "List and retrieve suppliers")
	g.AddCommand(
		s.newListCmd(token, "list", "List suppliers", "/suppliers"),
		s.newGetCmd(token, "get <id>", "Retrieve a supplier by id", "/suppliers"),
	)
	return g
}

func (s *Service) newCustomerInvoiceCmd(token string) *cobra.Command {
	g := newGroupCmd("customer-invoice", "List, retrieve, and issue customer (AR) invoices")
	g.AddCommand(
		s.newListCmd(token, "list", "List customer invoices", "/customer_invoices"),
		s.newGetCmd(token, "get <id>", "Retrieve a customer invoice by id", "/customer_invoices"),
		s.newBodyCmd(token, "create", "Create a customer invoice", http.MethodPost, "/customer_invoices"),
	)
	return g
}

func (s *Service) newSupplierInvoiceCmd(token string) *cobra.Command {
	g := newGroupCmd("supplier-invoice", "List and retrieve supplier (AP) invoices")
	g.AddCommand(
		s.newListCmd(token, "list", "List supplier invoices", "/supplier_invoices"),
		s.newGetCmd(token, "get <id>", "Retrieve a supplier invoice by id", "/supplier_invoices"),
	)
	return g
}

func (s *Service) newProductCmd(token string) *cobra.Command {
	g := newGroupCmd("product", "List and retrieve products")
	g.AddCommand(
		s.newListCmd(token, "list", "List products", "/products"),
		s.newGetCmd(token, "get <id>", "Retrieve a product by id", "/products"),
	)
	return g
}

// newTransactionCmd: categorize maps to PUT /transactions/{id}/categories, whose
// body is a JSON array of {id, weight} category assignments.
func (s *Service) newTransactionCmd(token string) *cobra.Command {
	g := newGroupCmd("transaction", "List, retrieve, and categorize bank transactions")
	g.AddCommand(
		s.newListCmd(token, "list", "List bank transactions", "/transactions"),
		s.newGetCmd(token, "get <id>", "Retrieve a bank transaction by id", "/transactions"),
		s.newIDBodyCmd(token, "categorize <id>", "Categorize a bank transaction (PUT /transactions/{id}/categories)", http.MethodPut, "/transactions/", "/categories"),
	)
	return g
}

// newLedgerCmd wraps the four read-only accounting-report endpoints, each behind
// its own granular readonly scope.
func (s *Service) newLedgerCmd(token string) *cobra.Command {
	g := newGroupCmd("ledger", "Read accounting reports (trial balance, ledger entries, journals, accounts)")
	g.AddCommand(
		s.newListCmd(token, "trial-balance", "Get the trial balance", "/trial_balance"),
		s.newListCmd(token, "entries", "List ledger entries", "/ledger_entries"),
		s.newListCmd(token, "journals", "List journals", "/journals"),
		s.newListCmd(token, "accounts", "List ledger accounts", "/ledger_accounts"),
	)
	return g
}
