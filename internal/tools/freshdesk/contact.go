package freshdesk

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newContactCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{Use: "contact", Short: "Contacts (list, get, create, update, search)"}
	cmd.AddCommand(
		s.newContactListCmd(c),
		s.newContactGetCmd(c),
		s.newContactCreateCmd(c),
		s.newContactUpdateCmd(c),
		s.newContactSearchCmd(c),
	)
	return cmd
}

func (s *Service) newContactListCmd(c *client) *cobra.Command {
	var email, companyID, updatedSince string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts (GET /contacts)",
		Long: "The filters are exact-match only — --email, --company-id,\n" +
			"--updated-since — with no name matching and no partial matching;\n" +
			"`contact search` is the path for those. Contacts are requesters, a\n" +
			"different population from the staff `agent list` returns.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setNonEmpty(q, "email", email)
			setNonEmpty(q, "company_id", companyID)
			setNonEmpty(q, "_updated_since", updatedSince)
			applyPaging(q, page, perPage)
			resp, err := c.call(cmd.Context(), http.MethodGet, "/contacts", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by email")
	cmd.Flags().StringVar(&companyID, "company-id", "", "filter by company id")
	cmd.Flags().StringVar(&updatedSince, "updated-since", "", "ISO-8601 updated-since timestamp")
	registerPagingFlags(cmd, &page, &perPage)
	return cmd
}

func (s *Service) newContactGetCmd(c *client) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a contact (GET /contacts/{id})",
		Long: "Takes the numeric contact id, never an email address; resolve one with\n" +
			"`contact search --query \"email:'someone@example.com'\"`. The response\n" +
			"carries the `company_id` that a B2B ticket's account context hangs off,\n" +
			"and `other_emails`, which is where a second address for the same person\n" +
			"lives.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := c.call(cmd.Context(), http.MethodGet, "/contacts/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newContactCreateCmd(c *client) *cobra.Command {
	var name, email, phone, mobile, companyID, customFieldsJSON string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact (POST /contacts)",
		Long: "Freshdesk wants a name plus at least one way to reach the person — email,\n" +
			"phone or mobile — and rejects an email another contact already holds with\n" +
			"a validation error rather than returning that contact, so search before\n" +
			"creating. `ticket create --email` already creates a contact implicitly for\n" +
			"an unknown address, which makes this the command for filling in the\n" +
			"details that flow cannot capture.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			setBodyStr(body, "name", name)
			setBodyStr(body, "email", email)
			setBodyStr(body, "phone", phone)
			setBodyStr(body, "mobile", mobile)
			setBodyInt(body, "company_id", companyID)
			if err := applyCustomFields(body, customFieldsJSON); err != nil {
				return err
			}
			resp, err := c.call(cmd.Context(), http.MethodPost, "/contacts", nil, body)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "contact name")
	cmd.Flags().StringVar(&email, "email", "", "contact email")
	cmd.Flags().StringVar(&phone, "phone", "", "work phone")
	cmd.Flags().StringVar(&mobile, "mobile", "", "mobile phone")
	cmd.Flags().StringVar(&companyID, "company-id", "", "company id")
	cmd.Flags().StringVar(&customFieldsJSON, "custom-fields", "", "custom fields JSON object (raw passthrough)")
	return cmd
}

func (s *Service) newContactUpdateCmd(c *client) *cobra.Command {
	var id, name, email, phone, mobile, companyID, customFieldsJSON string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a contact (PUT /contacts/{id})",
		Long: "Only flags actually passed are sent, so omitted fields keep their current\n" +
			"values despite the PUT. Changing --email re-keys which contact future mail\n" +
			"from that address attaches to, and fails if another contact already holds\n" +
			"it. Setting --company-id is what puts the person under a B2B account.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			setBodyStr(body, "name", name)
			setBodyStr(body, "email", email)
			setBodyStr(body, "phone", phone)
			setBodyStr(body, "mobile", mobile)
			setBodyInt(body, "company_id", companyID)
			if err := applyCustomFields(body, customFieldsJSON); err != nil {
				return err
			}
			resp, err := c.call(cmd.Context(), http.MethodPut, "/contacts/"+url.PathEscape(id), nil, body)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	cmd.Flags().StringVar(&name, "name", "", "contact name")
	cmd.Flags().StringVar(&email, "email", "", "contact email")
	cmd.Flags().StringVar(&phone, "phone", "", "work phone")
	cmd.Flags().StringVar(&mobile, "mobile", "", "mobile phone")
	cmd.Flags().StringVar(&companyID, "company-id", "", "company id")
	cmd.Flags().StringVar(&customFieldsJSON, "custom-fields", "", "custom fields JSON object (raw passthrough)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newContactSearchCmd(c *client) *cobra.Command {
	var query string
	var page int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search contacts (GET /search/contacts). --query is Freshdesk query syntax.",
		Long: "--query is Freshdesk query syntax with inner quotes around string values:\n" +
			"\"email:'jane@acme.com'\", \"name:'Jane'\", \"company_id:42\". This is the only\n" +
			"way to find a contact by name. Results are 30 to a page with no\n" +
			"--per-page, and --page stops at 10.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("query", quoteQuery(query))
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			resp, err := c.call(cmd.Context(), http.MethodGet, "/search/contacts", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Freshdesk query, e.g. \"email:'jane@acme.com'\"")
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-10; search is capped at 10 pages)")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}
