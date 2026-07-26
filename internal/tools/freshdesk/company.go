package freshdesk

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newCompanyCmd(c *client) *cobra.Command {
	cmd := &cobra.Command{Use: "company", Short: "Companies (list, get, search)"}
	cmd.AddCommand(
		s.newCompanyListCmd(c),
		s.newCompanyGetCmd(c),
		s.newCompanySearchCmd(c),
	)
	return cmd
}

func (s *Service) newCompanyListCmd(c *client) *cobra.Command {
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List companies (GET /companies)",
		Long: "Takes no filter at all beyond --page / --per-page, so finding one company\n" +
			"by name means `company search`. Companies are the B2B accounts a contact's\n" +
			"`company_id` points at, not a population of people.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyPaging(q, page, perPage)
			resp, err := c.call(cmd.Context(), http.MethodGet, "/companies", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	registerPagingFlags(cmd, &page, &perPage)
	return cmd
}

func (s *Service) newCompanyGetCmd(c *client) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a company (GET /companies/{id})",
		Long: "Takes the numeric company id, usually read off a contact's or a ticket's\n" +
			"`company_id`. Returns the account's own fields and custom fields; it does\n" +
			"NOT return the company's tickets or its people — those are\n" +
			"`ticket list --company-id` and `contact list --company-id`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := c.call(cmd.Context(), http.MethodGet, "/companies/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "company id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCompanySearchCmd(c *client) *cobra.Command {
	var query string
	var page int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search companies (GET /search/companies). --query is Freshdesk query syntax.",
		Long: "--query is Freshdesk query syntax with inner quotes around string values,\n" +
			"e.g. \"name:'Acme'\". Same ceiling as the other searches: 30 rows a page,\n" +
			"no --per-page, and --page stops at 10.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("query", quoteQuery(query))
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			resp, err := c.call(cmd.Context(), http.MethodGet, "/search/companies", q, nil)
			if err != nil {
				return err
			}
			return c.emit(resp)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Freshdesk query, e.g. \"name:'Acme'\"")
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-10; search is capped at 10 pages)")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}
