package mailjet

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newStatCmd groups delivery/engagement statistics. `counters` reads the
// multi-source /v3/REST/statcounters aggregate; `recipient-esp` reads
// per-mailbox-provider deliverability for one campaign.
func (s *Service) newStatCmd(basic string) *cobra.Command {
	cmd := newGroupCmd("stat", "Delivery and engagement statistics (counters, recipient-esp)")
	cmd.AddCommand(
		s.newStatCountersCmd(basic),
		s.newStatRecipientESPCmd(basic),
	)
	return cmd
}

// newStatCountersCmd queries /v3/REST/statcounters. The Source axis
// (API key / campaign / list) is selected by CounterSource + SourceId.
func (s *Service) newStatCountersCmd(basic string) *cobra.Command {
	var sourceID int64
	var counterSource, counterTiming, counterResolution string
	cmd := &cobra.Command{
		Use:         "counters",
		Short:       "Delivery/open/click counters (GET /v3/REST/statcounters)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("CounterSource", counterSource)
			q.Set("CounterTiming", counterTiming)
			q.Set("CounterResolution", counterResolution)
			if sourceID != 0 {
				q.Set("SourceID", itoa64(sourceID))
			}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/statcounters", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(resp)
		},
	}
	cmd.Flags().Int64Var(&sourceID, "source-id", 0, "campaign or list ID (required unless CounterSource=APIKey)")
	cmd.Flags().StringVar(&counterSource, "counter-source", "APIKey", "counter source: APIKey|Campaign|List")
	cmd.Flags().StringVar(&counterTiming, "counter-timing", "Message", "counter timing: Message|Event")
	cmd.Flags().StringVar(&counterResolution, "counter-resolution", "Lifetime", "resolution: Lifetime|Day|Hour|IntervalMonth")
	return cmd
}

// newStatRecipientESPCmd queries per-mailbox-provider deliverability for one
// campaign (GET /v3/REST/statistics/recipient-esp?CampaignId=<id>).
func (s *Service) newStatRecipientESPCmd(basic string) *cobra.Command {
	var campaignID int64
	cmd := &cobra.Command{
		Use:         "recipient-esp",
		Short:       "Per-mailbox-provider deliverability for a campaign (GET /v3/REST/statistics/recipient-esp)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("CampaignId", itoa64(campaignID))
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/statistics/recipient-esp", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(resp)
		},
	}
	cmd.Flags().Int64Var(&campaignID, "campaign-id", 0, "campaign ID")
	_ = cmd.MarkFlagRequired("campaign-id")
	return cmd
}
