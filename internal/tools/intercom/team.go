package intercom

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newTeamCmd builds the team resource group: the teams a conversation can be
// assigned to.
func (s *Service) newTeamCmd(token string) *cobra.Command {
	cmd := newGroupCmd("team", "Teams: list, get")
	cmd.AddCommand(
		s.newTeamListCmd(token),
		s.newTeamGetCmd(token),
	)
	return cmd
}

func (s *Service) newTeamListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List teams (GET /teams)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/teams", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newTeamGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one team (GET /teams/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/teams/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "team id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
