package stripe

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// The subscription list/get Longs sit here because both leaves come from the
// shared list/get constructors.
const (
	longSubscriptionList = "Filter with `--param customer=cus_123`. The status filter matters more than\n" +
		"it looks: by default Stripe omits canceled subscriptions, so\n" +
		"`--param status=all` is required to see the full history rather than just\n" +
		"live ones. Each entry's `items` name the prices actually billed."

	longSubscriptionGet = "Takes a `sub_` id. `status`, `current_period_end` and `cancel_at_period_end`\n" +
		"are the three fields that answer whether the customer still has access and\n" +
		"until when."
)

// newSubscriptionCmd groups subscription reporting (list/get) plus cancel — the
// one lifecycle mutation a support colleague performs. Cancel is DELETE on the
// subscription object.
func (s *Service) newSubscriptionCmd(token string) *cobra.Command {
	group := newGroupCmd("subscription", "Report and cancel subscriptions")
	group.AddCommand(
		s.newListCmd(token, "/subscriptions", longSubscriptionList),
		s.newGetByIDCmd(token, "/subscriptions", longSubscriptionGet),
		s.newSubscriptionCancelCmd(token),
	)
	return group
}

// newSubscriptionCancelCmd is DELETE /v1/subscriptions/:id. Optional --param
// entries (e.g. invoice_now, prorate) pass through as the form body Stripe
// reads on cancel.
func (s *Service) newSubscriptionCancelCmd(token string) *cobra.Command {
	var o mutOpts
	cmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a subscription by id",
		Long: "Cancels IMMEDIATELY: billing stops now and, by default, nothing already\n" +
			"charged is prorated or refunded. Letting access run to the end of the paid\n" +
			"period instead means setting `cancel_at_period_end`, which no verb in this\n" +
			"tool exposes. `--param prorate=true` and `--param invoice_now=true` change\n" +
			"what happens to the unbilled remainder, and a refund of what was already\n" +
			"collected is a separate `refund create`.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := o.form()
			if err != nil {
				return err
			}
			path := "/subscriptions/" + url.PathEscape(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodDelete, path, callOpts{form: form, idempotencyKey: o.idempotencyKey})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerMutationFlags(cmd, &o)
	return cmd
}
