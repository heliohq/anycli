package square

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newPaymentListCmd(token string) *cobra.Command {
	var beginTime, endTime, sortOrder, cursor, locationID, status string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List payments (GET /v2/payments)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setNonEmpty(q, "begin_time", beginTime)
			setNonEmpty(q, "end_time", endTime)
			setNonEmpty(q, "sort_order", sortOrder)
			setNonEmpty(q, "cursor", cursor)
			setNonEmpty(q, "location_id", locationID)
			setNonEmpty(q, "status", status)
			if limit > 0 {
				q.Set("limit", intToString(limit))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/payments", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&beginTime, "begin-time", "", "RFC 3339 lower bound on created_at")
	cmd.Flags().StringVar(&endTime, "end-time", "", "RFC 3339 upper bound on created_at")
	cmd.Flags().StringVar(&sortOrder, "sort-order", "", "ASC or DESC by created_at")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().StringVar(&locationID, "location-id", "", "limit to a location")
	cmd.Flags().StringVar(&status, "status", "", "filter by payment status")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page")
	return cmd
}

func (s *Service) newPaymentGetCmd(token string) *cobra.Command {
	var paymentID string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Retrieve a payment (GET /v2/payments/{payment_id})",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/payments/"+url.PathEscape(paymentID), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&paymentID, "payment-id", "", "payment id")
	_ = cmd.MarkFlagRequired("payment-id")
	return cmd
}
