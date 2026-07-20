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
		Use:   "create <post-id>",
		Short: "Like a post",
		Args:  cobra.ExactArgs(1),
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
		Use:   "delete <post-id>",
		Short: "Unlike a post",
		Args:  cobra.ExactArgs(1),
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

// newPostAudienceCmd builds `post liking-users` / `post reposters`: the users
// who engaged with a post (GET /2/tweets/:id/liking_users | retweeted_by).
func (s *Service) newPostAudienceCmd(token, use, short, endpoint string) *cobra.Command {
	var nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:   use + " <post-id>",
		Short: short + " (one page)",
		Args:  cobra.ExactArgs(1),
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
