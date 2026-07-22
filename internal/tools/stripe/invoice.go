package stripe

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newInvoiceCmd groups invoice reads plus the draft/finalize/send lifecycle an
// assistant drives when billing a customer.
func (s *Service) newInvoiceCmd(token string) *cobra.Command {
	group := newGroupCmd("invoice", "Draft, finalize, and send invoices")
	group.AddCommand(
		s.newListCmd(token, "/invoices"),
		s.newGetByIDCmd(token, "/invoices"),
		s.newCreateCmd(token, "invoice", "/invoices"),
		s.newInvoiceActionCmd(token, "finalize", "/finalize", "Finalize a draft invoice"),
		s.newInvoiceActionCmd(token, "send", "/send", "Send a finalized invoice to the customer"),
	)
	return group
}

// newInvoiceActionCmd builds a POST action verb on a single invoice
// (finalize / send), where the action is a path suffix on the object.
func (s *Service) newInvoiceActionCmd(token, use, suffix, short string) *cobra.Command {
	var o mutOpts
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := o.form()
			if err != nil {
				return err
			}
			path := "/invoices/" + url.PathEscape(args[0]) + suffix
			body, err := s.call(cmd.Context(), token, http.MethodPost, path, callOpts{form: form, idempotencyKey: o.idempotencyKey})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerMutationFlags(cmd, &o)
	return cmd
}
