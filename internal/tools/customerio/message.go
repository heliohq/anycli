package customerio

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newMessageListCmd(key string) *cobra.Command {
	var metric, msgType, start string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Workspace-wide delivery search (GET /v1/messages)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if metric != "" {
				q.Set("metric", metric)
			}
			if msgType != "" {
				q.Set("type", msgType)
			}
			if start != "" {
				q.Set("start", start)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/messages", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&metric, "metric", "", "delivery outcome filter (e.g. sent, delivered, opened, clicked, bounced, failed)")
	cmd.Flags().StringVar(&msgType, "type", "", "message type filter (e.g. email, sms, push)")
	cmd.Flags().StringVar(&start, "start", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (0 = provider default)")
	return cmd
}

func (s *Service) newMessageGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a single delivery (GET /v1/messages/{id})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/messages/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "message (delivery) id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
