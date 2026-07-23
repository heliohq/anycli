package tally

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newWorkspaceCmd(token string) *cobra.Command {
	cmd := newGroupCmd("workspace", "Workspaces (list)")
	cmd.AddCommand(s.newWorkspaceListCmd(token))
	return cmd
}

func (s *Service) newWorkspaceListCmd(token string) *cobra.Command {
	var page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List workspaces (GET /workspaces)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if cmd.Flags().Changed("page") {
				q.Set("page", strconv.Itoa(page))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/workspaces", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	return cmd
}

func (s *Service) newMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "me",
		Short:       "Get the authenticated user (GET /users/me)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
