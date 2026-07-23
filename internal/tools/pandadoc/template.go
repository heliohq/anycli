package pandadoc

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTemplateListCmd(authz string) *cobra.Command {
	var q string
	var count, page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List templates",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setIf(query, "q", q)
			if cmd.Flags().Changed("count") {
				query.Set("count", fmt.Sprintf("%d", count))
			}
			if cmd.Flags().Changed("page") {
				query.Set("page", fmt.Sprintf("%d", page))
			}
			body, err := s.call(cmd.Context(), authz, http.MethodGet, "/templates", query, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderList(body)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "search by template name")
	cmd.Flags().IntVar(&count, "count", 0, "max results per page")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

func (s *Service) newTemplateDetailsCmd(authz string) *cobra.Command {
	return &cobra.Command{
		Use:         "details <id>",
		Short:       "Show a template's roles, tokens, and fields",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), authz, http.MethodGet, "/templates/"+url.PathEscape(args[0])+"/details", nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderItem(body)
		},
	}
}
