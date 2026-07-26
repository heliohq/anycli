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
		Long: "A POST rather than a GET, because its filter body is complex — and that is\n" +
			"why --limit and --starting-after ride the BODY here instead of the query\n" +
			"string as they do on every other list in this tool. --campaign, --list-id\n" +
			"and --search cover the common cuts, with --search matching name, email\n" +
			"and company. Anything richer goes in --data as the raw filter body, over\n" +
			"which the typed flags win.",
		Args: cobra.NoArgs,
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
		Long: "--id is the lead's own id, from `lead list` — there is no lookup by email\n" +
			"address here, and `lead update-interest` is the only command in the group\n" +
			"keyed on one. Returns the lead with its custom variables, which are what\n" +
			"the campaign sequence's merge tags render from.",
		Args: cobra.NoArgs,
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
		Long: "The single-lead path; `lead add` is the bulk one and takes up to 1000.\n" +
			"--email plus one of --campaign or --list-id is the useful minimum, and\n" +
			"--first-name, --last-name and --company-name fill merge fields the\n" +
			"sequence renders. Any other field goes through --data, over which the\n" +
			"typed flags win. Creating a lead directly in an ACTIVE campaign enrols it\n" +
			"in the sequence, so outreach follows without a further command.",
		Args: cobra.NoArgs,
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
		Long: "--id is required and --data is the raw patch body; there are no per-field\n" +
			"flags, so read the lead with `lead get` first. Custom variables written\n" +
			"here are what the sequence's merge tags render, so a misspelled variable\n" +
			"name surfaces as an empty field in a real email rather than as an error.",
		Args: cobra.NoArgs,
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
		Long: "--id is required. This removes the record rather than stopping outreach to\n" +
			"the person — `lead update-interest` or pausing the campaign keeps the\n" +
			"history. Nothing here restores a deleted lead, and re-adding the same\n" +
			"address starts the sequence again from step one.",
		Args: cobra.NoArgs,
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
		Long: "The bulk path, up to 1000 leads per call. The `leads` array is required\n" +
			"and has NO flag — it must come through --data, e.g.\n" +
			"`{\"leads\":[{\"email\":\"a@b.com\"}]}` — while --campaign-id and --list-id\n" +
			"are conveniences merged over that body. Adding into a campaign that is\n" +
			"already active starts sending to every one of them at the next schedule\n" +
			"window, so stage into a lead list first when that is not intended.",
		Args: cobra.NoArgs,
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
		Long: "Returns a background JOB, not a result: nothing has moved when this\n" +
			"returns, so poll `job get --id <job-id>` before assuming the leads\n" +
			"landed. --to-campaign-id or --to-list-id names the destination, and which\n" +
			"leads move comes from --data (an ids array, or a filter). Moving leads\n" +
			"INTO an active campaign starts sending to them.",
		Args: cobra.NoArgs,
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
		Long: "Keyed on --lead-email, not a lead id — the one command in this group\n" +
			"addressed by address. --interest-value is a required numeric code (1 for\n" +
			"interested, -1 for not interested, plus whatever else the workspace\n" +
			"configures), never a free-text label. --campaign-id scopes it when the\n" +
			"same address sits in several campaigns; without it the scope is whatever\n" +
			"the provider picks.",
		Args: cobra.NoArgs,
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
