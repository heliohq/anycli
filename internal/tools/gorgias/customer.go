package gorgias

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCustomerCmd(token, base string) *cobra.Command {
	cmd := newGroupCmd("customer", "Look up customers (list, get)")
	cmd.AddCommand(
		s.newCustomerListCmd(token, base),
		s.newCustomerGetCmd(token, base),
	)
	return cmd
}

func (s *Service) newCustomerListCmd(token, base string) *cobra.Command {
	var page pageFlags
	var email, name, externalID string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List / find customers, filterable by email or name (GET /customers)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			if email != "" {
				q.Set("email", email)
			}
			if name != "" {
				q.Set("name", name)
			}
			if externalID != "" {
				q.Set("external_id", externalID)
			}
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, "/customers", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	page.register(cmd)
	cmd.Flags().StringVar(&email, "email", "", "filter by primary email address")
	cmd.Flags().StringVar(&name, "name", "", "filter by full name")
	cmd.Flags().StringVar(&externalID, "external-id", "", "filter by foreign-system id (Stripe, Aircall, ...)")
	return cmd
}

func (s *Service) newCustomerGetCmd(token, base string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <customer-id>",
		Short:       "Retrieve a customer (GET /customers/{id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, base, http.MethodGet, "/customers/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
