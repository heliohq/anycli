package twitch

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newClipListCmd lists clips by broadcaster, game, or clip id. No scope
// required. Exactly one of --broadcaster-id / --game-id / --id must be given
// (Helix requires one selector).
func (s *Service) newClipListCmd(rc *reqCtx) *cobra.Command {
	var (
		broadcasterID string
		gameID        string
		ids           []string
		startedAt     string
		endedAt       string
		page          paginationFlags
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List clips by broadcaster, game, or clip id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selectors := 0
			if broadcasterID != "" {
				selectors++
			}
			if gameID != "" {
				selectors++
			}
			if len(ids) > 0 {
				selectors++
			}
			if selectors != 1 {
				return &usageError{msg: "twitch: clip list requires exactly one of --broadcaster-id, --game-id, or --id"}
			}
			q := url.Values{}
			if broadcasterID != "" {
				q.Set("broadcaster_id", broadcasterID)
			}
			if gameID != "" {
				q.Set("game_id", gameID)
			}
			addRepeated(q, "id", ids)
			if startedAt != "" {
				q.Set("started_at", startedAt)
			}
			if endedAt != "" {
				q.Set("ended_at", endedAt)
			}
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/clips", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().StringVar(&broadcasterID, "broadcaster-id", "", "list a broadcaster's clips")
	cmd.Flags().StringVar(&gameID, "game-id", "", "list a game's clips")
	cmd.Flags().StringArrayVar(&ids, "id", nil, "specific clip id (repeatable)")
	cmd.Flags().StringVar(&startedAt, "started-at", "", "RFC3339 start of the date range")
	cmd.Flags().StringVar(&endedAt, "ended-at", "", "RFC3339 end of the date range")
	registerPaginationFlags(cmd, &page)
	return cmd
}

// newClipCreateCmd creates a clip from a broadcaster's live stream (self by
// default). Requires the clips:edit scope. Returns the clip id and edit URL.
func (s *Service) newClipCreateCmd(rc *reqCtx) *cobra.Command {
	var (
		broadcasterID string
		hasDelay      bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a clip from a live stream (self by default)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := s.broadcasterOrSelf(cmd.Context(), rc, broadcasterID)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("broadcaster_id", id)
			if hasDelay {
				q.Set("has_delay", "true")
			}
			body, err := s.call(cmd.Context(), rc, http.MethodPost, "/clips", q, nil)
			if err != nil {
				return err
			}
			return s.emitOne(body)
		},
	}
	cmd.Flags().StringVar(&broadcasterID, "broadcaster-id", "", "the live broadcaster to clip (default: self)")
	cmd.Flags().BoolVar(&hasDelay, "has-delay", false, "account for the broadcaster's stream delay")
	return cmd
}

// newVideoListCmd lists videos (VODs) by video id, user, or game. No scope
// required. Exactly one of --id / --user-id / --game-id must be given.
func (s *Service) newVideoListCmd(rc *reqCtx) *cobra.Command {
	var (
		ids       []string
		userID    string
		gameID    string
		videoType string
		videoSort string
		page      paginationFlags
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List videos (VODs) by id, user, or game",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selectors := 0
			if len(ids) > 0 {
				selectors++
			}
			if userID != "" {
				selectors++
			}
			if gameID != "" {
				selectors++
			}
			if selectors != 1 {
				return &usageError{msg: "twitch: video list requires exactly one of --id, --user-id, or --game-id"}
			}
			q := url.Values{}
			addRepeated(q, "id", ids)
			if userID != "" {
				q.Set("user_id", userID)
			}
			if gameID != "" {
				q.Set("game_id", gameID)
			}
			if videoType != "" {
				q.Set("type", videoType)
			}
			if videoSort != "" {
				q.Set("sort", videoSort)
			}
			page.apply(q)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/videos", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(body)
		},
	}
	cmd.Flags().StringArrayVar(&ids, "id", nil, "specific video id (repeatable)")
	cmd.Flags().StringVar(&userID, "user-id", "", "list a user's videos")
	cmd.Flags().StringVar(&gameID, "game-id", "", "list a game's videos")
	cmd.Flags().StringVar(&videoType, "type", "", "video type: all|archive|highlight|upload")
	cmd.Flags().StringVar(&videoSort, "sort", "", "sort order: time|trending|views")
	registerPaginationFlags(cmd, &page)
	return cmd
}
