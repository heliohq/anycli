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
		s.newTimelineLeafCmd(token, connectedUserID, "user", "Posts by a user", longTimelineUser, func(id string) string {
			return "/2/users/" + url.PathEscape(id) + "/tweets"
		}),
		s.newTimelineLeafCmd(token, connectedUserID, "mentions", "Posts mentioning a user", longTimelineMentions, func(id string) string {
			return "/2/users/" + url.PathEscape(id) + "/mentions"
		}),
		s.newHomeTimelineCmd(token, connectedUserID),
	)
	return cmd
}

// longTimelineUser and longTimelineMentions are the two per-user timeline
// Longs. They sit next to the shared builder because it is the builder that
// fixes the 5-100 limit range and the --user-id default both describe.
const (
	longTimelineUser = "Defaults to the connected account; pass --user-id for someone else. X caps\n" +
		"this endpoint at roughly the account's 3200 most recent posts — anything\n" +
		"older is unreachable here and has to be fetched by id with `post get`.\n" +
		"Reposts and replies made by the account are included. --limit is 5-100,\n" +
		"default 10; continue with --next-token or poll with --since-id."

	longTimelineMentions = "The incremental way to find new engagement: keep the newest id returned and\n" +
		"pass it as --since-id next time, which makes polling exact and far cheaper\n" +
		"than re-running `post search`. Defaults to the connected account; pass\n" +
		"--user-id for another account. X serves roughly the 800 most recent\n" +
		"mentions here. --limit is 5-100, default 10; continue with --next-token."
)

func (s *Service) newTimelineLeafCmd(token, connectedUserID, use, short, long string, pathFor func(string) string) *cobra.Command {
	userID := connectedUserID
	var nextToken, sinceID string
	var limit int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short + " (one page)",
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
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
			if err := requireOptionalNumericID("since id", sinceID); err != nil {
				return err
			}
			values := timelineQuery(limit, nextToken, sinceID)
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
	cmd.Flags().StringVar(&sinceID, "since-id", "", "only posts newer than this post id")
	return cmd
}

func (s *Service) newHomeTimelineCmd(token, connectedUserID string) *cobra.Command {
	var nextToken, sinceID string
	var limit int
	cmd := &cobra.Command{
		Use:   "home",
		Short: "Reverse-chronological home timeline for the connected user (one page)",
		Long: "Always the connected account — there is no --user-id on this command. It\n" +
			"is the feed of accounts that account follows (X's \"Following\" tab); the\n" +
			"ranked \"For You\" feed is not exposed by the API. --limit is 1-100,\n" +
			"default 10 — a floor of 1, unlike the 5 on `timeline user` and\n" +
			"`timeline mentions`; continue with --next-token or poll with --since-id.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
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
			if err := requireOptionalNumericID("since id", sinceID); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(connectedUserID) + "/timelines/reverse_chronological"
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, timelineQuery(limit, nextToken, sinceID), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum posts in this page (1-100)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	cmd.Flags().StringVar(&sinceID, "since-id", "", "only posts newer than this post id")
	return cmd
}

func timelineQuery(limit int, nextToken, sinceID string) url.Values {
	values := url.Values{
		"max_results":  {strconv.Itoa(limit)},
		"tweet.fields": {defaultPostFields},
	}
	if nextToken != "" {
		values.Set("pagination_token", nextToken)
	}
	if sinceID != "" {
		values.Set("since_id", sinceID)
	}
	return values
}
