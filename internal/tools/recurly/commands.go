package recurly

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// listFlags holds the query filters shared by every `list` leaf. They map
// directly to Recurly's collection query parameters.
type listFlags struct {
	limit     int
	cursor    string
	state     string
	typ       string
	order     string
	sort      string
	beginTime string
	endTime   string
	account   string // account-scoped lists only
}

// registerListFlags attaches the shared list filters. accountScoped adds
// --account, which reroutes the request under /accounts/<code>/… .
func registerListFlags(cmd *cobra.Command, accountScoped bool) *listFlags {
	lf := &listFlags{}
	f := cmd.Flags()
	f.IntVar(&lf.limit, "limit", 0, "max records to return (1-200)")
	f.StringVar(&lf.cursor, "cursor", "", "pagination cursor from a prior response's next")
	f.StringVar(&lf.state, "state", "", "filter by state")
	f.StringVar(&lf.typ, "type", "", "filter by type")
	f.StringVar(&lf.order, "order", "", "sort order: asc or desc")
	f.StringVar(&lf.sort, "sort", "", "field to sort by (created_at or updated_at)")
	f.StringVar(&lf.beginTime, "begin-time", "", "inclusive lower time bound (ISO-8601)")
	f.StringVar(&lf.endTime, "end-time", "", "exclusive upper time bound (ISO-8601)")
	if accountScoped {
		f.StringVar(&lf.account, "account", "", "scope the list to an account (ID or code-<code>)")
	}
	return lf
}

// query builds the Recurly query string from the set filters (unset fields are
// omitted).
func (lf *listFlags) query() url.Values {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	if lf.limit > 0 {
		q.Set("limit", strconv.Itoa(lf.limit))
	}
	set("cursor", lf.cursor)
	set("state", lf.state)
	set("type", lf.typ)
	set("order", lf.order)
	set("sort", lf.sort)
	set("begin_time", lf.beginTime)
	set("end_time", lf.endTime)
	return q
}

// runList issues a collection GET and emits the provider-neutral list envelope.
func (s *Service) runList(ctx context.Context, key, region, path string, q url.Values) error {
	body, err := s.call(ctx, key, region, "GET", path, q, nil)
	if err != nil {
		return err
	}
	env, err := toListEnvelope(body)
	if err != nil {
		return err
	}
	return s.emitJSON(env)
}

// runGet issues a single-resource request and passes the resource JSON through
// unwrapped.
func (s *Service) runGet(ctx context.Context, key, region, method, path string, q url.Values, payload []byte) error {
	body, err := s.call(ctx, key, region, method, path, q, payload)
	if err != nil {
		return err
	}
	return s.emitJSON(body)
}

// newAccountGroup builds `recurly account …`.
func (s *Service) newAccountGroup(key, region string) *cobra.Command {
	g := newGroupCmd("account", "Look up accounts (customers)")

	list := &cobra.Command{Use: "list", Short: "List accounts", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/accounts", lf.query())
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one account", func(id string) string { return "/accounts/" + id }),
		s.newGetLeaf(key, region, "balance", "Get an account balance", func(id string) string { return "/accounts/" + id + "/balance" }),
		s.newGetLeaf(key, region, "billing-info", "Get an account's billing info", func(id string) string { return "/accounts/" + id + "/billing_info" }),
	)
	return g
}

// newSubscriptionGroup builds `recurly subscription …`.
func (s *Service) newSubscriptionGroup(key, region string) *cobra.Command {
	g := newGroupCmd("subscription", "Manage subscriptions")

	list := &cobra.Command{Use: "list", Short: "List subscriptions", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/subscriptions"), lf.query())
	}

	create := &cobra.Command{Use: "create", Short: "Create a subscription", Args: cobra.NoArgs, Annotations: sideEffect(true)}
	createBody := create.Flags().String("body", "", "subscription JSON body (required)")
	create.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := requireBody(*createBody)
		if err != nil {
			return err
		}
		return s.runGet(cmd.Context(), key, region, "POST", "/subscriptions", nil, payload)
	}

	change := s.newBodyLeaf(key, region, "change", "Modify a subscription", "PUT",
		func(id string) string { return "/subscriptions/" + id })

	pause := &cobra.Command{Use: "pause <id>", Short: "Pause a subscription", Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	cycles := pause.Flags().Int("cycles", 0, "number of billing cycles to pause (remaining_pause_cycles)")
	pause.RunE = func(cmd *cobra.Command, args []string) error {
		payload, _ := json.Marshal(map[string]any{"remaining_pause_cycles": *cycles})
		return s.runGet(cmd.Context(), key, region, "PUT", "/subscriptions/"+args[0]+"/pause", nil, payload)
	}

	terminate := &cobra.Command{Use: "terminate <id>", Short: "Terminate a subscription", Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	refund := terminate.Flags().String("refund", "", "refund type: none, partial, or full")
	terminate.RunE = func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if *refund != "" {
			q.Set("refund", *refund)
		}
		return s.runGet(cmd.Context(), key, region, "DELETE", "/subscriptions/"+args[0], q, nil)
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one subscription", func(id string) string { return "/subscriptions/" + id }),
		create,
		change,
		s.newActionLeaf(key, region, "cancel", "Cancel a subscription", func(id string) string { return "/subscriptions/" + id + "/cancel" }),
		pause,
		s.newActionLeaf(key, region, "resume", "Resume a paused subscription", func(id string) string { return "/subscriptions/" + id + "/resume" }),
		terminate,
	)
	return g
}

