package instantly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLeadCmd(token string) *cobra.Command {
	cmd := newGroupCmd("lead", "Leads (list, get, create, update, delete, add, move, interest)")
	cmd.AddCommand(
		s.newLeadListSubCmd(token),
		s.newLeadGetCmd(token),
		s.newLeadCreateCmd(token),
		s.newLeadUpdateCmd(token),
		s.newLeadDeleteCmd(token),
		s.newLeadAddCmd(token),
		s.newLeadMoveCmd(token),
		s.newLeadInterestCmd(token),
	)
	return cmd
}

// newLeadListSubCmd wraps POST /leads/list — a POST because of its complex
// filter body (documented REST deviation). Pagination rides the body, not the
// query.
func (s *Service) newLeadListSubCmd(token string) *cobra.Command {
	var page pageFlags
	var campaign, listID, search, data string
	cmd := &cobra.Command{
		Use:         "list",
		Annotations: readOnly,
		Short:       "List/search leads (POST /leads/list)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			page.applyBody(body)
			if cmd.Flags().Changed("campaign") {
				body["campaign"] = campaign
			}
			if cmd.Flags().Changed("list-id") {
				body["list_id"] = listID
			}
			if cmd.Flags().Changed("search") {
				body["search"] = search
			}
			return s.send(cmd, token, http.MethodPost, "/leads/list", body)
		},
	}
	registerPageFlags(cmd, &page)
	cmd.Flags().StringVar(&campaign, "campaign", "", "filter by campaign id")
	cmd.Flags().StringVar(&listID, "list-id", "", "filter by lead-list id")
	cmd.Flags().StringVar(&search, "search", "", "free-text search (name/email/company)")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON filter body (merged; flags override)")
	return cmd
}

func (s *Service) newLeadGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Annotations: readOnly,
		Short:       "Get a lead (GET /leads/{id})",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.get(cmd, token, "/leads/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadCreateCmd(token string) *cobra.Command {
	var data, email, campaign, listID, firstName, lastName, companyName string
	cmd := &cobra.Command{
		Use:         "create",
		Annotations: writeAction,
		Short:       "Create a single lead (POST /leads)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			setBodyIfChanged(cmd, body, "email", "email", email)
			setBodyIfChanged(cmd, body, "campaign", "campaign", campaign)
			setBodyIfChanged(cmd, body, "list-id", "list_id", listID)
			setBodyIfChanged(cmd, body, "first-name", "first_name", firstName)
			setBodyIfChanged(cmd, body, "last-name", "last_name", lastName)
			setBodyIfChanged(cmd, body, "company-name", "company_name", companyName)
			return s.send(cmd, token, http.MethodPost, "/leads", body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON lead body (merged; flags override)")
	cmd.Flags().StringVar(&email, "email", "", "lead email")
	cmd.Flags().StringVar(&campaign, "campaign", "", "campaign id to add the lead to")
	cmd.Flags().StringVar(&listID, "list-id", "", "lead-list id to add the lead to")
	cmd.Flags().StringVar(&firstName, "first-name", "", "lead first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "lead last name")
	cmd.Flags().StringVar(&companyName, "company-name", "", "lead company name")
	return cmd
}

func (s *Service) newLeadUpdateCmd(token string) *cobra.Command {
	var id, data string
	cmd := &cobra.Command{
		Use:         "update",
		Annotations: writeAction,
		Short:       "Update a lead (PATCH /leads/{id}). --data is the raw JSON body",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			return s.send(cmd, token, http.MethodPatch, "/leads/"+url.PathEscape(id), body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead id")
	cmd.Flags().StringVar(&data, "data", "", "raw JSON patch body")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newLeadDeleteCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "delete",
		Annotations: writeAction,
		Short:       "Delete a lead (DELETE /leads/{id})",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.send(cmd, token, http.MethodDelete, "/leads/"+url.PathEscape(id), nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "lead id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newLeadAddCmd wraps POST /leads/add — bulk-add up to 1000 leads to a campaign
// or list. The leads array must be supplied via --data (it is the required
// field); --campaign-id / --list-id are convenience overrides.
func (s *Service) newLeadAddCmd(token string) *cobra.Command {
	var data, campaignID, listID string
	cmd := &cobra.Command{
		Use:         "add",
		Annotations: writeAction,
		Short:       "Bulk-add leads to a campaign or list (POST /leads/add)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			setBodyIfChanged(cmd, body, "campaign-id", "campaign_id", campaignID)
			setBodyIfChanged(cmd, body, "list-id", "list_id", listID)
			return s.send(cmd, token, http.MethodPost, "/leads/add", body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", `raw JSON body incl. the required "leads" array`)
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "destination campaign id")
	cmd.Flags().StringVar(&listID, "list-id", "", "destination lead-list id")
	return cmd
}

// newLeadMoveCmd wraps POST /leads/move — a background job (poll via `job get`).
func (s *Service) newLeadMoveCmd(token string) *cobra.Command {
	var data, toCampaignID, toListID string
	cmd := &cobra.Command{
		Use:         "move",
		Annotations: writeAction,
		Short:       "Move leads between campaigns/lists (POST /leads/move; returns a background job)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeDataFlag(data)
			if err != nil {
				return err
			}
			setBodyIfChanged(cmd, body, "to-campaign-id", "to_campaign_id", toCampaignID)
			setBodyIfChanged(cmd, body, "to-list-id", "to_list_id", toListID)
			return s.send(cmd, token, http.MethodPost, "/leads/move", body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON selector body (ids/filter/campaign/list)")
	cmd.Flags().StringVar(&toCampaignID, "to-campaign-id", "", "destination campaign id")
	cmd.Flags().StringVar(&toListID, "to-list-id", "", "destination lead-list id")
	return cmd
}

func (s *Service) newLeadInterestCmd(token string) *cobra.Command {
	var leadEmail, campaignID string
	var interestValue int
	cmd := &cobra.Command{
		Use:         "update-interest",
		Annotations: writeAction,
		Short:       "Set a lead's interest status (POST /leads/update-interest-status)",
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"lead_email":     leadEmail,
				"interest_value": interestValue,
			}
			if cmd.Flags().Changed("campaign-id") {
				body["campaign_id"] = campaignID
			}
			return s.send(cmd, token, http.MethodPost, "/leads/update-interest-status", body)
		},
	}
	cmd.Flags().StringVar(&leadEmail, "lead-email", "", "lead email address")
	cmd.Flags().IntVar(&interestValue, "interest-value", 0, "interest status code (e.g. 1=interested, -1=not interested)")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign id scope (optional)")
	_ = cmd.MarkFlagRequired("lead-email")
	_ = cmd.MarkFlagRequired("interest-value")
	return cmd
}
