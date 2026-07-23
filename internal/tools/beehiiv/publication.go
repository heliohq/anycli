package beehiiv

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newPublicationCmd(token string) *cobra.Command {
	cmd := newGroupCmd("publication", "Publications the credential can see (list, get)")
	cmd.AddCommand(
		s.newPublicationListCmd(token),
		s.newPublicationGetCmd(token),
	)
	return cmd
}

func (s *Service) newPublicationListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List publications (GET /publications)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/publications", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newPublicationGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <publicationId>",
		Short: "Get one publication (GET /publications/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pub, err := requirePublicationID(args[0])
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/publications/"+pub, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
