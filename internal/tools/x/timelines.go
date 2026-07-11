package x

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newTimelineCmd(token, connectedUserID string) *cobra.Command {
	cmd := &cobra.Command{Use: "timeline", Short: "Post timelines"}
	cmd.AddCommand(
		s.newTimelineLeafCmd(token, connectedUserID, "user", "Posts by a user", func(id string) string {
			return "/2/users/" + url.PathEscape(id) + "/tweets"
		}),
		s.newTimelineLeafCmd(token, connectedUserID, "mentions", "Posts mentioning a user", func(id string) string {
			return "/2/users/" + url.PathEscape(id) + "/mentions"
		}),
		s.newHomeTimelineCmd(token, connectedUserID),
	)
	return cmd
}

func (s *Service) newTimelineLeafCmd(token, connectedUserID, use, short string, pathFor func(string) string) *cobra.Command {
	userID := connectedUserID
	var nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:   use,
		Short: short + " (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if userID == "" {
				return fmt.Errorf("user id is required: pass --user-id or reconnect X to populate X_USER_ID")
			}
			if err := requireNumericID("user id", userID); err != nil {
				return err
			}
			if err := requireLimit(limit, 5, 100); err != nil {
				return err
			}
			values := timelineQuery(limit, nextToken)
			body, err := s.call(cmd.Context(), token, http.MethodGet, pathFor(userID), values, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&userID, "user-id", connectedUserID, "X user id (defaults to the connected user)")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum posts in this page (5-100)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	return cmd
}

func (s *Service) newHomeTimelineCmd(token, connectedUserID string) *cobra.Command {
	var nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "home",
		Short: "Reverse-chronological home timeline for the connected user (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if connectedUserID == "" {
				return fmt.Errorf("user id is required: reconnect X to populate X_USER_ID")
			}
			if err := requireNumericID("connected user id", connectedUserID); err != nil {
				return err
			}
			if err := requireLimit(limit, 1, 100); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(connectedUserID) + "/timelines/reverse_chronological"
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, timelineQuery(limit, nextToken), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum posts in this page (1-100)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	return cmd
}

func timelineQuery(limit int, nextToken string) url.Values {
	values := url.Values{
		"max_results":  {strconv.Itoa(limit)},
		"tweet.fields": {defaultPostFields},
	}
	if nextToken != "" {
		values.Set("pagination_token", nextToken)
	}
	return values
}
