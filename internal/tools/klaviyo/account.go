package klaviyo

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newAccountCmd builds `account get` → GET /accounts. Klaviyo tokens bind to a
// single account, so `get` returns the (single-element) accounts collection
// Klaviyo recommends as the post-install identity call.
func (s *Service) newAccountCmd(token string) *cobra.Command {
	group := newGroupCmd("account", "Read the connected Klaviyo account")
	get := &cobra.Command{
		Use:   "get",
		Short: "Get the connected account (GET /accounts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/accounts", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	group.AddCommand(get)
	return group
}
