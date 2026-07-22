package chargebee

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// sideEffectFalse / sideEffectTrue are the design-318 annotation maps a leaf
// carries so Inspect/approval-gate coverage can classify it without executing.
func sideEffect(write bool) map[string]string {
	return map[string]string{"anycli.side_effect": strconv.FormatBool(write)}
}

// resourceCommands builds the full grouped-by-resource tree plus the read-only
// GET escape hatch.
func (s *Service) resourceCommands(cfg reqConfig) []*cobra.Command {
	customer := s.readGroup(cfg, "customer", "Manage customers", "/customers")
	customer.AddCommand(
		s.formWriteCmd(cfg, "create", "Create a customer", http.MethodPost, "/customers", false),
		s.formWriteByIDCmd(cfg, "update", "Update a customer", "/customers/%s"),
	)

	subscription := s.readGroup(cfg, "subscription", "Manage subscriptions", "/subscriptions")
	subscription.AddCommand(
		s.subscriptionCreateCmd(cfg),
		s.subscriptionChangeCmd(cfg),
		s.subscriptionActionCmd(cfg, "cancel", "Cancel a subscription", "/subscriptions/%s/cancel_for_items"),
		s.subscriptionActionCmd(cfg, "reactivate", "Reactivate a subscription", "/subscriptions/%s/reactivate"),
	)

	invoice := s.readGroup(cfg, "invoice", "Read invoices", "/invoices")
	invoice.AddCommand(s.invoicePDFCmd(cfg))

	usage := newGroup("usage", "Metered usage")
	usage.AddCommand(
		s.listCmd(cfg, "/usages"),
		s.usageCreateCmd(cfg),
	)

	paymentSource := newGroup("payment-source", "Read payment instruments")
	paymentSource.AddCommand(s.listCmd(cfg, "/payment_sources"))

	return []*cobra.Command{
		customer,
		subscription,
		invoice,
		s.readGroup(cfg, "credit-note", "Read credit notes", "/credit_notes"),
		s.readGroup(cfg, "item", "Read catalog items", "/items"),
		s.readGroup(cfg, "item-price", "Read item prices", "/item_prices"),
		s.readGroup(cfg, "plan", "Read plans (Product Catalog 1.0)", "/plans"),
		s.readGroup(cfg, "transaction", "Read payment transactions", "/transactions"),
		s.readGroup(cfg, "event", "Read the billing event stream", "/events"),
		usage,
		paymentSource,
		s.getCmd(cfg),
	}
}

// newGroup is a runnable command group: a bare group prints help; an unknown
// subcommand fails (cobra.NoArgs). Groups carry no side_effect annotation
// (design 318 (b)/(f)).
func newGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// readGroup is a resource group pre-populated with the shared list + get reads.
func (s *Service) readGroup(cfg reqConfig, use, short, resourcePath string) *cobra.Command {
	group := newGroup(use, short)
	group.AddCommand(
		s.listCmd(cfg, resourcePath),
		s.getCmdForResource(cfg, resourcePath),
	)
	return group
}

// listCmd is the shared paged list read: --limit / --offset plus repeated
// --filter <field[op]=value> mapped verbatim to Chargebee bracket-operator query
// params.
func (s *Service) listCmd(cfg reqConfig, resourcePath string) *cobra.Command {
	var (
		limit   int
		offset  string
		filters []string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List resources",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			if limit > 0 {
				query.Set("limit", strconv.Itoa(limit))
			}
			if offset != "" {
				query.Set("offset", offset)
			}
			if err := applyFilters(query, filters); err != nil {
				return err
			}
			return s.read(cmd, cfg, resourcePath, query)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (Chargebee caps at 100)")
	cmd.Flags().StringVar(&offset, "offset", "", "opaque pagination cursor (next_offset from a prior page)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "bracket-operator filter, e.g. status[is]=active (repeatable)")
	return cmd
}

// getCmdForResource is the shared single-object read: GET <resource>/{id}.
func (s *Service) getCmdForResource(cfg reqConfig, resourcePath string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve one resource by id",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.read(cmd, cfg, resourcePath+"/"+url.PathEscape(args[0]), nil)
		},
	}
}

// getCmd is the top-level read-only GET escape hatch for the long tail (quotes,
// estimates, orders, exports).
func (s *Service) getCmd(cfg reqConfig) *cobra.Command {
	var (
		path    string
		queries []string
	)
	cmd := &cobra.Command{
		Use:         "get --path <p> [--query k=v ...]",
		Short:       "GET any Chargebee v2 path (read-only escape hatch)",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !strings.HasPrefix(path, "/") {
				return &usageError{msg: "--path must start with '/'"}
			}
			query := url.Values{}
			for _, raw := range queries {
				key, value, ok := strings.Cut(raw, "=")
				if !ok {
					return &usageError{msg: fmt.Sprintf("--query %q must be key=value", raw)}
				}
				query.Add(key, value)
			}
			return s.read(cmd, cfg, path, query)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "v2 path to GET, e.g. /quotes")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter key=value (repeatable)")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

// formWriteCmd is a POST that takes flat form fields via repeated --param.
func (s *Service) formWriteCmd(cfg reqConfig, use, short, method, path string, requireParam bool) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			return s.write(cmd, cfg, method, path, form)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "form field key=value (repeatable)")
	if requireParam {
		_ = cmd.MarkFlagRequired("param")
	}
	return cmd
}

