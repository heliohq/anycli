package youtube

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// validSearchTypes are the resource kinds search.list can return.
var validSearchTypes = map[string]bool{"video": true, "channel": true, "playlist": true}

// validSearchOrders are the accepted --order values.
var validSearchOrders = map[string]bool{
	"relevance": true, "date": true, "rating": true, "viewCount": true, "title": true,
}

func (s *Service) newSearchCmd(token string) *cobra.Command {
	var query, typ, channel, order, publishedAfter, publishedBefore, region string
	var max int
	var page string
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search videos, channels and playlists (100-unit quota cost)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" && channel == "" {
				return &usageError{msg: "--query is required (or --channel to list a channel's content)"}
			}
			if typ != "" && !validSearchTypes[typ] {
				return &usageError{msg: fmt.Sprintf("--type must be video, channel or playlist, got %q", typ)}
			}
			if order != "" && !validSearchOrders[order] {
				return &usageError{msg: fmt.Sprintf("--order must be one of relevance|date|rating|viewCount|title, got %q", order)}
			}
			q := url.Values{}
			q.Set("part", "snippet")
			if query != "" {
				q.Set("q", query)
			}
			if typ != "" {
				q.Set("type", typ)
			}
			if channel != "" {
				q.Set("channelId", channel)
			}
			if order != "" {
				q.Set("order", order)
			}
			if publishedAfter != "" {
				q.Set("publishedAfter", publishedAfter)
			}
			if publishedBefore != "" {
				q.Set("publishedBefore", publishedBefore)
			}
			if region != "" {
				q.Set("regionCode", region)
			}
			applyListFlags(q, max, page)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/search", q, nil)
			if err != nil {
				return err
			}
			lr, err := decodeList(body)
			if err != nil {
				return err
			}
			items := flattenSearchItems(lr.Items)
			if jsonOut(cmd) {
				out := map[string]any{"items": items}
				if lr.NextPageToken != "" {
					out["nextPageToken"] = lr.NextPageToken
				}
				return s.emitJSON(out)
			}
			return s.renderSearch(items, lr.NextPageToken)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search terms")
	cmd.Flags().StringVar(&typ, "type", "", "restrict to video|channel|playlist")
	cmd.Flags().StringVar(&channel, "channel", "", "restrict results to this channel id")
	cmd.Flags().StringVar(&order, "order", "", "sort order: relevance|date|rating|viewCount|title")
	cmd.Flags().StringVar(&publishedAfter, "published-after", "", "only results after this RFC3339 timestamp")
	cmd.Flags().StringVar(&publishedBefore, "published-before", "", "only results before this RFC3339 timestamp")
	cmd.Flags().StringVar(&region, "region", "", "ISO 3166-1 alpha-2 region code")
	addListFlags(cmd, &max, &page)
	return cmd
}

func (s *Service) renderSearch(items []searchItem, nextPage string) error {
	if len(items) == 0 {
		fmt.Fprintln(s.stdout(), "no results")
		return nil
	}
	for _, it := range items {
		title := snippetTitle(it.Snippet)
		fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", it.Kind, it.ID, title)
	}
	if nextPage != "" {
		fmt.Fprintf(s.stdout(), "next page token: %s\n", nextPage)
	}
	return nil
}
