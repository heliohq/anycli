package billcom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// resourceDocs carries one resource's per-leaf Long texts. One constructor
// builds all five resource groups, so the prose cannot live at the builder —
// a bill, an invoice and a payment mean different things and each leaf has to
// say its own.
type resourceDocs struct {
	list   string
	get    string
	create string
}

var billDocs = resourceDocs{
	list: "Bills are the payable side: what the org owes its vendors. Filter with\n" +
		"repeatable `--filter field:op:value` — due date, vendor id and approval or\n" +
		"payment status are the useful axes for \"what is outstanding\" — and order\n" +
		"with `--sort field:asc`. The rows report what is owed and whether it is\n" +
		"paid; they never move money.",
	get: "Takes the BILL bill id (the `id` from `bill list`), and returns the\n" +
		"provider's full record — line items, amounts, due date, vendor reference\n" +
		"and approval state — rather than the trimmed list row. Use it before\n" +
		"reporting an amount, since list entries may omit line-item detail.",
	create: "Drafts a payable record: the org now owes the vendor. It does not pay,\n" +
		"schedule or approve anything. `--data` is a JSON object handed to BILL\n" +
		"unchanged, so it must use BILL's own field names (a vendor id, an invoice\n" +
		"number, a due date, line items) — anycli checks only that it parses. The\n" +
		"vendor must already exist; create it with `vendor create` first and reuse\n" +
		"the id from `vendor list`.",
}

var vendorDocs = resourceDocs{
	list: "Vendors are the parties the org pays. Every bill hangs off a vendor id, so\n" +
		"this is the lookup that turns a supplier name into the id `bill create`\n" +
		"needs. `--filter field:op:value` is repeatable and passed to BILL verbatim.",
	get: "Takes the BILL vendor id from `vendor list` and returns the provider's full\n" +
		"vendor record, including the contact and payment-method details a list row\n" +
		"leaves out. It does not return the vendor's bills — filter `bill list` by\n" +
		"the vendor id for those.",
	create: "`--data` is a JSON object in BILL's own vendor shape, passed through\n" +
		"unchanged; anycli validates only that it parses. Check `vendor list` first\n" +
		"— nothing here merges duplicates, so a second record for an existing\n" +
		"supplier will split that supplier's bills across two ids.",
}

var invoiceDocs = resourceDocs{
	list: "Invoices are the receivable side: what customers owe the org. Filter with\n" +
		"repeatable `--filter field:op:value` (due date, customer id, paid status)\n" +
		"and order with `--sort field:asc`. For what the org owes instead, the\n" +
		"payable equivalent is `bill list`.",
	get: "Takes the BILL invoice id from `invoice list` and returns the provider's\n" +
		"full record — line items, amounts, due date and customer reference. The\n" +
		"amount still outstanding lives here, not in the payment records.",
	create: "Drafts a receivable: the customer now owes the org. It does not send,\n" +
		"email or charge anything. `--data` is a JSON object in BILL's own invoice\n" +
		"shape (a customer id, an invoice number, a due date, line items), passed\n" +
		"through unchanged. The customer must exist first — create one with\n" +
		"`customer create` or reuse an id from `customer list`.",
}

var customerDocs = resourceDocs{
	list: "Customers are the parties that pay the org. Every invoice hangs off a\n" +
		"customer id, so this is the lookup that turns a company name into the id\n" +
		"`invoice create` needs. `--filter field:op:value` is repeatable and passed\n" +
		"to BILL verbatim.",
	get: "Takes the BILL customer id from `customer list` and returns the provider's\n" +
		"full customer record, including billing address and contact fields a list\n" +
		"row omits. It does not return the customer's invoices — filter\n" +
		"`invoice list` by the customer id for those.",
	create: "`--data` is a JSON object in BILL's own customer shape, passed through\n" +
		"unchanged; anycli validates only that it parses. Look in `customer list`\n" +
		"first — a duplicate record splits one customer's invoices across two ids\n" +
		"and nothing here merges them.",
}

var paymentDocs = resourceDocs{
	list: "Payments are read-only: this reports money that already moved, and no\n" +
		"command creates or schedules one. Filter with repeatable\n" +
		"`--filter field:op:value` to reconcile against bills — a bill's paid state\n" +
		"is on the bill record, while the disbursement itself is here.",
	get: "Takes the BILL payment id from `payment list` and returns the provider's\n" +
		"full record: amount, method, status and the bills the payment was applied\n" +
		"to. Read-only — a payment cannot be created, edited, approved or voided\n" +
		"through this tool.",
}

