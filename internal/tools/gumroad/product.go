package gumroad

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newProductCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "product", Short: "Products (list, get, enable, disable, delete)"}
	cmd.AddCommand(
		s.newProductListCmd(token),
		s.newProductGetCmd(token),
		s.newProductEnableCmd(token),
		s.newProductDisableCmd(token),
		s.newProductDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newProductListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the account's products (GET /products)",
		Long: "Returns the whole catalog in one response: there is no pagination, no\n" +
			"filter and no page-size parameter, so every product comes back every call.\n" +
			"Draft and unpublished products are included and distinguished by the\n" +
			"`published` boolean on each entry. This is the only way to resolve a\n" +
			"product id, which `sale list`, `subscriber list`, `offer-code *` and\n" +
			"`license *` all require.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/products", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newProductGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a product (GET /products/:id)",
		Long: "`--id` is the opaque product id from `product list`, not the permalink, the\n" +
			"short URL or the product name. Returns that one product's price,\n" +
			"`published` state, variant categories and custom fields. An id belonging to\n" +
			"another account fails with Gumroad's `success:false` rather than returning\n" +
			"an empty object.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/products/"+id, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "product id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newProductEnableCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Publish/enable a product (PUT /products/:id/enable)",
		Long: "Publishes the product, making it purchasable on the creator's live store as\n" +
			"soon as the call returns. It is the exact inverse of `product disable` and\n" +
			"restores the original listing — same id, same URL, same price — rather than\n" +
			"creating a new one.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/products/"+id+"/enable", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "product id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newProductDisableCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Unpublish/disable a product (PUT /products/:id/disable)",
		Long: "Takes the product off sale immediately on a live store. It affects new\n" +
			"buyers only: existing sales, subscribers and issued license keys are\n" +
			"untouched, so this neither refunds nor revokes anything. `product enable`\n" +
			"puts it back, which makes disable the reversible alternative to\n" +
			"`product delete`.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // PUT
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/products/"+id+"/disable", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "product id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newProductDeleteCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a product (DELETE /products/:id)",
		Long: "PERMANENT: there is no restore command and no Gumroad endpoint to bring the\n" +
			"product back, so `product disable` is the reversible way to stop selling\n" +
			"something. Offer codes are addressed under their product, so the product's\n" +
			"codes become unreachable with it.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // DELETE
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodDelete, "/products/"+id, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "product id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
