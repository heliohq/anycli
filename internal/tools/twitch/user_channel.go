package twitch

import (
	"context"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newUserGetCmd looks up users. With no --id/--login it returns the
// authenticated user (Get Users returns the token's own user when no filter is
// given); with filters it returns those users. No scope required.
func (s *Service) newUserGetCmd(rc *reqCtx) *cobra.Command {
	var ids, logins []string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get users (self when no --id/--login is given)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			addRepeated(q, "id", ids)
			addRepeated(q, "login", logins)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/users", q, nil)
			if err != nil {
				return err
			}
			// Single self/lookup returns one object; multiple filters return a
			// list. Emit a list when the caller asked for more than one id/login,
			// else the single object.
			if len(ids)+len(logins) > 1 {
				return s.emitList(body)
			}
			return s.emitOne(body)
		},
	}
	cmd.Flags().StringArrayVar(&ids, "id", nil, "user id to look up (repeatable)")
	cmd.Flags().StringArrayVar(&logins, "login", nil, "user login name to look up (repeatable)")
	return cmd
}

// newChannelGetCmd reads channel information for a broadcaster (self by
// default). No scope required.
func (s *Service) newChannelGetCmd(rc *reqCtx) *cobra.Command {
	var broadcasterID string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get channel information (self by default)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := s.broadcasterOrSelf(cmd.Context(), rc, broadcasterID)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("broadcaster_id", id)
			body, err := s.call(cmd.Context(), rc, http.MethodGet, "/channels", q, nil)
			if err != nil {
				return err
			}
			return s.emitOne(body)
		},
	}
	cmd.Flags().StringVar(&broadcasterID, "broadcaster-id", "", "target channel's broadcaster id (default: self)")
	return cmd
}

// channelUpdatePayload is the PATCH /channels body. Only set fields are sent so
// an update leaves untouched fields unchanged.
type channelUpdatePayload struct {
	Title               *string  `json:"title,omitempty"`
	GameID              *string  `json:"game_id,omitempty"`
	BroadcasterLanguage *string  `json:"broadcaster_language,omitempty"`
	Delay               *int     `json:"delay,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

// newChannelUpdateCmd updates the caller's channel metadata (title, category,
// language, tags, stream delay). Requires the channel:manage:broadcast scope.
func (s *Service) newChannelUpdateCmd(rc *reqCtx) *cobra.Command {
	var (
		broadcasterID string
		title         string
		gameID        string
		language      string
		tags          []string
		delay         int
	)
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update channel information (title, category, language, tags)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := channelUpdatePayload{}
			touched := false
			if cmd.Flags().Changed("title") {
				payload.Title = &title
				touched = true
			}
			if cmd.Flags().Changed("game-id") {
				payload.GameID = &gameID
				touched = true
			}
			if cmd.Flags().Changed("language") {
				payload.BroadcasterLanguage = &language
				touched = true
			}
			if cmd.Flags().Changed("delay") {
				payload.Delay = &delay
				touched = true
			}
			if cmd.Flags().Changed("tags") {
				payload.Tags = tags
				touched = true
			}
			if !touched {
				return &usageError{msg: "twitch: channel update needs at least one of --title/--game-id/--language/--tags/--delay"}
			}
			id, err := s.broadcasterOrSelf(cmd.Context(), rc, broadcasterID)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("broadcaster_id", id)
			if _, err := s.call(cmd.Context(), rc, http.MethodPatch, "/channels", q, payload); err != nil {
				return err
			}
			// PATCH /channels returns 204 No Content on success — emit a receipt.
			return s.emitValue(map[string]any{"updated": true, "broadcaster_id": id})
		},
	}
	cmd.Flags().StringVar(&broadcasterID, "broadcaster-id", "", "target channel's broadcaster id (default: self)")
	cmd.Flags().StringVar(&title, "title", "", "stream title")
	cmd.Flags().StringVar(&gameID, "game-id", "", "category / game id (empty string clears it)")
	cmd.Flags().StringVar(&language, "language", "", "broadcaster language (ISO 639-1, e.g. en)")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "stream tags (repeatable; replaces the full set)")
	cmd.Flags().IntVar(&delay, "delay", 0, "stream delay in seconds (Partners only)")
	return cmd
}

// broadcasterOrSelf returns explicit when non-empty, otherwise the
// authenticated user's id.
func (s *Service) broadcasterOrSelf(ctx context.Context, rc *reqCtx, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	return s.resolveSelfID(ctx, rc)
}
