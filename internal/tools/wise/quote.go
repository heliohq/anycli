package wise

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newQuoteCreateCmd prices a hypothetical transfer via the unauthenticated
// quote endpoint (POST /v3/quotes): no profileId, no persistent resource — it
// returns a mid-market rate + fee estimate and moves nothing. The tool still
// sends its Bearer token (the endpoint ignores it). Exactly one of
// --source-amount / --target-amount must be given.
func (s *Service) newQuoteCreateCmd(token string) *cobra.Command {
	var sourceCurrency, targetCurrency string
	var sourceAmount, targetAmount float64
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Price a hypothetical transfer (POST /v3/quotes, unauthenticated)",
		Args:  cobra.NoArgs,
		// Non-committal pricing: the unauthenticated quote creates no persistent
		// resource and moves no money, so it is read-only from the caller's view.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sourceCurrency == "" || targetCurrency == "" {
				return &usageError{msg: "wise quote create: --source-currency and --target-currency are required"}
			}
			hasSource := cmd.Flags().Changed("source-amount")
			hasTarget := cmd.Flags().Changed("target-amount")
			if hasSource == hasTarget {
				return &usageError{msg: "wise quote create: provide exactly one of --source-amount or --target-amount"}
			}
			payload := map[string]any{
				"sourceCurrency": sourceCurrency,
				"targetCurrency": targetCurrency,
			}
			if hasSource {
				payload["sourceAmount"] = sourceAmount
			} else {
				payload["targetAmount"] = targetAmount
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/v3/quotes", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&sourceCurrency, "source-currency", "", "source currency code (required)")
	cmd.Flags().StringVar(&targetCurrency, "target-currency", "", "target currency code (required)")
	cmd.Flags().Float64Var(&sourceAmount, "source-amount", 0, "amount to send in the source currency")
	cmd.Flags().Float64Var(&targetAmount, "target-amount", 0, "amount to receive in the target currency")
	return cmd
}
