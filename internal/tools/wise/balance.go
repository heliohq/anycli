package wise

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// defaultBalanceTypes is the balance list default. types is a required
// comma-separated Wise param; STANDARD + SAVINGS ensures money held in Jars
// (SAVINGS) is not silently under-reported.
const defaultBalanceTypes = "STANDARD,SAVINGS"

// newBalanceListCmd reads a profile's multi-currency balances.
// GET /v4/profiles/{profileId}/balances?types=STANDARD,SAVINGS
func (s *Service) newBalanceListCmd(token string) *cobra.Command {
	var profile, types string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List a profile's multi-currency balances (GET /v4/profiles/{id}/balances)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profile == "" {
				return &usageError{msg: "wise balance list: --profile is required"}
			}
			q := url.Values{}
			q.Set("types", types)
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/v4/profiles/"+url.PathEscape(profile)+"/balances", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "profile id (required)")
	cmd.Flags().StringVar(&types, "types", defaultBalanceTypes, "comma-separated balance types (STANDARD,SAVINGS)")
	return cmd
}

// newBalanceGetCmd drills into one currency balance.
// GET /v4/profiles/{profileId}/balances/{balanceId}
func (s *Service) newBalanceGetCmd(token string) *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:         "get <balanceId>",
		Short:       "Get one balance by id (GET /v4/profiles/{id}/balances/{balanceId})",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, args []string) error {
			if profile == "" {
				return &usageError{msg: "wise balance get: --profile is required"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/v4/profiles/"+url.PathEscape(profile)+"/balances/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "profile id (required)")
	return cmd
}
