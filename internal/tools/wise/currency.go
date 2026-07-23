package wise

import (
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
)

// newCurrencyListCmd is the supported-currencies reference. Not profile-scoped.
// GET /v1/currencies
func (s *Service) newCurrencyListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List supported currencies (GET /v1/currencies)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/v1/currencies", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// intToString renders an int as a base-10 query value.
func intToString(n int) string { return strconv.Itoa(n) }