// newResourceGroup builds a resource command group (list/get[/create]) over the
// given v3 collection path. allowCreate is false for read-only resources such as
// payments (money-movement carve-out). docs carries that resource's leaf Longs.
func (s *Service) newResourceGroup(c *client, name, path string, allowCreate bool, docs resourceDocs) *cobra.Command {
	g := newGroupCmd(name, "Manage "+name+"s")
	g.AddCommand(s.newListCmd(c, name, path, docs.list), s.newGetCmd(c, name, path, docs.get))
	if allowCreate {
		g.AddCommand(s.newCreateCmd(c, name, path, docs.create))
	}
	return g
}

// newListCmd builds `<resource> list`, exposing BILL's cursor pagination
// (--max / --page) plus passthrough --filter / --sort, and normalizing BILL's
// {results,nextPage} into a provider-neutral {items,next_page} envelope.
func (s *Service) newListCmd(c *client, name, path, long string) *cobra.Command {
	var (
		max     int
		page    string
		filters []string
		sort    string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + name + "s",
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if max > 0 {
				q.Set("max", strconv.Itoa(max))
			}
			if page != "" {
				q.Set("page", page)
			}
			for _, f := range filters {
				q.Add("filters", f)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			body, err := c.do(cmd.Context(), http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().IntVar(&max, "max", 0, "maximum number of results to return")
	cmd.Flags().StringVar(&page, "page", "", "pagination token (the next_page value from a prior call)")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "BILL filter, e.g. field:op:value (repeatable)")
	cmd.Flags().StringVar(&sort, "sort", "", "BILL sort expression, e.g. field:asc")
	return cmd
}

// newGetCmd builds `<resource> get <id>`.
func (s *Service) newGetCmd(c *client, name, path, long string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one " + name + " by id",
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := c.do(cmd.Context(), http.MethodGet, path+"/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
}

// newCreateCmd builds `<resource> create --data <json>`, posting the supplied
// JSON object as the request body.
func (s *Service) newCreateCmd(c *client, name, path, long string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a " + name + " from a JSON body (--data)",
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if data == "" {
				return &usageError{msg: "create requires --data with a JSON object body"}
			}
			var payload any
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				return &usageError{msg: fmt.Sprintf("invalid --data JSON: %v", err)}
			}
			body, err := c.do(cmd.Context(), http.MethodPost, path, nil, payload)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "JSON object body for the new "+name)
	return cmd
}

// newOrgCmd builds `org list` — the login organizations for these credentials.
func (s *Service) newOrgCmd(c *client) *cobra.Command {
	g := newGroupCmd("org", "List login organizations")
	g.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List organizations available to this login",
		Long: "Shows every BILL organization the stored login can reach, which is how to\n" +
			"tell whether bills that seem missing simply live under another entity.\n" +
			"Informational only: the session is pinned to the organization stored with\n" +
			"the credential and no flag re-targets it, so working in a different one\n" +
			"means reconnecting with that organization.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.do(cmd.Context(), http.MethodGet, "/organizations", nil, nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	})
	return g
}

// newWhoamiCmd builds `whoami` — getsessioninfo (org id, user id, MFA status).
func (s *Service) newWhoamiCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the current BILL session info (org id, user id, MFA status)",
		Long: "The cheapest way to confirm the credential works and to learn which\n" +
			"organization and user every other command is acting as. The MFA status it\n" +
			"reports explains the payment carve-out: an untrusted session is exactly\n" +
			"what BILL refuses money-movement operations on.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.do(cmd.Context(), http.MethodGet, "/login/session", nil, nil)
			if err != nil {
				return err
			}
			return s.emitRaw(body)
		},
	}
}

// emitRaw writes a provider JSON response to stdout verbatim.
func (s *Service) emitRaw(body []byte) error {
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

// emitList normalizes BILL's {results,nextPage} list response into a
// provider-neutral {items,next_page} envelope.
func (s *Service) emitList(body []byte) error {
	var in struct {
		Results  json.RawMessage `json:"results"`
		NextPage string          `json:"nextPage"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return &apiError{msg: fmt.Sprintf("bill-com: decode list response: %v", err), err: err}
	}
	items := in.Results
	if len(items) == 0 {
		items = json.RawMessage("[]")
	}
	out, err := json.Marshal(struct {
		Items    json.RawMessage `json:"items"`
		NextPage string          `json:"next_page"`
	}{Items: items, NextPage: in.NextPage})
	if err != nil {
		return &apiError{msg: fmt.Sprintf("bill-com: encode list envelope: %v", err), err: err}
	}
	return s.emitRaw(out)
}
