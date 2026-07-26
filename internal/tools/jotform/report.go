package jotform

import (
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newReportCmd(key string) *cobra.Command {
	cmd := newGroupCmd("report", "List shareable report views")
	cmd.AddCommand(s.newReportListCmd(key))
	return cmd
}

func (s *Service) newReportListCmd(key string) *cobra.Command {
	var form string
	cmd := &cobra.Command{
		Use:   "list [--form <formID>]",
		Short: "List reports account-wide (GET /user/reports) or for one form (GET /form/{id}/reports)",
		Long: "A Jotform report is a shareable read-only view of a form's results — a table,\n" +
			"calendar or card wall — with its own public URL that can be handed out without\n" +
			"granting access to the account. The rows carry that URL, the report type and\n" +
			"the form it belongs to. Listing is all this tool does; creating, updating or\n" +
			"deleting a report is not exposed.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/user/reports"
			if form != "" {
				path = "/form/" + url.PathEscape(form) + "/reports"
			}
			body, err := s.get(cmd.Context(), key, path, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&form, "form", "", "scope reports to one form id")
	return cmd
}
