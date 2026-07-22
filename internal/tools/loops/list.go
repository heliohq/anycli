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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/v1/lists", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
