package freshbooks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// resourceSpec describes one FreshBooks accounting resource family: the CLI
// command word, the accounting path segment, the singular/plural JSON keys
// FreshBooks wraps payloads and results in, and which verbs it exposes.
type resourceSpec struct {
	use      string
	short    string
	pathSeg  string
	singular string
	plural   string
	verbs    verbSet
}

// verbSet flags which subcommands a resource exposes. FreshBooks does not offer
// every verb on every family (items are read-only; estimates/payments have no
// soft-delete/send here), so each spec opts in explicitly.
type verbSet struct {
	list   bool
	get    bool
	create bool
	update bool
	del    bool
	send   bool
}

// resourceSpecs is the fixed FreshBooks accounting surface an AI bookkeeper
// needs: resolve/create clients, run the invoice lifecycle (incl. send), log
// expenses, quote estimates, record payments, and read the billable-item
// catalog. Time-tracking/projects (keyed by business_id, not account_id) are
// out of scope.
var resourceSpecs = []resourceSpec{
	{use: "client", short: "Manage clients", pathSeg: "users/clients", singular: "client", plural: "clients",
		verbs: verbSet{list: true, get: true, create: true, update: true}},
	{use: "invoice", short: "Manage invoices", pathSeg: "invoices/invoices", singular: "invoice", plural: "invoices",
		verbs: verbSet{list: true, get: true, create: true, update: true, del: true, send: true}},
	{use: "expense", short: "Manage expenses", pathSeg: "expenses/expenses", singular: "expense", plural: "expenses",
		verbs: verbSet{list: true, get: true, create: true, update: true}},
	{use: "estimate", short: "Manage estimates", pathSeg: "estimates/estimates", singular: "estimate", plural: "estimates",
		verbs: verbSet{list: true, get: true, create: true}},
	{use: "payment", short: "Manage payments", pathSeg: "payments/payments", singular: "payment", plural: "payments",
		verbs: verbSet{list: true, get: true, create: true}},
	{use: "item", short: "Read billable items", pathSeg: "items/items", singular: "item", plural: "items",
		verbs: verbSet{list: true, get: true}},
}

// newResourceGroup builds the runnable command group for one resource family.
func (s *Service) newResourceGroup(token string, spec resourceSpec) *cobra.Command {
	group := &cobra.Command{
		Use:   spec.use,
		Short: spec.short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	if spec.verbs.list {
		group.AddCommand(s.newListCmd(token, spec))
	}
	if spec.verbs.get {
		group.AddCommand(s.newGetCmd(token, spec))
	}
	if spec.verbs.create {
		group.AddCommand(s.newCreateCmd(token, spec))
	}
	if spec.verbs.update {
		group.AddCommand(s.newUpdateCmd(token, spec))
	}
	if spec.verbs.del {
		group.AddCommand(s.newDeleteCmd(token, spec))
	}
	if spec.verbs.send {
		group.AddCommand(s.newSendCmd(token, spec))
	}
	return group
}

// sideEffect renders the anycli.side_effect annotation (design 318): reads are
// "false", writes are "true". Every runnable leaf must declare one.
func sideEffect(mutates bool) map[string]string {
	if mutates {
		return map[string]string{"anycli.side_effect": "true"}
	}
	return map[string]string{"anycli.side_effect": "false"}
}

// accountingPath builds /accounting/account/<accountId>/<pathSeg>[/<id>].
func accountingPath(accountID, pathSeg, id string) string {
	base := "/accounting/account/" + url.PathEscape(accountID) + "/" + pathSeg
	if id != "" {
		base += "/" + url.PathEscape(id)
	}
	return base
}

func (s *Service) newListCmd(token string, spec resourceSpec) *cobra.Command {
	var page, perPage int
	var queries []string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + spec.plural,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			account, err := s.resolveAccount(cmd.Context(), token, accountFlag(cmd))
			if err != nil {
				return err
			}
			q := url.Values{}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if perPage > 0 {
				q.Set("per_page", strconv.Itoa(perPage))
			}
			for _, raw := range queries {
				key, value, ok := strings.Cut(raw, "=")
				if !ok || strings.TrimSpace(key) == "" {
					return &usageError{msg: fmt.Sprintf("--query %q must be key=value", raw)}
				}
				q.Add(key, value)
			}
			path := accountingPath(account, spec.pathSeg, "")
			if encoded := q.Encode(); encoded != "" {
				path += "?" + encoded
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			env, err := unwrapList(body, spec.plural)
			if err != nil {
				return err
			}
			return s.emitJSON(env)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "results per page")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "extra query filter key=value (repeatable, e.g. search[customerid]=42)")
	return cmd
}

