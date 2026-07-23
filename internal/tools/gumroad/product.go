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
		Use:         "list",
		Short:       "List the account's products (GET /products)",
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
		Use:         "get",
		Short:       "Get a product (GET /products/:id)",
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
		Use:         "enable",
		Short:       "Publish/enable a product (PUT /products/:id/enable)",
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
		Use:         "disable",
		Short:       "Unpublish/disable a product (PUT /products/:id/disable)",
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
		Use:         "delete",
		Short:       "Delete a product (DELETE /products/:id)",
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
