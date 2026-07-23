package posthog

import (
	"github.com/spf13/cobra"
)

// newInsightCmd groups saved-insight read access.
func (s *Service) newInsightCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "insight", Short: "Saved insights (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List insights (GET /api/projects/<id>/insights/)", "/insights/", true),
		s.newProjectGetCmd(token, "get", "Get an insight (GET /api/projects/<id>/insights/<id>/)", "/insights/"),
	)
	return cmd
}

// newDashboardCmd groups dashboard read access.
func (s *Service) newDashboardCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "dashboard", Short: "Dashboards (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List dashboards (GET /api/projects/<id>/dashboards/)", "/dashboards/", true),
		s.newProjectGetCmd(token, "get", "Get a dashboard (GET /api/projects/<id>/dashboards/<id>/)", "/dashboards/"),
	)
	return cmd
}
