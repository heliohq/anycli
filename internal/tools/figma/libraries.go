package figma

import "github.com/spf13/cobra"

func (s *Service) newLibrariesCommand(token string) *cobra.Command {
	libraries := &cobra.Command{Use: "libraries", Short: "Read published Figma library assets"}
	libraries.AddCommand(
		s.newLibraryAssetGroup(token, "components", "components", "getTeamComponents", "getFileComponents", "getComponent"),
		s.newLibraryAssetGroup(token, "component-sets", "component sets", "getTeamComponentSets", "getFileComponentSets", "getComponentSet"),
		s.newLibraryAssetGroup(token, "styles", "styles", "getTeamStyles", "getFileStyles", "getStyle"),
	)
	return libraries
}

func (s *Service) newLibraryAssetGroup(token, use, label, teamOperation, fileOperation, getOperation string) *cobra.Command {
	group := &cobra.Command{Use: use, Short: "Read published Figma " + label}
	group.AddCommand(
		s.newOperationCommand(token, operationCommandSpec{Use: "team", Short: "List " + label + " published by a team", OperationID: teamOperation}),
		s.newOperationCommand(token, operationCommandSpec{Use: "file", Short: "List " + label + " published by a file", OperationID: fileOperation}),
		s.newOperationCommand(token, operationCommandSpec{Use: "get", Short: "Get one published " + label + " asset", OperationID: getOperation}),
	)
	return group
}
