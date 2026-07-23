package docusign

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTemplateListCmd(c *apiClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List reusable templates a send can reference by id",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.callJSON(cmd.Context(), http.MethodGet, "/templates", nil, nil)
			if err != nil {
				return err
			}
			var raw rawTemplateList
			if err := decodeInto(body, &raw); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			views := make([]templateView, 0, len(raw.EnvelopeTemplates))
			for _, t := range raw.EnvelopeTemplates {
				views = append(views, t.view())
			}
			if jsonMode(cmd) {
				return emitJSON(out, map[string]any{"templates": views, "count": len(views)})
			}
			for _, v := range views {
				emitLine(out, v.ID, v.Name)
			}
			return nil
		},
	}
	return cmd
}

func (s *Service) newTemplateGetCmd(c *apiClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <template-id>",
		Short:       "Inspect one template",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := c.callJSON(cmd.Context(), http.MethodGet, "/templates/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			var raw rawTemplate
			if err := decodeInto(body, &raw); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			v := raw.view()
			if jsonMode(cmd) {
				return emitJSON(out, v)
			}
			emitLine(out, v.ID, v.Name, v.Description)
			return nil
		},
	}
	return cmd
}
