package figma

import "github.com/spf13/cobra"

func (s *Service) newOEmbedCommand(token string) *cobra.Command {
	group := &cobra.Command{Use: "oembed", Short: "Read embeddable metadata for a Figma URL"}
	group.AddCommand(s.newOperationCommand(token, operationCommandSpec{Use: "get", Short: "Get oEmbed metadata", OperationID: "getOEmbed"}))
	return group
}

func (s *Service) newPaymentsCommand(token string) *cobra.Command {
	group := &cobra.Command{Use: "payments", Short: "Read Figma Community payment status"}
	group.AddCommand(s.newOperationCommand(token, operationCommandSpec{Use: "list", Short: "Get payment status", OperationID: "getPayments"}))
	return group
}
