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
		Use:   "list",
		Short: "List dynamic templates (GET /v3/templates?generations=dynamic)",
		Long: "Hard-filtered to dynamic templates, so a legacy transactional template\n" +
			"visible in the SendGrid UI never appears here and its id will not work\n" +
			"with `mail send --template-id`. --page-size is 1-200, default 100.\n" +
			"Entries carry the template name and version summaries; the version body\n" +
			"comes from `template get`.",
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
		Use:   "get",
		Short: "Get a template with its versions (GET /v3/templates/{id})",
		Long: "--id is required and takes the dynamic template id, the `d-` prefixed\n" +
			"form, not the version id. The active version's subject and HTML come\n" +
			"back with it, and the {{placeholders}} inside them are the exact keys\n" +
			"`mail send --data` must supply — read this before composing a templated\n" +
			"send rather than guessing the field names.",
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
