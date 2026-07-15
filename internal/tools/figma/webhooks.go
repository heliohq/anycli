package figma

import "github.com/spf13/cobra"

func (s *Service) newWebhooksCommand(token string) *cobra.Command {
	webhooks := &cobra.Command{Use: "webhooks", Short: "Read and manage Figma webhooks"}
	webhooks.AddCommand(
		s.newOperationCommand(token, operationCommandSpec{Use: "list", Short: "List webhooks by context or plan", OperationID: "getWebhooks"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "create", Short: "Create a webhook", OperationID: "postWebhook"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "get", Short: "Get a webhook", OperationID: "getWebhook"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "update", Short: "Update a webhook", OperationID: "putWebhook"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "delete", Short: "Delete a webhook", OperationID: "deleteWebhook"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "requests", Short: "List recent delivery attempts for a webhook", OperationID: "getWebhookRequests"}),
		s.newOperationCommand(token, operationCommandSpec{Use: "team", Short: "List team webhooks (deprecated upstream)", OperationID: "getTeamWebhooks"}),
	)
	return webhooks
}