func (s *Service) newGetCmd(token string, spec resourceSpec) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one " + spec.singular,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := s.resolveAccount(cmd.Context(), token, accountFlag(cmd))
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, accountingPath(account, spec.pathSeg, args[0]), nil)
			if err != nil {
				return err
			}
			obj, err := unwrapObject(body, spec.singular)
			if err != nil {
				return err
			}
			return s.emitJSON(obj)
		},
	}
}

func (s *Service) newCreateCmd(token string, spec resourceSpec) *cobra.Command {
	var data, file string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a " + spec.singular,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := resourcePayload(spec.singular, data, file)
			if err != nil {
				return err
			}
			account, err := s.resolveAccount(cmd.Context(), token, accountFlag(cmd))
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, accountingPath(account, spec.pathSeg, ""), payload)
			if err != nil {
				return err
			}
			obj, err := unwrapObject(body, spec.singular)
			if err != nil {
				return err
			}
			return s.emitJSON(obj)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "resource fields as a JSON object")
	cmd.Flags().StringVar(&file, "file", "", "read resource JSON from a file (mutually exclusive with --data)")
	return cmd
}

func (s *Service) newUpdateCmd(token string, spec resourceSpec) *cobra.Command {
	var data, file string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a " + spec.singular,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := resourcePayload(spec.singular, data, file)
			if err != nil {
				return err
			}
			account, err := s.resolveAccount(cmd.Context(), token, accountFlag(cmd))
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, accountingPath(account, spec.pathSeg, args[0]), payload)
			if err != nil {
				return err
			}
			obj, err := unwrapObject(body, spec.singular)
			if err != nil {
				return err
			}
			return s.emitJSON(obj)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "fields to update as a JSON object")
	cmd.Flags().StringVar(&file, "file", "", "read update JSON from a file (mutually exclusive with --data)")
	return cmd
}

// newDeleteCmd soft-deletes via FreshBooks' vis_state=1 PUT (there is no DELETE
// verb on accounting resources).
func (s *Service) newDeleteCmd(token string, spec resourceSpec) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a " + spec.singular,
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := s.resolveAccount(cmd.Context(), token, accountFlag(cmd))
			if err != nil {
				return err
			}
			payload := map[string]any{spec.singular: map[string]any{"vis_state": 1}}
			body, err := s.call(cmd.Context(), token, http.MethodPut, accountingPath(account, spec.pathSeg, args[0]), payload)
			if err != nil {
				return err
			}
			obj, err := unwrapObject(body, spec.singular)
			if err != nil {
				return err
			}
			return s.emitJSON(obj)
		},
	}
}

// newSendCmd emails an invoice (action_email) to its client or explicit
// recipients.
func (s *Service) newSendCmd(token string, spec resourceSpec) *cobra.Command {
	var recipients []string
	cmd := &cobra.Command{
		Use:         "send <id>",
		Short:       "Email a " + spec.singular + " to its client",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := s.resolveAccount(cmd.Context(), token, accountFlag(cmd))
			if err != nil {
				return err
			}
			fields := map[string]any{"action_email": true}
			if len(recipients) > 0 {
				fields["email_recipients"] = recipients
			}
			payload := map[string]any{spec.singular: fields}
			body, err := s.call(cmd.Context(), token, http.MethodPut, accountingPath(account, spec.pathSeg, args[0]), payload)
			if err != nil {
				return err
			}
			obj, err := unwrapObject(body, spec.singular)
			if err != nil {
				return err
			}
			return s.emitJSON(obj)
		},
	}
	cmd.Flags().StringArrayVar(&recipients, "to", nil, "recipient email (repeatable; defaults to the client's email)")
	return cmd
}

// resourcePayload builds the {singular: {...fields}} wrapper FreshBooks expects
// on create/update from either --data or --file. The two are mutually exclusive
// and exactly one must be supplied; the value must be a JSON object.
func resourcePayload(singular, data, file string) (map[string]any, error) {
	if data != "" && file != "" {
		return nil, &usageError{msg: "--data and --file are mutually exclusive"}
	}
	raw := data
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --file %s: %v", file, err)}
		}
		raw = string(b)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, &usageError{msg: "one of --data or --file is required (a JSON object of fields)"}
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("resource JSON is not a valid object: %v", err)}
	}
	return map[string]any{singular: fields}, nil
}

// accountFlag reads the persistent --account flag from the root command.
func accountFlag(cmd *cobra.Command) string {
	v, _ := cmd.Root().PersistentFlags().GetString("account")
	return v
}
