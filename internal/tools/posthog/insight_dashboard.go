package posthog

import (
	"github.com/spf13/cobra"
)

// The four saved-work Longs live here rather than at the call site because
// every one of these leaves is built by a shared project-scoped constructor.
const (
	longInsightList = "Returns insights the team has already saved and named in PostHog, not\n" +
		"computed numbers. `--search` matches the insight name, so a broad term is\n" +
		"the way in when the exact one is unknown. For a question nobody has saved a\n" +
		"chart for, `query run` is the path instead."

	longInsightGet = "Returns one insight's full definition, including the query node behind it.\n" +
		"That node can be handed to `query run --query-json` to re-run the same\n" +
		"computation with an edited filter, which is cheaper and more faithful than\n" +
		"reconstructing the query by hand. --id is the numeric insight id."

	longDashboardList = "`--search` matches the dashboard name. The rows are metadata only — the\n" +
		"tiles a dashboard holds come from `dashboard get`."

	longDashboardGet = "Expands one dashboard into its tiles. Each tile names the insight it renders,\n" +
		"which makes this the cheapest way to discover the insights a team actually\n" +
		"watches; read them with `insight get`. Tiles carry layout and filter\n" +
		"overrides, never computed numbers."
)

// newInsightCmd groups saved-insight read access.
func (s *Service) newInsightCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "insight", Short: "Saved insights (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List insights (GET /api/projects/<id>/insights/)", longInsightList, "/insights/", true),
		s.newProjectGetCmd(token, "get", "Get an insight (GET /api/projects/<id>/insights/<id>/)", longInsightGet, "/insights/"),
	)
	return cmd
}

// newDashboardCmd groups dashboard read access.
func (s *Service) newDashboardCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "dashboard", Short: "Dashboards (list, get)"}
	cmd.AddCommand(
		s.newProjectListCmd(token, "list", "List dashboards (GET /api/projects/<id>/dashboards/)", longDashboardList, "/dashboards/", true),
		s.newProjectGetCmd(token, "get", "Get a dashboard (GET /api/projects/<id>/dashboards/<id>/)", longDashboardGet, "/dashboards/"),
	)
	return cmd
}
