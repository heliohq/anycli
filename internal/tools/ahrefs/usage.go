package ahrefs

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newUsageCmd wraps GET /subscription-info/limits-and-usage: the plan, unit
// limits/usage, and reset date. This endpoint is free (0 units) and takes no
// parameters, so it doubles as the connection health / verify probe.
func (s *Service) newUsageCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Plan, API unit limits/usage, and reset date (free; GET /subscription-info/limits-and-usage)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/subscription-info/limits-and-usage", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
