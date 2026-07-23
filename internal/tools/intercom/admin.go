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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
