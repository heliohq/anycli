package stripe

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// The invoice Longs live here because every one of these leaves comes from a
// shared constructor — list/get/create generically, finalize and send from the
// action builder below.
const (
	longInvoiceList = "Filter with `--param customer=cus_123`,\n" +
		"`--param status=draft|open|paid|uncollectible|void`, or\n" +
		"`--param subscription=sub_123`. Drafts appear here alongside real\n" +
		"invoices, so a status filter is what separates what was billed from what is\n" +
		"still being assembled."

	longInvoiceGet = "Takes an `in_` id. `status` decides what can happen next: a draft can still\n" +
		"be edited and finalized, an open invoice can be sent or paid, and paid or\n" +
		"void are terminal. `hosted_invoice_url` and `invoice_pdf` are populated only\n" +
		"after finalization."

	longInvoiceCreate = "Creates a DRAFT: nothing is owed and the customer sees nothing yet. It\n" +
		"sweeps up that customer's pending invoice items, so line items have to exist\n" +
		"before this call rather than being added after it. The lifecycle is create →\n" +
		"`invoice finalize` → `invoice send`, and only finalize makes the amount\n" +
		"real."

	longInvoiceFinalize = "Turns a draft into an open invoice: the amount becomes owed, the invoice\n" +
		"number is assigned, and the PDF and hosted URL are generated. It emails\n" +
		"nobody — that is `invoice send`. On a customer set to charge automatically,\n" +
		"finalizing is also what triggers the payment attempt. A finalized invoice\n" +
		"can no longer be edited, only voided."

	longInvoiceSend = "Emails the invoice to the customer's address on file, and only works once the\n" +
		"invoice is finalized — a draft sends nothing. Calling it again re-sends the\n" +
		"same email, which makes it the reminder verb as well."
)

// newInvoiceCmd groups invoice reads plus the draft/finalize/send lifecycle an
// assistant drives when billing a customer.
func (s *Service) newInvoiceCmd(token string) *cobra.Command {
	group := newGroupCmd("invoice", "Draft, finalize, and send invoices")
	group.AddCommand(
		s.newListCmd(token, "/invoices", longInvoiceList),
		s.newGetByIDCmd(token, "/invoices", longInvoiceGet),
		s.newCreateCmd(token, "invoice", "/invoices", longInvoiceCreate),
		s.newInvoiceActionCmd(token, "finalize", "/finalize", "Finalize a draft invoice", longInvoiceFinalize),
		s.newInvoiceActionCmd(token, "send", "/send", "Send a finalized invoice to the customer", longInvoiceSend),
	)
	return group
}

// newInvoiceActionCmd builds a POST action verb on a single invoice
// (finalize / send), where the action is a path suffix on the object.
func (s *Service) newInvoiceActionCmd(token, use, suffix, short, long string) *cobra.Command {
	var o mutOpts
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Long:        long,
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
