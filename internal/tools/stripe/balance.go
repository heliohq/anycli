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
		Use:         "get",
		Short:       "Retrieve the current account balance",
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
		Use:         "transactions",
		Short:       "List balance transactions (settlement activity)",
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
