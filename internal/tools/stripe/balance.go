package stripe

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newBalanceCmd groups the account-balance reads: `get` (the current balance
// singleton) and `transactions` (the paginated settlement-activity list).
func (s *Service) newBalanceCmd(token string) *cobra.Command {
	group := newGroupCmd("balance", "Account balance and settlement activity")
	group.AddCommand(
		s.newBalanceGetCmd(token),
		s.newBalanceTransactionsCmd(token),
	)
	return group
}

// newBalanceGetCmd is GET /v1/balance — the current available/pending balance.
func (s *Service) newBalanceGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Retrieve the current account balance",
		Long: "The account's own money, split into `available` (payable out now) and\n" +
			"`pending` (still settling). Both are ARRAYS keyed by currency, so a\n" +
			"multi-currency account has several entries and no single total exists.\n" +
			"This is the standing balance, not the movements that produced it.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/balance", callOpts{})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newBalanceTransactionsCmd is GET /v1/balance_transactions — paginated recent
// settlement activity.
func (s *Service) newBalanceTransactionsCmd(token string) *cobra.Command {
	var o listOpts
	cmd := &cobra.Command{
		Use:   "transactions",
		Short: "List balance transactions (settlement activity)",
		Long: "Every movement through the Stripe balance — charges, refunds, fees, payouts\n" +
			"and adjustments — each carrying the `fee` and `net` that a charge object\n" +
			"alone does not show, plus a `source` pointing back at the object that\n" +
			"caused it. This is the reconciliation surface: `--param payout=po_123`\n" +
			"breaks a deposit down into what it settled.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := o.query()
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/balance_transactions", callOpts{query: q})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, &o)
	return cmd
}
