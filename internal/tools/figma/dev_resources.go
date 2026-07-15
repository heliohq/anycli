package figma

import "github.com/spf13/cobra"

func (s *Service) newDevResourcesCommand(token string) *cobra.Command {
	resources := &cobra.Command{Use: "dev-resources", Short: "Read and manage Figma Dev Mode resources"}
	resources.AddCommand(
		s.newOperationCommand(token, operationCommandSpec{Use: "list", Short: "List dev resources for a file or nodes", OperationID: "getDevResources"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "create", Short: "Create dev resources", OperationID: "postDevResources"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "update", Short: "Update dev resources", OperationID: "putDevResources"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "delete", Short: "Delete a dev resource", OperationID: "deleteDevResource"}),
	)
	return resources
}
