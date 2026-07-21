package lemlist

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newCampaignCmd groups campaign enumeration, inspection, reporting, and
// start/pause control.
func (s *Service) newCampaignCmd(key string) *cobra.Command {
	cmd := newGroupCmd("campaign", "Campaigns: list, get, stats, start, pause")
	cmd.AddCommand(
		s.newCampaignListCmd(key),
		s.newCampaignGetCmd(key),
		s.newCampaignStatsCmd(key),
		s.newCampaignStartCmd(key),
		s.newCampaignPauseCmd(key),
	)
	return cmd
}

func (s *Service) newCampaignListCmd(key string) *cobra.Command {
	var status, sortBy, sortOrder string
	var offset, limit, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns (GET /campaigns)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("version", "v2")
			if status != "" {
				q.Set("status", status)
			}
			if sortBy != "" {
				q.Set("sortBy", sortBy)
			}
			if sortOrder != "" {
				q.Set("sortOrder", sortOrder)
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/campaigns", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status: running|draft|archived|ended|paused|errors")
	cmd.Flags().StringVar(&sortBy, "sort-by", "", "sort field (createdAt)")
	cmd.Flags().StringVar(&sortOrder, "sort-order", "", "sort direction: asc|desc")
	cmd.Flags().IntVar(&offset, "offset", 0, "records to skip (pagination)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max campaigns to return (max 100)")
	cmd.Flags().IntVar(&page, "page", 0, "page number to retrieve")
	return cmd
}

func (s *Service) newCampaignGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <campaignId>",
		Short: "Get one campaign (GET /campaigns/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/campaigns/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newCampaignStatsCmd(key string) *cobra.Command {
	var startDate, endDate string
	cmd := &cobra.Command{
		Use:   "stats <campaignId>",
		Short: "Get campaign open/click/reply/bounce stats (GET /v2/campaigns/{id}/stats)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if startDate != "" {
				q.Set("startDate", startDate)
			}
			if endDate != "" {
				q.Set("endDate", endDate)
			}
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/v2/campaigns/"+url.PathEscape(args[0])+"/stats", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&startDate, "start-date", "", "window start (Unix seconds or ISO 8601)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "window end (Unix seconds or ISO 8601)")
	return cmd
}

func (s *Service) newCampaignStartCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "start <campaignId>",
		Short: "Start (resume) a campaign (POST /campaigns/{id}/start)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodPost, "/campaigns/"+url.PathEscape(args[0])+"/start", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newCampaignPauseCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "pause <campaignId>",
		Short: "Pause a campaign (POST /campaigns/{id}/pause)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodPost, "/campaigns/"+url.PathEscape(args[0])+"/pause", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
