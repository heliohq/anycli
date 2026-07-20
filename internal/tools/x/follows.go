package x

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newFollowCmd(token, userID string) *cobra.Command {
	cmd := &cobra.Command{Use: "follow", Short: "Follows"}
	cmd.AddCommand(s.newFollowCreateCmd(token, userID), s.newFollowDeleteCmd(token, userID))
	return cmd
}

func (s *Service) newFollowCreateCmd(token, userID string) *cobra.Command {
	return &cobra.Command{
		Use:   "create <user-id>",
		Short: "Follow a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConnectedUserAndTargetID(userID, args[0]); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(userID) + "/following"
			body, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, struct {
				TargetUserID string `json:"target_user_id"`
			}{TargetUserID: args[0]})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newFollowDeleteCmd(token, userID string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <user-id>",
		Short: "Unfollow a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConnectedUserAndTargetID(userID, args[0]); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(userID) + "/following/" + url.PathEscape(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newUserConnectionsCmd builds `user followers` / `user following`
// (GET /2/users/:id/followers | following), defaulting to the connected user.
func (s *Service) newUserConnectionsCmd(token, connectedUserID, use, short, endpoint string) *cobra.Command {
	userID := connectedUserID
	var nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:   use,
		Short: short + " (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if userID == "" {
				return fmt.Errorf("user id is required: pass --user-id or reconnect X to populate X_USER_ID")
			}
			if err := requireNumericID("user id", userID); err != nil {
				return err
			}
			if err := requireLimit(limit, 1, 1000); err != nil {
				return err
			}
			path := "/2/users/" + url.PathEscape(userID) + "/" + endpoint
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, userListQuery(limit, nextToken), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&userID, "user-id", connectedUserID, "X user id (defaults to the connected user)")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum users in this page (1-1000)")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	return cmd
}

func userListQuery(limit int, nextToken string) url.Values {
	values := url.Values{"max_results": {strconv.Itoa(limit)}}
	if nextToken != "" {
		values.Set("pagination_token", nextToken)
	}
	return values
}
