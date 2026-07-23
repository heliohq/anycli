package stripe

import "github.com/spf13/cobra"

// newRefundCmd groups refund reporting (list/get) plus create — the top
// support action. Create takes --param charge=<id> or --param
// payment_intent=<id> (+ optional amount, reason) and honors --idempotency-key.
func (s *Service) newRefundCmd(token string) *cobra.Command {
	group := newGroupCmd("refund", "Report and issue refunds")
	group.AddCommand(
		s.newListCmd(token, "/refunds"),
		s.newGetByIDCmd(token, "/refunds"),
		s.newCreateCmd(token, "refund", "/refunds"),
	)
	return group
}
