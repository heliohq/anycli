package missive

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newDraftsCmd builds the `drafts` group. A draft is a customer-facing reply
// (email or SMS); with send:true in the body Missive sends it immediately.
func (s *Service) newDraftsCmd(token string) *cobra.Command {
	group := newGroupCmd("drafts", "Draft and send customer-facing replies")
	group.AddCommand(s.newDraftsCreateCmd(token))
	return group
}

func (s *Service) newDraftsCreateCmd(token string) *cobra.Command {
	var inline, file string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create (and optionally send) a draft (POST /drafts). Body: {\"drafts\":{...}}",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := s.decodeJSONBody(inline, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/drafts", nil, payload)
			if err != nil {
				return err
			}
			return s.emitBodyOrOK(body)
		},
	}
	addBodyFlags(cmd, &inline, &file)
	return cmd
}
