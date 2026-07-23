package hunter

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newLeadCmd groups the Leads CRUD (GET/POST/PUT/DELETE /leads[/:id]). Free.
func (s *Service) newLeadCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "lead", Short: "Leads (list, get, create, update, delete)"}
	cmd.AddCommand(
		s.newLeadListSubCmd(key),
		s.newLeadGetCmd(key),
		s.newLeadCreateCmd(key),
		s.newLeadUpdateCmd(key),
		s.newLeadDeleteCmd(key),
	)
	return cmd
}

// newLeadListSubCmd wraps GET /leads. Filter flags pass through Hunter's own
// string / wildcard filter semantics verbatim.
func (s *Service) newLeadListSubCmd(key string) *cobra.Command {
	var leadsListID, email, firstName, lastName, company, query string
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List leads (GET /leads)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "leads_list_id", leadsListID)
			setIf(q, "email", email)
			setIf(q, "first_name", firstName)
			setIf(q, "last_name", lastName)
			setIf(q, "company", company)
			setIf(q, "query", query)
			if cmd.Flags().Changed("limit") {
				q.Set("limit", itoa(limit))
			}
			if cmd.Flags().Changed("offset") {
				q.Set("offset", itoa(offset))
			}
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/leads", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&leadsListID, "leads-list-id", "", "filter by leads list id")
	cmd.Flags().StringVar(&email, "email", "", "filter by email")
	cmd.Flags().StringVar(&firstName, "first-name", "", "filter by first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "filter by last name")
	cmd.Flags().StringVar(&company, "company", "", "filter by company")
	cmd.Flags().StringVar(&query, "query", "", "full-text search across lead fields")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size (1-1000, default 20)")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}

func (s *Service) newLeadGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one lead (GET /leads/{id})",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/leads/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadCreateCmd(key string) *cobra.Command {
	var email, firstName, lastName, position, company, website, countryCode, linkedinURL, phoneNumber, twitter, leadsListID, attributes string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a lead (POST /leads)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := leadBody(email, firstName, lastName, position, company, website, countryCode, linkedinURL, phoneNumber, twitter, leadsListID, attributes)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/leads", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	bindLeadFlags(cmd, &email, &firstName, &lastName, &position, &company, &website, &countryCode, &linkedinURL, &phoneNumber, &twitter, &leadsListID, &attributes)
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newLeadUpdateCmd(key string) *cobra.Command {
	var id, email, firstName, lastName, position, company, website, countryCode, linkedinURL, phoneNumber, twitter, leadsListID, attributes string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a lead (PUT /leads/{id})",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := leadBody(email, firstName, lastName, position, company, website, countryCode, linkedinURL, phoneNumber, twitter, leadsListID, attributes)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPut, "/leads/"+url.PathEscape(id), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead id")
	bindLeadFlags(cmd, &email, &firstName, &lastName, &position, &company, &website, &countryCode, &linkedinURL, &phoneNumber, &twitter, &leadsListID, &attributes)
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadDeleteCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a lead (DELETE /leads/{id})",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/leads/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			// DELETE returns 204 with an empty body; emit a small receipt.
			if len(resp) == 0 {
				return s.emit([]byte(`{"deleted":true}`))
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// bindLeadFlags declares the shared lead field flags on create/update.
func bindLeadFlags(cmd *cobra.Command, email, firstName, lastName, position, company, website, countryCode, linkedinURL, phoneNumber, twitter, leadsListID, attributes *string) {
	bindStringFlags(cmd, []stringFlag{
		{email, "email", "lead email address"},
		{firstName, "first-name", "lead first name"},
		{lastName, "last-name", "lead last name"},
		{position, "position", "job position"},
		{company, "company", "company name"},
		{website, "website", "company website"},
		{countryCode, "country-code", "ISO 3166-1 alpha-2 country code"},
		{linkedinURL, "linkedin-url", "LinkedIn profile URL"},
		{phoneNumber, "phone-number", "phone number"},
		{twitter, "twitter", "Twitter handle"},
		{leadsListID, "leads-list-id", "leads list id to attach the lead to"},
		{attributes, "attributes", "raw JSON object of additional/custom lead fields"},
	})
}

// leadBody assembles the create/update request body, merging --attributes raw
// JSON last so explicit flags take precedence over overlapping keys.
func leadBody(email, firstName, lastName, position, company, website, countryCode, linkedinURL, phoneNumber, twitter, leadsListID, attributes string) (map[string]any, error) {
	body := map[string]any{}
	if attributes != "" {
		merged, err := decodeJSONObjectFlag("attributes", attributes)
		if err != nil {
			return nil, err
		}
		for k, v := range merged {
			body[k] = v
		}
	}
	setBodyIf(body, "email", email)
	setBodyIf(body, "first_name", firstName)
	setBodyIf(body, "last_name", lastName)
	setBodyIf(body, "position", position)
	setBodyIf(body, "company", company)
	setBodyIf(body, "website", website)
	setBodyIf(body, "country_code", countryCode)
	setBodyIf(body, "linkedin_url", linkedinURL)
	setBodyIf(body, "phone_number", phoneNumber)
	setBodyIf(body, "twitter", twitter)
	setBodyIf(body, "leads_list_id", leadsListID)
	return body, nil
}
