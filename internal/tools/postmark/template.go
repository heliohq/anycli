package postmark

import (
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTemplateCmd(token string) *cobra.Command {
	group := newGroupCmd("template", "Discover email templates")
	group.AddCommand(s.newTemplateListCmd(token), s.newTemplateGetCmd(token))
	return group
}

func (s *Service) newTemplateListCmd(token string) *cobra.Command {
	var count, offset int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List templates (GET /templates)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("count", itoa(count))
			q.Set("offset", itoa(offset))
			return s.getAndEmit(cmd.Context(), token, "/templates", q)
		},
	}
	registerPaging(cmd, &count, &offset)
	return cmd
}

func (s *Service) newTemplateGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id-or-alias>",
		Short:       "Get one template (GET /templates/{idOrAlias})",
		Args:        requireArgs(1, "get requires a <id-or-alias>"),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getAndEmit(cmd.Context(), token, "/templates/"+url.PathEscape(args[0]), nil)
		},
	}
}
