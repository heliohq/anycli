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
		Use:   "list",
		Short: "List a product's offer codes (GET /products/:product_id/offer_codes)",
		Long: "Offer codes belong to a product, so `--product-id` is required and there is\n" +
			"no account-wide code list — a coupon defined on another product does not\n" +
			"appear here. Each entry carries the id that `offer-code update` and\n" +
			"`offer-code delete` need, plus the coupon string, the discount, and how\n" +
			"many times it has been redeemed.",
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
		Use:   "get",
		Short: "Get an offer code (GET /products/:product_id/offer_codes/:id)",
		Long: "Needs both `--product-id` and the code's `--id`. The coupon string buyers\n" +
			"type is the code's `name` and is NOT addressable here, so a code known only\n" +
			"by its string has to be found through `offer-code list` first. Returns the\n" +
			"discount, whether it is cents or percent, and the redemption count against\n" +
			"`max_purchase_count`.",
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
		Use:   "create",
		Short: "Create an offer code (POST /products/:product_id/offer_codes)",
		Long: "`--name` is the coupon string buyers type at checkout. `--amount-off` is\n" +
			"CENTS by default — `--amount-off 1000` takes $10 off — and `--percent`\n" +
			"reinterprets the same number as whole percent, so `--amount-off 50\n" +
			"--percent` is half price. Getting that pair wrong is the difference between\n" +
			"a 50-cent discount and a 50% one. `--max-purchase-count` caps total\n" +
			"redemptions and is unlimited when omitted. The code is redeemable on the\n" +
			"live product as soon as this returns.",
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
		Use:   "update",
		Short: "Update an offer code (PUT /products/:product_id/offer_codes/:id)",
		Long: "`--max-purchase-count` is the ONLY field this changes. The coupon string,\n" +
			"the discount amount and the cents-or-percent mode are fixed at creation, so\n" +
			"changing a discount means `offer-code delete` followed by a fresh\n" +
			"`offer-code create`. Both `--product-id` and `--id` are required, since\n" +
			"codes are addressed under their product.",
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
		Use:   "delete",
		Short: "Delete an offer code (DELETE /products/:product_id/offer_codes/:id)",
		Long: "The code stops being redeemable immediately; orders already placed with it\n" +
			"keep their discount, so this closes it to new buyers rather than repricing\n" +
			"anything. Deleting and re-creating is also the only way to change a\n" +
			"discount amount, which `offer-code update` cannot touch.",
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
