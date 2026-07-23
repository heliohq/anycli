package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newPurchasesCmd builds the `purchases` resource group: revenue / quantity
// time-series (GET export).
func (s *Service) newPurchasesCmd(c *client) *cobra.Command {
	group := newGroupCmd("purchases", "Revenue and purchase-quantity analytics")
	group.AddCommand(s.newPurchasesSeriesCmd(c))
	return group
}

// newPurchasesSeriesCmd is `purchases series`: GET
// /purchases/{revenue_series,quantity_series} selected by --metric.
func (s *Service) newPurchasesSeriesCmd(c *client) *cobra.Command {
	var metric, endingAt, appID, productID string
	var length int
	cmd := &cobra.Command{
		Use:         "series",
		Short:       "Get revenue or purchase-quantity time-series",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().StringVar(&metric, "metric", "revenue", "revenue|quantity")
	cmd.Flags().IntVar(&length, "length", 7, "number of days (max 100) ending at --ending-at")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (optional; default now)")
	cmd.Flags().StringVar(&appID, "app-id", "", "restrict to a single app (optional)")
	cmd.Flags().StringVar(&productID, "product-id", "", "restrict to a single product (optional)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		var path string
		switch metric {
		case "revenue":
			path = "/purchases/revenue_series"
		case "quantity":
			path = "/purchases/quantity_series"
		default:
			return &usageError{msg: "--metric must be revenue or quantity"}
		}
		q := url.Values{}
		q.Set("length", strconv.Itoa(length))
		if endingAt != "" {
			q.Set("ending_at", endingAt)
		}
		if appID != "" {
			q.Set("app_id", appID)
		}
		if productID != "" {
			q.Set("product_id", productID)
		}
		body, err := c.get(cmd.Context(), path, q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
