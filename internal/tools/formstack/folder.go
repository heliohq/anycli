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
		Long: "Returns every folder on the account, unpaged and unfiltered; folders\n" +
			"cannot be created, renamed or deleted from here. The `id` is what\n" +
			"`form list --folder` and `form create --folder` take. Forms that sit in\n" +
			"no folder are invisible to a folder filter, so folders are not a\n" +
			"reliable way to enumerate an account's forms — `form list` is.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
