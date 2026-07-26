package paypal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// maxTransactionWindow is PayPal Transaction Search's hard limit: a single
// query may span at most 31 days. We guard it locally so a too-wide range
// fails as a usage error (exit 2) with an actionable message rather than an
// opaque PayPal 400.
const maxTransactionWindow = 31 * 24 * time.Hour

// --- invoice ---------------------------------------------------------------

func (s *Service) newInvoiceListCmd(cl *client) *cobra.Command {
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List invoices (receivables), newest page first",
		Long: "Returns every invoice the account owns, drafts included, with no filter\n" +
			"beyond paging — `--page` is 1-1000 and `--page-size` 1-100, default 20.\n" +
			"To narrow by status, recipient or date use `invoice search`, which is a\n" +
			"different endpoint and not reachable from here. Each row is a summary;\n" +
			"`invoice get` returns the line items and payment history.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{
				"page":           {strconv.Itoa(page)},
				"page_size":      {strconv.Itoa(pageSize)},
				"total_required": {"true"},
			}
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v2/invoicing/invoices", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body, "items")
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (1-1000)")
	cmd.Flags().IntVar(&pageSize, "page-size", 20, "items per page (1-100)")
	return cmd
}

func (s *Service) newInvoiceGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <invoice-id>",
		Short: "Read one invoice's full detail and status",
		Long: "Takes the invoice id PayPal assigns (`INV2-…`), which comes back from\n" +
			"`invoice create-draft`, `invoice list` or `invoice search` — the\n" +
			"human-readable invoice number printed on the document is a different\n" +
			"value and is not accepted. Adds what the collection rows omit: `items`,\n" +
			"`amount` breakdown, `payments`, `refunds` and the recipient's\n" +
			"`billing_info`. `status` is the field that decides what can happen next —\n" +
			"only a DRAFT can be sent, only a SENT or PARTIALLY_PAID invoice is\n" +
			"awaiting money.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v2/invoicing/invoices/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitObject(body)
		},
	}
}

func (s *Service) newInvoiceSearchCmd(cl *client) *cobra.Command {
	var status, recipientEmail, startDate, endDate string
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search invoices by status, recipient, or invoice date range",
		Long: "The filtered counterpart to `invoice list`, and a separate PayPal\n" +
			"endpoint rather than a query on the collection. `--status` takes one\n" +
			"PayPal status at a time (DRAFT, SENT, UNPAID, PAID, PARTIALLY_PAID,\n" +
			"MARKED_AS_PAID, CANCELLED, REFUNDED), not a comma list.\n" +
			"`--start-date`/`--end-date` are plain `YYYY-MM-DD` and filter on the\n" +
			"invoice date, not the sent or paid date; `--end-date` alone is silently\n" +
			"ignored, since the range is only sent when `--start-date` is present.\n" +
			"With no flags at all it degenerates into `invoice list`.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// search-invoices is a top-level sibling of the invoices collection,
			// NOT a path under /invoices — do not derive it by appending.
			payload := map[string]any{}
			if status != "" {
				payload["status"] = []string{status}
			}
			if recipientEmail != "" {
				payload["recipient_email"] = recipientEmail
			}
			if startDate != "" {
				payload["invoice_date_range"] = map[string]string{"start": startDate, "end": endDate}
			}
			q := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}, "total_required": {"true"}}
			body, err := cl.call(cmd.Context(), http.MethodPost, "/v2/invoicing/search-invoices", q, payload)
			if err != nil {
				return err
			}
			return s.emitList(body, "items")
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "invoice status filter (e.g. SENT, PAID, UNPAID, DRAFT)")
	cmd.Flags().StringVar(&recipientEmail, "recipient-email", "", "recipient email filter")
	cmd.Flags().StringVar(&startDate, "start-date", "", "invoice date range start (YYYY-MM-DD); requires --end-date")
	cmd.Flags().StringVar(&endDate, "end-date", "", "invoice date range end (YYYY-MM-DD); requires --start-date")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().IntVar(&pageSize, "page-size", 20, "items per page (1-100)")
	return cmd
}

