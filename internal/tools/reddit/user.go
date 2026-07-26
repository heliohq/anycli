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
		s.newUserHistoryCmd(token, "posts", "submitted", longUserPosts, true),
		s.newUserHistoryCmd(token, "comments", "comments", longUserComments, false),
	)
	return cmd
}

func (s *Service) newUserAboutCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "about <name>",
		Short: "A Redditor's profile (karma, account age)",
		Long: "Any Redditor's public profile by bare name, without the `u/` prefix: id,\n" +
			"link karma, comment karma and creation time. Karma and account age are\n" +
			"exactly what subreddit rules commonly gate on, so this answers whether\n" +
			"an account clears a community's threshold. A suspended or deleted\n" +
			"account fails rather than returning an empty profile.",
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

// longUserPosts and longUserComments are the two history Longs. They live next
// to the shared builder because it is the builder that fixes the Reddit
// endpoint and the emitted record shape each one describes.
const (
	longUserPosts = "Any Redditor's submitted posts, newest first, one page per call: pass the\n" +
		"bare username without `u/`, `--limit` 1-100, `--after` to continue.\n" +
		"Reddit stops serving a user's history at roughly the last thousand\n" +
		"items, so this cannot reconstruct a long-lived account's full record no\n" +
		"matter how far it is paged. Records come back in the post shape, not the\n" +
		"comment shape."

	longUserComments = "Any Redditor's comments, newest first, one page per call, in the COMMENT\n" +
		"shape (`body`, `parent_id`, `depth`, `score`) rather than the post\n" +
		"shape. `--limit` is 1-100 and `--after` continues. Reddit stops at\n" +
		"roughly the last thousand items here too. It is the quickest read of\n" +
		"what someone has been saying lately, but says nothing about what each\n" +
		"comment replies to without resolving `parent_id`."
)

// newUserHistoryCmd builds `user posts` / `user comments`, both Listings.
func (s *Service) newUserHistoryCmd(token, use, segment, long string, posts bool) *cobra.Command {
	var after string
	var limit int
	cmd := &cobra.Command{
		Use:         use + " <name>",
		Short:       "A Redditor's " + use,
		Long:        long,
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
