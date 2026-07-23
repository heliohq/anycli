package mailchimp

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newReportCmd builds the report group: list and get.
func (s *Service) newReportCmd(r *requester) *cobra.Command {
	group := newGroupCmd("report", "Campaign performance reports")
	group.AddCommand(
		s.newReportListCmd(r),
		s.newReportGetCmd(r),
	)
	return group
}

func (s *Service) newReportListCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List campaign reports (GET /reports)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := r.do(cmd.Context(), http.MethodGet, "/reports", listQuery(cmd), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd)
	return cmd
}

func (s *Service) newReportGetCmd(r *requester) *cobra.Command {
	return &cobra.Command{
		Use:         "get <campaign_id>",
		Short:       "Get one campaign report (GET /reports/{campaign_id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := r.do(cmd.Context(), http.MethodGet, "/reports/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newTemplateCmd builds the template group: list.
func (s *Service) newTemplateCmd(r *requester) *cobra.Command {
	group := newGroupCmd("template", "Email templates")
	group.AddCommand(s.newTemplateListCmd(r))
	return group
}

func (s *Service) newTemplateListCmd(r *requester) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List templates (GET /templates)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := r.do(cmd.Context(), http.MethodGet, "/templates", listQuery(cmd), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd)
	return cmd
}
