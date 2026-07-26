package mercury

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newTransactionListCmd lists an account's transactions
// (GET /account/{accountId}/transactions). Mercury paginates transactions with
// limit/offset and filters by date range, status, and free-text search.
func (s *Service) newTransactionListCmd(token string) *cobra.Command {
	var account, order, start, end, search, status string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List an account's transactions (GET /account/{id}/transactions)",
		Long: "--account is the BANK account id from `account list`. Amounts are SIGNED\n" +
			"— a debit is negative, a credit positive — and there is no separate\n" +
			"direction field to consult. --status filters to pending, sent,\n" +
			"cancelled, failed, reversed or blocked, and a pending transaction has\n" +
			"not settled, so its amount can still change. --search matches the\n" +
			"description or the counterparty name.\n" +
			"\n" +
			"With --start and --end unset, Mercury returns a recent window rather\n" +
			"than all history, so any question about an older period must pass them\n" +
			"(YYYY-MM-DD or ISO 8601). This is the only offset-paged command here:\n" +
			"--limit is 1-1000 defaulting to 1000, --offset skips, and the response's\n" +
			"`total` is how to tell whether more remain.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			if account == "" {
				return &usageError{msg: "--account is required"}
			}
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			if order != "" {
				q.Set("order", order)
			}
			if start != "" {
				q.Set("start", start)
			}
			if end != "" {
				q.Set("end", end)
			}
			if search != "" {
				q.Set("search", search)
			}
			if status != "" {
				q.Set("status", status)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/account/"+url.PathEscape(account)+"/transactions", q)
			if err != nil {
				return err
			}
			return s.emitList(body, "transactions", "total")
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "account id to list transactions for (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max transactions per page (1-1000; Mercury defaults to 1000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "number of transactions to skip (pagination)")
	cmd.Flags().StringVar(&order, "order", "", "sort order: asc|desc (Mercury defaults to desc)")
	cmd.Flags().StringVar(&start, "start", "", "earliest date filter (YYYY-MM-DD or ISO 8601)")
	cmd.Flags().StringVar(&end, "end", "", "latest date filter (YYYY-MM-DD or ISO 8601)")
	cmd.Flags().StringVar(&search, "search", "", "filter by description or counterparty name")
	cmd.Flags().StringVar(&status, "status", "", "filter by status: pending|sent|cancelled|failed|reversed|blocked")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// newTransactionGetCmd fetches one transaction
// (GET /account/{accountId}/transaction/{transactionId}).
func (s *Service) newTransactionGetCmd(token string) *cobra.Command {
	var account string
	cmd := &cobra.Command{
		Use:   "get <transaction-id>",
		Short: "Get one transaction by id (GET /account/{id}/transaction/{txId})",
		Long: "A transaction id does not resolve on its own — --account must name the\n" +
			"account holding it, so an id quoted from somewhere else is unusable\n" +
			"until its account is known. The fields match a `transaction list`\n" +
			"entry, which makes this worth calling only when the id did not come\n" +
			"from a listing.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, args []string) error {
			if account == "" {
				return &usageError{msg: "--account is required"}
			}
			path := "/account/" + url.PathEscape(account) + "/transaction/" + url.PathEscape(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			return s.emitObject(body)
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "account id the transaction belongs to (required)")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}
