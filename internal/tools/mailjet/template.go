package mailjet

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newTemplateCmd groups reusable-template discovery/inspection over
// /v3/REST/template.
func (s *Service) newTemplateCmd(basic string) *cobra.Command {
	cmd := newGroupCmd("template", "Discover and inspect email templates (list, get)")
	cmd.AddCommand(
		s.newTemplateListCmd(basic),
		s.newTemplateGetCmd(basic),
	)
	return cmd
}

func (s *Service) newTemplateListCmd(basic string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List templates (GET /v3/REST/template)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("Limit", itoa(limit))
			q.Set("Offset", itoa(offset))
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/template", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "max templates to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}

// newTemplateGetCmd fetches a template's editable content
// (GET /v3/REST/template/{id}/detailcontent).
func (s *Service) newTemplateGetCmd(basic string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a template's content (GET /v3/REST/template/{id}/detailcontent)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/template/"+url.PathEscape(id)+"/detailcontent", nil, nil)
			if err != nil {
				return err
			}
			return s.emitOne(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "template ID")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