func (s *Service) newInvoiceCreateDraftCmd(cl *client) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "create-draft",
		Short: "Create a DRAFT invoice (safe: not emailed until `invoice send`)",
		Long: "`--body` is a full PayPal Invoicing v2 create payload, parsed locally\n" +
			"before the call so malformed JSON fails as usage without reaching PayPal.\n" +
			"A workable minimum is `detail.currency_code`, one entry in\n" +
			"`primary_recipients[].billing_info.email_address`, and `items[]` with\n" +
			"`name`, `quantity` and `unit_amount`; leave `detail.invoice_number` out\n" +
			"and PayPal assigns the next one. Keep the `id` from the response — it is\n" +
			"what `invoice get` and `invoice send` take, and nothing here lists drafts\n" +
			"by their content.",
		Args: cobra.NoArgs,
		// Writes a draft — may-mutate, so the fact is true (design 318). A draft
		// is inert until send, but it still creates a resource.
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if body == "" {
				return &usageError{msg: "invoice create-draft requires --body with the invoice JSON payload"}
			}
			var payload any
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				return &usageError{msg: fmt.Sprintf("invoice create-draft --body is not valid JSON: %v", err)}
			}
			out, err := cl.call(cmd.Context(), http.MethodPost, "/v2/invoicing/invoices", nil, payload)
			if err != nil {
				return err
			}
			return s.emitObject(out)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "invoice JSON payload (PayPal Invoicing v2 create shape)")
	return cmd
}

func (s *Service) newInvoiceSendCmd(cl *client) *cobra.Command {
	var subject, note string
	var sendToRecipient bool
	cmd := &cobra.Command{
		Use:   "send <invoice-id>",
		Short: "Send (email) a drafted invoice to its recipient",
		Long: "Only a DRAFT can be sent, and sending is one-way: the invoice moves to\n" +
			"SENT, the recipient gets a real email from PayPal, and no command here\n" +
			"cancels or recalls it. `--subject` and `--note` ride on that email.\n" +
			"`--send-to-recipient=false` marks the invoice SENT without emailing\n" +
			"anyone, which is the path to take when the payment link is to be\n" +
			"delivered some other way. The response body is thin — confirm the\n" +
			"resulting state with `invoice get` rather than reading it here.",
		Args: cobra.ExactArgs(1),
		// Sends a real invoice email — a genuine side effect.
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"send_to_recipient": sendToRecipient}
			if subject != "" {
				payload["subject"] = subject
			}
			if note != "" {
				payload["note"] = note
			}
			out, err := cl.call(cmd.Context(), http.MethodPost, "/v2/invoicing/invoices/"+url.PathEscape(args[0])+"/send", nil, payload)
			if err != nil {
				return err
			}
			return s.emitObject(out)
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "email subject override")
	cmd.Flags().StringVar(&note, "note", "", "note to the recipient")
	cmd.Flags().BoolVar(&sendToRecipient, "send-to-recipient", true, "email the recipient (false records the send without emailing)")
	return cmd
}

// --- transaction -----------------------------------------------------------

func (s *Service) newTransactionListCmd(cl *client) *cobra.Command {
	var startDate, endDate, fields string
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List transactions in a date window (max 31 days, last 3 years)",
		Long: "Both bounds are required and must be RFC3339 with a zone\n" +
			"(`2026-07-01T00:00:00Z`); the span is checked locally, so a too-wide\n" +
			"range fails as usage without spending a call. Cover a quarter by walking\n" +
			"consecutive windows and paging each one — `--page-size` is 1-500,\n" +
			"default 100. PayPal also settles this report asynchronously: the last\n" +
			"few hours of activity may be missing, so re-read a window rather than\n" +
			"treating a fresh one as final. `--fields` defaults to `all`; narrowing it\n" +
			"to `transaction_info` makes large windows markedly lighter.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if startDate == "" || endDate == "" {
				return &usageError{msg: "transaction list requires --start-date and --end-date (RFC3339, e.g. 2026-07-01T00:00:00Z)"}
			}
			if err := validateWindow(startDate, endDate); err != nil {
				return err
			}
			q := url.Values{
				"start_date": {startDate},
				"end_date":   {endDate},
				"fields":     {fields},
				"page":       {strconv.Itoa(page)},
				"page_size":  {strconv.Itoa(pageSize)},
			}
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/reporting/transactions", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body, "transaction_details")
		},
	}
	cmd.Flags().StringVar(&startDate, "start-date", "", "window start, RFC3339 (required)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "window end, RFC3339 (required); must be within 31 days of start")
	cmd.Flags().StringVar(&fields, "fields", "all", "which transaction fields to include")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "items per page (1-500)")
	return cmd
}

