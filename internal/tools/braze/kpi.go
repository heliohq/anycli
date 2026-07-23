package braze

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newKPICmd builds the `kpi` resource group: dau / mau / new-users /
// uninstalls, each a GET /kpi/{metric}/data_series over a day window.
func (s *Service) newKPICmd(c *client) *cobra.Command {
	group := newGroupCmd("kpi", "Workspace KPI time-series (DAU, MAU, new users, uninstalls)")
	group.AddCommand(
		s.newKPIMetricCmd(c, "dau", "dau", "Daily active users by date"),
		s.newKPIMetricCmd(c, "mau", "mau", "Monthly active users (rolling 30 days) by date"),
		s.newKPIMetricCmd(c, "new-users", "new_users", "Daily new users by date"),
		s.newKPIMetricCmd(c, "uninstalls", "uninstalls", "Daily app uninstalls by date"),
	)
	return group
}

// newKPIMetricCmd builds one KPI subcommand mapping `use` to
// GET /kpi/{metric}/data_series with the shared --length / --ending-at window.
func (s *Service) newKPIMetricCmd(c *client, use, metric, short string) *cobra.Command {
	var endingAt string
	var length int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	cmd.Flags().IntVar(&length, "length", 14, "number of days (max 100) ending at --ending-at")
	cmd.Flags().StringVar(&endingAt, "ending-at", "", "ISO-8601 end date/time (optional; default now)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		q.Set("length", strconv.Itoa(length))
		if endingAt != "" {
			q.Set("ending_at", endingAt)
		}
		body, err := c.get(cmd.Context(), "/kpi/"+metric+"/data_series", q)
		if err != nil {
			return err
		}
		return c.emit(body)
	}
	return cmd
}
