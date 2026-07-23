package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// playlistsListPart is the default set of playlist sections to hydrate.
const playlistsListPart = "snippet,contentDetails,status"

func (s *Service) newPlaylistsListCmd(token string) *cobra.Command {
	var mine bool
	var channel, part string
	var max int
	var page string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List playlists (your own with --mine, or a channel's with --channel)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("part", resolvePart(part, playlistsListPart))
			switch {
			case mine:
				q.Set("mine", "true")
			case channel != "":
				q.Set("channelId", channel)
			default:
				return &usageError{msg: "one of --mine or --channel is required"}
			}
			applyListFlags(q, max, page)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/playlists", q, nil)
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
			return s.renderPlaylists(lr)
		},
	}
	cmd.Flags().BoolVar(&mine, "mine", false, "the authenticated user's own playlists")
	cmd.Flags().StringVar(&channel, "channel", "", "another channel's id")
	registerPartFlag(cmd, &part)
	addListFlags(cmd, &max, &page)
	return cmd
}

func (s *Service) newPlaylistsCreateCmd(token string) *cobra.Command {
	var title, description, privacy string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a playlist",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return &usageError{msg: "--title is required"}
			}
			if privacy != "" && !validPrivacy[privacy] {
				return &usageError{msg: fmt.Sprintf("--privacy must be public|unlisted|private, got %q", privacy)}
			}
			snippet := map[string]any{"title": title}
			if description != "" {
				snippet["description"] = description
			}
			payload := map[string]any{"snippet": snippet}
			if privacy != "" {
				payload["status"] = map[string]any{"privacyStatus": privacy}
			}
			q := url.Values{}
			q.Set("part", "snippet,status")
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/playlists", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			return s.renderCreatedPlaylist(body)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "playlist title")
	cmd.Flags().StringVar(&description, "description", "", "playlist description")
	cmd.Flags().StringVar(&privacy, "privacy", "", "public|unlisted|private")
	return cmd
}

// newPlaylistsUpdateCmd read-modify-writes a playlist: playlists.update replaces
// the whole snippet part (title is required), so the current snippet is fetched
// and merged before the PUT.
func (s *Service) newPlaylistsUpdateCmd(token string) *cobra.Command {
	var id, title, description, privacy string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a playlist's title / description / privacy",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			if !anySet(title, description, privacy) {
				return &usageError{msg: "nothing to update: set at least one of --title/--description/--privacy"}
			}
			if privacy != "" && !validPrivacy[privacy] {
				return &usageError{msg: fmt.Sprintf("--privacy must be public|unlisted|private, got %q", privacy)}
			}
			snippet, err := s.fetchPlaylistSnippet(cmd.Context(), token, id)
			if err != nil {
				return err
			}
			if title != "" {
				snippet["title"] = title
			}
			if description != "" {
				snippet["description"] = description
			}
			payload := map[string]any{"id": id, "snippet": snippet}
			parts := "snippet"
			if privacy != "" {
				payload["status"] = map[string]any{"privacyStatus": privacy}
				parts = "snippet,status"
			}
			q := url.Values{}
			q.Set("part", parts)
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/playlists", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			fmt.Fprintf(s.stdout(), "updated playlist %s\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "playlist id")
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&privacy, "privacy", "", "public|unlisted|private")
	return cmd
}

func (s *Service) newPlaylistsDeleteCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a playlist",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			q := url.Values{}
			q.Set("id", id)
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/playlists", q, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitOK(id)
			}
			fmt.Fprintf(s.stdout(), "deleted playlist %s\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "playlist id")
	return cmd
}

// fetchPlaylistSnippet GETs the current snippet so an update preserves fields
// the caller did not change. The playlist must exist.
func (s *Service) fetchPlaylistSnippet(ctx context.Context, token, id string) (map[string]any, error) {
	q := url.Values{}
	q.Set("part", "snippet")
	q.Set("id", id)
	body, err := s.call(ctx, token, http.MethodGet, "/playlists", q, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Items []struct {
			Snippet map[string]any `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: decode playlist for update: %v", err), err: err}
	}
	if len(resp.Items) == 0 {
		return nil, &apiError{msg: fmt.Sprintf("youtube: playlist %s not found", id)}
	}
	snippet := resp.Items[0].Snippet
	if snippet == nil {
		snippet = map[string]any{}
	}
	return snippet, nil
}

func (s *Service) renderPlaylists(lr listResponse) error {
	if len(lr.Items) == 0 {
		fmt.Fprintln(s.stdout(), "no playlists")
		return nil
	}
	for _, raw := range lr.Items {
		var p struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
			ContentDetails struct {
				ItemCount int64 `json:"itemCount"`
			} `json:"contentDetails"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return &apiError{msg: fmt.Sprintf("youtube: decode playlist: %v", err), err: err}
		}
		fmt.Fprintf(s.stdout(), "%s\t%s\t%d items\n", p.ID, p.Snippet.Title, p.ContentDetails.ItemCount)
	}
	if lr.NextPageToken != "" {
		fmt.Fprintf(s.stdout(), "next page token: %s\n", lr.NextPageToken)
	}
	return nil
}

func (s *Service) renderCreatedPlaylist(body []byte) error {
	var p struct {
		ID      string `json:"id"`
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return &apiError{msg: fmt.Sprintf("youtube: decode created playlist: %v", err), err: err}
	}
	fmt.Fprintf(s.stdout(), "created playlist %s (%s)\n", p.Snippet.Title, p.ID)
	return nil
}
