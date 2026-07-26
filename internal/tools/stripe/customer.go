package stripe

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// The customer Longs live here because list, get, create and update are all
// built by shared constructors that take their prose as a parameter.
const (
	longCustomerList = "Returns customers newest first and filters only on exact fields through\n" +
		"--param (`email`, `created`). Anything looser — a partial name, a metadata\n" +
		"value — needs `customer search`, which takes a query language instead."

	longCustomerGet = "Takes a `cus_` id. A response carrying `deleted:true` is a deleted customer,\n" +
		"not an error. Subscriptions and invoices are NOT embedded: read them with\n" +
		"`subscription list --param customer=cus_123` and\n" +
		"`invoice list --param customer=cus_123`."

	longCustomerCreate = "--param carries the fields: `email`, `name`, `description`,\n" +
		"`metadata[...]`. Stripe does not deduplicate — creating the same email\n" +
		"twice yields two distinct customers with separate billing histories, so\n" +
		"search before creating and pass --idempotency-key on anything that may be\n" +
		"retried."

	longCustomerSearch = "Stripe Search Query Language over customers: `email:'a@b.com'`,\n" +
		"`name:'Acme'`, `metadata['tier']:'gold'`. The index is EVENTUALLY\n" +
		"CONSISTENT, so a customer created moments ago may not be findable yet —\n" +
		"retrieve it by the id `customer create` returned instead. Paging is --page\n" +
		"from `next_page`, not --starting-after."

	longCustomerUpdate = "A POST to the object despite the name. Only the fields passed change, with\n" +
		"one exception: `metadata` is MERGED rather than replaced, and a key given an\n" +
		"empty value is deleted, so `--param metadata[tier]=` removes tier instead of\n" +
		"blanking it."
)

// newCustomerCmd groups customer reads plus the two support mutations an
// assistant reaches for: create and update. Search is the Stripe Search Query
// Language passthrough scoped to customers.
func (s *Service) newCustomerCmd(token string) *cobra.Command {
	group := newGroupCmd("customer", "Look up and maintain customer records")
	group.AddCommand(
		s.newListCmd(token, "/customers", longCustomerList),
		s.newGetByIDCmd(token, "/customers", longCustomerGet),
		s.newCustomerSearchCmd(token),
		s.newCreateCmd(token, "customer", "/customers", longCustomerCreate),
		s.newUpdateByIDCmd(token, "customer", "/customers", longCustomerUpdate),
	)
	return group
}

// newCustomerSearchCmd is GET /v1/customers/search?query= — Stripe Search
// Query Language, cursor-paginated via `page` (surfaced through --param page=).
func (s *Service) newCustomerSearchCmd(token string) *cobra.Command {
	return s.newResourceSearchCmd(token, "/customers", longCustomerSearch)
}

// newCreateCmd builds a POST create verb for basePath, wiring the shared
// mutation flags (--param, --idempotency-key).
func (s *Service) newCreateCmd(token, singular, basePath, long string) *cobra.Command {
	var o mutOpts
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a " + singular,
		Long:        long,
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
func (s *Service) newUpdateByIDCmd(token, singular, basePath, long string) *cobra.Command {
	var o mutOpts
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a " + singular + " by id",
		Long:        long,
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
