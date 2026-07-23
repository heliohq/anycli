package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newOrderSearchCmd(token string) *cobra.Command {
	var bodyJSON string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search orders (POST /v2/orders/search)",
		Args:  cobra.NoArgs,
		// POST /v2/orders/search is a documented lookup (POST-shaped read); it
		// never mutates provider state under any input.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("body", bodyJSON)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/orders/search", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "SearchOrders request body as raw JSON (location_ids, query, limit, cursor)")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newOrderGetCmd(token string) *cobra.Command {
	var orderID string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Retrieve an order (GET /v2/orders/{order_id})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/orders/"+url.PathEscape(orderID), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&orderID, "order-id", "", "order id")
	_ = cmd.MarkFlagRequired("order-id")
	return cmd
}
