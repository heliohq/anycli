package twitch

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newStreamListCmd lists live streams, optionally filtered by user, game, type,
// or language. No scope required.
func (s *Service) newStreamListCmd(rc *reqCtx) *cobra.Command {
	var (
		userIDs, userLogins, gameIDs, languages []string
		streamType                              string
		page                                    paginationFlags
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List live streams (optionally filtered)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			addRepeated(q, "user_id", userIDs)
			addRepeated(q, "user_login", userLogins)
			addRepeated(q, "game_id", gameIDs)
			addRepeated(q, "language", languages)
			if streamType != "" {
				q.Set("type", streamType)
			}
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/streams", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().StringArrayVar(&userIDs, "user-id", nil, "filter by broadcaster user id (repeatable)")
	cmd.Flags().StringArrayVar(&userLogins, "user-login", nil, "filter by broadcaster login (repeatable)")
	cmd.Flags().StringArrayVar(&gameIDs, "game-id", nil, "filter by game/category id (repeatable)")
	cmd.Flags().StringArrayVar(&languages, "language", nil, "filter by stream language (repeatable)")
	cmd.Flags().StringVar(&streamType, "type", "", "stream type: all|live")
	registerPaginationFlags(cmd, &page)
	return cmd
}

// newStreamFollowedCmd lists live streams from channels the authenticated user
// follows. Requires the user:read:follows scope; user_id defaults to self.
func (s *Service) newStreamFollowedCmd(rc *reqCtx) *cobra.Command {
	var userID string
	var page paginationFlags
	cmd := &cobra.Command{
		Use:   "followed",
		Short: "List live streams from channels you follow",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := s.broadcasterOrSelf(cmd.Context(), rc, userID)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("user_id", id)
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/streams/followed", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().StringVar(&userID, "user-id", "", "the following user's id (default: self)")
	registerPaginationFlags(cmd, &page)
	return cmd
}

// newSearchChannelsCmd searches channels by query (matches names and stream
// titles). No scope required.
func (s *Service) newSearchChannelsCmd(rc *reqCtx) *cobra.Command {
	var (
		query    string
		liveOnly bool
		page     paginationFlags
	)
	cmd := &cobra.Command{
		Use:   "channels",
		Short: "Search channels by query",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" {
				return &usageError{msg: "twitch: search channels requires --query"}
			}
			q := url.Values{}
			q.Set("query", query)
			if liveOnly {
				q.Set("live_only", "true")
			}
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/search/channels", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query (required)")
	cmd.Flags().BoolVar(&liveOnly, "live-only", false, "return only channels that are currently live")
	registerPaginationFlags(cmd, &page)
	return cmd
}
