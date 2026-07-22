package metaads

import (
	"net/url"

	"github.com/spf13/cobra"
)

const defaultAccountFields = "id,name,account_status,currency,amount_spent,business_name,timezone_name"

// newAccountsCmd is the ad-account discovery command. An assistant runs
// `accounts list` first, then passes the chosen act_<id> to every other
// command via --account. The connection identity is the Facebook user, not an
// ad account, so account targeting is per-command, never connection state.
func (s *Service) newAccountsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "accounts", Short: "Ad accounts the connected user can access"}
	cmd.AddCommand(s.newAccountsListCmd(token))
	return cmd
}

func (s *Service) newAccountsListCmd(token string) *cobra.Command {
	var fields string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ad accounts (GET /me/adaccounts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLimit(limit, 1, 500); err != nil {
				return err
			}
			query := url.Values{
				"fields": {fields},
				"limit":  {itoa(limit)},
			}
			body, err := s.get(cmd.Context(), token, "/me/adaccounts", query)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultAccountFields, "comma-separated account fields")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum accounts in this page (1-500)")
	return cmd
}
