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
		Long: "The only command needing no `--publication-id`, and where those ids come\n" +
			"from: each entry's `id` is the `pub_…` value everything else requires.\n" +
			"A connection can see several publications and nothing picks a default,\n" +
			"so this is the first call of a session. Entries carry the name and\n" +
			"organization, which is how two similarly named newsletters are told\n" +
			"apart.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "The one place a publication is a positional argument rather than the\n" +
			"`--publication-id` flag; the `pub_` prefix is still checked locally.\n" +
			"Returns that publication's own record — name, organization and\n" +
			"creation time — and takes no `--expand`, so subscriber and post\n" +
			"numbers come from `subscription list` and `post list --expand stats`\n" +
			"instead.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
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
