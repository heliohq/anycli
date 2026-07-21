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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
