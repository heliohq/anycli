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
		Use:         "list",
		Short:       "List invoices for a location (GET /v2/invoices)",
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
		Args:  cobra.NoArgs,
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
		Use:         "get",
		Short:       "Retrieve an invoice (GET /v2/invoices/{invoice_id})",
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
		Use:         "create",
		Short:       "Create a draft invoice (POST /v2/invoices)",
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
		Use:         "publish",
		Short:       "Publish an invoice (POST /v2/invoices/{invoice_id}/publish)",
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
