package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// videosGetPart is the default set of video sections to hydrate.
const videosGetPart = "snippet,statistics,contentDetails,status"

// validPrivacy are the accepted --privacy values shared by videos + playlists.
var validPrivacy = map[string]bool{"public": true, "unlisted": true, "private": true}

// validRatings are the accepted --rating values.
var validRatings = map[string]bool{"like": true, "dislike": true, "none": true}

func (s *Service) newVideosGetCmd(token string) *cobra.Command {
	var id, part string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one or more videos' metadata + statistics",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required (comma-separated video ids)"}
			}
			q := url.Values{}
			q.Set("part", resolvePart(part, videosGetPart))
			q.Set("id", id)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/videos", q, nil)
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
			return s.renderVideos(lr)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "comma-separated video ids")
	registerPartFlag(cmd, &part)
	return cmd
}

// newVideosMineCmd lists the authenticated channel's uploads via the uploads
// playlist — never search. channels.list(mine, contentDetails) yields the
// uploads playlist id, then playlistItems.list pages it. This is ~1-2 quota
// units, complete, and immediately consistent (unlike search.list?forMine).
func (s *Service) newVideosMineCmd(token string) *cobra.Command {
	var max int
	var page string
	cmd := &cobra.Command{
		Use:         "mine",
		Short:       "List the connected channel's own uploads (via the uploads playlist, not search)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			uploads, err := s.uploadsPlaylistID(cmd.Context(), token)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("part", "snippet,contentDetails")
			q.Set("playlistId", uploads)
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
	addListFlags(cmd, &max, &page)
	return cmd
}

// uploadsPlaylistID resolves the connected channel's uploads playlist id via
// channels.list(mine=true, part=contentDetails).
func (s *Service) uploadsPlaylistID(ctx context.Context, token string) (string, error) {
	q := url.Values{}
	q.Set("part", "contentDetails")
	q.Set("mine", "true")
	body, err := s.call(ctx, token, http.MethodGet, "/channels", q, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Items []struct {
			ContentDetails struct {
				RelatedPlaylists struct {
					Uploads string `json:"uploads"`
				} `json:"relatedPlaylists"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", &apiError{msg: fmt.Sprintf("youtube: decode channel contentDetails: %v", err), err: err}
	}
	if len(resp.Items) == 0 || resp.Items[0].ContentDetails.RelatedPlaylists.Uploads == "" {
		return "", &apiError{msg: "youtube: no uploads playlist found for the connected channel"}
	}
	return resp.Items[0].ContentDetails.RelatedPlaylists.Uploads, nil
}

// newVideosUpdateCmd does a read-modify-write: videos.update replaces the whole
// named part, so the current snippet (title + categoryId are required by the
// API) is fetched and merged with the caller's fields before the PUT.
func (s *Service) newVideosUpdateCmd(token string) *cobra.Command {
	var id, title, description, tags, categoryID, privacy string
	cmd := &cobra.Command{
		Use:         "update",
		Short:       "Update a video's title / description / tags / category / privacy",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			if !anySet(title, description, tags, categoryID, privacy) {
				return &usageError{msg: "nothing to update: set at least one of --title/--description/--tags/--category-id/--privacy"}
			}
			if privacy != "" && !validPrivacy[privacy] {
				return &usageError{msg: fmt.Sprintf("--privacy must be public|unlisted|private, got %q", privacy)}
			}
			parts := "snippet"
			if privacy != "" {
				parts = "snippet,status"
			}
			cur, err := s.fetchVideoForUpdate(cmd.Context(), token, id, parts)
			if err != nil {
				return err
			}
			if title != "" {
				cur.Snippet["title"] = title
			}
			if description != "" {
				cur.Snippet["description"] = description
			}
			if tags != "" {
				cur.Snippet["tags"] = splitCSV(tags)
			}
			if categoryID != "" {
				cur.Snippet["categoryId"] = categoryID
			}
			payload := map[string]any{"id": id, "snippet": cur.Snippet}
			if privacy != "" {
				if cur.Status == nil {
					cur.Status = map[string]any{}
				}
				cur.Status["privacyStatus"] = privacy
				payload["status"] = cur.Status
			}
			q := url.Values{}
			q.Set("part", parts)
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/videos", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			fmt.Fprintf(s.stdout(), "updated video %s\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "video id")
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tags (replaces existing)")
	cmd.Flags().StringVar(&categoryID, "category-id", "", "numeric category id")
	cmd.Flags().StringVar(&privacy, "privacy", "", "public|unlisted|private")
	return cmd
}

// videoParts is the decoded current snippet/status of a video being updated.
type videoParts struct {
	Snippet map[string]any
	Status  map[string]any
}

// fetchVideoForUpdate GETs the current snippet (+ status when needed) so the
// PUT preserves fields the caller did not change. The video must exist.
func (s *Service) fetchVideoForUpdate(ctx context.Context, token, id, parts string) (*videoParts, error) {
	q := url.Values{}
	q.Set("part", parts)
	q.Set("id", id)
	body, err := s.call(ctx, token, http.MethodGet, "/videos", q, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Items []struct {
			Snippet map[string]any `json:"snippet"`
			Status  map[string]any `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: decode video for update: %v", err), err: err}
	}
	if len(resp.Items) == 0 {
		return nil, &apiError{msg: fmt.Sprintf("youtube: video %s not found", id)}
	}
	vp := &videoParts{Snippet: resp.Items[0].Snippet, Status: resp.Items[0].Status}
	if vp.Snippet == nil {
		vp.Snippet = map[string]any{}
	}
	return vp, nil
}

func (s *Service) newVideosRateCmd(token string) *cobra.Command {
	var id, rating string
	cmd := &cobra.Command{
		Use:         "rate",
		Short:       "Like, dislike or clear your rating on a video",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &usageError{msg: "--id is required"}
			}
			if !validRatings[rating] {
				return &usageError{msg: fmt.Sprintf("--rating must be like|dislike|none, got %q", rating)}
			}
			q := url.Values{}
			q.Set("id", id)
			q.Set("rating", rating)
			if _, err := s.call(cmd.Context(), token, http.MethodPost, "/videos/rate", q, nil); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitOK(id)
			}
			fmt.Fprintf(s.stdout(), "rated video %s: %s\n", id, rating)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "video id")
	cmd.Flags().StringVar(&rating, "rating", "", "like|dislike|none")
	return cmd
}

// renderVideos prints a compact line per video: title — views / likes (id).
func (s *Service) renderVideos(lr listResponse) error {
	if len(lr.Items) == 0 {
		fmt.Fprintln(s.stdout(), "no videos")
		return nil
	}
	for _, raw := range lr.Items {
		var v struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
			Statistics struct {
				ViewCount string `json:"viewCount"`
				LikeCount string `json:"likeCount"`
			} `json:"statistics"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return &apiError{msg: fmt.Sprintf("youtube: decode video: %v", err), err: err}
		}
		fmt.Fprintf(s.stdout(), "%s — %s views / %s likes (%s)\n",
			v.Snippet.Title, dash(v.Statistics.ViewCount), dash(v.Statistics.LikeCount), v.ID)
	}
	return nil
}

// anySet reports whether any of the strings is non-empty.
func anySet(vals ...string) bool {
	for _, v := range vals {
		if v != "" {
			return true
		}
	}
	return false
}

// splitCSV splits a comma list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
