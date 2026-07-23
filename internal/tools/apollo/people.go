package apollo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newPeopleCmd builds the `people` group: net-new prospecting (search) and the
// credit-consuming enrichment step (enrich / bulk-enrich).
func (s *Service) newPeopleCmd(token string) *cobra.Command {
	cmd := newGroupCmd("people", "Find and enrich people (prospects)")
	cmd.AddCommand(
		s.newPeopleSearchCmd(token),
		s.newPeopleEnrichCmd(token),
		s.newPeopleBulkEnrichCmd(token),
	)
	return cmd
}

// newPeopleSearchCmd wraps POST /mixed_people/api_search. Returns matching
// people (no contact details until enriched). Master-API-key-gated: an OAuth
// token may receive 403 (surfaced with an access hint).
func (s *Service) newPeopleSearchCmd(token string) *cobra.Command {
	var body string
	var titles, seniorities, locations, orgDomains []string
	var q string
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search for people by title/seniority/location/company (POST /mixed_people/api_search)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStrSlice(b, "person_titles", titles)
			setStrSlice(b, "person_seniorities", seniorities)
			setStrSlice(b, "person_locations", locations)
			setStrSlice(b, "q_organization_domains_list", orgDomains)
			setStr(b, "q_keywords", q)
			applyPageBody(b, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/mixed_people/api_search", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&titles, "title", nil, "person title filter (repeatable)")
	cmd.Flags().StringArrayVar(&seniorities, "seniority", nil, "seniority filter, e.g. director|vp|c_suite (repeatable)")
	cmd.Flags().StringArrayVar(&locations, "location", nil, "person location filter (repeatable)")
	cmd.Flags().StringArrayVar(&orgDomains, "org-domain", nil, "employer company domain filter (repeatable)")
	cmd.Flags().StringVar(&q, "q", "", "free-text keyword filter")
	registerPageFlags(cmd, &page, &perPage)
	registerBodyFlag(cmd, &body)
	return cmd
}

// newPeopleEnrichCmd wraps POST /people/match — resolve one person to verified
// email/phone. Consumes credits when contact data is revealed.
func (s *Service) newPeopleEnrichCmd(token string) *cobra.Command {
	var body string
	var email, name, firstName, lastName, domain, orgName, linkedinURL, id string
	var revealPersonalEmails, revealPhone bool
	cmd := &cobra.Command{
		Use:         "enrich",
		Short:       "Enrich one person to verified email/phone (POST /people/match)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "email", email)
			setStr(b, "name", name)
			setStr(b, "first_name", firstName)
			setStr(b, "last_name", lastName)
			setStr(b, "domain", domain)
			setStr(b, "organization_name", orgName)
			setStr(b, "linkedin_url", linkedinURL)
			setStr(b, "id", id)
			if revealPersonalEmails {
				b["reveal_personal_emails"] = true
			}
			if revealPhone {
				b["reveal_phone_number"] = true
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/people/match", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "person email (strongest match signal)")
	cmd.Flags().StringVar(&name, "name", "", "full name")
	cmd.Flags().StringVar(&firstName, "first-name", "", "first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "last name")
	cmd.Flags().StringVar(&domain, "org-domain", "", "employer company domain (no www./@)")
	cmd.Flags().StringVar(&orgName, "org", "", "employer company name")
	cmd.Flags().StringVar(&linkedinURL, "linkedin-url", "", "LinkedIn profile URL")
	cmd.Flags().StringVar(&id, "id", "", "Apollo person id")
	cmd.Flags().BoolVar(&revealPersonalEmails, "reveal-personal-emails", false, "reveal personal emails (consumes credits)")
	cmd.Flags().BoolVar(&revealPhone, "reveal-phone", false, "reveal phone number (consumes credits; async via webhook)")
	registerBodyFlag(cmd, &body)
	return cmd
}

// newPeopleBulkEnrichCmd wraps POST /people/bulk_match — enrich up to 10 people
// in one call. Details are supplied as a raw JSON array via --details-json (the
// Apollo `details` field), since each entry mirrors the single-match schema.
func (s *Service) newPeopleBulkEnrichCmd(token string) *cobra.Command {
	var body, detailsJSON string
	var revealPersonalEmails, revealPhone bool
	cmd := &cobra.Command{
		Use:         "bulk-enrich",
		Short:       "Enrich up to 10 people in one call (POST /people/bulk_match)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			if detailsJSON != "" {
				v, err := decodeJSONArray("details-json", detailsJSON)
				if err != nil {
					return err
				}
				b["details"] = v
			}
			if revealPersonalEmails {
				b["reveal_personal_emails"] = true
			}
			if revealPhone {
				b["reveal_phone_number"] = true
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/people/bulk_match", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&detailsJSON, "details-json", "", "JSON array of up to 10 person match objects (Apollo `details`)")
	cmd.Flags().BoolVar(&revealPersonalEmails, "reveal-personal-emails", false, "reveal personal emails (consumes credits)")
	cmd.Flags().BoolVar(&revealPhone, "reveal-phone", false, "reveal phone numbers (consumes credits)")
	registerBodyFlag(cmd, &body)
	return cmd
}
