package gumroad

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newOfferCodeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "offer-code", Short: "Discount offer codes (list, get, create, update, delete)"}
	cmd.AddCommand(
		s.newOfferCodeListCmd(token),
		s.newOfferCodeGetCmd(token),
		s.newOfferCodeCreateCmd(token),
		s.newOfferCodeUpdateCmd(token),
		s.newOfferCodeDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newOfferCodeListCmd(token string) *cobra.Command {
	var productID string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a product's offer codes (GET /products/:product_id/offer_codes)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/products/"+productID+"/offer_codes", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id")
	_ = cmd.MarkFlagRequired("product-id")
	return cmd
}

func (s *Service) newOfferCodeGetCmd(token string) *cobra.Command {
	var productID, id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get an offer code (GET /products/:product_id/offer_codes/:id)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/products/"+productID+"/offer_codes/"+id, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id")
	cmd.Flags().StringVar(&id, "id", "", "offer code id")
	_ = cmd.MarkFlagRequired("product-id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newOfferCodeCreateCmd(token string) *cobra.Command {
	var productID, name string
	var amountOff, maxPurchaseCount int
	var percent bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create an offer code (POST /products/:product_id/offer_codes)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			form.Set("name", name)
			form.Set("amount_off", strconv.Itoa(amountOff))
			// Gumroad discriminates absolute vs percentage discounts via
			// offer_type: cents (absolute) | percent.
			if percent {
				form.Set("offer_type", "percent")
			} else {
				form.Set("offer_type", "cents")
			}
			if cmd.Flags().Changed("max-purchase-count") {
				form.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/products/"+productID+"/offer_codes", form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id")
	cmd.Flags().StringVar(&name, "name", "", "offer code (the coupon string buyers enter)")
	cmd.Flags().IntVar(&amountOff, "amount-off", 0, "discount amount: cents when absolute, whole percent when --percent")
	cmd.Flags().BoolVar(&percent, "percent", false, "treat --amount-off as a percentage discount")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "max redemptions (optional)")
	_ = cmd.MarkFlagRequired("product-id")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("amount-off")
	return cmd
}

func (s *Service) newOfferCodeUpdateCmd(token string) *cobra.Command {
	var productID, id string
	var maxPurchaseCount int
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update an offer code (PUT /products/:product_id/offer_codes/:id)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT
		RunE: func(cmd *cobra.Command, _ []string) error {
			form := url.Values{}
			if cmd.Flags().Changed("max-purchase-count") {
				form.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/products/"+productID+"/offer_codes/"+id, form)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id")
	cmd.Flags().StringVar(&id, "id", "", "offer code id")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "new max redemptions")
	_ = cmd.MarkFlagRequired("product-id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newOfferCodeDeleteCmd(token string) *cobra.Command {
	var productID, id string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete an offer code (DELETE /products/:product_id/offer_codes/:id)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // DELETE
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodDelete, "/products/"+productID+"/offer_codes/"+id, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&productID, "product-id", "", "product id")
	cmd.Flags().StringVar(&id, "id", "", "offer code id")
	_ = cmd.MarkFlagRequired("product-id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
