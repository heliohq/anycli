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
		Args:  cobra.NoArgs,
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
