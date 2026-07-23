package googleads

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newAccountsListCmd is the entry point: GET customers:listAccessibleCustomers
// returns the resource names of every account the OAuth user can reach. It
// takes no customer id (and ignores login-customer-id server-side), so the
// assistant runs it first, then targets a specific account with --customer-id.
func (s *Service) newAccountsListCmd(c creds) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List reachable customer accounts (GET customers:listAccessibleCustomers)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// listAccessibleCustomers ignores login-customer-id and takes no
			// customer id; send it without the manager header to avoid any
			// server-side surprise.
			bare := creds{accessToken: c.accessToken, developerToken: c.developerToken}
			body, err := s.call(cmd.Context(), bare, http.MethodGet, "/customers:listAccessibleCustomers", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
