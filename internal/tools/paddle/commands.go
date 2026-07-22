package paddle

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// sideEffectAnnotation is the design-318 side-effect key Inspect reads to decide
// whether a leaf needs approval. Kept as a literal (the root-package constant is
// unexported and importing it would cycle).
const sideEffectAnnotation = "anycli.side_effect"

func sideEffect(mutates bool) map[string]string {
	return map[string]string{sideEffectAnnotation: strconv.FormatBool(mutates)}
}

// newRoot builds the grouped-by-resource cobra tree. Each resource is a runnable
// group; verbs hang under it.
func (s *Service) newRoot(token string) *cobra.Command {
	root := &cobra.Command{
		Use:           "paddle",
		Short:         "Paddle Billing built-in service (subscription billing, merchant of record)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "force structured {data, meta} JSON output")

	root.AddCommand(
		s.customerGroup(token),
		s.subscriptionGroup(token),
		s.transactionGroup(token),
		s.catalogGroup(token, "product", "/products", "Manage products"),
		s.catalogGroup(token, "price", "/prices", "Manage prices"),
		s.catalogGroup(token, "discount", "/discounts", "Manage discounts"),
		s.adjustmentGroup(token),
		s.reportGroup(token),
		s.eventGroup(token),
	)
	return root
}

func (s *Service) customerGroup(token string) *cobra.Command {
	g := newGroupCmd("customer", "Look up and manage customers")
	g.AddCommand(
		s.listCmd(token, "/customers", nil),
		s.getCmd(token, "/customers", "get", "Get one customer"),
		s.createCmd(token, "/customers", "Create a customer"),
		s.updateCmd(token, "/customers", "Update a customer"),
		s.subGetCmd(token, "/customers", "credit-balances", "Get a customer's credit balances"),
		s.subGetCmd(token, "/customers", "addresses", "List a customer's addresses"),
		s.subGetCmd(token, "/customers", "businesses", "List a customer's businesses"),
	)
	return g
}

func (s *Service) subscriptionGroup(token string) *cobra.Command {
	g := newGroupCmd("subscription", "Look up and manage subscriptions")
	g.AddCommand(
		s.listCmd(token, "/subscriptions", func(c *cobra.Command) {
			c.Flags().String("customer-id", "", "filter by customer id (ctm_…)")
		}),
		s.getCmd(token, "/subscriptions", "get", "Get one subscription"),
		s.updateCmd(token, "/subscriptions", "Update a subscription (items, proration)"),
		s.actionCmd(token, "/subscriptions", "cancel", "cancel", "Cancel a subscription", true),
		s.actionCmd(token, "/subscriptions", "pause", "pause", "Pause a subscription", true),
		s.actionCmd(token, "/subscriptions", "resume", "resume", "Resume a subscription", true),
		s.actionCmd(token, "/subscriptions", "activate", "activate", "Activate a trialing subscription", true),
		s.actionCmd(token, "/subscriptions", "charge", "charge", "Create a one-time charge on a subscription", true),
		s.actionCmd(token, "/subscriptions", "preview-charge", "charge/preview", "Preview a one-time charge (dry run)", false),
		s.actionCmd(token, "/subscriptions", "preview-update", "preview", "Preview a subscription update (dry run)", false),
	)
	return g
}

func (s *Service) transactionGroup(token string) *cobra.Command {
	g := newGroupCmd("transaction", "Look up transactions and invoices")
	g.AddCommand(
		s.listCmd(token, "/transactions", func(c *cobra.Command) {
			c.Flags().String("customer-id", "", "filter by customer id (ctm_…)")
			c.Flags().String("subscription-id", "", "filter by subscription id (sub_…)")
		}),
		s.getCmd(token, "/transactions", "get", "Get one transaction"),
		s.createCmd(token, "/transactions", "Create a transaction"),
		s.subGetCmd(token, "/transactions", "invoice", "Get a transaction's invoice PDF URL"),
		s.previewCmd(token, "/transactions/preview", "preview", "Preview a transaction (dry run)"),
	)
	return g
}

// catalogGroup builds the list/get/create/update shape shared by products,
// prices, and discounts.
func (s *Service) catalogGroup(token, use, path, short string) *cobra.Command {
	g := newGroupCmd(use, short)
	g.AddCommand(
		s.listCmd(token, path, nil),
		s.getCmd(token, path, "get", "Get one "+use),
		s.createCmd(token, path, "Create a "+use),
		s.updateCmd(token, path, "Update a "+use),
	)
	return g
}

func (s *Service) adjustmentGroup(token string) *cobra.Command {
	g := newGroupCmd("adjustment", "List and create refunds/credits")
	g.AddCommand(
		s.listCmd(token, "/adjustments", func(c *cobra.Command) {
			c.Flags().String("customer-id", "", "filter by customer id (ctm_…)")
			c.Flags().String("subscription-id", "", "filter by subscription id (sub_…)")
		}),
		s.createCmd(token, "/adjustments", "Create an adjustment (refund or credit)"),
	)
	return g
}

func (s *Service) reportGroup(token string) *cobra.Command {
	g := newGroupCmd("report", "Create and download revenue reports")
	g.AddCommand(
		s.createCmd(token, "/reports", "Create a report"),
		s.listCmd(token, "/reports", nil),
		s.getCmd(token, "/reports", "get", "Get one report"),
		s.subGetCmd(token, "/reports", "download-url", "Get a report's CSV download URL"),
	)
	return g
}

func (s *Service) eventGroup(token string) *cobra.Command {
	g := newGroupCmd("event", "Audit events and notification settings")
	g.AddCommand(
		s.listCmd(token, "/events", nil),
		s.rawGetCmd(token, "/event-types", "types", "List available event types"),
		s.rawGetCmd(token, "/notification-settings", "notification-settings", "List notification (webhook) settings"),
	)
	return g
}

// listCmd is GET <path> with cursor pagination + status/filter query flags.
func (s *Service) listCmd(token, path string, extra func(*cobra.Command)) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + strings.TrimPrefix(path, "/"),
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, _ []string) error {
			q, err := listQuery(c)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodGet, path, q, nil)
		},
	}
	cmd.Flags().String("status", "", "filter by status")
	cmd.Flags().String("after", "", "pagination cursor from meta.pagination.next")
	cmd.Flags().Int("per-page", 0, "results per page")
	cmd.Flags().StringArray("filter", nil, "extra filter key=value (repeatable)")
	if extra != nil {
		extra(cmd)
	}
	return cmd
}

