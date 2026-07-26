package intercom

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAdminCmd builds the admin resource group: workspace orientation — who am
// I (/me), and who are the teammates (list/get).
func (s *Service) newAdminCmd(token string) *cobra.Command {
	cmd := newGroupCmd("admin", "Admins (teammates): me, list, get")
	cmd.AddCommand(
		s.newAdminMeCmd(token),
		s.newAdminListCmd(token),
		s.newAdminGetCmd(token),
	)
	return cmd
}

func (s *Service) newAdminMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Identify the authenticating admin and workspace (GET /me)",
		Long: "Returns the admin id that every write is attributed to when --admin-id is\n" +
			"omitted, alongside the workspace `app` object. This is the same lookup\n" +
			"those writes perform implicitly, so running it once and reusing the id\n" +
			"saves a request per write. Cheapest way to confirm which workspace a token\n" +
			"actually points at when more than one is connected.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newAdminListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List admins (GET /admins)",
		Long: "Returns every teammate in the workspace in one unpaginated response. These\n" +
			"are the ids --admin-id takes, and one of the two id spaces --assignee-id\n" +
			"accepts; team ids come from `team list` and are not interchangeable with\n" +
			"these even though the same flag carries both.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/admins", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newAdminGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one admin (GET /admins/{id})",
		Long: "Takes an Intercom admin id. There is no lookup by email or name, so\n" +
			"resolve a teammate through `admin list` first; ids also appear on a\n" +
			"conversation's assignee and on each admin-authored part.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/admins/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "admin id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
