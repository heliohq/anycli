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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), key, "/user/folders", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
