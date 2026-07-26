package kit

import (
	"net/http"

	"github.com/spf13/cobra"
)

// accountCmd groups the account (identity + stats) commands.
func (s *Service) accountCmd(token string) *cobra.Command {
	group := newGroupCmd("account", "Account identity and stats")

	get := &cobra.Command{
		Use:   "get",
		Short: "Show the authenticated account (whoami + plan)",
		Long: "Identifies the creator account every command writes to, along with its\n" +
			"plan — which is what gates several Kit features, so a capability that\n" +
			"appears missing may be a plan limit rather than an API gap. Cheapest check\n" +
			"that the credential still works.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/account", nil, nil)
			if err != nil {
				return err
			}
			return s.emitData(body, "")
		},
	}

	var growth, email bool
	stats := &cobra.Command{
		Use:   "stats",
		Short: "Show growth or email stats",
		Long: "Exactly one of --growth or --email is required; they are two different\n" +
			"endpoints, not two views of one. --growth reports how the subscriber list\n" +
			"has changed over time, --email the account-wide send, open and click\n" +
			"rates. Neither breaks down by newsletter — the numbers for a single send\n" +
			"are `broadcast stats <id>`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if growth == email {
				return &usageError{msg: "exactly one of --growth or --email is required"}
			}
			path := "/account/growth_stats"
			if email {
				path = "/account/email_stats"
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emitData(body, "")
		},
	}
	stats.Flags().BoolVar(&growth, "growth", false, "report list growth stats")
	stats.Flags().BoolVar(&email, "email", false, "report email send/open/click stats")

	group.AddCommand(get, stats)
	return group
}
