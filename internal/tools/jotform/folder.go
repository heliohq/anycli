package jotform

import "github.com/spf13/cobra"

func (s *Service) newFolderCmd(key string) *cobra.Command {
	cmd := newGroupCmd("folder", "List the account's folder tree")
	cmd.AddCommand(s.newFolderListCmd(key))
	return cmd
}

func (s *Service) newFolderListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List folders (GET /user/folders)",
		Long: "Folders organize forms in the Jotform UI, and no other command here takes a\n" +
			"folder id — so this answers \"how is this account arranged\" rather than feeding\n" +
			"a later call. The response is a nested tree, each folder carrying its\n" +
			"subfolders and the ids of the forms inside it, which makes it a cheap way to\n" +
			"see which forms belong together without paging `form list`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), key, "/user/folders", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
