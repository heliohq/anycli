package reddit

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// newUserCmd groups reads about another Redditor: about, posts, comments.
func (s *Service) newUserCmd(token string) *cobra.Command {
	cmd := newGroup("user", "Read a Redditor's profile and history")
	cmd.AddCommand(
		s.newUserAboutCmd(token),
		s.newUserHistoryCmd(token, "posts", "submitted", true),
		s.newUserHistoryCmd(token, "comments", "comments", false),
	)
	return cmd
}

func (s *Service) newUserAboutCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "about <name>",
		Short:       "A Redditor's profile (karma, account age)",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.get(cmd.Context(), token, "/user/"+url.PathEscape(args[0])+"/about", nil)
			if err != nil {
				return err
			}
			var t thing
			if err := json.Unmarshal(body, &t); err != nil {
				return &apiError{msg: fmt.Sprintf("reddit: decode user: %v", err), err: err}
			}
			d, err := decodeThingData(t.Data)
			if err != nil {
				return err
			}
			if jsonMode(cmd) {
				return s.emitValue(map[string]any{
					"id":            d.ID,
					"name":          d.Name,
					"link_karma":    d.LinkKarma,
					"comment_karma": d.CommentKarma,
					"created_utc":   d.CreatedUTC,
				})
			}
			return s.emitLine(fmt.Sprintf("u/%s\tlink_karma=%d comment_karma=%d", d.Name, d.LinkKarma, d.CommentKarma))
		},
	}
}

// newUserHistoryCmd builds `user posts` / `user comments`, both Listings.
func (s *Service) newUserHistoryCmd(token, use, segment string, posts bool) *cobra.Command {
	var after string
	var limit int
	cmd := &cobra.Command{
		Use:         use + " <name>",
		Short:       "A Redditor's " + use,
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLimit(limit); err != nil {
				return err
			}
			q := url.Values{}
			if limit != 0 {
				q.Set("limit", intToStr(limit))
			}
			if after != "" {
				q.Set("after", after)
			}
			body, err := s.get(cmd.Context(), token, "/user/"+url.PathEscape(args[0])+"/"+segment, q)
			if err != nil {
				return err
			}
			if posts {
				return s.emitPostListing(jsonFlag(jsonMode(cmd)), body)
			}
			return s.emitCommentListing(jsonFlag(jsonMode(cmd)), body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum items in this page (1-100)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page")
	return cmd
}
