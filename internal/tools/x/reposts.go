package x

import (
	"fmt"
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
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRepostIDs(userID, args[0]); err != nil {
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
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRepostIDs(userID, args[0]); err != nil {
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

func requireRepostIDs(userID, postID string) error {
	if userID == "" {
		return fmt.Errorf("X_USER_ID is not set — reconnect X")
	}
	if err := requireNumericID("connected user id", userID); err != nil {
		return err
	}
	return requireNumericID("post id", postID)
}
