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
		Use:   "list",
		Short: "List templates (GET /templates)",
		Long: "Returns each template's numeric id, alias, name, active flag and type (a\n" +
			"standard template or a layout) — never its content. Pass either the id or\n" +
			"the alias to `email send-template`; the alias is the same across a user's\n" +
			"servers while the numeric id is not. Pages with `--count` (default 100,\n" +
			"Postmark caps it at 500) and `--offset`.",
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
		Use:   "get <id-or-alias>",
		Short: "Get one template (GET /templates/{idOrAlias})",
		Long: "Accepts the numeric id or the alias interchangeably. Returns the template\n" +
			"source — `Subject`, `HtmlBody`, `TextBody` — with its `{{placeholders}}`\n" +
			"intact, which is how to discover the keys `email send-template --model` has\n" +
			"to supply. Templates cannot be created, edited or deleted from here;\n" +
			"authoring happens in Postmark.",
		Args:        requireArgs(1, "get requires a <id-or-alias>"),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getAndEmit(cmd.Context(), token, "/templates/"+url.PathEscape(args[0]), nil)
		},
	}
}
