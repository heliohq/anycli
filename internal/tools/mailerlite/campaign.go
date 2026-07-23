package mailerlite

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCampaignCmd builds the `mailerlite campaign` command tree — the
// draft → schedule → check-report loop.
func (s *Service) newCampaignCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "campaign", Short: "Campaigns (list, get, create, update, schedule, cancel, delete, report)"}
	cmd.AddCommand(
		s.newCampaignListCmd(token),
		s.newCampaignGetCmd(token),
		s.newCampaignDataCmd(token, "create", "Create a campaign (POST /campaigns)", http.MethodPost, ""),
		s.newCampaignDataCmd(token, "update", "Update a campaign (PUT /campaigns/{id})", http.MethodPut, ""),
		s.newCampaignDataCmd(token, "schedule", "Schedule a campaign (POST /campaigns/{id}/schedule)", http.MethodPost, "/schedule"),
		s.newCampaignActionCmd(token, "cancel", "Cancel a scheduled campaign (POST /campaigns/{id}/cancel)", http.MethodPost, "/cancel"),
		s.newCampaignActionCmd(token, "delete", "Delete a campaign (DELETE /campaigns/{id})", http.MethodDelete, ""),
		s.newCampaignReportCmd(token),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(token string) *cobra.Command {
	var status, campaignType string
	var limit, page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List campaigns (GET /campaigns)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("filter[status]", status)
			}
			if campaignType != "" {
				q.Set("filter[type]", campaignType)
			}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status: sent|draft|ready")
	cmd.Flags().StringVar(&campaignType, "type", "", "filter by type: regular|ab|resend|rss")
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}

func (s *Service) newCampaignGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get a campaign (GET /campaigns/{id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

// newCampaignDataCmd builds a write command whose body is supplied as raw JSON
// via --data (the campaign create/update/schedule payloads carry nested email
// blocks and delivery config that resist flat flags). create takes no id; the
// others take an id and append suffix to the path.
func (s *Service) newCampaignDataCmd(token, use, short, method, suffix string) *cobra.Command {
	var data string
	takesID := use != "create"
	args := cobra.NoArgs
	if takesID {
		args = cobra.ExactArgs(1)
	}
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Annotations: writeAction,
		Args:        args,
		RunE: func(cmd *cobra.Command, cmdArgs []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			path := "/campaigns"
			if takesID {
				path += "/" + url.PathEscape(cmdArgs[0]) + suffix
			}
			resp, err := s.call(cmd.Context(), token, method, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	if takesID {
		cmd.Use = use + " <id>"
	}
	cmd.Flags().StringVar(&data, "data", "", "request body as a JSON object (required)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

// newCampaignActionCmd builds a bodyless action (cancel/delete) keyed on the
// campaign id.
func (s *Service) newCampaignActionCmd(token, use, short, method, suffix string) *cobra.Command {
	return &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, method, "/campaigns/"+url.PathEscape(args[0])+suffix, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newCampaignReportCmd(token string) *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use:         "report <id>",
		Short:       "Campaign subscriber-activity report (GET /campaigns/{id}/reports/subscriber-activity)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			setLimitPage(cmd, q, limit, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/campaigns/"+url.PathEscape(args[0])+"/reports/subscriber-activity", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "page size (default 25)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (starts at 1)")
	return cmd
}
