package intercom

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCompanyCmd builds the company resource group: the account records a
// contact belongs to. Intercom creates and updates companies through the same
// upsert endpoint (POST /companies) keyed by company_id.
func (s *Service) newCompanyCmd(token string) *cobra.Command {
	cmd := newGroupCmd("company", "Companies: list, get, upsert")
	cmd.AddCommand(
		s.newCompanyListCmd(token),
		s.newCompanyGetCmd(token),
		s.newCompanyUpsertCmd(token),
	)
	return cmd
}

func (s *Service) newCompanyListCmd(token string) *cobra.Command {
	var perPage int
	var page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List companies (GET /companies)",
		Long: "Page-numbered with --page, not cursor-based like the contact and\n" +
			"conversation lists, so there is no `starting_after` to feed back. Returns\n" +
			"Intercom's internal company ids, which is what `company get` needs.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if perPage > 0 {
				q.Set("per_page", intToString(perPage))
			}
			if page > 0 {
				q.Set("page", intToString(page))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/companies", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&perPage, "per-page", 0, "results per page")
	cmd.Flags().IntVar(&page, "page", 0, "page number (companies list is page-numbered)")
	return cmd
}

func (s *Service) newCompanyGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one company by Intercom id (GET /companies/{id})",
		Long: "Takes Intercom's INTERNAL company id, not the external `company_id` that\n" +
			"`company upsert` keys on — the two are different identifiers and passing\n" +
			"the external one here returns a 404. Internal ids come from `company list`\n" +
			"or from the companies attached to a contact.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/companies/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Intercom company id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCompanyUpsertCmd(token string) *cobra.Command {
	var companyID, name, bodyJSON string
	cmd := &cobra.Command{
		Use:   "upsert",
		Short: "Create or update a company by company_id (POST /companies)",
		Long: "Intercom has no separate company-create and company-update endpoints: this\n" +
			"one call keys on --company-id, your own external identifier, creating the\n" +
			"record the first time that value is seen and updating it afterwards. Only\n" +
			"the fields sent are touched, and --body-json merges over the scalar flags\n" +
			"for custom attributes and plan or seat data the flags do not cover.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{}
			if companyID != "" {
				payload["company_id"] = companyID
			}
			if name != "" {
				payload["name"] = name
			}
			if err := mergeBodyJSON(payload, bodyJSON); err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/companies", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&companyID, "company-id", "", "your system's company_id (upsert key)")
	cmd.Flags().StringVar(&name, "name", "", "company name")
	cmd.Flags().StringVar(&bodyJSON, "body-json", "", "raw company JSON (merged; overrides the scalar flags)")
	_ = cmd.MarkFlagRequired("company-id")
	return cmd
}
