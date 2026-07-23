package youtube

import (
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// defaultMaxResults is the page size the tool requests when --max is unset.
// The Data API caps maxResults at 50 for the paged list endpoints.
const defaultMaxResults = 5

// addListFlags attaches the shared paging flags (--max / --page) to cmd.
func addListFlags(cmd *cobra.Command, max *int, page *string) {
	cmd.Flags().IntVar(max, "max", defaultMaxResults, "maximum results per page (1-50)")
	cmd.Flags().StringVar(page, "page", "", "page token from a previous response's nextPageToken")
}

// applyListFlags sets maxResults and pageToken on the query.
func applyListFlags(q url.Values, max int, page string) {
	q.Set("maxResults", strconv.Itoa(max))
	if page != "" {
		q.Set("pageToken", page)
	}
}

// registerPartFlag attaches the shared --part override.
func registerPartFlag(cmd *cobra.Command, dst *string) {
	cmd.Flags().StringVar(dst, "part", "", "comma-separated resource parts to hydrate (overrides the per-verb default)")
}

// resolvePart returns the caller override when set, else the per-verb default.
func resolvePart(override, def string) string {
	if override != "" {
		return override
	}
	return def
}

// searchItem is the normalized shape of one search.list result: YouTube nests
// the id under id.videoId / id.channelId / id.playlistId, so the raw item is
// flattened to a top-level id + kind while keeping the snippet verbatim.
type searchItem struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind,omitempty"`
	Snippet json.RawMessage `json:"snippet,omitempty"`
}

// flattenSearchItems rewrites each raw search result to the flattened shape.
// An item whose id object carries no recognized id field keeps an empty id
// rather than being dropped, so nothing silently disappears.
func flattenSearchItems(raw []json.RawMessage) []searchItem {
	out := make([]searchItem, 0, len(raw))
	for _, item := range raw {
		var parsed struct {
			ID struct {
				Kind       string `json:"kind"`
				VideoID    string `json:"videoId"`
				ChannelID  string `json:"channelId"`
				PlaylistID string `json:"playlistId"`
			} `json:"id"`
			Snippet json.RawMessage `json:"snippet"`
		}
		if err := json.Unmarshal(item, &parsed); err != nil {
			continue
		}
		id := parsed.ID.VideoID
		if id == "" {
			id = parsed.ID.ChannelID
		}
		if id == "" {
			id = parsed.ID.PlaylistID
		}
		out = append(out, searchItem{ID: id, Kind: parsed.ID.Kind, Snippet: parsed.Snippet})
	}
	return out
}

// snippetTitle pulls the human title out of a raw snippet object, empty when
// absent.
func snippetTitle(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(raw, &s)
	return s.Title
}

// truncate shortens a string to max runes for the human list view.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
