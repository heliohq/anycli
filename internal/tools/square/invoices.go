package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newInvoiceListCmd(token string) *cobra.Command {
	var locationID, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List invoices for a location (GET /v2/invoices)",
		Long: "`--location-id` is REQUIRED and enforced locally before any request goes out:\n" +
			"Square's ListInvoices is location-scoped and has no seller-wide form, so run\n" +
			"`location list` first. Page with `--cursor` and cap the page with `--limit`.\n" +
			"To span several locations or filter by customer, `invoice search` is the\n" +
			"command.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("location_id", locationID)
			setNonEmpty(q, "cursor", cursor)
			if limit > 0 {
				q.Set("limit", intToString(limit))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/invoices", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&locationID, "location-id", "", "location id (required by Square)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page")
	_ = cmd.MarkFlagRequired("location-id")
	return cmd
}

func (s *Service) newInvoiceSearchCmd(token string) *cobra.Command {
	var bodyJSON string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search invoices (POST /v2/invoices/search)",
		Long: "POST, but a read. `--body` is required and its `query.filter` must still carry\n" +
			"`location_ids` — this is not a way around the location scoping, it is the way\n" +
			"to cover SEVERAL locations, or to filter by customer, in one call. Sorting,\n" +
			"`limit` and `cursor` all live inside the body.",
		Args: cobra.NoArgs,
		// POST /v2/invoices/search is a documented lookup; never mutates.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/invoices/search", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "SearchInvoices request body as raw JSON (query with location_ids filter, limit, cursor)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newInvoiceGetCmd(token string) *cobra.Command {
	var invoiceID string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Retrieve an invoice (GET /v2/invoices/{invoice_id})",
		Long: "`--invoice-id` is required. Returns the invoice's `status` — DRAFT, UNPAID,\n" +
			"SCHEDULED, PARTIALLY_PAID, PAID, CANCELED — and, critically, its `version`.\n" +
			"`invoice publish` demands that exact version number in its body, and a stale\n" +
			"one is rejected, so read it here immediately before publishing.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/invoices/"+url.PathEscape(invoiceID), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&invoiceID, "invoice-id", "", "invoice id")
	_ = cmd.MarkFlagRequired("invoice-id")
	return cmd
}

func (s *Service) newInvoiceCreateCmd(token string) *cobra.Command {
	var bodyJSON string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a draft invoice (POST /v2/invoices)",
		Long: "Creates a DRAFT: nothing is sent to the customer and no money is requested\n" +
			"until `invoice publish` is called on it. `--body` is required and holds the\n" +
			"`invoice` object plus an `idempotency_key` UUID, without which a retry leaves\n" +
			"a second draft behind. The invoice's location and its customer are fixed at\n" +
			"this point.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST creates a draft invoice
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/invoices", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "CreateInvoice request body as raw JSON (invoice object + idempotency_key)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newInvoicePublishCmd(token string) *cobra.Command {
	var invoiceID, bodyJSON string
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish an invoice (POST /v2/invoices/{invoice_id}/publish)",
		Long: "The outward step: Square delivers the invoice to the customer by whatever\n" +
			"method the invoice specifies and starts its payment schedule. Both\n" +
			"`--invoice-id` and `--body` are required, and the body must carry the\n" +
			"invoice's CURRENT `version` from `invoice get` plus an `idempotency_key`; a\n" +
			"stale version is refused rather than forced. There is no cancel verb here, so\n" +
			"nothing in this tool retracts a published invoice.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST sends/publishes the invoice
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/invoices/"+url.PathEscape(invoiceID)+"/publish", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&invoiceID, "invoice-id", "", "invoice id")
	cmd.Flags().StringVar(&bodyJSON, "body", "", "PublishInvoice request body as raw JSON (version + idempotency_key)")
	_ = cmd.MarkFlagRequired("invoice-id")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}
