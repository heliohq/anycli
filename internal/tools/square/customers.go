package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCustomerListCmd(token string) *cobra.Command {
	var cursor, sortField, sortOrder string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List customers (GET /v2/customers)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setNonEmpty(q, "cursor", cursor)
			setNonEmpty(q, "sort_field", sortField)
			setNonEmpty(q, "sort_order", sortOrder)
			if limit > 0 {
				q.Set("limit", intToString(limit))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/customers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().StringVar(&sortField, "sort-field", "", "DEFAULT or CREATED_AT")
	cmd.Flags().StringVar(&sortOrder, "sort-order", "", "ASC or DESC")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page")
	return cmd
}

func (s *Service) newCustomerSearchCmd(token string) *cobra.Command {
	var bodyJSON string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search customers (POST /v2/customers/search)",
		Args:  cobra.NoArgs,
		// POST /v2/customers/search is a documented lookup; never mutates.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/customers/search", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "SearchCustomers request body as raw JSON (query, limit, cursor)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newCustomerGetCmd(token string) *cobra.Command {
	var customerID string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Retrieve a customer (GET /v2/customers/{customer_id})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/customers/"+url.PathEscape(customerID), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "customer id")
	_ = cmd.MarkFlagRequired("customer-id")
	return cmd
}

func (s *Service) newCustomerCreateCmd(token string) *cobra.Command {
	var bodyJSON string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a customer (POST /v2/customers)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST creates a profile
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/customers", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "CreateCustomer request body as raw JSON (given_name, email_address, …)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newCustomerUpdateCmd(token string) *cobra.Command {
	var customerID, bodyJSON string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a customer (PUT /v2/customers/{customer_id})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT mutates a profile
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/v2/customers/"+url.PathEscape(customerID), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "customer id")
	cmd.Flags().StringVar(&bodyJSON, "body", "", "UpdateCustomer request body as raw JSON (fields to change)")
	_ = cmd.MarkFlagRequired("customer-id")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}
