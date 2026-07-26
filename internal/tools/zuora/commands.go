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
		Use:   "get <account-key>",
		Short: "Read one account (account number or id): balance, currency, bill-to/sold-to",
		Long: "Returns the account record itself — balance, currency, payment terms,\n" +
			"bill-to and sold-to contacts — and nothing about its activity. When the\n" +
			"question is what is going on with a customer, `account summary` answers\n" +
			"it in one call instead of four. The `id` in this response is the\n" +
			"internal account id that `invoice list` and `payment list` require.",
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
		Use:   "summary <account-key>",
		Short: "Rolled-up account view: subscriptions + recent invoices/payments/usage (one look at a customer)",
		Long: "One call returning the account with its subscriptions and only the most\n" +
			"RECENT invoices, payments and usage — Zuora truncates each of those\n" +
			"lists, so this answers the state of a customer but never \"every invoice\n" +
			"ever\". Full history needs `invoice list` or `payment list`.",
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
		Use:   "list <account-key>",
		Short: "List all subscriptions for an account (account number or id)",
		Long: "Takes the ACCOUNT key, not a subscription key. Subscriptions come back\n" +
			"in every state, cancelled and expired included, so read each `status`\n" +
			"before describing a customer as active. Charge-level detail is not here\n" +
			"— follow up with `subscription get` on the one that matters.",
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
		Use:   "get <subscription-key>",
		Short: "Read one subscription (number or id): rate plans, charges, term",
		Long: "The rate plans, their individual charges and the term dates come back\n" +
			"here, and that is where the contracted price actually lives — an\n" +
			"account's balance says what is owed, not what the customer committed to.\n" +
			"Charges carry their own effective dates, so a plan listed on the\n" +
			"subscription is not necessarily billing today.",
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
		Use:   "get <invoice-id>",
		Short: "Read one invoice: amount, balance, status, due date",
		Long: "`amount` is what was billed and `balance` is what remains unpaid, so a\n" +
			"posted invoice with a zero balance is settled and one with a positive\n" +
			"balance is outstanding regardless of its due date. Take the id from\n" +
			"`invoice list`; the invoice number is a different string.",
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
		Long: "The argument is matched against AccountId, so it must be the INTERNAL\n" +
			"account id from `account get` — an account number produces an empty\n" +
			"result set, not an error, which is easy to misread as \"this customer\n" +
			"has no invoices\". The selected fields are fixed (id, number, amount,\n" +
			"balance, status, due and invoice dates); anything else means writing the\n" +
			"select by hand with `query --zoql`. --limit 0 leaves Zuora's own row cap\n" +
			"in place.",
		Args: cobra.ExactArgs(1),
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
		Use:   "get <payment-id>",
		Short: "Read one payment (requires the tenant's Invoice Settlement feature)",
		Long: "Rides the settlement-era payments endpoint, which errors outright on a\n" +
			"tenant that never enabled Invoice Settlement — a failure here is a\n" +
			"tenant feature gap, not a bad id or a permissions problem. On such a\n" +
			"tenant read the same record with\n" +
			"`query --zoql \"select ... from Payment where Id = '...'\"`, which has no\n" +
			"such dependency.",
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
		Long: "Deliberately routed over the Payment object instead of the payments\n" +
			"endpoint, so it works on tenants where `payment get` cannot. The\n" +
			"argument is matched against AccountId and must be the INTERNAL account\n" +
			"id from `account get`; an account number returns zero rows rather than\n" +
			"an error. --limit 0 leaves Zuora's own row cap in place.",
		Args: cobra.ExactArgs(1),
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
		Use:   "products",
		Short: "List the product catalog + rate plans (also the cheapest authenticated smoke read)",
		Long: "The price book — products with their rate plans and charges — not\n" +
			"anything a customer has bought. --page is 1-based and --page-size\n" +
			"defaults to 20, so a real catalog spans several pages and a single call\n" +
			"is rarely the whole list. Effective dates on a rate plan decide whether\n" +
			"it can be sold today.",
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
		Long: "--zoql must begin with select; anything else is refused before the\n" +
			"request leaves, so this cannot mutate the tenant. ZOQL is not SQL: this\n" +
			"endpoint supports no joins and no aggregate functions, so counting or\n" +
			"summing means selecting the rows and doing it afterwards. Objects are\n" +
			"addressed by their API field names — AccountId, InvoiceNumber,\n" +
			"EffectiveDate — which are not always the names the REST responses print.\n" +
			"Zuora caps the returned rows and paginates the rest.",
		Args: cobra.NoArgs,
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
