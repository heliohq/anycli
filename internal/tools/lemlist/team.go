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
		Long: "The account this key belongs to: the team `_id`, its name and the plan it\n" +
			"runs on. The cheapest confirmation that the credential is live and\n" +
			"points at the team you think it does — worth one call before enrolling\n" +
			"leads into campaigns that belong to someone else.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "The members who can send, each with the campaigns they send from. A\n" +
			"campaign goes out through its sender's own mailbox, so this is what\n" +
			"connects a campaign id to the person whose sending reputation and daily\n" +
			"limits are actually at stake — and how a campaign with no working\n" +
			"sender shows up before it silently fails to send.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "What is left on the account. Lemlist meters enrichment separately from\n" +
			"sending, so read this before a batch that would spend either: an\n" +
			"exhausted balance surfaces later as a failed enrichment or a campaign\n" +
			"that stops moving, not as an error on the call that spent the last\n" +
			"credit.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/team/credits", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
