package iterable

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newCampaignCmd groups the campaign verbs.
func (s *Service) newCampaignCmd(cred credential) *cobra.Command {
	cmd := newGroupCmd("campaign", "Inspect campaigns and their metrics")
	cmd.AddCommand(
		s.newCampaignListCmd(cred),
		s.newCampaignMetricsCmd(cred),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(cred credential) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all campaigns (GET /api/campaigns)",
		Long: "Returns every campaign in the project with its numeric `campaignId`, the\n" +
			"id that `campaign metrics` and `email send` reference. There are no\n" +
			"filters, so narrowing to one campaign means matching its name locally.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/campaigns", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newCampaignMetricsCmd(cred credential) *cobra.Command {
	var campaignID string
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Get metrics for a campaign (GET /api/campaigns/metrics?campaignId=…)",
		Long: "--campaign-id is required and is the numeric id from `campaign list`. It\n" +
			"reports aggregate counts for the whole campaign — sends, opens, clicks,\n" +
			"unsubscribes. It says nothing about an individual recipient; for one\n" +
			"person's delivery history use `event list` instead.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if campaignID == "" {
				return &usageError{msg: "iterable: --campaign-id is required"}
			}
			query := url.Values{"campaignId": {campaignID}}
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/campaigns/metrics", query, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "campaign id (required)")
	_ = cmd.MarkFlagRequired("campaign-id")
	return cmd
}
