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
		Use:   "list",
		Short: "List campaign reports (GET /reports)",
		Long: "One row per SENT campaign: drafts and scheduled campaigns never appear, so a\n" +
			"missing id usually means the campaign has not gone out rather than that it\n" +
			"does not exist. Each row already carries `emails_sent`, `opens`, `clicks`,\n" +
			"`bounces` and `unsubscribed`, which is often enough without a follow-up\n" +
			"`report get`.",
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
		Use:   "get <campaign_id>",
		Short: "Get one campaign report (GET /reports/{campaign_id})",
		Long: "Keyed by the CAMPAIGN id — the same id `campaign create` returned — not by a\n" +
			"separate report id. Adds the breakdowns the list rows omit: `open_rate` and\n" +
			"`click_rate`, bounces split into hard and soft, `forwards`, `industry_stats`\n" +
			"for comparison against the account's sector, and a `timeseries` for the first\n" +
			"24 hours after sending. A campaign that has not been sent has no report.",
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
		Use:   "list",
		Short: "List templates (GET /templates)",
		Long: "Templates are authored in the Mailchimp web app; this tool lists them and can\n" +
			"render a campaign from one, but cannot create, edit or delete them. The `id`\n" +
			"here is what `campaign set-content --template` takes. The response mixes\n" +
			"Mailchimp's own gallery templates with the account's saved ones, so check the\n" +
			"`type` on each row before assuming a template is user-made.",
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
