package formstack

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newFolderCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "folder", Short: "Folders (list)"}
	cmd.AddCommand(s.newFolderListCmd(token))
	return cmd
}

func (s *Service) newFolderListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List folders (GET /folder.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/folder.json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
