package sendgrid

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newTemplateCmd(token string, region *string) *cobra.Command {
	cmd := &cobra.Command{Use: "template", Short: "Dynamic transactional templates (list, get)"}
	cmd.AddCommand(
		s.newTemplateListCmd(token, region),
		s.newTemplateGetCmd(token, region),
	)
	return cmd
}

func (s *Service) newTemplateListCmd(token string, region *string) *cobra.Command {
	var pageSize int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List dynamic templates (GET /v3/templates?generations=dynamic)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("generations", "dynamic")
			q.Set("page_size", intToString(pageSize))
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/templates", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "templates per page (1-200)")
	return cmd
}

func (s *Service) newTemplateGetCmd(token string, region *string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a template with its versions (GET /v3/templates/{id})",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, *region, http.MethodGet, "/templates/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "template id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
