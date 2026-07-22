package gumroad

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newSubscriberCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "subscriber", Short: "Membership/subscription subscribers (list, get)"}
	cmd.AddCommand(
		s.newSubscriberListCmd(token),
		s.newSubscriberGetCmd(token),
	)
	return cmd
}

func (s *Service) newSubscriberListCmd(token string) *cobra.Command {
	var productID string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a product's subscribers (GET /products/:product_id/subscribers)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/products/"+productID+"/subscribers", nil)
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

func (s *Service) newSubscriberGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a subscriber (GET /subscribers/:id)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/subscribers/"+id, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "subscriber id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
