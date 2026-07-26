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
		Long: "A Klaviyo token binds to exactly one account, so this returns a\n" +
			"one-element collection rather than a scalar. Its attributes carry the\n" +
			"public API key, the contact details and the account TIMEZONE — the\n" +
			"timezone every report timeframe and metric interval is resolved in, which\n" +
			"is worth reading before interpreting any dated number.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
