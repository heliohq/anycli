package youtube

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newSubscriptionsListCmd(token string) *cobra.Command {
	var mine bool
	var channel string
	var max int
	var page string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List the channels an account subscribes to",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("part", "snippet")
			switch {
			case mine:
				q.Set("mine", "true")
			case channel != "":
				q.Set("channelId", channel)
			default:
				return &usageError{msg: "one of --mine or --channel is required"}
			}
			applyListFlags(q, max, page)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/subscriptions", q, nil)
			if err != nil {
				return err
			}
			lr, err := decodeList(body)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitList(lr)
			}
			return s.renderSubscriptions(lr)
		},
	}
	cmd.Flags().BoolVar(&mine, "mine", false, "the authenticated user's own subscriptions")
	cmd.Flags().StringVar(&channel, "channel", "", "another channel's id")
	addListFlags(cmd, &max, &page)
	return cmd
}

func (s *Service) renderSubscriptions(lr listResponse) error {
	if len(lr.Items) == 0 {
		fmt.Fprintln(s.stdout(), "no subscriptions")
		return nil
	}
	for _, raw := range lr.Items {
		var sub struct {
			Snippet struct {
				Title      string `json:"title"`
				ResourceID struct {
					ChannelID string `json:"channelId"`
				} `json:"resourceId"`
			} `json:"snippet"`
		}
		if err := json.Unmarshal(raw, &sub); err != nil {
			return &apiError{msg: fmt.Sprintf("youtube: decode subscription: %v", err), err: err}
		}
		fmt.Fprintf(s.stdout(), "%s\t%s\n", sub.Snippet.ResourceID.ChannelID, sub.Snippet.Title)
	}
	if lr.NextPageToken != "" {
		fmt.Fprintf(s.stdout(), "next page token: %s\n", lr.NextPageToken)
	}
	return nil
}
