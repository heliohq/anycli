package gumroad

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newSaleCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "sale", Short: "Sales (list, get, mark-shipped, refund)"}
	cmd.AddCommand(
		s.newSaleListCmd(token),
		s.newSaleGetCmd(token),
		s.newSaleMarkShippedCmd(token),
		s.newSaleRefundCmd(token),
	)
	return cmd
}

func (s *Service) newSaleListCmd(token string) *cobra.Command {
	var after, before, email, productID, pageKey string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List successful sales (GET /sales)",
		Long: "Returns ONE page plus a `next_page_key`. Continue by passing that value\n" +
			"back as `--page-key` and loop until a response carries no `next_page_key` —\n" +
			"there is no page-size flag and no numeric page parameter, so a report that\n" +
			"stops at the first page silently under-counts revenue. `--after` and\n" +
			"`--before` take YYYY-MM-DD dates and are the reporting window; `--email`\n" +
			"and `--product-id` narrow it further, and all four combine.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "after", after)
			setIf(q, "before", before)
			setIf(q, "email", email)
			setIf(q, "product_id", productID)
			setIf(q, "page_key", pageKey)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/sales", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&after, "after", "", "only sales after this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&before, "before", "", "only sales before this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&email, "email", "", "filter by buyer email")
	cmd.Flags().StringVar(&productID, "product-id", "", "filter by product id")
	cmd.Flags().StringVar(&pageKey, "page-key", "", "pagination cursor (next_page_key from a prior page)")
	return cmd
}

func (s *Service) newSaleGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a sale (GET /sales/:id)",
		Long: "`--id` is the opaque sale id from `sale list`, not the buyer's email and not\n" +
			"the order number on a receipt. Returns the full sale: buyer email, product,\n" +
			"selected variants, custom-field answers, and the refunded / disputed /\n" +
			"chargeback flags that `sale refund` and a buyer dispute set.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/sales/"+id, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "sale id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newSaleMarkShippedCmd(token string) *cobra.Command {
	var id, trackingURL string
	cmd := &cobra.Command{
		Use:   "mark-shipped",
		Short: "Mark a sale as shipped (PUT /sales/:id/mark_as_shipped)",
		Long: "Gumroad emails the buyer a shipping notification when a sale is marked\n" +
			"shipped, so this is customer-facing, not a private bookkeeping flag.\n" +
			"`--tracking-url` is optional and rides along in that email. It applies to\n" +
			"physical-goods sales, and there is no command here to unmark one.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			setIf(form, "tracking_url", trackingURL)
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/sales/"+id+"/mark_as_shipped", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "sale id")
	cmd.Flags().StringVar(&trackingURL, "tracking-url", "", "shipment tracking URL (optional)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newSaleRefundCmd(token string) *cobra.Command {
	var id string
	var amountCents int
	cmd := &cobra.Command{
		Use:   "refund",
		Short: "Refund a sale, fully or partially (PUT /sales/:id/refund)",
		Long: "Omitting `--amount-cents` refunds the sale in FULL; passing it refunds that\n" +
			"many cents of it. The unit is cents, not dollars — `--amount-cents 500` is\n" +
			"a $5.00 refund, and a slip of two zeroes is a real one. Money moves on\n" +
			"Gumroad's side as the call returns and there is no un-refund command, in\n" +
			"this tool or in the API.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			// Omit amount_cents for a full refund; only send it for a partial.
			if cmd.Flags().Changed("amount-cents") {
				form.Set("amount_cents", strconv.Itoa(amountCents))
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/sales/"+id+"/refund", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "sale id")
	cmd.Flags().IntVar(&amountCents, "amount-cents", 0, "partial refund amount in cents (omit for a full refund)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// setIf sets key=value in q only when value is non-empty.
func setIf(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}
