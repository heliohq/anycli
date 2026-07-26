package loops

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newListCmd groups mailing-list operations. Loops mailing lists carry the ids
// used with --mailing-list on contact/event commands.
func (s *Service) newListCmd(key string) *cobra.Command {
	cmd := newGroup("list", "Mailing lists")
	cmd.AddCommand(s.newListLsCmd(key))
	return cmd
}

func (s *Service) newListLsCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List mailing lists (GET /v1/lists)",
		Long: "Returns each list's id, name and whether it is public. Those ids are what\n" +
			"--mailing-list id=true|false takes on `contact create`, `contact update`\n" +
			"and `event send`. Nothing here creates, renames or deletes a list, and no\n" +
			"command enumerates a list's members.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/v1/lists", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
