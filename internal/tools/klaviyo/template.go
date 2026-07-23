package klaviyo

import "github.com/spf13/cobra"

// newTemplateCmd builds the `template` group: list/get (read-only).
func (s *Service) newTemplateCmd(token string) *cobra.Command {
	group := newGroupCmd("template", "Read email templates")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List templates (GET /templates)", "/templates", "template"),
		s.newResourceGetCmd(token, "get", "Get one template (GET /templates/{id})", "/templates/", "template"),
	)
	return group
}