// validateWindow enforces the Transaction Search 31-day maximum locally. Both
// bounds must be RFC3339; a start after end, or a span over 31 days, is a usage
// error the AI can correct without a round trip.
func validateWindow(startDate, endDate string) error {
	start, err := time.Parse(time.RFC3339, startDate)
	if err != nil {
		return &usageError{msg: fmt.Sprintf("--start-date must be RFC3339 (e.g. 2026-07-01T00:00:00Z): %v", err)}
	}
	end, err := time.Parse(time.RFC3339, endDate)
	if err != nil {
		return &usageError{msg: fmt.Sprintf("--end-date must be RFC3339 (e.g. 2026-07-31T00:00:00Z): %v", err)}
	}
	if !end.After(start) {
		return &usageError{msg: "--end-date must be after --start-date"}
	}
	if end.Sub(start) > maxTransactionWindow {
		return &usageError{msg: "PayPal Transaction Search allows at most a 31-day window; narrow --start-date/--end-date"}
	}
	return nil
}

// --- balance ---------------------------------------------------------------

func (s *Service) newBalanceListCmd(cl *client) *cobra.Command {
	var asOfTime string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Read the current (or as-of) account balances snapshot",
		Long: "One entry per currency the account holds, each with `total_balance`,\n" +
			"`available_balance` and `withheld_balance` — money in a pending or\n" +
			"reserved state sits in the total but not in the available figure, so\n" +
			"quote `available_balance` when the question is what can be spent.\n" +
			"`--as-of-time` is RFC3339 and rewinds the snapshot to a past instant;\n" +
			"without it the answer is now. This is a point-in-time snapshot, not a\n" +
			"movement history — for that use `transaction list`.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if asOfTime != "" {
				q.Set("as_of_time", asOfTime)
			}
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/reporting/balances", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body, "balances")
		},
	}
	cmd.Flags().StringVar(&asOfTime, "as-of-time", "", "balance snapshot time, RFC3339 (default: now)")
	return cmd
}

// --- subscription ----------------------------------------------------------

func (s *Service) newSubscriptionGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <subscription-id>",
		Short: "Look up a subscription's status and detail",
		Long: "Takes a PayPal Billing subscription id (`I-…`). There is no command that\n" +
			"lists or searches subscriptions, so the id has to arrive from somewhere\n" +
			"else — the merchant's own records, or a transaction row. The reply\n" +
			"carries `status` (APPROVAL_PENDING, ACTIVE, SUSPENDED, CANCELLED,\n" +
			"EXPIRED) plus `billing_info` with `next_billing_time`,\n" +
			"`last_payment` and `failed_payments_count`, which is where a churn\n" +
			"question is actually answered. Read only: nothing here activates,\n" +
			"suspends or cancels a subscription.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := cl.call(cmd.Context(), http.MethodGet, "/v1/billing/subscriptions/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitObject(body)
		},
	}
}

// --- output ----------------------------------------------------------------

// emitObject writes a single PayPal resource body to stdout verbatim.
func (s *Service) emitObject(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitList normalizes a PayPal HATEOAS collection into the provider-neutral
// {results, page, total_pages, total_items} envelope: the per-endpoint
// collection key (items / transaction_details / balances) becomes results,
// PayPal's own page counters pass through when present, and the raw links array
// is dropped. Malformed JSON surfaces as an apiError rather than a silent empty
// list.
func (s *Service) emitList(body []byte, collectionKey string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return &apiError{msg: fmt.Sprintf("paypal: decode list response: %v", err), err: err}
	}
	out := map[string]any{}
	if results, ok := raw[collectionKey]; ok {
		out["results"] = results
	} else {
		out["results"] = json.RawMessage("[]")
	}
	for _, key := range []string{"page", "total_pages", "total_items"} {
		if v, ok := raw[key]; ok {
			out[key] = v
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("paypal: encode list response: %v", err), err: err}
	}
	return s.emitObject(b)
}
