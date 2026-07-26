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
		Long: "Searches deliveries across every recipient, which is the right shape for\n" +
			"\"how many bounced yesterday\" but the wrong one for \"what did Jane get\" —\n" +
			"that is `person messages`, one scoped request instead of paging the\n" +
			"workspace. --metric filters on outcome (sent, delivered, opened, clicked,\n" +
			"bounced, failed) and --type on channel (email, sms, push); combine them to\n" +
			"narrow before paging with --start and --limit.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Takes a delivery id from `message list` or `person messages` and returns\n" +
			"that one send in full, including the metrics and failure detail the list\n" +
			"view summarises. This is the level at which \"it says delivered but they\n" +
			"never saw it\" gets an answer.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
