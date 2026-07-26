package stripe

import "github.com/spf13/cobra"

// The refund Longs sit here because list, get and create all come from shared
// constructors.
const (
	longRefundList = "Refunds across the whole account. Narrow to one payment with\n" +
		"`--param charge=ch_123` or `--param payment_intent=pi_123` to see what has\n" +
		"already been returned before issuing more."

	longRefundGet = "Takes an `re_` id. `status` is the field that matters: a refund can sit\n" +
		"pending and can still fail afterwards, so a clean `refund create` response\n" +
		"is not proof the money reached the customer."

	longRefundCreate = "Real money leaves the account and there is NO undo. Identify the payment\n" +
		"with `--param charge=ch_123` or `--param payment_intent=pi_123`; omitting\n" +
		"`amount` refunds the full remaining amount, and `--param amount=500` means\n" +
		"$5.00 because the unit is cents. Always pass --idempotency-key — a retry\n" +
		"without one issues a second refund. Read `amount_refunded` on the charge\n" +
		"first to see what has already gone back."
)

// newRefundCmd groups refund reporting (list/get) plus create — the top
// support action. Create takes --param charge=<id> or --param
// payment_intent=<id> (+ optional amount, reason) and honors --idempotency-key.
func (s *Service) newRefundCmd(token string) *cobra.Command {
	group := newGroupCmd("refund", "Report and issue refunds")
	group.AddCommand(
		s.newListCmd(token, "/refunds", longRefundList),
		s.newGetByIDCmd(token, "/refunds", longRefundGet),
		s.newCreateCmd(token, "refund", "/refunds", longRefundCreate),
	)
	return group
}
