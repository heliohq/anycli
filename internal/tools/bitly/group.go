package bitly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newGroupCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "group", Short: "Groups (list, get, tags, aggregate metrics)"}
	cmd.AddCommand(
		s.newGroupListCmd(token),
		s.newGroupMetricCmd(token, "get", "Get a group (GET /groups/{group_guid})", "", false, false),
		s.newGroupMetricCmd(token, "tags", "List a group's tags (GET /groups/{group_guid}/tags)", "/tags", false, false),
		s.newGroupMetricCmd(token, "shorten-counts", "Shorten counts over time (GET /groups/{group_guid}/shorten_counts)", "/shorten_counts", true, false),
		s.newGroupMetricCmd(token, "clicks", "Group clicks over time (GET /groups/{group_guid}/clicks)", "/clicks", true, false),
		s.newGroupMetricCmd(token, "countries", "Group clicks by country (GET /groups/{group_guid}/countries)", "/countries", true, true),
		s.newGroupMetricCmd(token, "referrers", "Group clicks by referrer (GET /groups/{group_guid}/referrers)", "/referrers", true, true),
		s.newGroupMetricCmd(token, "devices", "Group clicks by device (GET /groups/{group_guid}/devices)", "/devices", true, true),
		s.newGroupMetricCmd(token, "cities", "Group clicks by city (GET /groups/{group_guid}/cities)", "/cities", true, true),
	)
	return cmd
}

func (s *Service) newGroupListCmd(token string) *cobra.Command {
	var organization string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List groups (GET /groups)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if organization != "" {
				q.Set("organization_guid", organization)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/groups", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&organization, "organization", "", "filter by organization guid")
	return cmd
}

// newGroupMetricCmd builds one group command keyed on a group_guid path
// segment (auto-resolved when --group is omitted). withAnalytics adds the
// unit-window flags; withSize adds the breakdown --size flag.
func (s *Service) newGroupMetricCmd(token, use, short, suffix string, withAnalytics, withSize bool) *cobra.Command {
	var group string
	var analytics analyticsParams
	var size int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // all group endpoints here are GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			guid, err := s.resolveGroup(cmd.Context(), token, group)
			if err != nil {
				return err
			}
			q := url.Values{}
			if withAnalytics {
				analytics.apply(q)
			}
			if withSize {
				q.Set("size", intToString(size))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/groups/"+url.PathEscape(guid)+suffix, q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group guid (auto-resolved when omitted)")
	if withAnalytics {
		registerAnalyticsFlags(cmd, &analytics)
	}
	if withSize {
		registerSizeFlag(cmd, &size)
	}
	return cmd
}