// formWriteByIDCmd is a POST to <pathTemplate % id> with flat --param fields.
func (s *Service) formWriteByIDCmd(cfg reqConfig, use, short, pathTemplate string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			return s.write(cmd, cfg, http.MethodPost, fmt.Sprintf(pathTemplate, url.PathEscape(args[0])), form)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "form field key=value (repeatable)")
	return cmd
}

// subscriptionCreateCmd posts to the customer-scoped create-for-items path with
// the bracketed indexed subscription_items array plus flat --param fields.
func (s *Service) subscriptionCreateCmd(cfg reqConfig) *cobra.Command {
	var (
		customerID string
		itemPrices []string
		params     []string
	)
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a subscription for a customer",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			if err := applyItemPrices(form, itemPrices); err != nil {
				return err
			}
			path := "/customers/" + url.PathEscape(customerID) + "/subscription_for_items"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringVar(&customerID, "customer-id", "", "customer the subscription belongs to")
	cmd.Flags().StringArrayVar(&itemPrices, "item-price", nil, "item price as id[:quantity] (repeatable)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra form field key=value (repeatable)")
	_ = cmd.MarkFlagRequired("customer-id")
	_ = cmd.MarkFlagRequired("item-price")
	return cmd
}

// subscriptionChangeCmd updates a subscription's items (and optional flat fields).
func (s *Service) subscriptionChangeCmd(cfg reqConfig) *cobra.Command {
	var (
		itemPrices []string
		params     []string
	)
	cmd := &cobra.Command{
		Use:         "change <id>",
		Short:       "Change a subscription's items",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			if err := applyItemPrices(form, itemPrices); err != nil {
				return err
			}
			path := "/subscriptions/" + url.PathEscape(args[0]) + "/update_subscription_for_items"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringArrayVar(&itemPrices, "item-price", nil, "item price as id[:quantity] (repeatable)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra form field key=value (repeatable)")
	return cmd
}

// subscriptionActionCmd is a by-id POST action (cancel/reactivate) with optional
// flat --param fields.
func (s *Service) subscriptionActionCmd(cfg reqConfig, use, short, pathTemplate string) *cobra.Command {
	return s.formWriteByIDCmd(cfg, use, short, pathTemplate)
}

// usageCreateCmd posts metered usage to the subscription-scoped usages path.
// There is no flat POST /usages, so the subscription id is required.
func (s *Service) usageCreateCmd(cfg reqConfig) *cobra.Command {
	var (
		subscriptionID string
		params         []string
	)
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Record metered usage on a subscription",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			path := "/subscriptions/" + url.PathEscape(subscriptionID) + "/usages"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringVar(&subscriptionID, "subscription-id", "", "subscription the usage is recorded against")
	cmd.Flags().StringArrayVar(&params, "param", nil, "usage form field key=value (repeatable)")
	_ = cmd.MarkFlagRequired("subscription-id")
	return cmd
}

// invoicePDFCmd issues the POST /invoices/{id}/pdf request, which returns a JSON
// download object (transient download_url + valid_till), not raw PDF bytes.
func (s *Service) invoicePDFCmd(cfg reqConfig) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:         "pdf <id>",
		Short:       "Get a transient download URL for an invoice PDF",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			form, err := formFromParams(params)
			if err != nil {
				return err
			}
			path := "/invoices/" + url.PathEscape(args[0]) + "/pdf"
			return s.write(cmd, cfg, http.MethodPost, path, form)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "form field key=value, e.g. disposition_type=attachment (repeatable)")
	return cmd
}

// read performs a GET and emits the provider JSON passthrough.
func (s *Service) read(cmd *cobra.Command, cfg reqConfig, path string, query url.Values) error {
	body, err := s.call(cmd.Context(), cfg, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// write performs a form-encoded POST and emits the provider JSON passthrough.
func (s *Service) write(cmd *cobra.Command, cfg reqConfig, method, path string, form url.Values) error {
	if form == nil {
		form = url.Values{}
	}
	body, err := s.call(cmd.Context(), cfg, method, path, nil, form)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// applyFilters maps repeated bracket-operator --filter values (status[is]=active)
// verbatim onto query params. The key already carries Chargebee's [op] suffix.
func applyFilters(query url.Values, filters []string) error {
	for _, raw := range filters {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || key == "" {
			return &usageError{msg: fmt.Sprintf("--filter %q must be field[op]=value", raw)}
		}
		query.Add(key, value)
	}
	return nil
}

// formFromParams splits repeated --param key=value into a form value set.
func formFromParams(params []string) (url.Values, error) {
	form := url.Values{}
	for _, raw := range params {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || key == "" {
			return nil, &usageError{msg: fmt.Sprintf("--param %q must be key=value", raw)}
		}
		form.Add(key, value)
	}
	return form, nil
}

// applyItemPrices expands repeated id[:quantity] entries onto Chargebee's
// bracketed indexed subscription_items array
// (subscription_items[item_price_id][i], subscription_items[quantity][i]).
func applyItemPrices(form url.Values, itemPrices []string) error {
	for i, raw := range itemPrices {
		id, qty, hasQty := strings.Cut(raw, ":")
		if id == "" {
			return &usageError{msg: fmt.Sprintf("--item-price %q must be item_price_id[:quantity]", raw)}
		}
		form.Set(fmt.Sprintf("subscription_items[item_price_id][%d]", i), id)
		if hasQty {
			if _, err := strconv.Atoi(qty); err != nil {
				return &usageError{msg: fmt.Sprintf("--item-price %q quantity must be an integer", raw)}
			}
			form.Set(fmt.Sprintf("subscription_items[quantity][%d]", i), qty)
		}
	}
	return nil
}
