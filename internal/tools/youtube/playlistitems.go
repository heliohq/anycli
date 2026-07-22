package youtube

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// playlistItemsListPart is the default set of playlist-item sections.
const playlistItemsListPart = "snippet,contentDetails"

func (s *Service) newPlaylistItemsListCmd(token string) *cobra.Command {
	var playlist, part string
	var max int
	var page string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the videos in a playlist",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if playlist == "" {
				return &usageError{msg: "--playlist is required"}
			}
			q := url.Values{}
			q.Set("part", resolvePart(part, playlistItemsListPart))
			q.Set("playlistId", playlist)
			applyListFlags(q, max, page)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/playlistItems", q, nil)
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
			return s.renderPlaylistItems(lr)
		},
	}
	cmd.Flags().StringVar(&playlist, "playlist", "", "playlist id")
	registerPartFlag(cmd, &part)
	addListFlags(cmd, &max, &page)
	return cmd
}

func (s *Service) newPlaylistItemsAddCmd(token string) *cobra.Command {
	var playlist, video string
	var position int
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a video to a playlist",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if playlist == "" || video == "" {
				return &usageError{msg: "--playlist and --video are required"}
			}
			snippet := map[string]any{
				"playlistId": playlist,
				"resourceId": map[string]any{"kind": "youtube#video", "videoId": video},
			}
			if cmd.Flags().Changed("position") {
				snippet["position"] = position
			}
			payload := map[string]any{"snippet": snippet}
			q := url.Values{}
			q.Set("part", "snippet")
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/playlistItems", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderAddedPlaylistItem(body, playlist, video)
		},
	}
	cmd.Flags().StringVar(&playlist, "playlist", "", "playlist id")
	cmd.Flags().StringVar(&video, "video", "", "video id to add")
	cmd.Flags().IntVar(&position, "position", 0, "zero-based insert position (default: append)")
	return cmd
}

// newPlaylistItemsRemoveCmd deletes a playlist item by its playlistItem id (NOT
// the video id — the same video can appear multiple times, each with its own
// playlistItem id from `playlist-items list`).
func (s *Service) newPlaylistItemsRemoveCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an item from a playlist (by playlistItem id, not video id)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required (the playlistItem id from `playlist-items list`, not the video id)"}
			}
			q := url.Values{}
			q.Set("id", id)
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/playlistItems", q, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitOK(id)
			}
			fmt.Fprintf(s.stdout(), "removed playlist item %s\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "playlistItem id")
	return cmd
}

func (s *Service) renderPlaylistItems(lr listResponse) error {
	if len(lr.Items) == 0 {
		fmt.Fprintln(s.stdout(), "no items")
		return nil
	}
	for _, raw := range lr.Items {
		var it struct {
			ID      string `json:"id"`
			Snippet struct {
				Title      string `json:"title"`
				ResourceID struct {
					VideoID string `json:"videoId"`
				} `json:"resourceId"`
			} `json:"snippet"`
		}
		if err := json.Unmarshal(raw, &it); err != nil {
			return &apiError{msg: fmt.Sprintf("youtube: decode playlist item: %v", err), err: err}
		}
		fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", it.Snippet.ResourceID.VideoID, it.ID, truncate(it.Snippet.Title, 80))
	}
	if lr.NextPageToken != "" {
		fmt.Fprintf(s.stdout(), "next page token: %s\n", lr.NextPageToken)
	}
	return nil
}

func (s *Service) renderAddedPlaylistItem(body []byte, playlist, video string) error {
	var it struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &it)
	fmt.Fprintf(s.stdout(), "added video %s to playlist %s (item %s)\n", video, playlist, it.ID)
	return nil
}
