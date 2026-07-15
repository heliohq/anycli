package figma

import "github.com/spf13/cobra"

func (s *Service) newVariablesCommand(token string) *cobra.Command {
	variables := &cobra.Command{Use: "variables", Short: "Read and update Figma variables (Enterprise)"}
	variables.AddCommand(
		s.newOperationCommand(token, operationCommandSpec{Use: "local", Short: "Get local variables in a file", OperationID: "getLocalVariables"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "published", Short: "Get published variables in a file", OperationID: "getPublishedVariables"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "update", Short: "Bulk create, update, or delete variables", OperationID: "postVariables"}),
	)
	return variables
}
