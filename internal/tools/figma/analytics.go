package figma

import "github.com/spf13/cobra"

func (s *Service) newAnalyticsCommand(token string) *cobra.Command {
	analytics := &cobra.Command{Use: "analytics", Short: "Read Enterprise library analytics"}
	specs := []operationCommandSpec{
		{Use: "component-actions", Short: "Get component insert and detach actions", OperationID: "getLibraryAnalyticsComponentActions"},
		{Use: "component-usages", Short: "Get component usage counts", OperationID: "getLibraryAnalyticsComponentUsages"},
		{Use: "style-actions", Short: "Get style insert and detach actions", OperationID: "getLibraryAnalyticsStyleActions"},
		{Use: "style-usages", Short: "Get style usage counts", OperationID: "getLibraryAnalyticsStyleUsages"},
		{Use: "variable-actions", Short: "Get variable insert and detach actions", OperationID: "getLibraryAnalyticsVariableActions"},
		{Use: "variable-usages", Short: "Get variable usage counts", OperationID: "getLibraryAnalyticsVariableUsages"},
	}
	for _, spec := range specs {
		analytics.AddCommand(s.newOperationCommand(token, spec))
	}
	return analytics
}
