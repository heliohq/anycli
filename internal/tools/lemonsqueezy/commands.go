package lemonsqueezy

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// resourceGroups assembles the full grouped-by-resource command tree. Each
// resource is a cobra group whose leaves are the shared CRUD verbs plus any
// resource-specific actions (refund, generate-invoice, current-usage, cancel).
func (s *Service) resourceGroups(token string) []*cobra.Command {
	return []*cobra.Command{
		group("store", "Stores",
			s.list(token, "/stores"), s.getOne(token, "/stores")),
		group("product", "Products",
			s.list(token, "/products"), s.getOne(token, "/products")),
		group("variant", "Product variants",
			s.list(token, "/variants"), s.getOne(token, "/variants")),
		group("price", "Prices",
			s.list(token, "/prices"), s.getOne(token, "/prices")),
		group("file", "Digital-download files",
			s.list(token, "/files"), s.getOne(token, "/files")),
		group("order", "Orders",
			s.list(token, "/orders"), s.getOne(token, "/orders"),
			s.refund(token, "/orders"), s.generateInvoice(token, "/orders")),
		group("order-item", "Order items",
			s.list(token, "/order-items"), s.getOne(token, "/order-items")),
		group("customer", "Customers",
			s.list(token, "/customers"), s.getOne(token, "/customers"),
			s.create(token, "/customers"), s.update(token, "/customers")),
		group("subscription", "Subscriptions",
			s.list(token, "/subscriptions"), s.getOne(token, "/subscriptions"),
			s.update(token, "/subscriptions"), s.cancel(token, "/subscriptions")),
		group("subscription-invoice", "Subscription invoices",
			s.list(token, "/subscription-invoices"), s.getOne(token, "/subscription-invoices"),
			s.refund(token, "/subscription-invoices"), s.generateInvoice(token, "/subscription-invoices")),
		group("subscription-item", "Subscription items",
			s.list(token, "/subscription-items"), s.getOne(token, "/subscription-items"),
			s.update(token, "/subscription-items"), s.currentUsage(token, "/subscription-items")),
		group("usage-record", "Usage records",
			s.list(token, "/usage-records"), s.getOne(token, "/usage-records"),
			s.create(token, "/usage-records")),
		group("discount", "Discounts",
			s.list(token, "/discounts"), s.getOne(token, "/discounts"),
			s.create(token, "/discounts"), s.remove(token, "/discounts")),
		group("license-key", "License keys",
			s.list(token, "/license-keys"), s.getOne(token, "/license-keys"),
			s.update(token, "/license-keys")),
		group("license-key-instance", "License key instances",
			s.list(token, "/license-key-instances"), s.getOne(token, "/license-key-instances")),
		group("checkout", "Checkouts",
			s.list(token, "/checkouts"), s.getOne(token, "/checkouts"),
			s.create(token, "/checkouts")),
		group("webhook", "Webhooks",
			s.list(token, "/webhooks"), s.getOne(token, "/webhooks"),
			s.create(token, "/webhooks"), s.update(token, "/webhooks"),
			s.remove(token, "/webhooks")),
	}
}

// group is a runnable command group. cobra skips Args validation on
// non-runnable commands (help + exit 0 even for an unknown subcommand — a
// false success for an agent); making the group runnable restores it.
func group(use, short string, subs ...*cobra.Command) *cobra.Command {
	g := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	g.AddCommand(subs...)
	return g
}

// list is the shared collection GET verb with JSON:API paging/filter/include
// flat flags.
func (s *Service) list(token, path string) *cobra.Command {
	var page, pageSize int
	var filters []string
	var include string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List (GET " + path + ")",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := listQuery(page, pageSize, filters, include)
			if err != nil {
				return err
			}
			return s.get(cmd.Context(), token, path, q)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (page[number])")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "items per page (page[size])")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "filter as key=value (repeatable → filter[key]=value)")
	cmd.Flags().StringVar(&include, "include", "", "comma-separated related resources to include")
	return cmd
}

// getOne is the shared single-resource GET verb.
func (s *Service) getOne(token, path string) *cobra.Command {
	var include string
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve one by id (GET " + path + "/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.get(cmd.Context(), token, path+"/"+url.PathEscape(args[0]), includeQuery(include))
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "comma-separated related resources to include")
	return cmd
}

// create is the shared collection POST verb taking a raw JSON:API document.
func (s *Service) create(token, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create (POST " + path + ")",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := parseData(data)
			if err != nil {
				return err
			}
			if payload == nil {
				return &usageError{msg: "--data is required (a JSON:API document, e.g. {\"data\":{\"type\":...}})"}
			}
			return s.send(cmd.Context(), token, http.MethodPost, path, nil, payload)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON:API request document")
	return cmd
}

// update is the shared single-resource PATCH verb taking a raw JSON:API
// document.
func (s *Service) update(token, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update by id (PATCH " + path + "/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := parseData(data)
			if err != nil {
				return err
			}
			if payload == nil {
				return &usageError{msg: "--data is required (a JSON:API document)"}
			}
			return s.send(cmd.Context(), token, http.MethodPatch, path+"/"+url.PathEscape(args[0]), nil, payload)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON:API request document")
	return cmd
}

// remove is the shared single-resource DELETE verb (used by discounts and
// webhooks, where DELETE means destroy).
func (s *Service) remove(token, path string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete by id (DELETE " + path + "/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.send(cmd.Context(), token, http.MethodDelete, path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// cancel is the DELETE verb for subscriptions, where DELETE means "cancel"
// (the subscription stays valid through its grace period) rather than destroy.
func (s *Service) cancel(token, path string) *cobra.Command {
	return &cobra.Command{
		Use:         "cancel <id>",
		Short:       "Cancel a subscription (DELETE " + path + "/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.send(cmd.Context(), token, http.MethodDelete, path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// refund issues a refund against an order or subscription invoice
// (POST {path}/{id}/refund). --data is optional; omitting it issues a full
// refund, otherwise it carries the partial amount.
func (s *Service) refund(token, path string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "refund <id>",
		Short:       "Issue a refund (POST " + path + "/{id}/refund)",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := parseData(data)
			if err != nil {
				return err
			}
			return s.send(cmd.Context(), token, http.MethodPost, path+"/"+url.PathEscape(args[0])+"/refund", nil, payload)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "optional JSON:API document with a partial refund amount")
	return cmd
}

// generateInvoice generates a downloadable invoice
// (POST {path}/{id}/generate-invoice). Its fields are query parameters
// (name/address/city/…), passed as repeatable --param key=value.
func (s *Service) generateInvoice(token, path string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         "invoice <id>",
		Short:       "Generate a downloadable invoice (POST " + path + "/{id}/generate-invoice)",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := paramQuery(params)
			if err != nil {
				return err
			}
			return s.send(cmd.Context(), token, http.MethodPost, path+"/"+url.PathEscape(args[0])+"/generate-invoice", q, nil)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "invoice field as key=value (repeatable): name, address, city, state, zip_code, country, notes")
	return cmd
}

// currentUsage reads a subscription item's current usage
// (GET /subscription-items/{id}/current-usage).
func (s *Service) currentUsage(token, path string) *cobra.Command {
	return &cobra.Command{
		Use:         "current-usage <id>",
		Short:       "Retrieve a subscription item's current usage (GET " + path + "/{id}/current-usage)",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.get(cmd.Context(), token, path+"/"+url.PathEscape(args[0])+"/current-usage", nil)
		},
	}
}
