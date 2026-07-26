package x

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newRepostCmd(token, userID string) *cobra.Command {
	cmd := &cobra.Command{Use: "repost", Short: "Reposts"}
	cmd.AddCommand(s.newRepostCreateCmd(token, userID), s.newRepostDeleteCmd(token, userID))
	return cmd
}

func (s *Service) newRepostCreateCmd(token, userID string) *cobra.Command {
	return &cobra.Command{
		Use:   "create <post-id>",
		Short: "Repost a post",
		Long: "Reposts verbatim as the connected account, with no text of its own; to add\n" +
			"a comment use `post quote`, which creates a real post instead. The response\n" +
			"reports the resulting `retweeted` state rather than whether this call\n" +
			"changed anything, so a repeat is harmless but tells you nothing.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConnectedUserAndPostID(userID, args[0]); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(userID) + "/retweets"
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

func (s *Service) newRepostDeleteCmd(token, userID string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <post-id>",
		Short: "Undo a repost",
		Long: "Takes the ORIGINAL post's id, not the id of your repost of it. The response\n" +
			"reports the resulting `retweeted` state, so calling it on a post that was\n" +
			"never reposted is not an error.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConnectedUserAndPostID(userID, args[0]); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(userID) + "/retweets/" + url.PathEscape(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
