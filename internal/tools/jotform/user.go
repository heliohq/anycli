package jotform

import "github.com/spf13/cobra"

func (s *Service) newUserCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "user",
		Short: "Get the authenticated account (GET /user)",
		Long: "The identity read: username, email, account `status` and the plan tier. The\n" +
			"plan matters because Jotform gates submission volume, upload storage and API\n" +
			"calls by tier, and `usage` reports the counters against it. This works with\n" +
			"either kind of key, so a 401 here means the key itself is bad — unlike a 401\n" +
			"on a write, which means the key is merely read-only.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), key, "/user", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newUsageCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "usage",
		Short: "Get API/usage counters for the account (GET /user/usage)",
		Long: "The current period's counters — API calls made, submissions received, form\n" +
			"views and upload storage — against the plan's limits. Worth reading before a\n" +
			"batch of writes: exhausting the API-call allowance fails EVERY subsequent\n" +
			"request until the period rolls over, not just the one that crossed the line.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), key, "/user/usage", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