// newInvoiceGroup builds `recurly invoice …`.
func (s *Service) newInvoiceGroup(key, region string) *cobra.Command {
	g := newGroupCmd("invoice", "Look up invoices and retry collection")

	list := &cobra.Command{Use: "list", Short: "List invoices", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/invoices"), lf.query())
	}

	lineItems := &cobra.Command{Use: "line-items <id>", Short: "List an invoice's line items", Args: cobra.ExactArgs(1), Annotations: sideEffect(false)}
	lineItems.RunE = func(cmd *cobra.Command, args []string) error {
		return s.runList(cmd.Context(), key, region, "/invoices/"+args[0]+"/line_items", nil)
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one invoice", func(id string) string { return "/invoices/" + id }),
		lineItems,
		s.newActionLeaf(key, region, "collect", "Retry collection on an invoice", func(id string) string { return "/invoices/" + id + "/collect" }),
	)
	return g
}

// newTransactionGroup builds `recurly transaction …`.
func (s *Service) newTransactionGroup(key, region string) *cobra.Command {
	g := newGroupCmd("transaction", "Look up payment transactions")

	list := &cobra.Command{Use: "list", Short: "List transactions", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/transactions"), lf.query())
	}

	g.AddCommand(
		list,
		s.newGetLeaf(key, region, "get", "Get one transaction", func(id string) string { return "/transactions/" + id }),
	)
	return g
}

// newPlanGroup builds `recurly plan …`.
func (s *Service) newPlanGroup(key, region string) *cobra.Command {
	g := newGroupCmd("plan", "Look up plans (catalog)")
	list := &cobra.Command{Use: "list", Short: "List plans", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/plans", lf.query())
	}
	g.AddCommand(list, s.newGetLeaf(key, region, "get", "Get one plan", func(id string) string { return "/plans/" + id }))
	return g
}

// newCouponGroup builds `recurly coupon …`.
func (s *Service) newCouponGroup(key, region string) *cobra.Command {
	g := newGroupCmd("coupon", "Look up coupons (discounts)")
	list := &cobra.Command{Use: "list", Short: "List coupons", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/coupons", lf.query())
	}
	g.AddCommand(list, s.newGetLeaf(key, region, "get", "Get one coupon", func(id string) string { return "/coupons/" + id }))
	return g
}

// newLineItemGroup builds `recurly line-item …`.
func (s *Service) newLineItemGroup(key, region string) *cobra.Command {
	g := newGroupCmd("line-item", "Look up line items")
	list := &cobra.Command{Use: "list", Short: "List line items", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, true)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, accountScoped(lf.account, "/line_items"), lf.query())
	}
	g.AddCommand(list)
	return g
}

// newSiteGroup builds `recurly site …`.
func (s *Service) newSiteGroup(key, region string) *cobra.Command {
	g := newGroupCmd("site", "Look up Recurly sites")
	list := &cobra.Command{Use: "list", Short: "List sites", Args: cobra.NoArgs, Annotations: sideEffect(false)}
	lf := registerListFlags(list, false)
	list.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.runList(cmd.Context(), key, region, "/sites", lf.query())
	}
	g.AddCommand(list, s.newGetLeaf(key, region, "get", "Get one site", func(id string) string { return "/sites/" + id }))
	return g
}

// sideEffect builds the design-318 side-effect annotation map for a runnable
// leaf (write = "true", read = "false").
func sideEffect(write bool) map[string]string {
	if write {
		return map[string]string{"anycli.side_effect": "true"}
	}
	return map[string]string{"anycli.side_effect": "false"}
}

// newGetLeaf builds a single-resource GET (read) leaf taking one positional id.
func (s *Service) newGetLeaf(key, region, use, short string, path func(id string) string) *cobra.Command {
	c := &cobra.Command{Use: use + " <id>", Short: short, Args: cobra.ExactArgs(1), Annotations: sideEffect(false)}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return s.runGet(cmd.Context(), key, region, "GET", path(args[0]), nil, nil)
	}
	return c
}

// newActionLeaf builds a bodyless PUT action (write) leaf taking one positional
// id (cancel/resume/collect).
func (s *Service) newActionLeaf(key, region, use, short string, path func(id string) string) *cobra.Command {
	c := &cobra.Command{Use: use + " <id>", Short: short, Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return s.runGet(cmd.Context(), key, region, "PUT", path(args[0]), nil, nil)
	}
	return c
}

// newBodyLeaf builds a write leaf taking one positional id plus a required
// --body JSON payload (subscription change).
func (s *Service) newBodyLeaf(key, region, use, short, method string, path func(id string) string) *cobra.Command {
	c := &cobra.Command{Use: use + " <id>", Short: short, Args: cobra.ExactArgs(1), Annotations: sideEffect(true)}
	body := c.Flags().String("body", "", "JSON body (required)")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		payload, err := requireBody(*body)
		if err != nil {
			return err
		}
		return s.runGet(cmd.Context(), key, region, method, path(args[0]), nil, payload)
	}
	return c
}

// accountScoped returns the account-scoped collection path when account is set,
// otherwise the top-level collection path. The tail is derived from the
// top-level path (e.g. "/subscriptions" → "/accounts/<code>/subscriptions").
func accountScoped(account, topLevel string) string {
	if account == "" {
		return topLevel
	}
	return "/accounts/" + account + topLevel
}

// requireBody validates a --body flag value: it must be present and valid JSON.
// Both failures are usage errors (exit 2) that must not reach the network.
func requireBody(raw string) ([]byte, error) {
	if raw == "" {
		return nil, &usageError{msg: "--body is required (JSON payload)"}
	}
	var probe json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, &usageError{msg: "--body is not valid JSON: " + err.Error()}
	}
	return []byte(raw), nil
}
