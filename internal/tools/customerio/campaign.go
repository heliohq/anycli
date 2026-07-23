package customerio

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newCampaignListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List campaigns (GET /v1/campaigns)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/campaigns", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newCampaignGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a campaign (GET /v1/campaigns/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/campaigns/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newCampaignMetricsCmd(key string) *cobra.Command {
	var id string
	var links, journey bool
	var m metricsParams
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Campaign performance metrics (GET /v1/campaigns/{id}/metrics, /metrics/links, or /journey_metrics)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if links && journey {
				return &usageError{msg: "--links and --journey are mutually exclusive"}
			}
			q := url.Values{}
			m.apply(q)
			path := "/v1/campaigns/" + url.PathEscape(id) + "/metrics"
			switch {
			case links:
				path += "/links"
			case journey:
				path = "/v1/campaigns/" + url.PathEscape(id) + "/journey_metrics"
			}
			resp, err := s.call(cmd, key, http.MethodGet, path, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "campaign id")
	cmd.Flags().BoolVar(&links, "links", false, "report per-link click metrics (/metrics/links)")
	cmd.Flags().BoolVar(&journey, "journey", false, "report journey metrics (/journey_metrics)")
	registerMetricsFlags(cmd, &m)
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
