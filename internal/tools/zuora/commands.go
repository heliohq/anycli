package zuora

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// --- account ---------------------------------------------------------------

func (s *Service) newAccountGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <account-key>",
		Short:       "Read one account (account number or id): balance, currency, bill-to/sold-to",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/accounts/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newAccountSummaryCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "summary <account-key>",
		Short:       "Rolled-up account view: subscriptions + recent invoices/payments/usage (one look at a customer)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/accounts/"+url.PathEscape(args[0])+"/summary", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// --- subscription ----------------------------------------------------------

func (s *Service) newSubscriptionListCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "list <account-key>",
		Short:       "List all subscriptions for an account (account number or id)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/subscriptions/accounts/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newSubscriptionGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <subscription-key>",
		Short:       "Read one subscription (number or id): rate plans, charges, term",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/subscriptions/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// --- invoice ---------------------------------------------------------------

func (s *Service) newInvoiceGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <invoice-id>",
		Short:       "Read one invoice: amount, balance, status, due date",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/invoices/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newInvoiceListCmd(cl *client) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list <account-key>",
		Short: "List an account's invoices via ZOQL (no first-class list-by-account GET)",
		Args:  cobra.ExactArgs(1),
		// Read-only ZOQL over the Invoice object; the account key is bound as a
		// quoted literal, never string-concatenated into the SELECT verbs.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			zoql := "select Id, InvoiceNumber, AccountId, Amount, Balance, Status, DueDate, InvoiceDate from Invoice where AccountId = " + zoqlLiteral(args[0])
			if limit > 0 {
				zoql += " limit " + strconv.Itoa(limit)
			}
			body, err := cl.call(cmd.Context(), http.MethodPost, "/v1/action/query", nil, map[string]any{"queryString": zoql})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows to return (ZOQL limit; 0 = server default)")
	return cmd
}

// --- payment ---------------------------------------------------------------

func (s *Service) newPaymentGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <payment-id>",
		Short:       "Read one payment (requires the tenant's Invoice Settlement feature)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/payments/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newPaymentListCmd(cl *client) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list <account-key>",
		Short: "List an account's payments via ZOQL (no Invoice Settlement dependency)",
		Args:  cobra.ExactArgs(1),
		// ZOQL over the Payment object works whether or not Invoice Settlement
		// is enabled — the fallback to the settlement-gated GET /v1/payments.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			zoql := "select Id, AccountId, Amount, Status, EffectiveDate, PaymentNumber from Payment where AccountId = " + zoqlLiteral(args[0])
			if limit > 0 {
				zoql += " limit " + strconv.Itoa(limit)
			}
			body, err := cl.call(cmd.Context(), http.MethodPost, "/v1/action/query", nil, map[string]any{"queryString": zoql})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows to return (ZOQL limit; 0 = server default)")
	return cmd
}

// --- catalog ---------------------------------------------------------------

func (s *Service) newCatalogProductsCmd(cl *client) *cobra.Command {
	var page, pageSize int
	cmd := &cobra.Command{
		Use:         "products",
		Short:       "List the product catalog + rate plans (also the cheapest authenticated smoke read)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{
				"page":     {strconv.Itoa(page)},
				"pageSize": {strconv.Itoa(pageSize)},
			}
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/catalog/products", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 20, "products per page")
	return cmd
}

// --- query -----------------------------------------------------------------

func (s *Service) newQueryCmd(cl *client) *cobra.Command {
	var zoql string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Run a read-only ZOQL query (POST /v1/action/query) over any queryable object",
		Args:  cobra.NoArgs,
		// ZOQL is read-only (select … from …); the raw string is the AI's, so
		// guard against accidental write verbs before sending.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			trimmed := strings.TrimSpace(zoql)
			if trimmed == "" {
				return &usageError{msg: "query requires --zoql with a ZOQL SELECT statement"}
			}
			if !strings.HasPrefix(strings.ToLower(trimmed), "select") {
				return &usageError{msg: "query --zoql must be a read-only ZOQL SELECT statement"}
			}
			body, err := cl.call(cmd.Context(), http.MethodPost, "/v1/action/query", nil, map[string]any{"queryString": trimmed})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&zoql, "zoql", "", "ZOQL SELECT statement, e.g. \"select Id, Name from Account where Status = 'Active'\"")
	return cmd
}

// --- helpers ---------------------------------------------------------------

// emit writes a Zuora response body to stdout verbatim (Zuora's JSON is already
// the useful shape; the service does not re-wrap it), with a trailing newline.
func (s *Service) emit(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// zoqlLiteral renders a value as a single-quoted ZOQL string literal, escaping
// embedded single quotes by doubling them (ZOQL/SOQL convention) so a caller's
// account key cannot break out of the quoted literal in the bound where-clause.
func zoqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
