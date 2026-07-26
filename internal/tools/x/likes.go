package x

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newLikeCmd(token, userID string) *cobra.Command {
	cmd := &cobra.Command{Use: "like", Short: "Likes"}
	cmd.AddCommand(s.newLikeCreateCmd(token, userID), s.newLikeDeleteCmd(token, userID))
	return cmd
}

func (s *Service) newLikeCreateCmd(token, userID string) *cobra.Command {
	return &cobra.Command{
		Use:         "create <post-id>",
		Annotations: sideEffect(true),
		Short:       "Like a post",
		Long: "Likes as the connected account. The response reports the resulting `liked`\n" +
			"state rather than the effect of this call, so a repeat is harmless but is\n" +
			"not evidence that anything changed. Who liked a post is `post\n" +
			"liking-users`; the count alone is already in `public_metrics` on the post\n" +
			"returned by `post get`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConnectedUserAndPostID(userID, args[0]); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(userID) + "/likes"
			body, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, struct {
				PostID string `json:"tweet_id"`
			}{PostID: args[0]})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newLikeDeleteCmd(token, userID string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <post-id>",
		Annotations: sideEffect(true),
		Short:       "Unlike a post",
		Long: "Removes the connected account's like. The response reports the resulting\n" +
			"`liked` state, so calling it on a post that was never liked is not an\n" +
			"error.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConnectedUserAndPostID(userID, args[0]); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(userID) + "/likes/" + url.PathEscape(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// longPostLikingUsers and longPostReposters are the two audience-list Longs.
// They sit next to the shared builder because it is the builder that fixes the
// 1-100 limit range both describe.
const (
	longPostLikingUsers = "--limit is 1-100, default 10 — note this is not the 10-100 of the search\n" +
		"commands; continue with --next-token. If you only need the number, it is\n" +
		"already on the post: `like_count` inside the `public_metrics` field that\n" +
		"`post get` returns, which is one call instead of paging users."

	longPostReposters = "Lists accounts that reposted verbatim. Quote posts are NOT included — they\n" +
		"are a separate surface, read with `post quotes`. --limit is 1-100, default\n" +
		"10; continue with --next-token. For the number alone use `retweet_count`\n" +
		"inside the `public_metrics` field returned by `post get`."
)

// newPostAudienceCmd builds `post liking-users` / `post reposters`: the users
// who engaged with a post (GET /2/tweets/:id/liking_users | retweeted_by).
func (s *Service) newPostAudienceCmd(token, use, short, long, endpoint string) *cobra.Command {
	var nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:         use + " <post-id>",
		Annotations: sideEffect(false),
		Short:       short + " (one page)",
		Long:        long,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("post id", args[0]); err != nil {
				return err
			}
			if err := requireLimit(limit, 1, 100); err != nil {
				return err
			}
			path := "/2/tweets/" + url.PathEscape(args[0]) + "/" + endpoint
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, userListQuery(limit, nextToken), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum users in this page (1-100)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	return cmd
}
