package lemlist

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newTeamCmd groups account-context reads. `team get` doubles as the identity /
// verify endpoint (GET /team returns the team _id + name).
func (s *Service) newTeamCmd(key string) *cobra.Command {
	cmd := newGroupCmd("team", "Account context: team, senders, credits")
	cmd.AddCommand(
		s.newTeamGetCmd(key),
		s.newTeamSendersCmd(key),
		s.newTeamCreditsCmd(key),
	)
	return cmd
}

func (s *Service) newTeamGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get the authenticated team (GET /team)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/team", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newTeamSendersCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "senders",
		Short: "List team senders and their campaigns (GET /team/senders)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/team/senders", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newTeamCreditsCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "credits",
		Short: "Get remaining enrichment/send credits (GET /team/credits)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/team/credits", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