// getCmd is GET <path>/<id>.
func (s *Service) getCmd(token, path, use, short string) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, args []string) error {
			return s.run(c, token, http.MethodGet, path+"/"+url.PathEscape(args[0]), nil, nil)
		},
	}
}

// rawGetCmd is GET <path> for account-level collections that take no id.
func (s *Service) rawGetCmd(token, path, use, short string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, _ []string) error {
			return s.run(c, token, http.MethodGet, path, nil, nil)
		},
	}
}

// subGetCmd is GET <path>/<id>/<sub>.
func (s *Service) subGetCmd(token, path, sub, short string) *cobra.Command {
	return &cobra.Command{
		Use:         sub + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, args []string) error {
			return s.run(c, token, http.MethodGet, path+"/"+url.PathEscape(args[0])+"/"+sub, nil, nil)
		},
	}
}

// createCmd is POST <path> with a required --data JSON body.
func (s *Service) createCmd(token, path, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "create",
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(c *cobra.Command, _ []string) error {
			body, err := dataFlag(c, true)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodPost, path, nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

// updateCmd is PATCH <path>/<id> with a required --data JSON body.
func (s *Service) updateCmd(token, path, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(c *cobra.Command, args []string) error {
			body, err := dataFlag(c, true)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodPatch, path+"/"+url.PathEscape(args[0]), nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

// actionCmd is POST <path>/<id>/<sub> with an optional --data JSON body.
func (s *Service) actionCmd(token, path, use, sub, short string, mutates bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(mutates),
		RunE: func(c *cobra.Command, args []string) error {
			body, err := dataFlag(c, false)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodPost, path+"/"+url.PathEscape(args[0])+"/"+sub, nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

// previewCmd is POST <path> (no id) with a required --data JSON body — a
// read-only dry run.
func (s *Service) previewCmd(token, path, use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(c *cobra.Command, _ []string) error {
			body, err := dataFlag(c, true)
			if err != nil {
				return err
			}
			return s.run(c, token, http.MethodPost, path, nil, body)
		},
	}
	dataFlagDef(cmd)
	return cmd
}

func dataFlagDef(cmd *cobra.Command) {
	cmd.Flags().String("data", "", "request body as a JSON object")
}

// dataFlag reads and validates the --data JSON body. When required, an empty
// value is a usage error; when optional, an empty value sends no body.
func dataFlag(cmd *cobra.Command, required bool) ([]byte, error) {
	raw := strings.TrimSpace(mustString(cmd, "data"))
	if raw == "" {
		if required {
			return nil, newUsageError("--data is required (a JSON request body)")
		}
		return nil, nil
	}
	if !json.Valid([]byte(raw)) {
		return nil, newUsageError("--data is not valid JSON")
	}
	return []byte(raw), nil
}

// listQuery builds the list query from the registered flags. status/after/
// per-page map to Paddle's documented query params; --filter carries any other
// key=value pair; convenience --customer-id / --subscription-id map to the
// documented filter params.
func listQuery(cmd *cobra.Command) (url.Values, error) {
	q := url.Values{}
	if v := strings.TrimSpace(mustString(cmd, "status")); v != "" {
		q.Set("status", v)
	}
	if v := strings.TrimSpace(mustString(cmd, "after")); v != "" {
		q.Set("after", v)
	}
	if cmd.Flags().Changed("per-page") {
		n, _ := cmd.Flags().GetInt("per-page")
		if n > 0 {
			q.Set("per_page", strconv.Itoa(n))
		}
	}
	if f := cmd.Flags().Lookup("customer-id"); f != nil {
		if v := strings.TrimSpace(f.Value.String()); v != "" {
			q.Set("customer_id", v)
		}
	}
	if f := cmd.Flags().Lookup("subscription-id"); f != nil {
		if v := strings.TrimSpace(f.Value.String()); v != "" {
			q.Set("subscription_id", v)
		}
	}
	filters, _ := cmd.Flags().GetStringArray("filter")
	for _, kv := range filters {
		key, value, ok := strings.Cut(kv, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, newUsageError("--filter %q must be key=value", kv)
		}
		q.Set(key, strings.TrimSpace(value))
	}
	return q, nil
}

func mustString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// run executes one call and emits the result, honoring the root --json flag.
func (s *Service) run(cmd *cobra.Command, token, method, path string, q url.Values, body []byte) error {
	jsonMode, _ := cmd.Root().PersistentFlags().GetBool("json")
	env, err := s.call(cmd.Context(), token, method, path, q, body)
	if err != nil {
		return err
	}
	return s.emit(jsonMode, env)
}
