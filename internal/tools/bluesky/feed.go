package bluesky

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// feedResponse is the {feed:[{post}], cursor} shape shared by getTimeline and
// getAuthorFeed.
type feedResponse struct {
	Feed []struct {
		Post rawPost `json:"post"`
	} `json:"feed"`
	Cursor string `json:"cursor"`
}

func (fr feedResponse) posts() []rawPost {
	out := make([]rawPost, 0, len(fr.Feed))
	for _, item := range fr.Feed {
		out = append(out, item.Post)
	}
	return out
}

func (s *Service) newTimelineCmd(sess *session) *cobra.Command {
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Read the home timeline (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, err := feedQuery(limit, cursor)
			if err != nil {
				return err
			}
			return s.emitFeed(cmd.Context(), sess, "app.bsky.feed.getTimeline", query)
		},
	}
	addFeedFlags(cmd, &limit, &cursor)
	return cmd
}

func (s *Service) newFeedCmd(sess *session) *cobra.Command {
	cmd := &cobra.Command{Use: "feed", Short: "Feeds"}
	var actor, cursor string
	var limit int
	author := &cobra.Command{
		Use:   "author",
		Short: "Read an actor's posts (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if actor == "" {
				return fmt.Errorf("--actor is required")
			}
			query, err := feedQuery(limit, cursor)
			if err != nil {
				return err
			}
			query.Set("actor", actor)
			return s.emitFeed(cmd.Context(), sess, "app.bsky.feed.getAuthorFeed", query)
		},
	}
	author.Flags().StringVar(&actor, "actor", "", "handle or DID of the author")
	addFeedFlags(author, &limit, &cursor)
	cmd.AddCommand(author)
	return cmd
}

func (s *Service) emitFeed(ctx context.Context, sess *session, nsid string, query url.Values) error {
	body, err := sess.call(ctx, http.MethodGet, nsid, query, nil)
	if err != nil {
		return err
	}
	var resp feedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("bluesky: decode feed: %w", err)
	}
	return s.emitValue(shapePostList(resp.posts(), resp.Cursor))
}

func addFeedFlags(cmd *cobra.Command, limit *int, cursor *string) {
	cmd.Flags().IntVar(limit, "limit", 25, "maximum posts in this page (1-100)")
	cmd.Flags().StringVar(cursor, "cursor", "", "pagination cursor from a previous page")
}

func feedQuery(limit int, cursor string) (url.Values, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be between 1 and 100")
	}
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return query, nil
}
