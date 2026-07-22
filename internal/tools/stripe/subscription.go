package stripe

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSubscriptionCmd groups subscription reporting (list/get) plus cancel — the
// one lifecycle mutation a support colleague performs. Cancel is DELETE on the
// subscription object.
func (s *Service) newSubscriptionCmd(token string) *cobra.Command {
	group := newGroupCmd("subscription", "Report and cancel subscriptions")
	group.AddCommand(
		s.newListCmd(token, "/subscriptions"),
		s.newGetByIDCmd(token, "/subscriptions"),
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
		Use:         "cancel <id>",
		Short:       "Cancel a subscription by id",
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
