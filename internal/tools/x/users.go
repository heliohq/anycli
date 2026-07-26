package x

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the connected X user",
		Long: "Resolves which account the credential belongs to (GET /2/users/me). The id\n" +
			"it returns is what `--user-id` defaults to on the commands that take one,\n" +
			"and the identity every write command acts as, so this is the cheap check\n" +
			"before a first outward action.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/users/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newUserCmd(token, userID string) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Users"}
	cmd.AddCommand(
		s.newUserGetCmd(token),
		s.newUserSearchCmd(token),
		s.newUserConnectionsCmd(token, userID, "followers", "Followers of a user", longUserFollowers, "followers"),
		s.newUserConnectionsCmd(token, userID, "following", "Users a user follows", longUserFollowing, "following"),
	)
	return cmd
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	var id, username string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a user by id or username",
		Long: "Exactly one of --id or --username is required. --username takes the handle\n" +
			"without the leading @ (1-15 letters, digits or underscores); --id takes the\n" +
			"numeric id. This is the only command that accepts a handle — `follow\n" +
			"create`, `timeline user`, `user followers` and `user following` all want a\n" +
			"numeric user id, so resolve the handle here first.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireExactlyOne("--id", id, "--username", username); err != nil {
				return err
			}
			path := ""
			if id != "" {
				if err := requireNumericID("id", id); err != nil {
					return err
				}
				path = "/2/users/" + url.PathEscape(id)
			} else {
				if err := requireUsername(username); err != nil {
					return err
				}
				path = "/2/users/by/username/" + url.PathEscape(username)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "X user id")
	cmd.Flags().StringVar(&username, "username", "", "X username without @")
	return cmd
}

func (s *Service) newUserSearchCmd(token string) *cobra.Command {
	var query, nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search users (one page)",
		Long: "Matches accounts — name, handle, profile text — not post content; searching\n" +
			"what people wrote is `post search`. --limit is 1-1000, default 10. Continue\n" +
			"with --next-token from the response.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" {
				return fmt.Errorf("query is required")
			}
			if err := requireLimit(limit, 1, 1000); err != nil {
				return err
			}
			values := url.Values{
				"query":       {query},
				"max_results": {strconv.Itoa(limit)},
			}
			if nextToken != "" {
				values.Set("next_token", nextToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/users/search", values, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "user search query")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum users in this page")
	cmd.Flags().StringVar(&nextToken, "next-token", "", "provider token for the next page")
	return cmd
}
