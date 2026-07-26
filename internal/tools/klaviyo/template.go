package klaviyo

import "github.com/spf13/cobra"

// The template Longs. They sit beside this group rather than in the generic
// builders in common.go because the builders are resource-agnostic, while
// "read-only, unrendered" is specific to templates.
const (
	longTemplateList = "Email templates, read-only here: nothing in this tool creates, edits,\n" +
		"renders or clones one. Cursor-paged like every list — carry `page[cursor]`\n" +
		"from links.next into --cursor."

	longTemplateGet = "One template's stored HTML and text. Klaviyo's template variables come back\n" +
		"as written rather than filled in, so this is the source and not what any\n" +
		"particular recipient would receive."
)

// newTemplateCmd builds the `template` group: list/get (read-only).
func (s *Service) newTemplateCmd(token string) *cobra.Command {
	group := newGroupCmd("template", "Read email templates")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List templates (GET /templates)", longTemplateList, "/templates", "template"),
		s.newResourceGetCmd(token, "get", "Get one template (GET /templates/{id})", longTemplateGet, "/templates/", "template"),
	)
	return group
}
