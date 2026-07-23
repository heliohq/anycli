package bluesky

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newSearchCmd(sess *session) *cobra.Command {
	cmd := &cobra.Command{Use: "search", Short: "Search posts and people"}
	cmd.AddCommand(s.newSearchPostsCmd(sess), s.newSearchActorsCmd(sess))
	return cmd
}

func (s *Service) newSearchPostsCmd(sess *session) *cobra.Command {
	var q, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "posts",
		Short: "Search recent posts (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if q == "" {
				return fmt.Errorf("--q is required")
			}
			query, err := feedQuery(limit, cursor)
			if err != nil {
				return err
			}
			query.Set("q", q)
			body, err := sess.call(cmd.Context(), http.MethodGet, "app.bsky.feed.searchPosts", query, nil)
			if err != nil {
				return err
			}
			var resp struct {
				Posts  []rawPost `json:"posts"`
				Cursor string    `json:"cursor"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("bluesky: decode search: %w", err)
			}
			return s.emitValue(shapePostList(resp.Posts, resp.Cursor))
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "search query")
	addFeedFlags(cmd, &limit, &cursor)
	return cmd
}

func (s *Service) newSearchActorsCmd(sess *session) *cobra.Command {
	var q string
	var limit int
	cmd := &cobra.Command{
		Use:   "actors",
		Short: "Search people (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if q == "" {
				return fmt.Errorf("--q is required")
			}
			if limit < 1 || limit > 100 {
				return fmt.Errorf("limit must be between 1 and 100")
			}
			query := url.Values{"q": {q}, "limit": {strconv.Itoa(limit)}}
			body, err := sess.call(cmd.Context(), http.MethodGet, "app.bsky.actor.searchActors", query, nil)
			if err != nil {
				return err
			}
			var resp struct {
				Actors []rawProfile `json:"actors"`
				Cursor string       `json:"cursor"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("bluesky: decode actor search: %w", err)
			}
			actors := make([]profileView, 0, len(resp.Actors))
			for _, a := range resp.Actors {
				actors = append(actors, shapeProfile(a))
			}
			return s.emitValue(map[string]any{"actors": actors, "cursor": resp.Cursor})
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "search query")
	cmd.Flags().IntVar(&limit, "limit", 25, "maximum actors in this page (1-100)")
	return cmd
}
