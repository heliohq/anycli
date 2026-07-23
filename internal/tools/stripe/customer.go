package stripe

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCustomerCmd groups customer reads plus the two support mutations an
// assistant reaches for: create and update. Search is the Stripe Search Query
// Language passthrough scoped to customers.
func (s *Service) newCustomerCmd(token string) *cobra.Command {
	group := newGroupCmd("customer", "Look up and maintain customer records")
	group.AddCommand(
		s.newListCmd(token, "/customers"),
		s.newGetByIDCmd(token, "/customers"),
		s.newCustomerSearchCmd(token),
		s.newCreateCmd(token, "customer", "/customers"),
		s.newUpdateByIDCmd(token, "customer", "/customers"),
	)
	return group
}

// newCustomerSearchCmd is GET /v1/customers/search?query= — Stripe Search
// Query Language, cursor-paginated via `page` (surfaced through --param page=).
func (s *Service) newCustomerSearchCmd(token string) *cobra.Command {
	return s.newResourceSearchCmd(token, "/customers")
}

// newCreateCmd builds a POST create verb for basePath, wiring the shared
// mutation flags (--param, --idempotency-key).
func (s *Service) newCreateCmd(token, singular, basePath string) *cobra.Command {
	var o mutOpts
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a " + singular,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			form, err := o.form()
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, basePath, callOpts{form: form, idempotencyKey: o.idempotencyKey})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerMutationFlags(cmd, &o)
	return cmd
}

// newUpdateByIDCmd builds a POST `update <id>` verb for basePath (Stripe
// updates are POST to the object path).
func (s *Service) newUpdateByIDCmd(token, singular, basePath string) *cobra.Command {
	var o mutOpts
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a " + singular + " by id",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := o.form()
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, basePath+"/"+url.PathEscape(args[0]), callOpts{form: form, idempotencyKey: o.idempotencyKey})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerMutationFlags(cmd, &o)
	return cmd
}
